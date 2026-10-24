package probe

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sopov/portping/internal/models"
)

type fakeResolver struct {
	addrs []string
	err   error
	calls int
	host  string
	ctx   context.Context
}

func (f *fakeResolver) LookupNetIP(ctx context.Context, _ string, host string) ([]netip.Addr, error) {
	f.calls++
	f.host = host
	f.ctx = ctx
	if f.err != nil {
		return nil, f.err
	}
	out := make([]netip.Addr, 0, len(f.addrs))
	for _, a := range f.addrs {
		out = append(out, netip.MustParseAddr(a))
	}
	return out, nil
}

func ipStrings(ips []models.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.IP)
	}
	return out
}

func TestParseLiteral(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want string
		is4  bool
	}{
		{"127.0.0.1", true, "127.0.0.1", true},
		{"::1", true, "::1", false},
		{"2001:db8::1", true, "2001:db8::1", false},
		{"::1:80", true, "::1:80", false},
		{"fe80::1%eth0", true, "fe80::1%eth0", false},
		{"::ffff:192.0.2.1", true, "192.0.2.1", true},
		{"example.com", false, "", false},
		{"[::1]", false, "", false},
		{"1.2.3", false, "", false},
		{"", false, "", false},
	}
	for _, tt := range tests {
		got, ok := ParseLiteral(tt.in)
		if ok != tt.ok {
			t.Errorf("ParseLiteral(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && (got.String() != tt.want || got.Is4() != tt.is4) {
			t.Errorf("ParseLiteral(%q) = %s (v4=%v), want %s (v4=%v)", tt.in, got, got.Is4(), tt.want, tt.is4)
		}
	}
}

func TestResolve_Literals(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		v4, v6  bool
		want    string
		wantV4  bool
		wantErr string
	}{
		{name: "v4 default", host: "192.0.2.1", v4: true, want: "192.0.2.1", wantV4: true},
		{name: "v6 allowed", host: "2001:db8::1", v6: true, want: "2001:db8::1"},
		{name: "v6 with zone keeps zone", host: "fe80::1%eth0", v6: true, want: "fe80::1%eth0"},
		{name: "mapped v4 is v4", host: "::ffff:192.0.2.1", v4: true, want: "192.0.2.1", wantV4: true},
		{name: "v6 literal with only v4 allowed", host: "::1", v4: true, wantErr: "IPv6 address but only IPv4"},
		{name: "v4 literal with only v6 allowed", host: "127.0.0.1", v6: true, wantErr: "IPv4 address but only IPv6"},
		{name: "both allowed v4 literal", host: "127.0.0.1", v4: true, v6: true, want: "127.0.0.1", wantV4: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeResolver{}
			cfg := &models.Config{Host: tt.host, AllowIPv4: tt.v4, AllowIPv6: tt.v6, TimeoutDur: time.Second}
			ips, err := Resolve(context.Background(), r, cfg)
			if r.calls != 0 {
				t.Errorf("literal must not hit the resolver, got %d calls", r.calls)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ips) != 1 || ips[0].IP != tt.want || ips[0].IsIPv4 != tt.wantV4 {
				t.Errorf("got %+v, want %s (v4=%v)", ips, tt.want, tt.wantV4)
			}
		})
	}
}

func TestResolve_Names(t *testing.T) {
	both := []string{"2001:db8::2", "192.0.2.10", "2001:db8::1", "192.0.2.9", "::ffff:192.0.2.10"}
	tests := []struct {
		name    string
		addrs   []string
		err     error
		v4, v6  bool
		want    []string
		wantErr string
	}{
		{name: "default v4 only, sorted numerically, deduplicated", addrs: both, v4: true, want: []string{"192.0.2.9", "192.0.2.10"}},
		{name: "v6 only", addrs: both, v6: true, want: []string{"2001:db8::1", "2001:db8::2"}},
		{name: "both families, v4 first", addrs: both, v4: true, v6: true, want: []string{"192.0.2.9", "192.0.2.10", "2001:db8::1", "2001:db8::2"}},
		{name: "only v6 answers with v4 flag", addrs: []string{"2001:db8::1"}, v4: true, wantErr: "only IPv6 addresses; add -6"},
		{name: "only v4 answers with v6 flag", addrs: []string{"192.0.2.1"}, v6: true, wantErr: "only IPv4 addresses; add -4"},
		{name: "empty answer", addrs: nil, v4: true, wantErr: "no addresses found"},
		{name: "lookup error is wrapped", err: &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}, v4: true, wantErr: "resolve host `nonexistent.invalid`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeResolver{addrs: tt.addrs, err: tt.err}
			cfg := &models.Config{Host: "nonexistent.invalid", AllowIPv4: tt.v4, AllowIPv6: tt.v6, TimeoutDur: 100 * time.Millisecond}
			ips, err := Resolve(context.Background(), r, cfg)
			if r.calls != 1 || r.host != cfg.Host {
				t.Errorf("resolver calls = %d host = %q", r.calls, r.host)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.err != nil && !errors.Is(err, tt.err) {
					t.Errorf("wrapped error lost: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := ipStrings(ips); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			for _, ip := range ips {
				a := netip.MustParseAddr(ip.IP)
				if a.Is4() != ip.IsIPv4 {
					t.Errorf("%s: IsIPv4 = %v", ip.IP, ip.IsIPv4)
				}
			}
		})
	}
}

func TestResolve_TimeoutFloor(t *testing.T) {
	r := &fakeResolver{addrs: []string{"192.0.2.1"}}
	cfg := &models.Config{Host: "nonexistent.invalid", AllowIPv4: true, TimeoutDur: 100 * time.Millisecond}
	before := time.Now()
	if _, err := Resolve(context.Background(), r, cfg); err != nil {
		t.Fatal(err)
	}
	dl, ok := r.ctx.Deadline()
	if !ok {
		t.Fatal("resolver context has no deadline")
	}
	if dl.Sub(before) < minResolveTimeout-time.Second {
		t.Errorf("resolver deadline %v after start, want at least %v", dl.Sub(before), minResolveTimeout)
	}
}

// tcpListener starts a TCP listener on the loopback interface that accepts and
// closes every connection, and returns its address.
func tcpListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}

// udpServer starts a UDP server on the loopback interface. If replySize is
// positive every datagram is answered with replySize bytes; otherwise the
// server stays silent. It returns the server address.
func udpServer(t *testing.T, replySize int) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			_, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if replySize > 0 {
				_, _ = pc.WriteTo(make([]byte, replySize), addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

// closedPort returns a loopback address whose port was just released, so a
// connection attempt is refused rather than answered.
func closedPort(t *testing.T, network string) string {
	t.Helper()
	switch network {
	case "tcp":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()
		return addr
	default:
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen packet: %v", err)
		}
		addr := pc.LocalAddr().String()
		_ = pc.Close()
		return addr
	}
}

func pingOpts(ctx context.Context, timeout time.Duration, addr string, payload []byte) models.PingOptions {
	return models.PingOptions{
		Context: ctx,
		Config:  &models.Config{TimeoutDur: timeout},
		Address: addr,
		Payload: payload,
	}
}

func TestPingTCP_Success(t *testing.T) {
	addr := tcpListener(t)
	d, err := PingTCP(pingOpts(context.Background(), 2*time.Second, addr, nil))
	if err != nil {
		t.Fatalf("PingTCP() error = %v", err)
	}
	if d <= 0 || d > 2*time.Second {
		t.Errorf("PingTCP() duration = %v, want 0 < d <= 2s", d)
	}
}

func TestPingTCP_Refused(t *testing.T) {
	addr := closedPort(t, "tcp")
	d, err := PingTCP(pingOpts(context.Background(), 2*time.Second, addr, nil))
	if err == nil {
		t.Fatal("PingTCP() expected error for closed port")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("PingTCP() error = %v, must not be reported as cancellation", err)
	}
	if d < 0 {
		t.Errorf("duration = %v, want >= 0", d)
	}
}

func TestPingTCP_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	addr := tcpListener(t)
	_, err := PingTCP(pingOpts(ctx, 2*time.Second, addr, nil))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("PingTCP() error = %v, want context.Canceled", err)
	}
}

func TestPingTCP_NilContextUsesTimeout(t *testing.T) {
	addr := tcpListener(t)
	if _, err := PingTCP(pingOpts(nil, time.Second, addr, nil)); err != nil { //nolint:staticcheck // nil context is the case under test
		t.Fatalf("PingTCP() error = %v", err)
	}
}

func TestPingUDP_EmptyPayload(t *testing.T) {
	_, err := PingUDP(pingOpts(context.Background(), time.Second, "127.0.0.1:9", nil))
	if !errors.Is(err, ErrNoPayload) {
		t.Errorf("PingUDP() error = %v, want ErrNoPayload", err)
	}
}

// A reply longer than the old 1-byte buffer must be a success on every OS
// (Windows reports truncated datagrams as WSAEMSGSIZE errors).
func TestPingUDP_LargeReplyIsSuccess(t *testing.T) {
	for _, size := range []int{1, 2, 512, 1400, 8192} {
		addr := udpServer(t, size)
		d, err := PingUDP(pingOpts(context.Background(), 2*time.Second, addr, []byte{0xab, 0xcd}))
		if err != nil {
			t.Errorf("reply of %d bytes: PingUDP() error = %v", size, err)
		}
		if d <= 0 {
			t.Errorf("reply of %d bytes: duration = %v, want > 0", size, d)
		}
	}
}

func TestPingUDP_MaxPayload(t *testing.T) {
	addr := udpServer(t, 1)
	payload := make([]byte, MaxUDPPayload)
	if _, err := PingUDP(pingOpts(context.Background(), 2*time.Second, addr, payload)); err != nil {
		t.Fatalf("PingUDP() with %d-byte payload: %v", MaxUDPPayload, err)
	}
}

func TestPingUDP_NoReplyTimesOut(t *testing.T) {
	addr := udpServer(t, 0)
	const timeout = 150 * time.Millisecond
	start := time.Now()
	d, err := PingUDP(pingOpts(context.Background(), timeout, addr, []byte{1}))
	wall := time.Since(start)
	if err == nil {
		t.Fatal("PingUDP() expected timeout error")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("PingUDP() error = %v, want a deadline error", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("timeout must not be reported as cancellation")
	}
	if d < timeout || wall > timeout+2*time.Second {
		t.Errorf("duration = %v (wall %v), want about %v", d, wall, timeout)
	}
}

func TestPingUDP_CancelWakesBlockedRead(t *testing.T) {
	addr := udpServer(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := PingUDP(pingOpts(ctx, 10*time.Second, addr, []byte{1}))
	wall := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("PingUDP() error = %v, want context.Canceled", err)
	}
	if wall > 3*time.Second {
		t.Errorf("cancel took %v, the read did not wake up", wall)
	}
}

func TestPingUDP_AlreadyCanceled(t *testing.T) {
	addr := udpServer(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PingUDP(pingOpts(ctx, 10*time.Second, addr, []byte{1}))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("PingUDP() error = %v, want context.Canceled", err)
	}
}

func TestPingUDP_ClosedPortIsFailure(t *testing.T) {
	addr := closedPort(t, "udp")
	d, err := PingUDP(pingOpts(context.Background(), time.Second, addr, []byte{1}))
	if err == nil {
		t.Fatal("PingUDP() expected error for closed port (ICMP unreachable or timeout)")
	}
	if d < 0 {
		t.Errorf("duration = %v, want >= 0", d)
	}
}

func TestProbes_ConcurrentUseIsRaceFree(t *testing.T) {
	tcpAddr := tcpListener(t)
	udpAddr := udpServer(t, 64)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, err := PingTCP(pingOpts(context.Background(), 2*time.Second, tcpAddr, nil)); err != nil {
					t.Errorf("PingTCP: %v", err)
				}
				if _, err := PingUDP(pingOpts(context.Background(), 2*time.Second, udpAddr, []byte{1, 2, 3})); err != nil {
					t.Errorf("PingUDP: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

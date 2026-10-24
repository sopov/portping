package probe

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sopov/portping/internal/models"
)

func TestGetIPv4(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *models.Config
		ip       net.IP
		expected string
		found    bool
	}{
		{
			name:     "IPv4 address allowed",
			cfg:      &models.Config{AllowIPv4: true},
			ip:       net.ParseIP("192.168.1.1"),
			expected: "192.168.1.1",
			found:    true,
		},
		{
			name:     "IPv4 address not allowed",
			cfg:      &models.Config{AllowIPv4: false},
			ip:       net.ParseIP("192.168.1.1"),
			expected: "",
			found:    false,
		},
		{
			name:     "IPv6 address",
			cfg:      &models.Config{AllowIPv4: true},
			ip:       net.ParseIP("::1"),
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := getIPv4(tt.cfg, tt.ip)
			if found != tt.found {
				t.Errorf("getIPv4() found = %v, expected %v", found, tt.found)
			}
			if result != tt.expected {
				t.Errorf("getIPv4() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestGetIPv6(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *models.Config
		ip       net.IP
		expected string
		found    bool
	}{
		{
			name:     "IPv6 address allowed",
			cfg:      &models.Config{AllowIPv6: true},
			ip:       net.ParseIP("::1"),
			expected: "::1",
			found:    true,
		},
		{
			name:     "IPv6 address not allowed",
			cfg:      &models.Config{AllowIPv6: false},
			ip:       net.ParseIP("::1"),
			expected: "",
			found:    false,
		},
		{
			name:     "IPv4 address (should not be IPv6)",
			cfg:      &models.Config{AllowIPv6: true},
			ip:       net.ParseIP("192.168.1.1"),
			expected: "",
			found:    false,
		},
		{
			name:     "Full IPv6 address",
			cfg:      &models.Config{AllowIPv6: true},
			ip:       net.ParseIP("2001:db8::1"),
			expected: "2001:db8::1",
			found:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := getIPv6(tt.cfg, tt.ip)
			if found != tt.found {
				t.Errorf("getIPv6() found = %v, expected %v", found, tt.found)
			}
			if result != tt.expected {
				t.Errorf("getIPv6() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *models.Config
		ip       net.IP
		expected models.IP
		found    bool
	}{
		{
			name: "IPv4 preferred",
			cfg:  &models.Config{AllowIPv4: true, AllowIPv6: true},
			ip:   net.ParseIP("192.168.1.1"),
			expected: models.IP{
				IP:     "192.168.1.1",
				IsIPv4: true,
			},
			found: true,
		},
		{
			name: "IPv6 when IPv4 not allowed",
			cfg:  &models.Config{AllowIPv4: false, AllowIPv6: true},
			ip:   net.ParseIP("::1"),
			expected: models.IP{
				IP:     "::1",
				IsIPv4: false,
			},
			found: true,
		},
		{
			name:     "No match when both disabled",
			cfg:      &models.Config{AllowIPv4: false, AllowIPv6: false},
			ip:       net.ParseIP("192.168.1.1"),
			expected: models.IP{},
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := getIP(tt.cfg, tt.ip)
			if found != tt.found {
				t.Errorf("getIP() found = %v, expected %v", found, tt.found)
			}
			if found {
				if result.IP != tt.expected.IP {
					t.Errorf("getIP() IP = %q, expected %q", result.IP, tt.expected.IP)
				}
				if result.IsIPv4 != tt.expected.IsIPv4 {
					t.Errorf("getIP() IsIPv4 = %v, expected %v", result.IsIPv4, tt.expected.IsIPv4)
				}
				if result.IsIPv6() != tt.expected.IsIPv6() {
					t.Errorf("getIP() IsIPv6() = %v, expected %v", result.IsIPv6(), tt.expected.IsIPv6())
				}
			}
		})
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

func TestGetAddrs_Success(t *testing.T) {
	cfg := &models.Config{
		Host:       "localhost",
		AllowIPv4:  true,
		AllowIPv6:  true,
		TimeoutDur: 5 * time.Second,
	}

	ips, err := GetAddrs(cfg)

	if err != nil {
		t.Fatalf("GetAddrs() error = %v, expected nil", err)
	}
	if len(ips) == 0 {
		t.Error("GetAddrs() returned empty slice, expected at least one IP")
	}
	// Verify that all IPs are valid
	for _, ip := range ips {
		if ip.IP == "" {
			t.Error("GetAddrs() returned IP with empty IP field")
		}
		if !ip.IsIPv4 && !ip.IsIPv6() {
			t.Error("GetAddrs() returned IP with neither IsIPv4 nor IsIPv6 set")
		}
		if ip.IsIPv4 && ip.IsIPv6() {
			t.Error("GetAddrs() returned IP with both IsIPv4 and IsIPv6 set")
		}
	}
}

func TestGetAddrs_IPv4Only(t *testing.T) {
	cfg := &models.Config{
		Host:       "localhost",
		AllowIPv4:  true,
		AllowIPv6:  false,
		TimeoutDur: 5 * time.Second,
	}

	ips, err := GetAddrs(cfg)

	if err != nil {
		t.Fatalf("GetAddrs() error = %v, expected nil", err)
	}
	// Verify that all IPs are IPv4
	for _, ip := range ips {
		if !ip.IsIPv4 {
			t.Errorf("GetAddrs() returned non-IPv4 IP %s when AllowIPv6=false", ip.IP)
		}
		if ip.IsIPv6() {
			t.Errorf("GetAddrs() returned IPv6 IP %s when AllowIPv6=false", ip.IP)
		}
	}
}

func TestGetAddrs_IPv6Only(t *testing.T) {
	cfg := &models.Config{
		Host:       "localhost",
		AllowIPv4:  false,
		AllowIPv6:  true,
		TimeoutDur: 5 * time.Second,
	}

	ips, err := GetAddrs(cfg)

	// May get "no addresses found" error if no IPv6, or success if IPv6 exists
	if err != nil {
		if err.Error() != "no addresses found" {
			t.Errorf("GetAddrs() error = %v, expected 'no addresses found' or nil", err)
		}
		return
	}
	// If success, verify that all IPs are IPv6
	for _, ip := range ips {
		if !ip.IsIPv6() {
			t.Errorf("GetAddrs() returned non-IPv6 IP %s when AllowIPv4=false", ip.IP)
		}
		if ip.IsIPv4 {
			t.Errorf("GetAddrs() returned IPv4 IP %s when AllowIPv4=false", ip.IP)
		}
	}
}

func TestGetAddrs_InvalidHost(t *testing.T) {
	cfg := &models.Config{
		Host:      "invalid-host-that-does-not-exist-12345.example",
		AllowIPv4: true,
		AllowIPv6: true,
	}

	ips, err := GetAddrs(cfg)

	if err == nil {
		t.Error("GetAddrs() expected error for invalid host, got nil")
	}
	if len(ips) != 0 {
		t.Errorf("GetAddrs() expected empty slice on error, got %d IPs", len(ips))
	}
}

func TestGetAddrs_NoAddressesAfterFilter(t *testing.T) {
	cfg := &models.Config{
		Host:       "localhost",
		AllowIPv4:  false,
		AllowIPv6:  false, // Disable both types
		TimeoutDur: 5 * time.Second,
	}

	ips, err := GetAddrs(cfg)

	// Should get "no addresses found" error
	if err == nil {
		t.Error("GetAddrs() expected error when both IPv4 and IPv6 disabled, got nil")
	}
	if err != nil && err.Error() != "no addresses found" {
		t.Errorf("GetAddrs() error = %v, expected 'no addresses found'", err)
	}
	if len(ips) != 0 {
		t.Errorf("GetAddrs() expected empty slice, got %d IPs", len(ips))
	}
}

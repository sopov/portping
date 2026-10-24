package app

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/sopov/portping/internal/models"
)

func noColor(t *testing.T) {
	t.Helper()
	orig := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = orig })
}

// listen returns host and port of a loopback TCP listener that accepts and
// closes connections.
func listen(t *testing.T) (string, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	return host, port
}

// closedPort returns a loopback TCP port that was just released.
func closedPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	return port
}

func tcpConfig(port string, count int, ips ...models.IP) *models.Config {
	return &models.Config{
		Proto:      models.TCP,
		Host:       "127.0.0.1",
		Port:       port,
		Count:      count,
		Nonstop:    count == 0,
		TimeoutDur: 500 * time.Millisecond,
		DelayDur:   10 * time.Millisecond,
		IPs:        ips,
	}
}

func TestNewApp(t *testing.T) {
	cfg := tcpConfig("80", 1, models.IP{IP: "192.0.2.1", IsIPv4: true})
	a := NewApp(context.Background(), cfg, io.Discard)
	if a == nil || a.cfg != cfg || len(a.stats) != 0 {
		t.Fatalf("NewApp() = %+v", a)
	}
}

func TestRun_CountedSuccess(t *testing.T) {
	noColor(t)
	host, port := listen(t)
	cfg := tcpConfig(port, 3, models.IP{IP: host, IsIPv4: true})
	var out bytes.Buffer
	a := NewApp(context.Background(), cfg, &out)
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	s := a.Summary()
	if s.Targets != 1 || s.Attempts != 3 || s.Connects != 3 || s.Failures != 0 || s.Unreached != 0 || !s.AllReached() {
		t.Errorf("Summary = %+v", s)
	}
	text := out.String()
	if !strings.HasPrefix(text, "Ping of 127.0.0.1 on tcp "+port+" (1 IP)\nIPv4: 127.0.0.1\n") {
		t.Errorf("banner: %q", text)
	}
	if strings.Count(text, "\n  1\t") != 1 || strings.Count(text, "\n  3\t") != 1 || strings.Contains(text, "\n  4\t") {
		t.Errorf("expected exactly three probe rows:\n%s", text)
	}
	if !strings.Contains(text, "Statistics of ping 127.0.0.1 on tcp "+port) {
		t.Errorf("missing statistics table:\n%s", text)
	}
}

func TestRun_AllFailures(t *testing.T) {
	noColor(t)
	port := closedPort(t)
	cfg := tcpConfig(port, 2, models.IP{IP: "127.0.0.1", IsIPv4: true})
	var out bytes.Buffer
	a := NewApp(context.Background(), cfg, &out)
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	s := a.Summary()
	if s.Attempts != 2 || s.Connects != 0 || s.Failures != 2 || s.Unreached != 1 || s.AllReached() {
		t.Errorf("Summary = %+v", s)
	}
	if strings.Count(out.String(), "Err: ") != 2 {
		t.Errorf("expected two error rows:\n%s", out.String())
	}
}

func TestRun_MultipleAddressesPartialFailure(t *testing.T) {
	noColor(t)
	host, port := listen(t)
	// Second address: IPv6 loopback on the same port, which has no listener.
	// If IPv6 loopback is unavailable the dial fails too, which is the same
	// outcome for this test.
	cfg := tcpConfig(port, 2, models.IP{IP: host, IsIPv4: true}, models.IP{IP: "::1"})
	var out bytes.Buffer
	a := NewApp(context.Background(), cfg, &out)
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	s := a.Summary()
	if s.Targets != 2 || s.Attempts != 4 || s.Connects != 2 || s.Unreached != 1 || s.AllReached() {
		t.Errorf("Summary = %+v", s)
	}
	text := out.String()
	for _, row := range []string{"\n1.1\t", "\n1.2\t", "\n2.1\t", "\n2.2\t"} {
		if !strings.Contains(text, row) {
			t.Errorf("missing row %q:\n%s", row, text)
		}
	}
}

func TestRun_CancelDuringDelay(t *testing.T) {
	host, port := listen(t)
	cfg := tcpConfig(port, 0, models.IP{IP: host, IsIPv4: true})
	cfg.DelayDur = 10 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	a := NewApp(ctx, cfg, io.Discard)
	start := time.Now()
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("Run did not stop on cancel during the delay")
	}
	if s := a.Summary(); s.Attempts != 1 || s.Connects != 1 {
		t.Errorf("Summary = %+v", s)
	}
}

func TestRun_CancelBeforeStart(t *testing.T) {
	host, port := listen(t)
	cfg := tcpConfig(port, 0, models.IP{IP: host, IsIPv4: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := NewApp(ctx, cfg, io.Discard)
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if s := a.Summary(); s.Attempts != 0 || s.AllReached() {
		t.Errorf("Summary = %+v", s)
	}
}

func TestRun_InterruptedProbeNotCounted(t *testing.T) {
	// A UDP probe to a silent listener blocks until cancelled; the
	// interrupted probe must not appear as a failure.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	_, port, _ := net.SplitHostPort(pc.LocalAddr().String())
	cfg := &models.Config{
		Proto: models.UDP, Host: "127.0.0.1", Port: port, Nonstop: true,
		TimeoutDur: 10 * time.Second, DelayDur: time.Millisecond,
		UDPPayload: []byte{1}, IPs: []models.IP{{IP: "127.0.0.1", IsIPv4: true}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	a := NewApp(ctx, cfg, io.Discard)
	start := time.Now()
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("Run did not stop promptly on cancel")
	}
	if s := a.Summary(); s.Attempts != 0 || s.Failures != 0 {
		t.Errorf("interrupted probe was counted: %+v", s)
	}
}

func TestRun_ConcurrentAppsAreRaceFree(t *testing.T) {
	host, port := listen(t)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := tcpConfig(port, 3, models.IP{IP: host, IsIPv4: true})
			cfg.DelayDur = time.Millisecond
			a := NewApp(context.Background(), cfg, io.Discard)
			if err := a.Run(); err != nil {
				t.Error(err)
			}
			if !a.Summary().AllReached() {
				t.Error("expected success")
			}
		}()
	}
	wg.Wait()
}

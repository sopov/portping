package probe

import (
	"context"
	"encoding/hex"
	"github.com/sopov/portping/internal/models"
	"net"
	"testing"
	"time"
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

func TestPingTCP_WithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context immediately

	cfg := &models.Config{
		Proto:      models.TCP,
		TimeoutDur: 100 * time.Millisecond,
	}

	opts := models.PingOptions{
		Context: ctx,
		Config:  cfg,
		Address: "127.0.0.1:99999", // Unreachable port for quick timeout
	}

	// Test should return quickly due to context cancellation
	duration, err := PingTCP(opts)

	if duration < 0 {
		t.Errorf("PingTCP() duration should be >= 0, got %v", duration)
	}
	// May get context.Canceled or connection error
	_ = err
}

func TestPingTCP_WithoutContext(t *testing.T) {
	cfg := &models.Config{
		Proto:      models.TCP,
		TimeoutDur: 100 * time.Millisecond,
	}

	opts := models.PingOptions{
		Context: context.Background(),
		Config:  cfg,
		Address: "127.0.0.1:99999", // Unreachable port
	}

	duration, err := PingTCP(opts)

	if duration < 0 {
		t.Errorf("PingTCP() duration should be >= 0, got %v", duration)
	}
	// Expect connection error
	if err == nil {
		t.Error("PingTCP() expected error for unreachable address, got nil")
	}
}

func TestPingUDP(t *testing.T) {
	cfg := &models.Config{
		TimeoutDur: 1000 * time.Millisecond,
	}

	opts := models.PingOptions{
		Context: context.Background(),
		Config:  cfg,
		Address: "127.0.0.1:99999", // Unreachable address
		Payload: []byte("test"),    // Test payload
	}

	// PingUDP now requires payload and address
	duration, err := PingUDP(opts)

	if duration < 0 {
		t.Errorf("PingUDP() duration should be >= 0, got %v", duration)
	}
	// Expect error for unreachable address or no response
	if err == nil {
		t.Error("PingUDP() expected error for unreachable address or no response, got nil")
	}
}

func TestPingTCP_RealHost(t *testing.T) {
	// Real TCP ping to one.one.one.one (Cloudflare DNS)
	cfg := &models.Config{
		Proto:      models.TCP,
		TimeoutDur: 5 * time.Second,
	}

	opts := models.PingOptions{
		Context: context.Background(),
		Config:  cfg,
		Address: "1.1.1.1:443", // Cloudflare DNS on port 443
	}

	duration, err := PingTCP(opts)

	if duration < 0 {
		t.Errorf("PingTCP() duration should be >= 0, got %v", duration)
	}
	if err != nil {
		t.Errorf("PingTCP() to one.one.one.one:443 failed: %v", err)
	}
	if duration > cfg.TimeoutDur {
		t.Errorf("PingTCP() duration = %v, should be < timeout %v", duration, cfg.TimeoutDur)
	}
	// Verify connection was successful
	if err == nil && duration > 0 {
		t.Logf("Successfully connected to 1.1.1.1:443 in %v", duration)
	}
}

func TestPingUDP_DNSQuery(t *testing.T) {
	// UDP test with DNS A record query on port 53
	// DNS A record query for example.com in hex format (string)
	// Format: ID (1234) + Flags (0100) + Questions (0001) +
	// Answer/Authority/Additional RRs (000000000000) +
	// Question: example.com (076578616d706c6503636f6d00) + Type A (0001) + Class IN (0001)
	dnsQueryHex := "123401000001000000000000076578616d706c6503636f6d0000010001"

	// Decode hex string to bytes
	dnsQuery, err := hex.DecodeString(dnsQueryHex)
	if err != nil {
		t.Fatalf("Failed to decode DNS query hex string: %v", err)
	}

	cfg := &models.Config{
		Proto:      models.UDP,
		TimeoutDur: 5 * time.Second,
	}

	opts := models.PingOptions{
		Context: context.Background(),
		Config:  cfg,
		Address: "1.1.1.1:53", // Cloudflare DNS
		Payload: dnsQuery,     // Decoded DNS query
	}

	duration, err := PingUDP(opts)

	// Verify that request was sent and response received:
	// 1. Request should be successful (err == nil)
	if err != nil {
		t.Errorf("PingUDP() failed: %v (expected success for DNS query)", err)
	}

	// 2. Response should be fast (duration < timeout)
	if duration >= cfg.TimeoutDur {
		t.Errorf("PingUDP() duration = %v, expected < %v (should be fast)", duration, cfg.TimeoutDur)
	}

	// 3. Duration should be reasonable (usually DNS response comes in < 1 second)
	if duration > 1*time.Second {
		t.Errorf("PingUDP() duration = %v, expected < 1s for DNS query", duration)
	}

	// 4. Duration should be > 0 (real request was sent)
	if duration <= 0 {
		t.Errorf("PingUDP() duration = %v, expected > 0", duration)
	}

	// If all checks passed, log success
	if err == nil && duration > 0 && duration < cfg.TimeoutDur {
		t.Logf("Successfully sent DNS query to 1.1.1.1:53 and received response in %v", duration)
	}
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

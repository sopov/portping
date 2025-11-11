package cli

import (
	"flag"
	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseStrings(t *testing.T) {
	tests := []struct {
		name            string
		args            []string // Command line arguments (without command)
		isUDP           bool
		expectedHost    string
		expectedPort    string
		expectedPayload string
	}{
		// Standard format: host port
		{
			name:            "Standard format: host port",
			args:            []string{"example.com", "80"},
			isUDP:           false,
			expectedHost:    "example.com",
			expectedPort:    "80",
			expectedPayload: "",
		},
		{
			name:            "Standard format with UDP",
			args:            []string{"-udp", "example.com", "53", "payload"},
			isUDP:           true,
			expectedHost:    "example.com",
			expectedPort:    "53",
			expectedPayload: "payload",
		},

		// Format host:port
		{
			name:            "Hostname with port",
			args:            []string{"example.com:80"},
			isUDP:           false,
			expectedHost:    "example.com",
			expectedPort:    "80",
			expectedPayload: "",
		},
		{
			name:            "Hostname with port and UDP payload",
			args:            []string{"-udp", "example.com:53", "payload"},
			isUDP:           true,
			expectedHost:    "example.com",
			expectedPort:    "53",
			expectedPayload: "payload",
		},

		// IPv4 with port
		{
			name:            "IPv4 with port",
			args:            []string{"192.168.1.1:8080"},
			isUDP:           false,
			expectedHost:    "192.168.1.1",
			expectedPort:    "8080",
			expectedPayload: "",
		},
		{
			name:            "IPv4 with high port",
			args:            []string{"192.168.1.1:65535"},
			isUDP:           false,
			expectedHost:    "192.168.1.1",
			expectedPort:    "65535",
			expectedPayload: "",
		},

		// IPv6 with brackets
		{
			name:            "IPv6 with brackets without port",
			args:            []string{"[::1]", "80"},
			isUDP:           false,
			expectedHost:    "::1",
			expectedPort:    "80",
			expectedPayload: "",
		},
		{
			name:            "IPv6 with brackets and port",
			args:            []string{"[::1]:80"},
			isUDP:           false,
			expectedHost:    "::1",
			expectedPort:    "80",
			expectedPayload: "",
		},
		{
			name:            "IPv6 full address with brackets and port",
			args:            []string{"[2001:db8::1]:443"},
			isUDP:           false,
			expectedHost:    "2001:db8::1",
			expectedPort:    "443",
			expectedPayload: "",
		},
		{
			name:            "IPv6 with brackets, port and UDP payload",
			args:            []string{"-udp", "[::1]:53", "test-payload"},
			isUDP:           true,
			expectedHost:    "::1",
			expectedPort:    "53",
			expectedPayload: "test-payload",
		},

		// IPv6 without brackets but with port (edge case)
		{
			name:            "IPv6 without brackets but with port",
			args:            []string{"::1:80"},
			isUDP:           false,
			expectedHost:    "::1",
			expectedPort:    "80",
			expectedPayload: "",
		},

		// Subdomains and complex names
		{
			name:            "Subdomain with port",
			args:            []string{"subdomain.example.com:443"},
			isUDP:           false,
			expectedHost:    "subdomain.example.com",
			expectedPort:    "443",
			expectedPayload: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			cfg := &models.Config{}
			if tt.isUDP {
				cfg.Proto = models.UDP
			}
			initFlags(fs, cfg)

			// Parse flags from args
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Parse arguments
			if err := parseArgs(fs, cfg); err != nil {
				t.Fatalf("parseArgs failed: %v", err)
			}

			if cfg.Host != tt.expectedHost {
				t.Errorf("Host: expected '%s', got '%s'", tt.expectedHost, cfg.Host)
			}
			if cfg.Port != tt.expectedPort {
				t.Errorf("Port: expected '%s', got '%s'", tt.expectedPort, cfg.Port)
			}
			if tt.isUDP {
				// parseArgs sets UDPPayloadHex, not Preset
				if cfg.UDPPayloadHex != tt.expectedPayload {
					t.Errorf("UDP Payload Hex: expected '%s', got '%s'", tt.expectedPayload, cfg.UDPPayloadHex)
				}
			}
		})
	}
}

func TestParse(t *testing.T) {
	// Save original flag.CommandLine
	originalFlagSet := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlagSet
	}()

	os.Args = []string{"portping", "-t", "2000", "-d", "500", "example.com", "80"}
	// Reset flag.CommandLine so Parse() can create flags from scratch
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	// Don't parse here, Parse() will call flag.Parse() itself

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Parse() returned nil")
	}
	if cfg.Timeout != 2000 {
		t.Errorf("Expected Timeout = 2000, got %d", cfg.Timeout)
	}
	if cfg.Delay != 500 {
		t.Errorf("Expected Delay = 500, got %d", cfg.Delay)
	}
	if cfg.TimeoutDur != 2000*time.Millisecond {
		t.Errorf("Expected TimeoutDur = 2000ms, got %v", cfg.TimeoutDur)
	}
	if cfg.DelayDur != 500*time.Millisecond {
		t.Errorf("Expected DelayDur = 500ms, got %v", cfg.DelayDur)
	}
	if cfg.Host != "example.com" {
		t.Errorf("Expected Host = 'example.com', got '%s'", cfg.Host)
	}
	if cfg.Port != "80" {
		t.Errorf("Expected Port = '80', got '%s'", cfg.Port)
	}
	if cfg.Proto != models.TCP {
		t.Errorf("Expected Proto = TCP, got %s", cfg.Proto)
	}
	if !cfg.Nonstop {
		t.Error("Expected Nonstop = true when Count = 0")
	}
}

func TestParse_UDP(t *testing.T) {
	originalFlagSet := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlagSet
	}()

	os.Args = []string{"portping", "-udp", "example.com:53", "payload"}
	// Reset flag.CommandLine so Parse() can create flags from scratch
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	// Don't parse here, Parse() will call flag.Parse() itself

	cfg, err := Parse()
	// Parse() now returns error if UDP payload hex string is invalid
	if err == nil {
		t.Error("Expected error for invalid hex string 'payload', got nil")
	}
	if err != nil && err.Error() != "invalid UDP payload, should be hex string" {
		t.Errorf("Expected error 'invalid UDP payload, should be hex string', got: %v", err)
	}
	// If error occurred, cfg might be nil or partially filled
	if err == nil {
		if cfg.Proto != models.UDP {
			t.Errorf("Expected Proto = UDP, got %s", cfg.Proto)
		}
		if cfg.UDPPayloadHex != "payload" {
			t.Errorf("Expected UDPPayloadHex = 'payload', got '%s'", cfg.UDPPayloadHex)
		}
	}
}

func TestParse_WithCount(t *testing.T) {
	originalFlagSet := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlagSet
	}()

	os.Args = []string{"portping", "-c", "10", "example.com", "80"}
	// Reset flag.CommandLine so Parse() can create flags from scratch
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	// Don't parse here, Parse() will call flag.Parse() itself

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse() returned error: %v", err)
	}

	if cfg.Count != 10 {
		t.Errorf("Expected Count = 10, got %d", cfg.Count)
	}
	if cfg.Nonstop {
		t.Error("Expected Nonstop = false when Count > 0")
	}
}

func TestParse_PresetBoolShortcut(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "-dns", "example.com:53"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Proto != models.UDP {
		t.Errorf("expected UDP proto, got %v", cfg.Proto)
	}

	if cfg.Port == "" {
		t.Error("expected port from preset")
	}

	if cfg.UDPPayloadHex == "" {
		t.Error("expected payload from preset")
	}

	// Verify DNS preset values
	if cfg.Port != "53" {
		t.Errorf("expected port 53 from DNS preset, got %q", cfg.Port)
	}

	expectedPayload := probe.Predefined["dns"].UDPPayloadHex
	if cfg.UDPPayloadHex != expectedPayload {
		t.Errorf("expected DNS preset payload, got %q", cfg.UDPPayloadHex)
	}
}

func TestParse_PresetOverriddenByArgPayload(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "-preset", "dns", "example.com:53", "abcd"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.UDPPayloadHex != "abcd" {
		t.Errorf("expected UDPPayloadHex to be overridden by arg, got %q", cfg.UDPPayloadHex)
	}

	// Port should still come from preset if not specified in host:port
	if cfg.Port != "53" {
		t.Errorf("expected port 53 from DNS preset, got %q", cfg.Port)
	}
}

func TestParse_IPv4Flag(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "-4", "example.com", "80"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.AllowIPv4 {
		t.Error("expected AllowIPv4=true when -4 flag is set")
	}
	if cfg.AllowIPv6 {
		t.Error("expected AllowIPv6=false when only -4 flag is set")
	}
}

func TestParse_IPv6Flag(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "-6", "example.com", "80"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.AllowIPv6 {
		t.Error("expected AllowIPv6=true when -6 flag is set")
	}
	if cfg.AllowIPv4 {
		t.Error("expected AllowIPv4=false when only -6 flag is set (v4 not explicitly enabled)")
	}
}

func TestParse_IPv4AndIPv6Flags(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "-4", "-6", "example.com", "80"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.AllowIPv4 {
		t.Error("expected AllowIPv4=true when both -4 and -6 flags are set")
	}
	if !cfg.AllowIPv6 {
		t.Error("expected AllowIPv6=true when both -4 and -6 flags are set")
	}
}

func TestParse_DefaultIPv4(t *testing.T) {
	original := flag.CommandLine
	defer func() { flag.CommandLine = original }()

	os.Args = []string{"portping", "example.com", "80"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.AllowIPv4 {
		t.Error("expected AllowIPv4=true by default")
	}
	if cfg.AllowIPv6 {
		t.Error("expected AllowIPv6=false by default")
	}
}

func TestUsage(t *testing.T) {
	originalFlagSet := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlagSet
	}()

	flag.CommandLine = flag.NewFlagSet("portping", flag.ContinueOnError)
	usage := Usage()

	if usage == "" {
		t.Error("Usage() returned empty string")
	}
	if !strings.Contains(usage, app.Name) {
		t.Errorf("Usage should contain app name '%s'", app.Name)
	}
	if !strings.Contains(usage, app.Version) {
		t.Errorf("Usage should contain app version '%s'", app.Version)
	}
	if !strings.Contains(usage, "destination") {
		t.Error("Usage should contain 'destination'")
	}
	if !strings.Contains(usage, "port") {
		t.Error("Usage should contain 'port'")
	}
}

func TestValidate_EmptyHost(t *testing.T) {
	cfg := &models.Config{
		Host:    "",
		Port:    "80",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for empty host, got nil")
	}
}

func TestValidate_EmptyPort(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for empty port, got nil")
	}
}

func TestValidate_InvalidPort_TooLow(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "0",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for invalid port 0, got nil")
	}
}

func TestValidate_InvalidPort_TooHigh(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "65536",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for invalid port 65536, got nil")
	}
}

func TestValidate_InvalidPort_NotANumber(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "abc",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for invalid port 'abc', got nil")
	}
}

func TestValidate_InvalidTimeout(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Timeout: 0,
		Delay:   1000,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for timeout 0, got nil")
	}
}

func TestValidate_InvalidDelay(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Timeout: 1000,
		Delay:   0,
		Count:   0,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for delay 0, got nil")
	}
}

func TestValidate_InvalidCount(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Timeout: 1000,
		Delay:   1000,
		Count:   -1,
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected error for count -1, got nil")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Timeout: 1000,
		Delay:   1000,
		Count:   0,
	}

	// This test may fail if example.com is unreachable or doesn't resolve
	err := Validate(cfg)
	// May get error from probe.GetAddrs, but not from validation
	if err != nil {
		// Verify that error is not related to port validation
		if err.Error() == "invalid port `80`" {
			t.Errorf("Unexpected validation error: %v", err)
		}
	}
}

func TestSortIPs_IPv4BeforeIPv6(t *testing.T) {
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "::1", IsIPv4: false},
			{IP: "192.168.1.1", IsIPv4: true},
			{IP: "2001:db8::1", IsIPv4: false},
			{IP: "10.0.0.1", IsIPv4: true},
		},
	}

	SortIPs(cfg)

	expected := []models.IP{
		{IP: "10.0.0.1", IsIPv4: true},
		{IP: "192.168.1.1", IsIPv4: true},
		{IP: "2001:db8::1", IsIPv4: false},
		{IP: "::1", IsIPv4: false},
	}

	if len(cfg.IPs) != len(expected) {
		t.Errorf("Expected %d IPs, got %d", len(expected), len(cfg.IPs))
		return
	}

	for i, exp := range expected {
		if cfg.IPs[i].IP != exp.IP {
			t.Errorf("Position %d: expected IP '%s', got '%s'", i, exp.IP, cfg.IPs[i].IP)
		}
		if cfg.IPs[i].IsIPv4 != exp.IsIPv4 {
			t.Errorf("Position %d: expected IsIPv4 %v, got %v", i, exp.IsIPv4, cfg.IPs[i].IsIPv4)
		}
		if cfg.IPs[i].IsIPv6() != exp.IsIPv6() {
			t.Errorf("Position %d: expected IsIPv6 %v, got %v", i, exp.IsIPv6(), cfg.IPs[i].IsIPv6())
		}
	}
}

func TestSortIPs_WithinGroup(t *testing.T) {
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "192.168.1.10", IsIPv4: true},
			{IP: "192.168.1.1", IsIPv4: true},
			{IP: "10.0.0.2", IsIPv4: true},
			{IP: "10.0.0.1", IsIPv4: true},
		},
	}

	SortIPs(cfg)

	expected := []string{"10.0.0.1", "10.0.0.2", "192.168.1.1", "192.168.1.10"}

	for i, exp := range expected {
		if cfg.IPs[i].IP != exp {
			t.Errorf("Position %d: expected IP '%s', got '%s'", i, exp, cfg.IPs[i].IP)
		}
	}
}

func TestSortIPs_IPv6Only(t *testing.T) {
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "2001:db8::2", IsIPv4: false},
			{IP: "::1", IsIPv4: false},
			{IP: "2001:db8::1", IsIPv4: false},
		},
	}

	SortIPs(cfg)

	expected := []string{"2001:db8::1", "2001:db8::2", "::1"}

	for i, exp := range expected {
		if cfg.IPs[i].IP != exp {
			t.Errorf("Position %d: expected IP '%s', got '%s'", i, exp, cfg.IPs[i].IP)
		}
	}
}

func TestSortIPs_Empty(t *testing.T) {
	cfg := &models.Config{
		IPs: []models.IP{},
	}

	SortIPs(cfg)

	if len(cfg.IPs) != 0 {
		t.Errorf("Expected empty IPs, got %d", len(cfg.IPs))
	}
}

func TestSortIPs_SingleIP(t *testing.T) {
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "192.168.1.1", IsIPv4: true},
		},
	}

	SortIPs(cfg)

	if len(cfg.IPs) != 1 {
		t.Errorf("Expected 1 IP, got %d", len(cfg.IPs))
	}
	if cfg.IPs[0].IP != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got '%s'", cfg.IPs[0].IP)
	}
}

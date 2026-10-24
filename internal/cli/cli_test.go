package cli

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
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

		// An unbracketed IPv6 literal is an address, never host:port
		{
			name:            "Unbracketed IPv6 literal that looks like host:port",
			args:            []string{"::1:80"},
			isUDP:           false,
			expectedHost:    "::1:80",
			expectedPort:    "",
			expectedPayload: "",
		},
		{
			name:         "Bare IPv6 loopback with separate port",
			args:         []string{"::1", "80"},
			expectedHost: "::1",
			expectedPort: "80",
		},
		{
			name:         "Bare full IPv6 with separate port",
			args:         []string{"2001:db8::1", "443"},
			expectedHost: "2001:db8::1",
			expectedPort: "443",
		},
		{
			name:         "IPv6 with zone, bracketed with port",
			args:         []string{"[fe80::1%eth0]:22"},
			expectedHost: "fe80::1%eth0",
			expectedPort: "22",
		},
		{
			name:         "IPv6 with zone, bare with separate port",
			args:         []string{"fe80::1%eth0", "22"},
			expectedHost: "fe80::1%eth0",
			expectedPort: "22",
		},
		{
			name:         "IPv4-mapped IPv6 literal",
			args:         []string{"::ffff:192.0.2.1", "80"},
			expectedHost: "::ffff:192.0.2.1",
			expectedPort: "80",
		},
		{
			name:            "UDP: host, port and payload",
			args:            []string{"-udp", "192.0.2.1", "53", "abcd"},
			isUDP:           true,
			expectedHost:    "192.0.2.1",
			expectedPort:    "53",
			expectedPayload: "abcd",
		},
		{
			name:            "UDP: bare IPv6, port and payload",
			args:            []string{"-udp", "2001:db8::1", "53", "abcd"},
			isUDP:           true,
			expectedHost:    "2001:db8::1",
			expectedPort:    "53",
			expectedPayload: "abcd",
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

func TestParseArgs_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unterminated bracket", []string{"[::1"}, "invalid address"},
		{"bracket without literal", []string{"[example.com]:80"}, "invalid IPv6 literal"},
		{"bracket with port but garbage", []string{"[zz]:80"}, "invalid IPv6 literal"},
		{"missing host", []string{":80"}, "invalid address"},
		{"missing port after colon", []string{"example.com:"}, "invalid address"},
		{"unbracketed v6 with port hint", []string{"2001:db8::1:zz"}, "[address]:port"},
		{"extra argument tcp", []string{"example.com", "80", "junk"}, "unexpected argument"},
		{"extra argument host:port", []string{"example.com:80", "443"}, "unexpected argument"},
		{"extra argument udp", []string{"-udp", "example.com", "53", "abcd", "junk"}, "unexpected argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			cfg := &models.Config{}
			initFlags(fs, cfg)
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("flag parse: %v", err)
			}
			err := parseArgs(fs, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parseArgs(%v) error = %v, want containing %q", tt.args, err, tt.want)
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

	os.Args = []string{"portping", "-t", "2000", "-d", "500", "127.0.0.1", "80"}
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
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected Host = '127.0.0.1', got '%s'", cfg.Host)
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

	os.Args = []string{"portping", "-udp", "127.0.0.1:53", "payload"}
	// Reset flag.CommandLine so Parse() can create flags from scratch
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	// Don't parse here, Parse() will call flag.Parse() itself

	cfg, err := Parse()
	// Parse() now returns error if UDP payload hex string is invalid
	if err == nil {
		t.Error("Expected error for invalid hex string 'payload', got nil")
	}
	if err != nil && !strings.HasPrefix(err.Error(), "invalid UDP payload, should be hex string") {
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

	os.Args = []string{"portping", "-c", "10", "127.0.0.1", "80"}
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

	os.Args = []string{"portping", "-dns", "127.0.0.1:53"}
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

	os.Args = []string{"portping", "-preset", "dns", "127.0.0.1:53", "abcd"}
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

	os.Args = []string{"portping", "-4", "127.0.0.1", "80"}
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

	os.Args = []string{"portping", "-6", "::1", "80"}
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

	os.Args = []string{"portping", "-4", "-6", "127.0.0.1", "80"}
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

	os.Args = []string{"portping", "127.0.0.1", "80"}
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

// parseArgsWith runs the full parse pipeline on args with a private FlagSet.
func parseArgsWith(t *testing.T, args ...string) (*models.Config, error) {
	t.Helper()
	fs := flag.NewFlagSet("portping", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return parseWith(fs, args)
}

func TestParse_Validation(t *testing.T) {
	big := strings.Repeat("00", probe.MaxUDPPayload+1)
	tests := []struct {
		name    string
		args    []string
		wantErr string
		check   func(t *testing.T, cfg *models.Config)
	}{
		{name: "timeout zero", args: []string{"-t", "0", "127.0.0.1", "80"}, wantErr: "timeout must be between"},
		{name: "timeout overflow", args: []string{"-t", "99999999999999999", "127.0.0.1", "80"}, wantErr: "timeout must be between"},
		{name: "timeout above 24h", args: []string{"-t", "86400001", "127.0.0.1", "80"}, wantErr: "timeout must be between"},
		{name: "delay zero", args: []string{"-d", "0", "127.0.0.1", "80"}, wantErr: "delay must be between"},
		{name: "delay overflow", args: []string{"-d", "99999999999999999", "127.0.0.1", "80"}, wantErr: "delay must be between"},
		{name: "count negative", args: []string{"-c", "-1", "127.0.0.1", "80"}, wantErr: "count must be"},
		{name: "port missing", args: []string{"127.0.0.1"}, wantErr: "port is required"},
		{name: "port zero", args: []string{"127.0.0.1", "0"}, wantErr: "invalid port"},
		{name: "port signed", args: []string{"127.0.0.1", "+80"}, wantErr: "invalid port"},
		{name: "port too large", args: []string{"127.0.0.1:65536"}, wantErr: "invalid port"},
		{name: "port canonicalised", args: []string{"127.0.0.1", "0080"}, check: func(t *testing.T, cfg *models.Config) {
			if cfg.Port != "80" {
				t.Errorf("Port = %q, want 80", cfg.Port)
			}
		}},
		{name: "udp without payload", args: []string{"-udp", "127.0.0.1", "53"}, wantErr: "UDP payload is required"},
		{name: "udp odd-length payload", args: []string{"-udp", "127.0.0.1", "53", "abc"}, wantErr: "invalid UDP payload"},
		{name: "udp 0x prefix rejected", args: []string{"-udp", "127.0.0.1", "53", "0xabcd"}, wantErr: "invalid UDP payload"},
		{name: "udp payload too large", args: []string{"-udp", "127.0.0.1", "53", big}, wantErr: "maximum is 65507"},
		{name: "udp payload at limit", args: []string{"-udp", "127.0.0.1", "53", strings.Repeat("00", probe.MaxUDPPayload)}, check: func(t *testing.T, cfg *models.Config) {
			if len(cfg.UDPPayload) != probe.MaxUDPPayload {
				t.Errorf("payload length = %d", len(cfg.UDPPayload))
			}
		}},
		{name: "payload flag with tcp", args: []string{"-payload", "abcd", "127.0.0.1", "80"}, wantErr: "-payload requires UDP"},
		{name: "payload flag with tcp preset", args: []string{"-http", "-payload", "abcd", "127.0.0.1"}, wantErr: "-payload requires UDP"},
		{name: "payload flag with udp preset", args: []string{"-dns", "-payload", "abcd", "127.0.0.1"}, check: func(t *testing.T, cfg *models.Config) {
			if cfg.UDPPayloadHex != "abcd" || cfg.Port != "53" {
				t.Errorf("payload %q port %q", cfg.UDPPayloadHex, cfg.Port)
			}
		}},
		{name: "positional payload with tcp is unexpected", args: []string{"127.0.0.1", "80", "abcd"}, wantErr: "unexpected argument"},
		{name: "preset case-insensitive", args: []string{"-preset", "DNS", "127.0.0.1"}, check: func(t *testing.T, cfg *models.Config) {
			if cfg.Preset != "dns" || cfg.Port != "53" || !cfg.IsUDP() {
				t.Errorf("preset %q port %q proto %q", cfg.Preset, cfg.Port, cfg.Proto)
			}
		}},
		{name: "unknown preset", args: []string{"-preset", "gopher", "127.0.0.1"}, wantErr: "invalid preset"},
		{name: "two preset flags", args: []string{"-dns", "-ntp", "127.0.0.1"}, wantErr: "multiple presets"},
		{name: "preset flag and -preset conflict", args: []string{"-dns", "-preset", "ntp", "127.0.0.1"}, wantErr: "conflicting presets"},
		{name: "tcp and udp", args: []string{"-tcp", "-udp", "127.0.0.1", "80"}, wantErr: "both -udp and -tcp"},
		{name: "tcp flag with udp preset uses tcp and preset port", args: []string{"-tcp", "-dns", "127.0.0.1"}, check: func(t *testing.T, cfg *models.Config) {
			if !cfg.IsTCP() || cfg.Port != "53" || cfg.UDPPayload != nil {
				t.Errorf("proto %q port %q payload %v", cfg.Proto, cfg.Port, cfg.UDPPayload)
			}
		}},
		{name: "positional port overrides preset port", args: []string{"-dns", "127.0.0.1", "5353"}, check: func(t *testing.T, cfg *models.Config) {
			if cfg.Port != "5353" {
				t.Errorf("Port = %q", cfg.Port)
			}
		}},
		{name: "no arguments", args: nil, wantErr: "destination host is required"},
		{name: "unknown flag", args: []string{"-bogus", "127.0.0.1", "80"}, wantErr: "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseArgsWith(t, tt.args...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestParseArgs_HelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-help"}, {"--help"}} {
		if _, err := ParseArgs(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("ParseArgs(%v) error = %v, want flag.ErrHelp", args, err)
		}
	}
	for _, args := range [][]string{{"-version"}, {"--version"}, {"-version", "ignored"}} {
		cfg, err := ParseArgs(args)
		if err != nil || !cfg.ShowVersion {
			t.Errorf("ParseArgs(%v) = %+v, %v; want ShowVersion", args, cfg, err)
		}
	}
	// A host literally called "version" is a host.
	cfg, err := ParseArgs([]string{"version", "80"})
	if err != nil || cfg.ShowVersion || cfg.Host != "version" {
		t.Errorf("ParseArgs(version 80) = %+v, %v", cfg, err)
	}
}

func TestParseArgs_IsReentrant(t *testing.T) {
	if _, err := ParseArgs([]string{"-dns", "-udp", "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseArgs([]string{"127.0.0.1", "80"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Preset != "" || !cfg.IsTCP() || cfg.Port != "80" {
		t.Errorf("state leaked between parses: %+v", cfg)
	}
}

func TestResolve_Literal(t *testing.T) {
	cfg, err := ParseArgs([]string{"-4", "-6", "::1", "80"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.IPs) != 0 {
		t.Fatalf("ParseArgs must not resolve, got %v", cfg.IPs)
	}
	if err := Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.IPs) != 1 || cfg.IPs[0].IP != "::1" || cfg.IPs[0].IsIPv4 {
		t.Errorf("IPs = %+v", cfg.IPs)
	}
	bad, _ := ParseArgs([]string{"-6", "127.0.0.1", "80"})
	if err := Resolve(bad); err == nil {
		t.Error("Resolve(-6 127.0.0.1) expected a family error")
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
	for _, flagName := range []string{"-t ", "-d ", "-c ", "-preset", "-payload", "-udp", "-tcp", "-4", "-6", "-version", "-dns", "-https"} {
		if !strings.Contains(usage, flagName) {
			t.Errorf("Usage should list %q", flagName)
		}
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

	// Validate must not touch the network, so a name is fine here.
	if err := Validate(cfg); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
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
		{IP: "::1", IsIPv4: false},
		{IP: "2001:db8::1", IsIPv4: false},
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

	expected := []string{"::1", "2001:db8::1", "2001:db8::2"}

	for i, exp := range expected {
		if cfg.IPs[i].IP != exp {
			t.Errorf("Position %d: expected IP '%s', got '%s'", i, exp, cfg.IPs[i].IP)
		}
	}
}

func TestSortIPs_Numeric(t *testing.T) {
	cfg := &models.Config{IPs: []models.IP{
		{IP: "10.0.0.2", IsIPv4: true},
		{IP: "9.0.0.1", IsIPv4: true},
		{IP: "100.0.0.1", IsIPv4: true},
	}}
	SortIPs(cfg)
	want := []string{"9.0.0.1", "10.0.0.2", "100.0.0.1"}
	for i, w := range want {
		if cfg.IPs[i].IP != w {
			t.Errorf("position %d: got %s, want %s", i, cfg.IPs[i].IP, w)
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

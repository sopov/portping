package cli

import (
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/colors"
	"github.com/sopov/portping/internal/helpers"
	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

var cfgFlags struct {
	udp bool
	tcp bool
	v4  bool
	v6  bool
}

func Parse() (*models.Config, error) {
	return parseWith(flag.CommandLine, os.Args[1:])
}

func parseWith(fs *flag.FlagSet, args []string) (*models.Config, error) {
	cfg := &models.Config{}
	initFlags(fs, cfg)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	// tcp/udp
	if cfgFlags.udp && cfgFlags.tcp {
		return nil, fmt.Errorf("both -udp and -tcp are set")
	}
	if cfgFlags.udp {
		cfg.Proto = models.UDP
	} else if cfgFlags.tcp {
		cfg.Proto = models.TCP
	}
	// ipv4/ipv6
	if cfgFlags.v4 || cfgFlags.v6 {
		cfg.AllowIPv4 = cfgFlags.v4
		cfg.AllowIPv6 = cfgFlags.v6
	} else {
		cfg.AllowIPv4 = true
	}

	if err := parseArgs(fs, cfg); err != nil {
		return nil, err
	}

	if cfg.Proto != models.UDP && cfg.Proto != models.TCP {
		cfg.Proto = models.TCP
	}

	if cfg.Proto == models.UDP && cfg.UDPPayloadHex != "" {
		b, err := hex.DecodeString(cfg.UDPPayloadHex)
		if err != nil {
			return nil, errors.New("invalid UDP payload, should be hex string")
		}
		cfg.UDPPayload = b
	}

	if cfg.Proto != models.UDP {
		cfg.UDPPayloadHex = ""
		cfg.UDPPayload = nil
	}

	cfg.TimeoutDur = time.Duration(cfg.Timeout) * time.Millisecond
	cfg.DelayDur = time.Duration(cfg.Delay) * time.Millisecond
	cfg.Nonstop = cfg.Count == 0 // boolean flag for nonstop mode

	colors.NoColor(cfg.NoColor)

	return cfg, Validate(cfg)
}

func initFlags(fs *flag.FlagSet, cfg *models.Config) {
	if helpers.IsWindows() {
		cfg.NoColor = true
	} else {
		fs.BoolVar(&cfg.NoColor, "nocolor", false, "Disable color output")
	}

	fs.IntVar(&cfg.Timeout, "t", 1000, "Timeout in milliseconds")
	fs.IntVar(&cfg.Delay, "d", 1000, "Delay in milliseconds")
	fs.IntVar(&cfg.Count, "c", 0, "Stop after connecting count times")

	fs.BoolVar(&cfgFlags.v4, "4", false, "Allow IPv4 (default)")
	fs.BoolVar(&cfgFlags.v6, "6", false, "Allow IPv6")

	fs.StringVar(&cfg.Preset, "preset", "", "Preset name: "+presetsHelp())
	fs.StringVar(&cfg.UDPPayloadHex, "payload", "", "UDP Payload in hex string format")

	fs.BoolVar(&cfgFlags.udp, "udp", false, "UDP Ping")
	fs.BoolVar(&cfgFlags.tcp, "tcp", false, "TCP Ping (default)")

	names := make([]string, 0, len(probe.Predefined))
	for n := range probe.Predefined {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		pr := probe.Predefined[name]
		usage := fmt.Sprintf("Use preset `%s` (%s, port %s)", strings.ToUpper(name), pr.Proto.String(), pr.Port)
		fs.Bool(name, false, usage) // дефолт и так false, лишний SetDefValue не нужен
	}
}

func parseArgs(fs *flag.FlagSet, cfg *models.Config) error {
	args := fs.Args()
	if len(args) == 0 {
		return nil
	}
	host, port, payloadFromArg, portInHost := "", "", "", false

	var chosen string
	var count int
	names := make([]string, 0, len(probe.Predefined))
	for n := range probe.Predefined {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if f := fs.Lookup(name); f != nil && f.Value.String() == "true" {
			chosen = name
			count++
			if count > 1 {
				return fmt.Errorf("multiple presets selected")
			}
		}
	}
	if chosen != "" && cfg.Preset == "" {
		cfg.Preset = chosen
	}
	if chosen != "" && cfg.Preset != "" && cfg.Preset != chosen {
		return fmt.Errorf("conflicting presets: %q and %q", cfg.Preset, chosen)
	}

	raw0 := args[0]
	// IPv6 with brackets or host:port
	if strings.HasPrefix(raw0, "[") || strings.Contains(raw0, ":") {
		h, p, err := trySplitHostPort(raw0)
		if err == nil {
			host, port, portInHost = h, p, true
		}
	}

	if host == "" {
		host = strings.Trim(raw0, "[]")
		if len(args) > 1 {
			port = args[1]
		}
	}

	// resolve boolean shortcuts preset

	if cfg.Preset != "" {
		pr, ok := probe.GetPreset(cfg.Preset)
		if !ok {
			return fmt.Errorf("invalid preset %q", cfg.Preset)
		}
		if port == "" {
			port = pr.Port
		}

		if cfg.Proto != models.UDP && cfg.Proto != models.TCP {
			cfg.Proto = pr.Proto
		}

		if cfg.Proto == models.UDP && cfg.UDPPayloadHex == "" {
			cfg.UDPPayloadHex = pr.UDPPayloadHex
		}
	}

	// UDP payload from args (check after preset resolution so cfg.UDP is set)
	// Check for positional payload even if preset is set (it will override)
	if cfg.IsUDP() {
		idx := 2
		if portInHost {
			idx = 1
		}
		if len(args) > idx {
			payloadFromArg = args[idx]
		}
	}

	// if found positional UDP payload, override preset payload
	// Positional payload overrides preset payload
	if payloadFromArg != "" {
		cfg.UDPPayloadHex = payloadFromArg
	}

	cfg.Host = host
	cfg.Port = port

	return nil
}

// trySplitHostPort handles cases like "::1:80" -> "[::1]:80"
func trySplitHostPort(s string) (host, port string, err error) {
	// Normal forms: "host:port", "[v6]:port"
	if h, p, e := net.SplitHostPort(s); e == nil {
		return h, p, nil
	}

	// IPv6 without brackets: "::1:80"
	if strings.Count(s, ":") >= 2 && !strings.HasPrefix(s, "[") {
		i := strings.LastIndex(s, ":")
		if i > 0 && i < len(s)-1 {
			h := s[:i]
			p := s[i+1:]
			// Let net.SplitHostPort validate port strictly
			return net.SplitHostPort("[" + h + "]:" + p)
		}
	}

	return "", "", fmt.Errorf("invalid address `%s`", s)
}

func Validate(cfg *models.Config) error {
	if len(cfg.Host) < 1 || len(cfg.Port) < 1 {
		return fmt.Errorf("host or port is required")
	}
	if !helpers.ValidPort(cfg.Port) {
		return fmt.Errorf("invalid port `%s`", cfg.Port)
	}
	if cfg.Timeout < 1 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	if cfg.Delay < 1 {
		return fmt.Errorf("delay must be greater than 0")
	}
	if cfg.Count < 0 {
		return fmt.Errorf("count must be greater than or equal to 0")
	}
	if cfg.IsUDP() && cfg.UDPPayloadHex == "" && len(cfg.UDPPayload) == 0 {
		return fmt.Errorf("UDP payload is required for UDP ping")
	}
	ips, err := probe.GetAddrs(cfg)
	if err != nil {
		return err
	}
	cfg.IPs = ips
	SortIPs(cfg)
	return nil
}

func SortIPs(cfg *models.Config) {
	sort.Slice(cfg.IPs, func(i, j int) bool {
		ipA, ipB := cfg.IPs[i], cfg.IPs[j]

		// IPv4 always comes before IPv6
		if ipA.IsIPv4 != ipB.IsIPv4 {
			return ipA.IsIPv4
		}

		// Within group, sort by string value
		return ipA.IP < ipB.IP
	})
}

func Usage() string {
	cmd := app.Name
	if helpers.IsWindows() {
		cmd += ".exe"
	}

	var args bytes.Buffer
	flag.CommandLine.SetOutput(&args)
	flag.PrintDefaults()

	usage := strings.Join(
		[]string{
			"%s [options] <destination> <port> [UDP HEX PAYLOAD (UDP only)]",
			"",
			"Options:",
			"%s",    // Usage Args
			"--",    // Copyrights
			"%s %s", // App + Ver
			"Copyright (c) 2017, 2025, Leonid Sopov <leonid@sopov.org>",
			"https://github.com/sopov/portping/",
		}, "\n",
	)

	return fmt.Sprintf(usage, cmd, args.String(), cmd, app.Version)
}

func presetsHelp() string {
	var payloads []string
	for name := range probe.Predefined {
		payloads = append(payloads, name)
	}
	sort.Strings(payloads)
	return strings.Join(payloads, ", ")
}

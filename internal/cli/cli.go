package cli

import (
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/colors"
	"github.com/sopov/portping/internal/helpers"
	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
)

var cfgFlags struct {
	udp bool
	tcp bool
	v4  bool
	v6  bool
}

// Parse parses os.Args[1:]. See ParseArgs.
func Parse() (*models.Config, error) {
	return ParseArgs(os.Args[1:])
}

// ParseArgs parses command-line arguments into a validated Config. It never
// prints and never exits: -h/-help yields flag.ErrHelp, every other problem
// a descriptive error. Name resolution is a separate step, see Resolve.
func ParseArgs(args []string) (*models.Config, error) {
	fs := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return parseWith(fs, args)
}

// maxMillis bounds -t and -d: 24 hours, far below the point where the
// conversion to time.Duration would overflow.
const maxMillis = 24 * 60 * 60 * 1000

func parseWith(fs *flag.FlagSet, args []string) (*models.Config, error) {
	cfg := &models.Config{}
	initFlags(fs, cfg)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cfg.ShowVersion {
		return cfg, nil
	}
	payloadFlagSet := cfg.UDPPayloadHex != ""
	cfg.Preset = strings.ToLower(cfg.Preset)
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
			return nil, fmt.Errorf("invalid UDP payload, should be hex string: %w", err)
		}
		if len(b) > probe.MaxUDPPayload {
			return nil, fmt.Errorf("UDP payload is %d bytes, maximum is %d", len(b), probe.MaxUDPPayload)
		}
		cfg.UDPPayload = b
	}

	if cfg.Proto != models.UDP {
		if payloadFlagSet {
			return nil, errors.New("-payload requires UDP (-udp or a UDP preset)")
		}
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

	fs.BoolVar(&cfg.ShowVersion, "version", false, "Print version and exit")
	fs.IntVar(&cfg.Timeout, "t", 1000, "Timeout in milliseconds")
	fs.IntVar(&cfg.Delay, "d", 1000, "Delay in milliseconds")
	fs.IntVar(&cfg.Count, "c", 0, "Stop after count attempts (0 = run until interrupted)")

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
		fs.Bool(name, false, usage)
	}
}

func parseArgs(fs *flag.FlagSet, cfg *models.Config) error {
	args := fs.Args()
	if len(args) == 0 {
		return nil
	}
	var payloadFromArg string

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

	host, port, err := splitDestination(args[0])
	if err != nil {
		return err
	}
	rest := args[1:]
	portInHost := port != ""
	if !portInHost && len(rest) > 0 {
		port, rest = rest[0], rest[1:]
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

	// A positional UDP payload follows host and port (or host:port) and
	// overrides a preset payload.
	if cfg.IsUDP() && len(rest) > 0 {
		payloadFromArg, rest = rest[0], rest[1:]
	}
	if payloadFromArg != "" {
		cfg.UDPPayloadHex = payloadFromArg
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected argument(s): %s", strings.Join(rest, " "))
	}

	cfg.Host = host
	cfg.Port = port

	return nil
}

// splitDestination interprets the destination argument. Accepted forms:
//
//	host            hostname or IP literal (IPv6 with or without a zone)
//	[v6]            bracketed IPv6 literal
//	host:port       hostname or IPv4 literal with port
//	[v6]:port       bracketed IPv6 literal with port
//
// An IP literal is never split: "::1:80" is the address ::1:80, not ::1 on
// port 80. IPv6 with a port therefore needs the bracket form.
func splitDestination(raw string) (host, port string, err error) {
	if raw == "" {
		return "", "", errors.New("destination is required")
	}
	if strings.HasPrefix(raw, "[") {
		if strings.HasSuffix(raw, "]") {
			host = raw[1 : len(raw)-1]
			if _, ok := probe.ParseLiteral(host); !ok {
				return "", "", fmt.Errorf("invalid IPv6 literal `%s`", raw)
			}
			return host, "", nil
		}
		h, p, e := net.SplitHostPort(raw)
		if e != nil || p == "" {
			return "", "", fmt.Errorf("invalid address `%s`", raw)
		}
		if _, ok := probe.ParseLiteral(h); !ok {
			return "", "", fmt.Errorf("invalid IPv6 literal `%s`", raw)
		}
		return h, p, nil
	}
	if _, ok := probe.ParseLiteral(raw); ok {
		return raw, "", nil
	}
	if strings.Contains(raw, ":") {
		h, p, e := net.SplitHostPort(raw)
		if e != nil || h == "" || p == "" {
			if strings.Count(raw, ":") > 1 {
				return "", "", fmt.Errorf("invalid address `%s`: write IPv6 with a port as [address]:port", raw)
			}
			return "", "", fmt.Errorf("invalid address `%s`", raw)
		}
		return h, p, nil
	}
	return raw, "", nil
}

func Validate(cfg *models.Config) error {
	if cfg.Host == "" {
		return errors.New("destination host is required")
	}
	if cfg.Port == "" {
		return errors.New("port is required (second argument, host:port, or a preset)")
	}
	port, err := helpers.NormalizePort(cfg.Port)
	if err != nil {
		return err
	}
	cfg.Port = port
	if cfg.Timeout < 1 || cfg.Timeout > maxMillis {
		return fmt.Errorf("timeout must be between 1 and %d ms", maxMillis)
	}
	if cfg.Delay < 1 || cfg.Delay > maxMillis {
		return fmt.Errorf("delay must be between 1 and %d ms", maxMillis)
	}
	if cfg.Count < 0 {
		return errors.New("count must be greater than or equal to 0")
	}
	if cfg.IsUDP() && cfg.UDPPayloadHex == "" && len(cfg.UDPPayload) == 0 {
		return errors.New("UDP payload is required for UDP ping")
	}
	return nil
}

// Resolve fills cfg.IPs from cfg.Host. It is the only step of configuration
// that touches the network, so callers can report its failures as runtime
// errors rather than usage errors.
func Resolve(cfg *models.Config) error {
	ips, err := probe.GetAddrs(cfg)
	if err != nil {
		return err
	}
	cfg.IPs = ips
	SortIPs(cfg)
	return nil
}

func SortIPs(cfg *models.Config) {
	models.SortIPs(cfg.IPs)
}

func Usage() string {
	cmd := app.Name
	if helpers.IsWindows() {
		cmd += ".exe"
	}

	var args bytes.Buffer
	fs := flag.NewFlagSet(app.Name, flag.ContinueOnError)
	initFlags(fs, &models.Config{})
	fs.SetOutput(&args)
	fs.PrintDefaults()

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
	payloads := make([]string, 0, len(probe.Predefined))
	for name := range probe.Predefined {
		payloads = append(payloads, name)
	}
	sort.Strings(payloads)
	return strings.Join(payloads, ", ")
}

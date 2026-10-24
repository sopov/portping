package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/sopov/portping/internal/models"
)

// MaxUDPPayload is the largest payload that fits in a single IPv4 UDP datagram
// (65535 bytes minus the IPv4 and UDP headers).
const MaxUDPPayload = 65507

// maxDatagram is the size of the receive buffer. Reading into anything smaller
// is not portable: Windows returns WSAEMSGSIZE as an error when a datagram is
// truncated, while Linux and the BSDs truncate silently.
const maxDatagram = 65535

// ErrNoPayload is returned by PingUDP when no payload was supplied.
var ErrNoPayload = errors.New("udp payload required")

var udpBufPool = sync.Pool{New: func() any {
	b := make([]byte, maxDatagram)
	return &b
}}

// Resolver is the part of net.Resolver used by Resolve. Tests supply a fake.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// minResolveTimeout is the floor for the DNS lookup budget. The per-probe
// timeout is often set far below what an uncached recursive lookup needs.
const minResolveTimeout = 5 * time.Second

// ParseLiteral reports whether host is an IP literal and returns it in
// canonical form: zone identifiers are kept, IPv4-mapped IPv6 addresses are
// unmapped to IPv4.
func ParseLiteral(host string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func toIP(addr netip.Addr) models.IP {
	addr = addr.Unmap()
	return models.IP{IP: addr.String(), IsIPv4: addr.Is4()}
}

func allowed(cfg *models.Config, addr netip.Addr) bool {
	if addr.Unmap().Is4() {
		return cfg.AllowIPv4
	}
	return cfg.AllowIPv6
}

// GetAddrs resolves cfg.Host into the addresses to probe, filtered by the
// address-family flags and sorted IPv4 first.
func GetAddrs(cfg *models.Config) ([]models.IP, error) {
	return Resolve(context.Background(), net.DefaultResolver, cfg)
}

// Resolve is GetAddrs with an explicit context and resolver.
func Resolve(ctx context.Context, r Resolver, cfg *models.Config) ([]models.IP, error) {
	if addr, ok := ParseLiteral(cfg.Host); ok {
		if !allowed(cfg, addr) {
			return nil, familyError(cfg, addr)
		}
		return []models.IP{toIP(addr)}, nil
	}

	timeout := cfg.TimeoutDur
	if timeout < minResolveTimeout {
		timeout = minResolveTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addrs, err := r.LookupNetIP(ctx, "ip", cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("resolve host `%s`: %w", cfg.Host, err)
	}

	seen := make(map[netip.Addr]struct{}, len(addrs))
	ips := make([]models.IP, 0, len(addrs))
	var skipped4, skipped6 int
	for _, addr := range addrs {
		addr = addr.Unmap()
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		if !allowed(cfg, addr) {
			if addr.Is4() {
				skipped4++
			} else {
				skipped6++
			}
			continue
		}
		ips = append(ips, toIP(addr))
	}
	if len(ips) == 0 {
		switch {
		case skipped6 > 0 && !cfg.AllowIPv6:
			return nil, fmt.Errorf("host `%s` has only IPv6 addresses; add -6 to probe them", cfg.Host)
		case skipped4 > 0 && !cfg.AllowIPv4:
			return nil, fmt.Errorf("host `%s` has only IPv4 addresses; add -4 to probe them", cfg.Host)
		default:
			return nil, fmt.Errorf("resolve host `%s`: no addresses found", cfg.Host)
		}
	}
	models.SortIPs(ips)
	return ips, nil
}

func familyError(cfg *models.Config, addr netip.Addr) error {
	if addr.Is4() {
		if cfg.AllowIPv6 {
			return fmt.Errorf("%s is an IPv4 address but only IPv6 is allowed (-6); add -4", addr)
		}
		return fmt.Errorf("%s is an IPv4 address but IPv4 is not allowed", addr)
	}
	if cfg.AllowIPv4 {
		return fmt.Errorf("%s is an IPv6 address but only IPv4 is allowed; add -6", addr)
	}
	return fmt.Errorf("%s is an IPv6 address but IPv6 is not allowed", addr)
}

// PingTCP opens a TCP connection to opts.Address and closes it again. The
// returned duration is the connect time, also when the connect failed.
func PingTCP(opts models.PingOptions) (time.Duration, error) {
	ctx, cancel := probeContext(opts)
	defer cancel()

	start := time.Now()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, models.TCP.String(), opts.Address)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, probeError(ctx, err)
	}
	_ = conn.Close()
	return elapsed, nil
}

// PingUDP sends opts.Payload to opts.Address and waits for any datagram in
// reply. A reply of any size counts as success. An ICMP port unreachable
// surfaces as a "connection refused" read error, a silent peer as a timeout.
// The whole probe (write and read) shares one deadline, and cancelling the
// context wakes a blocked read immediately.
func PingUDP(opts models.PingOptions) (time.Duration, error) {
	if len(opts.Payload) == 0 {
		return 0, ErrNoPayload
	}

	ctx, cancel := probeContext(opts)
	defer cancel()

	start := time.Now()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, models.UDP.String(), opts.Address)
	if err != nil {
		return time.Since(start), probeError(ctx, err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return time.Since(start), err
		}
	}
	// Registered after the deadline is set so that an already-cancelled
	// context cannot be overwritten by SetDeadline above.
	stopWatch := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stopWatch()

	if _, err := conn.Write(opts.Payload); err != nil {
		return time.Since(start), probeError(ctx, err)
	}

	bufp, _ := udpBufPool.Get().(*[]byte)
	defer udpBufPool.Put(bufp)
	_, err = conn.Read(*bufp)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, probeError(ctx, err)
	}
	return elapsed, nil
}

// probeContext returns a context that carries a deadline: the caller's if it
// has one, otherwise Config.TimeoutDur from now.
func probeContext(opts models.PingOptions) (context.Context, context.CancelFunc) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || opts.Config == nil || opts.Config.TimeoutDur <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, opts.Config.TimeoutDur)
}

// probeError reports context.Canceled when the probe was interrupted, so the
// caller can tell an interrupted probe from a failed one. Timeouts keep the
// network error so the output still says what happened on the wire.
func probeError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	return err
}

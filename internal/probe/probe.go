package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
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

func getIPv4(cfg *models.Config, addr net.IP) (string, bool) {
	if !cfg.AllowIPv4 {
		return "", false
	}
	if ipv4 := addr.To4(); ipv4 != nil {
		return ipv4.String(), true
	}
	return "", false
}

func getIPv6(cfg *models.Config, addr net.IP) (string, bool) {
	if !cfg.AllowIPv6 {
		return "", false
	}
	if addr.To4() != nil {
		return "", false
	}
	if ipv6 := addr.To16(); ipv6 != nil {
		return ipv6.String(), true
	}
	return "", false
}

func getIP(cfg *models.Config, addr net.IP) (models.IP, bool) {
	if ip, found := getIPv4(cfg, addr); found {
		return models.IP{
			IP:     ip,
			IsIPv4: true,
		}, true
	}
	if ip, found := getIPv6(cfg, addr); found {
		return models.IP{
			IP:     ip,
			IsIPv4: false,
		}, true
	}
	return models.IP{}, false
}

func GetAddrs(cfg *models.Config) ([]models.IP, error) {
	if ip := net.ParseIP(cfg.Host); ip != nil {
		if got, ok := getIP(cfg, ip); ok {
			return []models.IP{got}, nil
		}
		return nil, errors.New("no addresses found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TimeoutDur)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("resolve host `%s`: %w", cfg.Host, err)
	}

	ips := make([]models.IP, 0, len(addrs))
	for _, addr := range addrs {
		if ip, found := getIP(cfg, addr); found {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return ips, errors.New("no addresses found")
	}

	return ips, nil
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

package probe

import (
	"context"
	"errors"
	"fmt"
	"github.com/sopov/portping/internal/models"
	"net"
	"time"
)

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

func PingTCP(opts models.PingOptions) (time.Duration, error) {
	start := time.Now()
	d := net.Dialer{}
	conn, err := d.DialContext(opts.Context, models.TCP.String(), opts.Address)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, err
	}
	_ = conn.Close()

	return elapsed, nil

}

func PingUDP(opts models.PingOptions) (time.Duration, error) {
	if len(opts.Payload) == 0 {
		return 0, errors.New("udp payload required")
	}

	start := time.Now()
	d := net.Dialer{}
	conn, err := d.DialContext(opts.Context, models.UDP.String(), opts.Address)
	if err != nil {
		return time.Since(start), err
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(opts.Config.TimeoutDur)); err != nil {
		return time.Since(start), err
	}
	if _, err = conn.Write(opts.Payload); err != nil {
		return time.Since(start), err
	}
	if err := conn.SetReadDeadline(time.Now().Add(opts.Config.TimeoutDur)); err != nil {
		return time.Since(start), err
	}
	var tmp [1]byte
	_, err = conn.Read(tmp[:])

	return time.Since(start), err
}

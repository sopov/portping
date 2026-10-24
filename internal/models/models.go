package models

import (
	"context"
	"net/netip"
	"sort"
	"time"
)

type Proto string

const TCP Proto = "tcp"
const UDP Proto = "udp"

func (proto Proto) String() string {
	return string(proto)
}

type IP struct {
	IP     string
	IsIPv4 bool
}

func (ip IP) IsIPv6() bool {
	return !ip.IsIPv4
}

// Less orders IPv4 before IPv6 and numerically within a family, so that
// 9.0.0.1 sorts before 10.0.0.2 and ::1 before 2001:db8::1. Addresses that
// do not parse fall back to string order.
func (ip IP) Less(other IP) bool {
	if ip.IsIPv4 != other.IsIPv4 {
		return ip.IsIPv4
	}
	a, errA := netip.ParseAddr(ip.IP)
	b, errB := netip.ParseAddr(other.IP)
	if errA == nil && errB == nil {
		return a.Compare(b) < 0
	}
	return ip.IP < other.IP
}

// SortIPs sorts addresses in place using IP.Less.
func SortIPs(ips []IP) {
	sort.Slice(ips, func(i, j int) bool { return ips[i].Less(ips[j]) })
}

type Config struct {
	Proto         Proto
	Host          string
	Port          string
	Timeout       int
	TimeoutDur    time.Duration
	Delay         int
	DelayDur      time.Duration
	Count         int
	Nonstop       bool
	AllowIPv4     bool
	AllowIPv6     bool
	NoColor       bool
	ShowVersion   bool
	IPs           []IP
	Preset        string
	UDPPayloadHex string
	UDPPayload    []byte
}

func (c *Config) IsUDP() bool { return c.Proto == UDP }
func (c *Config) IsTCP() bool { return c.Proto == TCP }

// Stats holds the counters for one address. The counters are int64 so that
// long-running continuous mode cannot overflow on 32-bit builds.
type Stats struct {
	IP       IP
	Attempts int64
	Connects int64
	Failures int64
	Minimum  time.Duration
	Maximum  time.Duration
	Total    time.Duration
}

type PingOptions struct {
	Context context.Context
	Config  *Config
	Address string
	Payload []byte
}

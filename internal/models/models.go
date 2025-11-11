package models

import (
	"context"
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
	IPs           []IP
	Preset        string
	UDPPayloadHex string
	UDPPayload    []byte
}

func (c *Config) IsUDP() bool { return c.Proto == UDP }
func (c *Config) IsTCP() bool { return c.Proto == TCP }

type Stats struct {
	IP       IP
	Attempts int
	Connects int
	Failures int
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

package main

import (
	"errors"
	"net"
	"os"
	"time"
)

func ping(addr stats) (stats, float64, error) {
	var ip = addr.IP
	if addr.IsIPv6 {
		ip = "[" + addr.IP + "]"
	}
	addr.Attempts++
	var ips = ip + ":" + port
	if !IsValidPort(port) {

	}
	start := time.Now().UnixNano()
	conn, err := net.DialTimeout(proto, ips, time.Duration(timeout)*time.Millisecond)
	delta := float64(time.Now().UnixNano()-start) / float64(time.Millisecond)
	var er error
	if err != nil {
		addr.Failures++
		if opError, ok := err.(*net.OpError); ok {
			er = opError.Err
			if opError.Timeout() {
				er = errors.New("Connection Timeout")
			} else if sysError, ok := opError.Err.(*os.SyscallError); ok {
				er = sysError.Err
			}
		}
	} else {
		addr.Connects++
		addr.Total += delta
		if delta > addr.Maximum {
			addr.Maximum = delta
		}
		if addr.Minimum > delta || addr.Minimum == 0 {
			addr.Minimum = delta
		}
		defer conn.Close()
	}
	return addr, delta, er
}

func GetAddrs(host string) ([]string, []string, error) {
	addrs, err := net.LookupIP(host)
	if err != nil {
		return make([]string, 0), make([]string, 0), err
	}
	addrs4, addrs6 := make([]string, 0, len(addrs)/2), make([]string, 0, len(addrs)/2)
	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			if allowIPv4 {
				addrs4 = append(addrs4, ipv4.String())
			}
		} else if allowIPv6 {
			if ipv6 := addr.To16(); ipv6 != nil {
				addrs6 = append(addrs6, ipv6.String())
			}
		}
	}
	if len(addrs4) == 0 && len(addrs6) == 0 {
		return addrs4, addrs6, errors.New("No Active IPs, try run with -6")
	}
	return addrs4, addrs6, nil
}

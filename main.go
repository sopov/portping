package main

import (
	"flag"
	"fmt"
	"github.com/fatih/color"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const ver = "1.0"

type stats struct {
	IP     string
	IsIPv4 bool
	IsIPv6 bool
	// default 0
	Attempts uint
	Connects uint
	Failures uint
	Minimum  float64
	Maximum  float64
	Total    float64
}

var (
	timeout   uint = 1000
	ips            = map[string]stats{}
	proto          = "tcp"
	nocolor        = false
	host      string
	port      string
	allowIPv4 = true
	allowIPv6 = true
	// untill ctrl+c
	Continous = true
	Count     uint
	ipMaxLen  int
)

func main() {
	flag.BoolVar(&nocolor, "nocolor", false, "Disable color output")
	flag.UintVar(&timeout, "t", 1000, "Timeout in milliseconds")
	flag.UintVar(&Count, "c", 0, "Stop after connecting count times")
	flag.BoolVar(&allowIPv6, "6", false, "Allow IPv6")
	flag.Parse()

	if Count > 0 {
		Continous = false
	}
	color.NoColor = nocolor

	host = flag.Arg(0)
	port = flag.Arg(1)

	Usage := "Syntax: portping [options] <destination> <port>\n\n" +
		"Options:\n" +
		"  -6   Allow IPv6\n" +
		"  -c uint\n" +
		"       Stop after connecting count times\n" +
		"  -nocolor\n" +
		"       Disable color output\n" +
		"  -t uint\n" +
		"       Timeout in milliseconds (default 1000)\n" +
		"\n\nportping " + ver +

		"\nCopyright (c) 2017, Leonid Sopov <leonid@sopov.org>" +
		"\nhttps://github.com/sopov/portping/\n"

	if len(host) < 1 || len(port) < 1 {
		fmt.Println(Usage)
		os.Exit(2)
	}
	if !IsValidPort(port) {
		fmt.Println("Wrong port number", port)
		println()
		fmt.Println(Usage)
		os.Exit(2)
	}
	addrs4, addrs6, err := GetAddrs(host)

	if err != nil {
		fmt.Println("Error:\n\t", err)
		println()
		fmt.Println(Usage)
		os.Exit(2)
	}

	total := len(addrs4) + len(addrs6)
	addrs := make([]string, 0, total)
	for _, ip := range addrs4 {
		if _, exists := ips[ip]; !exists {
			addrs = append(addrs, ip)
			ips[ip] = stats{IP: ip, IsIPv4: true}
			if ipMaxLen == 0 || ipMaxLen < len(ip) {
				ipMaxLen = len(ip)
			}
		}
	}

	for _, ip := range addrs6 {
		if _, exists := ips[ip]; !exists {
			addrs = append(addrs, ip)
			ips[ip] = stats{IP: ip, IsIPv6: true}
			if ipMaxLen == 0 || ipMaxLen < len(ip) {
				ipMaxLen = len(ip)
			}
		}
	}

	ipsufix := "IP"
	if total > 1 {
		ipsufix += "s"
	}
	fmt.Printf("Ping of %s on %s %s (%d %s)\n", hyellow(host), hyellow(proto), hyellow(port), total, ipsufix)
	if total > 1 {
		for _, ip := range addrs4 {
			fmt.Printf("IPv4: %s\n", hyellow(ip))
		}
		for _, ip := range addrs6 {
			fmt.Printf("IPv6: %s\n", hyellow(ip))
		}
		println()
	}
	// READY FOR PING
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		println()
		println()
		ShowStats(addrs4, addrs6)
		os.Exit(1)
	}()
	var cnt uint
	var okfm string = "% 3s\t% " + strconv.Itoa(ipMaxLen) + "s\t%19s"
	if nocolor {
		okfm = "% 3s\t% " + strconv.Itoa(ipMaxLen) + "s\t%10s"
	}
	var erfm string = okfm + "\tErr: %s\n"
	okfm += "\n"

	for Continous || cnt < Count {
		sc := 0
		cnt++
		for _, ip := range addrs {
			i, t, err := ping(ips[ip])
			ips[ip] = i
			att := strconv.Itoa(int(i.Attempts))
			if total > 1 {
				sc++
				att += "." + strconv.Itoa(sc)
			}
			if err != nil {
				fmt.Printf(erfm, att, ip, hred(fmt.Sprintf("%0.2fms", t)), red(err.Error()))
			} else {
				fmt.Printf(okfm, att, ip, hgreen(fmt.Sprintf("%0.2fms", t)))
			}
			if t < 1000 {
				time.Sleep(time.Millisecond * time.Duration(1000-t))
			}
		}
	}
	println()
	ShowStats(addrs4, addrs6)
}

package main

import (
	"fmt"
	"strconv"
)

func ShowStats(addrs4 []string, addrs6 []string) {
	fm := "% " + strconv.Itoa(int(ipMaxLen)+10) + "s% 12s% 21s % 24s% 10s %10s  %10s\n"
	if nocolor {
		fm = "% " + strconv.Itoa(int(ipMaxLen)+1) + "s% 12s% 11s % 15s% 10s %10s  %10s\n"
	}
	fmt.Printf("Statistics of ping %s on %s %s\n", hyellow(host), hyellow(proto), hyellow(port))
	fmt.Printf(fm, yellow("IP Address"), "Attempted", green("Connected"), red("Failed"), "Minimum", "Maximum", "Average")
	for _, ip := range append(addrs4, addrs6...) {
		if ips[ip].Attempts > 0 {
			var avg float64 = 0
			if ips[ip].Connects > 0 {
				avg = ips[ip].Total / float64(ips[ip].Connects)
			}
			fmt.Printf(fm,
				hyellow(ip),
				strconv.Itoa(int(ips[ip].Attempts)),
				hgreen(strconv.Itoa(int(ips[ip].Connects))),
				fmt.Sprintf("%s % 6s",
					hred(strconv.Itoa(int(ips[ip].Failures))),
					fmt.Sprintf("(%.2f%%)", float64(100*ips[ip].Failures/ips[ip].Attempts))),
				fmt.Sprintf("%.2fms", float64(ips[ip].Minimum)),
				fmt.Sprintf("%.2fms", float64(ips[ip].Maximum)),
				fmt.Sprintf("%.2fms", float64(avg)))
		}
	}
}

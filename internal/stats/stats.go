// Package stats prints the banner, per-probe lines and the final table, and
// maintains the per-address counters.
package stats

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/sopov/portping/internal/colors"
	"github.com/sopov/portping/internal/helpers"
	"github.com/sopov/portping/internal/models"
)

// ShowBanner prints the header: target, protocol, port, resolved addresses
// and, for UDP, the payload.
func ShowBanner(w io.Writer, cfg *models.Config) {
	ipSuffix := "IP"
	if len(cfg.IPs) > 1 {
		ipSuffix += "s"
	}
	fmt.Fprintf(w, "Ping of %s on %s %s (%d %s)\n",
		colors.HYellow(cfg.Host),
		colors.HYellow(cfg.Proto),
		colors.HYellow(cfg.Port),
		len(cfg.IPs),
		ipSuffix,
	)

	for _, ip := range cfg.IPs {
		t := "IPv4"
		if ip.IsIPv6() {
			t = "IPv6"
		}
		fmt.Fprintf(w, "%s: %s\n", t, colors.HYellow(ip.IP))
	}

	if cfg.IsUDP() {
		fmt.Fprintf(w, "Payload (hex): %s\n", colors.HYellow(cfg.UDPPayloadHex))
	}
}

// ShowStats prints the final table. Addresses without attempts are omitted.
func ShowStats(w io.Writer, cfg *models.Config, statsMap map[string]*models.Stats) {
	if len(statsMap) == 0 {
		return
	}

	var maxLen int
	for _, ip := range cfg.IPs {
		if l := len(ip.IP); l > maxLen {
			maxLen = l
		}
	}

	// Column widths include the ANSI sequences when colour is on.
	format := "% " + strconv.Itoa(maxLen+1) + "s% 12s% 11s % 15s% 10s %10s  %10s\n"
	if colors.Enabled() {
		format = "% " + strconv.Itoa(maxLen+10) + "s% 12s% 21s % 24s% 10s %10s  %10s\n"
	}
	fmt.Fprintf(w,
		"\nStatistics of ping %s on %s %s\n",
		colors.HYellow(cfg.Host),
		colors.HYellow(cfg.Proto),
		colors.HYellow(cfg.Port))

	fmt.Fprintf(w, format,
		colors.Yellow("IP Address"),
		"Attempted",
		colors.Green("Connected"),
		colors.Red("Failed"),
		"Minimum",
		"Maximum",
		"Average",
	)

	for _, ip := range cfg.IPs {
		st := statsMap[ip.IP]
		if st == nil || st.Attempts == 0 {
			continue
		}
		fmt.Fprintf(w,
			format,
			colors.HYellow(ip.IP),
			strconv.FormatInt(st.Attempts, 10),
			colors.HGreen(strconv.FormatInt(st.Connects, 10)),
			fmt.Sprintf("%s % 6s",
				colors.HRed(strconv.FormatInt(st.Failures, 10)),
				fmt.Sprintf("(%.2f%%)", LossPercent(st))),
			helpers.DurStr(st.Minimum),
			helpers.DurStr(st.Maximum),
			helpers.DurStr(Average(st)),
		)
	}
}

// rowFormat builds the per-probe line format for the given address width.
func rowFormat(maxIPLen int, ok bool) string {
	postMsgLen := "10"
	if colors.Enabled() {
		postMsgLen = "19"
	}
	f := "% 3s\t%" + strconv.Itoa(maxIPLen) + "s\t%" + postMsgLen + "s"
	if ok {
		return f + "%s\n"
	}
	return f + "\tErr: %s\n"
}

// ShowCurrent prints one probe result. subcnt > 0 renders "cnt.subcnt" for
// multi-address targets.
func ShowCurrent(w io.Writer, cnt, subcnt int64, ip string, maxIPLen int, duration time.Duration, err error) {
	errMsg := ""
	durStr := helpers.DurStr(duration)
	if err != nil {
		errMsg = colors.Red(err.Error())
		durStr = colors.HRed(durStr)
	} else {
		durStr = colors.HGreen(durStr)
	}
	attempt := strconv.FormatInt(cnt, 10)
	if subcnt > 0 {
		attempt += "." + strconv.FormatInt(subcnt, 10)
	}
	fmt.Fprintf(w,
		rowFormat(maxIPLen, err == nil),
		attempt,
		ip,
		durStr,
		errMsg,
	)
}

// Update records one probe result.
func Update(stats *models.Stats, duration time.Duration, err error) {
	stats.Attempts++
	if err != nil {
		stats.Failures++
		return
	}

	stats.Connects++
	stats.Total += duration

	if stats.Connects == 1 || duration < stats.Minimum {
		stats.Minimum = duration
	}
	if duration > stats.Maximum {
		stats.Maximum = duration
	}
}

// Average is the mean round-trip time of successful probes, 0 without any.
func Average(st *models.Stats) time.Duration {
	if st.Connects == 0 {
		return 0
	}
	return st.Total / time.Duration(st.Connects)
}

// LossPercent is the share of failed attempts in percent, 0 without attempts.
func LossPercent(st *models.Stats) float64 {
	if st.Attempts == 0 {
		return 0
	}
	return 100 * float64(st.Failures) / float64(st.Attempts)
}

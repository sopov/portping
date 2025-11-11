package stats

import (
	"fmt"
	"github.com/sopov/portping/internal/colors"
	"github.com/sopov/portping/internal/helpers"
	"github.com/sopov/portping/internal/models"
	"sort"
	"strconv"
	"time"
)

var (
	okFmt, errFmt string
)

func ShowBanner(cfg *models.Config) {
	ipSuffix := "IP"
	if len(cfg.IPs) > 1 {
		ipSuffix += "s"
	}
	fmt.Printf("Ping of %s on %s %s (%d %s)\n",
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

		fmt.Printf("%s: %s\n", t, colors.HYellow(ip.IP))
	}

	if cfg.IsUDP() {
		fmt.Printf("Payload (hex): %s\n", colors.HYellow(cfg.UDPPayloadHex))
	}

}

func ShowStats(cfg *models.Config, statsMap map[string]*models.Stats) {
	if len(statsMap) == 0 {
		return
	}

	keys := make([]string, 0, len(statsMap))
	for ip := range statsMap {
		keys = append(keys, ip)
	}
	sort.Strings(keys)

	var maxLen int
	for _, ip := range keys {
		if l := len(ip); l > maxLen {
			maxLen = l
		}
	}

	format := "% " + strconv.Itoa(maxLen+10) + "s% 12s% 21s % 24s% 10s %10s  %10s\n"
	if cfg.NoColor {
		format = "% " + strconv.Itoa(maxLen+1) + "s% 12s% 11s % 15s% 10s %10s  %10s\n"
	}
	fmt.Printf(
		"\nStatistics of ping %s on %s %s\n",
		colors.HYellow(cfg.Host),
		colors.HYellow(cfg.Proto),
		colors.HYellow(cfg.Port))

	fmt.Printf(format,
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
		var avg time.Duration
		if st.Connects > 0 {
			avg = time.Duration(float64(st.Total) / float64(st.Connects))
		}

		fmt.Printf(
			format,
			colors.HYellow(ip.IP),                    // IP
			strconv.Itoa(st.Attempts),                // Attempts
			colors.HGreen(strconv.Itoa(st.Connects)), // Connected
			fmt.Sprintf("%s % 6s", // Failed
				colors.HRed(strconv.Itoa(st.Failures)),
				fmt.Sprintf("(%.2f%%)", 100*float64(st.Failures)/float64(st.Attempts))),
			helpers.DurStr(st.Minimum),
			helpers.DurStr(st.Maximum),
			helpers.DurStr(avg),
		)
	}
}

func showCurrentFmt(cfg *models.Config, maxIPLen int, ok bool) string {
	if okFmt == "" {
		postMsgLen := "19"
		if cfg.NoColor {
			postMsgLen = "10"
		}
		okFmt = "% 3s\t%" + strconv.Itoa(maxIPLen) + "s\t%" + postMsgLen + "s"
		errFmt = okFmt + "\tErr: %s\n"
		okFmt += "%s\n"
	}

	if ok {
		return okFmt
	}
	return errFmt
}

func ShowCurrent(cfg *models.Config, cnt, subcnt int, ip string, maxIPLen int, duration time.Duration, err error) {
	errMsg := ""
	durStr := helpers.DurStr(duration)
	if err != nil {
		errMsg = colors.Red(err.Error())
		durStr = colors.HRed(durStr)
	} else {
		durStr = colors.HGreen(durStr)
	}
	attempt := strconv.Itoa(cnt)
	if subcnt > 0 {
		attempt += "." + strconv.Itoa(subcnt)
	}
	fmt.Printf(
		showCurrentFmt(cfg, maxIPLen, err == nil),
		attempt,
		ip,
		durStr,
		errMsg,
	)
}

func Update(stats *models.Stats, duration time.Duration, err error) {
	stats.Attempts++
	if err != nil {
		stats.Failures++
		return
	}

	stats.Connects++
	stats.Total += duration

	if float64(stats.Minimum) == 0 || stats.Minimum > duration {
		stats.Minimum = duration
	}
	if stats.Maximum < duration {
		stats.Maximum = duration
	}
}

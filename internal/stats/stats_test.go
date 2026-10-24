package stats

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/sopov/portping/internal/models"
)

func noColor(t *testing.T) {
	t.Helper()
	orig := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = orig })
}

func TestUpdate_MinMaxAvgTotal(t *testing.T) {
	s := &models.Stats{}
	for _, d := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 5 * time.Millisecond} {
		Update(s, d, nil)
	}
	if s.Attempts != 3 || s.Connects != 3 || s.Failures != 0 {
		t.Errorf("counters = %d/%d/%d", s.Attempts, s.Connects, s.Failures)
	}
	if s.Minimum != 5*time.Millisecond || s.Maximum != 20*time.Millisecond || s.Total != 35*time.Millisecond {
		t.Errorf("min/max/total = %v/%v/%v", s.Minimum, s.Maximum, s.Total)
	}
	if got := Average(s); got != 35*time.Millisecond/3 {
		t.Errorf("Average = %v, want %v", got, 35*time.Millisecond/3)
	}
	if got := LossPercent(s); got != 0 {
		t.Errorf("LossPercent = %v, want 0", got)
	}
}

// The first successful sample sets the minimum even if it is slower than a
// later one, and a genuine zero-duration sample is a valid minimum.
func TestUpdate_FirstSampleAndZero(t *testing.T) {
	s := &models.Stats{}
	Update(s, 100*time.Millisecond, errors.New("timeout")) // failure must not seed min/max
	if s.Minimum != 0 || s.Maximum != 0 || s.Total != 0 {
		t.Fatalf("failure changed timing fields: %+v", s)
	}
	Update(s, 30*time.Millisecond, nil)
	if s.Minimum != 30*time.Millisecond || s.Maximum != 30*time.Millisecond {
		t.Errorf("first sample: min=%v max=%v", s.Minimum, s.Maximum)
	}
	Update(s, 0, nil)
	if s.Minimum != 0 {
		t.Errorf("zero sample must become the minimum, got %v", s.Minimum)
	}
	if s.Maximum != 30*time.Millisecond {
		t.Errorf("maximum = %v", s.Maximum)
	}
}

func TestUpdate_FailuresAndLoss(t *testing.T) {
	s := &models.Stats{}
	Update(s, 10*time.Millisecond, nil)
	Update(s, 100*time.Millisecond, errors.New("timeout"))
	Update(s, 20*time.Millisecond, nil)
	Update(s, 100*time.Millisecond, errors.New("refused"))
	if s.Attempts != 4 || s.Connects != 2 || s.Failures != 2 {
		t.Errorf("counters = %d/%d/%d", s.Attempts, s.Connects, s.Failures)
	}
	if s.Total != 30*time.Millisecond {
		t.Errorf("Total = %v (failures must not be added)", s.Total)
	}
	if got := LossPercent(s); got != 50 {
		t.Errorf("LossPercent = %v, want 50", got)
	}
	if got := Average(s); got != 15*time.Millisecond {
		t.Errorf("Average = %v, want 15ms", got)
	}
}

func TestAverageAndLoss_ZeroReplies(t *testing.T) {
	s := &models.Stats{}
	if Average(s) != 0 || LossPercent(s) != 0 {
		t.Errorf("empty stats: avg=%v loss=%v", Average(s), LossPercent(s))
	}
	Update(s, time.Second, errors.New("timeout"))
	if Average(s) != 0 {
		t.Errorf("Average with zero replies = %v, want 0", Average(s))
	}
	if LossPercent(s) != 100 {
		t.Errorf("LossPercent = %v, want 100", LossPercent(s))
	}
}

func TestUpdate_LargeCountsDoNotOverflow(t *testing.T) {
	s := &models.Stats{Attempts: 1<<31 - 1, Connects: 1<<31 - 1}
	Update(s, time.Millisecond, nil)
	if s.Attempts != 1<<31 || s.Connects != 1<<31 {
		t.Errorf("counters wrapped: %d/%d", s.Attempts, s.Connects)
	}
}

func TestShowStats_Table(t *testing.T) {
	noColor(t)
	cfg := &models.Config{
		Host:  "example.test",
		Port:  "80",
		Proto: models.TCP,
		IPs: []models.IP{
			{IP: "192.0.2.1", IsIPv4: true},
			{IP: "192.0.2.2", IsIPv4: true},
			{IP: "2001:db8::1"},
		},
	}
	statsMap := map[string]*models.Stats{
		"192.0.2.1":   {Attempts: 4, Connects: 3, Failures: 1, Minimum: 1500 * time.Microsecond, Maximum: 3 * time.Millisecond, Total: 6 * time.Millisecond},
		"192.0.2.2":   {Attempts: 2, Connects: 0, Failures: 2},
		"2001:db8::1": {Attempts: 0},
	}
	var buf bytes.Buffer
	ShowStats(&buf, cfg, statsMap)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// blank, title, header, two rows (the address without attempts is skipped)
	if len(lines) != 5 {
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[1], "Statistics of ping example.test on tcp 80") {
		t.Errorf("title: %q", lines[1])
	}
	for _, want := range []string{"IP Address", "Attempted", "Connected", "Failed", "Minimum", "Maximum", "Average"} {
		if !strings.Contains(lines[2], want) {
			t.Errorf("header lacks %q: %q", want, lines[2])
		}
	}
	row := strings.Fields(lines[3])
	want := []string{"192.0.2.1", "4", "3", "1", "(25.00%)", "1.50ms", "3.00ms", "2.00ms"}
	if strings.Join(row, " ") != strings.Join(want, " ") {
		t.Errorf("row 1 = %v, want %v", row, want)
	}
	row = strings.Fields(lines[4])
	want = []string{"192.0.2.2", "2", "0", "2", "(100.00%)", "0.00ms", "0.00ms", "0.00ms"}
	if strings.Join(row, " ") != strings.Join(want, " ") {
		t.Errorf("row 2 = %v, want %v", row, want)
	}
	if strings.Contains(out, "2001:db8::1") {
		t.Error("address without attempts must be omitted")
	}
	// Header and rows align on the same width when colour is off.
	if len(lines[2]) != len(lines[3]) || len(lines[3]) != len(lines[4]) {
		t.Errorf("misaligned table:\n%s", out)
	}
}

func TestShowStats_EmptyAndNil(t *testing.T) {
	noColor(t)
	cfg := &models.Config{Host: "h", Port: "80", Proto: models.TCP, IPs: []models.IP{{IP: "192.0.2.1", IsIPv4: true}}}
	var buf bytes.Buffer
	ShowStats(&buf, cfg, map[string]*models.Stats{})
	if buf.Len() != 0 {
		t.Errorf("empty map must print nothing, got %q", buf.String())
	}
	ShowStats(&buf, cfg, map[string]*models.Stats{"192.0.2.1": nil})
	if strings.Count(buf.String(), "\n") != 3 {
		t.Errorf("nil entry must print title and header only, got %q", buf.String())
	}
}

func TestShowCurrent_Lines(t *testing.T) {
	noColor(t)
	var buf bytes.Buffer
	ShowCurrent(&buf, 1, 0, "192.0.2.1", 11, 10*time.Millisecond, nil)
	ShowCurrent(&buf, 2, 0, "192.0.2.1", 11, 100*time.Millisecond, errors.New("timeout"))
	ShowCurrent(&buf, 3, 2, "2001:db8::1", 11, 1500*time.Microsecond, nil)
	want := "  1\t  192.0.2.1\t   10.00ms\n" +
		"  2\t  192.0.2.1\t  100.00ms\tErr: timeout\n" +
		"3.2\t2001:db8::1\t    1.50ms\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

// The row format is a pure function of its arguments (no cached width).
func TestShowCurrent_WidthNotCached(t *testing.T) {
	noColor(t)
	var a, b bytes.Buffer
	ShowCurrent(&a, 1, 0, "1.1.1.1", 7, time.Millisecond, nil)
	ShowCurrent(&b, 1, 0, "192.0.2.100", 11, time.Millisecond, nil)
	if len(a.String()) == len(b.String()) {
		t.Errorf("width was not recomputed:\n%q\n%q", a.String(), b.String())
	}
}

func TestShowBanner(t *testing.T) {
	noColor(t)
	cfg := &models.Config{
		Host: "example.test", Port: "53", Proto: models.UDP, UDPPayloadHex: "abcd",
		IPs: []models.IP{{IP: "192.0.2.1", IsIPv4: true}, {IP: "2001:db8::1"}},
	}
	var buf bytes.Buffer
	ShowBanner(&buf, cfg)
	want := "Ping of example.test on udp 53 (2 IPs)\nIPv4: 192.0.2.1\nIPv6: 2001:db8::1\nPayload (hex): abcd\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
	buf.Reset()
	cfg.Proto, cfg.IPs = models.TCP, cfg.IPs[:1]
	ShowBanner(&buf, cfg)
	if !strings.HasPrefix(buf.String(), "Ping of example.test on tcp 53 (1 IP)\n") || strings.Contains(buf.String(), "Payload") {
		t.Errorf("tcp banner: %q", buf.String())
	}
}

func TestFormatting_ConcurrentIsRaceFree(_ *testing.T) {
	cfg := &models.Config{Host: "h", Port: "80", Proto: models.TCP, IPs: []models.IP{{IP: "192.0.2.1", IsIPv4: true}}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := &models.Stats{}
			for j := 0; j < 50; j++ {
				var err error
				if j%3 == 0 {
					err = errors.New("x")
				}
				Update(s, time.Duration(j)*time.Millisecond, err)
				ShowCurrent(io.Discard, int64(j), int64(i), "192.0.2.1", 9+i, time.Millisecond, err)
			}
			ShowBanner(io.Discard, cfg)
			ShowStats(io.Discard, cfg, map[string]*models.Stats{"192.0.2.1": s})
		}(i)
	}
	wg.Wait()
}

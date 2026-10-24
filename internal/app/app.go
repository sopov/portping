package app

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
	"github.com/sopov/portping/internal/stats"
)

const Name = "portping"

var Version = "dev"
var Commit = ""
var BuildDate = ""

// App runs the probe loop for one configuration.
type App struct {
	ctx   context.Context
	cfg   *models.Config
	out   io.Writer
	stats map[string]*models.Stats
}

// NewApp creates an App that writes its report to out.
func NewApp(ctx context.Context, cfg *models.Config, out io.Writer) *App {
	return &App{
		ctx:   ctx,
		cfg:   cfg,
		out:   out,
		stats: make(map[string]*models.Stats, len(cfg.IPs)),
	}
}

// Run probes every address cfg.Count times (forever when Nonstop) and prints
// the statistics table when it returns, also on cancellation.
func (a *App) Run() error {
	defer stats.ShowStats(a.out, a.cfg, a.stats)
	stats.ShowBanner(a.out, a.cfg)

	pingOpts := make(map[string]models.PingOptions, len(a.cfg.IPs))
	maxIPLen := 0
	for _, ip := range a.cfg.IPs {
		pingOpts[ip.IP] = models.PingOptions{
			Context: a.ctx,
			Config:  a.cfg,
			Address: net.JoinHostPort(ip.IP, a.cfg.Port),
			Payload: a.cfg.UDPPayload,
		}
		a.stats[ip.IP] = &models.Stats{IP: ip}
		if l := len(ip.IP); l > maxIPLen {
			maxIPLen = l
		}
	}
	singleIP := len(a.cfg.IPs) == 1

	var attempt int64
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for a.cfg.Nonstop || attempt < int64(a.cfg.Count) {
		attempt++
		batchStart := time.Now()

		for idx, ip := range a.cfg.IPs {
			select {
			case <-a.ctx.Done():
				return nil
			default:
			}

			// per-ping timeout context
			ctx, cancel := context.WithTimeout(a.ctx, a.cfg.TimeoutDur)
			opts := pingOpts[ip.IP]
			opts.Context = ctx

			t, err := a.Ping(opts)
			cancel()
			if errors.Is(err, context.Canceled) {
				// Interrupted mid-probe: not a verdict about the target.
				return nil
			}

			stats.Update(a.stats[ip.IP], t, err)
			sub := int64(idx + 1)
			if singleIP {
				sub = 0
			}
			stats.ShowCurrent(a.out, attempt, sub, ip.IP, maxIPLen, t, err)
		}
		if a.cfg.Nonstop || attempt < int64(a.cfg.Count) {
			if since := time.Since(batchStart); since < a.cfg.DelayDur {
				wait := a.cfg.DelayDur - since
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(wait)
				select {
				case <-a.ctx.Done():
					return nil
				case <-timer.C:
				}
			}
		}
	}
	return nil
}

// Summary aggregates the per-address statistics after Run.
type Summary struct {
	Targets   int // addresses probed
	Unreached int // addresses that never answered
	Attempts  int64
	Connects  int64
	Failures  int64
}

// AllReached reports whether every probed address answered at least once.
// It is false when nothing was probed at all.
func (s Summary) AllReached() bool {
	return s.Targets > 0 && s.Unreached == 0
}

// Summary returns the aggregated outcome of Run.
func (a *App) Summary() Summary {
	var s Summary
	for _, ip := range a.cfg.IPs {
		st := a.stats[ip.IP]
		if st == nil {
			continue
		}
		s.Targets++
		s.Attempts += st.Attempts
		s.Connects += st.Connects
		s.Failures += st.Failures
		if st.Connects == 0 {
			s.Unreached++
		}
	}
	return s
}

// Ping runs one probe according to the configured protocol.
func (a *App) Ping(opts models.PingOptions) (time.Duration, error) {
	if a.cfg.IsTCP() {
		return probe.PingTCP(opts)
	}
	return probe.PingUDP(opts)

}

package app

import (
	"context"
	"errors"
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

type App struct {
	ctx   context.Context
	cfg   *models.Config
	stats map[string]*models.Stats
}

func NewApp(ctx context.Context, cfg *models.Config) *App {
	return &App{
		ctx:   ctx,
		cfg:   cfg,
		stats: make(map[string]*models.Stats, len(cfg.IPs)),
	}
}

func (a *App) Run() error {
	defer stats.ShowStats(a.cfg, a.stats)
	stats.ShowBanner(a.cfg)

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

	var attempt int
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for a.cfg.Nonstop || attempt < a.cfg.Count {
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
			sub := idx + 1
			if singleIP {
				sub = 0
			}
			stats.ShowCurrent(a.cfg, attempt, sub, ip.IP, maxIPLen, t, err)
		}
		if a.cfg.Nonstop || attempt < a.cfg.Count {
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
	Attempts  int
	Connects  int
	Failures  int
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

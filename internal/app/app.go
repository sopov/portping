package app

import (
	"context"
	"github.com/sopov/portping/internal/models"
	"github.com/sopov/portping/internal/probe"
	"github.com/sopov/portping/internal/stats"
	"net"
	"time"
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

	for {
		if !a.cfg.Nonstop && attempt >= a.cfg.Count {
			break
		}
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

func (a *App) Ping(opts models.PingOptions) (time.Duration, error) {
	if a.cfg.IsTCP() {
		return probe.PingTCP(opts)
	}
	return probe.PingUDP(opts)

}

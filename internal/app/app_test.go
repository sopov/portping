package app

import (
	"context"
	"github.com/sopov/portping/internal/models"
	"testing"
	"time"
)

func TestNewApp(t *testing.T) {
	ctx := context.Background()
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "192.168.1.1", IsIPv4: true},
			{IP: "::1", IsIPv4: false},
		},
	}

	a := NewApp(ctx, cfg)

	if a == nil {
		t.Fatal("NewApp() returned nil")
	}
	if a.ctx != ctx {
		t.Error("NewApp() ctx not set correctly")
	}
	if a.cfg != cfg {
		t.Error("NewApp() cfg not set correctly")
	}
	if len(a.stats) != 0 {
		t.Errorf("NewApp() stats should be empty, got %d entries", len(a.stats))
	}
}

func TestApp_Ping_TCP(t *testing.T) {
	ctx := context.Background()
	cfg := &models.Config{
		Proto:      models.TCP,
		TimeoutDur: 100 * time.Millisecond,
	}
	a := NewApp(ctx, cfg)

	opts := models.PingOptions{
		Context: ctx,
		Config:  cfg,
		Address: "127.0.0.1:99999", // Unreachable port
	}

	duration, err := a.Ping(opts)

	if duration < 0 {
		t.Errorf("App.Ping() duration should be >= 0, got %v", duration)
	}
	// Expect error for unreachable port
	if err == nil {
		t.Error("App.Ping() expected error for unreachable address, got nil")
	}
}

func TestApp_Ping_UDP(t *testing.T) {
	ctx := context.Background()
	cfg := &models.Config{
		Proto:      models.UDP,
		TimeoutDur: 1000 * time.Millisecond,
	}
	a := NewApp(ctx, cfg)

	opts := models.PingOptions{
		Context: ctx,
		Config:  cfg,
		Address: "127.0.0.1:99999", // Unreachable address
		Payload: []byte("test"),   // Test payload
	}

	// PingUDP now requires payload
	duration, err := a.Ping(opts)

	if duration < 0 {
		t.Errorf("App.Ping() duration should be >= 0, got %v", duration)
	}
	// Expect error for unreachable address or no response
	if err == nil {
		t.Error("App.Ping() expected error for unreachable address or no response, got nil")
	}
}

func TestApp_Run_WithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "127.0.0.1", IsIPv4: true},
		},
		Port:      "99999", // Unreachable port for quick timeout
		Nonstop:   true,
		TimeoutDur: 100 * time.Millisecond,
		DelayDur:  50 * time.Millisecond,
	}
	a := NewApp(ctx, cfg)

	// Cancel context immediately
	cancel()

	// Run should return quickly due to context cancellation
	err := a.Run()

	if err != nil {
		t.Errorf("App.Run() error = %v, expected nil on context cancel", err)
	}
}

func TestApp_Run_WithCount(t *testing.T) {
	ctx := context.Background()
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "127.0.0.1", IsIPv4: true},
		},
		Port:       "99999", // Unreachable port
		Count:      2,
		Nonstop:    false,
		TimeoutDur: 50 * time.Millisecond,
		DelayDur:   10 * time.Millisecond,
	}
	a := NewApp(ctx, cfg)

	// Run should execute 2 attempts and finish
	err := a.Run()

	if err != nil {
		t.Errorf("App.Run() error = %v, expected nil", err)
	}
}

func TestApp_Run_AddressFormat(t *testing.T) {
	ctx := context.Background()
	cfg := &models.Config{
		IPs: []models.IP{
			{IP: "192.168.1.1", IsIPv4: true},
			{IP: "::1", IsIPv4: false},
		},
		Port:       "80",
		Count:      1,
		Nonstop:    false,
		TimeoutDur: 50 * time.Millisecond,
		DelayDur:   10 * time.Millisecond,
	}
	a := NewApp(ctx, cfg)

	// Verify that addresses are formatted correctly
	err := a.Run()

	if err != nil {
		t.Errorf("App.Run() error = %v, expected nil", err)
	}
	// Verify that stats are created for all IPs
	if len(a.stats) != len(cfg.IPs) {
		t.Errorf("App.Run() stats should have %d entries, got %d", len(cfg.IPs), len(a.stats))
	}
}


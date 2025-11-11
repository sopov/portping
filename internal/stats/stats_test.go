package stats

import (
	"errors"
	"github.com/sopov/portping/internal/models"
	"testing"
	"time"
)

func TestUpdate_Success(t *testing.T) {
	s := &models.Stats{
		Attempts: 0,
		Connects: 0,
		Failures: 0,
		Minimum:  0,
		Maximum:  0,
		Total:    0,
	}

	// Successful ping
	Update(s, 10*time.Millisecond, nil)

	if s.Attempts != 1 {
		t.Errorf("Expected Attempts = 1, got %d", s.Attempts)
	}
	if s.Connects != 1 {
		t.Errorf("Expected Connects = 1, got %d", s.Connects)
	}
	if s.Failures != 0 {
		t.Errorf("Expected Failures = 0, got %d", s.Failures)
	}
	if s.Minimum != 10*time.Millisecond {
		t.Errorf("Expected Minimum = 10ms, got %v", s.Minimum)
	}
	if s.Maximum != 10*time.Millisecond {
		t.Errorf("Expected Maximum = 10ms, got %v", s.Maximum)
	}
	if s.Total != 10*time.Millisecond {
		t.Errorf("Expected Total = 10ms, got %v", s.Total)
	}

	// Second successful ping
	Update(s, 20*time.Millisecond, nil)

	if s.Attempts != 2 {
		t.Errorf("Expected Attempts = 2, got %d", s.Attempts)
	}
	if s.Connects != 2 {
		t.Errorf("Expected Connects = 2, got %d", s.Connects)
	}
	if s.Minimum != 10*time.Millisecond {
		t.Errorf("Expected Minimum = 10ms, got %v", s.Minimum)
	}
	if s.Maximum != 20*time.Millisecond {
		t.Errorf("Expected Maximum = 20ms, got %v", s.Maximum)
	}
	if s.Total != 30*time.Millisecond {
		t.Errorf("Expected Total = 30ms, got %v", s.Total)
	}

	// Third successful ping
	Update(s, 5*time.Millisecond, nil)

	if s.Minimum != 5*time.Millisecond {
		t.Errorf("Expected Minimum = 5ms, got %v", s.Minimum)
	}
	if s.Maximum != 20*time.Millisecond {
		t.Errorf("Expected Maximum = 20ms, got %v", s.Maximum)
	}
}

func TestUpdate_Failure(t *testing.T) {
	s := &models.Stats{
		Attempts: 0,
		Connects: 0,
		Failures: 0,
	}

	err := errors.New("connection timeout")
	Update(s, 100*time.Millisecond, err)

	if s.Attempts != 1 {
		t.Errorf("Expected Attempts = 1, got %d", s.Attempts)
	}
	if s.Connects != 0 {
		t.Errorf("Expected Connects = 0, got %d", s.Connects)
	}
	if s.Failures != 1 {
		t.Errorf("Expected Failures = 1, got %d", s.Failures)
	}
	if s.Total != 0 {
		t.Errorf("Expected Total = 0, got %v", s.Total)
	}
}

func TestUpdate_Mixed(t *testing.T) {
	s := &models.Stats{}

	// Successful ping
	Update(s, 10*time.Millisecond, nil)
	// Failed ping
	Update(s, 100*time.Millisecond, errors.New("timeout"))
	// Successful ping
	Update(s, 20*time.Millisecond, nil)

	if s.Attempts != 3 {
		t.Errorf("Expected Attempts = 3, got %d", s.Attempts)
	}
	if s.Connects != 2 {
		t.Errorf("Expected Connects = 2, got %d", s.Connects)
	}
	if s.Failures != 1 {
		t.Errorf("Expected Failures = 1, got %d", s.Failures)
	}
	if s.Total != 30*time.Millisecond {
		t.Errorf("Expected Total = 25ms, got %v", s.Total)
	}
}

func TestShowStats_EmptyStats(_ *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Proto:   models.TCP,
		IPs:     []models.IP{{IP: "192.168.1.1", IsIPv4: true}},
		NoColor: true,
	}

	stats := make(map[string]*models.Stats)

	// No panic with empty stats
	ShowStats(cfg, stats)
}

func TestShowStats_WithNilEntry(_ *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Proto:   models.TCP,
		IPs:     []models.IP{{IP: "192.168.1.1", IsIPv4: true}},
		NoColor: true,
	}

	stats := make(map[string]*models.Stats)
	// Add nil entry -- not panic
	stats["192.168.1.1"] = nil

	ShowStats(cfg, stats)
}

func TestShowStats_WithZeroAttempts(_ *testing.T) {
	cfg := &models.Config{
		Host:    "example.com",
		Port:    "80",
		Proto:   models.TCP,
		IPs:     []models.IP{{IP: "192.168.1.1", IsIPv4: true}},
		NoColor: true,
	}

	stats := make(map[string]*models.Stats)
	stats["192.168.1.1"] = &models.Stats{
		IP:       models.IP{IP: "192.168.1.1", IsIPv4: true},
		Attempts: 0,
	}

	// Should not show stats with 0 attempts
	ShowStats(cfg, stats)
}

func TestShowCurrent(_ *testing.T) {
	cfg := &models.Config{
		NoColor: true,
	}
	maxIPLen := 15

	ShowCurrent(cfg, 1, 0, "192.168.1.1", maxIPLen, 10*time.Millisecond, nil)
	ShowCurrent(cfg, 2, 0, "192.168.1.1", maxIPLen, 100*time.Millisecond, errors.New("timeout"))
	ShowCurrent(cfg, 1, 1, "192.168.1.1", maxIPLen, 15*time.Millisecond, nil)
}

func TestShowBanner(_ *testing.T) {
	cfg := &models.Config{
		Host:  "example.com",
		Port:  "80",
		Proto: models.TCP,
		IPs: []models.IP{
			{IP: "192.168.1.1", IsIPv4: true},
		},
		NoColor: true,
	}

	ShowBanner(cfg)

	cfg.IPs = []models.IP{
		{IP: "192.168.1.1", IsIPv4: true},
		{IP: "::1", IsIPv4: false},
	}

	ShowBanner(cfg)
}

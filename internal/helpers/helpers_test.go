package helpers

import (
	"testing"
	"time"
)

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{"Valid port 1", "1", true},
		{"Valid port 80", "80", true},
		{"Valid port 443", "443", true},
		{"Valid port 65535", "65535", true},
		{"Invalid port 0", "0", false},
		{"Invalid port 65536", "65536", false},
		{"Invalid port negative", "-1", false},
		{"Invalid port not a number", "abc", false},
		{"Invalid port empty", "", false},
		{"Invalid port with spaces", " 80 ", false},
		{"Invalid port float", "80.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidPort(tt.port)
			if result != tt.expected {
				t.Errorf("ValidPort(%q) = %v, expected %v", tt.port, result, tt.expected)
			}
		})
	}
}

func TestMs2Float64(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected float64
	}{
		{"1 millisecond", 1 * time.Millisecond, 1.0},
		{"10 milliseconds", 10 * time.Millisecond, 10.0},
		{"100 milliseconds", 100 * time.Millisecond, 100.0},
		{"1 second", 1 * time.Second, 1000.0},
		{"500 microseconds", 500 * time.Microsecond, 0.5},
		{"0 duration", 0, 0.0},
		{"1.5 milliseconds", 1500 * time.Microsecond, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Ms2Float64(tt.duration)
			if result != tt.expected {
				t.Errorf("Ms2Float64(%v) = %v, expected %v", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestIsWindows(_ *testing.T) {
	// This test simply checks that the function does not panic.
	result := IsWindows()
	_ = result
}

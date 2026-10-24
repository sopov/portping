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
		{"Invalid port plus sign", "+80", false},
		{"Invalid port hex", "0x50", false},
		{"Valid port leading zeros", "0080", true},
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

func TestNormalizePort(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"80", "80", true},
		{"0080", "80", true},
		{"65535", "65535", true},
		{"0", "", false},
		{"65536", "", false},
		{"+80", "", false},
		{"-1", "", false},
		{" 80", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, err := NormalizePort(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("NormalizePort(%q) = %q, %v; want %q, ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
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

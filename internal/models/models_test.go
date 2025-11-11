package models

import (
	"testing"
)

func TestProto_String(t *testing.T) {
	tests := []struct {
		name     string
		proto    Proto
		expected string
	}{
		{"TCP", TCP, "tcp"},
		{"UDP", UDP, "udp"},
		{"Empty", Proto(""), ""},
		{"Custom", Proto("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.proto.String()
			if result != tt.expected {
				t.Errorf("Proto.String() = %q, expected %q", result, tt.expected)
			}
		})
	}
}


package colors

import (
	"github.com/fatih/color"
	"testing"
)

func TestNoColor(t *testing.T) {
	// Save original value
	originalValue := color.NoColor

	// Test setting to true
	NoColor(true)
	if !color.NoColor {
		t.Error("NoColor(true) should set color.NoColor to true")
	}

	// Test setting to false
	NoColor(false)
	if color.NoColor {
		t.Error("NoColor(false) should set color.NoColor to false")
	}

	// Restore original value
	NoColor(originalValue)
}

func TestColorFunctions(t *testing.T) {
	// Verify that functions don't panic and return strings
	tests := []struct {
		name string
		fn   func(...interface{}) string
		text string
	}{
		{"HRed", HRed, "test"},
		{"Red", Red, "test"},
		{"Yellow", Yellow, "test"},
		{"HYellow", HYellow, "test"},
		{"Green", Green, "test"},
		{"HGreen", HGreen, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.text)
			if result == "" {
				t.Error("Color function returned empty string")
			}
			// Verify that function doesn't panic
			_ = result
		})
	}
}


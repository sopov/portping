package colors

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func withColorState(t *testing.T, noColor bool) {
	t.Helper()
	orig := color.NoColor
	color.NoColor = noColor
	t.Cleanup(func() { color.NoColor = orig })
}

func TestDisableIsOneWay(t *testing.T) {
	withColorState(t, false)
	if !Enabled() {
		t.Fatal("Enabled() = false with colour on")
	}
	NoColor(false)
	if !Enabled() {
		t.Error("NoColor(false) must not change state")
	}
	NoColor(true)
	if Enabled() {
		t.Error("NoColor(true) must disable colour")
	}
	NoColor(false)
	if Enabled() {
		t.Error("NoColor(false) must not re-enable colour")
	}
}

func TestColorFunctionsHonourEnabled(t *testing.T) {
	fns := map[string]func(...interface{}) string{
		"HRed": HRed, "Red": Red, "Yellow": Yellow, "HYellow": HYellow, "Green": Green, "HGreen": HGreen,
	}
	withColorState(t, true)
	for name, fn := range fns {
		if got := fn("x"); got != "x" {
			t.Errorf("%s(x) with colour off = %q, want plain", name, got)
		}
	}
	withColorState(t, false)
	for name, fn := range fns {
		if got := fn("x"); !strings.Contains(got, "\x1b[") || !strings.Contains(got, "x") {
			t.Errorf("%s(x) with colour on = %q, want escape sequence", name, got)
		}
	}
}

func TestErrPlainWhenStderrNotTerminal(t *testing.T) {
	withColorState(t, false)
	if stderrIsTerminal {
		t.Skip("stderr is a terminal in this run")
	}
	if got := Err("boom"); got != "boom" {
		t.Errorf("Err() = %q, want plain text on a non-terminal stderr", got)
	}
}

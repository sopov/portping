// Package colors wraps fatih/color so that colour is decided in one place:
// on by default only when stdout is a terminal (and NO_COLOR is unset), off
// for good once Disable is called.
package colors

import (
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

var (
	HRed = color.New(color.FgHiRed).SprintFunc()
	Red  = color.New(color.FgRed).SprintFunc()

	Yellow  = color.New(color.FgYellow).SprintFunc()
	HYellow = color.New(color.FgHiYellow).SprintFunc()

	Green  = color.New(color.FgGreen).SprintFunc()
	HGreen = color.New(color.FgHiGreen).SprintFunc()
)

// stderrIsTerminal is evaluated once; stdout detection lives in fatih/color.
var stderrIsTerminal = isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())

// Disable turns colour off for the rest of the process.
func Disable() {
	color.NoColor = true
}

// NoColor disables colour when disable is true. It never turns colour on, so
// the library's terminal detection (and NO_COLOR) stays in effect.
func NoColor(disable bool) {
	if disable {
		Disable()
	}
}

// Enabled reports whether colour sequences are currently emitted on stdout.
func Enabled() bool {
	return !color.NoColor
}

// Err formats a message for stderr. stderr is coloured only when it is a
// terminal itself; the stdout decision does not carry over to a redirected
// stderr.
func Err(s string) string {
	if Enabled() && stderrIsTerminal {
		return Red(s)
	}
	return s
}

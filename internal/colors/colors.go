package colors

import "github.com/fatih/color"

var (
	HRed = color.New(color.FgHiRed).SprintFunc()
	Red  = color.New(color.FgRed).SprintFunc()

	Yellow  = color.New(color.FgYellow).SprintFunc()
	HYellow = color.New(color.FgHiYellow).SprintFunc()

	Green  = color.New(color.FgGreen).SprintFunc()
	HGreen = color.New(color.FgHiGreen).SprintFunc()
)

// NoColor forces colour off when disable is true. It never turns colour on,
// so the library's terminal detection (and NO_COLOR) stays in effect.
func NoColor(disable bool) {
	if disable {
		color.NoColor = true
	}
}

// Err formats an error message for stderr.
func Err(s string) string {
	return Red(s)
}

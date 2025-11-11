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

func NoColor(disable bool) {
	color.NoColor = disable
}

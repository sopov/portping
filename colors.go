package main

import "github.com/fatih/color"

var (
	hred = color.New(color.FgHiRed).SprintFunc()
	red  = color.New(color.FgRed).SprintFunc()

	yellow  = color.New(color.FgYellow).SprintFunc()
	hyellow = color.New(color.FgHiYellow).SprintFunc()

	green  = color.New(color.FgGreen).SprintFunc()
	hgreen = color.New(color.FgHiGreen).SprintFunc()
)

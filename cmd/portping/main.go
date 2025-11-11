package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/cli"
	"github.com/sopov/portping/internal/colors"
	"os"
	"os/signal"
	"syscall"
)

const (
	exitOK       = 0
	exitUsage    = 2
	exitRuntime  = 1
	exitCanceled = 130 // 128+SIGINT
)

func main() {

	version()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := cli.Parse()
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, cli.Usage())
			os.Exit(exitOK)
		}
		fmt.Fprintln(os.Stderr, colors.Red(err.Error()))
		fmt.Fprintln(os.Stderr, cli.Usage())
		os.Exit(exitUsage)
	}

	if err := app.NewApp(ctx, cfg).Run(); err != nil {
		fmt.Fprintln(os.Stderr, colors.Red(err.Error()))
		os.Exit(exitRuntime)
	}

	if ctx.Err() != nil {
		os.Exit(exitCanceled)
	}
}

func version() {
	flag.Bool("version", false, "Print version and exit")
	for _, a := range os.Args[1:] {
		if a == "version" || a == "-version" || a == "--version" {
			fmt.Fprintf(os.Stdout, "%s %s (%s, %s)\n", app.Name, app.Version, app.Commit, app.BuildDate)
			os.Exit(exitOK)
		}
	}
}

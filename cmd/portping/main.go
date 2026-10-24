package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sopov/portping/internal/app"
	"github.com/sopov/portping/internal/cli"
	"github.com/sopov/portping/internal/colors"
)

// Exit codes. 0 means every probed address answered at least once.
const (
	exitOK        = 0
	exitUnreached = 1 // at least one address never answered, or a runtime error
	exitUsage     = 2 // bad flags or arguments
	exitSigBase   = 128
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		printVersion(stdout)
		return exitOK
	}

	cfg, err := cli.ParseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stdout, cli.Usage())
		return exitOK
	}
	if err != nil {
		fmt.Fprintln(stderr, colors.Err(err.Error()))
		fmt.Fprintln(stderr, cli.Usage())
		return exitUsage
	}
	if cfg.ShowVersion {
		printVersion(stdout)
		return exitOK
	}

	if err := cli.Resolve(cfg); err != nil {
		fmt.Fprintln(stderr, colors.Err(err.Error()))
		return exitUnreached
	}

	ctx, sig, stop := signalContext(context.Background())
	defer stop()

	a := app.NewApp(ctx, cfg, stdout)
	if err := a.Run(); err != nil {
		fmt.Fprintln(stderr, colors.Err(err.Error()))
		return exitUnreached
	}

	if s := sig(); s != 0 {
		return exitSigBase + int(s)
	}
	if !a.Summary().AllReached() {
		return exitUnreached
	}
	return exitOK
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "%s %s (%s, %s)\n", app.Name, app.Version, app.Commit, app.BuildDate)
}

// signalContext returns a context that is cancelled by SIGINT or SIGTERM, a
// function reporting which signal arrived (0 if none), and a stop function.
// After the first signal the default disposition is restored, so a second
// Ctrl-C terminates the process immediately.
func signalContext(parent context.Context) (context.Context, func() syscall.Signal, func()) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	var (
		mu  sync.Mutex
		got syscall.Signal
	)
	go func() {
		select {
		case s := <-ch:
			signal.Stop(ch)
			if ss, ok := s.(syscall.Signal); ok {
				mu.Lock()
				got = ss
				mu.Unlock()
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	report := func() syscall.Signal {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
	stop := func() {
		signal.Stop(ch)
		cancel()
	}
	return ctx, report, stop
}

package main

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func run2(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	orig := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = orig }()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_HelpAndVersion(t *testing.T) {
	code, out, errOut := run2(t, "-h")
	if code != exitOK || !strings.HasPrefix(out, "portping [options] <destination> <port>") || errOut != "" {
		t.Errorf("-h: code=%d out=%q err=%q", code, out, errOut)
	}
	for _, args := range [][]string{{"version"}, {"-version"}, {"--version"}} {
		code, out, _ := run2(t, args...)
		if code != exitOK || !strings.HasPrefix(out, "portping ") {
			t.Errorf("%v: code=%d out=%q", args, code, out)
		}
	}
}

func TestRun_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-bogus", "127.0.0.1", "80"},
		{"127.0.0.1"},
		{"127.0.0.1", "99999"},
		{"-udp", "127.0.0.1", "53"},
		{"-t", "0", "127.0.0.1", "80"},
	} {
		code, out, errOut := run2(t, args...)
		if code != exitUsage {
			t.Errorf("%v: code=%d, want %d", args, code, exitUsage)
		}
		if out != "" || !strings.Contains(errOut, "portping [options]") {
			t.Errorf("%v: usage must go to stderr only; out=%q err=%q", args, out, errOut)
		}
	}
}

func TestRun_ResolutionErrorIsRuntime(t *testing.T) {
	code, out, errOut := run2(t, "-c", "1", "-6", "127.0.0.1", "80")
	if code != exitUnreached || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
	if !strings.Contains(errOut, "IPv4 address") || strings.Contains(errOut, "portping [options]") {
		t.Errorf("stderr = %q, want a one-line family error without usage", errOut)
	}
}

func TestRun_ExitCodesFromProbes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	_, open, _ := net.SplitHostPort(ln.Addr().String())

	closedLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, closed, _ := net.SplitHostPort(closedLn.Addr().String())
	_ = closedLn.Close()

	code, out, _ := run2(t, "-c", "2", "-d", "1", "-t", "500", "127.0.0.1", open)
	if code != exitOK || !strings.Contains(out, "Statistics of ping") {
		t.Errorf("open port: code=%d out=%q", code, out)
	}
	code, out, _ = run2(t, "-c", "2", "-d", "1", "-t", "500", "127.0.0.1", closed)
	if code != exitUnreached || !strings.Contains(out, "Err: ") {
		t.Errorf("closed port: code=%d out=%q", code, out)
	}
}

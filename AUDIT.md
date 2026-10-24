# portping security and correctness audit (2026-10)

Scope: `cmd/`, `internal/`, build and release tooling (`Makefile`, `.github/workflows`,
`.golangci.yml`, `go.mod`), and the GitHub Pages site files (`_config.yml`, `_layouts/`,
`index.md`, `CNAME`, `Gemfile`, `.jekyllignore`).

Baseline: commit `60702f8` (master). All `file:line` references in the findings below
point at that commit, not at the fixed code.

Method: full read of every source file, `go vet`, `go test -race`, `golangci-lint v2.5.0`
with a probe configuration, `go list -m -u all`, and black-box runs of the built binary
for every scenario listed as "observed" below. Platform-specific claims (Windows) were
verified against the Go 1.25.4 standard library sources rather than on a Windows host.

Tooling results at baseline:

| Check | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race ./...` | 8 failures in `internal/cli` when DNS is unavailable (tests resolve `example.com`) |
| `gofmt -l` | 3 test files not formatted |
| `golangci-lint run` | refuses to start: `.golangci.yml` is in the v1 format |
| `govulncheck ./...` | could not run in the audit sandbox (`vuln.go.dev` is not reachable from it); runs in CI, see M8 |
| `go list -m -u all` | every dependency behind: `fatih/color` 1.18.0 (1.19.0), `go-colorable` 0.1.13 (0.1.16), `go-isatty` 0.0.20 (0.0.24), `x/sys` 0.25.0 (0.49.0) |
| tracked binaries | none; `dist/` and `*.exe` are ignored and were never committed in any revision |
| GoReleaser | no configuration exists; releases are built by `make dist` and uploaded with `softprops/action-gh-release` |

Severity scale: **blocker** (tool gives wrong answers or is unusable on a supported
platform), **major** (wrong behaviour in a realistic scenario, or a security or
supply-chain weakness), **minor** (correctness or robustness defect with limited impact),
**note** (observation, no defect).

---

## 1. Findings

### Blockers

#### B1. UDP replies are reported as failures on Windows
- `internal/probe/probe.go:116-117`
- `PingUDP` reads the reply into a 1-byte buffer. On Windows `WSARecv` on a datagram
  socket returns `WSAEMSGSIZE` when the datagram is larger than the buffer, and Go's
  `internal/poll.(*FD).Read` passes that error through unchanged
  (`$GOROOT/src/internal/poll/fd_windows.go`, `Read`, `kindNet` branch; the only place
  `WSAEMSGSIZE` is swallowed is the 0-byte peek used by `RawRead`). Every reply produced by
  the shipped presets (DNS, NTP, STUN) is longer than one byte, so on Windows every UDP
  probe that *does* get an answer is printed as `Err: wsarecv: A message sent on a datagram
  socket was larger than the internal message buffer ...` and counted as a failure, while a
  silent port looks identical to a dead one. On Linux and macOS the kernel truncates
  silently, which is why the bug is invisible to the author.
- Failure scenario: `portping_windows_amd64.exe -dns 8.8.8.8` reports 100% failure against a
  working resolver.
- Fix: read into a 64 KiB buffer (the maximum UDP datagram), reused via a `sync.Pool`.

#### B2. Exit status is 0 when every probe fails
- `internal/app/app.go:106`, `cmd/portping/main.go:41-48`
- `App.Run` always returns `nil`, and `main` only maps parse errors (2) and interrupts
  (130). A reachability checker whose exit code does not encode reachability is unusable
  from scripts, health checks and CI.
- Observed: `portping -c 2 127.0.0.1 1` (connection refused twice) exits 0.
- Fix: exit 1 when at least one resolved address never answered during the run; keep 0 when
  every address answered at least once; keep 2 for usage errors and 130/143 for signals.
  Document the policy in README. Behavioural change, see section 3.

### Major

#### M1. Bare IPv6 literals are split as `host:port`
- `internal/cli/cli.go:149-154`, `internal/cli/cli.go:207-226`
- Any first argument containing a colon is passed to `trySplitHostPort`, whose fallback
  treats the text after the *last* colon as a port. An unbracketed IPv6 address therefore
  never reaches the resolver intact.
- Observed: `portping ::1 80` probes host `:` port `1` and fails with
  ``resolve host `:` ``; `portping 2001:db8::1 80` probes host `2001:db8:` port `1`. The
  only form that works is the ambiguous `::1:80`, which is itself a valid IPv6 address.
- Fix: if the first argument parses as an IP literal it is the host and the port must come
  from the second argument or a preset; `[addr]:port` and `host:port` remain supported;
  reject unbracketed `addr:port` for IPv6 with a message pointing at the bracket form.
  Behavioural change for `::1:80`, see section 3.

#### M2. IPv6 zone identifiers are dropped
- `internal/probe/probe.go:52`, `internal/probe/probe.go:62`, `internal/app/app.go:42`
- `net.ParseIP` rejects `fe80::1%eth0`, so the literal falls through to
  `LookupIP`, which strips the zone. The probe then dials `[fe80::1]:port`, which fails
  with "invalid argument"/"no route to host" or picks the wrong interface.
- Observed: `portping '[fe80::1%lo]:80'` prints `IPv6: fe80::1` and dials without the zone.
- Fix: parse literals with `netip.ParseAddr`, keep the zone in the stored address, and let
  `net.JoinHostPort` carry it into the dial string.

#### M3. Ctrl-C is ignored while a UDP probe waits for a reply
- `internal/probe/probe.go:113-117`
- The context only governs `DialContext`; the read uses a fixed deadline. The signal
  context cancels, but nothing wakes the blocked `Read`, so the process lingers for the
  full `-t` value (which users set high for lossy links) and the final statistics are
  delayed. Hanging reads also mean a hidden second timeout: write deadline and read
  deadline are both `TimeoutDur` from "now", so one probe may exceed `-t`.
- Observed: with `-t 5000` and a listener that never answers, SIGINT 0.5 s into the probe
  exits after 5.01 s.
- Fix: derive one absolute deadline from the probe context, and use `context.AfterFunc` to
  slam the connection deadline when the context is cancelled. Do not count an interrupted
  probe as a failure.

#### M4. Colour escapes are written when stdout is not a terminal
- `internal/cli/cli.go:80`, `internal/colors/colors.go:16-18`
- `fatih/color` disables colour at init when stdout is not a TTY or `TERM=dumb`.
  `parseWith` then calls `colors.NoColor(cfg.NoColor)` with `false`, overriding that
  decision, so pipes and log files receive ANSI sequences. Only `NO_COLOR` still works
  because that check is per colour object.
- Observed: `portping -c 1 127.0.0.1 1 | od -c` contains 22 escape sequences.
- Related: the column widths in `ShowStats`/`showCurrentFmt` are chosen from `cfg.NoColor`
  (`internal/stats/stats.go:63-65`, `:110-112`), not from whether colour is actually
  enabled, so once auto-detection works the table would misalign; and error messages are
  coloured for stderr based on the stdout decision (`cmd/portping/main.go:36,42`).
- Fix: only *disable* colour from the flag, never enable it; expose `colors.Enabled()` and
  use it for width selection; colour stderr only when stderr is a terminal.

#### M5. Help and flag-error handling in `main` is dead code
- `cmd/portping/main.go:30-39`, `internal/cli/cli.go:28-30`
- `cli.Parse` uses `flag.CommandLine`, whose error handling is `ExitOnError`. `-h`
  therefore prints the bare flag list to **stderr** and exits before `Parse` returns
  `flag.ErrHelp`, so the custom `cli.Usage()` (with the synopsis line) is never shown; an
  unknown flag prints `Usage of ./portping:` and exits 2 before the red error path runs.
- Observed: `portping -h >/dev/null` prints 45 lines to stderr and nothing to stdout.
- Fix: parse with a private `FlagSet` in `ContinueOnError` mode with output discarded, so
  `main` owns all printing and exit codes.

#### M6. An unresolvable host is treated as a usage error
- `internal/cli/cli.go:247-250`, `cmd/portping/main.go:36-38`
- DNS resolution happens inside `Validate`, so a lookup failure returns through the same
  path as a bad flag: exit code 2 and the full option list is dumped after the error.
- Observed: `portping nonexistent.invalid 80` prints 53 lines and exits 2.
- Fix: split validation (pure) from resolution (network); resolution failures exit 1 with a
  one-line message.

#### M7. The test suite requires Internet access
- `internal/cli/cli_test.go:171-399` (eight `TestParse*` functions resolve
  `example.com`), `internal/probe/probe_test.go:237-320` (TCP to `1.1.1.1:443`, UDP DNS
  query to `1.1.1.1:53`), `internal/probe/probe_test.go:322-402` (`localhost` lookups).
- `go test ./...` fails on any machine without DNS, flakes on slow links, and sends
  traffic to a third party on every run. Several probe tests also use port `99999`, which
  is an invalid port rather than a closed one, so they test the address parser, not the
  probe.
- Fix: table tests with literal addresses, a fake resolver injected into `probe`, and
  local `net.Listen`/`net.ListenPacket` servers.

#### M8. Lint configuration is unusable and no CI runs any check
- `.golangci.yml:1-11`, `.github/workflows/` (no CI workflow)
- The config is v1 syntax; golangci-lint v2 exits with "unsupported version of the
  configuration". Nothing runs `go vet`, `go test -race`, the linter or `govulncheck` on
  push or pull request; the only automation is the tag-triggered release, which builds
  without testing (`.github/workflows/release.yml:20-22`).
- Fix: v2 configuration that passes, and a CI workflow with a `permissions` block running
  vet, race tests, golangci-lint and govulncheck.

#### M9. Release supply chain: unpinned third-party action with write token, no checksums
- `.github/workflows/release.yml:8-9` grants `contents: write` at workflow level;
  `:65` runs `softprops/action-gh-release@v2`, a third-party action referenced by a
  mutable tag, with that token; `:17-19` uses `actions/setup-go@v4` (superseded) with a
  Go version duplicated from `go.mod`; no `SHA256SUMS`, signature, provenance or SBOM is
  produced, so users of the README's `curl | chmod +x` recipe cannot verify what they run;
  no Dependabot configuration exists for Go modules or actions.
- Fix (this audit): generate and upload `SHA256SUMS.txt`, test before building, move
  permissions to job level, `setup-go@v5` with `go-version-file`, add Dependabot, document
  checksum verification. Recommended beyond this audit: pin actions to commit SHAs and
  adopt GoReleaser with cosign/SLSA provenance (section 4).

#### M10. DNS lookup timeout is tied to the per-probe timeout
- `internal/probe/probe.go:59`
- `LookupIP` runs with `cfg.TimeoutDur`. `-t 100` is a normal value for LAN checks, but
  an uncached recursive lookup often takes longer, so the tool fails before the first
  probe with ``resolve host `x`: i/o timeout``.
- Fix: resolve with `max(-t, 5s)`.

### Minor

#### m1. `-t` and `-d` overflow when converted to `time.Duration`
- `internal/cli/cli.go:76-77`, `internal/cli/cli.go:235-240`
- Only `>= 1` is checked. `int(ms) * time.Millisecond` wraps for values above
  `9.2e12`; the result is a negative duration, i.e. an already-expired deadline.
- Fix: upper bound of 24 h for both.

#### m2. UDP payload size is unbounded
- `internal/cli/cli.go:63-69`
- A hex payload longer than 65 507 bytes decodes fine and then every probe fails with
  `message too long`. Combined with the missing bound on argument length this is a
  confusing failure mode rather than a validation error.
- Fix: reject payloads above 65 507 bytes at parse time.

#### m3. Port strings are accepted verbatim
- `internal/helpers/helpers.go:10-13`
- `strconv.Atoi` accepts `+80` and `0080`; the banner and statistics then print the
  un-normalised text and the dial string contains `+80`.
- Fix: `strconv.ParseUint(port, 10, 16)` and store the canonical decimal form.

#### m4. Extra positional arguments are silently ignored
- `internal/cli/cli.go:147-161`
- `portping host:80 443` probes port 80; `portping host 80 junk` ignores `junk` in TCP
  mode. The user's intent is lost without a diagnostic.
- Fix: reject unexpected arguments.

#### m5. `-payload` is silently discarded in TCP mode
- `internal/cli/cli.go:71-74`
- An explicit flag that has no effect should be an error.
- Fix: `-payload`/positional payload without UDP is a usage error.

#### m6. `version` is detected by scanning all arguments
- `cmd/portping/main.go:51-59`
- A host literally named `version` (or any argument `-version` in any position, including
  flag values) prints the version and exits instead of probing.
- Fix: honour `-version`/`--version` as a real flag and the bare word `version` only as the
  sole argument.

#### m7. Preset names are case-sensitive
- `internal/cli/cli.go:166`, `internal/probe/predefined_proto.go:28-31`
- `-preset DNS` fails with `invalid preset "DNS"` while the help text prints presets in
  upper case.
- Fix: lower-case the lookup.

#### m8. Package-level mutable state makes parsing and formatting non-reentrant
- `internal/cli/cli.go:21-26` (`cfgFlags`), `internal/stats/stats.go:13-15`
  (`okFmt`/`errFmt` cached with the first call's width), `internal/cli/cli.go:277`
  (`flag.CommandLine.SetOutput` never restored).
- Harmless in a single run, but it is why the existing tests must rebuild
  `flag.CommandLine`, and it would be a data race if probing ever became concurrent.
- Fix: locals only; compute the row format per call.

#### m9. UDP probe timing is measured inconsistently
- `internal/probe/probe.go:99-119`
- `start` is taken before the dial, but write and read deadlines each start after it, so a
  probe can take up to roughly `2 × -t` plus the dial, while the printed time is measured
  from `start`. Covered by the M3 fix (single absolute deadline).

#### m10. Statistics: zero sentinel and 32-bit counters
- `internal/stats/stats.go:157`, `internal/models/models.go:48-56`, `internal/app/app.go:52`
- The minimum is initialised through `float64(stats.Minimum) == 0` instead of "first
  successful sample"; counters are `int`, which is 32-bit on the released `386`, `arm`,
  `mips*` builds and overflows after 2^31 attempts (about 25 days at `-d 1`). Average and
  loss percentage are otherwise correct and guarded against zero divisors
  (`stats.go:85-90`).
- Fix: `int64` counters, min/max keyed on `Connects == 1`.

#### m11. `-c` help text describes the wrong thing
- `internal/cli/cli.go:94`
- "Stop after connecting count times" counts attempts, not successful connections (README
  says attempts).
- Fix: wording.

#### m12. Failure reporting mixes stdout TTY state with stderr output
- `cmd/portping/main.go:36,42` — folded into M4.

#### m13. Dependencies behind; vulnerability scan not possible in the audit sandbox
- `go.mod:5-11`
- All four modules have newer releases. No known vulnerability affects the APIs used
  (`fatih/color`, `go-isatty`, `go-colorable`, `x/sys` as a transitive of those), but
  `govulncheck` must run in CI to keep that true.
- Fix: update modules; run `govulncheck` in CI (M8).

#### m14. Jekyll site configuration
- `_config.yml:10` `remote_theme: jekyll/minima` is unpinned, so every build pulls the
  theme's default branch (currently a 3.0 development line whose includes may change);
  `_layouts/content.html:4,7,15` depends on `head.html`, `header.html`, `footer.html` from
  that theme.
- `_config.yml:46-50` `include: README.md` together with `permalink: /:path/` publishes the
  README a second time at `/README/`; the site should exclude `AUDIT.md`, `LICENSE` and
  the `Gemfile` as well.
- `_config.yml:22` `github: [metadata]` is a legacy no-op; `:28-33` exclude directories that
  do not exist.
- `Gemfile:1-9` is not used by `actions/jekyll-build-pages` (which ships the `github-pages`
  gem on Jekyll 3.10) and pins Jekyll 4, so local `bundle exec jekyll serve` builds a
  different site than production; the first line has a leading space.
- `.jekyllignore` is not a Jekyll feature; the file is dead.
- `.github/workflows/pages.yml:23` `fetch-depth: 0` is unnecessary.
- No secrets or tokens in any site file; `CNAME` is correct.
- Fix: pin the theme (`jekyll/minima@v2.5.1`), exclude `AUDIT.md`, `LICENSE`, `Gemfile`,
  `README.md` duplication, drop the dead file, align the Gemfile with `github-pages`.

#### m15. README install recipe
- `README.md:30-38`
- No checksum verification; `uname -m` is not mapped for `armv7l`/`i686`; the "Build"
  badge points at the release workflow, not a CI run.
- Fix: verify `SHA256SUMS.txt`; map the architectures; point the badge at CI.

#### m16. `make dist` hides failed targets
- `Makefile:29-40`
- `set +e` plus a `skipped` message means a platform can silently vanish from a release.
- Fix: collect failures and exit non-zero at the end (no supported target needs CGO).

#### m17. Lint findings at baseline
- `cmd/portping/main.go:33,55` unchecked `Fprint*` errors; `internal/probe/probe.go:105`
  `defer conn.Close()` unchecked; `cmd/portping/main.go:34` `os.Exit` after `defer stop()`;
  `internal/app/app_test.go`, `internal/colors/colors_test.go`,
  `internal/models/models_test.go` not `gofmt`-ed; imports not grouped; `cli.go:113` has a
  comment in Russian in an otherwise English code base.
- Fix: clean up as part of the lint commit.

#### m23. SIGTERM exits with 130
- `cmd/portping/main.go:20,46-48`
- `signal.NotifyContext` does not say which signal arrived, so `kill -TERM` (what
  systemd, Docker and CI runners send) reports 130 instead of 143.
- Fix: a small signal handler that records the signal.

#### m24. UDP semantics are undocumented
- `README.md`
- "Connected" for UDP means "a datagram came back within `-t`"; a timeout means "no reply"
  (open-but-silent, filtered, or down), and "connection refused" means an ICMP port
  unreachable was received (port closed; on Windows the same arrives as
  `WSAECONNRESET`). None of this is written down, so users read 100% loss on a silent
  service as "down".
- Fix: document.

#### m25. Family mismatch on a literal gives a generic error
- `internal/probe/probe.go:52-57`
- `portping -6 1.2.3.4 80` fails with `no addresses found`.
- Fix: say which family the literal is and which flag excluded it.

#### m26. Multi-address ordering is lexicographic
- `internal/cli/cli.go:256-268`
- `10.0.0.2` sorts before `9.0.0.1`. Cosmetic but surprising in the banner and table.
- Fix: compare with `netip.Addr.Compare`.

### Notes

- **n1.** Input validation and injection: host, port, preset and payload values are only
  ever passed to Go library APIs (`net`, `encoding/hex`, map lookups); nothing is
  interpolated into a shell or a format string. No injection surface was found.
- **n2.** DNS: `LookupIP(ctx, "ip", host)` returns both families and the flags filter
  them. Hosts resolving to both are handled correctly once `-4 -6` are given; the default
  is IPv4 only, which matches the README. IPv4-mapped IPv6 answers are normalised to IPv4.
- **n3.** Continuous mode (`-c 0`): memory is constant (per-address stats structs, one
  reused timer, no accumulation); the 64 KiB UDP buffer introduced by B1 is pooled. No
  goroutines are created by the probe loop, so there is nothing to leak.
- **n4.** Concurrency: the program is single-goroutine apart from the signal handler;
  `go test -race` reports nothing. The shared state in m8 is the only candidate for a race
  and is removed. A race test for concurrent probes and formatting is added anyway.
- **n5.** TCP semantics: a completed three-way handshake followed by an immediate close
  (FIN). Services with connection logging will see one empty connection per probe; this is
  inherent to TCP "ping" and documented.
- **n6.** `go.mod` pins `go 1.25.4`; with `GOTOOLCHAIN=auto` this downloads the exact
  toolchain on older installs, which is fine for reproducibility.
- **n7.** `models.PingOptions` duplicates `Config` fields (`Payload`, `Address`); kept for
  compatibility of internal call sites, but see section 4.
- **n8.** No GoReleaser configuration exists although the project description mentions it;
  the audit covers the Makefile plus `action-gh-release` pipeline that is actually used.

---

## 2. Fix status

Every blocker and major finding is fixed. Every minor finding that is a correctness or
security issue is fixed. Nothing was deferred for lack of time; the two items not done in
this branch could not be done from the audit sandbox and are listed at the end of this
section.

Checks after the fixes (`go vet`, `go test -race -count=3`, `golangci-lint run` with the
new configuration, `make dist` for all 34 release targets): clean. The test suite no
longer opens any connection that leaves the loopback interface and no longer performs any
DNS lookup.

| Finding | Status | Commit |
|---|---|---|
| B1 UDP replies fail on Windows | fixed | `probe: 64 KiB UDP receive buffer, shared probe deadline, cancellation wakes reads` |
| B2 exit 0 on total failure | fixed | `cli,main: own flag parsing, working help and errors, version flag, exit codes, signal handling` |
| M1 bare IPv6 literal split as host:port | fixed | `cli,probe: parse IPv6 literals safely, keep zones, ...` |
| M2 zone identifiers dropped | fixed | same |
| M3 Ctrl-C ignored during UDP read | fixed | `probe: 64 KiB UDP receive buffer, ...` |
| M4 colour on non-terminals | fixed | `output: honour terminal detection, ...` |
| M5 dead help/error paths | fixed | `cli,main: own flag parsing, ...` |
| M6 unresolvable host is a usage error | fixed | same |
| M7 tests need Internet | fixed | `probe: ...` (probe tests), `cli,probe: ...` (resolver fake), `tests: loopback-only app and main tests, concurrent-run race test` |
| M8 lint config unusable, no CI | fixed | `ci: golangci-lint v2 config, CI workflow, release checksums, dependabot, dependency update` |
| M9 release supply chain | fixed in part | same: checksums, job-level permissions, tests before build, `setup-go@v5` with `go-version-file`, Dependabot. Not done: pinning actions to commit SHAs (see below) |
| M10 DNS timeout tied to `-t` | fixed | `cli,probe: parse IPv6 literals safely, ...` |
| m1 `-t`/`-d` overflow | fixed | `cli: bound -t/-d, canonical ports, payload size limit, ...` |
| m2 unbounded payload | fixed | same |
| m3 ports accepted verbatim | fixed | same |
| m4 extra positional arguments | fixed | `cli,probe: parse IPv6 literals safely, ...` |
| m5 `-payload` ignored in TCP mode | fixed | `cli: bound -t/-d, ...` |
| m6 `version` scanned from all arguments | fixed | `cli,main: own flag parsing, ...` |
| m7 case-sensitive presets | fixed | `cli: bound -t/-d, ...` |
| m8 package-level mutable state | fixed | `cli,main: ...` (private FlagSet), `output: ...` (format cache removed) |
| m9 UDP timing | fixed | `probe: ...` |
| m10 min sentinel, 32-bit counters | fixed | `output: ...` |
| m11 `-c` help text | fixed | `cli: bound -t/-d, ...` |
| m12 stderr colour | fixed | `output: ...` |
| m13 dependencies | updated to the newest versions that build with Go 1.25 (`x/sys` 0.49 and `x/tools` current require Go 1.26); `govulncheck` runs in CI | `ci: ...` |
| m14 Jekyll configuration | fixed | `site,docs: pin Jekyll theme, ...` |
| m15 README install recipe | fixed | same |
| m16 `make dist` hides failures | fixed | `ci: ...` |
| m17 lint findings | fixed | `ci: ...` and later commits |
| m20 widths follow flag not state | fixed | `output: ...` |
| m23 SIGTERM exit code | fixed | `cli,main: ...` |
| m24 UDP semantics undocumented | fixed | `site,docs: ...` |
| m25 family mismatch message | fixed | `cli,probe: ...` |
| m26 lexicographic address order | fixed | same |

Not done in this branch:

- **Pinning GitHub Actions to commit SHAs (part of M9).** The audit session could not
  read the upstream action repositories to obtain and verify the SHAs, and an unverified
  SHA is worse than a tag. Dependabot now watches the actions; pin them in a follow-up
  with `pin-github-action` or by hand from the upstream release pages.
- **`govulncheck` result at baseline (m13).** `vuln.go.dev` was unreachable from the audit
  sandbox. The CI job runs it on every push; the first run on this branch is the
  authoritative result.

## 3. Behavioural changes

All changes are deliberate consequences of findings; the README lists them for users.

| Change | Finding | Before | After |
|---|---|---|---|
| Exit status | B2 | always 0 unless usage error or signal | 1 when any probed address never answered, or on runtime errors such as a failed lookup |
| Bare IPv6 literal | M1 | `::1 80` probed host `:` port `1`; `::1:80` meant `::1` port 80 | `::1 80` probes `::1`; `::1:80` is the address `::1:80` and needs a port (`[::1]:80`) |
| Zone identifiers | M2 | silently dropped | kept and used for the dial |
| Colour | M4 | always on unless `-nocolor`/`NO_COLOR` | on only when stdout is a terminal; `-nocolor` available on Windows too (output there stays uncoloured as before) |
| `-h` | M5 | flag list on stderr, exit 0 | full usage on stdout, exit 0 |
| Unknown flag | M5 | Go's default usage on stderr, exit 2 | error line plus usage on stderr, exit 2 |
| Unresolvable host | M6 | error plus full usage, exit 2 | one line, exit 1 |
| DNS lookup budget | M10 | `-t` | at least 5 s |
| `-t`, `-d` | m1 | any positive int | 1 to 86 400 000 ms |
| UDP payload | m2 | unlimited | at most 65 507 bytes |
| Port | m3 | `+80`, `0080` accepted and printed as given | `+80` rejected, `0080` printed as `80` |
| Extra positional arguments | m4 | ignored | error |
| `-payload` with TCP | m5 | ignored | error |
| `version` | m6 | matched anywhere in the arguments | `-version` flag, or `version` as the only argument |
| Preset names | m7 | case-sensitive | case-insensitive |
| Interrupted probe | M3 | counted as a failure after waiting for the timeout | aborted immediately, not counted |
| SIGTERM | m23 | exit 130 | exit 143 |
| Address order | m26 | lexicographic within a family | numeric within a family |
| Statistics table | m20 | — | no visible change; widths now follow the actual colour state |

Output format (banner, per-probe rows, statistics table) and every existing flag are
otherwise unchanged.

## 4. Recommendations beyond this audit

Not implemented; ordered by value for the effort.

1. **Pin actions to SHAs and adopt GoReleaser with signed, attested releases** (effort:
   1 day). GoReleaser produces archives, checksums, SBOMs, cosign keyless signatures and
   SLSA provenance from one config, replaces the hand-maintained release body and the
   Makefile matrix, and can publish Homebrew/Scoop manifests and deb/rpm via nfpm. The
   current pipeline uploads bare binaries with a mutable-tag third-party action.
2. **Machine-readable output** (effort: 1 day). A `-json` mode emitting one object per
   probe plus a summary object, and a `-q` quiet mode printing only the summary, are the
   features most often missing when such a tool is used from monitoring or CI. Exit codes
   now make scripting possible; JSON makes it robust.
3. **Richer statistics** (effort: half a day). Standard deviation / jitter (`mdev` as in
   `ping`), percentiles (p50/p95) and the elapsed wall time. The `Stats` struct and the
   table already isolate this.
4. **Probe addresses concurrently within a round** (effort: half a day). With `-4 -6` or
   multi-homed hosts a round currently takes up to `N × -t`; a fan-out with ordered
   printing keeps a round at one timeout and makes the `-d` interval meaningful.
5. **Validate UDP replies per preset** (effort: 1 day). Match the DNS transaction ID, the
   NTP mode and the STUN transaction ID so that an unrelated datagram cannot count as a
   reply, and report protocol-level errors (for example DNS `REFUSED`) distinctly.
6. **Flag layer with long names and `ping`-style aliases** (effort: 1 day). `--timeout`,
   `--interval`, `--count`, `-i`, `-W`, `-w <deadline>`, plus `-4 -6` meaning "both" when
   given together, with the current short flags kept as aliases. Consider a `-color`
   flag to force colour when piping into `less -R`.
7. **Windows colour through `go-colorable`** (effort: 2 hours). Writing through
   `color.Output` enables colour on Windows 10+ consoles without the global disable.
8. **Internal architecture** (effort: 1 day). Introduce a `Prober` interface (TCP, UDP,
   later TLS/HTTP), split `Config` into parsed options and runtime state, drop the
   duplicated fields in `PingOptions`, and describe presets as data (name, protocol,
   port, payload, description, reply validator) so the help, README table and tests are
   generated from one source.
9. **More presets and probe types** (effort: 2 to 4 hours each). TLS handshake (with
   certificate expiry in the output), HTTP `HEAD`, QUIC initial, mDNS, SNMP `get`,
   source-address/interface binding (`-I`), and a randomised probe order for load
   spreading.
10. **Fuzz tests** (effort: 1 hour). `splitDestination`, `NormalizePort` and the payload
    decoder are ideal `testing.F` targets.
11. **Release process hygiene** (effort: 1 hour). Branch protection requiring the CI
    workflow, tag protection for `v*`, a `CHANGELOG.md`, and a release checklist that
    includes running the Windows CI job before tagging.
12. **Website** (effort: 2 hours). Move site sources into `docs/` so the repository root
    is not a Jekyll source tree, or replace Jekyll with a static `index.html`; either
    removes the theme dependency and the exclude list.

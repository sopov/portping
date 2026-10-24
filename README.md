# portping — TCP/UDP port connectivity checker

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![CI](https://github.com/sopov/portping/actions/workflows/ci.yml/badge.svg)](https://github.com/sopov/portping/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sopov/portping)](https://goreportcard.com/report/github.com/sopov/portping)

A CLI network diagnostic tool for testing port reachability. Performs TCP ping and UDP ping checks with IPv4/IPv6 support, protocol presets, and colored output.

---

## Features

- TCP ping and UDP ping checks  
- IPv4 / IPv6 selection (`-4`, `-6`)  
- Protocol presets (`dns`, `ntp`, `http`, `https`, `ssh`)  
- Custom UDP payloads (hex)  
- Continuous or fixed-count pings (`-c`)  
- Millisecond-accurate stats  
- Script-friendly exit codes (see [Exit codes](#exit-codes))  
- Colorized output on terminals only (`-nocolor` or `NO_COLOR` to disable)  
- Cross-platform binaries for Linux, macOS, Windows, *BSD  
- Single static binary (CGO disabled), no external dependencies at runtime

---

## Installation

### From GitHub Releases

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  armv7l|armv6l) ARCH=arm ;;
  i686|i386) ARCH=386 ;;
esac
BASE="https://github.com/sopov/portping/releases/latest/download"
curl -fsSL "$BASE/portping_${OS}_${ARCH}" -o portping
curl -fsSL "$BASE/SHA256SUMS.txt" -o SHA256SUMS.txt
grep " portping_${OS}_${ARCH}\$" SHA256SUMS.txt | sha256sum -c -
chmod +x portping
sudo mv portping /usr/local/bin/
```

Every release ships `SHA256SUMS.txt`; the `sha256sum -c` line fails if the
download does not match the published checksum.

### From Source

```bash
git clone https://github.com/sopov/portping.git
cd portping
make build
sudo mv dist/portping /usr/local/bin/
```

Or inside Docker:

```bash
make build-docker
sudo mv dist/portping /usr/local/bin/
```

---

## Usage

```bash
portping [options] <destination> <port> [UDP HEX PAYLOAD (UDP only)]
```

### Examples

```bash
# TCP ping to check port reachability
portping google.com 443

# UDP ping with DNS preset
portping -dns 8.8.8.8

# Custom UDP payload (hex)
portping -udp 1.1.1.1 53 0000010000000000000100000377777706676f6f676c6503636f6d0000010001

# IPv6 ping tool example
portping -http -6 google.com

# IPv6 literals: bare address plus port, or bracketed address:port
portping -6 2001:db8::1 443
portping -6 '[fe80::1%eth0]:22'

# Port checker with two attempts and 500ms timeout
portping -t 500 -c 2 example.com 22

# In scripts: exit status 0 only if the port answered
portping -c 1 -t 2000 db.internal 5432 && echo "database reachable"
```

Run `portping -h` for all flags.

### Destination syntax

| Form | Meaning |
|------|---------|
| `host port` | hostname or IP literal, port as second argument |
| `host:port` | hostname or IPv4 literal with port |
| `[v6]:port` | IPv6 literal with port (brackets required) |
| `v6 port` | bare IPv6 literal, with or without a zone (`fe80::1%eth0`) |
| `-preset name host` | port (and for UDP the payload) from the preset |

An unbracketed IPv6 literal is always an address: `::1:80` is the address
`::1:80`, not `::1` on port 80. The port can be given once; a port in `host:port`
plus a second positional argument is rejected.

### What "connected" means

- **TCP**: the three-way handshake completed. The connection is closed
  immediately afterwards, so the service sees one empty connection per probe.
- **UDP**: a datagram of any size came back within the timeout. UDP has no
  handshake, so the payload must provoke a reply (the presets send a DNS query,
  an NTP request and a STUN binding request). Three outcomes exist:
  - a reply: *connected*;
  - `connection refused` (or `WSAECONNRESET` on Windows): the host answered with
    ICMP port unreachable, the port is closed;
  - `i/o timeout`: no reply at all. The port may be open but silent for this
    payload, filtered by a firewall, or the host may be down. A timeout alone
    does not prove that a UDP port is closed.

---

## Options

| Flag | Description |
|------|-------------|
| `-tcp` / `-udp` | Protocol selection (default TCP) |
| `-preset <name>` | Use preset (see Presets table below) |
| `-dns`, `-ntp`, `-http`, `-https`, `-ssh`, etc. | Shortcut flags for presets |
| `-payload <hex>` | Custom UDP payload (hex string) |
| `-4` / `-6` | Force IPv4 / IPv6 |
| `-t <ms>` | Timeout per attempt (default: 1000) |
| `-d <ms>` | Delay between attempts (default: 1000) |
| `-c <n>` | Stop after `n` attempts (default: infinite) |
| `-nocolor` | Disable colored output (colour is already off when stdout is not a terminal, when `NO_COLOR` is set, and on Windows) |
| `-version` | Show version info |

`-t` and `-d` accept 1 to 86400000 ms. UDP payloads may be up to 65507 bytes
(the largest IPv4 datagram). Preset names are case-insensitive.

---

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | every probed address answered at least once |
| `1` | at least one probed address never answered, the host did not resolve, or another runtime error |
| `2` | invalid flags or arguments (usage is printed to stderr) |
| `130` / `143` | interrupted by SIGINT (Ctrl-C) / SIGTERM; the statistics table is still printed |

With `-c 1` this makes `portping` usable as a reachability test in shell
conditions and health checks. A probe that was in flight when the signal
arrived is not counted.

---

## Output Example

```bash
Ping of 8.8.8.8 on udp 53 (1 IP)
IPv4: 8.8.8.8
Payload (hex): 0000010000000000000100000377777706676f6f676c6503636f6d0000010001
  1    8.8.8.8     21.45ms
  2    8.8.8.8     20.88ms

Statistics of ping 8.8.8.8 on udp 53
  IP Address   Attempted   Connected   Failed   Minimum   Maximum   Average
  8.8.8.8             2           2        0     20.88ms   21.45ms   21.16ms
```

---

## Presets

| Name | Proto | Port | Description |
|------|-------|------|------------|
| `dns`   | UDP | 53   | DNS query A/IN |
| `ntp`   | UDP | 123  | Network Time Protocol |
| `stun`  | UDP | 3478 | STUN binding request |
| `ftp`   | TCP | 21   | FTP check |
| `http`  | TCP | 80   | HTTP check |
| `https` | TCP | 443  | HTTPS check |
| `ssh`   | TCP | 22   | SSH check |
| `smtp`  | TCP | 25   | SMTP check |
| `pop3`  | TCP | 110  | POP3 check |
| `imap`  | TCP | 143  | IMAP check |
| `mysql` | TCP | 3306 | MySQL check |
| `postgres` | TCP | 5432 | PostgreSQL check |

---

## Development

```bash
make deps
make lint
make vet
make test
make dist
```

For reproducible builds (CI-like):

```bash
make dist-docker
```

---

## Versioning

```bash
portping version
# portping v1.1.0 (dd70914, 2025-11-12T03:57:30Z)
```

---

## Changes from the 2026-10 audit

The behaviour below changed deliberately; the reasons are in
[AUDIT.md](AUDIT.md).

- Exit status is `1` when a probed address never answered (was always `0`).
- Bare IPv6 literals are no longer split at the last colon: `portping ::1 80`
  works, `portping ::1:80` now asks for a port (use `[::1]:80`).
- Colour is emitted only when stdout is a terminal; `-nocolor` exists on every
  platform (Windows output stays uncoloured).
- Unknown flags, a missing port, extra positional arguments, `-payload`
  without UDP, oversized payloads and out-of-range `-t`/`-d` are errors.
- An unresolvable host exits `1` with one line instead of printing the help.
- Ctrl-C during a UDP wait returns immediately; SIGTERM exits `143`.

---

## License

MIT License © Leonid Sopov

---


## Links

- Repo: [https://github.com/sopov/portping](https://github.com/sopov/portping)  
- Issues: [https://github.com/sopov/portping/issues](https://github.com/sopov/portping/issues)  
- Author: [www.sopov.org](https://www.sopov.org)

# portping — TCP/UDP port connectivity checker

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Build](https://github.com/sopov/portping/actions/workflows/release.yml/badge.svg)](https://github.com/sopov/portping/actions)
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
- Colorized output (`--nocolor` to disable)  
- Cross-platform binaries for Linux, macOS, Windows  
- Single static binary (CGO disabled)

---

## Installation

### From GitHub Releases

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
[ "$ARCH" = "x86_64" ] && ARCH="amd64"
[ "$ARCH" = "aarch64" ] && ARCH="arm64"
curl -L https://github.com/sopov/portping/releases/latest/download/portping_${OS}_${ARCH} -o portping
chmod +x portping
sudo mv portping /usr/local/bin/
```

### From Source

```bash
git clone https://github.com/sopov/portping.git
cd portping
make build
```

Or inside Docker:

```bash
make build-docker
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

# Port checker with two attempts and 500ms timeout
portping -t 500 -c 2 example.com 22
```

Run `portping -h` for all flags.

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
| `-nocolor` | Disable colored output |
| `-version` | Show version info |

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
# portping v1.1.0 (17b15bf, 2025-11-12T02:10:14Z)
```

---

## License

MIT License © Leonid Sopov

---


## Links

- Repo: [https://github.com/sopov/portping](https://github.com/sopov/portping)  
- Issues: [https://github.com/sopov/portping/issues](https://github.com/sopov/portping/issues)  
- Author: [www.sopov.org](https://www.sopov.org)

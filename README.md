# WharfEye

Inspect container metrics, audit security configurations, and get performance recommendations across Docker, Podman, and containerd.

## Features

- Live CPU, memory, and network metrics per container
- Security misconfiguration checks based on CIS Docker Benchmark and NIST SP 800-190
- Performance recommendations based on observed container metrics
- Interactive TUI dashboard and browser-based web dashboard
- JSON output and HTML/CSV report export
- Auto-detects Docker, Podman (rootful/rootless), and containerd

## Install

```bash
git clone https://github.com/mathesh-me/wharfeye.git
cd wharfeye
make build
```

## Usage

```bash
# Interactive TUI
wharfeye

# Web dashboard (http://localhost:9090)
wharfeye web

# Container status and metrics
wharfeye status
wharfeye status <container-name>

# Security scan
wharfeye security
wharfeye security <container-name>    # detailed hardening advice

# Performance recommendations
wharfeye recommend
wharfeye recommend <container-name>

# Full report
wharfeye report --export html
wharfeye report --export csv

# Show detected runtime
wharfeye detect
```

All commands support `--format json` for scripted use.

## Runtime Selection

```bash
wharfeye --runtime docker
wharfeye --runtime podman
wharfeye --runtime containerd
wharfeye --runtime podman --socket /run/podman/podman.sock
```

Default is `auto` - probes known socket paths and picks the first available runtime.

## TUI Keys

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Dashboard / Security / Advisor tabs |
| `Enter` | Container detail view |
| `Esc` | Back |
| `j` / `k` or arrows | Navigate |
| `/` | Filter containers |
| `s` | Run security scan |
| `q` / `Ctrl+C` | Quit |

## Configuration

```bash
wharfeye config init    # writes ~/.config/wharfeye/config.yaml
```

```yaml
runtime:
  type: auto    # auto | docker | podman | containerd
  socket: ""    # auto-detect if empty

web:
  port: 9090
  host: 0.0.0.0
```

## Development

```bash
make build
make test
make vet
make lint    # requires golangci-lint
```

## Reference

- [Security checks (SEC-001 to SEC-020)](docs/security-checks.md)
- [Advisor rules (PERF-001 to PERF-012)](docs/advisor-rules.md)

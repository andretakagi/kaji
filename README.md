# 舵 Kaji

[![CI](https://github.com/andretakagi/kaji/actions/workflows/ci.yml/badge.svg)](https://github.com/andretakagi/kaji/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/andretakagi/kaji)](LICENSE)
[![Release](https://img.shields.io/github/v/release/andretakagi/kaji)](https://github.com/andretakagi/kaji/releases/latest)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fandretakagi%2Fkaji-blue)](https://github.com/andretakagi/kaji/pkgs/container/kaji)

A lightweight web GUI for [Caddy](https://caddyserver.com). Point subdomains at ports. No YAML, no Caddyfile editing, no CLI.

Kaji is the **Nginx Proxy Manager of Caddy**: a simple, self-hosted dashboard for homelabbers who want automatic HTTPS reverse proxying without the config file gymnastics.

## Features

- **Route management** - Create, edit, delete, and enable/disable reverse proxy routes. Disabled routes are preserved and can be re-enabled without recreating them.
- **Per-route toggles** - Force HTTPS, gzip/zstd compression, security headers, CORS (with origin whitelist), TLS skip verify, basic auth, access logging, WebSocket passthrough, and load balancing with multiple upstreams (round robin, first, least connections, random, IP hash).
- **Route settings** - Global auto HTTPS mode and ACME email configuration, scoped alongside routes. Per-route Force HTTPS locks when the global setting is active.
- **Config snapshots** - Automatic snapshots before config changes with manual snapshot support. Restore any previous config state with one click.
- **Log viewer** - Filter by level, host, or status code. Switch between paginated history and real-time streaming. Configurable log sinks with domain tracking, levels, formats, outputs, and file rotation. Built-in access log that can be toggled on/off per route.
- **Prometheus metrics** - Per-host and global metrics toggles, configured alongside log settings.
- **Service control** - Start, stop, and restart Caddy from the dashboard. Status, route count, and upstream health all visible at a glance.
- **Auth and API keys** - Optional password auth with session cookies. Generate API keys for script and automation access.
- **Setup wizard** - First-run flow to set an admin password and Caddy admin URL.
- **Caddyfile import/export** - Import existing Caddyfile configs or export current config as a Caddyfile.
- **Single binary** - Go backend with the React frontend embedded at build time. No runtime dependencies. Fully offline, no external CDN or network calls.
- **Docker ready** - Multi-arch images for amd64 and arm64. Auto-detects Docker mode or set `CADDY_GUI_MODE=docker`.

## Quick Start

### Binary

Download a pre-built binary from the [latest release](https://github.com/andretakagi/kaji/releases/latest), or:

```bash
wget https://github.com/andretakagi/kaji/releases/latest/download/kaji-linux-amd64
chmod +x kaji-linux-amd64
./kaji-linux-amd64
```

Open `http://localhost:8080` and follow the first-run setup.

### Docker

Save this as `docker-compose.yml` and run `docker compose up -d`:

```yaml
services:
  kaji:
    image: ghcr.io/andretakagi/kaji:latest
    restart: unless-stopped
    ports:
      - "80:80"       # Caddy HTTP (your proxied sites)
      - "443:443"     # Caddy HTTPS (your proxied sites)
      - "443:443/udp" # Caddy HTTP/3
      - "8080:8080"   # Kaji dashboard
    volumes:
      - kaji_caddy_data:/data             # TLS certs and Caddy state
      - kaji_caddy_config:/etc/caddy       # Caddy configuration
      - kaji_gui_config:/etc/caddy-gui     # Kaji config and snapshots
      - kaji_caddy_logs:/var/log/caddy     # Access and error logs
    environment:
      - CADDY_GUI_MODE=docker

volumes:
  kaji_caddy_data:
  kaji_caddy_config:
  kaji_gui_config:
  kaji_caddy_logs:
```

Open `http://localhost:8080` and follow the first-run setup.

## Configuration

Most settings (auth, Caddy admin URL, log sinks) are configured through the dashboard itself. The setup wizard handles initial configuration on first run.

These environment variables control runtime behavior:

| Variable | Default | Description |
|----------|---------|-------------|
| `CADDY_GUI_MODE` | auto-detect | Set to `docker` to use the built-in process manager instead of systemd. Auto-detected when running in Docker. |
| `KAJI_LISTEN_ADDR` | `:8080` | Address and port for the Kaji dashboard (e.g. `:9090` or `127.0.0.1:8080`). |
| `KAJI_CONFIG_PATH` | `/etc/caddy-gui/config.json` | Path to the Kaji config file. Useful for non-standard installs or development. |

## Development

```bash
# Frontend dev server (terminal 1)
cd frontend && bun install && bun dev

# Go backend (terminal 2)
go run .
```

Frontend runs on `:5173` with API requests proxied to the Go backend on `:8080`.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go (stdlib `net/http`) |
| Frontend | React + TypeScript (Vite) |
| Styling | Vanilla CSS |
| Linting | Biome (frontend), gofmt + go vet (backend) |

## License

[MIT](LICENSE)

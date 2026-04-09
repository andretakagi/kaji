# 舵 Kaji

[![CI](https://github.com/andretakagi/kaji/actions/workflows/ci.yml/badge.svg)](https://github.com/andretakagi/kaji/actions/workflows/ci.yml)

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

```yaml
services:
  kaji:
    image: ghcr.io/andretakagi/kaji:latest
    restart: unless-stopped
    ports:
      - "8880:80"
      - "8443:443"
      - "8443:443/udp"
      - "8080:8080"
    volumes:
      - kaji_caddy_data:/data
      - kaji_caddy_config:/etc/caddy
      - kaji_gui_config:/etc/caddy-gui
      - kaji_caddy_logs:/var/log/caddy
    environment:
      - CADDY_GUI_MODE=docker

volumes:
  kaji_caddy_data:
  kaji_caddy_config:
  kaji_gui_config:
  kaji_caddy_logs:
```

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

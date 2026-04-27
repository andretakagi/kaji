# 舵 Kaji

[![CI](https://github.com/andretakagi/kaji/actions/workflows/ci.yml/badge.svg)](https://github.com/andretakagi/kaji/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/andretakagi/kaji)](LICENSE)
[![Release](https://img.shields.io/github/v/release/andretakagi/kaji)](https://github.com/andretakagi/kaji/releases/latest)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fandretakagi%2Fkaji-blue)](https://github.com/andretakagi/kaji/pkgs/container/kaji)

A lightweight web GUI for [Caddy](https://caddyserver.com). Point subdomains at ports. No YAML, no Caddyfile editing, no CLI.

Kaji gives you a clean dashboard for managing Caddy reverse proxy routes, TLS certificates, and logging - without editing config files. Single binary, works on bare metal or Docker.

## Features

- **Domain management** - Create, edit, delete, and enable/disable domains and subdomains with global auto HTTPS and ACME email configuration. Each domain or subdomain has a primary handler plus an optional list of path matchers for finer-grained routing.
- **Multiple handler types** - Reverse proxy (with multi-upstream load balancing), file server with optional directory browsing, redirects with configurable status and preserve-path, and static responses with custom headers and body.
- **Per-domain toggles** - Force HTTPS, gzip/zstd compression, security headers, CORS, custom request and response headers, TLS skip verify, basic auth, access logging, and WebSocket passthrough.
- **IP allow/block lists** - Named whitelist and blacklist definitions with composable child lists. Cascade logic rebuilds affected domains when lists change.
- **Backup and restore** - Export your entire Kaji setup (Caddy config, app settings, snapshots) as a ZIP file. Import backups on another instance or restore after a fresh install. The setup wizard can also import a backup during first-run configuration. Version-aware migrations handle config changes between releases.
- **Config snapshots** - Automatic snapshots before config changes with manual snapshot support. Restore any previous state with one click.
- **Logs and metrics** - Filter by level, host, or status code. Paginated history and real-time streaming with configurable log sinks.
- **Prometheus metrics** - Per-host request metrics with toggleable metric endpoints.
- **Loki push** - Forward access logs to a Loki instance with configurable endpoint, labels, flush interval, and tenant ID.
- **Caddyfile import/export** - Import existing Caddyfile configs or export current config as a Caddyfile.
- **Cloudflare DNS** - Built-in `caddy-dns/cloudflare` module for DNS-01 ACME challenges. Supports wildcard certs and domains where HTTP-01 isn't viable.
- **Auth and API keys** - Optional password auth with API key support for automation.
- **Single binary, Docker ready** - Go backend with embedded React frontend. Fully offline, no external CDN calls. Multi-arch Docker images for amd64 and arm64.

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
      - kaji_caddy_data:/data              # TLS certs and Caddy state
      - kaji_caddy_config:/etc/caddy       # Caddy configuration
      - kaji_gui_config:/etc/caddy-gui     # Kaji config and snapshots
      - kaji_caddy_logs:/var/log/caddy     # Access and error logs
    read_only: true
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    mem_limit: 512m
    tmpfs:
      - /tmp
    environment:
      - CADDY_GUI_MODE=docker
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/caddy/status"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

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
| `CADDY_LOG_DIR` | `/var/log/caddy/` | Directory where Caddy writes log files. The dashboard displays filenames relative to this path and validates that log sinks write within it. |

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

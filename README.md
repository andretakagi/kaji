# 舵 Kaji

A lightweight web GUI for [Caddy](https://caddyserver.com). Point subdomains at ports. No YAML, no Caddyfile editing, no CLI.

Kaji is the **Nginx Proxy Manager of Caddy**: a simple, self-hosted dashboard for homelabbers who want automatic HTTPS reverse proxying without the config file gymnastics.

## Features

- **Route management** - Create, edit, delete, and enable/disable reverse proxy routes. Disabled routes are preserved and can be re-enabled without recreating them.
- **Per-route toggles** - Force HTTPS, gzip/zstd compression, security headers, CORS (with origin whitelist), TLS skip verify, basic auth, access logging, WebSocket passthrough, and load balancing (round robin, first, least connections, random, IP hash).
- **Live log viewer** - Filter by level, host, or status code. Switch between paginated history and real-time streaming. Configure log sinks with custom levels, formats, outputs, and file rotation.
- **Service control** - Start, stop, and restart Caddy from the dashboard. Status, route count, and upstream health all visible at a glance.
- **Global settings** - Auto HTTPS mode, HTTP-to-HTTPS redirect, Prometheus metrics (with per-host option), debug logging, and ACME email for Let's Encrypt.
- **Auth and API keys** - Optional password auth with session cookies. Generate API keys for script and automation access.
- **Setup wizard** - First-run flow to set an admin password, Caddy admin URL, ACME email, and global settings.
- **Single binary** - Go backend with the React frontend embedded at build time. No runtime dependencies.
- **Docker ready** - Multi-arch images for amd64 and arm64. Auto-detects Docker mode or set `CADDY_GUI_MODE=docker`.

## Quick Start

### Binary

```bash
# Download the latest release for your platform
wget https://github.com/andretakagi/kaji/releases/download/v1/kaji-linux-amd64
chmod +x kaji-linux-amd64
./kaji-linux-amd64
```

Open `http://localhost:8080` and follow the first-run setup.

### Docker

```yaml
services:
  kaji:
    image: andretakagi/kaji:latest
    restart: unless-stopped
    ports:
      - "8880:80"
      - "8443:443"
      - "8443:443/udp"
      - "8080:8080"
    volumes:
      - caddy_data:/data
      - caddy_config:/etc/caddy
      - gui_config:/etc/caddy-gui
      - caddy_logs:/var/log/caddy
    environment:
      - CADDY_GUI_MODE=docker

volumes:
  caddy_data:
  caddy_config:
  gui_config:
  caddy_logs:
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

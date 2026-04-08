# Changelog

All notable changes to Kaji are documented in this file.

## [1.1.0] - 2026-04-08

### Added
- Multiple upstream support for load balancing
- Access log domain tracking in log config UI
- Design spec for access log integration

### Changed
- Reworked auth settings UI with inline confirmation flow
- Auth toggle now requires confirmation before disabling
- Updated favicon to use kanji character
- Polished UI details across components

### Fixed
- Setup wizard navigation state on completion
- Release download URL to use correct v1.0.0 tag
- README inaccuracies for ports, setup wizard, and load balancing strategy

### Dependencies
- Updated Vite from 8.0.3 to 8.0.5

## [1.0.0] - 2026-03-15

Initial release. Lightweight web GUI for managing Caddy reverse proxy.

- Route management with add, delete, enable/disable
- Global Caddy settings (auto HTTPS, HTTP-to-HTTPS redirects, debug mode, metrics)
- ACME email configuration
- Optional authentication with bcrypt password hashing and session cookies
- API key support for programmatic access
- Log viewer with filtering
- Setup wizard for first-time configuration
- Caddyfile import and export
- Docker and bare metal (systemd) deployment
- Fully offline/air-gapped -- all assets bundled

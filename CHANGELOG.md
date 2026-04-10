# Changelog

All notable changes to Kaji are documented in this file.

## [1.3.0] - 2026-04-10

### Added
- IP whitelist/blacklist management with named lists, composable child lists, and recursive resolution. Per-route IP filtering toggle with cascade logic that rebuilds affected routes when lists change.
- Cloudflare DNS module (`caddy-dns/cloudflare`) for DNS-01 ACME challenges and wildcard certs. Toggle and API token configuration in Route Settings.
- Custom Caddy binaries with cloudflare module included in GitHub releases
- Light/dark mode toggle in Settings with localStorage persistence. Light theme uses an inverted purple/violet palette that preserves brand identity. Inline script prevents flash of wrong theme on load.
- Security policy with GitHub private vulnerability reporting

### Changed
- Upgraded Caddy from 2.9.1 to 2.11.2
- Docker image now includes the cloudflare DNS module (built via xcaddy)
- Multi-platform Docker images (linux/amd64 and linux/arm64)
- Upgraded base image to Alpine 3.23

### Fixed
- Log viewer could show partial lines when a log entry straddled a read boundary

## [1.2.3] - 2026-04-09

### Added
- GitHub Actions CI workflow for lint and build checks on push/PR
- Docker image build and push workflow for GHCR on version tags
- Release workflow for cross-compiled binaries on version tags

### Changed
- Split route, auth, settings, and log handlers into separate files for easier navigation

## [1.2.0] - 2026-04-09

### Added
- Config snapshots with auto-snapshot on changes
- Flexible named log sinks (replaces hardcoded kaji_access)
- Prometheus metrics toggles moved to logs page with descriptions

### Changed
- Simplified snapshots UI from branching timeline to flat card list
- Reworked snapshot store to linear history model
- Made kaji_access log permanent and toggleable instead of deleteable
- Consolidated HTTPS and ACME settings into route settings section
- Moved default logger from global toggles to log config UI
- Removed standalone log config creation from logs page
- Removed settings step from first-time setup wizard
- Snapshot settings Save button only appears when values changed
- Improved accessibility, input validation, overflow handling, and mobile layout

### Fixed
- Deleting disabled routes no longer fails silently
- Access log no longer shows disabled routes
- Confirm dialog no longer hidden when route card is collapsed
- Access log timestamps now use local time

## [1.1.0] - 2026-04-08

### Added
- Multiple upstream support for load balancing
- Access log domain tracking in log config UI

### Changed
- Reworked auth settings UI with inline confirmation flow
- Auth toggle now requires confirmation before disabling
- Updated favicon to use kanji character

### Fixed
- Setup wizard navigation state on completion
- Release download URL to use correct v1.0.0 tag
- README inaccuracies for ports, setup wizard, and load balancing strategy

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

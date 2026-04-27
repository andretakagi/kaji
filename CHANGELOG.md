# Changelog

All notable changes to Kaji are documented in this file.

## [1.7.0] - 2026-04-27

### Added
- New handler types alongside reverse proxy:
  - File server for serving static directories, with optional directory browsing.
  - Redirect with configurable status code, target URL, and preserve-path option.
  - Static response for returning fixed responses with custom status, headers, and body.
- Subdomains as first-class entities. Each subdomain has its own primary handler, paths, toggles, and enable state, and can be configured independently from its parent domain.
- Custom request and response headers per route, in basic and advanced modes with per-header overrides.
- Multi-step domain creation wizard with toggles, root rule, optional subdomains, per-target paths, and a review step before applying.
- Domain rule toggle. The Domain card now has its own enable switch alongside the domain wrapper toggle, so the root rule can be turned off without affecting subdomains or paths.
- Parent-lock cascade. When a domain is disabled, its subdomain and path toggles lock so they can't be flipped while the parent is off, making the on/off state of every route obvious at a glance.

### Changed
- "Routes" renamed to "Domains" throughout the UI, terminology, and config schema.
- Domain routing now follows a clearer model. Each domain and subdomain has a single primary handler, and additional path matchers live alongside it as their own list. Existing configs migrate automatically on upgrade.
- Failed Caddy mutations now roll back the in-memory app config instead of leaving it out of sync with Caddy.

### Fixed
- Path form override toggles did not refresh when the target domain changed.
- Subdomain sync emitted duplicate path groups in the domain card view.
- Domain creation could fail with a duplicate root rule in some cases.

## [1.6.0] - 2026-04-15

### Added
- Full backup and restore - export entire system state as a ZIP and import on another instance, with version-aware migration and automatic rollback on failure
- Snapshots now capture full system state (Caddy config, app settings, Kaji version) instead of just Caddy JSON. Legacy snapshots are detected and still work.
- Import review step in setup wizard showing routes and settings before applying
- Caddyfile import during setup pre-fills ACME email and global toggles from the backup

### Changed
- Certificate status simplified to three tiers: expired, expiring, valid

### Fixed
- Settings page could enter an infinite render loop
- Global body size limit blocked backup imports over 1MB
- Caddyfile import could leave the admin URL out of sync

## [1.5.0] - 2026-04-14

### Added
- Loki log push - forward access logs to a Loki instance with configurable endpoint, labels, flush interval, and tenant ID
- Unified log viewer combining history and real-time streaming into a single view
- Log rotation detection so tailing recovers when Caddy rotates log files
- Contributing guide for early testers

### Fixed
- Loki endpoint URL auto-appends `/loki/api/v1/push` if omitted

## [1.4.0] - 2026-04-13

### Added
- Certificate deletion now shows which routes will be affected before confirming
- Log directory is configurable via `CADDY_LOG_DIR` environment variable

### Changed
- Setup wizard consolidated from 6 steps to 3

### Fixed
- Access log errors are now surfaced as warnings instead of being silently dropped
- Setup wizard no longer proceeds before Caddy is reachable
- DNS challenge setup errors were silently swallowed, making failures unrecoverable

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

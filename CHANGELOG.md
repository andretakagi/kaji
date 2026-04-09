# Changelog

All notable changes to Kaji are documented in this file.

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
- Updated README to reflect current feature set
- Polished frontend accessibility, input validation, overflow handling, and mobile layout

### Fixed
- Deleting disabled routes now uses correct ConfigStore method
- Access log no longer shows disabled routes
- Confirm dialog no longer hidden when route card is collapsed
- Access log timestamps now use local time
- Replaced deprecated React.FormEvent with React.SubmitEvent

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

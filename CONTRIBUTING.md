# Contributing to Kaji

Kaji is a young project and there's a lot of ground to cover. The most valuable thing you can do right now is **use it and report what you find**.

## What we need help with

### Testing

Kaji hasn't been tested across a wide range of setups yet. If you run Caddy, we'd love for you to try Kaji and tell us what breaks or feels incomplete.

Things to pay attention to:

- **Routes** - Does creating, editing, enabling/disabling, and deleting routes work as expected? Do the per-route toggles (compression, security headers, CORS, basic auth, etc.) actually apply correctly in Caddy?
- **IP allow/block lists** - Do list definitions and cascade logic behave correctly? Do changes propagate to affected routes?
- **Config snapshots** - Can you snapshot, restore, and get back to a known-good state reliably?
- **Caddyfile import/export** - Does importing your existing Caddyfile produce the right routes? Does exporting match what you'd expect?
- **Logs and metrics** - Are log filters, pagination, and real-time streaming working? Do per-host metrics show up correctly?
- **Loki push** - If you run Loki, does enabling log forwarding work? Do labels, flush interval, and tenant ID behave correctly? Does the connection indicator reflect actual status?
- **Docker vs bare metal** - We support both, but edge cases likely exist in each mode. Note which you're running when reporting issues.
- **Mobile** - The UI is meant to work on phones. If something is broken or awkward on a small screen, that's a bug.

### Reporting issues

Open a [GitHub issue](https://github.com/andretakagi/kaji/issues) with:

1. What you did
2. What you expected
3. What happened instead
4. Your setup (Docker or binary, OS, architecture, Caddy version if known)

Even vague reports like "the snapshot restore didn't seem to work" are useful at this stage. We'll ask follow-up questions.

### Feature gaps

If you hit something Kaji should handle but doesn't, open an issue and describe the use case. We're keeping scope intentionally small, but real-world needs inform what's worth adding.

## Development

See the [README](README.md#development) for dev setup instructions.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

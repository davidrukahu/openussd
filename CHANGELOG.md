# Changelog

All notable changes to OpenUSSD are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [semantic versioning](https://semver.org/). Interfaces will break
before 1.0.

## [0.1.0] - 2026-09-20

First code. A USSD dialogue reaches a tenant application and a screen comes
back, with no telco account required.

### Added

- **Gateway** (`gateway/`). Canonical session events per
  [RFC-0001](docs/rfcs/0001-telco-adapter-interface.md), a telco adapter
  interface with a registry, session storage behind an interface with
  in-memory and Redis implementations, and tenant routing on
  `(shortcode, sub-prefix)` with HMAC-signed outbound webhooks.
- **Africa's Talking adapter**, with fixture-driven contract tests. First
  rather than Safaricom Daraja, which is the M-Pesa API portal and exposes
  no USSD sandbox. See [docs/telco-access.md](docs/telco-access.md).
- **`adapter.TrustedProxy`**, a source-address allowlist for networks that
  authenticate nothing. This resolves RFC-0001's open question as a wrapper
  rather than a flag, so the weakness is visible in the deployment wiring.
- **Go SDK** (`sdk/go`). Typed screens and state machine, render helpers
  that enforce the screen budget, an i18n bundle, and a handler that
  verifies the gateway's signature before decoding.
- **Fediverse reference adapter**, read-only Mastodon public timeline
  paginated into USSD screens, with alt text for attachments.
- **`cmd/ussdsim`**, a terminal handset, so the whole thing runs with no
  telco account, no sandbox registration and no inbound tunnel.
- **[docs/webhook-protocol.md](docs/webhook-protocol.md)**, the full wire
  contract, so a tenant can be written in any language.
- Docker Compose, Makefile, and CI running gofmt, vet, race tests and
  golangci-lint.

### Notes

- The screen budget is **182 GSM 03.38 septets, or 70 UCS-2 units** once a
  screen contains any non-GSM character. Everything measures cost in the
  encoding the text forces, rather than counting runes against 182.
- Session state is opaque JSON, resolving
  [#10](https://github.com/davidrukahu/openussd/issues/10).
- The Africa's Talking fixtures are hand-written from the published request
  shape. They prove the adapter is self-consistent, not that it matches the
  network. Capturing them against the live sandbox is what
  [#7](https://github.com/davidrukahu/openussd/issues/7) should become.

### Known limitations

- No replay nonce. The signature's 5-minute window bounds replay but does
  not prevent it; tenants should deduplicate on `(session_id, turn)`.
- The simulator adapter ships enabled in the example config and
  authenticates nothing. It is the demo. Do not expose it.
- The Redis session store has no tests.
- No release pipeline. Build from source or use Docker Compose.

[0.1.0]: https://github.com/davidrukahu/openussd/releases/tag/v0.1.0

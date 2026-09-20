# OpenUSSD

> An open gateway and SDK for the feature-phone web.

OpenUSSD is an open-source gateway and software development kit that makes web services accessible from feature phones over USSD and SMS.

A working vertical slice is in this repository: you can dial a shortcode from your terminal and read a Mastodon timeline, with no telco account.

## Why

Roughly 1.1 billion people worldwide still use feature phones as their primary or only connected device, concentrated in Sub-Saharan Africa and South Asia. For these users, USSD shortcodes are often the only interface to digital services - banking, government, health information, civic participation.

Today, every service that wants USSD reach rebuilds the integration from scratch against proprietary gateway APIs (Africa's Talking, Infobip, Twilio), country by country, telco by telco. There is no open protocol, no reusable SDK, and no bridge between USSD and the federated web. The previous open-source attempt - Praekelt's Vumi - has been unmaintained since 2020.

OpenUSSD treats feature-phone users as a legitimate audience for the open web rather than a legacy to be replaced.

## What

Three components, developed in this monorepo:

1. **Gateway** - a self-hostable service that speaks USSD and SMS protocols and exposes a clean HTTP webhook interface to application developers. Multi-tenant. Telco-adapter abstraction over Safaricom, MTN, Airtel, and others.
2. **SDK** - libraries (Go, TypeScript) for building USSD applications as state machines, with typed sessions, multi-language support, and built-in handling of the per-screen character limit - 182 in the GSM alphabet, but only 70 the moment a screen contains an emoji or a non-Latin script, which content from the federated web constantly does.
3. **Fediverse adapter** - a reference adapter that exposes ActivityPub-compatible Fediverse content (Mastodon timelines, PeerTube titles, PixelFed feeds) through USSD menus, demonstrating how the federated web can reach feature phones.

See [`docs/architecture.md`](docs/architecture.md) for the high-level design, [`docs/webhook-protocol.md`](docs/webhook-protocol.md) for the wire contract a tenant in any language implements, and [`docs/rfcs/`](docs/rfcs/) for in-progress design notes.

## Try it

```bash
export FEDIVERSE_WEBHOOK_SECRET=$(openssl rand -hex 32)
docker compose up --build
```

Then, in another terminal:

```bash
go run ./cmd/ussdsim -shortcode '*384*1234#'
```

```
+----------------------------------+
| Latest posts                     |
| 1. anemoi (now)                  |
| 2. Berlin Cycling Diary (now)    |
| 3. Teletekst (1m)                |
| 0. Quit                          |
+----------------------------------+

Reply: 1
```

No telco account, no sandbox registration, no inbound tunnel: `cmd/ussdsim`
is a terminal handset that speaks to the gateway through a local adapter,
and it flags any screen a real network would refuse, so a screen that would
be unreadable on a handset is unreadable here too.

To dial from a real handset instead, enable the Africa's Talking adapter
and point a sandbox USSD channel at `/ussd/africastalking` - see
[`docs/telco-access.md`](docs/telco-access.md).

## Status

**v0.1 spike - working, not production.** The gateway, the Go SDK, and a
read-only Fediverse adapter run end to end. What that means precisely:

| Working | Not yet |
|---|---|
| Canonical session events, adapter interface ([RFC-0001](docs/rfcs/0001-telco-adapter-interface.md)) | Captured fixtures from a live sandbox ([#7](https://github.com/davidrukahu/openussd/issues/7)) |
| Africa's Talking adapter, fixture-driven contract tests | A second real network (MTN) |
| Session store: in-memory and Redis, 180s idle expiry | Postgres audit store |
| Tenant routing with HMAC-signed webhooks | TypeScript SDK |
| Go SDK: typed screens, state, i18n, encoding-aware screen budget | Fediverse write paths, identity binding ([#9](https://github.com/davidrukahu/openussd/issues/9)) |
| Mastodon public timeline over USSD, paginated | Independent security audit |

Interfaces will break before v1.0. Follow the
[milestones](https://github.com/davidrukahu/openussd/milestones) for what
lands next.

## Roadmap

Indicative 12-month plan from project kickoff:

| Milestone | Target | Scope |
|---|---|---|
| M1 - Gateway core | Month 1-2 | USSD/SMS protocol handling, session state, Africa's Talking adapter (see [note](docs/telco-access.md) on Safaricom) |
| M2 - Go SDK + Fediverse prototype | Month 3-4 | State-machine primitives, Mastodon read-only adapter |
| M3 - TypeScript SDK + second telco | Month 5-6 | TS SDK, MTN sandbox adapter, developer documentation v1 |
| M4 - Full Fediverse adapter | Month 7-8 | Mastodon read/write, PeerTube, PixelFed; security hardening |
| M5 - Audit + pilots | Month 9-10 | Independent security audit, community pilot deployments, doc translations |
| M6 - v1.0 | Month 11-12 | Release, conference talks, governance handover |

## Repository layout

```
.
├── canonical/            # Wire-neutral session event and response types
├── webhook/              # Gateway ↔ tenant signing and envelope
├── gateway/
│   ├── cmd/gateway/      # The gateway binary
│   ├── internal/adapter/ # Telco adapters: africastalking, simulator
│   ├── internal/session/ # Session store: memory, redis
│   ├── internal/tenant/  # Routing and signed webhook delivery
│   └── testdata/         # Contract-test fixtures, one directory per MNO
├── sdk/go/               # Go SDK: screens, state, render budget, i18n
├── cmd/ussdsim/          # Terminal handset for local development
├── adapters/fediverse/   # ActivityPub → USSD reference adapter
└── docs/                 # Architecture, RFCs, telco access notes
```

Still planned: `sdk/typescript/`, further telco adapters, `examples/`.

## Installing

Docker Compose is the supported path today, and building from source needs
only a Go toolchain:

```bash
go install github.com/davidrukahu/openussd/gateway/cmd/gateway@latest
```

There is no release pipeline yet, so no prebuilt binaries. It is tracked in
[TODOS.md](TODOS.md).

## License

The gateway and Fediverse adapter are licensed under [AGPL-3.0-or-later](LICENSE). The SDK packages will be re-licensed under Apache-2.0 if/when split into their own repositories so they can be embedded in applications without copyleft propagation; for as long as everything lives in this monorepo the whole tree is AGPL-3.0. Documentation is CC BY-SA 4.0.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code of Conduct](CODE_OF_CONDUCT.md).

```bash
make test     # race detector included
make check    # formatting, vet, tests - what CI runs
```

Design feedback still shapes the protocol: RFC-0001 is implemented but not
accepted, and the questions in [`docs/rfcs/`](docs/rfcs/) are open. If you
have integrated against an African MNO, [#11](https://github.com/davidrukahu/openussd/issues/11)
wants to hear about the wire format that surprised you.

## Funding

OpenUSSD is seeking grant funding from open-source and public-interest funders. The project is registered against [FLOSS/fund](https://floss.fund/) via [`funding.json`](funding.json) so a single manifest can serve multiple grant opportunities.

## Maintainer

[David W](https://github.com/davidrukahu) - Nairobi, Kenya. Author of three plugins on the WordPress.org directory ([profiles.wordpress.org/davidrukahu](https://profiles.wordpress.org/davidrukahu/)), and an open-source day job.

Reach out via GitHub issues for project topics, or via the email listed in [`funding.json`](funding.json) for grant/funding correspondence.

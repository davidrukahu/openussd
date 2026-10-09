# OpenUSSD

> An open gateway and SDK for the feature-phone web.

[![CI](https://github.com/davidrukahu/openussd/actions/workflows/ci.yml/badge.svg)](https://github.com/davidrukahu/openussd/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/davidrukahu/openussd/sdk/go.svg)](https://pkg.go.dev/github.com/davidrukahu/openussd/sdk/go)
[![Release](https://img.shields.io/github/v/tag/davidrukahu/openussd?label=release)](https://github.com/davidrukahu/openussd/releases)
[![Status](https://img.shields.io/badge/status-v0.1%20spike-orange)](#status)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)

[openussd.org](https://openussd.org)

OpenUSSD is an open-source gateway and software development kit for developers and public-interest organisations that run USSD services, the `*123#` menus that work on every phone, and want to own them. It runs on your own server, so the service and its data stay yours, and it works across providers instead of tying you to one. SMS and direct network connections over SMPP are planned.

A working vertical slice is in this repository: you can dial a shortcode from your terminal and read a Mastodon timeline, with no telco account.

## Why

In Sub-Saharan Africa, 60% of mobile internet subscribers still use a feature phone or a 3G smartphone ([GSMA, 2025](https://www.gsma.com/somic/wp-content/uploads/2025/09/The-State-of-Mobile-Internet-Connectivity-2025-Overview-Report.pdf)), and about half of all mobile connections are not smartphones ([GSMA, 2024](https://event-assets.gsma.com/pdf/GSMA_ME_SSA_2024_Web.pdf)). For many of these users, especially on feature phones, USSD shortcodes are the only interface to digital services - banking, government, health information, civic participation.

Today, every service that wants USSD reach rebuilds the integration against proprietary gateway APIs (Africa's Talking, Infobip and others), country by country, telco by telco. Good open-source work exists: application frameworks such as [laravel-ussd](https://github.com/spesohq/laravel-ussd), which renders responses for several aggregators, and Praekelt's [vumi2](https://github.com/praekeltfoundation/vumi2), the actively developed successor to [Vumi](https://github.com/praekeltfoundation/vumi) (archived after its last commit in 2020), with USSD and SMPP transports. OpenUSSD's focus is different: a small gateway one person can self-host, a canonical session model with a signed webhook any language can implement, SDKs that measure every screen in the encoding the network will use, and a bridge between USSD and the federated web.

OpenUSSD treats feature-phone users as a legitimate audience for the open web rather than a legacy to be replaced.

## Who it is for

- Developers and agencies that build USSD services for several clients, countries or providers.
- Public-interest organisations, such as health, education, farming and local government teams, that want to run their own service and keep their data.
- Teams that want to connect to a network directly over SMPP instead of through a provider (planned).

If you need one simple menu in one country, a provider such as Africa's Talking on its own may be enough.

## What

Three components, developed in this monorepo:

1. **Gateway** - a self-hostable service that receives USSD sessions and exposes a clean HTTP webhook interface to application developers. Multi-tenant. One telco-adapter interface over networks and aggregators: Africa's Talking today, MTN and SMPP planned. SMS is planned.
2. **SDK** - libraries (Go today, TypeScript planned) for building USSD applications as state machines, with typed sessions, multi-language support, and built-in handling of the per-screen character limit - 182 in the GSM alphabet, but only 70 the moment a screen contains an emoji or a non-Latin script, which content from the federated web constantly does.
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
| Canonical session events, adapter interface ([RFC-0001](docs/rfcs/0001-telco-adapter-interface.md)) | SMS |
| Africa's Talking adapter, contract tests including fixtures captured from the live sandbox | A second real network (MTN) |
| Session store: in-memory and Redis, 180s idle expiry | Postgres audit store |
| Tenant routing with HMAC-signed webhooks | TypeScript SDK |
| Go SDK: typed screens, state, i18n, encoding-aware screen budget | Fediverse write paths, identity binding ([#9](https://github.com/davidrukahu/openussd/issues/9)) |
| Mastodon public timeline over USSD, paginated | Independent security audit |

Interfaces will break before v1.0. See [`ROADMAP.md`](ROADMAP.md) for what
lands next.

## Roadmap

[`ROADMAP.md`](ROADMAP.md) has the full plan: what a grant would pay for,
task by task, with hours, costs and how each task can be checked. In short:

| Work package | Hours |
|---|---|
| 1. A gateway you can run in production | 230 |
| 2. More networks: SMPP, MTN, a second aggregator | 290 |
| 3. SMS | 130 |
| 4. TypeScript SDK and developer guides | 220 |
| 5. The Fediverse on any phone: posting and account linking | 160 |
| 6. Security audit, v1.0 release and two pilots in Kenya | 220 |
| **Total** | **1,250 hours, €50,000** |

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

A read-only mirror lives on Codeberg at
[codeberg.org/davidrukahu/openussd](https://codeberg.org/davidrukahu/openussd).

## How this project uses AI tools

I designed OpenUSSD's architecture, the canonical session model, the webhook
protocol and the RFCs, and I direct and review every change. Much of the
v0.1 Go implementation was written with Claude Code (Anthropic) under my
direction; I reviewed and tested each change before release. The project
website and some later fixes were also written with Claude Code.

Grant-funded code will be written by people. AI tools may help with review,
research and documentation, and any such use is disclosed.

## Funding

OpenUSSD is seeking grant funding from open-source and public-interest funders. [`ROADMAP.md`](ROADMAP.md) sets out what funding would pay for, task by task. The project is registered against [FLOSS/fund](https://floss.fund/) via [`funding.json`](funding.json) so a single manifest can serve multiple grant opportunities.

## Maintainer

[David W](https://github.com/davidrukahu) - Nairobi, Kenya. WordPress.org plugin author ([profiles.wordpress.org/davidrukahu](https://profiles.wordpress.org/davidrukahu/)), and an open-source day job.

Reach out via GitHub issues for project topics, or via the email listed in [`funding.json`](funding.json) for grant/funding correspondence.

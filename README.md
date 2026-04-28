# OpenUSSD

> An open gateway and SDK for the feature-phone web.

OpenUSSD is an open-source gateway and software development kit that makes web services accessible from feature phones over USSD and SMS. The project is in its planning phase; this repository hosts the design documents and will hold the implementation as it is built.

## Why

Roughly 1.1 billion people worldwide still use feature phones as their primary or only connected device, concentrated in Sub-Saharan Africa and South Asia. For these users, USSD shortcodes are often the only interface to digital services — banking, government, health information, civic participation.

Today, every service that wants USSD reach rebuilds the integration from scratch against proprietary gateway APIs (Africa's Talking, Infobip, Twilio), country by country, telco by telco. There is no open protocol, no reusable SDK, and no bridge between USSD and the federated web. The previous open-source attempt — Praekelt's Vumi — has been unmaintained since 2020.

OpenUSSD treats feature-phone users as a legitimate audience for the open web rather than a legacy to be replaced.

## What

Three components, developed in this monorepo:

1. **Gateway** — a self-hostable service that speaks USSD and SMS protocols and exposes a clean HTTP webhook interface to application developers. Multi-tenant. Telco-adapter abstraction over Safaricom, MTN, Airtel, and others.
2. **SDK** — libraries (Go, TypeScript) for building USSD applications as state machines, with typed sessions, multi-language support, and built-in handling of the 182-character-per-screen constraint.
3. **Fediverse adapter** — a reference adapter that exposes ActivityPub-compatible Fediverse content (Mastodon timelines, PeerTube titles, PixelFed feeds) through USSD menus, demonstrating how the federated web can reach feature phones.

See [`docs/architecture.md`](docs/architecture.md) for the high-level design and [`docs/rfcs/`](docs/rfcs/) for in-progress design notes.

## Status

**Planning / pre-implementation.** This repository currently holds the design documents, contribution guidance, and roadmap. Code lands once initial funding is in place.

Watch this repo or follow the [milestones](https://github.com/davidrukahu/openussd/milestones) to track progress.

## Roadmap

Indicative 12-month plan from project kickoff:

| Milestone | Target | Scope |
|---|---|---|
| M1 — Gateway core | Month 1–2 | USSD/SMS protocol handling, session state, Safaricom sandbox adapter |
| M2 — Go SDK + Fediverse prototype | Month 3–4 | State-machine primitives, Mastodon read-only adapter |
| M3 — TypeScript SDK + second telco | Month 5–6 | TS SDK, MTN sandbox adapter, developer documentation v1 |
| M4 — Full Fediverse adapter | Month 7–8 | Mastodon read/write, PeerTube, PixelFed; security hardening |
| M5 — Audit + pilots | Month 9–10 | Independent security audit, community pilot deployments, doc translations |
| M6 — v1.0 | Month 11–12 | Release, conference talks, governance handover |

## Repository layout (planned)

```
.
├── gateway/              # USSD/SMS gateway service (Go)
├── sdk/
│   ├── go/               # Go SDK
│   └── typescript/       # TypeScript SDK
├── adapters/
│   ├── fediverse/        # ActivityPub → USSD reference adapter
│   └── telco/            # Per-MNO adapters (Safaricom, MTN, Airtel, …)
├── examples/             # Sample USSD apps using the SDK
├── docs/                 # Architecture, protocol notes, tutorials
└── .github/              # Issue templates, workflows
```

## License

The gateway and Fediverse adapter are licensed under [AGPL-3.0-or-later](LICENSE). The SDK packages will be re-licensed under Apache-2.0 if/when split into their own repositories so they can be embedded in applications without copyleft propagation; for as long as everything lives in this monorepo the whole tree is AGPL-3.0. Documentation is CC BY-SA 4.0.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code of Conduct](CODE_OF_CONDUCT.md). Issues, ideas, and design feedback are welcome before any code lands — early input shapes the protocol.

## Funding

OpenUSSD is seeking grant funding from open-source and public-interest funders. The project is registered against [FLOSS/fund](https://floss.fund/) via [`funding.json`](funding.json) so a single manifest can serve multiple grant opportunities.

## Maintainer

[David W](https://github.com/davidrukahu) — Nairobi, Kenya. Software engineer at Automattic; WordPress.org plugin author ([profiles.wordpress.org/davidrukahu](https://profiles.wordpress.org/davidrukahu/)).

Reach out via GitHub issues for project topics, or via the email listed in [`funding.json`](funding.json) for grant/funding correspondence.

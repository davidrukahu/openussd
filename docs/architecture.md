# OpenUSSD architecture (sketch)

> Status: **draft**, partially implemented as of 2026-09. The gateway, the
> Go SDK, and a read-only Fediverse adapter exist and run end to end; see
> the repository README for what is and is not built. Expect breaking
> changes until v1.0.

This document describes how the three OpenUSSD components fit together: the **gateway**, the **SDK**, and the reference **Fediverse adapter**. It is intentionally short and decision-oriented; each subsystem will get a deeper RFC under [`rfcs/`](rfcs/) before code lands.

## High-level diagram

```
              +-------------------+        +---------------------+
              |   Feature phone   |        |  Smartphone /       |
              |   (USSD / SMS)    |        |  fediverse client   |
              +---------+---------+        +----------+----------+
                        |                             |
                        | USSD session (MNO)          | ActivityPub
                        v                             v
              +-------------------+        +---------------------+
              |       MNO         |        |  Mastodon /         |
              |  (Safaricom, MTN, |        |  PeerTube /         |
              |   Airtel, ...)    |        |  PixelFed instance  |
              +---------+---------+        +----------+----------+
                        |                             ^
                        | HTTP webhook (per-MNO fmt)  |
                        v                             |
            +------------------------+                |
            |   OpenUSSD Gateway     |                |
            |  ┌──────────────────┐  |                |
            |  │ Telco adapter    │  |  canonical     |
            |  │  layer (Saf/MTN) │--┼-- session ---->|
            |  └────────┬─────────┘  |   events       |
            |           v            |                |
            |  ┌──────────────────┐  |                |
            |  │ Session store    │  |                |
            |  │ (Redis/Postgres) │  |                |
            |  └────────┬─────────┘  |                |
            |           v            |                |
            |  ┌──────────────────┐  |                |
            |  │ Tenant router    │  |                |
            |  └────────┬─────────┘  |                |
            +-----------|------------+                |
                        |                             |
                        | HTTP webhook (canonical)    |
                        v                             |
            +------------------------+                |
            |  Application using     |                |
            |  the OpenUSSD SDK      |                |
            |  (Go or TypeScript)    |                |
            |                        |                |
            |  ┌──────────────────┐  |                |
            |  │ State machine    │  |                |
            |  │ Session types    │  |                |
            |  │ i18n / budget    │  |                |
            |  └──────────────────┘  |                |
            +-----------|------------+                |
                        |                             |
                        | (optional: Fediverse        |
                        |  reference adapter)         |
                        +-----------------------------+
```

## Components

### 1. Gateway

A self-hostable Go service. Responsibilities:

- **Telco adapter layer.** Per-MNO HTTP handlers (Safaricom Daraja, MTN, Airtel, …) that accept the MNO's native callback format and translate it into a single canonical session event the rest of the system understands. Each adapter is a small package implementing one interface; adding an MNO is one new file plus contract tests.
- **Session store.** Each USSD screen is an independent HTTP request; the gateway maintains continuity across screens. Sessions are addressed by `(mno, session_id)` and carry an opaque blob owned by the application. Default backing store is Redis for hot sessions plus Postgres for durable audit; both are pluggable. Session timeout default 180s of user inactivity.
- **Tenant router.** Most African shortcodes are shared. The gateway routes `(shortcode, sub-prefix)` to a tenant configuration and forwards the canonical event to that tenant's webhook URL. Per-tenant secrets sign outbound webhooks so applications can verify the request origin.
- **Outbound channels.** SMS and (later) USSD push for asynchronous notifications. Same telco-adapter abstraction as inbound.
- **Observability.** Structured logs, per-tenant metrics, sampled session traces. The audit log is the source of truth for "what did the user actually see?".

License: AGPL-3.0-or-later.

### 2. SDK

Two packages, one design. Released as `github.com/davidrukahu/openussd/sdk/go` and `@openussd/sdk` (npm) once split.

Core primitives:

- **`Session`** - typed value object exposing the user's MSISDN, language, tenant, and an application-defined state struct. The SDK persists state back to the gateway on each turn.
- **`State` / `Screen`** - a state machine. Each state declares the prompt to render, the input it accepts, and the transitions it allows. Inputs are validated before transitioning; invalid input re-renders the same screen with an error.
- **`Render`** - helpers for the per-screen budget: `Truncate`, `Paginate`, `Menu`, `MenuFit`, `Shrink`, with i18n bundles (Swahili, French, English at launch). Rendered output is measured at runtime as a guard.

  The budget is not a single number. GSM 03.38 packs 182 septets into a USSD string, but any character outside that alphabet - an emoji, a Chinese character, a curly quote - re-encodes the whole screen as UCS-2, where the limit is 70 units. The SDK measures cost in the encoding the text itself forces, and offers `ToGSM` to transliterate where the trade is worth making: a menu of fediverse display names is worth more than the emoji in them, while a post written in Chinese is not worth anything transliterated, so it simply paginates further.
- **`Auth`** - opt-in PIN and OTP flows. Documents the spoofing risks of trusting MNO-supplied MSISDNs and gives vetted defaults.

Out of scope for v1: visual flow builders, IVR, WhatsApp.

License: AGPL-3.0-or-later while in monorepo, Apache-2.0 if/when SDK packages are split out so they can embed in proprietary apps without copyleft propagation.

### 3. Fediverse adapter

A reference application (not a framework) built on the SDK. Demonstrates ActivityPub → USSD mapping decisions:

- **Read paths first.** Mastodon home / public timeline, PeerTube channel titles, PixelFed feeds, rendered into paginated USSD menus.
- **Write paths second.** Posting, replying, boosting, follow-from-USSD. PIN-gated.
- **Identity.** Each MSISDN binds to one Fediverse account via an enrolment flow (USSD-initiated, browser-completed). Spoof-resistant via a one-time link delivered to the bound account.
- **Character-budget strategy.** Long posts paginate with `Next` / `Prev` controls; image attachments surface as `[image: alt text]`; mentions and hashtags survive truncation.

This component is the research contribution as much as the engineering - the goal is to publish a clear protocol-mapping document alongside the code so other implementers can reuse the design.

License: AGPL-3.0-or-later.

## Cross-cutting concerns

### Security

- **Webhook signing** between gateway and tenant applications using shared secrets, rotated per tenant.
- **MNO claim trust boundary** explicitly documented. The gateway treats MNO-supplied MSISDNs as *claims* and surfaces them to applications as such; PIN/OTP overlays exist for any flow that needs higher assurance.
- **Independent security audit** scheduled in M5 of the year-1 plan.

### Multi-tenancy

- Tenants are isolated at the routing layer. Session payloads are tenant-scoped; the SDK cannot read another tenant's state even if deployed in the same process (different signing keys, different webhook URLs).
- The gateway never persists application-level PII beyond the session window; durable storage of conversation state is the application's concern.

### Telco coverage at v1

Year-1 targets:

- **Africa's Talking** (implemented) - an aggregator reaching Safaricom and
  Airtel in Kenya, MTN and Airtel in Uganda, and several other markets from
  one adapter. It is the only USSD sandbox obtainable without a commercial
  agreement, which is why it is first.
- **Safaricom direct** - requires a commercial shortcode agreement with a
  registered Kenyan entity, so it belongs in the funded phase. Note that
  Daraja, named in earlier drafts, is the M-Pesa API portal and exposes no
  USSD. See [`telco-access.md`](telco-access.md).
- **MTN USSD** (one of Uganda / Nigeria sandboxes - to be picked once sandbox access is confirmed)

Architectural placeholder for SS7-level signaling exists but is out of scope for v1.

## Open questions (tracked in RFCs)

1. ~~Canonical session-event schema~~ - implemented and validated against a
   first adapter; see [`rfcs/0001-telco-adapter-interface.md`](rfcs/0001-telco-adapter-interface.md).
   Still draft until a second real network lands.
2. ~~Session-state encoding (CBOR vs JSON)~~ - **JSON, opaque to the
   gateway**. Being able to read live state with `redis-cli` during an
   incident beats CBOR's ~30% saving until Redis pressure is measurable,
   and the codec seam remains for when it is ([#10](https://github.com/davidrukahu/openussd/issues/10)).
3. ActivityPub identity binding flow - how do we prove the USSD user owns the Fediverse account they claim? Still open; the shipped adapter is read-only precisely because this is unresolved ([#9](https://github.com/davidrukahu/openussd/issues/9)).
4. Whether PeerTube and PixelFed adapters are first-class in v1 or stretch goals.
5. **New:** how should a shared shortcode render its first screen? The
   router requires exactly one tenant per shortcode with an empty prefix to
   answer it. A gateway-owned selection menu is the alternative, and that
   is a product decision rather than a default.

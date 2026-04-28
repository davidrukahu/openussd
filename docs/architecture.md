# OpenUSSD architecture (sketch)

> Status: **draft**, pre-implementation. Expect breaking changes until v0.1.

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
            |  │ i18n / 182-char  │  |                |
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

- **`Session`** — typed value object exposing the user's MSISDN, language, tenant, and an application-defined state struct. The SDK persists state back to the gateway on each turn.
- **`State` / `Screen`** — a state machine. Each state declares the prompt to render, the input it accepts, and the transitions it allows. Inputs are validated before transitioning; invalid input re-renders the same screen with an error.
- **`Render`** — helpers for the 182-character budget: `Truncate`, `Paginate`, `Menu`, with i18n bundles (Swahili, French, English at launch). Rendered output is character-counted at compile time where possible and at runtime as a guard.
- **`Auth`** — opt-in PIN and OTP flows. Documents the spoofing risks of trusting MNO-supplied MSISDNs and gives vetted defaults.

Out of scope for v1: visual flow builders, IVR, WhatsApp.

License: AGPL-3.0-or-later while in monorepo, Apache-2.0 if/when SDK packages are split out so they can embed in proprietary apps without copyleft propagation.

### 3. Fediverse adapter

A reference application (not a framework) built on the SDK. Demonstrates ActivityPub → USSD mapping decisions:

- **Read paths first.** Mastodon home / public timeline, PeerTube channel titles, PixelFed feeds, rendered into paginated USSD menus.
- **Write paths second.** Posting, replying, boosting, follow-from-USSD. PIN-gated.
- **Identity.** Each MSISDN binds to one Fediverse account via an enrolment flow (USSD-initiated, browser-completed). Spoof-resistant via a one-time link delivered to the bound account.
- **Character-budget strategy.** Long posts paginate with `Next` / `Prev` controls; image attachments surface as `[image: alt text]`; mentions and hashtags survive truncation.

This component is the research contribution as much as the engineering — the goal is to publish a clear protocol-mapping document alongside the code so other implementers can reuse the design.

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

- **Safaricom Daraja USSD** (Kenya)
- **MTN USSD** (one of Uganda / Nigeria sandboxes — to be picked once sandbox access is confirmed)

Architectural placeholder for SS7-level signaling exists but is out of scope for v1.

## Open questions (tracked in RFCs)

1. Canonical session-event schema — see [`rfcs/0001-telco-adapter-interface.md`](rfcs/0001-telco-adapter-interface.md).
2. Session-state encoding (CBOR vs JSON; size implications for Redis).
3. ActivityPub identity binding flow — how do we prove the USSD user owns the Fediverse account they claim?
4. Whether PeerTube and PixelFed adapters are first-class in v1 or stretch goals.

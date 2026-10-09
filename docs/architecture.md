# OpenUSSD architecture (sketch)

> Status: **draft**, partially implemented as of 2026-09. The gateway, the
> Go SDK and a read-only Fediverse adapter exist and run end to end. The
> repository README says what is built and what is not. Expect breaking
> changes until v1.0.

This document describes how the three OpenUSSD components fit together:
the **gateway**, the **SDK** and the reference **Fediverse adapter**. It is
short on purpose and focuses on decisions. Each subsystem will get a deeper
RFC under [`rfcs/`](rfcs/) before its code lands.

## High-level diagram

```
              +-------------------+        +---------------------+
              |   Feature phone   |        |  Smartphone /       |
              |  (USSD; SMS later)|        |  fediverse client   |
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
            |  │  layer (AT, MTN) │--┼-- session ---->|
            |  └────────┬─────────┘  |   events       |
            |           v            |                |
            |  ┌──────────────────┐  |                |
            |  │ Session store    │  |                |
            |  │ (memory or Redis)│  |                |
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
            |  (Go today)            |                |
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

A self-hostable Go service. It has these responsibilities:

- **Telco adapter layer.** There is one handler per network or per
  aggregator. (A network is a mobile network operator, shown as MNO in the
  diagram.) Africa's Talking works today. MTN and SMPP (Short Message
  Peer-to-Peer) are planned. Each handler accepts the native callback
  format of its network or aggregator. It translates that format into one
  canonical session event that the rest of the system understands. Each
  adapter is a small package that implements one interface. Adding a
  network takes one new file plus contract tests.
- **Session store.** USSD (Unstructured Supplementary Service Data) is the
  text menu service a user reaches by dialling a code on a mobile phone.
  Each USSD screen is an independent HTTP request, so the gateway keeps the
  conversation continuous across screens. Sessions are addressed by
  `(mno, session_id)` and carry an opaque blob that the application owns.
  The default store keeps sessions in process memory. This is correct for
  a single replica, and the demo runs on it. Redis is the option for more
  than one replica, because otherwise two replicas would each hold half of
  every conversation. A durable Postgres audit store is designed for but
  not built. The default session timeout is 180s of user inactivity.
- **Tenant router.** A tenant is an application that receives webhooks
  from the gateway. Most African shortcodes are shared. The gateway routes
  `(shortcode, sub-prefix)` to a tenant configuration. It then forwards the
  canonical event to that tenant's webhook URL. Each tenant has its own
  secret, and the gateway signs outbound webhooks with it. This lets
  applications verify where a request came from.
- **Outbound channels (planned).** SMS and USSD push for asynchronous
  notifications. They use the same telco adapter abstraction as inbound
  traffic.
- **Observability.** The gateway writes structured logs today. Per-tenant
  metrics and sampled session traces are planned. Once the Postgres store
  exists, its audit log is the source of truth for "what did the user
  actually see?".

License: AGPL-3.0-or-later.

### 2. SDK

The Go SDK is in [`sdk/go`](../sdk/go) today, as the module
`github.com/davidrukahu/openussd/sdk/go`. A TypeScript SDK with the same
design is planned, to be published on npm as `@openussd/sdk`.

What the Go SDK gives you:

- **`App` and `Screen`** - an app is a set of named screens. Each screen
  has a `Prompt` that renders what the user sees, and a `Handle` that reads
  the user's reply. `Handle` returns an action: `Goto` another screen,
  `Stay` on this screen with a message (the path for invalid input, so the
  user sees what went wrong without losing their place), or `Finish` the
  session with a final message.
- **`Context`** - what a screen sees on each turn: the canonical event,
  including the phone number (MSISDN, the subscriber number as the network
  reports it), the turn number, the language, translations through `T`,
  and the app's own state. The state is a typed Go value. The SDK stores it
  in the gateway between turns.
- **`Handler`** - an `http.Handler` that checks the webhook signature and
  runs one turn of the app.
- **Screen budget helpers** - `Truncate`, `Paginate`, `Menu`, `MenuFit`,
  `Shrink`, `Fits` and `ToGSM`, with translation bundles (`Bundle`) for
  English, Swahili and French. The SDK also measures rendered output at
  runtime, as a guard.

  The budget is not a single number. GSM 03.38 is the standard 7-bit GSM
  alphabet, also called GSM-7, and it packs 182 septets (7-bit characters)
  into a USSD string. Any character outside that alphabet changes this. An
  emoji, a Chinese character or a curly quote re-encodes the whole screen
  as UCS-2, a 16-bit encoding where the limit is 70 units. The SDK measures
  cost in the encoding that the text itself forces. It offers `ToGSM` to
  transliterate text where the trade is worth making. A menu of fediverse
  display names is worth more than the emoji in them. A post written in
  Chinese is worth nothing once transliterated, so it simply paginates
  further.

PIN and OTP (one-time password) flows are planned, not built. Until then,
the SDK documents that the phone number is a claim by the network, and
advises a PIN or OTP before anything that matters.

Out of scope for v1: visual flow builders, IVR (interactive voice response),
WhatsApp.

License: AGPL-3.0-or-later while the SDK is in the monorepo. If and when
the SDK packages are split out, they move to Apache-2.0, so proprietary
apps can embed them without copyleft propagation.

### 3. Fediverse adapter

A reference application built on the SDK. It is not a framework.
ActivityPub is the protocol that Fediverse servers use to talk to each
other. This adapter shows the decisions we made when mapping ActivityPub to
USSD:

- **Read paths first.** Mastodon home and public timelines, PeerTube
  channel titles and PixelFed feeds, rendered as paginated USSD menus.
- **Write paths second.** Posting, replying, boosting and following from
  USSD. All of these need a PIN.
- **Identity.** Each MSISDN binds to one Fediverse account through an
  enrolment flow. The user starts the flow on USSD and completes it in a
  browser. A one-time link delivered to the bound account makes the flow
  resistant to spoofing.
- **Character-budget strategy.** Long posts paginate with `Next` / `Prev`
  controls. Image attachments appear as `[image: alt text]`. Mentions and
  hashtags survive truncation.

This component is the research contribution as much as the engineering
one. The goal is to publish a clear protocol-mapping document next to the
code, so other implementers can reuse the design.

License: AGPL-3.0-or-later.

## Cross-cutting concerns

### Security

- Webhooks between the gateway and tenant applications are signed with
  shared secrets. The secrets are rotated per tenant.
- We document the trust boundary for network claims explicitly. The
  gateway treats MSISDNs supplied by the network as *claims* and passes
  them to applications as claims. PIN or OTP checks are planned for any
  flow that needs higher assurance.
- An independent security audit is planned before v1.0. See
  [`ROADMAP.md`](../ROADMAP.md).

### Multi-tenancy

- Tenants are isolated at the routing layer. Each session payload belongs
  to one tenant. The SDK cannot read another tenant's state, even when
  deployed in the same process, because each tenant has a different
  signing key and a different webhook URL.
- The gateway never persists application-level PII (personally
  identifiable information) beyond the session window. Durable storage of
  conversation state is the application's job.

### Telco coverage at v1

Year-1 targets:

- **Africa's Talking** (implemented) - an aggregator. One adapter reaches
  Safaricom and Airtel in Kenya, MTN and Airtel in Uganda, and several
  other markets. It is the only USSD sandbox you can get without a
  commercial agreement, which is why it comes first.
- **Safaricom direct** - needs a commercial shortcode agreement with a
  registered Kenyan entity, so it belongs in the funded phase. Earlier
  drafts named Daraja, but Daraja is the M-Pesa API portal and exposes no
  USSD. See [`telco-access.md`](telco-access.md).
- **MTN USSD** - built against MTN's published USSD API, in one market,
  once access is confirmed.
- **SMPP** - the protocol many aggregators and operators use between
  themselves, so one adapter can reach several providers.

The architecture has a placeholder for SS7-level signaling (SS7 is the
signaling system inside operator networks), but it is out of scope for v1.

## Open questions (tracked in RFCs)

1. ~~Canonical session-event schema~~ - implemented and validated against a
   first adapter; see [`rfcs/0001-telco-adapter-interface.md`](rfcs/0001-telco-adapter-interface.md).
   It stays draft until a second real network lands.
2. ~~Session-state encoding (CBOR vs JSON)~~ - **JSON, opaque to the
   gateway**. During an incident, reading live state with `redis-cli` is
   worth more than CBOR's ~30% saving, until Redis pressure is measurable.
   The codec seam stays in place for when it is ([#10](https://github.com/davidrukahu/openussd/issues/10)).
3. ActivityPub identity binding flow: how do we prove that the USSD user
   owns the Fediverse account they claim? This is still open. The shipped
   adapter is read-only because this question is unresolved ([#9](https://github.com/davidrukahu/openussd/issues/9)).
4. Whether the PeerTube and PixelFed adapters are first-class in v1 or
   stretch goals.
5. **New:** how should a shared shortcode render its first screen? The
   router requires exactly one tenant per shortcode with an empty prefix to
   answer it. The alternative is a selection menu owned by the gateway.
   That is a product decision, not a default.

# Roadmap and funded plan

This page is for anyone deciding whether to fund OpenUSSD, and for anyone
who wants to know what comes next. It says what already works, what a
grant would pay for, task by task, and how each task can be checked.

Plan as of October 2026. It replaces the earlier 12-month plan that was in
the README until then: v0.1 shipped since, so the work it covers is no
longer in the plan.

## In short

- **Already done, without funding:** a working gateway, a Go SDK, an
  Africa's Talking adapter tested against their live sandbox, and a
  Fediverse adapter that reads Mastodon over USSD. See
  [Status](README.md#status) and the
  [v0.1.0 release](https://github.com/davidrukahu/openussd/releases/tag/v0.1.0).
- **What funding pays for:** 1,250 hours of new work at €40 an hour,
  €50,000 in total, in six work packages below. Every euro pays for
  development time. Nothing is spent on hosting, devices or travel.
- **What it delivers:** a v1.0 that an organisation can run in production,
  on more networks (SMPP, and MTN where access is granted), with SMS, a TypeScript SDK, Fediverse
  posting, a security audit with its findings fixed, and two real pilots
  in Kenya.
- **How long:** about seven months from the start of funding.

## Why it matters now

**Urgency.** In Sub-Saharan Africa, 60% of mobile internet subscribers
still use a feature phone or a 3G smartphone
([GSMA, 2025](https://www.gsma.com/somic/wp-content/uploads/2025/09/The-State-of-Mobile-Internet-Connectivity-2025-Overview-Report.pdf)).
For many of them a USSD code is the only way to reach a service. Today every
organisation that wants that reach rebuilds the same integration against a
provider's own API, and many small organisations cannot afford to. Open
source options exist, such as Praekelt's vumi2, a Python messaging stack,
but none offers a small self-hosted gateway with a signed webhook any
language can use and screens measured in the network's own encoding. A
working prototype of that now exists, so the time to make it usable is now.

**Relevance to the open internet.** OpenUSSD is free software (AGPL-3.0)
built on open, documented interfaces: a
[webhook protocol](docs/webhook-protocol.md) any language can implement,
and design decisions written up as [RFCs](docs/rfcs/). It lets a service
run its own gateway instead of depending on one provider, and it carries
the federated web, including European projects such as Mastodon and
PeerTube, to phones that cannot run an app.

**Value for money.**

- A working v0.1 already exists, built without funding, so the risk that
  the project never works is much lower.
- €40 an hour is a Nairobi rate, well below typical European rates.
- One adapter reaches several networks: Africa's Talking covers six
  countries, and SMPP is a protocol many operators and aggregators offer,
  some of them for USSD as well as SMS.
- An independent security audit is expected to be provided separately,
  at no cost to this budget ([#8](https://github.com/davidrukahu/openussd/issues/8)).
- Everything is reusable: any service, in any language, can use the
  gateway, the protocol and the SDKs.

## Who does the work

- **The maintainer** ([David W](https://github.com/davidrukahu), Nairobi):
  about 450 hours, part-time alongside a day job. Architecture, the
  hardest gateway work, reviewing every change, security fixes and the
  pilots.
- **Contracted developers in Nairobi:** about 800 hours. Adapters, the SMS
  channel, the TypeScript SDK, packaging and examples.

All code is public in this repository from the first commit, under
AGPL-3.0. Grant-funded code will be written by people; see
[how this project uses AI tools](README.md#how-this-project-uses-ai-tools).

## The plan, task by task

Hours are estimates. Each work package ends with something anyone can
check in this repository.

### 1. A gateway you can run in production (230 hours, €9,200)

| Task | What it delivers | Hours |
|---|---|---|
| 1.1 | One callback at a time per session, so a retried callback cannot reach the service twice | 40 |
| 1.2 | A duplicate-detection key built from what the network sends, documented in the webhook protocol | 30 |
| 1.3 | Webhook replay protection: a one-time value in every signed request | 25 |
| 1.4 | Session-end events from Africa's Talking, so a hung-up session is closed at once | 25 |
| 1.5 | A Postgres store with a record of each session, with retention limits for privacy | 50 |
| 1.6 | Metrics and tracing for operators, with an example dashboard | 30 |
| 1.7 | Rate limits and per-service quotas, and limits on memory use | 30 |

**How to check it:** the race and replay cases have tests in CI, and the
limitations on duplicates and replay recorded in [`TODOS.md`](TODOS.md) and
the [webhook protocol](docs/webhook-protocol.md#signature) are gone.

### 2. More networks (290 hours, €11,600)

| Task | What it delivers | Hours |
|---|---|---|
| 2.1 | A shared conformance kit, grown from the Africa's Talking fixture tests: cases every adapter must pass, such as session end, empty input and long input | 40 |
| 2.2 | An SMPP adapter for USSD, with a test SMSC in CI | 110 |
| 2.3 | An MTN adapter, in a market where MTN offers USSD access to developers | 70 |
| 2.4 | A second aggregator adapter, so no single provider is required | 50 |
| 2.5 | Recorded test traffic and setup notes for each new network | 20 |

**How to check it:** each adapter passes the conformance kit in CI, and
[`docs/telco-access.md`](docs/telco-access.md) explains how to connect each
one.

### 3. SMS (130 hours, €5,200)

| Task | What it delivers | Hours |
|---|---|---|
| 3.1 | A design note (RFC-0002) for SMS next to USSD sessions | 25 |
| 3.2 | SMS in and out through Africa's Talking, with delivery reports | 45 |
| 3.3 | SMS over SMPP, reusing task 2.2 | 35 |
| 3.4 | Hand-over in the SDKs: send anything too long for a USSD screen by SMS | 25 |

**How to check it:** an example app sends a long post by SMS when it does
not fit on a USSD screen.

### 4. TypeScript SDK and developer guides (220 hours, €8,800)

| Task | What it delivers | Hours |
|---|---|---|
| 4.1 | A TypeScript SDK that matches the Go SDK: screens, state, screen budget, translations, signature checks | 100 |
| 4.2 | Test cases shared by both SDKs, so they behave the same | 30 |
| 4.3 | Three example apps in Go and TypeScript: a community notice line, a savings group balance, a clinic reminder | 50 |
| 4.4 | Guides: getting started, SDK reference, running the gateway | 40 |

**How to check it:** the same example app runs in Go and in TypeScript
against the same tests, and the TypeScript SDK is published on npm.

### 5. The Fediverse on any phone (160 hours, €6,400)

| Task | What it delivers | Hours |
|---|---|---|
| 5.1 | Linking a phone to a Fediverse account (design in [#9](https://github.com/davidrukahu/openussd/issues/9)), with a PIN before any post | 60 |
| 5.2 | Posting, replying, boosting and following from a phone | 40 |
| 5.3 | Browsing PeerTube channels and Pixelfed feeds | 30 |
| 5.4 | Caching and safe fetching, so a busy shortcode does not overload an instance | 15 |
| 5.5 | A public note on how ActivityPub maps to USSD, reviewed by a Fediverse developer | 15 |

**How to check it:** a phone can post to Mastodon over USSD in the
simulator, and the mapping note is published.

### 6. Audit, release and pilots (220 hours, €8,800)

| Task | What it delivers | Hours |
|---|---|---|
| 6.1 | A threat model and fuzz tests for every adapter's input parsing | 40 |
| 6.2 | All serious findings from the independent audit fixed, and the report published | 50 |
| 6.3 | Signed release binaries, container images, a Helm chart and a Nix package | 45 |
| 6.4 | Two pilot deployments in Kenya: setup, support and a public write-up. Pilot partners pay their own shortcode and session fees | 50 |
| 6.5 | Two developer workshops in Nairobi | 20 |
| 6.6 | v1.0: a stability promise, and a plan for maintenance after the grant | 15 |

**How to check it:** v1.0 is tagged, the audit report and the pilot
write-ups are public.

### Totals

| Work package | Hours | Cost |
|---|---|---|
| 1. Production gateway | 230 | €9,200 |
| 2. More networks | 290 | €11,600 |
| 3. SMS | 130 | €5,200 |
| 4. TypeScript SDK and guides | 220 | €8,800 |
| 5. Fediverse | 160 | €6,400 |
| 6. Audit, release and pilots | 220 | €8,800 |
| **Total** | **1,250** | **€50,000** |

## Order of work

| Months | Work |
|---|---|
| 1 to 2 | Production gateway (1), conformance kit and SMPP adapter (2.1, 2.2) |
| 3 to 4 | MTN and second aggregator (2.3 to 2.5), SMS (3), TypeScript SDK (4.1, 4.2) |
| 5 to 6 | Examples and guides (4.3, 4.4), Fediverse (5), hardening before the audit (6.1), pilot partners chosen and set up (6.4) |
| 7 | Audit fixes, packaging, pilot support and write-ups, workshops and v1.0 (6.2 to 6.6) |

## Changes from the earlier plan

- **Finished work moved out.** Most of the original M1 (gateway core) and
  M2 (Go SDK and Fediverse prototype) shipped in v0.1 without funding.
  Those hours now pay for work that is not built yet.
- **Only development time is budgeted.** The audit is expected to be
  provided separately. Hosting, test devices and sandbox fees are paid by the maintainer.
- **Safaricom.** The earlier plan named Safaricom's Daraja sandbox for the
  first adapter. Daraja is M-Pesa only and has no USSD, so the first adapter
  is Africa's Talking, which reaches Safaricom and Airtel in Kenya. Direct
  Safaricom access needs a commercial agreement and is not in this plan.
  Details in [`docs/telco-access.md`](docs/telco-access.md).
- **Shorter, with a team.** The work is sized for about seven months with
  contracted developers instead of twelve months alone.

## Risks

| Risk | What we do about it |
|---|---|
| A network will not give access in time | SMPP works against a test SMSC in CI, and aggregators give real-network reach without a direct agreement |
| The plan slips | Most work packages stand alone, and the most useful (1, 2.1, 2.2, 4.1) start first |
| Contracted developers are not found in time | The maintainer starts on package 1, and packages 2 to 4 are scoped so they can be handed over |
| The audit report arrives late | Hardening (6.1) comes first, and late findings are fixed in a patch release after v1.0 |
| The maintainer becomes unavailable | Design is written down as it is decided, in RFCs and the guides from task 4.4, and v1.0 includes a maintenance plan |
| Pilots fall through | Several Kenyan organisations will be approached from month 5, and two are needed |

## After the grant

The project stays free software. Maintenance after v1.0 is described in
task 6.6, and [`funding.json`](funding.json) lists ways to support it.

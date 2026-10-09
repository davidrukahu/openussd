# RFC 0001 - Telco adapter interface

- **Status:** draft (implemented; see "Validated against a first implementation")
- **Author:** David W
- **Updated:** 2026-10-09

## Context

USSD (Unstructured Supplementary Service Data) is the menu system a user reaches by dialling a code such as `*123#` on any phone. Each mobile network operator (MNO), or network, exposes USSD through a slightly different HTTP callback shape:

- **Safaricom** - direct access needs a commercial shortcode agreement. Earlier drafts named Daraja, but Daraja is the M-Pesa portal and has no USSD (see [telco-access](../telco-access.md)).
- **MTN** - the shape varies by country. Nigeria's format differs from Uganda's, and both differ from Africa's Talking.
- **Airtel** - different again. Some markets expose only SOAP.
- **Aggregators (Africa's Talking, Infobip)** - yet another shape, on top of the network underneath. Africa's Talking posts a form with `sessionId`, `serviceCode`, `phoneNumber`, `networkCode` and `text`. The `text` field is the menu path joined by `*`.

If the gateway lets network-specific shapes leak into application code, every USSD app has to handle every network it deploys to as a special case. That defeats the point of the project.

## Decision (proposed)

The gateway defines **one canonical session event** that all telco adapters produce, and **one canonical response** that adapters consume. An adapter author writes a small package that implements two functions: parse-inbound and serialise-outbound.

```go
// Conceptual - final names TBD.

type Adapter interface {
    // Parse decodes an MNO-native HTTP request body into a canonical event.
    Parse(r *http.Request) (Event, error)

    // Render serialises a canonical Response into the MNO-native wire format.
    Render(w http.ResponseWriter, resp Response) error

    // Verify checks any MNO-supplied authenticity signal (IP allowlist,
    // shared secret, mTLS) and returns an error if the request is not
    // trustworthy.
    Verify(r *http.Request) error
}

type Event struct {
    MNO         string  // "safaricom", "mtn-ug", ...
    SessionID   string  // MNO-assigned session id, opaque
    MSISDN      string  // E.164, claimed by MNO (NOT authenticated)
    Shortcode   string  // *123#
    Path        []string // user input segments since session start
    Phase       Phase   // Begin | Continue | Cancel | Timeout
    ReceivedAt  time.Time
    Raw         []byte  // original body, retained for audit
}

type Response struct {
    Body     string   // must fit one screen; see note on encoding below
    EndSession bool   // true => USSD "END", false => "CON"
}
```

### Why this shape

- `Path` is a `[]string`, not a `*`-joined `text` field in the Africa's Talking style. Adapters split or rebuild the path as they need to. Applications never parse the raw string.
- `Phase` is an enum, not a boolean `isNew`. Networks and aggregators express the session lifecycle in different ways, and the enum gives them one form.
- `Raw` is kept so the audit log can reproduce exactly what the network sent. We need this to investigate incidents and to write contract tests.
- `Verify` is separate from `Parse`, so a request that fails the authenticity check does not allocate session state. Adapters with no verification (for example, unauthenticated dev sandboxes) return `nil`.

### Conformance

Each adapter ships with **fixture-driven contract tests**: a directory of `request.http` / `expected-event.json` pairs. Where possible they are captured from the provider's own sandbox, and the provenance of each one is recorded next to it. CI replays them on every change. The fixtures also document the network's wire format.

## Alternatives considered

1. **Pass network-native types through to applications.** Rejected. It removes the reason to use the SDK at all, and it makes application code network-specific.
2. **One giant union type.** Rejected. `Event` would fill up with optional fields that nobody uses. Adding a new network could also break existing adapters without anyone noticing.
3. **Generate adapters from an OpenAPI spec.** Considered. We should look at this again once we have three or four adapters and can see how much they really differ. It is too early now.

## Open questions

- Do we model **outbound USSD push** (dialogues that the server starts, asynchronously) in the same `Adapter` interface, or as a separate `Pusher`? Leaning separate.
- How does `Verify` express "this MNO does not authenticate; trust the IP allowlist instead"? Probably a wrapper adapter, not a flag.
- Should `Path` carry a timestamp for each segment, for debouncing analysis? Probably not in v1.

## Next steps

- ~~Land Safaricom adapter against this interface to validate it.~~
  Safaricom has no public USSD sandbox. Daraja is the M-Pesa portal, not a
  USSD one. We landed an **Africa's Talking** adapter instead; see
  [`docs/telco-access.md`](../telco-access.md).
- ~~Capture the Africa's Talking fixtures against the live sandbox.~~
  Captured 2026-10-09. The captures sit next to the hand-written fixtures
  and do not replace them, because the hand-written ones cover cases the
  simulator cannot produce.
- Land a second real network: MTN through a country sandbox, or Africa's
  Talking production. Then adjust the interface based on what did not fit.
- Promote the RFC to *accepted* once two adapters and one production tenant ship without interface changes for one release cycle.

## Validated against a first implementation (2026-09)

We implemented the interface above for two adapters, Africa's Talking and a
local simulator, and for a gateway that uses it end to end. This is what the
work changed.

**The interface held.** `Parse` / `Render` / `Verify` needed no new
methods. Running `Verify` before `Parse` helped straight away: the gateway
can reject a request it cannot attribute without allocating session state,
and the handler code reads in that same order.

**`Path []string` was the right choice.** Africa's Talking sends the
`*`-joined field that the RFC expected. The adapter splits it, so the
tenant router can match sub-prefixes by structure, and the SDK's screens
never see a separator. The RFC did not say one thing: an **empty segment
has meaning**. `1**3` is a user who pressed send on an empty prompt. So the
adapter keeps empty segments and does not filter them out, and a fixture
covers this case.

**`Phase` needed a `Terminal()` helper, not more cases.** The four cases
are right. But `Cancel` and `Timeout` share a property that the gateway
needs to branch on: no response will reach the handset. The Africa's
Talking dialogue callback never carries either phase. A separate Events URL
callback reports how a session ended, and the gateway does not handle it
yet (see [telco-access](../telco-access.md)). The session store expires an
abandoned dialogue without any signal, so today only the local simulator
produces these phases.

**`Raw` was worth keeping.** Because of it, capturing a fixture means
copying from the audit log, not capturing network packets. The gateway
strips `Raw` before it forwards the event to a tenant. Keeping it would
leak the wire details that the SDK exists to hide, and it would make every
webhook body larger.

### Open question resolved: networks that authenticate nothing

The RFC asked how `Verify` should express "this MNO does not authenticate;
trust the IP allowlist instead", and leaned towards a wrapper over a flag.

**Resolved as a wrapper.** `adapter.TrustedProxy` wraps an adapter. It
checks the effective client address against an allowlist, then passes the
request on to the wrapped adapter's `Verify`. A wrapper beat a flag for
three reasons:

1. The weakness is visible in the deployment wiring. When you read the
   config, you can see which adapters are trusted on network position alone.
2. It composes. If you wrap an adapter that *does* authenticate, the
   wrapper adds a second check and does not replace the first.
3. `X-Forwarded-For` handling has to live somewhere, and it is a
   deployment concern, not a protocol one. The wrapper trusts the header
   only from configured forwarder ranges. It takes the last hop that those
   forwarders did not set. That is the furthest-right entry, which an
   attacker cannot forge by adding entries at the front.

An empty allowlist rejects everything; it does not accept everything. The
gateway also refuses to start if Africa's Talking is enabled and no
allowlist is configured.

### Correction: the screen budget is not one number

This RFC first wrote `Body string // ≤182 chars`. That is right only for
text in the GSM 03.38 alphabet (GSM-7, the standard 7-bit character set
for SMS and USSD). If even one character is outside that alphabet, the
whole string is re-encoded as UCS-2, a 16-bit encoding. In UCS-2 a screen
holds **70** 16-bit units, and an emoji outside the Basic Multilingual Plane costs
two of them.

`canonical.ScreenCost` reports the cost of a string and the encoding it
forces. `canonical.Budget` reports the capacity available to that string.
`Response.Validate` compares the two. The SDK measures with these
functions, not by counting runes.

This matters more for this project than for most. The reference adapter
renders content from the federated web, where emoji and non-Latin scripts
are normal, not rare. A simple limit of 182 would have produced screens
that the network refuses.

### Still open

- Outbound push still has no model. Nothing we learned here argues
  against the RFC's lean towards a separate `Pusher`.
- Nobody missed per-segment timestamps in `Path`. Leaving them out for v1
  looks correct.
- Two adapters are not two networks. The simulator is ours, so it cannot
  disagree with us. Promotion to *accepted* still needs a second real
  network. Captured fixtures (`07` to `09`) now confirm the Africa's
  Talking sandbox wire format. Production callbacks are not confirmed yet.

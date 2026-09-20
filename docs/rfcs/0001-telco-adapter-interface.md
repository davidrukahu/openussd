# RFC 0001 - Telco adapter interface

- **Status:** draft (implemented; see "Validated against a first implementation")
- **Author:** David W
- **Updated:** 2026-09-20

## Context

Each Mobile Network Operator (MNO) exposes USSD through a slightly different HTTP callback shape:

- **Safaricom Daraja** - JSON, fields like `sessionId`, `serviceCode`, `phoneNumber`, `text` (where `text` is the menu path joined by `*`).
- **MTN** - varies by country; Nigeria's format differs from Uganda's, both differ from Safaricom.
- **Airtel** - different again; some markets expose only SOAP.
- **Aggregators (Africa's Talking, Infobip, Twilio)** - yet another shape, layered over the underlying MNO.

If the gateway lets MNO-specific shapes leak into application code, every USSD app has to special-case every MNO it deploys to. That defeats the point of the project.

## Decision (proposed)

The gateway defines **one canonical session event** that all telco adapters produce, and **one canonical response** that adapters consume. Adapter authors write a small package implementing two functions: parse-inbound and serialise-outbound.

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

- **Path as `[]string`** rather than the Safaricom-style `*`-joined `text` field. Adapters split or reconstruct as needed; applications never parse the raw blob.
- **`Phase` enum** rather than a boolean `isNew`. MTN and Safaricom express session lifecycle differently; the enum normalises.
- **`Raw` retained** so the audit log can reproduce exactly what the MNO sent. Required for incident investigation and for contract tests.
- **`Verify` separate from `Parse`** so a request that fails authenticity does not allocate session state. Adapters that have no verification (e.g. unauthenticated dev sandboxes) return `nil`.

### Conformance

Each adapter ships with **fixture-driven contract tests**: a directory of `request.http` / `expected-event.json` pairs captured against the real MNO sandbox. CI replays them on every change. The fixtures double as documentation of the MNO's wire format.

## Alternatives considered

1. **Pass through MNO-native types to applications.** Rejected - defeats the SDK value proposition, makes application code MNO-specific.
2. **One giant union type.** Rejected - `Event` would balloon with optional fields nobody uses, and onboarding a new MNO would risk silent breakage of existing adapters.
3. **Code-generated adapters from an OpenAPI spec.** Considered. Worth revisiting once we have three or four adapters and can see the actual variance; premature now.

## Open questions

- Do we model **outbound USSD push** (asynchronous server-initiated dialogues) in the same `Adapter` interface, or as a separate `Pusher`? Leaning separate.
- How does `Verify` express "this MNO does not authenticate; trust the IP allowlist instead"? Probably a wrapper adapter, not a flag.
- Should `Path` carry the timestamps of each segment for debouncing analysis? Probably not in v1.

## Next steps

- ~~Land Safaricom adapter against this interface to validate it.~~
  Safaricom has no public USSD sandbox - Daraja is the M-Pesa portal, not a
  USSD one. Landed an **Africa's Talking** adapter instead; see
  [`docs/telco-access.md`](../telco-access.md).
- Capture the Africa's Talking fixtures against the live sandbox, replacing
  the hand-written ones.
- Land a second real network - MTN via a country sandbox, or Africa's
  Talking production - and adjust the interface based on what did not fit.
- Promote RFC to *accepted* once two adapters and one production tenant ship without interface changes for one release cycle.

## Validated against a first implementation (2026-09)

The interface above has been implemented for two adapters - Africa's
Talking and a local simulator - and a gateway that uses it end to end. What
the exercise changed:

**The interface held.** `Parse` / `Render` / `Verify` needed no new
methods, and `Verify` running before `Parse` paid off immediately: the
gateway can reject an unattributable request without allocating session
state, and the handler reads in that order.

**`Path []string` was the right call.** Africa's Talking sends the
`*`-joined field the RFC anticipated. Splitting it in the adapter meant the
tenant router could match sub-prefixes structurally, and the SDK's screens
never see a separator. One detail the RFC did not state: an **empty
segment is meaningful** - `1**3` is a user who pressed send on an empty
prompt - so segments are preserved rather than filtered, and there is a
fixture for it.

**`Phase` needed a `Terminal()` helper, not more cases.** The four cases
are right, but `Cancel` and `Timeout` share a property the gateway needs to
branch on: no response will reach the handset. Africa's Talking never
delivers either as a callback; the session simply stops. Those phases are
synthesised by the session store on expiry, never produced by that adapter.

**`Raw` earned its place.** It is what makes fixture capture a copy from
the audit log rather than a packet-capture exercise. It is stripped before
the event is forwarded to a tenant: keeping it would leak wire details the
SDK exists to hide, and grow every webhook body.

### Open question resolved: networks that authenticate nothing

The RFC asked how `Verify` should express "this MNO does not authenticate;
trust the IP allowlist instead", and leaned towards a wrapper over a flag.

**Resolved as a wrapper.** `adapter.TrustedProxy` wraps an adapter, checks
the effective client address against an allowlist, and then delegates to
the wrapped `Verify`. Three reasons it beat a flag:

1. The weakness is visible in the deployment wiring. Reading the config
   shows which adapters are trusted on network position alone.
2. It composes. Wrapping an adapter that *does* authenticate adds a second
   gate rather than replacing the first.
3. `X-Forwarded-For` handling has to live somewhere, and it is a
   deployment concern, not a protocol one. The wrapper trusts the header
   only from configured forwarder ranges, and takes the last hop those
   forwarders did not set - the furthest right an attacker cannot forge by
   prepending.

An empty allowlist rejects everything rather than accepting everything, and
the gateway refuses to start with Africa's Talking enabled and no
allowlist configured.

### Correction: the screen budget is not one number

This RFC originally wrote `Body string // ≤182 chars`. That is right only
for text in the GSM 03.38 alphabet. One character outside it re-encodes the
whole string as UCS-2, where a screen holds **70** 16-bit units - and an
emoji outside the Basic Multiplane costs two of them.

`canonical.ScreenCost` reports the cost and the encoding it forces,
`canonical.Budget` the capacity available to that string, and
`Response.Validate` compares the two. The SDK measures with these rather
than counting runes.

This matters more for this project than for most: the reference adapter
renders content from the federated web, where emoji and non-Latin scripts
are the norm rather than the exception, and a naive 182 would have produced
screens the network refuses.

### Still open

- **Outbound push** is still unmodelled. Nothing learned here argues
  against the RFC's lean towards a separate `Pusher`.
- **Per-segment timestamps** in `Path` were not missed. Leaving them out
  for v1 looks correct.
- **Two adapters is not two networks.** The simulator is ours, so it cannot
  disagree with us. Promotion to *accepted* still needs a second real
  network - and the Africa's Talking fixtures still need capturing against
  the live sandbox rather than being written from the published shape.

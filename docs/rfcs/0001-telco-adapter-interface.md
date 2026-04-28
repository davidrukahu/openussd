# RFC 0001 — Telco adapter interface

- **Status:** draft
- **Author:** David W
- **Updated:** 2026-04-28

## Context

Each Mobile Network Operator (MNO) exposes USSD through a slightly different HTTP callback shape:

- **Safaricom Daraja** — JSON, fields like `sessionId`, `serviceCode`, `phoneNumber`, `text` (where `text` is the menu path joined by `*`).
- **MTN** — varies by country; Nigeria's format differs from Uganda's, both differ from Safaricom.
- **Airtel** — different again; some markets expose only SOAP.
- **Aggregators (Africa's Talking, Infobip, Twilio)** — yet another shape, layered over the underlying MNO.

If the gateway lets MNO-specific shapes leak into application code, every USSD app has to special-case every MNO it deploys to. That defeats the point of the project.

## Decision (proposed)

The gateway defines **one canonical session event** that all telco adapters produce, and **one canonical response** that adapters consume. Adapter authors write a small package implementing two functions: parse-inbound and serialise-outbound.

```go
// Conceptual — final names TBD.

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
    Body     string   // ≤182 chars after rendering
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

1. **Pass through MNO-native types to applications.** Rejected — defeats the SDK value proposition, makes application code MNO-specific.
2. **One giant union type.** Rejected — `Event` would balloon with optional fields nobody uses, and onboarding a new MNO would risk silent breakage of existing adapters.
3. **Code-generated adapters from an OpenAPI spec.** Considered. Worth revisiting once we have three or four adapters and can see the actual variance; premature now.

## Open questions

- Do we model **outbound USSD push** (asynchronous server-initiated dialogues) in the same `Adapter` interface, or as a separate `Pusher`? Leaning separate.
- How does `Verify` express "this MNO does not authenticate; trust the IP allowlist instead"? Probably a wrapper adapter, not a flag.
- Should `Path` carry the timestamps of each segment for debouncing analysis? Probably not in v1.

## Next steps

- Land Safaricom adapter against this interface to validate it.
- Land MTN adapter and adjust the interface based on what didn't fit.
- Promote RFC to *accepted* once two adapters and one production tenant ship without interface changes for one release cycle.

# Tenant webhook protocol

> Status: **v1**, draft. Implemented by the gateway and by the Go SDK. Expect
> changes before v1.0 of the project, signalled by the `version` field below.

This is everything needed to write a tenant application in any language. The
[Go SDK](../sdk/go) implements this protocol; it is a convenience, not a
requirement.

A tenant is an HTTP endpoint. The gateway POSTs one signed JSON request per
screen and expects one JSON reply.

## Request

```
POST /your/endpoint HTTP/1.1
Content-Type: application/json
User-Agent: openussd-gateway/0.1
X-OpenUSSD-Tenant: fediverse
X-OpenUSSD-Timestamp: 1758369600
X-OpenUSSD-Signature: v1=3f9a...c2
```

```json
{
  "version": "1",
  "turn": 2,
  "state": {"screen": "timeline", "cursor": 0},
  "event": {
    "mno": "africastalking",
    "session_id": "ATUid_5f0c1a2b3c4d5e6f",
    "msisdn": "+254711223344",
    "shortcode": "*384*1234#",
    "path": ["1"],
    "phase": "continue",
    "network_code": "63902",
    "received_at": "2026-09-20T12:00:00Z"
  }
}
```

| Field | Type | Notes |
|---|---|---|
| `version` | string | Protocol version, currently `"1"`. Refuse a version you do not recognise rather than guessing at the payload. |
| `turn` | number | Screens served in this session, starting at 1. |
| `state` | any | Whatever your last reply returned, verbatim. **Absent** on the first turn and after you clear it. Absent and `null` mean the same thing. |
| `event.mno` | string | Which adapter produced this. Matches the inbound endpoint name. |
| `event.session_id` | string | Network-assigned, opaque, unique only within one `mno`. At most 128 bytes. |
| `event.msisdn` | string | E.164. **A claim by the network, not an authenticated identity.** See below. |
| `event.shortcode` | string | The code the user dialled. |
| `event.path` | array of strings | Input segments since the session began, oldest first. **Always an array**, never null; empty on the first turn and when a routing prefix consumed every segment. Segments never contain `*`. |
| `event.phase` | string | One of `begin`, `continue`, `cancel`, `timeout`. In practice a tenant sees only `begin` and `continue`: the gateway handles the terminal phases itself and does not deliver them. |
| `event.network_code` | string | MNO-reported network identifier where the adapter has one. Omitted otherwise. |
| `event.received_at` | string | RFC 3339, when the gateway accepted the request. |

`event.raw`, the MNO's original bytes, exists on the gateway's internal type
but is always stripped before delivery. Do not expect it.

**Unknown fields must be ignored.** Adding a field is not a version change.

## Reply

Reply `200 OK` with `Content-Type: application/json`. Any other status fails
the dialogue, and the subscriber gets the gateway's generic error screen.

```json
{
  "response": {"body": "1. Timeline\n2. Quit", "end_session": false},
  "state": {"screen": "menu"},
  "clear_state": false
}
```

| Field | Type | Notes |
|---|---|---|
| `response.body` | string | The screen text. Must fit one screen; see the budget below. |
| `response.end_session` | bool | `true` closes the dialogue. The adapter renders this in the network's terminating form. |
| `state` | any | Replaces the stored state. **Omit it to keep what is stored**, so a handler that only reads state need not echo it back. |
| `clear_state` | bool | Discards the stored state even when `state` is set. |

The reply must be at most 16KB. Since `state` rides in it, whatever you store
is bounded by that too.

### The screen budget

A screen holds **182 characters** in the GSM 03.38 alphabet. One character
outside that alphabet, an emoji, a non-Latin script, even a curly quote, and
the whole string is sent as UCS-2 where the limit is **70 16-bit units**, with
characters outside the Basic Multilingual Plane costing two.

The gateway rejects a reply whose body does not fit, and the subscriber gets
an error rather than a truncated screen. Measure before you send.

## Signature

Every request is signed. Verify it before you decode: the endpoint is a
public URL, and the payload asserts a subscriber's phone number.

The signed string is `<timestamp>.<body>`, where `body` is the raw bytes of
the request and `timestamp` is the value of `X-OpenUSSD-Timestamp`:

```
signature = "v1=" + hex(HMAC_SHA256(secret, timestamp + "." + body))
```

To verify:

1. Read the raw body **before** parsing it. The signature covers bytes, and
   re-serialising the JSON will not reproduce them.
2. Reject a timestamp more than 5 minutes from your own clock, in either
   direction.
3. Recompute the signature and compare in **constant time**. A byte-at-a-time
   compare leaks the expected value to anyone willing to time a few thousand
   requests.

Python, for illustration:

```python
import hashlib, hmac, time

def verify(headers, body: bytes, secret: str) -> bool:
    ts = headers.get("X-OpenUSSD-Timestamp", "")
    got = headers.get("X-OpenUSSD-Signature", "")
    if not ts.isdigit() or abs(time.time() - int(ts)) > 300:
        return False
    want = "v1=" + hmac.new(
        secret.encode(), f"{ts}.".encode() + body, hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(got, want)
```

The `v1=` prefix versions the signature scheme independently of the payload
version, so either can rotate without the other.

**Replay:** the timestamp window bounds replay to 5 minutes but does not
prevent it, and there is no nonce yet.

Make anything with side effects idempotent at the application level, keyed
on something your own handler decides, such as the state you are about to
leave. Do not key on `(session_id, turn)`: `turn` is the gateway's own
counter rather than anything the network sent, so a retried callback can
arrive with a higher turn than the input it repeats, and a retry after a
storage failure can arrive with the same turn as genuinely new input.
Tightening this is tracked in the repository's TODOS.

## The MSISDN is a claim

The network asserts the subscriber's number and the gateway passes the
assertion along. Anything that can reach the gateway's inbound endpoint can
claim any number, and for adapters whose provider signs nothing the only
control is a source-address allowlist.

Treat it as a routing hint. Put a PIN or an OTP in front of anything that
matters.

## Sessions

Each screen is an independent HTTP request. The gateway keeps the dialogue
continuous and expires it 180 seconds after the last turn, matching what
networks allow. You do not need your own session store: put what you need in
`state`.

On a shared shortcode a dialogue can be handed from one tenant to another
when a routing prefix first matches. The new tenant starts with no state; it
cannot read what the previous one stored.

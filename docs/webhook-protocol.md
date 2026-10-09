# Tenant webhook protocol

> Status: **v1**, draft. The gateway and the Go SDK implement it. Expect
> changes before v1.0 of the project. The `version` field below signals them.

This page has everything you need to write a tenant application in any
language. The [Go SDK](../sdk/go) implements this protocol. It saves you work,
but you do not have to use it.

A tenant is an HTTP endpoint. For each screen, the gateway POSTs one signed
JSON request and expects one JSON reply.

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

`event.raw` holds the original bytes from the MNO (mobile network operator).
It exists on the gateway's internal type, but the gateway always strips it
before delivery. Do not expect it.

**You must ignore unknown fields.** Adding a field is not a version change.

## Reply

Reply with `200 OK` and `Content-Type: application/json`. Any other status
fails the dialogue, and the subscriber gets the gateway's generic error
screen.

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

The reply must be at most 16 KiB (16,384 bytes). `state` travels inside the reply, so the
state you store must fit inside that limit too.

### The screen budget

A screen holds **182 characters** in the GSM 03.38 alphabet. GSM 03.38, also
called GSM-7, is the 7-bit alphabet of the GSM standard. It covers basic Latin
letters, digits and some symbols.

One character outside that alphabet changes the whole screen. That character
can be an emoji, a letter from a non-Latin script, or even a curly quote. The
whole string is then sent as UCS-2, a 16-bit encoding, where the limit is
**70 16-bit units**. A character outside the Basic Multilingual Plane costs
two units.

The gateway rejects a reply whose body does not fit. The subscriber then gets
an error, not a truncated screen. Measure the body before you send it.

## Signature

The gateway signs every request. Verify the signature before you decode the
body. Your endpoint is a public URL, and the payload claims a subscriber's
phone number.

The signed string is `<timestamp>.<body>`. Here `body` is the raw bytes of
the request, and `timestamp` is the value of `X-OpenUSSD-Timestamp`:

```
signature = "v1=" + hex(HMAC_SHA256(secret, timestamp + "." + body))
```

To verify a request:

1. Read the raw body **before** you parse it. The signature covers bytes. If
   you parse the JSON and serialise it again, you will not get the same
   bytes.
2. Reject a timestamp more than 5 minutes from your own clock, in either
   direction.
3. Compute the signature yourself with the formula above.
4. Compare it with the received signature in **constant time**. A comparison
   that goes one byte at a time leaks the expected value to anyone willing to
   time a few thousand requests.

Here is the check in Python, as an illustration:

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

The `v1=` prefix gives the signature scheme its own version, separate from
the payload version. So either one can change to a new version without the
other.

**Replay:** the timestamp window limits replay to 5 minutes, but it does not
prevent replay. There is no nonce yet.

Make anything with side effects idempotent in your application: doing it
twice must have the same effect as doing it once. Use a key that your own
handler decides, such as the state you are about to leave. Do not use
`(session_id, turn)` as the key. `turn` is the gateway's own counter, not a
value the network sent. A retried callback can arrive with a higher turn than
the input it repeats. A retry after a storage failure can arrive with the same
turn as input that really is new. The repository's TODOS file tracks the work
to tighten this.

## The MSISDN is a claim

The MSISDN is the subscriber's phone number. The network claims the number,
and the gateway passes that claim on. Anything that can reach the gateway's
inbound endpoint can claim any number. Some adapters use a provider that signs
nothing. For those adapters, the only control is a source-address allowlist.

Treat the number as a routing hint. Put a PIN or an OTP (one-time password)
in front of anything that matters.

## Sessions

Each screen is a separate HTTP request. The gateway joins these requests into
one continuous dialogue. By default it expires the dialogue 180 seconds after the last
turn, which matches what networks allow. Operators can change this with
`session.ttl`. You do not need your own session
store. Put what you need in `state`.

On a shared shortcode, the gateway can hand a dialogue from one tenant to
another when a routing prefix first matches. The new tenant starts with no
state. It cannot read what the previous tenant stored.

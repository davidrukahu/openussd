# TODOS

Work that is deliberately deferred, with the reason. The plan lives in
[`ROADMAP.md`](ROADMAP.md); this file is for decisions taken during
implementation.

## Telco adapters

### Handle Africa's Talking session-end events

**Priority:** P2

The sandbox captures show a separate Events callback carrying
a `status` when a session ends (`Success` and `Incomplete` observed so
far; other values may exist). It is the only signal that
a session ended without the application finishing it, so it should become a
canonical `Cancel` (or `Timeout`, once a timeout has been captured and shown
to differ) and let the gateway drop the session instead of waiting for the
idle timeout. The
payload is recorded in `docs/telco-access.md`.
The Events callback is as unsigned as the dialogue one, so it must sit
behind the same `TrustedProxy` allowlist; otherwise a forged `status` could
end another subscriber's session.

### Second real network

**Priority:** P2

RFC-0001 stays draft until a second network has exercised the interface. The
simulator does not count: it is ours, so it cannot disagree with us.

## Gateway

### Replay protection for tenant webhooks

**Priority:** P2

The signature's 5-minute timestamp window bounds replay but does not prevent
it. A nonce inside the signed string plus a seen-cache would. Documented as
a limitation in `docs/webhook-protocol.md`, which tells tenants to
deduplicate on `(session_id, turn)` meanwhile.

### Ship the simulator adapter disabled by default

**Priority:** P2

`gateway/gateway.example.yaml` enables an adapter that authenticates
nothing, and `docker-compose.yml` publishes its port. That is the demo, and
the gateway warns when it is bound to a non-loopback address, but the
default should flip once there is a second way to try the project.

### Bound the in-memory session store

**Priority:** P3

`session.Memory` has no maximum entry count. Session ids are bounded and the
inbound endpoints are allowlisted, so this is a second-order concern, but a
store that only grows is a store that eventually falls over.

### Per-session concurrency control

**Priority:** P1

`httpx.Inbound.handle` loads a session, dispatches to the tenant, and saves,
with no lock, lease or compare-and-set. Aggregator retries routinely produce
two callbacks for one session, and both currently reach the tenant: a double
side effect, then last-write-wins on the state. The race detector does not
find this, because it is a logical race rather than a memory one. Wants a
per-session single-flight in the gateway, or a version field on the stored
session and a CAS in both stores.

### A sound idempotency key for tenants

**Priority:** P1

`docs/webhook-protocol.md` tells tenants to deduplicate on
`(session_id, turn)`, and `turn` is gateway-side mutable state rather than
anything the network sent. It fails in both directions: a retried callback
after a successful turn arrives with a higher turn and gets reprocessed, and
a retry after a `Save` failure arrives with the same turn as a genuinely new
input. For the Africa's Talking wire format the input path length is the
network's own sequence number, which is the material a real key would be
built from. Fix the advice and the mechanism together.

### Cache the upstream timeline

**Priority:** P2

`adapters/fediverse` fetches from the instance on every timeline keypress,
with no cache, rate limit or circuit breaker. A shortcode with real traffic
becomes an unattributed load generator against a third-party instance and
gets the operator's egress IP blocked. A 10 to 30 second TTL cache keyed on
the instance removes the class.

### Decide a redirect policy for instance fetches

**Priority:** P2

`defaultInstanceClient` follows any redirect, including to plain `http://`
and to private or link-local addresses. Now that the demo defaults to an
instance the operator does not run, a hostile or compromised instance could
point the adapter at internal services, and the first 200 printable bytes of
their JSON `error` field would reach the operator log. Allowing only
same-host or HTTPS redirects in `CheckRedirect` would close that.

### Cap the host in instance errors

**Priority:** P2

`PublicTimeline` cuts the remote error text to 200 bytes but not the host
it names. Behind an HTTP proxy, a hostile instance can redirect to a very
long host name that only the proxy resolves, and every handset request then
logs it. The host wants the same cut as the error text, and an empty host
(possible only with a custom transport) should fall back to the configured
one.

### Check the instance at startup

**Priority:** P2

A closed or misconfigured instance shows up only when a subscriber dials:
`/healthz` always answers ok, and an instance given without a scheme
(`fosstodon.org`) starts cleanly and then fails every dialogue. Refusing to
start without an http or https URL, and logging one probe of the timeline
with the sign-in hint, would surface both at deploy time.

### Make instance errors easier to act on

**Priority:** P3

Return a typed error for "requires sign-in" so the menu handler can log it
once, or at a lower level, instead of as an error on every keypress. Name
both the configured and the answering host when a redirect happened. Treat
right-to-left letters and invisible fillers in remote text as `printable`
already treats bidi controls. Pin the 200-byte cap exactly in the test.

### Keep no-break spaces in logged instance errors

**Priority:** P3

`printable` drops every character `unicode.IsPrint` rejects, which includes
U+00A0 and U+202F. French typography puts those before `:` and `?`, so an
instance answering in French logs words run together. Dropping only format
and control characters would keep them.

### Confirm the UCS-2 screen limit for USSD

**Priority:** P3

The screen budget uses 70 UCS-2 units (`canonical.MaxUCS2Units`), which is
the SMS figure (140 octets). USSD carries 160 octets, which would hold 80
UCS-2 characters. Confirm what Africa's Talking and a real network accept
before raising it; 70 is safe meanwhile, only stricter than needed.

## Testing

### Test the Redis session store

**Priority:** P2

`gateway/internal/session/redis.go` has no coverage, and it is what
multi-replica deployments use. Proper coverage needs a test-only dependency
such as `github.com/alicebob/miniredis/v2`; the decision to add one is
pending.

## Distribution

### Release pipeline for the binaries

**Priority:** P2

The repo builds three binaries and publishes none. Docker Compose is the
documented path and it works, so this is deferred rather than missing. A
real pipeline wants a cross-platform matrix and signed artifacts, which is
M6 work.

## Completed

### Africa's Talking fixtures captured from the sandbox

**Completed:** v0.1.2 (2026-10-09). Three fixtures captured from the sandbox simulator
alongside the hand-written ones; see the provenance table in
`gateway/testdata/fixtures/africastalking/PROVENANCE.md`.

### Gateway core, Go SDK, and Fediverse reference adapter

**Completed:** v0.1.0 (2026-09-20)

# TODOS

Work that is deliberately deferred, with the reason. Milestones M1 to M6
live in [GitHub issues](https://github.com/davidrukahu/openussd/issues);
this file is for decisions taken during implementation.

## Telco adapters

### Capture Africa's Talking fixtures against the live sandbox

**Priority:** P1

The fixtures in `gateway/testdata/fixtures/africastalking/` are hand-written
from the provider's published request shape, so they prove the adapter is
self-consistent rather than that it matches the network. Register for the
sandbox, point a USSD channel at a tunnelled gateway, dial from the browser
simulator, and copy the bodies the gateway logs in `Event.Raw`. This is what
[#7](https://github.com/davidrukahu/openussd/issues/7) should become, now
that Daraja is known to be the wrong target.

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

### Gateway core, Go SDK, and Fediverse reference adapter

**Completed:** v0.1.0 (2026-09-20)

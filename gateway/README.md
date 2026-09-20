# OpenUSSD gateway

A self-hostable service that accepts USSD callbacks from a mobile network,
keeps each dialogue continuous across the independent HTTP requests a USSD
session is made of, and forwards canonical session events to tenant
applications over signed webhooks.

> **Status: v0.1 spike.** The interfaces here are the ones described in
> [RFC-0001](../docs/rfcs/0001-telco-adapter-interface.md) and they work,
> but they have not yet met a production network. Expect breaking changes.

## Running it in an afternoon

```bash
export FEDIVERSE_WEBHOOK_SECRET=$(openssl rand -hex 32)
docker compose up --build
```

Then dial from the terminal handset:

```bash
go run ./cmd/ussdsim -shortcode '*384*1234#'
```

Without Docker. The example config points its tenant at `http://fediverse:8081/ussd`, which is the compose service name, so override it for a host run:

```bash
make build
export FEDIVERSE_WEBHOOK_SECRET=$(openssl rand -hex 32)
FEDIVERSE_LISTEN=127.0.0.1:8081 ./bin/fediverse &
sed 's|http://fediverse:8081|http://127.0.0.1:8081|' \
  gateway/gateway.example.yaml > /tmp/gateway.yaml
./bin/gateway -config /tmp/gateway.yaml
```

## Configuration

See [`gateway.example.yaml`](gateway.example.yaml). `${NAME}` is expanded
from the environment, and an unset variable is a startup error - secrets
stay out of the file.

| Key | Meaning |
|---|---|
| `listen` | Bind address. Default `:8080`. |
| `log_level` | `debug`, `info`, `warn`, `error`. |
| `session.backend` | `memory` (default) or `redis`. |
| `session.redis_url` | Required when the backend is `redis`. |
| `session.ttl` | Idle session lifetime. Default 180s, matching what networks allow. |
| `adapters.<name>.enabled` | Mounts `/ussd/<name>`. |
| `adapters.<name>.allowed_sources` | CIDRs permitted to deliver callbacks. |
| `adapters.<name>.trusted_forwarders` | CIDRs of your own proxies; the only hops allowed to set `X-Forwarded-For`. |
| `tenants[]` | Name, shortcode, optional sub-prefix, webhook URL, secret, timeout. |

### Endpoints

| Path | Purpose |
|---|---|
| `/ussd/africastalking` | Inbound callbacks, form-encoded, `CON`/`END` replies. |
| `/ussd/simulator` | Inbound callbacks from `cmd/ussdsim`, JSON. Development only. |
| `/healthz` | Liveness. Checks nothing external on purpose: a liveness probe that fails when Redis is down restarts a gateway that was working. |
| `/readyz` | Readiness, the enabled adapter names, and a tenant count. It reports counts rather than the routing table: the endpoint is unauthenticated, and a list of tenants with their shortcodes and prefixes is what you would need to aim a forged callback. The full table is logged at startup. |

## Screen budget

A screen is not simply "182 characters". GSM 03.38 packs 182 septets, but
one character outside that alphabet re-encodes the whole screen as UCS-2,
where the limit is 70 units. The gateway rejects a tenant response that
does not fit the encoding its own text forces, and says which encoding it
measured, so an operator sees the real constraint in the logs rather than a
mangled screen on a handset.

## Session handling

Sessions are keyed by `(mno, session_id)` and expire 180s after the last
turn, so the gateway and the network give up at about the same time.

Application state is stored as **opaque JSON**. The gateway never inspects
it: it cannot validate a schema it does not know, and reading tenant state
would undermine the isolation the routing layer provides. CBOR is roughly
30% smaller and remains an option behind the same interface, but being able
to read live state with `redis-cli GET` during an incident is worth more
than the bytes until Redis pressure is measurable
([#10](https://github.com/davidrukahu/openussd/issues/10)).

The memory backend is correct for exactly one replica. Two replicas each
hold half of every conversation; use Redis.

## Routing

A tenant claims a `(shortcode, prefix)` pair. Prefixes select a service on a
shared shortcode and are stripped before the event reaches the tenant - the
selector is the gateway's routing, not the application's menu.

Every shortcode needs exactly one tenant with an empty prefix to answer the
first screen, and the router refuses to start otherwise. Routing is
re-evaluated each turn, so a menu tenant can hand a dialogue to a service
tenant; when that happens the stored state is dropped, so no tenant ever
reads another's data.

## Security

**Inbound.** Adapters `Verify` before they `Parse`, so a request that cannot
be attributed never allocates session state. Africa's Talking signs nothing
and sends no secret, so the only control available is a source allowlist:
configure `allowed_sources` and keep the inbound listener off the public
internet. The gateway refuses to start with that adapter enabled and the
allowlist empty.

**Outbound.** Each tenant webhook is signed HMAC-SHA256 over
`<timestamp>.<body>`, sent as `X-OpenUSSD-Signature: v1=…` with
`X-OpenUSSD-Timestamp`. Verifiers reject anything more than five minutes
from their own clock. The SDK does this for you, and
[`docs/webhook-protocol.md`](../docs/webhook-protocol.md) has the full wire
contract for tenants written in other languages.

**MSISDN is a claim, not an identity.** The network asserts it and the
gateway passes it along as an assertion. Anything that reaches the inbound
endpoint can claim any number. Gate anything that matters behind a PIN or
an OTP.

**The simulator adapter authenticates nothing.** It is for local
development. The gateway logs a warning when it is enabled on a non-loopback
address.

## Adding a network

1. Create `gateway/internal/adapter/<name>/` implementing `Parse`, `Render`,
   and `Verify`.
2. Add fixtures under `gateway/testdata/fixtures/<name>/` as
   `request.http` / `expected-event.json` pairs, ideally captured from the
   real sandbox, and record their provenance.
3. Register the adapter in `newRegistry` in
   [`cmd/gateway/main.go`](cmd/gateway/main.go).

If the network's shape does not fit `canonical.Event`, that is a finding
about the contract, not about your adapter - raise it against RFC-0001.

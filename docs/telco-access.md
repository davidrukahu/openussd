# Getting USSD access in practice

> Status: **notes from doing it**, 2026-09, sandbox captures 2026-10. Corrections welcome on
> [#11](https://github.com/davidrukahu/openussd/issues/11). This page is
> only as good as the markets people have actually shipped in.

USSD is the menu service a phone opens when you dial a code such as
`*384*70421#`. Getting sandbox access to it is the most likely reason for a
milestone to slip. So this page writes down what you can actually get,
before any code depends on it.

## The Daraja correction

The project's original plan named the **Safaricom Daraja USSD sandbox** as
the first telco adapter, and [#7](https://github.com/davidrukahu/openussd/issues/7)
tracked getting access to it.

Daraja is Safaricom's developer portal for **M-Pesa**. It covers STK push,
C2B and B2C payments, transaction status and account balance. It does not
expose USSD. There is no USSD sandbox behind it to register for.

Safaricom USSD runs on a commercial shortcode. You need a registered Kenyan
entity, a service application and a commercial agreement, and the lead time
is months. A funded project can reasonably go after that. It is not
reasonable to make the first adapter depend on it.

**Consequence for the plan:** direct Safaricom access cannot happen soon,
and the first adapter cannot wait for it. So the first adapter is Africa's
Talking instead. Direct integration with Safaricom waits until a commercial
agreement is in place; it is not part of the current
[roadmap](../ROADMAP.md).

## What is obtainable today

| Provider | USSD sandbox | Cost | Notes |
|---|---|---|---|
| **Africa's Talking** | Yes, immediate | Free | Browser handset simulator, no commercial agreement. Reaches KE, UG, NG, RW, TZ, MW. The adapter this repo ships. |
| Safaricom (direct) | No | Commercial | Shortcode agreement, registered entity, months. |
| MTN (per country) | Varies | Varies | Developer portals exist in some markets; USSD is usually not self-service. |
| Airtel | No public sandbox | Commercial | Some markets expose SOAP only. |
| Infobip / Twilio | Aggregator accounts | Paid | Twilio has no USSD product in most markets; Infobip is sales-led. |

An aggregator is not a second-best choice for the architecture. RFC-0001
names aggregators as a valid adapter class. One aggregator integration
reaches several networks in several countries. That is more coverage per
adapter than a direct integration with one network gives.

A direct integration gives you a lower cost per session at volume, no
dependence on an intermediary, and a wire format that no aggregator can
change. Those things matter at production scale, not at v0.1.

## Africa's Talking, concretely

1. Create an account and use the **sandbox** app.
2. Create a USSD channel. The sandbox assigns a code of the form
   `*384*NNNNN#`, for example `*384*70421#`.
3. Point the channel's callback at your gateway:
   `https://<host>/ussd/africastalking`. A tunnel is fine for sandbox
   testing if you set it up carefully, because the tunnel makes the gateway
   public. For anything beyond testing, follow
   [`gateway/README.md`](../gateway/README.md) and keep the inbound
   listener off the public internet. For a test tunnel:
   - **Turn the simulator off** (`simulator.enabled: false`). The simulator
     accepts anything from anyone, and the example config turns it on.
   - **Use an HTTP tunnel that appends the caller's address to
     `X-Forwarded-For`.** cloudflared does this. A TCP tunnel such as
     `ssh -R` passes no address, so if you trust it, anyone can claim any
     source.
   - **Run the gateway directly on your machine, listening on loopback
     only** (`listen: "127.0.0.1:8080"`). Do not run it under Docker
     Compose. Then nothing outside your machine can reach the gateway except
     through the tunnel. Trusting loopback means every program and browser
     tab on that machine can claim any source. So use a test machine you
     control, and stop the tunnel when you are done.
   - Every request then arrives from the tunnel agent. So put
     `127.0.0.1/32` in `trusted_forwarders` and the provider's address in
     `allowed_sources`. Point the tunnel at `127.0.0.1:8080`, not
     `localhost`, so that it does not connect over IPv6. Never put a
     loopback address, `0.0.0.0/0` or `::/0` in `allowed_sources`. The
     tunnel is public, so any of those admits anyone.
4. Set the tenant's `shortcode` in the gateway config to the code the
   sandbox assigned, for example `*384*70421#`. The example config uses
   `*384*1234#`, and a code with no tenant gets no route. If you run outside
   Docker Compose, also point the tenant's `webhook_url` at an address the
   host can reach, for example `http://127.0.0.1:8081/ussd`. The
   `fediverse` hostname only exists inside Compose.
5. Dial the code from the browser simulator.

The callback is a form POST with the fields `sessionId`, `serviceCode`,
`phoneNumber`, `networkCode` and `text`. The reply is plain text that begins
with `CON ` or `END `. In the sandbox, `networkCode` is `99999`. Africa's
Talking signs nothing and sends no secret. That is why the adapter's
`Verify` always succeeds, and why the deployment is expected to wrap it in
`adapter.TrustedProxy` with the provider's published source ranges. Check
those ranges against your own account. Do not copy them from the example
config: they differ by region, and they change.

The example config ships a placeholder range that matches nothing. So the
adapter rejects every request until you fill in the real range.

For the **sandbox**: on 2026-10-09 every callback came from
`18.133.205.228`, an AWS address in London. Keep two cautions in mind
before you allow it. First, every sandbox account shares this address. So
it proves that a request came from Africa's Talking, not that it came from
your channel. What binds a request to you is the gateway's exact shortcode
routing, which drops any code you have not configured. Second, it is a
cloud address and it may change. AWS can give a released address to
another customer, so a stale entry admits a stranger. Check your own first
callbacks instead of copying this address. Also remember that in the
sandbox, `phoneNumber` is whatever the person dialling typed.

For **production**: ask Africa's Talking support for the published list of
callback addresses for your account. Allow all of them, not only the first
one you see. Otherwise callbacks will drop when they fail over to another
address. Every Africa's Talking customer shares these addresses too. So the
allowlist proves who sent a callback, not which application it is for.
Anything sensitive needs its own check, such as a PIN.

### Session-end events

A channel can also have an **Events URL**. When a session ends, Africa's
Talking posts a summary to that URL, separate from the dialogue itself:

```
durationInMillis=192&phoneNumber=%2B254000000001&errorMessage=
&serviceCode=%2A384%2A70421%23
&lastAppResponse=CON+OpenUSSD+Fediverse%0A1.+Public+timeline%0A2.+About%0A0.+Quit
&hopsMetadata=&hopsCount=1&status=Incomplete
&sessionId=ATUid_f219bf23880979c479e835cb4d640287&cost=0.00
&date=2026-10-09+12%3A17%3A37&networkCode=99999&input=
```

The payload is wrapped here for reading. It arrives as one line. `status` is
`Success` when the application ended the session with `END`. The
`Incomplete` above came from a session that the simulator abandoned when
the next dial started. What a deliberate hang-up or a network timeout
reports is not confirmed yet. Either way, this is the only signal that a
session ended without the application finishing it. So it should map onto
the canonical `Cancel` or `Timeout` phase once a timeout has been captured.
The gateway does not handle this endpoint yet.

## Fixtures

Contract-test fixtures live in
[`gateway/testdata/fixtures/africastalking/`](../gateway/testdata/fixtures/africastalking/).
Three of them (`07` to `09`) were **captured from the sandbox** on
2026-10-09. They show that the adapter parses what the sandbox actually
sends. Production callbacks, with real operator codes, are not captured yet.
The hand-written fixtures stay for cases the simulator cannot easily
produce, such as an empty segment or a number without `+`.
[`PROVENANCE.md`](../gateway/testdata/fixtures/africastalking/PROVENANCE.md)
records which fixtures are which and how the captures were made.

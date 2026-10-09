# Getting USSD access in practice

> Status: **notes from doing it**, 2026-09, sandbox captures 2026-10. Corrections welcome on
> [#11](https://github.com/davidrukahu/openussd/issues/11) - this page is
> only as good as the markets people have actually shipped in.

Sandbox access is the single most likely cause of a milestone slipping, so
it is worth writing down what is actually obtainable before code depends on
it.

## The Daraja correction

The project's original plan named **Safaricom Daraja USSD sandbox** as the
first telco adapter, and [#7](https://github.com/davidrukahu/openussd/issues/7)
tracked obtaining it.

Daraja is Safaricom's **M-Pesa** developer portal. It covers STK push, C2B
and B2C payments, transaction status, and account balance. It does not
expose USSD. There is no USSD sandbox behind it to register for.

Safaricom USSD runs on a commercial shortcode: a registered Kenyan entity,
a service application, a commercial agreement, and a lead time measured in
months. That is a reasonable thing for a funded project to pursue and an
unreasonable thing to make M1 depend on.

**Consequence for the plan:** direct Safaricom access cannot happen
before funding, and M1 cannot wait for it. The first adapter is Africa's Talking
instead, and direct MNO integration moves to the funded phase where the
commercial process has time to run.

## What is obtainable today

| Provider | USSD sandbox | Cost | Notes |
|---|---|---|---|
| **Africa's Talking** | Yes, immediate | Free | Browser handset simulator, no commercial agreement. Reaches KE, UG, NG, RW, TZ, MW. The adapter this repo ships. |
| Safaricom (direct) | No | Commercial | Shortcode agreement, registered entity, months. |
| MTN (per country) | Varies | Varies | Developer portals exist in some markets; USSD is usually not self-service. |
| Airtel | No public | Commercial | Some markets expose SOAP only. |
| Infobip / Twilio | Aggregator accounts | Paid | Twilio has no USSD product in most markets; Infobip is sales-led. |

An aggregator is not a second-best option architecturally. RFC-0001 names
aggregators as a valid adapter class, and one aggregator integration
reaches several networks across several countries, which is more coverage
per adapter than a direct MNO integration gives.

What a direct integration buys is lower per-session cost at volume,
independence from an intermediary, and a wire format the aggregator cannot
change under you. Those matter at production scale, not at v0.1.

## Africa's Talking, concretely

1. Create an account and use the **sandbox** app.
2. Create a USSD channel. The sandbox assigns a code of the form
   `*384*NNNNN#`, for example `*384*70421#`.
3. Point the channel's callback at your gateway:
   `https://<host>/ussd/africastalking`. A tunnel is fine for sandbox
   testing if you set it up carefully, because it makes the gateway public.
   For anything beyond testing, follow
   [`gateway/README.md`](../gateway/README.md) and keep the inbound
   listener off the public internet. For a test tunnel:
   - **Turn the simulator off** (`simulator.enabled: false`). It accepts
     anything from anyone, and the example config has it on.
   - **Use an HTTP tunnel that appends the caller's address to
     `X-Forwarded-For`** (cloudflared does). A TCP tunnel such as `ssh -R`
     passes no address, so trusting it would let anyone claim any source.
   - **Run the gateway directly on your machine, listening on loopback
     only** (`listen: "127.0.0.1:8080"`), not under Docker Compose, so
     nothing outside your machine can reach it except through the tunnel.
     Trusting loopback means every program and browser tab on that machine
     can claim any source, so use a test machine you control and stop the
     tunnel when you are done.
   - Every request then arrives from the agent, so put `127.0.0.1/32` in
     `trusted_forwarders` and the provider's address in `allowed_sources`.
     Point the tunnel at `127.0.0.1:8080` rather than `localhost`, so it
     does not connect over IPv6. Never put a loopback address, `0.0.0.0/0`
     or `::/0` in `allowed_sources`: the tunnel is public, so that admits
     anyone.
4. Set the tenant's `shortcode` in the gateway config to the code the
   sandbox assigned (for example `*384*70421#`); the example config uses
   `*384*1234#`, and a code with no tenant gets no route. Outside Docker
   Compose, also point the tenant's `webhook_url` at an address the host
   can reach (for example `http://127.0.0.1:8081/ussd`), since the
   `fediverse` hostname only exists inside Compose.
5. Dial the code from the browser simulator.

The callback is a form POST - `sessionId`, `serviceCode`, `phoneNumber`,
`networkCode`, `text` - and the reply is plain text beginning `CON ` or
`END `. In the sandbox, `networkCode` is `99999`. Nothing is signed and no secret is sent, which is why the adapter's
`Verify` always succeeds and the deployment is expected to wrap it in
`adapter.TrustedProxy` with the provider's published source ranges. Confirm
those ranges against your own account rather than copying the example
config: they differ by region and they change.

The example config ships a placeholder range that matches nothing, so the
adapter rejects every request until you fill it in.

For the **sandbox**: on 2026-10-09 every callback came from
`18.133.205.228`, an AWS address in London. Two cautions before allowing it.
It is shared by every sandbox account, so it proves a request came from
Africa's Talking, not from your channel; what binds a request to you is the
gateway's exact shortcode routing, which drops any code you have not
configured. And it is a cloud address that may change; a released address
can be handed to another AWS customer, so a stale entry admits a stranger.
Check your own first callbacks rather than copying it, and remember that in
the sandbox `phoneNumber` is whatever the person dialling typed.

For **production**: ask Africa's Talking support for the published list of
callback addresses for your account and allow all of them, not only the
first one you see, or callbacks will drop when they fail over. Those
addresses are shared by every Africa's Talking customer too, so the
allowlist proves who sent a callback, not which application it is for:
anything sensitive needs its own check, such as a PIN.

### Session-end events

A channel can also have an **Events URL**. When a session ends, Africa's
Talking posts a summary there, separately from the dialogue itself:

```
durationInMillis=192&phoneNumber=%2B254000000001&errorMessage=
&serviceCode=%2A384%2A70421%23
&lastAppResponse=CON+OpenUSSD+Fediverse%0A1.+Public+timeline%0A2.+About%0A0.+Quit
&hopsMetadata=&hopsCount=1&status=Incomplete
&sessionId=ATUid_f219bf23880979c479e835cb4d640287&cost=0.00
&date=2026-10-09+12%3A17%3A37&networkCode=99999&input=
```

(Wrapped here for reading; it arrives as one line.) `status` is `Success`
when the application ended the session with `END`. The `Incomplete` above
came from a session the simulator abandoned when the next dial started;
what a deliberate hang-up or a network timeout reports is not yet
confirmed. Either way this is
the only signal that a session ended without the application finishing it,
so it should map onto the canonical `Cancel` or `Timeout` phase once a
timeout has been captured. The gateway does not handle this endpoint yet.

## Fixtures

Contract-test fixtures live in
[`gateway/testdata/fixtures/africastalking/`](../gateway/testdata/fixtures/africastalking/).
Three of them (`07` to `09`) were **captured from the sandbox** on
2026-10-09 and show that the adapter parses what the sandbox actually
sends. Production callbacks (real operator codes) are not yet captured.
The hand-written ones stay for cases the simulator cannot easily produce, such
as an empty segment or a number without `+`.
[`PROVENANCE.md`](../gateway/testdata/fixtures/africastalking/PROVENANCE.md)
records which is which and how the captures were made.

# Getting USSD access in practice

> Status: **notes from doing it**, 2026-09. Corrections welcome on
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

**Consequence for the plan:** #7 cannot be closed as written before
funding, and M1 cannot wait for it. The first adapter is Africa's Talking
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
   `*384*NNNN#`.
3. Point the channel's callback at your gateway:
   `https://<host>/ussd/africastalking`. A tunnel is fine for testing; the
   allowlist below assumes the gateway is not otherwise reachable.
4. Dial the code from the browser simulator.

The callback is a form POST - `sessionId`, `serviceCode`, `phoneNumber`,
`networkCode`, `text` - and the reply is plain text beginning `CON ` or
`END `. Nothing is signed and no secret is sent, which is why the adapter's
`Verify` always succeeds and the deployment is expected to wrap it in
`adapter.TrustedProxy` with the provider's published source ranges. Confirm
those ranges against your own account rather than copying the example
config: they differ by region and they change.

## Fixtures

Contract-test fixtures live in
[`gateway/testdata/fixtures/africastalking/`](../gateway/testdata/fixtures/africastalking/).
They are currently **hand-written from the published request shape**, which
proves the adapter is self-consistent but not that it matches the network.

Replacing them with captures from a real sandbox session is the concrete
deliverable that #7 should become. The gateway retains every inbound body
in `Event.Raw` precisely so a capture is a copy-and-paste from the audit
log.

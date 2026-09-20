# Africa's Talking contract-test fixtures

Each directory holds one inbound request as raw HTTP (`request.http`) and the
canonical event the adapter must produce from it (`expected-event.json`).
`TestFixtures` in `../../../internal/adapter/africastalking` replays every
directory on each change; a mismatch fails CI.

`expected-event.json` omits `received_at` (the test injects a fixed clock) and
`raw` (the test asserts it equals the request body verbatim).

## Provenance

Fixtures are one of two kinds, and the distinction matters - a hand-written
fixture only proves the adapter is self-consistent, while a captured one
proves it matches the network.

| Fixture | Kind | Notes |
|---|---|---|
| `01-begin` | hand-written | Fresh dial, empty `text`. |
| `02-continue-single` | hand-written | One menu selection. |
| `03-continue-nested` | hand-written | Three levels deep; `*`-joined. |
| `04-empty-segment` | hand-written | User sent an empty reply mid-session; the empty segment is preserved, not filtered. |
| `05-msisdn-without-plus` | hand-written | Uganda shortcode, `phoneNumber` missing the E.164 `+`. Guards the normalisation that otherwise forks the session key. |
| `06-free-text-input` | hand-written | Free-text entry, form-encoded space. |

**All fixtures are currently hand-written** from the provider's published
request shape. They have not yet been captured against the live sandbox.

Replacing them is issue #7's real deliverable: register for the Africa's
Talking sandbox, point a USSD channel at a tunnelled gateway, dial from the
browser simulator, and copy the bodies the gateway logs in `Event.Raw` into
these files. Any fixture replaced that way moves to *captured* in the table
above, with the capture date.

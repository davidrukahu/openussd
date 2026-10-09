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
| `07-captured-begin` | captured 2026-10-09 | Sandbox, fresh dial. |
| `08-captured-continue` | captured 2026-10-09 | Sandbox, one selection. |
| `09-captured-nested` | captured 2026-10-09 | Sandbox, three selections. |

The captured fixtures came from the Africa's Talking sandbox simulator,
dialling `*384*70421#`, a channel on the shared `*384#` code, pointed at a
tunnelled gateway. Bodies are what the provider sent, except for the
subscriber number noted below. The headers are reduced to the ones the
provider set (`User-Agent`, `Content-Type`, `Content-Length`), `Host` is
replaced with the placeholder `gateway.example`, and headers added by the
capture tunnel are left out. The subscriber number in the captures has been
replaced with `+254000000001`, which no Kenyan network allocates; it has the
same length, so `Content-Length` is unchanged.

What the captures showed that the hand-written fixtures did not:

- The field order differs (`phoneNumber` first) and `serviceCode` arrives
  fully percent-encoded (`%2A384%2A70421%23`).
- The sandbox reports `networkCode=99999` rather than a real operator code.
- `User-Agent` is `at-ussd-api/1.0`.

The hand-written fixtures stay: they cover cases the simulator cannot easily
produce (an empty segment, a number without `+`, free text with spaces).

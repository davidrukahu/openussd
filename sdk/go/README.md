# OpenUSSD Go SDK

Build a USSD application as a set of screens. The SDK handles what every
USSD app otherwise rewrites: verifying the gateway's signature, decoding
the canonical event, tracking which screen the user is on, persisting typed
state between turns, and keeping every screen inside what the network will
actually carry.

```go
import openussd "github.com/davidrukahu/openussd/sdk/go"
```

The wire protocol this SDK speaks is documented in
[`docs/webhook-protocol.md`](../../docs/webhook-protocol.md), so a tenant can
be written in any language.

> Licensed AGPL-3.0-or-later while it lives in this monorepo. It will be
> re-licensed Apache-2.0 if it is split into its own module, so it can be
> embedded without copyleft propagation.

## A whole application

```go
type state struct{ Name string `json:"name"` }

app, err := openussd.NewApp[state]("menu",
    openussd.Screen[state]{
        Name: "menu",
        Prompt: func(c *openussd.Context[state]) (string, error) {
            return openussd.Menu("Welcome", []openussd.MenuItem{
                {Key: "1", Label: "Set name"},
                {Key: "0", Label: "Quit"},
            }), nil
        },
        Handle: func(c *openussd.Context[state], input string) (openussd.Action, error) {
            switch input {
            case "1":
                return openussd.Goto("name")
            case "0":
                return openussd.Finish("Goodbye.")
            default:
                return openussd.Stay("Invalid choice.")
            }
        },
    },
    openussd.Screen[state]{
        Name:   "name",
        Prompt: func(c *openussd.Context[state]) (string, error) { return "Enter your name:", nil },
        Handle: func(c *openussd.Context[state], input string) (openussd.Action, error) {
            if strings.TrimSpace(input) == "" {
                return openussd.Stay("Name cannot be empty.")
            }
            c.State.Name = input
            return openussd.Goto("done")
        },
    },
    openussd.Screen[state]{
        // No Handle, so the dialogue ends with this screen displayed.
        Name:   "done",
        Prompt: func(c *openussd.Context[state]) (string, error) { return "Saved, " + c.State.Name + ".", nil },
    },
)

http.Handle("/ussd", openussd.NewHandler(app, os.Getenv("WEBHOOK_SECRET"), nil))
```

## Concepts

**Screens.** `Prompt` renders; `Handle` decides what happens next. A screen
without `Handle` is terminal.

**Actions.** `Goto` moves on, `Finish` ends the dialogue, and `Stay`
re-renders the current screen with a message above it - the invalid-input
path, so a user never loses their place over a typo.

**Context.** `Context.Ctx()` carries the gateway's deadline for this turn.
Pass it to anything you call out to: the gateway abandons a turn after its
per-tenant timeout because the handset is already gone, and an outbound call
that ignores the cancellation keeps running for a dialogue nobody is reading.

**State.** `Context.State` is a pointer to your own struct. It is encoded
into the gateway's opaque blob after each turn and decoded before the next.
Corrupt state restarts the dialogue rather than killing it.

**The budget.** A screen holds 182 characters in the GSM 03.38 alphabet -
and 70 units the moment it contains anything else, because one emoji or one
Chinese character re-encodes the whole string as UCS-2. `Truncate`,
`Paginate`, `Menu`, `MenuFit` and `Shrink` measure against the encoding the
text itself forces, and every rendered screen passes through `Shrink` as a
backstop. `Fits` is there for your own tests.

Use `MenuFit` whenever an option must stay reachable: it renders the footer
first, so a long list cannot push "Back" off the screen.

Use `ToGSM` or `Label` where the trade is worth making. A menu of display
names loses nothing important by dropping emoji, and gains most of its
options back - one emoji in one name can cut a ten-option list to one.
A post body is the opposite case: transliterating away a message written in
Chinese leaves nothing to read, so let it paginate instead.

**Languages.** `NewBundle` holds per-language strings; `Context.T` resolves
one, falling back to the bundle's fallback language and then to the key
itself, so a missing translation is visible in testing rather than blank in
production.

## Testing an application

`App.Turn` takes a context, an event, a turn number, and the stored state,
and returns the screen and the new state. No HTTP, no gateway:

```go
resp, state, err := app.Turn(context.Background(), canonical.Event{
    MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
    Path: []string{"1"}, Phase: canonical.PhaseContinue,
}, 2, previousState)
```

See [`app_test.go`](app_test.go) for a dialogue driver worth copying.

## The MSISDN is a claim

`Context.MSISDN()` returns what the network asserted. It is not an
authenticated identity: anything that can reach the gateway's inbound
endpoint can claim any number. Gate anything that matters behind a PIN or
an OTP.

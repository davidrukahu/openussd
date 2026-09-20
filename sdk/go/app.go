package openussd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/davidrukahu/openussd/canonical"
)

// Context is what a screen sees for one turn of a dialogue.
//
// S is the application's own state type. It is decoded from the gateway's
// stored blob before the screen runs and re-encoded afterwards, so screens
// work with a typed value rather than a map of strings.
type Context[S any] struct {
	// Event is the canonical event for this turn.
	Event canonical.Event
	// Turn counts screens served in this session, starting at 1.
	Turn int
	// State is the application's state, mutable in place.
	State *S
	// Lang is the resolved language for this session.
	Lang string

	notice string
	ctx    context.Context
	bundle *Bundle
}

// Notice is the message to show above this screen, set when the previous
// turn returned Stay: a validation error, usually.
//
// A screen that builds a menu should fold it into the title it passes to
// Menu or MenuFit, so the notice is measured as part of the screen. A
// screen that ignores it still gets it prepended, but then the fit was
// calculated without it and a long menu can lose its last lines.
func (c *Context[S]) Notice() string { return c.notice }

// Ctx returns the context for this turn, carrying the gateway's deadline.
//
// Screens that call out to another service should pass it along. The
// gateway abandons a turn after its per-tenant timeout because the handset
// is already gone by then; an outbound call that ignores the cancellation
// keeps running and holding a connection for a dialogue nobody is reading.
func (c *Context[S]) Ctx() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// MSISDN returns the subscriber number claimed by the network.
//
// This is a claim, not an authenticated identity: anything that can reach
// the gateway's inbound endpoint can assert any number. Treat it as a
// routing hint. Gate anything that matters behind a PIN or an OTP.
func (c *Context[S]) MSISDN() string { return c.Event.MSISDN }

// T translates a key in the session's language.
func (c *Context[S]) T(key string) string { return c.bundle.T(c.Lang, key) }

// Action is what a screen decides to do with the user's input.
type Action struct {
	// Next is the screen to show. Empty means stay on the current screen.
	Next string
	// Text overrides the next screen's prompt. Used for a terminating
	// message, or for a validation error rendered above a re-prompt.
	Text string
	// End closes the dialogue after showing Text.
	End bool
}

// Goto moves to another screen.
func Goto(screen string) (Action, error) { return Action{Next: screen}, nil }

// Stay re-renders the current screen with a message above it. This is the
// invalid-input path: the user sees what went wrong without losing their
// place.
func Stay(message string) (Action, error) { return Action{Text: message}, nil }

// Finish ends the dialogue with a final message.
func Finish(message string) (Action, error) { return Action{Text: message, End: true}, nil }

// Screen is one step of a dialogue.
type Screen[S any] struct {
	// Name identifies the screen in transitions and stored state.
	Name string
	// Prompt renders what the user sees. Required.
	Prompt func(c *Context[S]) (string, error)
	// Handle processes the user's reply and chooses what happens next. A
	// screen with no Handle is terminal: the dialogue ends after Prompt.
	Handle func(c *Context[S], input string) (Action, error)
}

// App is a USSD application: a set of screens and the state they share.
type App[S any] struct {
	screens map[string]Screen[S]
	start   string
	bundle  *Bundle
	log     *slog.Logger
	// DefaultLang is used when a session has no language preference.
	DefaultLang string
}

// NewApp builds an application whose first screen is start.
func NewApp[S any](start string, screens ...Screen[S]) (*App[S], error) {
	app := &App[S]{
		screens:     make(map[string]Screen[S], len(screens)),
		start:       start,
		DefaultLang: DefaultLang,
	}

	for _, s := range screens {
		switch {
		case s.Name == "":
			return nil, fmt.Errorf("openussd: a screen has no name")
		case s.Prompt == nil:
			return nil, fmt.Errorf("openussd: screen %q has no Prompt", s.Name)
		}
		if _, dup := app.screens[s.Name]; dup {
			return nil, fmt.Errorf("openussd: screen %q is declared twice", s.Name)
		}
		app.screens[s.Name] = s
	}

	if _, ok := app.screens[start]; !ok {
		return nil, fmt.Errorf("openussd: start screen %q is not declared", start)
	}

	// Transitions are not checked here. A Handle returns its target at
	// runtime, often computed, so there is nothing static to walk; Turn
	// reports an unknown target when one is actually taken.
	return app, nil
}

// WithLogger attaches a logger. Without one the SDK logs to slog.Default.
func (a *App[S]) WithLogger(log *slog.Logger) *App[S] {
	a.log = log
	return a
}

func (a *App[S]) logger() *slog.Logger {
	if a.log == nil {
		return slog.Default()
	}
	return a.log
}

// WithBundle attaches translations.
func (a *App[S]) WithBundle(b *Bundle) *App[S] {
	a.bundle = b
	return a
}

// state is what the SDK stores in the gateway's opaque blob: the screen the
// user is on, plus the application's own state.
type state[S any] struct {
	Screen string `json:"screen"`
	Lang   string `json:"lang,omitempty"`
	App    S      `json:"app"`
}

// Turn runs one turn of a dialogue.
//
// It takes the event, the turn number, and the state blob the gateway
// stored last turn, and returns the screen to render plus the state to
// store. It is deliberately independent of HTTP so an application can be
// tested by calling it directly.
//
// The context reaches screens through Context.Ctx, so an outbound call a
// screen makes is cancelled with the turn.
func (a *App[S]) Turn(ctx context.Context, ev canonical.Event, turn int, stored json.RawMessage) (canonical.Response, json.RawMessage, error) {
	var st state[S]
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &st); err != nil {
			// Corrupt state restarts the dialogue rather than killing it:
			// the user gets the opening menu, not a dead shortcode. It is
			// logged because the usual cause is a state-schema change
			// meeting in-flight dialogues, and the alternative evidence
			// is user complaints.
			a.logger().Warn("discarding unreadable session state, restarting the dialogue",
				"error", err, "session_id", ev.SessionID, "turn", turn)
			st = state[S]{}
		}
	}
	if st.Screen == "" {
		st.Screen = a.start
	}
	if st.Lang == "" {
		st.Lang = a.DefaultLang
	}

	current, ok := a.screens[st.Screen]
	if !ok {
		return canonical.Response{}, nil, fmt.Errorf("openussd: stored screen %q no longer exists", st.Screen)
	}

	tctx := &Context[S]{Event: ev, Turn: turn, State: &st.App, Lang: st.Lang, ctx: ctx, bundle: a.bundle}

	// The first turn has no input to handle: render the start screen.
	if ev.Phase == canonical.PhaseBegin || len(ev.Path) == 0 {
		return a.render(tctx, current, "", &st)
	}

	if current.Handle == nil {
		// A terminal screen received input. The dialogue is over; say so
		// rather than silently re-rendering.
		return canonical.End(tctx.T("session.ended")), nil, nil
	}

	action, err := current.Handle(tctx, ev.LastInput())
	if err != nil {
		return canonical.Response{}, nil, err
	}

	if action.End {
		body := action.Text
		if body == "" {
			body = tctx.T("session.ended")
		}
		return canonical.End(Shrink(body)), nil, nil
	}

	next := current
	if action.Next != "" {
		next, ok = a.screens[action.Next]
		if !ok {
			return canonical.Response{}, nil, fmt.Errorf("openussd: screen %q transitions to unknown screen %q", current.Name, action.Next)
		}
		st.Screen = next.Name
	}
	return a.render(tctx, next, action.Text, &st)
}

// render produces a screen, optionally prefixed with a message, and encodes
// the state to store alongside it.
func (a *App[S]) render(ctx *Context[S], screen Screen[S], prefix string, st *state[S]) (canonical.Response, json.RawMessage, error) {
	ctx.notice = prefix
	body, err := screen.Prompt(ctx)
	if err != nil {
		return canonical.Response{}, nil, fmt.Errorf("openussd: rendering screen %q: %w", screen.Name, err)
	}

	// A screen that read Notice has already placed it, and measured the
	// rest of the screen around it. Only prepend for screens that did not.
	if prefix != "" && !strings.HasPrefix(body, prefix) {
		body = prefix + "\n" + body
	}

	// Enforce the budget here rather than trusting each screen, and
	// measure it in the encoding the text itself forces: a screen of emoji
	// holds 70 units, not 182 characters. A gateway rejecting an over-long
	// screen shows the user an error; shrinking it shows them most of
	// their menu.
	body = Shrink(body)

	st.Screen = screen.Name
	st.Lang = ctx.Lang
	encoded, err := json.Marshal(st)
	if err != nil {
		return canonical.Response{}, nil, fmt.Errorf("openussd: encoding state: %w", err)
	}

	// A screen with no Handle cannot take input, so the dialogue ends with
	// it displayed.
	if screen.Handle == nil {
		return canonical.End(body), nil, nil
	}
	return canonical.Continue(body), encoded, nil
}

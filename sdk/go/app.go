package openussd

import (
	"encoding/json"
	"fmt"

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

	bundle *Bundle
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

	// Catch dangling transitions at construction rather than when a user
	// walks into one. Only static targets are checkable; a Handle that
	// computes a name is on its own.
	return app, nil
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
func (a *App[S]) Turn(ev canonical.Event, turn int, stored json.RawMessage) (canonical.Response, json.RawMessage, error) {
	var st state[S]
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &st); err != nil {
			// Corrupt state restarts the dialogue rather than killing it:
			// the user gets the opening menu, not a dead shortcode.
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

	ctx := &Context[S]{Event: ev, Turn: turn, State: &st.App, Lang: st.Lang, bundle: a.bundle}

	// The first turn has no input to handle: render the start screen.
	if ev.Phase == canonical.PhaseBegin || len(ev.Path) == 0 {
		return a.render(ctx, current, "", &st)
	}

	if current.Handle == nil {
		// A terminal screen received input. The dialogue is over; say so
		// rather than silently re-rendering.
		return canonical.End(ctx.T("session.ended")), nil, nil
	}

	action, err := current.Handle(ctx, ev.LastInput())
	if err != nil {
		return canonical.Response{}, nil, err
	}

	if action.End {
		body := action.Text
		if body == "" {
			body = ctx.T("session.ended")
		}
		return canonical.End(Truncate(body, MaxScreen)), nil, nil
	}

	next := current
	if action.Next != "" {
		next, ok = a.screens[action.Next]
		if !ok {
			return canonical.Response{}, nil, fmt.Errorf("openussd: screen %q transitions to unknown screen %q", current.Name, action.Next)
		}
		st.Screen = next.Name
	}
	return a.render(ctx, next, action.Text, &st)
}

// render produces a screen, optionally prefixed with a message, and encodes
// the state to store alongside it.
func (a *App[S]) render(ctx *Context[S], screen Screen[S], prefix string, st *state[S]) (canonical.Response, json.RawMessage, error) {
	body, err := screen.Prompt(ctx)
	if err != nil {
		return canonical.Response{}, nil, fmt.Errorf("openussd: rendering screen %q: %w", screen.Name, err)
	}
	if prefix != "" {
		body = prefix + "\n" + body
	}

	// Enforce the budget here rather than trusting each screen. A gateway
	// rejecting an over-long screen shows the user an error; truncating it
	// shows them most of their menu.
	body = Truncate(body, MaxScreen)

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

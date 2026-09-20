package openussd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidrukahu/openussd/canonical"
)

// pinState is a small application state: enough to prove typed state
// survives the round trip through the gateway's opaque blob.
type pinState struct {
	Attempts int    `json:"attempts"`
	Name     string `json:"name"`
}

// pinApp is the kind of application the SDK is for: a menu, a validated
// input, and a terminal screen, in about fifty lines.
func pinApp(t *testing.T) *App[pinState] {
	t.Helper()

	app, err := NewApp[pinState]("menu",
		Screen[pinState]{
			Name: "menu",
			Prompt: func(c *Context[pinState]) (string, error) {
				return Menu("Welcome", []MenuItem{{Key: "1", Label: "Set name"}, {Key: "2", Label: "Quit"}}), nil
			},
			Handle: func(c *Context[pinState], input string) (Action, error) {
				switch input {
				case "1":
					return Goto("name")
				case "2":
					return Finish("Goodbye.")
				default:
					c.State.Attempts++
					return Stay("Invalid choice.")
				}
			},
		},
		Screen[pinState]{
			Name:   "name",
			Prompt: func(c *Context[pinState]) (string, error) { return "Enter your name:", nil },
			Handle: func(c *Context[pinState], input string) (Action, error) {
				if strings.TrimSpace(input) == "" {
					return Stay("Name cannot be empty.")
				}
				c.State.Name = input
				return Goto("done")
			},
		},
		Screen[pinState]{
			Name: "done",
			Prompt: func(c *Context[pinState]) (string, error) {
				return "Saved, " + c.State.Name + ". Attempts: " + itoa(c.State.Attempts), nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return app
}

// driver replays a dialogue against an app the way the gateway would.
type driver struct {
	t     *testing.T
	app   *App[pinState]
	state json.RawMessage
	path  []string
	turn  int
}

func (d *driver) send(input string) canonical.Response {
	d.t.Helper()
	d.turn++

	phase := canonical.PhaseBegin
	if d.turn > 1 {
		phase = canonical.PhaseContinue
		d.path = append(d.path, input)
	}

	resp, state, err := d.app.Turn(canonical.Event{
		MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
		Shortcode: "*384*1234#", Path: d.path, Phase: phase,
	}, d.turn, d.state)
	if err != nil {
		d.t.Fatalf("turn %d: %v", d.turn, err)
	}
	if !Fits(resp.Body) {
		d.t.Errorf("turn %d produced a %d-rune screen", d.turn, len([]rune(resp.Body)))
	}

	d.state = state
	return resp
}

func TestDialogueHappyPath(t *testing.T) {
	d := &driver{t: t, app: pinApp(t)}

	if got := d.send(""); !strings.Contains(got.Body, "1. Set name") {
		t.Fatalf("opening screen = %q", got.Body)
	}
	if got := d.send("1"); got.Body != "Enter your name:" {
		t.Fatalf("second screen = %q", got.Body)
	}

	got := d.send("Wanjiru")
	if !got.EndSession {
		t.Error("terminal screen did not end the session")
	}
	if !strings.Contains(got.Body, "Saved, Wanjiru") {
		t.Errorf("final screen = %q, want the stored name", got.Body)
	}
}

// TestInvalidInputRedisplaysScreen is the behaviour the architecture
// promises: a bad entry re-renders the same screen with an error, rather
// than advancing or dropping the dialogue.
func TestInvalidInputRedisplaysScreen(t *testing.T) {
	d := &driver{t: t, app: pinApp(t)}
	d.send("")

	got := d.send("7")
	if got.EndSession {
		t.Fatal("invalid input ended the session")
	}
	if !strings.HasPrefix(got.Body, "Invalid choice.") {
		t.Errorf("screen = %q, want the error above the re-prompt", got.Body)
	}
	if !strings.Contains(got.Body, "1. Set name") {
		t.Errorf("screen = %q, want the original menu still shown", got.Body)
	}

	// And the dialogue is still usable afterwards.
	if next := d.send("1"); next.Body != "Enter your name:" {
		t.Errorf("screen after recovery = %q", next.Body)
	}
}

func TestStateAccumulatesAcrossTurns(t *testing.T) {
	d := &driver{t: t, app: pinApp(t)}
	d.send("")
	d.send("7")
	d.send("8")
	d.send("1")

	got := d.send("Amina")
	if !strings.Contains(got.Body, "Attempts: 2") {
		t.Errorf("final screen = %q, want two recorded invalid attempts", got.Body)
	}
}

func TestEndingClearsState(t *testing.T) {
	d := &driver{t: t, app: pinApp(t)}
	d.send("")

	got := d.send("2")
	if !got.EndSession || got.Body != "Goodbye." {
		t.Fatalf("response = %+v", got)
	}
	if d.state != nil {
		t.Errorf("state = %q, want nil so the gateway drops the session", d.state)
	}
}

func TestCorruptStateRestartsRatherThanFails(t *testing.T) {
	app := pinApp(t)

	resp, _, err := app.Turn(canonical.Event{
		MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
		Path: []string{"1"}, Phase: canonical.PhaseContinue,
	}, 2, json.RawMessage(`{"screen":`))
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if resp.Body != "Enter your name:" {
		t.Errorf("screen = %q, want the dialogue restarted from the start screen", resp.Body)
	}
}

func TestNewAppRejectsBadDeclarations(t *testing.T) {
	prompt := func(*Context[pinState]) (string, error) { return "x", nil }

	tests := []struct {
		name    string
		start   string
		screens []Screen[pinState]
		wantErr string
	}{
		{"no name", "a", []Screen[pinState]{{Prompt: prompt}}, "no name"},
		{"no prompt", "a", []Screen[pinState]{{Name: "a"}}, "no Prompt"},
		{"duplicate", "a", []Screen[pinState]{{Name: "a", Prompt: prompt}, {Name: "a", Prompt: prompt}}, "declared twice"},
		{"unknown start", "z", []Screen[pinState]{{Name: "a", Prompt: prompt}}, "not declared"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewApp(tc.start, tc.screens...)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestUnknownTransitionIsAnError(t *testing.T) {
	app, err := NewApp[pinState]("only", Screen[pinState]{
		Name:   "only",
		Prompt: func(*Context[pinState]) (string, error) { return "hi", nil },
		Handle: func(*Context[pinState], string) (Action, error) { return Goto("nowhere") },
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	_, _, err = app.Turn(canonical.Event{Path: []string{"1"}, Phase: canonical.PhaseContinue}, 2, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown screen") {
		t.Fatalf("err = %v, want an unknown-screen error", err)
	}
}

func TestTranslationsResolveWithFallback(t *testing.T) {
	b := NewBundle(map[string]map[string]string{
		"en": {"menu.title": "Welcome", "session.ended": "Goodbye."},
		"sw": {"menu.title": "Karibu"},
	})

	if got := b.T("sw", "menu.title"); got != "Karibu" {
		t.Errorf("sw menu.title = %q", got)
	}
	if got := b.T("sw", "session.ended"); got != "Goodbye." {
		t.Errorf("missing sw key = %q, want the English fallback", got)
	}
	if got := b.T("fr", "menu.title"); got != "Welcome" {
		t.Errorf("unknown language = %q, want the fallback", got)
	}
	if got := b.T("en", "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q, want the key itself", got)
	}
}

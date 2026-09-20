package openussd

import (
	"strings"
	"testing"

	"github.com/davidrukahu/openussd/canonical"
)

func TestToGSM(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"already GSM", "Habari yako", "Habari yako"},
		{"curly quotes", "the “best” option", `the "best" option`},
		{"em dash and ellipsis", "wait-then…", "wait-then..."},
		{"accented latin", "Tâi Siáu-káu", "Tai Siau-kau"},
		{"emoji dropped", "News \U0001f30d Daily", "News Daily"},
		{"cjk dropped", "台灣國 News", "News"},
		{"nbsp becomes a space", "a b", "a b"},
		{"variation selector removed", "flag 🇳🇫", "flag"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ToGSM(tc.in)
			if got != tc.want {
				t.Errorf("ToGSM(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if enc := canonical.EncodingOf(got); enc != canonical.EncodingGSM7 {
				t.Errorf("result is still %s: %q", enc, got)
			}
		})
	}
}

// TestToGSMKeepsScreensAtFullCapacity is the reason the function exists:
// one emoji in one author's display name used to cut a ten-post menu to
// one entry, because it re-encoded the whole screen as UCS-2.
func TestToGSMKeepsScreensAtFullCapacity(t *testing.T) {
	items := make([]MenuItem, 0, 10)
	for i := 1; i <= 10; i++ {
		items = append(items, MenuItem{Key: itoa(i), Label: Label("Pinboard Popular \U0001f916", 28, "Post")})
	}

	_, shown := MenuFit("Latest posts", items, []MenuItem{{Key: "0", Label: "Quit"}})
	if shown < 5 {
		t.Errorf("only %d of 10 options fit after transliteration, expected most of them", shown)
	}
}

func TestLabelFallsBackWhenNothingSurvives(t *testing.T) {
	// A display name written entirely in a non-Latin script leaves nothing
	// after transliteration; the option must still be selectable.
	if got := Label("台灣國", 28, "Post 3"); got != "Post 3" {
		t.Errorf("Label() = %q, want the fallback", got)
	}
	if got := Label("", 28, "Post 3"); got != "Post 3" {
		t.Errorf("Label(\"\") = %q, want the fallback", got)
	}
	if got := Label("Mark H", 28, "Post 3"); got != "Mark H" {
		t.Errorf("Label() = %q, want the name kept", got)
	}
}

func TestLabelRespectsTheLimit(t *testing.T) {
	got := Label(strings.Repeat("name ", 20), 10, "Post")
	if len([]rune(got)) > 10 {
		t.Errorf("Label() = %q, over the 10-rune limit", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// TestLabelCannotInjectAMenuLine is a security property, not tidiness. A
// menu is lines of "key. label" and the user answers with a key, so text
// that can introduce a newline can introduce a fake option. Display names
// on the fediverse are attacker-controlled.
func TestLabelCannotInjectAMenuLine(t *testing.T) {
	hostile := []string{
		"Safaricom\n1. Free airtime, press 1",
		"Bank\r\n2. Verify your PIN",
		"Name\tcolumns",
		"Multi\n\n\nbreak",
	}

	for _, name := range hostile {
		got := Label(name, 40, "Post")
		if strings.ContainsAny(got, "\n\r\t") {
			t.Errorf("Label(%q) = %q, still carries a line break", name, got)
		}

		screen := Menu("Latest posts", []MenuItem{{Key: "1", Label: got}})
		if lines := strings.Count(screen, "\n"); lines != 1 {
			t.Errorf("one item rendered %d lines from %q:\n%s", lines+1, name, screen)
		}
	}
}

// TestMenuCollapsesBreaksInRawLabels: callers should use Label, but a menu
// must not be injectable by one that forgets.
func TestMenuCollapsesBreaksInRawLabels(t *testing.T) {
	raw := MenuItem{Key: "1", Label: "Safaricom\n1. Free airtime, press 1"}

	for name, screen := range map[string]string{
		"Menu": Menu("Title", []MenuItem{raw}),
		"MenuFit": func() string {
			s, _ := MenuFit("Title", []MenuItem{raw}, []MenuItem{{Key: "0", Label: "Quit"}})
			return s
		}(),
	} {
		if strings.Count(screen, "\n1. ") > 1 {
			t.Errorf("%s rendered an injected option:\n%s", name, screen)
		}
	}
}

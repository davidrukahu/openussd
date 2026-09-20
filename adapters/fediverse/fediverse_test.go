package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidrukahu/openussd/canonical"
	openussd "github.com/davidrukahu/openussd/sdk/go"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "hello", "hello"},
		// Paragraphs collapse to a single newline rather than a blank line:
		// a blank line costs 1 of 182 characters and buys nothing.
		{"paragraphs become newlines", "<p>one</p><p>two</p>", "one\ntwo"},
		{"breaks become newlines", "a<br />b", "a\nb"},
		{"entities are decoded", "caf&#233; &amp; bar", "café & bar"},
		{
			name: "mentions and hashtags survive",
			in:   `<p>hi <span class="h-card"><a href="https://m.s/@alice">@<span>alice</span></a></span> <a href="https://m.s/tags/ussd">#<span>ussd</span></a></p>`,
			want: "hi @alice #ussd",
		},
		{"tags with attributes are removed", `<a href="x" rel="nofollow">link</a>`, "link"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripHTML(tc.in); got != tc.want {
				t.Errorf("stripHTML() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToPostRendersAttachmentsAsAltText(t *testing.T) {
	var s apiStatus
	s.Content = "<p>Look at this</p>"
	s.Account.DisplayName = "Wanjiru"
	s.MediaAttachments = []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}{
		{Type: "image", Description: "A matatu in Nairobi traffic"},
		{Type: "image", Description: ""},
	}

	got := toPost(s)
	if !strings.Contains(got.Text, "[image: A matatu in Nairobi traffic]") {
		t.Errorf("text = %q, want the alt text rendered", got.Text)
	}
	if !strings.Contains(got.Text, "[image: no description]") {
		t.Errorf("text = %q, want a placeholder for the undescribed image", got.Text)
	}
}

func TestToPostUnwrapsBoosts(t *testing.T) {
	inner := &apiStatus{Content: "<p>original words</p>"}
	inner.Account.Acct = "alice@example.social"

	var outer apiStatus
	outer.Reblog = inner
	outer.Account.DisplayName = "Booster"

	got := toPost(outer)
	if !strings.Contains(got.Text, "original words") {
		t.Errorf("text = %q, want the boosted content", got.Text)
	}
	if !strings.HasPrefix(got.Text, "RT @alice@example.social:") {
		t.Errorf("text = %q, want the boost attributed", got.Text)
	}
}

func TestToPostFallsBackToHandle(t *testing.T) {
	var s apiStatus
	s.Content = "<p>hi</p>"
	s.Account.Acct = "alice@example.social"

	if got := toPost(s).Author; got != "alice@example.social" {
		t.Errorf("author = %q, want the handle when no display name is set", got)
	}
}

// fakeInstance serves a fixed timeline so the dialogue test does not depend
// on a live Mastodon server.
func fakeInstance(t *testing.T, statuses []apiStatus) *Mastodon {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/timelines/public") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(statuses)
	}))
	t.Cleanup(srv.Close)
	return &Mastodon{Instance: srv.URL, Client: srv.Client()}
}

func longStatus(author, body string) apiStatus {
	var s apiStatus
	s.Content = "<p>" + body + "</p>"
	s.Account.DisplayName = author
	s.CreatedAt = "2026-09-20T10:00:00.000Z"
	return s
}

// TestDialogueBrowsesAndPaginates walks the demo the way the acceptance
// test in the README does, without a network or a gateway.
func TestDialogueBrowsesAndPaginates(t *testing.T) {
	client := fakeInstance(t, []apiStatus{
		longStatus("Wanjiru", strings.Repeat("A long post about USSD and the fediverse. ", 12)),
		longStatus("Otieno", "Short one."),
	})

	app, err := newApp(client, nil)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	var stored json.RawMessage
	turn := 0
	var path []string

	send := func(input string) canonical.Response {
		t.Helper()
		turn++
		phase := canonical.PhaseBegin
		if turn > 1 {
			phase = canonical.PhaseContinue
			path = append(path, input)
		}

		resp, state, err := app.Turn(canonical.Event{
			MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
			Shortcode: "*384*1234#", Path: path, Phase: phase,
		}, turn, stored)
		if err != nil {
			t.Fatalf("turn %d (%q): %v", turn, input, err)
		}
		if !openussd.Fits(resp.Body) {
			t.Errorf("turn %d produced a %d-rune screen:\n%s", turn, len([]rune(resp.Body)), resp.Body)
		}
		stored = state
		return resp
	}

	if got := send(""); !strings.Contains(got.Body, "1. Public timeline") {
		t.Fatalf("opening menu = %q", got.Body)
	}

	got := send("1")
	if !strings.Contains(got.Body, "1. Wanjiru") || !strings.Contains(got.Body, "2. Otieno") {
		t.Fatalf("timeline = %q, want both authors listed", got.Body)
	}

	first := send("1")
	if !strings.Contains(first.Body, "Wanjiru (1/") {
		t.Fatalf("post screen = %q, want a page counter", first.Body)
	}
	if !strings.Contains(first.Body, "1. Next") {
		t.Fatalf("post screen = %q, want a Next control on page one", first.Body)
	}
	if strings.Contains(first.Body, "2. Prev") {
		t.Error("page one offered Prev, which leads nowhere")
	}

	second := send("1")
	if !strings.Contains(second.Body, "(2/") || !strings.Contains(second.Body, "2. Prev") {
		t.Fatalf("second page = %q", second.Body)
	}

	back := send("0")
	if !strings.Contains(back.Body, "1. Wanjiru") {
		t.Fatalf("back = %q, want the timeline again", back.Body)
	}

	short := send("2")
	if strings.Contains(short.Body, "1. Next") {
		t.Errorf("a short post offered Next: %q", short.Body)
	}
	if !strings.Contains(short.Body, "Short one.") {
		t.Errorf("short post = %q", short.Body)
	}
}

func TestUnreachableInstanceKeepsTheDialogueAlive(t *testing.T) {
	app, err := newApp(&Mastodon{Instance: "http://127.0.0.1:1"}, nil)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	_, stored, err := app.Turn(canonical.Event{
		MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344", Phase: canonical.PhaseBegin,
	}, 1, nil)
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}

	resp, _, err := app.Turn(canonical.Event{
		MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
		Path: []string{"1"}, Phase: canonical.PhaseContinue,
	}, 2, stored)
	if err != nil {
		t.Fatalf("turn after a failed fetch: %v", err)
	}
	if resp.EndSession {
		t.Error("a failed fetch ended the dialogue")
	}
	if !strings.Contains(resp.Body, "Could not reach the instance") {
		t.Errorf("screen = %q, want an explanation and the menu", resp.Body)
	}
}

// TestUnofferedNavigationKeyIsRejected: a post with one page offers no
// Next, so pressing 1 must be told it is invalid rather than silently
// redrawing the same screen, which reads as a dropped keypress.
func TestUnofferedNavigationKeyIsRejected(t *testing.T) {
	client := fakeInstance(t, []apiStatus{longStatus("Otieno", "Short one.")})

	app, err := newApp(client, nil)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	var stored json.RawMessage
	turn := 0
	var path []string
	send := func(input string) canonical.Response {
		t.Helper()
		turn++
		phase := canonical.PhaseBegin
		if turn > 1 {
			phase = canonical.PhaseContinue
			path = append(path, input)
		}
		resp, state, err := app.Turn(canonical.Event{
			MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344", Path: path, Phase: phase,
		}, turn, stored)
		if err != nil {
			t.Fatalf("turn %d: %v", turn, err)
		}
		stored = state
		return resp
	}

	send("")
	send("1")
	send("1")

	got := send("1")
	if !strings.HasPrefix(got.Body, "Invalid choice.") {
		t.Errorf("screen = %q, want the invalid-choice message", got.Body)
	}
}

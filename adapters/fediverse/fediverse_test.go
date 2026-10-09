package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

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

		resp, state, err := app.Turn(context.Background(), canonical.Event{
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

	_, stored, err := app.Turn(context.Background(), canonical.Event{
		MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344", Phase: canonical.PhaseBegin,
	}, 1, nil)
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}

	resp, _, err := app.Turn(context.Background(), canonical.Event{
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
		resp, state, err := app.Turn(context.Background(), canonical.Event{
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

// TestPostTextIsCapped: the whole timeline rides in session state, which
// crosses the tenant webhook on every turn and is rejected above 16KB. Ten
// unbounded posts clear that on their own.
func TestPostTextIsCapped(t *testing.T) {
	var long apiStatus
	long.Content = "<p>" + strings.Repeat("台灣國語文字", 500) + "</p>"
	long.Account.DisplayName = "Verbose"

	got := toPost(long)
	if len(got.Text) > maxPostBytes {
		t.Errorf("post text is %d bytes, over the %d cap", len(got.Text), maxPostBytes)
	}
	if !strings.HasSuffix(got.Text, "...") {
		t.Errorf("a capped post should show it was cut: %q", got.Text[max(0, len(got.Text)-20):])
	}
	if !utf8.ValidString(got.Text) {
		t.Error("capping split a multi-byte character")
	}

	// A whole timeline of capped posts has to fit the gateway's reply cap.
	posts := make([]Post, 0, timelineSize)
	for range timelineSize {
		posts = append(posts, got)
	}
	blob, err := json.Marshal(state{Posts: posts})
	if err != nil {
		t.Fatalf("encoding state: %v", err)
	}
	if len(blob) > 12<<10 {
		t.Errorf("session state is %d bytes, too close to the 16KB reply cap", len(blob))
	}
}

// TestTimelineNamesClosedInstances checks that an instance which refuses
// signed-out reads is reported as such, that other refusals with the same
// statuses (a firewall's 403, an unrelated 422) are not, that an instance's
// own error text is stripped of characters that do not print and cut to a
// loggable length without splitting a character, and that the error never
// repeats credentials from the instance URL.
func TestTimelineNamesClosedInstances(t *testing.T) {
	const closed = `{"error":"This method requires an authenticated user"}`
	tests := []struct {
		name   string
		status int
		body   string
		want   string
		reject string
	}{
		{"closed instance", http.StatusUnprocessableEntity, closed, "requires sign-in", " replied "},
		{"closed instance answering 401", http.StatusUnauthorized, closed, "requires sign-in", " replied "},
		{"firewall block", http.StatusForbidden, "<html>Access denied</html>", "127.0.0.1", "requires sign-in"},
		{"other validation error", http.StatusUnprocessableEntity, `{"error":"Validation failed"}`, "Validation failed", "requires sign-in"},
		{"outage", http.StatusServiceUnavailable, "", "replied 503", "requires sign-in"},
		{"bidi override dropped", http.StatusUnprocessableEntity, "{\"error\":\"a\u202eb\"}", "Entity: ab", "\u202e"},
		// The leading "a" puts the 200-byte cut inside a two-byte character.
		{"long reason", http.StatusUnprocessableEntity, `{"error":"a` + strings.Repeat("é", 150) + `END"}`, "Entity: a" + strings.Repeat("é", 99), "END"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			instance := strings.Replace(srv.URL, "http://", "http://operator:hunter2@", 1)
			client := &Mastodon{Instance: instance, Client: srv.Client()}
			_, err := client.PublicTimeline(context.Background(), 5)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.reject) {
				t.Fatalf("PublicTimeline error = %v, want %q and not %q", err, tc.want, tc.reject)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Fatalf("PublicTimeline error leaks the URL password: %v", err)
			}
			if !utf8.ValidString(err.Error()) {
				t.Fatalf("PublicTimeline error is not valid UTF-8: %q", err)
			}
		})
	}
}

// TestEnvOrTreatsEmptyAsUnset checks that an environment variable which is
// set but empty falls back to the default, as an unset one does. The compose
// file passes FEDIVERSE_INSTANCE through empty when the operator has not set
// it, and the adapter must then use its own default instance.
func TestEnvOrTreatsEmptyAsUnset(t *testing.T) {
	const key = "OPENUSSD_TEST_ENVOR"
	tests := []struct {
		name  string
		unset bool
		value string
		want  string
	}{
		{"unset", true, "", "fallback"},
		{"empty", false, "", "fallback"},
		{"set", false, "https://example.social", "https://example.social"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setenv restores the variable after the subtest, including
			// when the unset case clears it.
			t.Setenv(key, tc.value)
			if tc.unset {
				if err := os.Unsetenv(key); err != nil {
					t.Fatalf("unsetting %s: %v", key, err)
				}
			}
			if got := envOr(key, "fallback"); got != tc.want {
				t.Errorf("envOr(%q) = %q, want %q", key, got, tc.want)
			}
		})
	}
}

// TestTimelineErrorIgnoresRemoteStatusText checks that the status in the
// error is rebuilt from the code, so a server cannot put its own reason
// phrase (bidi overrides, an oversized line) into the operator's log.
func TestTimelineErrorIgnoresRemoteStatusText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 422 Nope \u202e" + strings.Repeat("x", 5000) + "\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		_ = buf.Flush()
	}))
	t.Cleanup(srv.Close)

	client := &Mastodon{Instance: srv.URL, Client: srv.Client()}
	_, err := client.PublicTimeline(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "422 Unprocessable Entity") {
		t.Fatalf("PublicTimeline error = %v, want the standard 422 text", err)
	}
	if strings.Contains(err.Error(), "\u202e") || strings.Contains(err.Error(), "xxxx") {
		t.Fatalf("PublicTimeline error carries the remote reason phrase: %q", err)
	}
}

// TestTimelineErrorNamesRedirectTarget checks that after a redirect the
// error names the host that refused, not the one that was configured.
func TestTimelineErrorNamesRedirectTarget(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusMovedPermanently)
	}))
	t.Cleanup(origin.Close)

	client := &Mastodon{Instance: origin.URL, Client: origin.Client()}
	_, err := client.PublicTimeline(context.Background(), 5)
	targetHost := strings.TrimPrefix(target.URL, "http://")
	if err == nil || !strings.Contains(err.Error(), targetHost+" replied 403") {
		t.Fatalf("PublicTimeline error = %v, want it to name %s", err, targetHost)
	}
}

// stubTransport answers every request with a fixed response, the way a
// recording or caching RoundTripper might, including one that leaves
// Response.Request unset.
type stubTransport struct{ resp *http.Response }

func (s stubTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return s.resp, nil
}

// TestTimelineErrorWithUnusualResponses checks the error line against
// responses a real transport rarely produces: no Request on the response, a
// host carrying a bidi override, and a status code with no standard text.
func TestTimelineErrorWithUnusualResponses(t *testing.T) {
	bidiHost := &http.Request{URL: &url.URL{Scheme: "https", Host: "\u202eevil.example"}}
	tests := []struct {
		name   string
		resp   *http.Response
		want   string
		reject string
	}{
		{"no request on response", &http.Response{StatusCode: http.StatusServiceUnavailable}, "configured.example replied 503 Service Unavailable", "\u202e"},
		{"bidi in redirect host", &http.Response{StatusCode: http.StatusForbidden, Request: bidiHost}, "evil.example replied 403", "\u202e"},
		{"non-standard status", &http.Response{StatusCode: 520}, "replied 520", "520 "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.resp.Body = io.NopCloser(strings.NewReader(""))
			client := &Mastodon{
				Instance: "https://configured.example",
				Client:   &http.Client{Transport: stubTransport{resp: tc.resp}},
			}
			_, err := client.PublicTimeline(context.Background(), 5)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.reject) {
				t.Fatalf("PublicTimeline error = %q, want %q and not %q", err, tc.want, tc.reject)
			}
		})
	}
}

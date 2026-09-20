package africastalking

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/canonical"
)

const fixtureDir = "../../../testdata/fixtures/africastalking"

// fixedClock keeps ReceivedAt out of the comparison.
var fixedClock = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

// TestFixtures replays every recorded request/expectation pair. This is the
// conformance suite RFC-0001 requires of each adapter: the fixtures double as
// documentation of the provider's wire format, so a change that breaks them
// is a change to the contract and should be argued for, not merged quietly.
func TestFixtures(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("reading fixture dir: %v", err)
	}

	var cases int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cases++
		t.Run(entry.Name(), func(t *testing.T) {
			dir := filepath.Join(fixtureDir, entry.Name())
			req, body := readRequestFixture(t, filepath.Join(dir, "request.http"))

			var want canonical.Event
			wantRaw, err := os.ReadFile(filepath.Join(dir, "expected-event.json"))
			if err != nil {
				t.Fatalf("reading expectation: %v", err)
			}
			if err := json.Unmarshal(wantRaw, &want); err != nil {
				t.Fatalf("decoding expectation: %v", err)
			}

			got, err := (&Adapter{Now: fixedClock}).Parse(req)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if !bytes.Equal(got.Raw, body) {
				t.Errorf("Raw not retained verbatim:\n got %q\nwant %q", got.Raw, body)
			}
			if !got.ReceivedAt.Equal(fixedClock()) {
				t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, fixedClock())
			}

			// Compare through the canonical JSON shape so a field added to
			// Event without a matching fixture update is caught here.
			got.Raw, got.ReceivedAt = nil, want.ReceivedAt
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Errorf("event mismatch:\n got %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}

	if cases == 0 {
		t.Fatal("no fixtures found: the contract suite is silently empty")
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name, method, body, wantErr string
	}{
		{"wrong method", http.MethodGet, "", "expected POST"},
		{"no session id", http.MethodPost, "phoneNumber=%2B254711223344&text=", "no session id"},
		{"no msisdn", http.MethodPost, "sessionId=abc&text=", "no MSISDN"},
		{"malformed form", http.MethodPost, "sessionId=%zz", "malformed form body"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/ussd", strings.NewReader(tc.body))
			_, err := New().Parse(req)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseRejectsOversizedBody(t *testing.T) {
	body := "sessionId=abc&phoneNumber=%2B254711223344&text=" + strings.Repeat("9", maxBodyBytes)
	req := httptest.NewRequest(http.MethodPost, "/ussd", strings.NewReader(body))

	if _, err := New().Parse(req); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected a size error, got %v", err)
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		resp canonical.Response
		want string
	}{
		{"continue", canonical.Continue("1. Timeline\n2. Quit"), "CON 1. Timeline\n2. Quit"},
		{"end", canonical.End("Bye."), "END Bye."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := New().Render(rec, tc.resp); err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got := rec.Body.String(); got != tc.want {
				t.Errorf("body = %q, want %q", got, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("Content-Type = %q, want text/plain", ct)
			}
		})
	}
}

// TestRenderRefusesOversizedScreen keeps the 182-character budget a hard
// failure in our logs rather than a silent truncation by the network.
func TestRenderRefusesOversizedScreen(t *testing.T) {
	resp := canonical.Continue(strings.Repeat("x", canonical.MaxBodyLen+1))
	rec := httptest.NewRecorder()

	if err := New().Render(rec, resp); err == nil {
		t.Fatal("expected Render to reject an over-long body")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("rejected response still wrote %q", rec.Body.String())
	}
}

func TestVerifyAlwaysAccepts(t *testing.T) {
	// Documents the weakness deliberately: the provider offers nothing to
	// check, so the allowlist lives in adapter.TrustedProxy instead.
	if err := New().Verify(httptest.NewRequest(http.MethodPost, "/ussd", nil)); err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
}

// readRequestFixture parses a raw HTTP request file into a server-side
// *http.Request, returning the body separately so tests can assert that the
// adapter retained it verbatim.
func readRequestFixture(t *testing.T, path string) (*http.Request, []byte) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	req, err := http.ReadRequest(bufio.NewReader(f))
	if err != nil {
		t.Fatalf("parsing fixture as HTTP: %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading fixture body: %v", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return req, body
}

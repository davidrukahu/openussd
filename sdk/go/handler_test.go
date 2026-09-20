package openussd

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/canonical"
	"github.com/davidrukahu/openussd/webhook"
)

const tenantSecret = "tenant-secret"

func handlerFixture(t *testing.T) *Handler[pinState] {
	t.Helper()
	return NewHandler(pinApp(t), tenantSecret, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func beginBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(webhook.Request{
		Event: canonical.Event{
			MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344",
			Shortcode: "*384*1234#", Phase: canonical.PhaseBegin,
		},
		Turn: 1,
	})
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}
	return body
}

// TestHandlerRejectsRequestsItCannotAttribute is what stops a tenant webhook,
// which is a public URL, being driven by anyone who finds it.
func TestHandlerRejectsRequestsItCannotAttribute(t *testing.T) {
	tests := []struct {
		name string
		sign func(*http.Request, []byte)
	}{
		{"unsigned", func(*http.Request, []byte) {}},
		{
			name: "signed with the wrong secret",
			sign: func(r *http.Request, body []byte) {
				webhook.Sign(r, "guessed-the-url-not-the-secret", time.Now().UTC(), body)
			},
		},
		{
			name: "signed but replayed outside the window",
			sign: func(r *http.Request, body []byte) {
				webhook.Sign(r, tenantSecret, time.Now().UTC().Add(-time.Hour), body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := beginBody(t)
			req := httptest.NewRequest(http.MethodPost, "/ussd", bytes.NewReader(body))
			tc.sign(req, body)

			rec := httptest.NewRecorder()
			handlerFixture(t).ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
}

// TestHandlerRejectsATamperedBody: the signature covers bytes, so changing
// the subscriber after signing must fail.
func TestHandlerRejectsATamperedBody(t *testing.T) {
	body := beginBody(t)
	req := httptest.NewRequest(http.MethodPost, "/ussd", nil)
	webhook.Sign(req, tenantSecret, time.Now().UTC(), body)

	tampered := bytes.Replace(body, []byte("+254711223344"), []byte("+254700000000"), 1)
	req.Body = io.NopCloser(bytes.NewReader(tampered))

	rec := httptest.NewRecorder()
	handlerFixture(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a body changed after signing", rec.Code)
	}
}

func TestHandlerServesASignedRequest(t *testing.T) {
	body := beginBody(t)
	req := httptest.NewRequest(http.MethodPost, "/ussd", bytes.NewReader(body))
	webhook.Sign(req, tenantSecret, time.Now().UTC(), body)

	rec := httptest.NewRecorder()
	handlerFixture(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}

	var reply webhook.Reply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decoding reply: %v", err)
	}
	if reply.Response.Body == "" || reply.Response.EndSession {
		t.Errorf("reply = %+v, want an opening screen", reply.Response)
	}
	if len(reply.State) == 0 {
		t.Error("reply carried no state, so the next turn would start over")
	}
}

func TestHandlerRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   []byte
		signed bool
		want   int
	}{
		{"wrong method", http.MethodGet, nil, false, http.StatusMethodNotAllowed},
		{"oversized body", http.MethodPost, bytes.Repeat([]byte("a"), maxRequestBytes+1), false, http.StatusRequestEntityTooLarge},
		{"signed but not JSON", http.MethodPost, []byte("CON hello"), true, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/ussd", bytes.NewReader(tc.body))
			if tc.signed {
				webhook.Sign(req, tenantSecret, time.Now().UTC(), tc.body)
			}

			rec := httptest.NewRecorder()
			handlerFixture(t).ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// TestHandlerRefusesAnUnknownProtocolVersion: a newer gateway means the
// payload may not mean what this SDK thinks it means, so the turn is refused
// rather than guessed at.
func TestHandlerRefusesAnUnknownProtocolVersion(t *testing.T) {
	body, err := json.Marshal(webhook.Request{
		Version: "2",
		Event: canonical.Event{
			MNO: "simulator", SessionID: "s1", MSISDN: "+254711223344", Phase: canonical.PhaseBegin,
		},
		Turn: 1,
	})
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ussd", bytes.NewReader(body))
	webhook.Sign(req, tenantSecret, time.Now().UTC(), body)

	rec := httptest.NewRecorder()
	handlerFixture(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a version this SDK does not know", rec.Code)
	}
}

// TestHandlerPropagatesTheRequestContext: a screen's outbound call must be
// cancelled when the gateway gives up on the turn.
func TestHandlerPropagatesTheRequestContext(t *testing.T) {
	var seen bool
	app, err := NewApp[pinState]("only", Screen[pinState]{
		Name: "only",
		Prompt: func(c *Context[pinState]) (string, error) {
			seen = c.Ctx() != nil && c.Ctx().Err() == nil
			return "hello", nil
		},
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	body := beginBody(t)
	req := httptest.NewRequest(http.MethodPost, "/ussd", bytes.NewReader(body))
	webhook.Sign(req, tenantSecret, time.Now().UTC(), body)

	rec := httptest.NewRecorder()
	NewHandler(app, tenantSecret, slog.New(slog.NewTextHandler(io.Discard, nil))).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !seen {
		t.Error("the screen received no live context")
	}
}

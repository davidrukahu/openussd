package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/canonical"
	"github.com/davidrukahu/openussd/gateway/internal/adapter"
	"github.com/davidrukahu/openussd/gateway/internal/adapter/africastalking"
	"github.com/davidrukahu/openussd/gateway/internal/adapter/simulator"
	"github.com/davidrukahu/openussd/gateway/internal/session"
	"github.com/davidrukahu/openussd/gateway/internal/tenant"
	"github.com/davidrukahu/openussd/webhook"
)

// dialogue drives a gateway the way a network would: form POSTs with the
// input path joined by "*", reading back CON/END screens.
type dialogue struct {
	t       *testing.T
	handler http.Handler
	session string
	path    []string
}

func (d *dialogue) dial(input string) string {
	d.t.Helper()
	if input != "" {
		d.path = append(d.path, input)
	}

	body := strings.NewReader("sessionId=" + d.session +
		"&serviceCode=*384*1234%23&phoneNumber=%2B254711223344&networkCode=63902" +
		"&text=" + strings.Join(d.path, "*"))

	req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	d.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		d.t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// tenantServer is a minimal stateful tenant: it counts turns in its own
// state blob, proving state survives the round trip through the gateway.
func tenantServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := webhook.Verify(r.Header, "test-secret", body, time.Now().UTC(), webhook.DefaultTolerance); err != nil {
			t.Errorf("tenant could not verify the gateway's signature: %v", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		var in webhook.Request
		if err := json.Unmarshal(body, &in); err != nil {
			t.Errorf("decoding request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var state struct {
			Seen []string `json:"seen"`
		}
		if in.State != nil {
			_ = json.Unmarshal(in.State, &state)
		}
		state.Seen = append(state.Seen, in.Event.LastInput())
		next, _ := json.Marshal(state)

		reply := webhook.Reply{State: next}
		if in.Event.LastInput() == "0" {
			reply.Response = canonical.End("Bye after " + strings.Join(state.Seen, ","))
		} else {
			reply.Response = canonical.Continue("Turn " + strconv.Itoa(in.Turn) + " seen " + strings.Join(state.Seen, ","))
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
}

func newGateway(t *testing.T, webhookURL string, store session.Store) http.Handler {
	t.Helper()

	router, err := tenant.NewRouter([]tenant.Tenant{{
		Name: "demo", Shortcode: "*384*1234#", WebhookURL: webhookURL, Secret: "test-secret",
	}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewInbound(africastalking.New(), store, router, tenant.NewDispatcher(nil), log)
}

// TestDialogueCarriesStateAcrossScreens is the behaviour the whole gateway
// exists for: independent HTTP requests read as one conversation.
func TestDialogueCarriesStateAcrossScreens(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()

	d := &dialogue{t: t, handler: newGateway(t, srv.URL, store), session: "ATUid_state"}

	if got := d.dial(""); !strings.HasPrefix(got, "CON ") {
		t.Fatalf("first screen = %q, want a CON", got)
	}
	if got := d.dial("1"); !strings.Contains(got, "seen ,1") {
		t.Errorf("second screen = %q, want the tenant's accumulated state", got)
	}
	if got := d.dial("2"); !strings.Contains(got, "seen ,1,2") {
		t.Errorf("third screen = %q, want state from both prior turns", got)
	}

	got := d.dial("0")
	if !strings.HasPrefix(got, "END ") {
		t.Errorf("final screen = %q, want an END", got)
	}
	if store.Len() != 0 {
		t.Errorf("session survived an ended dialogue: %d entries left", store.Len())
	}
}

func TestUnroutableShortcodeGetsAUsableScreen(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()

	body := strings.NewReader("sessionId=ATUid_x&serviceCode=*999%23&phoneNumber=%2B254711223344&text=")
	req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking", body)
	rec := httptest.NewRecorder()
	newGateway(t, srv.URL, store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 so the handset shows our message", rec.Code)
	}
	if got := rec.Body.String(); !strings.HasPrefix(got, "END ") {
		t.Errorf("body = %q, want a terminating screen", got)
	}
}

// TestTenantFailureDoesNotLeakDetail: an operator needs the reason, a
// subscriber must not get a stack trace on a 182-character screen.
func TestTenantFailureDoesNotLeakDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "panic: nil map write in billing.go:42", http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()

	d := &dialogue{t: t, handler: newGateway(t, srv.URL, store), session: "ATUid_broken"}
	got := d.dial("")

	if got != "END "+userFacingError {
		t.Errorf("screen = %q, want the generic failure message", got)
	}
	if strings.Contains(got, "billing.go") {
		t.Error("gateway leaked tenant internals to the handset")
	}
}

func TestRejectsUntrustedSource(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()

	router, err := tenant.NewRouter([]tenant.Tenant{{
		Name: "demo", Shortcode: "*384*1234#", WebhookURL: srv.URL, Secret: "test-secret",
	}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	allowed, err := adapter.ParsePrefixes([]string{"203.0.113.0/24"})
	if err != nil {
		t.Fatalf("parsing allowlist: %v", err)
	}
	guarded := &adapter.TrustedProxy{Adapter: africastalking.New(), Allowed: allowed}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInbound(guarded, store, router, tenant.NewDispatcher(nil), log)

	req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking",
		strings.NewReader("sessionId=ATUid_1&phoneNumber=%2B254711223344&serviceCode=*384*1234%23&text="))
	req.RemoteAddr = "198.51.100.7:40000"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a source outside the allowlist", rec.Code)
	}
	if store.Len() != 0 {
		t.Error("a rejected request still allocated session state")
	}
}

// TestSessionReuseByAnotherSubscriberStartsFresh: the session id matched but
// the subscriber did not, so the dialogue must not inherit the stored tenant
// or state.
func TestSessionReuseByAnotherSubscriberStartsFresh(t *testing.T) {
	var states []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in webhook.Request
		_ = json.Unmarshal(body, &in)
		states = append(states, string(in.State))
		_ = json.NewEncoder(w).Encode(webhook.Reply{
			Response: canonical.Continue("screen"),
			State:    json.RawMessage(`{"secret":"first subscriber"}`),
		})
	}))
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()
	h := newGateway(t, srv.URL, store)

	post := func(msisdn, text string) {
		t.Helper()
		body := strings.NewReader("sessionId=ATUid_shared&serviceCode=*384*1234%23" +
			"&phoneNumber=" + url.QueryEscape(msisdn) + "&text=" + text)
		req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}

	post("+254711223344", "")
	post("+254700999888", "1")

	if len(states) != 2 {
		t.Fatalf("got %d deliveries, want 2", len(states))
	}
	if states[1] != "" {
		t.Errorf("second subscriber received %q, want no inherited state", states[1])
	}
}

// TestTenantHandoverClearsState: a shared shortcode hands a dialogue from its
// menu tenant to a service tenant. The multi-tenancy promise is that the new
// tenant cannot read what the previous one stored.
func TestTenantHandoverClearsState(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}

	serve := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var in webhook.Request
			_ = json.Unmarshal(body, &in)
			mu.Lock()
			seen[name] = string(in.State)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webhook.Reply{
				Response: canonical.Continue(name),
				State:    json.RawMessage(`{"owner":"` + name + `"}`),
			})
		}))
	}

	menu, service := serve("menu"), serve("service")
	defer menu.Close()
	defer service.Close()

	router, err := tenant.NewRouter([]tenant.Tenant{
		{Name: "menu", Shortcode: "*384*1234#", WebhookURL: menu.URL, Secret: "test-secret"},
		{Name: "service", Shortcode: "*384*1234#", Prefix: []string{"2"}, WebhookURL: service.URL, Secret: "test-secret"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	store := session.NewMemory()
	defer store.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInbound(africastalking.New(), store, router, tenant.NewDispatcher(nil), log)

	d := &dialogue{t: t, handler: h, session: "ATUid_handover"}
	d.dial("")  // the menu tenant answers and stores its own state
	d.dial("2") // the prefix hands the dialogue to the service tenant

	mu.Lock()
	defer mu.Unlock()
	if got := seen["service"]; got != "" {
		t.Errorf("service tenant received %s from the previous tenant, want nothing", got)
	}
}

// TestTerminalPhaseReleasesTheSession: a cancelled or timed-out dialogue has
// no handset left to render to, and must not leave state behind.
func TestTerminalPhaseReleasesTheSession(t *testing.T) {
	for _, phase := range []string{"cancel", "timeout"} {
		t.Run(phase, func(t *testing.T) {
			srv := tenantServer(t)
			defer srv.Close()

			store := session.NewMemory()
			defer store.Close()

			router, err := tenant.NewRouter([]tenant.Tenant{{
				Name: "demo", Shortcode: "*384*1234#", WebhookURL: srv.URL, Secret: "test-secret",
			}})
			if err != nil {
				t.Fatalf("NewRouter: %v", err)
			}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			h := NewInbound(simulator.New(), store, router, tenant.NewDispatcher(nil), log)

			send := func(body string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/ussd/simulator", strings.NewReader(body)))
				return rec
			}

			if rec := send(`{"session_id":"SIM_1","msisdn":"+254711223344","shortcode":"*384*1234#"}`); rec.Code != http.StatusOK || store.Len() != 1 {
				t.Fatalf("setup: status = %d, sessions = %d", rec.Code, store.Len())
			}

			rec := send(`{"session_id":"SIM_1","msisdn":"+254711223344","shortcode":"*384*1234#","path":["1"],"phase":"` + phase + `"}`)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want nothing rendered to an absent handset", rec.Body.String())
			}
			if store.Len() != 0 {
				t.Errorf("%s left %d sessions behind", phase, store.Len())
			}
		})
	}
}

// failSaveStore fails only on Save, to exercise the path where a screen was
// produced but the next turn would have no state to continue from.
type failSaveStore struct {
	session.Store
}

func (failSaveStore) Save(context.Context, session.Session) error {
	return errors.New("session store unavailable")
}

func TestSaveFailureFailsTheDialogue(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	mem := session.NewMemory()
	defer mem.Close()

	d := &dialogue{t: t, handler: newGateway(t, srv.URL, failSaveStore{Store: mem}), session: "ATUid_nosave"}
	if got := d.dial(""); got != "END "+userFacingError {
		t.Errorf("screen = %q, want the generic failure screen rather than a menu the next turn cannot follow", got)
	}
}

func TestMalformedRequestIsRejected(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()

	req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking", strings.NewReader("sessionId=%zz"))
	rec := httptest.NewRecorder()
	newGateway(t, srv.URL, store).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a body the adapter cannot parse", rec.Code)
	}
	if store.Len() != 0 {
		t.Error("an unparseable request still allocated session state")
	}
}

func TestRequestIDCorrelatesATurn(t *testing.T) {
	srv := tenantServer(t)
	defer srv.Close()

	store := session.NewMemory()
	defer store.Close()
	h := newGateway(t, srv.URL, store)

	send := func(header string) string {
		t.Helper()
		body := strings.NewReader("sessionId=ATUid_1&serviceCode=*384*1234%23&phoneNumber=%2B254711223344&text=")
		req := httptest.NewRequest(http.MethodPost, "/ussd/africastalking", body)
		if header != "" {
			req.Header.Set(HeaderRequestID, header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Header().Get(HeaderRequestID)
	}

	if got := send(""); got == "" {
		t.Error("no request id was generated")
	}
	if got := send("abc-123"); got != "abc-123" {
		t.Errorf("request id = %q, want the caller's own id echoed back", got)
	}

	// An id is echoed into log lines, so a caller must not be able to
	// forge entries by embedding newlines in it.
	got := send("abc\ndef\rlevel=ERROR")
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("request id = %q, want control characters stripped", got)
	}
	if len(send(strings.Repeat("x", 500))) > maxRequestIDLen {
		t.Error("an oversized request id was not bounded")
	}
}

package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/canonical"
	"github.com/davidrukahu/openussd/gateway/internal/adapter"
	"github.com/davidrukahu/openussd/gateway/internal/adapter/africastalking"
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

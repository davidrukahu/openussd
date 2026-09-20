package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/canonical"
	"github.com/davidrukahu/openussd/webhook"
)

func tenantFor(name string, prefix ...string) Tenant {
	return Tenant{
		Name:       name,
		Shortcode:  "*384*1234#",
		Prefix:     prefix,
		WebhookURL: "https://" + name + ".example/ussd",
		Secret:     "s3cret-" + name,
	}
}

func TestRouteLongestPrefixWins(t *testing.T) {
	router, err := NewRouter([]Tenant{
		tenantFor("root"),
		tenantFor("banking", "1"),
		tenantFor("savings", "1", "2"),
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	tests := []struct {
		name     string
		path     []string
		want     string
		wantPath []string
	}{
		{"first screen goes to the root tenant", nil, "root", nil},
		{"unmatched selection falls back to root", []string{"9"}, "root", []string{"9"}},
		{"one-segment prefix", []string{"1"}, "banking", []string{}},
		{"longer prefix beats shorter", []string{"1", "2"}, "savings", []string{}},
		{"prefix stripped, remainder kept", []string{"1", "2", "7"}, "savings", []string{"7"}},
		{"sibling under shorter prefix", []string{"1", "3"}, "banking", []string{"3"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, routed, err := router.Route(canonical.Event{Shortcode: "*384*1234#", Path: tc.path})
			if err != nil {
				t.Fatalf("Route: %v", err)
			}
			if got.Name != tc.want {
				t.Errorf("tenant = %q, want %q", got.Name, tc.want)
			}
			if strings.Join(routed.Path, "*") != strings.Join(tc.wantPath, "*") {
				t.Errorf("path = %q, want %q", routed.Path, tc.wantPath)
			}
		})
	}
}

func TestRouteUnknownShortcode(t *testing.T) {
	router, err := NewRouter([]Tenant{tenantFor("root")})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, _, err = router.Route(canonical.Event{Shortcode: "*999#"})
	var noRoute ErrNoRoute
	if !errors.As(err, &noRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

func TestNewRouterRejectsBadConfig(t *testing.T) {
	noSecret := tenantFor("insecure")
	noSecret.Secret = ""

	badScheme := tenantFor("ftp")
	badScheme.WebhookURL = "ftp://example/ussd"

	tests := []struct {
		name    string
		tenants []Tenant
		wantErr string
	}{
		{"no root tenant", []Tenant{tenantFor("banking", "1")}, "want exactly 1"},
		{"two root tenants", []Tenant{tenantFor("a"), tenantFor("b")}, "collides"},
		{"duplicate route", []Tenant{tenantFor("root"), tenantFor("a", "1"), tenantFor("b", "1")}, "collides"},
		{"missing secret", []Tenant{noSecret}, "no webhook secret"},
		{"non-http webhook", []Tenant{badScheme}, "not http(s)"},
		{"empty prefix segment", []Tenant{tenantFor("root"), tenantFor("bad", "")}, "prefix segment 0 is empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRouter(tc.tenants)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestDeliverSignsAndRoundTrips is the contract between the gateway and any
// SDK: the tenant must be able to verify what it received.
func TestDeliverSignsAndRoundTrips(t *testing.T) {
	var gotTenantHeader string
	var verifyErr error

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotTenantHeader = r.Header.Get(webhook.HeaderTenant)
		verifyErr = webhook.Verify(r.Header, "s3cret-demo", body, time.Now().UTC(), webhook.DefaultTolerance)

		var in webhook.Request
		if err := json.Unmarshal(body, &in); err != nil {
			t.Errorf("tenant could not decode event: %v", err)
		}
		if len(in.Event.Raw) != 0 {
			t.Error("gateway forwarded Event.Raw to the tenant")
		}
		if string(in.State) != `{"screen":"menu"}` {
			t.Errorf("tenant state = %q, want it forwarded verbatim", in.State)
		}

		_ = json.NewEncoder(w).Encode(webhook.Reply{
			Response: canonical.Continue("1. Timeline"),
			State:    json.RawMessage(`{"screen":"timeline"}`),
		})
	}))
	defer srv.Close()

	tn := Tenant{Name: "demo", Shortcode: "*384*1234#", WebhookURL: srv.URL, Secret: "s3cret-demo"}
	ev := canonical.Event{
		MNO: "africastalking", SessionID: "ATUid_1", MSISDN: "+254711223344",
		Shortcode: "*384*1234#", Phase: canonical.PhaseBegin, Raw: []byte("sessionId=ATUid_1"),
	}

	reply, err := NewDispatcher(srv.Client()).Deliver(context.Background(), tn, webhook.Request{
		Event: ev,
		Turn:  1,
		State: json.RawMessage(`{"screen":"menu"}`),
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if verifyErr != nil {
		t.Errorf("tenant failed to verify the signature: %v", verifyErr)
	}
	if gotTenantHeader != "demo" {
		t.Errorf("%s = %q, want %q", webhook.HeaderTenant, gotTenantHeader, "demo")
	}
	if reply.Response.Body != "1. Timeline" || reply.Response.EndSession {
		t.Errorf("response = %+v", reply.Response)
	}
	if string(reply.State) != `{"screen":"timeline"}` {
		t.Errorf("state = %q, want the tenant's new state", reply.State)
	}
}

func TestDeliverRejectsBadTenantReplies(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name:    "non-200",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: "replied 500",
		},
		{
			name:    "not json",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("CON hello")) },
			wantErr: "decoding reply",
		},
		{
			name: "over the screen budget",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(webhook.Reply{
					Response: canonical.Continue(strings.Repeat("x", canonical.MaxBodyLen+1)),
				})
			},
			wantErr: "limit is 182",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			tn := Tenant{Name: "demo", Shortcode: "*1#", WebhookURL: srv.URL, Secret: "s"}
			_, err := NewDispatcher(srv.Client()).Deliver(context.Background(), tn, webhook.Request{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestDeliverHonoursTimeout: the handset's dialogue is already gone by the
// time a slow tenant answers, so the gateway must give up on its own.
func TestDeliverHonoursTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer func() { close(release); srv.Close() }()

	tn := Tenant{
		Name: "slow", Shortcode: "*1#", WebhookURL: srv.URL, Secret: "s",
		Timeout: 50 * time.Millisecond,
	}

	start := time.Now()
	_, err := NewDispatcher(srv.Client()).Deliver(context.Background(), tn, webhook.Request{})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %s, expected to give up after ~50ms", elapsed)
	}
}

// TestNewRouterRejectsDuplicateNames: the gateway decides a dialogue has
// moved between tenants by comparing names, and clears session state when it
// has. Two tenants sharing a name would carry one's state into the other.
func TestNewRouterRejectsDuplicateNames(t *testing.T) {
	a := tenantFor("shared")
	b := tenantFor("shared")
	b.Shortcode = "*999#"

	_, err := NewRouter([]Tenant{a, b})
	if err == nil || !strings.Contains(err.Error(), "names must be unique") {
		t.Fatalf("err = %v, want a duplicate-name rejection", err)
	}
}

// TestDeliverAlwaysSendsPathAsAnArray: a nil slice marshals to null, so the
// two ways a turn can carry no input would otherwise reach a tenant as null
// and [] respectively.
func TestDeliverAlwaysSendsPathAsAnArray(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		_ = json.NewEncoder(w).Encode(webhook.Reply{Response: canonical.Continue("ok")})
	}))
	defer srv.Close()

	tn := Tenant{Name: "demo", Shortcode: "*384*1234#", WebhookURL: srv.URL, Secret: "s"}
	d := NewDispatcher(srv.Client())

	// A fresh dialogue, whose Path is nil.
	if _, err := d.Deliver(context.Background(), tn, webhook.Request{
		Event: canonical.Event{MNO: "m", SessionID: "s1", MSISDN: "+254711223344", Phase: canonical.PhaseBegin},
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	// A turn whose routing prefix consumed every segment.
	if _, err := d.Deliver(context.Background(), tn, webhook.Request{
		Event: canonical.Event{MNO: "m", SessionID: "s1", MSISDN: "+254711223344",
			Path: []string{}, Phase: canonical.PhaseContinue},
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	for i, body := range bodies {
		if strings.Contains(body, `"path":null`) {
			t.Errorf("delivery %d sent path as null: %s", i, body)
		}
		if !strings.Contains(body, `"path":[]`) {
			t.Errorf("delivery %d did not send path as an array: %s", i, body)
		}
	}
}

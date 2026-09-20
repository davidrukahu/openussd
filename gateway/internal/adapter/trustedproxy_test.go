package adapter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidrukahu/openussd/canonical"
)

// stub is an adapter that records whether its own Verify ran.
type stub struct {
	name     string
	verifyFn func(*http.Request) error
	verified bool
}

func (s *stub) Name() string { return s.name }
func (s *stub) Verify(r *http.Request) error {
	s.verified = true
	if s.verifyFn != nil {
		return s.verifyFn(r)
	}
	return nil
}
func (s *stub) Parse(*http.Request) (canonical.Event, error)         { return canonical.Event{}, nil }
func (s *stub) Render(http.ResponseWriter, canonical.Response) error { return nil }

func prefixes(t *testing.T, in ...string) []Prefix {
	t.Helper()
	out, err := ParsePrefixes(in)
	if err != nil {
		t.Fatalf("ParsePrefixes(%v): %v", in, err)
	}
	return out
}

func request(remote string, xff string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/ussd/x", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func TestTrustedProxyAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		allowed []string
		wantErr bool
	}{
		{"inside the range", "203.0.113.9:40000", []string{"203.0.113.0/24"}, false},
		{"outside the range", "198.51.100.9:40000", []string{"203.0.113.0/24"}, true},
		{"single host allowed", "203.0.113.9:40000", []string{"203.0.113.9"}, false},
		{"single host mismatch", "203.0.113.10:40000", []string{"203.0.113.9"}, true},
		{"ipv6 inside", "[2001:db8::5]:40000", []string{"2001:db8::/32"}, false},
		{"ipv6 outside", "[2001:db9::5]:40000", []string{"2001:db8::/32"}, true},
		{"ipv4-mapped ipv6 peer", "[::ffff:203.0.113.9]:40000", []string{"203.0.113.0/24"}, false},
		{"unparseable remote", "not-an-address", []string{"203.0.113.0/24"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &stub{name: "x"}
			tp := &TrustedProxy{Adapter: inner, Allowed: prefixes(t, tc.allowed...)}

			err := tp.Verify(request(tc.remote, ""))
			if tc.wantErr {
				if !errors.Is(err, ErrUntrusted) {
					t.Fatalf("err = %v, want ErrUntrusted", err)
				}
				if inner.verified {
					t.Error("rejected request still reached the wrapped adapter")
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !inner.verified {
				t.Error("wrapped adapter's own Verify did not run")
			}
		})
	}
}

// TestTrustedProxyEmptyAllowlistFailsClosed: an operator who forgets the
// allowlist should get no traffic, not all of it.
func TestTrustedProxyEmptyAllowlistFailsClosed(t *testing.T) {
	tp := &TrustedProxy{Adapter: &stub{name: "x"}}
	if err := tp.Verify(request("203.0.113.9:40000", "")); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("err = %v, want ErrUntrusted", err)
	}
}

// TestTrustedProxyForwardedFor covers the header that makes source-address
// trust forgeable when it is handled naively.
func TestTrustedProxyForwardedFor(t *testing.T) {
	allowed := []string{"203.0.113.0/24"}
	forwarders := []string{"10.0.0.0/8"}

	tests := []struct {
		name       string
		remote     string
		xff        string
		forwarders []string
		wantErr    bool
	}{
		{
			name:       "header ignored when no forwarders are configured",
			remote:     "10.0.0.1:40000",
			xff:        "203.0.113.9",
			forwarders: nil,
			wantErr:    true,
		},
		{
			name:       "allowed client behind our proxy",
			remote:     "10.0.0.1:40000",
			xff:        "203.0.113.9",
			forwarders: forwarders,
			wantErr:    false,
		},
		{
			name:       "disallowed client behind our proxy",
			remote:     "10.0.0.1:40000",
			xff:        "198.51.100.9",
			forwarders: forwarders,
			wantErr:    true,
		},
		{
			name:       "forged leading hops are ignored",
			remote:     "10.0.0.1:40000",
			xff:        "203.0.113.9, 198.51.100.9",
			forwarders: forwarders,
			wantErr:    true,
		},
		{
			name:       "our own proxies are skipped from the right",
			remote:     "10.0.0.1:40000",
			xff:        "203.0.113.9, 10.0.0.7",
			forwarders: forwarders,
			wantErr:    false,
		},
		{
			name:       "header from a non-forwarder peer is ignored",
			remote:     "198.51.100.9:40000",
			xff:        "203.0.113.9",
			forwarders: forwarders,
			wantErr:    true,
		},
		{
			name:       "unparseable header falls back to the peer",
			remote:     "10.0.0.1:40000",
			xff:        "garbage",
			forwarders: forwarders,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tp := &TrustedProxy{
				Adapter:    &stub{name: "x"},
				Allowed:    prefixes(t, allowed...),
				Forwarders: prefixes(t, tc.forwarders...),
			}

			err := tp.Verify(request(tc.remote, tc.xff))
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestTrustedProxyDoesNotReplaceInnerVerify: wrapping an adapter that does
// authenticate must add a gate, not swap one for another.
func TestTrustedProxyDoesNotReplaceInnerVerify(t *testing.T) {
	inner := &stub{name: "x", verifyFn: func(*http.Request) error { return ErrUntrusted }}
	tp := &TrustedProxy{Adapter: inner, Allowed: prefixes(t, "203.0.113.0/24")}

	if err := tp.Verify(request("203.0.113.9:40000", "")); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("err = %v, want the wrapped adapter's rejection to stand", err)
	}
}

func TestTrustedProxyKeepsTheWrappedName(t *testing.T) {
	// Wrapping is a deployment concern; changing the name would move the
	// endpoint path and the fixture directory with it.
	tp := &TrustedProxy{Adapter: &stub{name: "africastalking"}}
	if got := tp.Name(); got != "africastalking" {
		t.Errorf("Name() = %q, want the wrapped adapter's name", got)
	}
}

func TestParsePrefixes(t *testing.T) {
	if _, err := ParsePrefixes([]string{"not-a-cidr/24"}); err == nil {
		t.Error("accepted an invalid CIDR")
	}
	if _, err := ParsePrefixes([]string{"999.1.1.1"}); err == nil {
		t.Error("accepted an invalid address")
	}
	got, err := ParsePrefixes([]string{" 203.0.113.0/24 ", "", "198.51.100.7"})
	if err != nil {
		t.Fatalf("ParsePrefixes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d prefixes, want 2 (blank entries skipped)", len(got))
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	if err := reg.Register(&stub{name: "b"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(&stub{name: "a"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(&stub{name: "a"}); err == nil {
		t.Error("registering a duplicate name silently replaced the adapter")
	}
	if err := reg.Register(&stub{name: ""}); err == nil {
		t.Error("registered an adapter with no name")
	}
	if err := reg.Register(nil); err == nil {
		t.Error("registered a nil adapter")
	}

	if _, ok := reg.Lookup("a"); !ok {
		t.Error("Lookup missed a registered adapter")
	}
	if _, ok := reg.Lookup("nope"); ok {
		t.Error("Lookup found an adapter that was never registered")
	}

	names := reg.Names()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("Names() = %v, want them sorted", names)
	}
}

// TestTrustedProxyReadsEveryForwardedForLine: Go keeps repeated header lines
// as separate values and Header.Get returns only the first, so a proxy that
// adds its own line rather than appending would leave the scanned chain
// entirely attacker-supplied.
func TestTrustedProxyReadsEveryForwardedForLine(t *testing.T) {
	tp := &TrustedProxy{
		Adapter:    &stub{name: "x"},
		Allowed:    prefixes(t, "203.0.113.0/24"),
		Forwarders: prefixes(t, "10.0.0.0/8"),
	}

	// The attacker sets the first line; our proxy appends a second one
	// naming the address it actually saw.
	req := request("10.0.0.1:40000", "")
	req.Header.Add("X-Forwarded-For", "203.0.113.9")
	req.Header.Add("X-Forwarded-For", "198.51.100.9")

	if err := tp.Verify(req); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("err = %v, want the last observed hop to decide", err)
	}

	// And the legitimate ordering still passes.
	ok := request("10.0.0.1:40000", "")
	ok.Header.Add("X-Forwarded-For", "198.51.100.9")
	ok.Header.Add("X-Forwarded-For", "203.0.113.9")
	if err := tp.Verify(ok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

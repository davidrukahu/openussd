package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	testSecret = "correct-horse-battery-staple"
	testBody   = []byte(`{"session_id":"ATUid_1","path":["1"]}`)
	testNow    = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
)

func TestSignVerifyRoundTrip(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/ussd", nil)
	Sign(req, testSecret, testNow, testBody)

	if err := Verify(req.Header, testSecret, testBody, testNow, DefaultTolerance); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejects(t *testing.T) {
	sign := func() http.Header {
		req := httptest.NewRequest(http.MethodPost, "/ussd", nil)
		Sign(req, testSecret, testNow, testBody)
		return req.Header
	}

	tests := []struct {
		name   string
		header func() http.Header
		secret string
		body   []byte
		now    time.Time
	}{
		{
			name:   "wrong secret",
			header: sign,
			secret: "not-the-secret",
			body:   testBody,
			now:    testNow,
		},
		{
			name:   "tampered body",
			header: sign,
			secret: testSecret,
			body:   []byte(`{"session_id":"ATUid_1","path":["9"]}`),
			now:    testNow,
		},
		{
			name:   "replayed outside the window",
			header: sign,
			secret: testSecret,
			body:   testBody,
			now:    testNow.Add(6 * time.Minute),
		},
		{
			name:   "timestamp from the future",
			header: sign,
			secret: testSecret,
			body:   testBody,
			now:    testNow.Add(-6 * time.Minute),
		},
		{
			name:   "no secret configured",
			header: sign,
			secret: "",
			body:   testBody,
			now:    testNow,
		},
		{
			name:   "unsigned request",
			header: func() http.Header { return http.Header{} },
			secret: testSecret,
			body:   testBody,
			now:    testNow,
		},
		{
			name: "signature only, no timestamp",
			header: func() http.Header {
				h := sign()
				h.Del(HeaderTimestamp)
				return h
			},
			secret: testSecret,
			body:   testBody,
			now:    testNow,
		},
		{
			name: "timestamp moved after signing",
			header: func() http.Header {
				h := sign()
				h.Set(HeaderTimestamp, strconv.FormatInt(testNow.Add(time.Minute).Unix(), 10))
				return h
			},
			secret: testSecret,
			body:   testBody,
			now:    testNow,
		},
		{
			name: "unknown scheme",
			header: func() http.Header {
				h := sign()
				h.Set(HeaderSignature, "v2=deadbeef")
				return h
			},
			secret: testSecret,
			body:   testBody,
			now:    testNow,
		},
		{
			name: "unparseable timestamp",
			header: func() http.Header {
				h := sign()
				h.Set(HeaderTimestamp, "yesterday")
				return h
			},
			secret: testSecret,
			body:   testBody,
			now:    testNow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Verify(tc.header(), tc.secret, tc.body, tc.now, DefaultTolerance)
			if !errors.Is(err, ErrInvalidSignature) {
				t.Fatalf("err = %v, want ErrInvalidSignature", err)
			}
		})
	}
}

// TestVerifyAcceptsEdgeOfWindow pins the boundary so a future change to the
// comparison cannot silently widen or narrow the replay window.
func TestVerifyAcceptsEdgeOfWindow(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/ussd", nil)
	Sign(req, testSecret, testNow, testBody)

	atEdge := testNow.Add(DefaultTolerance)
	if err := Verify(req.Header, testSecret, testBody, atEdge, DefaultTolerance); err != nil {
		t.Errorf("rejected a request exactly at the tolerance edge: %v", err)
	}

	pastEdge := testNow.Add(DefaultTolerance + time.Second)
	if err := Verify(req.Header, testSecret, testBody, pastEdge, DefaultTolerance); err == nil {
		t.Error("accepted a request one second past the tolerance edge")
	}
}

func TestSignatureIsStable(t *testing.T) {
	first := Signature(testSecret, testNow, testBody)
	if second := Signature(testSecret, testNow, testBody); first != second {
		t.Fatalf("signature is not deterministic: %q then %q", first, second)
	}
	if Signature(testSecret, testNow.Add(time.Second), testBody) == first {
		t.Error("signature does not cover the timestamp")
	}
}

// TestRequestCarriesItsVersion pins the field a tenant keys on to decide
// whether it can parse the payload at all.
func TestRequestCarriesItsVersion(t *testing.T) {
	body, err := json.Marshal(Request{Version: Version, Turn: 1})
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if !strings.Contains(string(body), `"version":"1"`) {
		t.Errorf("request = %s, want an explicit version", body)
	}
}

// TestNormaliseState: an absent key and an explicit null are documented as
// equivalent, but only one of them decodes to a nil RawMessage.
func TestNormaliseState(t *testing.T) {
	var absent, explicit Reply
	if err := json.Unmarshal([]byte(`{"response":{"body":"hi"}}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"response":{"body":"hi"},"state":null}`), &explicit); err != nil {
		t.Fatal(err)
	}

	if explicit.State == nil {
		t.Skip("encoding/json now decodes an explicit null to nil; the guard is redundant")
	}
	if got := NormaliseState(explicit.State); got != nil {
		t.Errorf("explicit null normalised to %q, want nil", got)
	}
	if got := NormaliseState(absent.State); got != nil {
		t.Errorf("absent state normalised to %q, want nil", got)
	}
	if got := NormaliseState(json.RawMessage(`{"screen":"menu"}`)); string(got) != `{"screen":"menu"}` {
		t.Errorf("real state was altered: %q", got)
	}
}

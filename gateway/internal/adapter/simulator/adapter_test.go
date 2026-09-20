package simulator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidrukahu/openussd/canonical"
)

func post(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/ussd/simulator", strings.NewReader(body))
}

func TestParseInfersPhaseFromInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want canonical.Phase
	}{
		{
			name: "no input is a fresh dial",
			body: `{"session_id":"SIM_1","msisdn":"+254711223344","shortcode":"*384*1234#"}`,
			want: canonical.PhaseBegin,
		},
		{
			name: "input continues the session",
			body: `{"session_id":"SIM_1","msisdn":"+254711223344","shortcode":"*384*1234#","path":["1"]}`,
			want: canonical.PhaseContinue,
		},
		{
			name: "an explicit phase is honoured",
			body: `{"session_id":"SIM_1","msisdn":"+254711223344","shortcode":"*384*1234#","path":["1"],"phase":"cancel"}`,
			want: canonical.PhaseCancel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := New().Parse(post(tc.body))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if ev.Phase != tc.want {
				t.Errorf("phase = %q, want %q", ev.Phase, tc.want)
			}
			if ev.MNO != Name {
				t.Errorf("MNO = %q, want %q", ev.MNO, Name)
			}
			if string(ev.Raw) != tc.body {
				t.Errorf("Raw not retained verbatim: %q", ev.Raw)
			}
		})
	}
}

func TestParseRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name, body, wantErr string
	}{
		{"not json", "sessionId=1", "malformed json"},
		{"no session id", `{"msisdn":"+254711223344"}`, "no session id"},
		{"no msisdn", `{"session_id":"SIM_1"}`, "no MSISDN"},
		{
			name:    "begin phase carrying input",
			body:    `{"session_id":"SIM_1","msisdn":"+254711223344","path":["1"],"phase":"begin"}`,
			wantErr: "begin phase carries",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New().Parse(post(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestRenderIsCanonicalJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := New().Render(rec, canonical.End("Goodbye.")); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var got canonical.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding rendered response: %v", err)
	}
	if got.Body != "Goodbye." || !got.EndSession {
		t.Errorf("response = %+v", got)
	}
}

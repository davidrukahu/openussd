// Package simulator implements a telco adapter for the local handset
// simulator in cmd/ussdsim.
//
// It exists so the gateway can be run and demonstrated end to end with no
// telco account, no sandbox registration, and no inbound tunnel - the
// fastest path from `git clone` to a working USSD dialogue, and the one a
// reviewer or a first-time contributor will take.
//
// The wire format is the canonical shape in JSON, so this adapter is also
// the reference for what a well-behaved adapter produces: anything it has to
// do beyond decoding JSON would be a gap in the canonical types.
package simulator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/davidrukahu/openussd/canonical"
)

// Name is the adapter's stable identifier.
const Name = "simulator"

const maxBodyBytes = 64 << 10

// Adapter implements adapter.Adapter for the local simulator.
type Adapter struct {
	// Now is the clock used for Event.ReceivedAt. Nil means time.Now.
	Now func() time.Time
}

// New returns an adapter using the real clock.
func New() *Adapter { return &Adapter{} }

// Name reports the adapter identifier.
func (a *Adapter) Name() string { return Name }

// Verify always succeeds. The simulator endpoint is for local development;
// production deployments must not expose it, which the config validation
// warns about at startup.
func (a *Adapter) Verify(*http.Request) error { return nil }

// request is the simulator's inbound wire shape.
type request struct {
	SessionID string   `json:"session_id"`
	MSISDN    string   `json:"msisdn"`
	Shortcode string   `json:"shortcode"`
	Path      []string `json:"path"`
	// Phase is optional. When empty it is inferred from Path, matching how
	// real aggregators behave, so the simulator exercises the same code
	// path the network would.
	Phase canonical.Phase `json:"phase,omitempty"`
}

// Parse decodes a simulator request into a canonical event.
func (a *Adapter) Parse(r *http.Request) (canonical.Event, error) {
	if r.Method != http.MethodPost {
		return canonical.Event{}, fmt.Errorf("simulator: expected POST, got %s", r.Method)
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return canonical.Event{}, fmt.Errorf("simulator: reading body: %w", err)
	}
	if len(raw) > maxBodyBytes {
		return canonical.Event{}, fmt.Errorf("simulator: body exceeds %d bytes", maxBodyBytes)
	}

	var in request
	if err := json.Unmarshal(raw, &in); err != nil {
		return canonical.Event{}, fmt.Errorf("simulator: malformed json body: %w", err)
	}

	phase := in.Phase
	if phase == "" {
		phase = canonical.PhaseBegin
		if len(in.Path) > 0 {
			phase = canonical.PhaseContinue
		}
	}

	ev := canonical.Event{
		MNO:        Name,
		SessionID:  in.SessionID,
		MSISDN:     in.MSISDN,
		Shortcode:  in.Shortcode,
		Path:       in.Path,
		Phase:      phase,
		ReceivedAt: a.now(),
		Raw:        raw,
	}
	if err := ev.Validate(); err != nil {
		return canonical.Event{}, fmt.Errorf("simulator: %w", err)
	}
	return ev, nil
}

// Render writes the canonical response as JSON.
func (a *Adapter) Render(w http.ResponseWriter, resp canonical.Response) error {
	if err := resp.Validate(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(resp)
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

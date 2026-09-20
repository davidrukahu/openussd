// Package africastalking implements the telco adapter for the Africa's
// Talking USSD API.
//
// Africa's Talking is an aggregator rather than a single MNO: one integration
// reaches Safaricom and Airtel in Kenya, MTN and Airtel in Uganda, and
// several other markets. RFC-0001 names aggregators as a valid adapter class,
// and this is the one adapter whose sandbox can be obtained today without a
// commercial shortcode agreement, which is why it lands first.
//
// # Wire format
//
// Inbound is an HTML form POST:
//
//		sessionId=ATUid_abc123&serviceCode=*384*1234%23&phoneNumber=%2B254711223344
//		&networkCode=63902&text=1*2
//
//	  - text is the user's input so far, joined with "*", and empty on the
//	    first request of a session. There is no explicit lifecycle field, so
//	    the phase is inferred from it.
//	  - phoneNumber arrives in E.164 form with the leading "+".
//
// Outbound is text/plain whose first token is the lifecycle verb:
//
//	CON What is your name?      keeps the session open
//	END Goodbye.                closes the session
//
// # Authenticity
//
// Africa's Talking signs nothing and sends no shared secret. Verify
// therefore always succeeds, and deployments are expected to wrap this
// adapter in adapter.TrustedProxy with the published callback source ranges.
package africastalking

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/davidrukahu/openussd/canonical"
)

// Name is the adapter's stable identifier.
const Name = "africastalking"

// maxBodyBytes caps how much of an inbound request we will read. USSD
// callbacks are a few hundred bytes; anything larger is a mistake or an
// attack, and we would rather fail than buffer it.
const maxBodyBytes = 64 << 10

// Adapter implements adapter.Adapter for Africa's Talking.
type Adapter struct {
	// Now is the clock used for Event.ReceivedAt. Nil means time.Now.
	Now func() time.Time
}

// New returns an adapter using the real clock.
func New() *Adapter { return &Adapter{} }

// Name reports the adapter identifier.
func (a *Adapter) Name() string { return Name }

// Verify always succeeds: the provider offers no authenticity signal to
// check. See the package comment - wrap this adapter in adapter.TrustedProxy.
func (a *Adapter) Verify(*http.Request) error { return nil }

// Parse decodes an Africa's Talking form POST into a canonical event.
func (a *Adapter) Parse(r *http.Request) (canonical.Event, error) {
	if r.Method != http.MethodPost {
		return canonical.Event{}, fmt.Errorf("africastalking: expected POST, got %s", r.Method)
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return canonical.Event{}, fmt.Errorf("africastalking: reading body: %w", err)
	}
	if len(raw) > maxBodyBytes {
		return canonical.Event{}, fmt.Errorf("africastalking: body exceeds %d bytes", maxBodyBytes)
	}

	form, err := url.ParseQuery(string(raw))
	if err != nil {
		return canonical.Event{}, fmt.Errorf("africastalking: malformed form body: %w", err)
	}

	text := form.Get("text")
	ev := canonical.Event{
		MNO:         Name,
		SessionID:   strings.TrimSpace(form.Get("sessionId")),
		MSISDN:      normaliseMSISDN(form.Get("phoneNumber")),
		Shortcode:   strings.TrimSpace(form.Get("serviceCode")),
		Path:        splitPath(text),
		Phase:       phaseFor(text),
		NetworkCode: strings.TrimSpace(form.Get("networkCode")),
		ReceivedAt:  a.now(),
		Raw:         raw,
	}

	if err := ev.Validate(); err != nil {
		return canonical.Event{}, fmt.Errorf("africastalking: %w", err)
	}
	return ev, nil
}

// Render writes the canonical response in the provider's CON/END form.
func (a *Adapter) Render(w http.ResponseWriter, resp canonical.Response) error {
	if err := resp.Validate(); err != nil {
		return err
	}

	verb := "CON"
	if resp.EndSession {
		verb = "END"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprintf(w, "%s %s", verb, resp.Body)
	return err
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

// phaseFor infers the session lifecycle from the input field.
//
// The provider has no explicit lifecycle signal: an empty text field means
// the user has just dialled. Cancel and timeout are never delivered as
// callbacks at all, the session simply stops, so this adapter produces
// neither. Nothing else produces them either today: the session store
// expires an abandoned dialogue silently rather than synthesising a
// terminal event. Only the local simulator can send one, which is how the
// gateway's terminal-phase handling gets exercised.
func phaseFor(text string) canonical.Phase {
	if text == "" {
		return canonical.PhaseBegin
	}
	return canonical.PhaseContinue
}

// splitPath turns the "*"-joined input field into ordered segments.
//
// An empty segment is meaningful: the user pressed send on an empty prompt,
// which several menus treat as "accept the default", so empties are kept
// rather than filtered out.
func splitPath(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "*")
}

// normaliseMSISDN trims the number and guarantees the leading "+" of E.164.
// The provider is consistent about sending it, but a missing "+" would
// silently create a second session store key for the same subscriber.
func normaliseMSISDN(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "+") {
		return s
	}
	return "+" + s
}

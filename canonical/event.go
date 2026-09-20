// Package canonical defines the wire-neutral types every telco adapter
// produces and consumes. It is the contract described in RFC-0001; nothing
// downstream of an adapter should ever see an MNO-native shape.
package canonical

import (
	"fmt"
	"strings"
	"time"
)

// MaxBodyLen is the number of characters a single USSD screen may carry.
// The GSM 03.38 7-bit alphabet packs 182 septets into a USSD string; anything
// longer is truncated or rejected by the network, not by us.
const MaxBodyLen = 182

// Phase normalises the session lifecycle. MNOs express this differently —
// Safaricom infers "new" from an empty input field, MTN sends an explicit
// type code — so adapters map their native signal onto this enum.
type Phase string

const (
	// PhaseBegin is the first request of a session: the user has dialled the
	// shortcode and supplied no input yet.
	PhaseBegin Phase = "begin"
	// PhaseContinue carries user input for an already-open session.
	PhaseContinue Phase = "continue"
	// PhaseCancel means the user dismissed the dialogue on the handset.
	PhaseCancel Phase = "cancel"
	// PhaseTimeout means the network expired the session before the user
	// replied. No response body will be delivered to the handset.
	PhaseTimeout Phase = "timeout"
)

// Terminal reports whether the phase ends the session without any further
// response being rendered to the handset.
func (p Phase) Terminal() bool {
	return p == PhaseCancel || p == PhaseTimeout
}

// Valid reports whether p is one of the defined phases.
func (p Phase) Valid() bool {
	switch p {
	case PhaseBegin, PhaseContinue, PhaseCancel, PhaseTimeout:
		return true
	}
	return false
}

// Event is one inbound turn of a USSD dialogue, normalised across MNOs.
type Event struct {
	// MNO identifies the adapter that produced this event, e.g. "africastalking".
	MNO string `json:"mno"`
	// SessionID is the network-assigned session identifier. Opaque; unique
	// only within one MNO.
	SessionID string `json:"session_id"`
	// MSISDN is the subscriber number in E.164 form.
	//
	// This is a CLAIM made by the network, not an authenticated identity. It
	// is trivially spoofable by anyone who can reach the gateway's inbound
	// endpoint directly. Flows needing real assurance must layer PIN or OTP
	// on top. See docs/architecture.md, "MNO claim trust boundary".
	MSISDN string `json:"msisdn"`
	// Shortcode is the dialled code, e.g. "*384*1234#".
	Shortcode string `json:"shortcode"`
	// Path holds the user's input segments since the session began, oldest
	// first. Empty on PhaseBegin. Applications read this instead of parsing
	// any MNO-specific joined string.
	Path []string `json:"path"`
	// Phase is the session lifecycle signal.
	Phase Phase `json:"phase"`
	// NetworkCode is the MNO-reported network identifier where available
	// (Africa's Talking reports an MCCMNC). Empty when the adapter has none.
	NetworkCode string `json:"network_code,omitempty"`
	// ReceivedAt is when the gateway accepted the request.
	ReceivedAt time.Time `json:"received_at"`
	// Raw is the original request body, retained so the audit log can
	// reproduce exactly what the MNO sent. Required for incident
	// investigation and for capturing contract-test fixtures.
	Raw []byte `json:"raw,omitempty"`
}

// LastInput returns the most recent input segment, or "" on a fresh session.
func (e Event) LastInput() string {
	if len(e.Path) == 0 {
		return ""
	}
	return e.Path[len(e.Path)-1]
}

// Validate checks the invariants every adapter must uphold before the event
// is allowed into the rest of the gateway.
func (e Event) Validate() error {
	switch {
	case e.MNO == "":
		return fmt.Errorf("canonical: event has no MNO")
	case e.SessionID == "":
		return fmt.Errorf("canonical: event has no session id")
	case e.MSISDN == "":
		return fmt.Errorf("canonical: event has no MSISDN")
	case !e.Phase.Valid():
		return fmt.Errorf("canonical: unknown phase %q", e.Phase)
	case e.Phase == PhaseBegin && len(e.Path) > 0:
		return fmt.Errorf("canonical: begin phase carries %d input segments", len(e.Path))
	}
	for i, seg := range e.Path {
		if strings.ContainsRune(seg, '*') {
			return fmt.Errorf("canonical: path segment %d still contains a separator: %q", i, seg)
		}
	}
	return nil
}

// Response is the reply to one Event, before it is serialised into an
// MNO-native wire format by the adapter that produced the Event.
type Response struct {
	// Body is the text shown on the handset. Must be <= MaxBodyLen runes.
	Body string `json:"body"`
	// EndSession closes the dialogue: adapters render this as the network's
	// terminating form (Africa's Talking "END", others differ).
	EndSession bool `json:"end_session"`
}

// Continue builds a response that keeps the session open.
func Continue(body string) Response { return Response{Body: body} }

// End builds a response that closes the session.
func End(body string) Response { return Response{Body: body, EndSession: true} }

// Validate enforces the screen budget. Adapters call this before rendering so
// an over-long screen fails in our logs rather than being silently mangled by
// the network.
func (r Response) Validate() error {
	if n := len([]rune(r.Body)); n > MaxBodyLen {
		return fmt.Errorf("canonical: response body is %d chars, limit is %d", n, MaxBodyLen)
	}
	return nil
}

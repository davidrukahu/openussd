package webhook

import (
	"bytes"
	"encoding/json"

	"github.com/davidrukahu/openussd/canonical"
)

// Version is the protocol version carried by every request. It covers the
// envelope and the canonical event inside it; the signature carries its own
// scheme version separately, because the two can rotate independently.
//
// Adding a field is not a version change: tenants must ignore keys they do
// not recognise. Removing or reinterpreting one is.
const Version = "1"

// Request is the JSON body the gateway POSTs to a tenant.
//
// It carries the event and the application's own state from the previous
// turn. Shipping state to the tenant and back keeps tenant applications
// stateless: they can be restarted, scaled out, or run on a serverless
// platform without losing a dialogue mid-menu.
type Request struct {
	// Version is the protocol version, always Version for a gateway that
	// sent it. A tenant that does not recognise it should refuse the turn
	// rather than guess at the payload.
	Version string `json:"version"`

	Event canonical.Event `json:"event"`
	// Turn counts screens served in this session, starting at 1.
	Turn int `json:"turn"`
	// State is whatever the tenant returned last turn, verbatim. Null on
	// the first turn of a session, and after a tenant clears it.
	State json.RawMessage `json:"state,omitempty"`
}

// NormaliseState collapses the two ways a JSON document says "no state"
// into one.
//
// An absent key decodes to a nil RawMessage, but an explicit null decodes
// to the four bytes "null", which is not nil. The protocol documents the
// two as equivalent, so without this a tenant that sends "state": null
// would have the literal null stored as its state and handed back next
// turn.
func NormaliseState(state json.RawMessage) json.RawMessage {
	if len(state) == 0 || bytes.Equal(bytes.TrimSpace(state), []byte("null")) {
		return nil
	}
	return state
}

// Reply is the JSON body a tenant returns.
type Reply struct {
	// Response is the screen to render.
	Response canonical.Response `json:"response"`
	// State replaces the stored state for this session. A tenant that omits
	// it keeps whatever was stored, so a handler that only reads state does
	// not have to echo it back.
	State json.RawMessage `json:"state,omitempty"`
	// ClearState discards the stored state even if State is set. Useful when
	// a flow completes but the dialogue continues.
	ClearState bool `json:"clear_state,omitempty"`
}

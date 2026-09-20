package webhook

import (
	"encoding/json"

	"github.com/davidrukahu/openussd/canonical"
)

// Request is the JSON body the gateway POSTs to a tenant.
//
// It carries the event and the application's own state from the previous
// turn. Shipping state to the tenant and back keeps tenant applications
// stateless: they can be restarted, scaled out, or run on a serverless
// platform without losing a dialogue mid-menu.
type Request struct {
	Event canonical.Event `json:"event"`
	// Turn counts screens served in this session, starting at 1.
	Turn int `json:"turn"`
	// State is whatever the tenant returned last turn, verbatim. Null on
	// the first turn of a session, and after a tenant clears it.
	State json.RawMessage `json:"state,omitempty"`
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

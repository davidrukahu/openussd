// Package session keeps USSD dialogues continuous.
//
// Every USSD screen arrives as an independent HTTP request with no transport
// -level continuity, so the gateway is what makes a sequence of requests a
// conversation. Sessions are addressed by (MNO, session id) because session
// identifiers are only unique within one network.
//
// # Encoding
//
// Application state is stored as opaque JSON. The gateway never looks inside
// it: it cannot warn on a schema it does not know, and inspecting tenant
// state would be a multi-tenancy leak waiting to happen. JSON over CBOR is
// the resolution proposed in issue #10 - CBOR is roughly 30% smaller, but
// being able to read live state with `redis-cli GET` during an incident is
// worth more than the bytes until Redis pressure is measurable. Store is the
// seam: a CBOR implementation would satisfy the same interface, and nothing
// above it encodes or decodes the blob.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DefaultTTL is how long a session survives without user input. USSD
// networks generally abandon a dialogue near 180s; expiring at the same
// point means the gateway and the network agree on what is still live.
const DefaultTTL = 180 * time.Second

// ErrNotFound is returned by Load when no live session exists for the key.
var ErrNotFound = errors.New("session: not found")

// Key addresses one session. Session identifiers are unique per network,
// not globally, so the MNO is part of the key.
type Key struct {
	MNO       string
	SessionID string
}

// String renders the key in the form used as the storage key.
func (k Key) String() string { return k.MNO + ":" + k.SessionID }

// Validate rejects keys that would collide or address nothing.
func (k Key) Validate() error {
	switch {
	case k.MNO == "":
		return fmt.Errorf("session: key has no MNO")
	case k.SessionID == "":
		return fmt.Errorf("session: key has no session id")
	}
	return nil
}

// Session is the gateway's record of one in-flight dialogue.
type Session struct {
	Key Key `json:"key"`
	// Tenant is the tenant this session was routed to on its first turn.
	// Fixed for the life of the session: re-routing mid-dialogue would hand
	// one tenant's state to another.
	Tenant string `json:"tenant"`
	// MSISDN is the subscriber number as claimed by the network.
	MSISDN string `json:"msisdn"`
	// Shortcode is the code the user dialled.
	Shortcode string `json:"shortcode"`
	// Turn counts screens served, starting at 1. Useful for rate limiting
	// and for spotting dialogues that loop.
	Turn int `json:"turn"`
	// State is application-owned and opaque to the gateway.
	State json.RawMessage `json:"state,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists sessions for their idle lifetime.
//
// Implementations must expire entries TTL after the last Save. The memory
// store is the default and is enough for a single-process deployment; Redis
// is for running more than one gateway replica.
type Store interface {
	// Load returns the session for key. It returns ErrNotFound if no live
	// session exists, which callers treat as "this is a new dialogue"
	// rather than as a failure.
	Load(ctx context.Context, key Key) (Session, error)

	// Save writes the session and refreshes its idle expiry.
	Save(ctx context.Context, s Session) error

	// Delete removes the session. Deleting an absent session is not an error:
	// a dialogue ending twice is normal when a network retries.
	Delete(ctx context.Context, key Key) error

	// Close releases any resources held by the store.
	Close() error
}

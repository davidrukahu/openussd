// Package adapter defines the telco-adapter contract from RFC-0001 and a
// registry of the adapters compiled into a gateway build.
//
// Adding an MNO is one new package implementing Adapter plus a directory of
// contract-test fixtures. Nothing else in the gateway changes.
package adapter

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/davidrukahu/openussd/canonical"
)

// ErrUntrusted is returned by Verify when a request cannot be attributed to
// the MNO it claims to come from. The gateway rejects such requests before
// allocating any session state.
var ErrUntrusted = errors.New("adapter: request failed authenticity check")

// Adapter translates between one MNO's wire format and the canonical types.
type Adapter interface {
	// Name is the stable identifier written into Event.MNO and used for
	// routing, metrics, and fixture directories. Lowercase, no spaces.
	Name() string

	// Verify checks whatever authenticity signal the MNO offers - shared
	// secret, signature, mTLS, or source address - and returns ErrUntrusted
	// if the request should not be processed. It runs before Parse so an
	// unauthentic request never allocates session state.
	//
	// Adapters for networks that offer no signal at all return nil and are
	// expected to be wrapped by TrustedProxy at configuration time.
	Verify(r *http.Request) error

	// Parse decodes an MNO-native request body into a canonical event. Parse
	// may consume r.Body.
	Parse(r *http.Request) (canonical.Event, error)

	// Render serialises a canonical response into the MNO-native wire form.
	Render(w http.ResponseWriter, resp canonical.Response) error
}

// AllowlistRequirer is implemented by adapters whose provider offers no
// authenticity signal of its own, so source position is the only control
// available.
//
// This is a property of the adapter, not of the configuration. Asking the
// adapter means a new non-authenticating adapter is guarded the day it is
// written, rather than the day someone remembers to add its name to a
// check somewhere else.
type AllowlistRequirer interface {
	// RequiresAllowlist reports whether the gateway should refuse to start
	// with this adapter enabled and no allowed_sources configured.
	RequiresAllowlist() bool
}

// NeedsAllowlist reports whether a is an adapter that must not be exposed
// without a source allowlist. Adapters that say nothing are treated as not
// requiring one.
func NeedsAllowlist(a Adapter) bool {
	r, ok := a.(AllowlistRequirer)
	return ok && r.RequiresAllowlist()
}

// Registry holds the adapters available to a running gateway, keyed by name.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register adds a by name. Registering the same name twice is a programming
// error and returns an error rather than silently replacing the adapter.
func (reg *Registry) Register(a Adapter) error {
	if a == nil {
		return errors.New("adapter: cannot register nil adapter")
	}
	name := a.Name()
	if name == "" {
		return errors.New("adapter: cannot register adapter with empty name")
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, exists := reg.adapters[name]; exists {
		return fmt.Errorf("adapter: %q is already registered", name)
	}
	reg.adapters[name] = a
	return nil
}

// Lookup returns the adapter registered under name.
func (reg *Registry) Lookup(name string) (Adapter, bool) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	a, ok := reg.adapters[name]
	return a, ok
}

// Names returns the registered adapter names in sorted order.
func (reg *Registry) Names() []string {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	names := make([]string, 0, len(reg.adapters))
	for name := range reg.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Package tenant maps an inbound dialogue onto the application that should
// answer it, and delivers canonical events to that application.
//
// Shortcodes in most African markets are shared: several services sit behind
// one code, distinguished by the first menu selection. Routing therefore
// keys on (shortcode, sub-prefix) rather than shortcode alone.
package tenant

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/davidrukahu/openussd/canonical"
)

// DefaultWebhookTimeout bounds how long the gateway waits for a tenant.
//
// It is short on purpose. The user is holding a handset with a network
// dialogue open that the MNO will tear down in seconds; a gateway that waits
// 30s for a slow tenant produces a dead session either way, and blocks a
// connection while doing it.
const DefaultWebhookTimeout = 5 * time.Second

// Tenant is one application reachable through the gateway.
type Tenant struct {
	// Name identifies the tenant in logs, metrics, and the outbound
	// X-OpenUSSD-Tenant header.
	Name string `yaml:"name" json:"name"`

	// Shortcode is the dialled code this tenant answers on, e.g. "*384*1234#".
	Shortcode string `yaml:"shortcode" json:"shortcode"`

	// Prefix is the leading input segments that select this tenant on a
	// shared shortcode. Empty means the tenant answers the shortcode
	// directly; exactly one tenant per shortcode may do so.
	//
	// Matched segments are stripped before the event reaches the tenant:
	// the selector belongs to the gateway's routing, not the application's
	// menu.
	Prefix []string `yaml:"prefix" json:"prefix"`

	// WebhookURL receives the canonical event as a JSON POST.
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`

	// Secret signs outbound webhooks. Per tenant, rotatable, and never
	// logged.
	Secret string `yaml:"secret" json:"-"`

	// Timeout overrides DefaultWebhookTimeout for this tenant.
	Timeout time.Duration `yaml:"timeout" json:"timeout,omitempty"`
}

// Validate checks a tenant definition at load time, so a misconfiguration
// surfaces on startup rather than as a dead shortcode in production.
func (t Tenant) Validate() error {
	switch {
	case t.Name == "":
		return fmt.Errorf("tenant: entry has no name")
	case t.Shortcode == "":
		return fmt.Errorf("tenant %q: no shortcode", t.Name)
	case t.WebhookURL == "":
		return fmt.Errorf("tenant %q: no webhook url", t.Name)
	case t.Secret == "":
		// A tenant without a secret cannot tell a real event from a forged
		// one, so an unsigned tenant is a configuration error, not a
		// convenience.
		return fmt.Errorf("tenant %q: no webhook secret", t.Name)
	}

	u, err := url.Parse(t.WebhookURL)
	if err != nil {
		return fmt.Errorf("tenant %q: unparseable webhook url: %w", t.Name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("tenant %q: webhook url scheme %q is not http(s)", t.Name, u.Scheme)
	}
	for i, seg := range t.Prefix {
		if seg == "" {
			return fmt.Errorf("tenant %q: prefix segment %d is empty", t.Name, i)
		}
	}
	return nil
}

// EffectiveTimeout returns the tenant's timeout or the default.
func (t Tenant) EffectiveTimeout() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return DefaultWebhookTimeout
}

// ErrNoRoute means no configured tenant answers this shortcode and input.
type ErrNoRoute struct {
	Shortcode string
	Path      []string
}

func (e ErrNoRoute) Error() string {
	return fmt.Sprintf("tenant: no route for shortcode %q with input %q",
		e.Shortcode, strings.Join(e.Path, "*"))
}

// Router resolves events to tenants.
type Router struct {
	// byShortcode holds each shortcode's tenants, longest prefix first, so
	// the first match found is the most specific one.
	byShortcode map[string][]Tenant
}

// NewRouter validates the tenant set and builds the routing table.
//
// Two rules are enforced here rather than discovered at runtime: no two
// tenants may claim the same (shortcode, prefix), and every shortcode needs
// exactly one tenant with an empty prefix to answer the first screen. The
// second rule exists because the opening screen of a shared shortcode has no
// input to route on yet; serving a gateway-owned selection menu instead is
// possible, but that is a product decision, not a default.
func NewRouter(tenants []Tenant) (*Router, error) {
	table := make(map[string][]Tenant, len(tenants))
	seen := make(map[string]string, len(tenants))

	for _, t := range tenants {
		if err := t.Validate(); err != nil {
			return nil, err
		}
		route := t.Shortcode + "|" + strings.Join(t.Prefix, "*")
		if other, dup := seen[route]; dup {
			return nil, fmt.Errorf("tenant %q collides with %q: both claim %s", t.Name, other, route)
		}
		seen[route] = t.Name
		table[t.Shortcode] = append(table[t.Shortcode], t)
	}

	for shortcode, group := range table {
		roots := 0
		for _, t := range group {
			if len(t.Prefix) == 0 {
				roots++
			}
		}
		if roots != 1 {
			return nil, fmt.Errorf(
				"shortcode %s has %d tenants with an empty prefix, want exactly 1 to answer the first screen",
				shortcode, roots)
		}
		sort.SliceStable(group, func(i, j int) bool {
			return len(group[i].Prefix) > len(group[j].Prefix)
		})
	}

	return &Router{byShortcode: table}, nil
}

// Route returns the tenant for an event, and the event with any routing
// prefix stripped from its path.
func (r *Router) Route(ev canonical.Event) (Tenant, canonical.Event, error) {
	for _, t := range r.byShortcode[ev.Shortcode] {
		if hasPrefix(ev.Path, t.Prefix) {
			routed := ev
			routed.Path = ev.Path[len(t.Prefix):]
			return t, routed, nil
		}
	}
	return Tenant{}, ev, ErrNoRoute{Shortcode: ev.Shortcode, Path: ev.Path}
}

// Tenants returns every configured tenant, for startup logging and /readyz.
func (r *Router) Tenants() []Tenant {
	var out []Tenant
	for _, group := range r.byShortcode {
		out = append(out, group...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i, seg := range prefix {
		if path[i] != seg {
			return false
		}
	}
	return true
}

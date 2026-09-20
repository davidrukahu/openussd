package adapter

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/davidrukahu/openussd/canonical"
)

// TrustedProxy wraps an adapter whose MNO offers no authenticity signal of
// its own and substitutes a source-address allowlist.
//
// This answers the open question in RFC-0001: rather than giving Adapter a
// "this network does not authenticate" flag, the weakness is made explicit in
// the configuration — you can see from the wiring which adapters are trusted
// on network position alone.
//
// Source-address trust is weak. It holds only while nothing else can reach
// the inbound endpoint, so the deployment docs require the gateway's inbound
// listener to be unreachable from the public internet except via the allowed
// ranges.
type TrustedProxy struct {
	// Adapter is the wrapped implementation. Its own Verify still runs, so
	// wrapping an adapter that does authenticate adds a second gate rather
	// than replacing the first.
	Adapter Adapter

	// Allowed lists the source ranges permitted to deliver inbound requests.
	// An empty list rejects everything: failing closed on a misconfigured
	// allowlist is safer than silently accepting the internet.
	Allowed []netip.Prefix

	// Forwarders lists ranges that are permitted to set X-Forwarded-For —
	// your own load balancer or ingress. When empty, X-Forwarded-For is
	// ignored entirely and only the direct peer address is considered.
	Forwarders []netip.Prefix
}

// Prefix is a source range an adapter may be restricted to.
type Prefix = netip.Prefix

// ParsePrefixes converts CIDR strings, or bare addresses, into prefixes.
// A bare address is treated as a single-host prefix.
func ParsePrefixes(in []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("adapter: bad CIDR %q: %w", s, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("adapter: bad address %q: %w", s, err)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// Name reports the wrapped adapter's name unchanged: wrapping is a
// deployment concern and must not change routing or fixture paths.
func (t *TrustedProxy) Name() string { return t.Adapter.Name() }

// Parse delegates to the wrapped adapter.
func (t *TrustedProxy) Parse(r *http.Request) (canonical.Event, error) {
	return t.Adapter.Parse(r)
}

// Render delegates to the wrapped adapter.
func (t *TrustedProxy) Render(w http.ResponseWriter, resp canonical.Response) error {
	return t.Adapter.Render(w, resp)
}

// Verify accepts the request only if its effective client address falls
// inside Allowed, then defers to the wrapped adapter's own check.
func (t *TrustedProxy) Verify(r *http.Request) error {
	addr, err := t.clientAddr(r)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUntrusted, err)
	}
	if !containsAddr(t.Allowed, addr) {
		return fmt.Errorf("%w: source %s is not in the allowlist", ErrUntrusted, addr)
	}
	return t.Adapter.Verify(r)
}

// clientAddr resolves the address to test against the allowlist. The direct
// peer is used unless it is a configured forwarder, in which case the last
// non-forwarder entry in X-Forwarded-For is used — the closest hop the
// forwarder actually observed, and the furthest right an attacker cannot
// forge by prepending entries.
func (t *TrustedProxy) clientAddr(r *http.Request) (netip.Addr, error) {
	peer, err := peerAddr(r)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(t.Forwarders) == 0 || !containsAddr(t.Forwarders, peer) {
		return peer, nil
	}

	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			continue
		}
		hop = hop.Unmap()
		if !containsAddr(t.Forwarders, hop) {
			return hop, nil
		}
	}
	// Every hop was one of ours, or the header was absent or unparseable.
	return peer, nil
}

func peerAddr(r *http.Request) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("unparseable remote address %q", r.RemoteAddr)
	}
	return a.Unmap(), nil
}

func containsAddr(prefixes []netip.Prefix, a netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Package httpx holds the gateway's HTTP surface: the inbound USSD endpoint
// and the operational endpoints around it.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidrukahu/openussd/canonical"
	"github.com/davidrukahu/openussd/gateway/internal/adapter"
	"github.com/davidrukahu/openussd/gateway/internal/session"
	"github.com/davidrukahu/openussd/gateway/internal/tenant"
	"github.com/davidrukahu/openussd/webhook"
)

// HeaderRequestID correlates one inbound callback across the gateway's logs
// and the tenant's.
const HeaderRequestID = "X-Request-Id"

// maxRequestIDLen bounds an inbound correlation id. It is echoed into logs,
// so it is treated as untrusted text: bounded, and printable characters
// only.
const maxRequestIDLen = 64

// userFacingError is what a subscriber sees when the gateway cannot serve a
// dialogue. It says nothing about why: a handset is not a debugging surface,
// and the detail belongs in the operator's logs.
const userFacingError = "Service unavailable. Please try again later."

// Inbound handles one MNO's callback endpoint.
//
// One handler is mounted per adapter, at /ussd/{adapter}. Keeping the
// endpoints separate means a per-MNO allowlist is expressible in the
// deployment's own network policy, not just in ours.
type Inbound struct {
	adapter    adapter.Adapter
	sessions   session.Store
	router     *tenant.Router
	dispatcher *tenant.Dispatcher
	log        *slog.Logger
}

// NewInbound wires a handler for one adapter.
func NewInbound(a adapter.Adapter, sessions session.Store, router *tenant.Router, dispatcher *tenant.Dispatcher, log *slog.Logger) *Inbound {
	return &Inbound{adapter: a, sessions: sessions, router: router, dispatcher: dispatcher, log: log}
}

func (in *Inbound) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Every line about this callback carries the same id, and the id goes
	// back on the response so an operator can follow one dialogue turn
	// from the network's logs into ours.
	reqID := requestID(r)
	w.Header().Set(HeaderRequestID, reqID)
	log := in.log.With("mno", in.adapter.Name(), "request_id", reqID)

	// Authenticity first, so an unattributable request never reaches the
	// session store. RFC-0001 puts Verify before Parse for exactly this.
	if err := in.adapter.Verify(r); err != nil {
		log.Warn("rejected inbound request", "error", err, "remote", r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ev, err := in.adapter.Parse(r)
	if err != nil {
		log.Warn("could not parse inbound request", "error", err, "remote", r.RemoteAddr)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	log = log.With("session_id", ev.SessionID, "shortcode", ev.Shortcode, "phase", string(ev.Phase))

	resp, err := in.handle(r.Context(), log, ev)
	if err != nil {
		// The subscriber still gets a screen: a USSD dialogue that returns
		// nothing shows the handset's own "connection problem", which
		// tells the user less than we can.
		log.Error("dialogue failed", "error", err, "duration", time.Since(start))
		resp = canonical.End(userFacingError)
	}

	// A cancelled or timed-out session has no handset left to render to.
	if ev.Phase.Terminal() {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := in.adapter.Render(w, resp); err != nil {
		log.Error("could not render response", "error", err)
		return
	}
	cost, encoding := canonical.ScreenCost(resp.Body)
	log.Info("served screen",
		"end_session", resp.EndSession,
		"cost", cost,
		"encoding", string(encoding),
		"budget", canonical.Budget(resp.Body),
		"duration", time.Since(start))
}

// handle runs one turn of a dialogue: load session, route, deliver, persist.
func (in *Inbound) handle(ctx context.Context, log *slog.Logger, ev canonical.Event) (canonical.Response, error) {
	key := session.Key{MNO: ev.MNO, SessionID: ev.SessionID}

	sess, err := in.sessions.Load(ctx, key)
	switch {
	case errors.Is(err, session.ErrNotFound):
		sess = session.Session{Key: key, MSISDN: ev.MSISDN, Shortcode: ev.Shortcode}
	case err != nil:
		// Unreadable state is not fatal: a fresh dialogue beats a dead
		// shortcode. Log it loudly, because it should not happen.
		log.Error("could not load session, starting a new one", "error", err)
		sess = session.Session{Key: key, MSISDN: ev.MSISDN, Shortcode: ev.Shortcode}
	case sess.MSISDN != ev.MSISDN:
		// The session id matched but the subscriber did not. Session ids
		// are network-assigned and unguessable in practice, so this is
		// either a network reusing one or somebody probing the endpoint.
		// Either way the safe reading is that this is a different
		// dialogue, so it starts clean rather than inheriting the stored
		// tenant and state.
		log.Warn("session id reused by a different subscriber, starting a new dialogue")
		sess = session.Session{Key: key, MSISDN: ev.MSISDN, Shortcode: ev.Shortcode}
	}

	// The network abandoned this dialogue; release the state and render
	// nothing.
	if ev.Phase.Terminal() {
		if err := in.sessions.Delete(ctx, key); err != nil {
			log.Warn("could not delete ended session", "error", err)
		}
		log.Info("session ended by the network", "turns", sess.Turn)
		return canonical.Response{}, nil
	}

	target, routed, err := in.router.Route(ev)
	if err != nil {
		return canonical.Response{}, err
	}

	// Routing is re-evaluated every turn, so a shared shortcode can hand a
	// dialogue from its menu tenant to a service tenant when the service's
	// prefix is first matched. The handover must not carry state across the
	// boundary: the new tenant starts with a clean slate, and cannot read
	// what the previous one stored.
	if sess.Tenant != "" && sess.Tenant != target.Name {
		log.Info("dialogue handed to another tenant", "from", sess.Tenant, "to", target.Name)
		// A clean slate means a clean turn count too: the protocol defines
		// turn as screens served in this session starting at 1, and a
		// tenant gating first-turn behaviour on it would never see it.
		sess.State, sess.Turn = nil, 0
	}
	sess.Tenant = target.Name
	sess.Turn++

	reply, err := in.dispatcher.Deliver(ctx, target, webhook.Request{
		Event: routed,
		Turn:  sess.Turn,
		State: sess.State,
	})
	if err != nil {
		return canonical.Response{}, err
	}

	if reply.ClearState {
		sess.State = nil
	} else if reply.State != nil {
		sess.State = reply.State
	}

	if reply.Response.EndSession {
		if err := in.sessions.Delete(ctx, key); err != nil {
			log.Warn("could not delete finished session", "error", err)
		}
		return reply.Response, nil
	}

	if err := in.sessions.Save(ctx, sess); err != nil {
		// The screen was produced, but the next turn would start over. Fail
		// the dialogue rather than dropping the user into a menu that has
		// silently forgotten them.
		return canonical.Response{}, err
	}
	return reply.Response, nil
}

// requestID returns the caller's correlation id when it sent a usable one,
// and a fresh id otherwise.
//
// An inbound value is untrusted: it is bounded and stripped of anything
// non-printable before it reaches a log line, so a caller cannot forge log
// entries by embedding newlines in it.
func requestID(r *http.Request) string {
	if given := r.Header.Get(HeaderRequestID); given != "" {
		if id := sanitiseID(given); id != "" {
			return id
		}
	}

	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// The clock is a poor id but a better one than none: correlation
		// is a debugging aid, not a correctness property.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

func sanitiseID(in string) string {
	if len(in) > maxRequestIDLen {
		in = in[:maxRequestIDLen]
	}
	return strings.Map(func(r rune) rune {
		if r < ' ' || r > '~' {
			return -1
		}
		return r
	}, in)
}

// Health reports process liveness. It does no dependency checks: a liveness
// probe that fails when Redis is down restarts a gateway that was working.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Ready reports whether the gateway can serve traffic.
//
// It reports counts rather than the routing table. The endpoint is
// unauthenticated, and a list of tenant names with their shortcodes and
// routing prefixes is exactly what someone needs to aim a forged callback at
// a particular tenant. The full table is logged at startup instead, where
// the operator can see it and nobody else can.
func Ready(adapters []string, tenants []tenant.Tenant) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "ready",
			"adapters": adapters,
			"tenants":  len(tenants),
		})
	}
}

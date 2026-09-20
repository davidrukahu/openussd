package openussd

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/davidrukahu/openussd/webhook"
)

// maxRequestBytes caps how much of a gateway request we read. The canonical
// envelope is small; anything larger did not come from a gateway.
const maxRequestBytes = 256 << 10

// Handler serves an App over the gateway's webhook protocol.
//
// It verifies the gateway's signature before doing anything else, so an
// application exposed to the internet — which a webhook endpoint is —
// cannot be driven by anyone who guesses its URL.
type Handler[S any] struct {
	app    *App[S]
	secret string
	log    *slog.Logger
	now    func() time.Time
}

// NewHandler wires an app to an HTTP endpoint. The secret must match the
// one configured for this tenant in the gateway.
func NewHandler[S any](app *App[S], secret string, log *slog.Logger) *Handler[S] {
	if log == nil {
		log = slog.Default()
	}
	return &Handler[S]{app: app, secret: secret, log: log, now: func() time.Time { return time.Now().UTC() }}
}

func (h *Handler[S]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(body) > maxRequestBytes {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Verify against the exact bytes received, before decoding: the
	// signature covers bytes, and re-encoding would change them.
	if err := webhook.Verify(r.Header, h.secret, body, h.now(), webhook.DefaultTolerance); err != nil {
		h.log.Warn("rejected unsigned or missigned request", "error", err, "remote", r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req webhook.Request
	if err := json.Unmarshal(body, &req); err != nil {
		h.log.Warn("could not decode gateway request", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	resp, state, err := h.app.Turn(req.Event, req.Turn, req.State)
	if err != nil {
		// Returning 500 lets the gateway render its own failure screen,
		// which is a better user experience than this application guessing
		// at one while broken.
		h.log.Error("dialogue turn failed",
			"error", err, "session_id", req.Event.SessionID, "turn", req.Turn)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	reply := webhook.Reply{Response: resp, State: state, ClearState: state == nil}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reply); err != nil {
		h.log.Error("could not write reply", "error", err)
	}
}

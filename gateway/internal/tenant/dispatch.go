package tenant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/davidrukahu/openussd/webhook"
)

// maxTenantResponseBytes caps how much of a tenant's reply we read. A USSD
// screen is 182 characters; a tenant streaming a megabyte is broken or
// hostile, and either way we must not buffer it while a handset waits.
const maxTenantResponseBytes = 16 << 10

// Dispatcher delivers canonical events to tenant webhooks and decodes the
// canonical response.
type Dispatcher struct {
	client *http.Client
	now    func() time.Time
}

// maxIdleConnsPerTenant sizes the connection pool per tenant host.
//
// The standard library defaults to two, which suits a client talking to many
// hosts at low concurrency. A gateway is the opposite shape: a handful of
// tenant hosts at whatever concurrency the shortcode attracts. At the default
// every turn past the second would re-handshake TCP and TLS, adding round
// trips to a dialogue the network will abandon in seconds.
const maxIdleConnsPerTenant = 100

// maxConnsPerTenant bounds total connections to one tenant, open and idle
// together, so a slow tenant cannot exhaust the process's descriptors.
const maxConnsPerTenant = 256

// NewDispatcher returns a dispatcher. A nil client means a default one
// whose per-request deadline comes from the tenant's timeout.
func NewDispatcher(client *http.Client) *Dispatcher {
	if client == nil {
		client = &http.Client{
			Transport: pooledTransport(),
			// Never follow a redirect. Go strips Authorization and Cookie
			// across hosts but not our own signature headers, and on 307
			// or 308 it replays the body too. A tenant that is
			// compromised, misconfigured, or simply has an open redirect
			// could otherwise forward signed subscriber events anywhere,
			// including a cloud metadata endpoint.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Dispatcher{client: client, now: func() time.Time { return time.Now().UTC() }}
}

// pooledTransport clones the standard transport and widens its per-host idle
// pool.
func pooledTransport() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = maxIdleConnsPerTenant
	tr.MaxIdleConns = maxIdleConnsPerTenant * 4
	// Idle-pool size is not a concurrency cap. Without this a tenant
	// sitting at its timeout lets the gateway open one new socket per
	// in-flight dialogue until it runs out of file descriptors.
	tr.MaxConnsPerHost = maxConnsPerTenant
	return tr
}

// Deliver posts the event and the tenant's stored state to the tenant, and
// returns its reply.
//
// The context governs the whole call, including the tenant's timeout, so a
// cancelled inbound request does not leave an outbound one running.
func (d *Dispatcher) Deliver(ctx context.Context, t Tenant, payload webhook.Request) (webhook.Reply, error) {
	payload.Version = webhook.Version

	// Raw is the MNO's original bytes, kept for the gateway's own audit log.
	// Tenants get the canonical shape only; forwarding it would leak wire
	// details the SDK exists to hide, and grow every request.
	payload.Event.Raw = nil

	// path is always an array on the wire. A nil slice marshals to null,
	// and the two ways a turn can carry no input (a fresh dialogue, and a
	// routing prefix consuming everything) would otherwise reach tenants
	// as null and [] respectively. A tenant in a language without Go's
	// nil-slice equivalence would have to handle both.
	if payload.Event.Path == nil {
		payload.Event.Path = []string{}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return webhook.Reply{}, fmt.Errorf("tenant %q: encoding event: %w", t.Name, err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.EffectiveTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return webhook.Reply{}, fmt.Errorf("tenant %q: building request: %w", t.Name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "openussd-gateway/0.1")
	req.Header.Set(webhook.HeaderTenant, t.Name)
	webhook.Sign(req, t.Secret, d.now(), body)

	resp, err := d.client.Do(req)
	if err != nil {
		return webhook.Reply{}, fmt.Errorf("tenant %q: delivering event: %w", t.Name, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxTenantResponseBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return webhook.Reply{}, fmt.Errorf("tenant %q: replied %s", t.Name, resp.Status)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTenantResponseBytes+1))
	if err != nil {
		return webhook.Reply{}, fmt.Errorf("tenant %q: reading reply: %w", t.Name, err)
	}
	if len(raw) > maxTenantResponseBytes {
		return webhook.Reply{}, fmt.Errorf("tenant %q: reply exceeds %d bytes", t.Name, maxTenantResponseBytes)
	}

	var out webhook.Reply
	if err := json.Unmarshal(raw, &out); err != nil {
		return webhook.Reply{}, fmt.Errorf("tenant %q: decoding reply: %w", t.Name, err)
	}
	out.State = webhook.NormaliseState(out.State)

	if err := out.Response.Validate(); err != nil {
		// The tenant broke the screen budget. Failing here means the
		// operator sees it in gateway logs, rather than the user seeing a
		// silently truncated screen.
		return webhook.Reply{}, fmt.Errorf("tenant %q: %w", t.Name, err)
	}
	return out, nil
}

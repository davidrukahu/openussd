// Package webhook signs and verifies the requests the gateway sends to
// tenant applications.
//
// A tenant webhook endpoint is reachable by anyone who learns its URL, and
// the payload it carries claims a subscriber's MSISDN. Without a signature,
// forging "user +2547… pressed 1" is a curl command. Signing gives the
// application a way to know the request came from its gateway.
//
// This package is deliberately outside internal/: both the gateway and the
// SDK depend on it, and the SDK is intended to be split into its own module
// under Apache-2.0 later.
//
// # Scheme
//
// The signature is HMAC-SHA256 over "<unix-timestamp>.<body>", hex encoded
// and prefixed with a scheme version:
//
//	X-OpenUSSD-Timestamp: 1758369600
//	X-OpenUSSD-Signature: v1=3f9a...c2
//
// The timestamp is inside the signed string, so it cannot be changed without
// invalidating the signature, and verifiers reject timestamps outside a
// tolerance window to bound replay.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Header names carrying the signature material.
const (
	HeaderTimestamp = "X-OpenUSSD-Timestamp"
	HeaderSignature = "X-OpenUSSD-Signature"
	HeaderTenant    = "X-OpenUSSD-Tenant"
)

// SchemeV1 prefixes a signature produced by the scheme above. Signatures
// carry their version so the scheme can be rotated without a flag day.
const SchemeV1 = "v1"

// DefaultTolerance bounds how far a request's timestamp may be from the
// verifier's clock. Five minutes is generous for a protocol whose sessions
// last three, and it absorbs ordinary clock skew between hosts.
const DefaultTolerance = 5 * time.Minute

// ErrInvalidSignature means the request was not signed by the expected
// secret, is outside the tolerance window, or carries no signature at all.
// It is deliberately one error: telling a caller which part failed tells an
// attacker which part to fix.
var ErrInvalidSignature = errors.New("webhook: invalid signature")

// Signature computes the signature for a body at a point in time.
func Signature(secret string, ts time.Time, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts.Unix(), 10) + "."))
	mac.Write(body)
	return SchemeV1 + "=" + hex.EncodeToString(mac.Sum(nil))
}

// Sign attaches the timestamp and signature headers to an outbound request.
func Sign(r *http.Request, secret string, ts time.Time, body []byte) {
	r.Header.Set(HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
	r.Header.Set(HeaderSignature, Signature(secret, ts, body))
}

// Verify checks the headers on an inbound request against the body.
//
// The body must be the exact bytes received: read it once, verify, then
// decode. Re-encoding before verification will fail, which is intended -
// the signature covers bytes, not meaning.
func Verify(h http.Header, secret string, body []byte, now time.Time, tolerance time.Duration) error {
	if secret == "" {
		return fmt.Errorf("%w: no secret configured", ErrInvalidSignature)
	}

	rawTS := h.Get(HeaderTimestamp)
	if rawTS == "" {
		return fmt.Errorf("%w: missing %s", ErrInvalidSignature, HeaderTimestamp)
	}
	unix, err := strconv.ParseInt(rawTS, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: unparseable timestamp", ErrInvalidSignature)
	}

	ts := time.Unix(unix, 0)
	if tolerance <= 0 {
		tolerance = DefaultTolerance
	}
	if skew := now.Sub(ts); skew > tolerance || skew < -tolerance {
		return fmt.Errorf("%w: timestamp is %s from now", ErrInvalidSignature, skew.Round(time.Second))
	}

	got := h.Get(HeaderSignature)
	if !strings.HasPrefix(got, SchemeV1+"=") {
		return fmt.Errorf("%w: missing or unsupported signature scheme", ErrInvalidSignature)
	}

	// Constant-time comparison: a byte-at-a-time compare leaks the expected
	// signature to anyone willing to time a few thousand requests.
	if !hmac.Equal([]byte(got), []byte(Signature(secret, ts, body))) {
		return ErrInvalidSignature
	}
	return nil
}

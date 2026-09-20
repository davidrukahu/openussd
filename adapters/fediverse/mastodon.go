// Command fediverse is the OpenUSSD reference adapter: it exposes
// ActivityPub content - currently a Mastodon public timeline - as USSD
// menus, and demonstrates what the SDK is for.
//
// It is deliberately read-only. Write paths, identity binding between an
// MSISDN and a Fediverse account, PeerTube and PixelFed are milestone M4
// work; the value of this component today is the protocol mapping, not
// feature coverage.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// maxPostBytes caps what one post contributes to session state.
//
// The whole fetched timeline rides in the session blob so that paging back
// does not re-fetch, and that blob crosses the tenant webhook on every turn,
// where the gateway rejects a reply over 16KB. Ten unbounded posts clear
// that on their own: ten posts of 500 CJK characters measure about 15KB, so
// an ordinary timeline could fail the dialogue outright. 900 bytes is
// roughly five GSM screens or two of CJK, which is more than anyone reads
// on a feature phone.
const maxPostBytes = 900

// Post is one timeline entry, reduced to what a 182-character screen can
// carry. Everything a handset cannot render is dropped at the edge rather
// than carried through the session store.
type Post struct {
	Author string `json:"a"`
	Text   string `json:"t"`
	When   string `json:"w"`
}

// instanceTimeout is shorter than the gateway's own tenant timeout on
// purpose: a slow instance must not be the reason a handset sees nothing.
const instanceTimeout = 3 * time.Second

// defaultInstanceClient is shared across dialogues so its connection pool is
// reused. Every dialogue that opens the timeline fetches from the same host,
// and the standard two-idle-connection default would make most of those pay
// a fresh TLS handshake out of a three-second budget.
var defaultInstanceClient = &http.Client{
	Timeout: instanceTimeout,
	Transport: func() *http.Transport {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.MaxIdleConnsPerHost = 32
		return tr
	}(),
}

// Mastodon reads public timelines from one instance.
//
// No authentication: the public timeline endpoint is unauthenticated on a
// default Mastodon install, which keeps the demo free of an OAuth dance
// that a feature phone cannot complete anyway. Binding a USSD user to a
// real account is the open question in issue #9.
type Mastodon struct {
	// Instance is the base URL, e.g. https://mastodon.social.
	Instance string
	Client   *http.Client
}

// apiStatus is the subset of Mastodon's status object we read.
type apiStatus struct {
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	Account   struct {
		Acct        string `json:"acct"`
		DisplayName string `json:"display_name"`
	} `json:"account"`
	MediaAttachments []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"media_attachments"`
	Reblog *apiStatus `json:"reblog"`
}

// PublicTimeline fetches the most recent local posts.
func (m *Mastodon) PublicTimeline(ctx context.Context, limit int) ([]Post, error) {
	endpoint, err := url.Parse(strings.TrimRight(m.Instance, "/") + "/api/v1/timelines/public")
	if err != nil {
		return nil, fmt.Errorf("fediverse: bad instance url: %w", err)
	}
	q := endpoint.Query()
	q.Set("limit", fmt.Sprint(limit))
	q.Set("local", "true")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "openussd-fediverse-adapter/0.1 (+https://github.com/davidrukahu/openussd)")

	client := m.Client
	if client == nil {
		client = defaultInstanceClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fediverse: fetching timeline: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fediverse: instance replied %s", resp.Status)
	}

	var statuses []apiStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&statuses); err != nil {
		return nil, fmt.Errorf("fediverse: decoding timeline: %w", err)
	}

	posts := make([]Post, 0, len(statuses))
	for _, s := range statuses {
		posts = append(posts, toPost(s))
	}
	return posts, nil
}

func toPost(s apiStatus) Post {
	// A boost carries its content in the reblogged status; showing the
	// booster's empty body instead would render a blank screen.
	body := s
	prefix := ""
	if s.Reblog != nil {
		body = *s.Reblog
		prefix = "RT @" + body.Account.Acct + ": "
	}

	text := prefix + stripHTML(body.Content)
	for _, att := range body.MediaAttachments {
		// An image is nothing on a feature phone, but its description is
		// something. Alt text is the accessible form of the attachment,
		// which is exactly what this channel needs.
		desc := strings.TrimSpace(att.Description)
		if desc == "" {
			desc = "no description"
		}
		text += " [" + att.Type + ": " + desc + "]"
	}

	author := s.Account.DisplayName
	if strings.TrimSpace(author) == "" {
		author = s.Account.Acct
	}

	return Post{
		Author: strings.TrimSpace(author),
		Text:   capBytes(strings.TrimSpace(text), maxPostBytes),
		When:   humaniseTime(s.CreatedAt),
	}
}

// capBytes shortens s to at most limit bytes, cutting on a rune boundary and
// marking the cut.
func capBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	cut := 0
	for i := range s {
		if i > limit-3 {
			break
		}
		cut = i
	}
	return strings.TrimRightFunc(s[:cut], unicode.IsSpace) + "..."
}

var (
	tagPattern       = regexp.MustCompile(`<[^>]*>`)
	whitespacePatten = regexp.MustCompile(`[ \t]+`)
	blankLinePattern = regexp.MustCompile(`\n{3,}`)
)

// stripHTML reduces Mastodon's HTML content to plain text.
//
// Block boundaries become newlines before tags are removed, so paragraphs
// do not run together; mentions and hashtags survive because they are link
// text, not markup.
func stripHTML(in string) string {
	out := in
	for _, br := range []string{"<br>", "<br/>", "<br />", "</p>"} {
		out = strings.ReplaceAll(out, br, "\n")
	}
	out = tagPattern.ReplaceAllString(out, "")
	out = html.UnescapeString(out)
	out = whitespacePatten.ReplaceAllString(out, " ")
	out = blankLinePattern.ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

// humaniseTime renders an age as the few characters a screen can spare.
func humaniseTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}

	switch d := time.Since(t); {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// Package openussd is the Go SDK for building USSD applications against an
// OpenUSSD gateway.
//
// An application declares screens; the SDK handles the parts every USSD app
// otherwise rewrites: verifying the gateway's signature, decoding the
// canonical event, tracking which screen the user is on, persisting typed
// state between turns, and keeping every rendered screen inside the
// 182-character budget.
package openussd

import (
	"strings"
	"unicode"

	"github.com/davidrukahu/openussd/canonical"
)

// MaxScreen is the character budget for one USSD screen.
const MaxScreen = canonical.MaxBodyLen

// ellipsis marks a truncation. One rune, so it costs as little of the
// budget as possible.
const ellipsis = "…"

// maxWordLookback bounds how far Truncate will search backwards for a word
// boundary. Roughly the length of a long word: far enough to avoid cutting
// mid-word, near enough that a screen never loses a sentence to a single
// unbreakable token.
const maxWordLookback = 24

// Truncate shortens s to at most limit runes, marking the cut.
//
// It breaks at a word boundary where one is close enough to the limit to be
// worth it; a menu line ending mid-word reads like a bug, but sacrificing a
// third of a screen to avoid it reads worse.
func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit == 1 {
		return ellipsis
	}

	cut := runes[:limit-1]

	// The cut already lands on a word boundary when the next rune is a
	// space, so there is nothing to back off from.
	if unicode.IsSpace(runes[len(cut)]) {
		return strings.TrimRightFunc(string(cut), unicode.IsSpace) + ellipsis
	}

	floor := max(len(cut)-maxWordLookback, len(cut)/2)
	for i := len(cut) - 1; i >= floor; i-- {
		if unicode.IsSpace(cut[i]) {
			return strings.TrimRightFunc(string(cut[:i]), unicode.IsSpace) + ellipsis
		}
	}
	return string(cut) + ellipsis
}

// Paginate splits body into pages that fit the remaining budget once
// reserve characters are set aside for navigation controls.
//
// Splitting prefers paragraph breaks, then line breaks, then word
// boundaries, so a page rarely ends mid-sentence. A single word longer than
// a page is split rather than dropped: a URL is more useful broken than
// absent.
func Paginate(body string, reserve int) []string {
	budget := MaxScreen - reserve
	if budget <= 0 {
		return nil
	}

	var pages []string
	remaining := strings.TrimSpace(body)

	for remaining != "" {
		runes := []rune(remaining)
		if len(runes) <= budget {
			pages = append(pages, remaining)
			break
		}

		window := string(runes[:budget])
		cut := breakPoint(window)
		page := strings.TrimRightFunc(string([]rune(window)[:cut]), unicode.IsSpace)
		if page == "" {
			// No usable break: hard-split so we always make progress.
			page = window
			cut = budget
		}

		pages = append(pages, page)
		remaining = strings.TrimLeftFunc(string(runes[cut:]), unicode.IsSpace)
	}
	return pages
}

// breakPoint returns the rune index to split a full window at, preferring
// the latest paragraph, line, or word boundary in its final third.
func breakPoint(window string) int {
	runes := []rune(window)
	floor := len(runes) / 3

	for _, sep := range []string{"\n\n", "\n", " "} {
		if idx := strings.LastIndex(window, sep); idx > 0 {
			at := len([]rune(window[:idx]))
			if at >= floor {
				return at
			}
		}
	}
	return len(runes)
}

// MenuItem is one selectable line.
type MenuItem struct {
	// Key is what the user types. Usually a digit; "0" and "#" are
	// conventional for back and next.
	Key string
	// Label is the text shown beside the key.
	Label string
}

// Menu renders a title and numbered options within the screen budget.
//
// Labels are truncated before the menu is: a user can act on a shortened
// label, but not on an option that was dropped. Options that still do not
// fit are dropped from the end, so a list longer than one screen loses its
// tail silently — use MenuFit when some options must always be reachable,
// such as "Back" or "Quit".
func Menu(title string, items []MenuItem) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
	}

	for _, item := range items {
		line := "\n" + item.Key + ". " + item.Label
		if len([]rune(b.String()))+len([]rune(line)) > MaxScreen {
			// Try the line with its label truncated to whatever is left.
			left := MaxScreen - len([]rune(b.String())) - len([]rune("\n"+item.Key+". "))
			if left < 4 {
				break
			}
			line = "\n" + item.Key + ". " + Truncate(item.Label, left)
		}
		b.WriteString(line)
	}
	return b.String()
}

// MenuFit renders a title, as many items as fit, and then the footer items,
// which are always rendered.
//
// This is the menu a paginated list needs: dropping the tail of a list is
// survivable, but dropping "0. Back" strands the user on a screen with no
// way off it. The count of items actually shown is returned so the caller
// can page the rest.
func MenuFit(title string, items, footer []MenuItem) (string, int) {
	// Cost the footer first; it is not negotiable.
	footerText := ""
	for _, item := range footer {
		footerText += "\n" + item.Key + ". " + item.Label
	}

	budget := MaxScreen - len([]rune(footerText))
	if budget <= 0 {
		// A footer that fills the screen on its own is a programming
		// error, but truncating it beats returning nothing.
		return Truncate(strings.TrimPrefix(footerText, "\n"), MaxScreen), 0
	}

	var b strings.Builder
	b.WriteString(title)

	shown := 0
	for _, item := range items {
		line := "\n" + item.Key + ". " + item.Label
		if len([]rune(b.String()))+len([]rune(line)) > budget {
			break
		}
		b.WriteString(line)
		shown++
	}

	return b.String() + footerText, shown
}

// Fits reports whether s is within the screen budget. Applications call it
// in tests; the SDK enforces it on every render regardless.
func Fits(s string) bool { return len([]rune(s)) <= MaxScreen }

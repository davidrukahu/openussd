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

// MaxScreen is the character budget for a screen written in the GSM 03.38
// alphabet — the number every USSD document quotes.
//
// It is not the budget for every screen. Text containing anything outside
// that alphabet — an emoji, a Chinese character, a curly quote — is sent
// as UCS-2, where a screen holds 70 units rather than 182. Content from
// the federated web is full of such characters, so the helpers here
// measure with canonical.ScreenCost against canonical.Budget rather than
// counting runes against this constant.
const MaxScreen = canonical.MaxSeptets

// Shrink trims s until it fits one screen in whatever encoding its own
// content forces, breaking at a word boundary where it can.
//
// This is the backstop every rendered screen passes through. A screen the
// network would reject shows the user nothing; a shortened one shows them
// most of what they asked for.
func Shrink(s string) string {
	if canonical.FitsScreen(s) {
		return s
	}

	runes := []rune(s)
	// Binary search the longest prefix that still fits once the ellipsis
	// is added. Prefix cost is not linear in rune count — one emoji can
	// change the encoding of the whole string — so it is measured, not
	// estimated.
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if canonical.FitsScreen(string(runes[:mid]) + ellipsis) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return Truncate(string(runes[:lo+1]), lo)
}

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

// Paginate splits body into pages that each fit one screen once reserve
// units are set aside for a header and navigation controls.
//
// Reserve is counted in units of the page's own encoding, so a page of
// emoji gets fewer characters than a page of Latin text, which is what the
// network will actually carry.
//
// Splitting prefers paragraph breaks, then line breaks, then word
// boundaries, so a page rarely ends mid-sentence. A single word longer than
// a page is split rather than dropped: a URL is more useful broken than
// absent.
func Paginate(body string, reserve int) []string {
	if reserve >= canonical.MaxUCS2Units && canonical.EncodingOf(body) == canonical.EncodingUCS2 {
		return nil
	}
	if reserve >= MaxScreen {
		return nil
	}

	var pages []string
	remaining := strings.TrimSpace(body)

	for remaining != "" {
		runes := []rune(remaining)
		fit := fitRunes(remaining, reserve)
		if fit >= len(runes) {
			pages = append(pages, remaining)
			break
		}
		if fit == 0 {
			// Not even one character fits beside the reserve. Better to
			// stop than to loop producing empty pages.
			break
		}

		window := string(runes[:fit])
		cut := breakPoint(window)
		page := strings.TrimRightFunc(string([]rune(window)[:cut]), unicode.IsSpace)
		if page == "" {
			// No usable break: hard-split so we always make progress.
			page = window
			cut = fit
		}

		pages = append(pages, page)
		remaining = strings.TrimLeftFunc(string(runes[cut:]), unicode.IsSpace)
	}
	return pages
}

// fitRunes returns how many leading runes of s fit a screen alongside
// reserve units, measured rather than estimated because one character can
// change the encoding, and therefore the capacity, of the whole prefix.
func fitRunes(s string, reserve int) int {
	runes := []rune(s)

	fits := func(n int) bool {
		prefix := string(runes[:n])
		cost, _ := canonical.ScreenCost(prefix)
		return cost+reserve <= canonical.Budget(prefix)
	}

	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
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

// minLabelRunes is the shortest label worth rendering. Below this a menu
// line says nothing, so the option is dropped instead.
const minLabelRunes = 4

// longestLabelThatFits binary-searches the longest truncation of label
// whose menu line still fits alongside what has already been rendered.
// It returns "" when nothing worth showing fits.
func longestLabelThatFits(rendered, key, label string) string {
	runes := []rune(label)

	fits := func(n int) bool {
		return canonical.FitsScreen(rendered + "\n" + key + ". " + Truncate(label, n))
	}

	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo < minLabelRunes {
		return ""
	}
	return Truncate(label, lo)
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
		if !canonical.FitsScreen(b.String() + line) {
			// Shorten the label to the most that still fits. A shortened
			// option is still selectable; a dropped one is not.
			label := longestLabelThatFits(b.String(), item.Key, item.Label)
			if label == "" {
				break
			}
			line = "\n" + item.Key + ". " + label
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

	if !canonical.FitsScreen(title + footerText) {
		// A title and footer that fill a screen between them is a
		// programming error, but shrinking beats returning nothing.
		return Shrink(title + footerText), 0
	}

	var b strings.Builder
	b.WriteString(title)

	shown := 0
	for _, item := range items {
		line := "\n" + item.Key + ". " + item.Label
		if !canonical.FitsScreen(b.String() + line + footerText) {
			break
		}
		b.WriteString(line)
		shown++
	}

	return b.String() + footerText, shown
}

// Fits reports whether s fits one screen in the encoding its own content
// forces. Applications call it in tests; the SDK enforces it on every
// render regardless.
func Fits(s string) bool { return canonical.FitsScreen(s) }

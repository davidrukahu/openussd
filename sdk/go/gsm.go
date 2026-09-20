package openussd

import (
	"strings"

	"github.com/davidrukahu/openussd/canonical"
)

// transliterations map common non-GSM characters onto GSM ones. They are
// the characters that show up constantly in web text and cost a screen
// more than half its capacity for no benefit: curly quotes a keyboard
// produced by accident, dashes, ellipses, accented Latin outside the GSM
// table.
var transliterations = map[rune]string{
	'‘': "'", '’': "'", '‚': ",", '‛': "'",
	'“': `"`, '”': `"`, '„': `"`, '‟': `"`,
	'–': "-", '—': "-", '−': "-", '‒': "-",
	'…': "...", '•': "*", '·': ".", '‰': "%",
	'«': `"`, '»': `"`, '′': "'", '″': `"`,
	'á': "a", 'â': "a", 'ã': "a", 'å': "a", 'ā': "a",
	'é': "e", 'ê': "e", 'ë': "e", 'ē': "e",
	'í': "i", 'î': "i", 'ï': "i", 'ī': "i",
	'ó': "o", 'ô': "o", 'õ': "o", 'ō': "o",
	'ú': "u", 'û': "u", 'ū': "u",
	'ç': "c", 'š': "s", 'ž': "z", 'ý': "y",
	'Á': "A", 'Â': "A", 'Ã': "A", 'Ā': "A",
	'È': "E", 'Ê': "E", 'Ë': "E",
	'Í': "I", 'Î': "I", 'Ï': "I",
	'Ó': "O", 'Ô': "O", 'Õ': "O",
	'Ú': "U", 'Û': "U",
	' ': " ", '​': "", '️': "",
	'\t': " ",
}

// ToGSM rewrites s so it fits the GSM 03.38 alphabet, keeping a screen at
// its full 182 characters instead of dropping to 70.
//
// This is a trade, and only worth making for labels. One emoji in a menu
// of authors re-encodes the entire screen as UCS-2 and can cut a
// ten-option list to one - so for a menu, losing the emoji is plainly
// better than losing the menu. For a post's body it is the wrong trade:
// dropping the content of a message written in Chinese leaves nothing
// worth reading, so post bodies stay UCS-2 and simply paginate further.
//
// Characters with a sensible ASCII equivalent are transliterated. Anything
// else is dropped, and runs of resulting whitespace are collapsed.
func ToGSM(s string) string {
	if canonical.EncodingOf(s) == canonical.EncodingGSM7 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if canonical.EncodingOf(string(r)) == canonical.EncodingGSM7 {
			b.WriteRune(r)
			continue
		}
		if repl, ok := transliterations[r]; ok {
			b.WriteString(repl)
			continue
		}
		// Dropped characters become a space so words do not run together;
		// the collapse below removes the excess.
		b.WriteRune(' ')
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// Label prepares untrusted text for a single menu line: GSM-safe, stripped
// of line breaks, and shortened to limit. If nothing usable survives, a
// display name written entirely in a non-Latin script for instance, it
// returns fallback so the option is still selectable rather than blank.
//
// Stripping line breaks is a security property, not tidiness. A menu is
// lines of "key. label" and the user answers with a key, so text that can
// introduce a newline can introduce an option:
//
//	display name "Safaricom\n1. Free airtime, press 1"
//	renders as    1. Safaricom
//	              1. Free airtime, press 1
//
// The second line is indistinguishable from a real option. Anything drawn
// from a remote source, a fediverse display name, a post title, a
// user-supplied nickname, goes through Label rather than into a menu
// directly.
func Label(s string, limit int, fallback string) string {
	out := strings.TrimSpace(OneLine(ToGSM(s)))
	if len([]rune(out)) < 2 {
		return Truncate(fallback, limit)
	}
	return Truncate(out, limit)
}

// OneLine collapses every run of whitespace, line breaks included, into a
// single space. Control characters are removed outright.
func OneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == '\v' || r == '\f' || r == ' '
	}), " ")
}

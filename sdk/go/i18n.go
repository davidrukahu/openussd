package openussd

import "strings"

// DefaultLang is used when a session carries no language preference and
// none can be inferred.
const DefaultLang = "en"

// Bundle holds UI strings per language.
//
// Translations are part of the protocol surface, not an afterthought: a
// USSD menu in a language the user does not read is unusable in a way a
// mistranslated web page is not, because there is no page to scroll and no
// icon to recognise. The launch set is English, Swahili, and French.
type Bundle struct {
	// Fallback is the language used when a key is missing from the
	// requested one. Defaults to DefaultLang.
	Fallback string
	strings  map[string]map[string]string
}

// NewBundle builds a bundle from language code to key/value pairs.
func NewBundle(langs map[string]map[string]string) *Bundle {
	return &Bundle{Fallback: DefaultLang, strings: langs}
}

// T looks up key in lang, falling back to the fallback language and then to
// the key itself.
//
// Language codes are matched case-insensitively, so a session carrying "SW"
// resolves the same bundle as "sw". Returning the key rather than an empty
// string means a missing translation shows up as an odd-looking screen in
// testing, instead of a blank one in production.
func (b *Bundle) T(lang, key string) string {
	if b == nil {
		return key
	}
	if v, ok := b.strings[strings.ToLower(lang)][key]; ok {
		return v
	}
	fallback := b.Fallback
	if fallback == "" {
		fallback = DefaultLang
	}
	if v, ok := b.strings[strings.ToLower(fallback)][key]; ok {
		return v
	}
	return key
}

// Languages reports the language codes the bundle carries.
func (b *Bundle) Languages() []string {
	if b == nil {
		return nil
	}
	out := make([]string, 0, len(b.strings))
	for lang := range b.strings {
		out = append(out, lang)
	}
	return out
}

// Has reports whether the bundle carries a language.
func (b *Bundle) Has(lang string) bool {
	if b == nil {
		return false
	}
	_, ok := b.strings[strings.ToLower(lang)]
	return ok
}

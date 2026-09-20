package canonical

// A USSD string is carried in one of two encodings, and they do not have
// the same capacity.
//
// GSM 03.38 packs 7-bit septets, giving 182 of them in a USSD string. A
// handful of characters - ^ { } [ ] ~ | € - are not in the basic table and
// are sent as a two-septet escape sequence, so they cost double.
//
// Anything outside that alphabet forces the whole string to UCS-2, where
// the limit is 70 16-bit units. A single emoji or one Chinese character in
// an otherwise Latin screen cuts its capacity by more than half. This is
// not a detail a USSD SDK can leave to the application: content from the
// federated web is full of both.
const (
	// MaxSeptets is the capacity of a GSM 03.38 encoded screen.
	MaxSeptets = 182
	// MaxUCS2Units is the capacity of a UCS-2 encoded screen, counted in
	// 16-bit units - so a character outside the Basic Multilingual Plane,
	// such as most emoji, costs two.
	MaxUCS2Units = 70
)

// MaxBodyLen is the GSM-alphabet screen capacity.
//
// Deprecated in spirit: prefer Budget, which reports the capacity of the
// encoding a particular string actually forces. It is kept because it
// names the number every USSD document quotes.
const MaxBodyLen = MaxSeptets

// Encoding is the wire encoding a screen forces.
type Encoding string

const (
	// EncodingGSM7 is GSM 03.38, 182 septets.
	EncodingGSM7 Encoding = "gsm7"
	// EncodingUCS2 is UCS-2, 70 units.
	EncodingUCS2 Encoding = "ucs2"
)

// gsm7Basic is the GSM 03.38 basic character set. Characters in it cost
// one septet.
var gsm7Basic = buildSet(
	"@£$¥èéùìòÇ\nØø\rÅå_ÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
		"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§" +
		"¿abcdefghijklmnopqrstuvwxyzäöñüà" +
		"ΔΦΓΛΩΠΨΣΘΞ",
)

// gsm7Extended characters are reachable in GSM 03.38 only through an
// escape, so they cost two septets each.
var gsm7Extended = buildSet("^{}\\[~]|€")

func buildSet(s string) map[rune]struct{} {
	set := make(map[rune]struct{}, len(s))
	for _, r := range s {
		set[r] = struct{}{}
	}
	return set
}

// EncodingOf reports which encoding s forces.
func EncodingOf(s string) Encoding {
	for _, r := range s {
		if _, ok := gsm7Basic[r]; ok {
			continue
		}
		if _, ok := gsm7Extended[r]; ok {
			continue
		}
		return EncodingUCS2
	}
	return EncodingGSM7
}

// ScreenCost reports how much of a screen s consumes, and in which
// encoding. The unit depends on the encoding: septets for GSM 03.38,
// 16-bit units for UCS-2. Compare it against Budget for the same string.
func ScreenCost(s string) (int, Encoding) {
	cost := 0
	for _, r := range s {
		if _, ok := gsm7Basic[r]; ok {
			cost++
			continue
		}
		if _, ok := gsm7Extended[r]; ok {
			cost += 2
			continue
		}
		// One non-GSM character forces UCS-2 for the whole string, so the
		// GSM tally is abandoned rather than adjusted.
		return ucs2Units(s), EncodingUCS2
	}
	return cost, EncodingGSM7
}

// ucs2Units counts UTF-16 code units without allocating. A character outside
// the Basic Multilingual Plane is a surrogate pair and costs two.
//
// This is the hot path: the SDK's budget searches call ScreenCost once per
// probe, so encoding the string to []uint16 just to take its length turned a
// screen render into megabytes of garbage.
func ucs2Units(s string) int {
	units := 0
	for _, r := range s {
		if r > 0xFFFF {
			units += 2
			continue
		}
		units++
	}
	return units
}

// Budget reports the capacity available to s, given the encoding its own
// content forces.
func Budget(s string) int {
	if EncodingOf(s) == EncodingUCS2 {
		return MaxUCS2Units
	}
	return MaxSeptets
}

// FitsScreen reports whether s fits one USSD screen.
func FitsScreen(s string) bool {
	cost, enc := ScreenCost(s)
	if enc == EncodingUCS2 {
		return cost <= MaxUCS2Units
	}
	return cost <= MaxSeptets
}

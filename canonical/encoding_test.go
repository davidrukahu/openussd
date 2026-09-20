package canonical

import (
	"strings"
	"testing"
)

func TestEncodingOf(t *testing.T) {
	tests := []struct {
		name, in string
		want     Encoding
	}{
		{"ascii", "Hello", EncodingGSM7},
		{"swahili", "Habari yako rafiki", EncodingGSM7},
		{"french accents in the GSM table", "à é ù ì ò Ä Ö Ñ Ü", EncodingGSM7},
		{"gsm extension characters", "a[b]c{d}e~f|g^h€", EncodingGSM7},
		{"emoji", "hi \U0001f30d", EncodingUCS2},
		{"chinese", "台灣", EncodingUCS2},
		{"curly quote", "“hi”", EncodingUCS2},
		{"empty", "", EncodingGSM7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EncodingOf(tc.in); got != tc.want {
				t.Errorf("EncodingOf(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestExtensionCharactersCostTwo pins a detail that is easy to miss: the
// GSM escape table is reachable only through a two-septet sequence, so a
// screen of braces holds half as much as a screen of letters.
func TestExtensionCharactersCostTwo(t *testing.T) {
	cost, enc := ScreenCost("{}")
	if enc != EncodingGSM7 {
		t.Fatalf("encoding = %s, want gsm7", enc)
	}
	if cost != 4 {
		t.Errorf("cost = %d, want 4", cost)
	}
}

func TestScreenCostCountsUTF16UnitsForUCS2(t *testing.T) {
	// An emoji outside the Basic Multilingual Plane is a surrogate pair,
	// so it costs two of the screen's 70 units, not one.
	cost, enc := ScreenCost("\U0001f30d")
	if enc != EncodingUCS2 {
		t.Fatalf("encoding = %s, want ucs2", enc)
	}
	if cost != 2 {
		t.Errorf("cost = %d, want 2", cost)
	}
}

func TestFitsScreen(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"182 GSM characters", strings.Repeat("a", 182), true},
		{"183 GSM characters", strings.Repeat("a", 183), false},
		{"70 UCS-2 units", strings.Repeat("台", 70), true},
		{"71 UCS-2 units", strings.Repeat("台", 71), false},
		{"35 emoji is 70 units", strings.Repeat("\U0001f30d", 35), true},
		{"36 emoji is 72 units", strings.Repeat("\U0001f30d", 36), false},
		{"91 GSM extension characters", strings.Repeat("{", 91), true},
		{"92 GSM extension characters", strings.Repeat("{", 92), false},
		{"latin text with one emoji", strings.Repeat("a", 100) + "\U0001f30d", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FitsScreen(tc.in); got != tc.want {
				cost, enc := ScreenCost(tc.in)
				t.Errorf("FitsScreen() = %v, want %v (cost %d %s, budget %d)",
					got, tc.want, cost, enc, Budget(tc.in))
			}
		})
	}
}

func TestResponseValidateUsesTheRightBudget(t *testing.T) {
	if err := Continue(strings.Repeat("a", 182)).Validate(); err != nil {
		t.Errorf("182 GSM characters rejected: %v", err)
	}
	err := Continue(strings.Repeat("a", 100) + "\U0001f30d").Validate()
	if err == nil {
		t.Fatal("a 101-character UCS-2 screen was accepted")
	}
	if !strings.Contains(err.Error(), "ucs2") {
		t.Errorf("error = %q, want it to name the encoding", err)
	}
}

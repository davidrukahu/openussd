package openussd

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidrukahu/openussd/canonical"
)

// canonicalCost and budgetOf keep the encoding details in one place for
// the assertions below.
func canonicalCost(s string) (int, canonical.Encoding) { return canonical.ScreenCost(s) }
func budgetOf(s string) int                            { return canonical.Budget(s) }

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"short enough", "hello", 10, "hello"},
		{"exactly at the limit", "hello", 5, "hello"},
		{"breaks at a word", "the quick brown fox", 15, "the quick..."},
		{"mid-word when no break is close", "supercalifragilistic", 10, "superca..."},
		{"limit below the marker", "hello", 1, "."},
		{"limit at the marker", "hello", 3, "..."},
		{"zero limit", "hello", 0, ""},
		{"multibyte counted as runes", "Habari yako rafiki yangu", 12, "Habari..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.in, tc.limit)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
			if n := utf8.RuneCountInString(got); n > tc.limit {
				t.Errorf("result is %d runes, over the %d limit", n, tc.limit)
			}
		})
	}
}

// TestTruncateNeverExceedsLimit is the property that matters: every screen
// the SDK produces goes through here, and a screen over budget is rejected
// by the gateway.
func TestTruncateNeverExceedsLimit(t *testing.T) {
	inputs := []string{
		"",
		"short",
		strings.Repeat("a", 500),
		strings.Repeat("word ", 100),
		strings.Repeat("Habari ", 60),
		strings.Repeat("🌍", 200),
		"no-spaces-at-all-" + strings.Repeat("x", 300),
	}

	for _, in := range inputs {
		for _, limit := range []int{1, 2, 7, 40, MaxScreen} {
			got := Truncate(in, limit)
			if n := utf8.RuneCountInString(got); n > limit {
				t.Errorf("Truncate(%.20q…, %d) produced %d runes", in, limit, n)
			}
		}
	}
}

func TestPaginate(t *testing.T) {
	t.Run("short body is one page", func(t *testing.T) {
		pages := Paginate("hello world", 20)
		if len(pages) != 1 || pages[0] != "hello world" {
			t.Errorf("pages = %q", pages)
		}
	})

	t.Run("every page fits the budget", func(t *testing.T) {
		reserve := 20
		body := strings.Repeat("Mastodon post text that goes on. ", 40)

		pages := Paginate(body, reserve)
		if len(pages) < 2 {
			t.Fatalf("got %d pages, expected the body to split", len(pages))
		}
		for i, p := range pages {
			if n := utf8.RuneCountInString(p); n > MaxScreen-reserve {
				t.Errorf("page %d is %d runes, over the %d budget", i, n, MaxScreen-reserve)
			}
		}
	})

	t.Run("no content is lost", func(t *testing.T) {
		body := "alpha beta gamma delta epsilon " + strings.Repeat("zeta ", 80)
		joined := strings.Join(Paginate(body, 20), " ")

		wantWords := strings.Fields(body)
		gotWords := strings.Fields(joined)
		if len(gotWords) != len(wantWords) {
			t.Errorf("word count changed: %d pages words vs %d original", len(gotWords), len(wantWords))
		}
	})

	t.Run("unbreakable text still makes progress", func(t *testing.T) {
		pages := Paginate(strings.Repeat("x", 1000), 20)
		if len(pages) == 0 {
			t.Fatal("no pages produced")
		}
		for i, p := range pages {
			if utf8.RuneCountInString(p) > MaxScreen-20 {
				t.Errorf("page %d exceeds the budget", i)
			}
		}
	})

	t.Run("reserve larger than the screen yields nothing", func(t *testing.T) {
		if pages := Paginate("text", MaxScreen+1); pages != nil {
			t.Errorf("pages = %q, want nil", pages)
		}
	})
}

func TestMenu(t *testing.T) {
	t.Run("renders numbered options", func(t *testing.T) {
		got := Menu("Fediverse", []MenuItem{{Key: "1", Label: "Timeline"}, {Key: "2", Label: "Search"}})
		want := "Fediverse\n1. Timeline\n2. Search"
		if got != want {
			t.Errorf("Menu() = %q, want %q", got, want)
		}
	})

	t.Run("stays inside the budget with long labels", func(t *testing.T) {
		items := make([]MenuItem, 0, 9)
		for i := 1; i <= 9; i++ {
			items = append(items, MenuItem{Key: string(rune('0' + i)), Label: strings.Repeat("long label ", 5)})
		}

		got := Menu("Pick one", items)
		if !Fits(got) {
			t.Errorf("menu is %d runes, over the %d budget", utf8.RuneCountInString(got), MaxScreen)
		}
	})
}

// TestMenuFitAlwaysRendersTheFooter is the bug this primitive exists for:
// a list long enough to fill the screen used to push "Quit" off it, leaving
// the user on a menu with no exit.
func TestMenuFitAlwaysRendersTheFooter(t *testing.T) {
	items := make([]MenuItem, 0, 20)
	for i := 1; i <= 20; i++ {
		items = append(items, MenuItem{Key: itoa(i), Label: "A reasonably long author name"})
	}
	footer := []MenuItem{{Key: "0", Label: "Quit"}}

	got, shown := MenuFit("Latest posts", items, footer)

	if !strings.HasSuffix(got, "\n0. Quit") {
		t.Errorf("screen = %q, want it to end with the footer", got)
	}
	if !Fits(got) {
		t.Errorf("screen is %d runes, over the %d budget", utf8.RuneCountInString(got), MaxScreen)
	}
	if shown == 0 || shown >= len(items) {
		t.Errorf("shown = %d, want a partial list out of %d", shown, len(items))
	}
	if strings.Contains(got, itoa(shown+1)+". ") {
		t.Errorf("screen lists item %d but reported only %d shown", shown+1, shown)
	}
}

func TestMenuFitShortListShowsEverything(t *testing.T) {
	items := []MenuItem{{Key: "1", Label: "One"}, {Key: "2", Label: "Two"}}
	got, shown := MenuFit("Title", items, []MenuItem{{Key: "0", Label: "Back"}})

	if shown != 2 {
		t.Errorf("shown = %d, want 2", shown)
	}
	if got != "Title\n1. One\n2. Two\n0. Back" {
		t.Errorf("screen = %q", got)
	}
}

// TestEncodingAwareBudget is the constraint the project's own README gets
// wrong if it only ever says "182 characters": one non-GSM character
// re-encodes the whole screen as UCS-2, where the limit is 70 units.
func TestEncodingAwareBudget(t *testing.T) {
	latin := strings.Repeat("a", 182)
	if !Fits(latin) {
		t.Errorf("182 GSM characters should fit one screen")
	}
	if Fits(latin + "a") {
		t.Errorf("183 GSM characters should not fit")
	}

	emoji := strings.Repeat("a", 100) + "🌍"
	if Fits(emoji) {
		t.Errorf("100 characters plus an emoji is UCS-2 and must not fit a 70-unit screen")
	}

	swahili := strings.Repeat("Habari ", 20)
	if !Fits(swahili) {
		t.Errorf("Swahili is in the GSM alphabet and should get the full screen")
	}
}

func TestShrinkFitsEveryEncoding(t *testing.T) {
	inputs := []string{
		strings.Repeat("a", 500),
		strings.Repeat("Habari yako ", 40),
		strings.Repeat("🌍", 120),
		strings.Repeat("台灣", 200),
		"mixed " + strings.Repeat("text with émoji 🎉 ", 30),
		"Tâi Siáu-káu 台痟狗 " + strings.Repeat("content ", 50),
	}

	for _, in := range inputs {
		got := Shrink(in)
		if !Fits(got) {
			cost, enc := canonicalCost(got)
			t.Errorf("Shrink(%.20q…) still costs %d %s units", in, cost, enc)
		}
		if got == "" {
			t.Errorf("Shrink(%.20q…) returned nothing", in)
		}
	}
}

func TestShrinkLeavesFittingTextAlone(t *testing.T) {
	in := "1. Timeline\n2. Quit"
	if got := Shrink(in); got != in {
		t.Errorf("Shrink() = %q, want the input unchanged", got)
	}
}

// TestPaginateRespectsEncoding: an emoji-heavy post must produce more,
// smaller pages, not pages the network will refuse.
func TestPaginateRespectsEncoding(t *testing.T) {
	reserve := 20
	body := strings.Repeat("Habari 🌍 dunia nzima. ", 30)

	pages := Paginate(body, reserve)
	if len(pages) < 2 {
		t.Fatalf("got %d pages, expected the body to split", len(pages))
	}
	for i, p := range pages {
		cost, _ := canonicalCost(p)
		if !Fits(p) || cost+reserve > budgetOf(p) {
			t.Errorf("page %d costs %d units plus %d reserve, over budget", i, cost, reserve)
		}
	}
}

func TestMenuFitsWithNonLatinLabels(t *testing.T) {
	items := make([]MenuItem, 0, 10)
	for i := 1; i <= 10; i++ {
		items = append(items, MenuItem{Key: itoa(i), Label: "Tâi Siáu-káu 台痟狗 ㄊㄇㄉ 🇳🇫 台灣國"})
	}

	plain := Menu("Latest posts", items)
	if !Fits(plain) {
		cost, enc := canonicalCost(plain)
		t.Errorf("Menu with UCS-2 labels costs %d %s units", cost, enc)
	}

	withFooter, shown := MenuFit("Latest posts", items, []MenuItem{{Key: "0", Label: "Quit"}})
	if !Fits(withFooter) {
		t.Errorf("MenuFit with UCS-2 labels does not fit: %q", withFooter)
	}
	if !strings.HasSuffix(withFooter, "\n0. Quit") {
		t.Errorf("screen = %q, want the footer kept", withFooter)
	}
	if shown < 1 {
		t.Errorf("shown = %d, want at least one option", shown)
	}
}

// BenchmarkPaginate guards the cost of the operation a post screen performs
// on every keypress. Both budget searches are bounded by the screen size, so
// a long post should cost roughly its length, not its length squared.
func BenchmarkPaginate(b *testing.B) {
	cases := map[string]string{
		"gsm":  strings.Repeat("A long post about USSD and the fediverse. ", 120),
		"ucs2": strings.Repeat("台灣國 a long post about USSD and the fediverse. ", 100),
	}

	for name, body := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Paginate(body, 60)
			}
		})
	}
}

// TestTruncationMarkerStaysInTheGSMAlphabet is the regression test for the
// worst budget bug in this package's history: a "…" marker is not in GSM
// 03.38, so it re-encoded every truncated screen as UCS-2 and cut its
// capacity from 182 to 70. Truncate then returned strings the gateway
// rejected.
func TestTruncationMarkerStaysInTheGSMAlphabet(t *testing.T) {
	if canonical.EncodingOf(ellipsis) != canonical.EncodingGSM7 {
		t.Fatalf("the truncation marker %q is not GSM-safe", ellipsis)
	}

	ascii := strings.Repeat("plain ascii text ", 40)

	got := Truncate(ascii, 150)
	if _, enc := canonical.ScreenCost(got); enc != canonical.EncodingGSM7 {
		t.Errorf("Truncate re-encoded ASCII as %s", enc)
	}
	if !Fits(got) {
		t.Error("Truncate returned a screen the gateway would reject")
	}

	// Shrink must use the whole GSM budget, not the UCS-2 one.
	if n := len([]rune(Shrink(ascii))); n < 170 {
		t.Errorf("Shrink kept only %d of 182 available characters", n)
	}
}

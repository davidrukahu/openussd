package openussd

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"short enough", "hello", 10, "hello"},
		{"exactly at the limit", "hello", 5, "hello"},
		{"breaks at a word", "the quick brown fox", 15, "the quick…"},
		{"mid-word when no break is close", "supercalifragilistic", 10, "supercali…"},
		{"limit of one", "hello", 1, "…"},
		{"zero limit", "hello", 0, ""},
		{"multibyte counted as runes", "Habari yako rafiki yangu", 12, "Habari yako…"},
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

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	openussd "github.com/davidrukahu/openussd/sdk/go"
)

// timelineSize is how many posts one dialogue works with. Enough to be
// worth browsing, few enough that the session blob stays small — the whole
// list rides in session state so that paging back does not re-fetch.
const timelineSize = 10

// state is this application's session state. Field names are short because
// every byte is stored per session and travels on every turn.
type state struct {
	Posts []Post `json:"p,omitempty"`
	// Cursor is the index of the post being read.
	Cursor int `json:"c,omitempty"`
	// Page is the page within that post's paginated body.
	Page int `json:"g,omitempty"`
	// Listed is how many posts the last timeline screen actually showed.
	Listed int `json:"n,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fediverse: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", envOr("FEDIVERSE_LISTEN", ":8081"), "address to listen on")
	// Not mastodon.social: it requires an authenticated user for the
	// public timeline API, which a feature phone cannot provide. Any
	// instance that still serves that endpoint openly will do.
	instance := flag.String("instance", envOr("FEDIVERSE_INSTANCE", "https://mstdn.social"), "Mastodon instance base URL")
	flag.Parse()

	secret := os.Getenv("FEDIVERSE_WEBHOOK_SECRET")
	if secret == "" {
		// Without the shared secret this application cannot tell a real
		// gateway from anyone who found its URL, so refuse to start rather
		// than serve forged dialogues.
		return fmt.Errorf("FEDIVERSE_WEBHOOK_SECRET is not set")
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := &Mastodon{Instance: *instance}

	app, err := newApp(client, log)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/ussd", openussd.NewHandler(app, secret, log))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Info("fediverse adapter listening", "addr", *listen, "instance", *instance)
	return srv.ListenAndServe()
}

// newApp declares the dialogue. This is the whole application: three
// screens, in about as many lines as the README promises.
func newApp(client *Mastodon, log *slog.Logger) (*openussd.App[state], error) {
	if log == nil {
		log = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	bundle := openussd.NewBundle(map[string]map[string]string{
		"en": {
			"menu.title":     "OpenUSSD Fediverse",
			"menu.timeline":  "Public timeline",
			"menu.about":     "About",
			"menu.quit":      "Quit",
			"timeline.title": "Latest posts",
			"timeline.back":  "0. Back",
			"nav.next":       "1. Next",
			"nav.prev":       "2. Prev",
			"nav.back":       "0. Back",
			"error.fetch":    "Could not reach the instance. Try again.",
			"error.choice":   "Invalid choice.",
			"session.ended":  "Goodbye.",
			"about.body":     "OpenUSSD: an open gateway and SDK bringing the federated web to feature phones. github.com/davidrukahu/openussd",
		},
		// Launch-language stubs. Real translations land with M3
		// documentation; the keys exist now so screens are never written
		// against hard-coded English.
		"sw": {
			"menu.title":    "OpenUSSD Fediverse",
			"menu.timeline": "Ratiba ya umma",
			"menu.about":    "Kuhusu",
			"menu.quit":     "Ondoka",
			"error.choice":  "Chaguo si sahihi.",
			"session.ended": "Kwaheri.",
		},
		"fr": {
			"menu.title":    "OpenUSSD Fediverse",
			"menu.timeline": "Fil public",
			"menu.about":    "À propos",
			"menu.quit":     "Quitter",
			"error.choice":  "Choix invalide.",
			"session.ended": "Au revoir.",
		},
	})

	menu := openussd.Screen[state]{
		Name: "menu",
		Prompt: func(c *openussd.Context[state]) (string, error) {
			return openussd.Menu(c.T("menu.title"), []openussd.MenuItem{
				{Key: "1", Label: c.T("menu.timeline")},
				{Key: "2", Label: c.T("menu.about")},
				{Key: "0", Label: c.T("menu.quit")},
			}), nil
		},
		Handle: func(c *openussd.Context[state], input string) (openussd.Action, error) {
			switch input {
			case "1":
				posts, err := client.PublicTimeline(context.Background(), timelineSize)
				if err != nil {
					// A failing instance is not a failing dialogue: keep
					// the user on the menu with an explanation. The reason
					// goes to the operator, who can act on it — an
					// instance that has closed its public timeline to
					// unauthenticated reads looks identical to one that is
					// down, from a handset.
					log.Error("could not fetch timeline", "error", err, "session_id", c.Event.SessionID)
					return openussd.Stay(c.T("error.fetch"))
				}
				c.State.Posts = posts
				return openussd.Goto("timeline")
			case "2":
				return openussd.Goto("about")
			case "0":
				return openussd.Finish(c.T("session.ended"))
			default:
				return openussd.Stay(c.T("error.choice"))
			}
		},
	}

	timeline := openussd.Screen[state]{
		Name: "timeline",
		Prompt: func(c *openussd.Context[state]) (string, error) {
			items := make([]openussd.MenuItem, 0, len(c.State.Posts))
			for i, p := range c.State.Posts {
				// Author and age first: they are what makes a one-line
				// preview worth selecting.
				label := openussd.Truncate(p.Author, maxHeaderLen)
				if p.When != "" {
					label += " (" + p.When + ")"
				}
				items = append(items, openussd.MenuItem{Key: strconv.Itoa(i + 1), Label: label})
			}

			// Quit is a footer item so it survives a list too long for one
			// screen. Whatever does not fit is dropped from the list, and
			// the count tells the handler which keys are real.
			screen, shown := openussd.MenuFit(c.T("timeline.title"), items,
				[]openussd.MenuItem{{Key: "0", Label: c.T("menu.quit")}})
			c.State.Listed = shown
			return screen, nil
		},
		Handle: func(c *openussd.Context[state], input string) (openussd.Action, error) {
			if input == "0" {
				return openussd.Finish(c.T("session.ended"))
			}
			// Only the posts actually listed are selectable: offering a
			// key the user could not see is how a menu loses trust.
			n, err := strconv.Atoi(strings.TrimSpace(input))
			if err != nil || n < 1 || n > c.State.Listed {
				return openussd.Stay(c.T("error.choice"))
			}
			c.State.Cursor = n - 1
			c.State.Page = 0
			return openussd.Goto("post")
		},
	}

	post := openussd.Screen[state]{
		Name:   "post",
		Prompt: func(c *openussd.Context[state]) (string, error) { return renderPost(c), nil },
		Handle: func(c *openussd.Context[state], input string) (openussd.Action, error) {
			pages := postPages(c)

			// A control that is not offered on this page is not valid on
			// it either. Silently re-rendering the same screen would look
			// like the handset dropped the keypress.
			switch input {
			case "1":
				if c.State.Page >= len(pages)-1 {
					return openussd.Stay(c.T("error.choice"))
				}
				c.State.Page++
				return openussd.Goto("post")
			case "2":
				if c.State.Page == 0 {
					return openussd.Stay(c.T("error.choice"))
				}
				c.State.Page--
				return openussd.Goto("post")
			case "0":
				return openussd.Goto("timeline")
			default:
				return openussd.Stay(c.T("error.choice"))
			}
		},
	}

	about := openussd.Screen[state]{
		Name: "about",
		// No Handle: the dialogue ends with this screen displayed.
		Prompt: func(c *openussd.Context[state]) (string, error) { return c.T("about.body"), nil },
	}

	app, err := openussd.NewApp("menu", menu, timeline, post, about)
	if err != nil {
		return nil, err
	}
	return app.WithBundle(bundle), nil
}

// navReserve is the space kept aside on every post screen for the header
// and the navigation footer, so pagination never overruns the budget.
const navReserve = 60

// maxHeaderLen bounds the author line on a post screen. navReserve budgets
// for this plus the navigation footer.
const maxHeaderLen = 28

func postPages(c *openussd.Context[state]) []string {
	if c.State.Cursor >= len(c.State.Posts) {
		return nil
	}
	return openussd.Paginate(c.State.Posts[c.State.Cursor].Text, navReserve)
}

// renderPost shows one page of a post with only the controls that lead
// somewhere: offering "Next" on the last page trains users to distrust the
// menu.
func renderPost(c *openussd.Context[state]) string {
	pages := postPages(c)
	if len(pages) == 0 {
		return c.T("error.fetch")
	}
	if c.State.Page >= len(pages) {
		c.State.Page = len(pages) - 1
	}

	p := c.State.Posts[c.State.Cursor]
	// The header is bounded so a long display name cannot push the
	// navigation controls off the bottom of the screen: a user who cannot
	// see "0. Back" is stuck.
	header := openussd.Truncate(p.Author, maxHeaderLen)
	if len(pages) > 1 {
		header += fmt.Sprintf(" (%d/%d)", c.State.Page+1, len(pages))
	}

	nav := []string{}
	if c.State.Page < len(pages)-1 {
		nav = append(nav, c.T("nav.next"))
	}
	if c.State.Page > 0 {
		nav = append(nav, c.T("nav.prev"))
	}
	nav = append(nav, c.T("nav.back"))

	return header + "\n" + pages[c.State.Page] + "\n" + strings.Join(nav, " ")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

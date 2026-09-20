// Command ussdsim is a terminal handset for a local OpenUSSD gateway.
//
// It dials a shortcode against the gateway's simulator endpoint and reads
// back screens the way a feature phone would, including the 182-character
// limit — so a screen that would be unreadable on a handset is unreadable
// here too.
//
// It exists so the project can be demonstrated without a telco account, a
// sandbox registration, or an inbound tunnel: clone, `docker compose up`,
// dial.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/davidrukahu/openussd/canonical"
)

// screenWidth is the display width of the fake handset. Feature-phone
// screens are narrow; wrapping at terminal width would make menus look
// roomier than they are.
const screenWidth = 32

type request struct {
	SessionID string          `json:"session_id"`
	MSISDN    string          `json:"msisdn"`
	Shortcode string          `json:"shortcode"`
	Path      []string        `json:"path"`
	Phase     canonical.Phase `json:"phase"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ussdsim: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	gateway := flag.String("gateway", "http://localhost:8080", "gateway base URL")
	shortcode := flag.String("shortcode", "*384*1234#", "shortcode to dial")
	msisdn := flag.String("msisdn", "+254711223344", "subscriber number to claim")
	flag.Parse()

	endpoint := strings.TrimRight(*gateway, "/") + "/ussd/simulator"
	sessionID := fmt.Sprintf("SIM_%d_%04d", time.Now().Unix(), rand.Intn(10000))
	client := &http.Client{Timeout: 15 * time.Second}
	in := bufio.NewScanner(os.Stdin)

	fmt.Printf("Dialling %s as %s\n", *shortcode, *msisdn)

	var path []string
	phase := canonical.PhaseBegin

	for {
		resp, err := send(client, endpoint, request{
			SessionID: sessionID,
			MSISDN:    *msisdn,
			Shortcode: *shortcode,
			Path:      path,
			Phase:     phase,
		})
		if err != nil {
			return err
		}

		draw(resp.Body, resp.EndSession)
		if resp.EndSession {
			return nil
		}

		fmt.Print("Reply: ")
		if !in.Scan() {
			// The user hung up. Tell the gateway, so it can release the
			// session instead of holding it until the idle timeout.
			fmt.Println()
			return cancel(client, endpoint, request{
				SessionID: sessionID, MSISDN: *msisdn, Shortcode: *shortcode,
				Path: path, Phase: canonical.PhaseCancel,
			})
		}

		path = append(path, strings.TrimSpace(in.Text()))
		phase = canonical.PhaseContinue
	}
}

func send(client *http.Client, endpoint string, req request) (canonical.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return canonical.Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return canonical.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return canonical.Response{}, fmt.Errorf("reaching the gateway at %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return canonical.Response{}, fmt.Errorf("gateway replied %s", resp.Status)
	}

	var out canonical.Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return canonical.Response{}, fmt.Errorf("decoding gateway reply: %w", err)
	}
	return out, nil
}

func cancel(client *http.Client, endpoint string, req request) error {
	_, err := send(client, endpoint, req)
	return err
}

// draw prints the screen inside a handset-sized frame, and flags a screen
// that would not fit a real one.
func draw(body string, ended bool) {
	border := "+" + strings.Repeat("-", screenWidth+2) + "+"

	fmt.Println()
	fmt.Println(border)
	for _, line := range wrap(body, screenWidth) {
		fmt.Printf("| %-*s |\n", screenWidth, line)
	}
	fmt.Println(border)

	if n := len([]rune(body)); n > canonical.MaxBodyLen {
		fmt.Printf("!! %d characters: a real network would reject this screen\n", n)
	}
	if ended {
		fmt.Println("(session ended)")
	}
	fmt.Println()
}

// wrap breaks text at width, preferring word boundaries.
func wrap(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}

		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case len([]rune(line))+1+len([]rune(word)) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}

			// A single word longer than the screen still has to appear.
			for len([]rune(line)) > width {
				runes := []rune(line)
				out = append(out, string(runes[:width]))
				line = string(runes[width:])
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

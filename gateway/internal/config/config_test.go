package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidrukahu/openussd/gateway/internal/session"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

const minimal = `
adapters:
  simulator:
    enabled: true
tenants:
  - name: demo
    shortcode: "*384*1234#"
    webhook_url: "http://localhost:8081/ussd"
    secret: "${DEMO_SECRET}"
`

func TestLoadExpandsSecretsFromTheEnvironment(t *testing.T) {
	t.Setenv("DEMO_SECRET", "s3cret")

	cfg, err := Load(write(t, minimal))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Tenants[0].Secret; got != "s3cret" {
		t.Errorf("secret = %q, want it expanded", got)
	}
	if cfg.Listen != ":8080" || cfg.Session.Backend != "memory" || cfg.Session.TTL != session.DefaultTTL {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadReportsEveryUnsetVariable(t *testing.T) {
	body := strings.ReplaceAll(minimal, `"${DEMO_SECRET}"`, `"${MISSING_ONE}"`) + `
    timeout: 5s
`
	body = strings.ReplaceAll(body, `"*384*1234#"`, `"${MISSING_TWO}"`)

	_, err := Load(write(t, body))
	if err == nil {
		t.Fatal("expected an error for unset variables")
	}
	for _, want := range []string{"MISSING_ONE", "MISSING_TWO"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %s", err, want)
		}
	}
}

// TestCommentsMayDocumentTheSyntax is a regression test: expanding the
// file's text rather than its parsed values made a ${NAME} in a comment
// fail the load, so the example config could not document itself.
func TestCommentsMayDocumentTheSyntax(t *testing.T) {
	t.Setenv("DEMO_SECRET", "s3cret")
	body := "# Values may use ${NAME} to read from the environment.\n" + minimal

	if _, err := Load(write(t, body)); err != nil {
		t.Fatalf("a ${NAME} in a comment broke the load: %v", err)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name, body, wantErr string
	}{
		{
			name:    "no tenants",
			body:    "adapters:\n  simulator:\n    enabled: true\n",
			wantErr: "no tenants configured",
		},
		{
			name: "no adapters",
			body: `
tenants:
  - name: demo
    shortcode: "*1#"
    webhook_url: "http://x/ussd"
    secret: "s"
`,
			wantErr: "no adapters enabled",
		},
		{
			name: "redis backend without a url",
			body: `
session:
  backend: redis
adapters:
  simulator:
    enabled: true
tenants:
  - name: demo
    shortcode: "*1#"
    webhook_url: "http://x/ussd"
    secret: "s"
`,
			wantErr: "no redis_url",
		},
		{
			name: "unknown backend",
			body: `
session:
  backend: postgres
adapters:
  simulator:
    enabled: true
tenants:
  - name: demo
    shortcode: "*1#"
    webhook_url: "http://x/ussd"
    secret: "s"
`,
			wantErr: "unknown session backend",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestSimulatorExposedWarnsOnlyOffLoopback(t *testing.T) {
	tests := []struct {
		listen string
		want   bool
	}{
		{"127.0.0.1:8080", false},
		{"localhost:8080", false},
		{":8080", true},
		{"0.0.0.0:8080", true},
	}

	for _, tc := range tests {
		cfg := &Config{Listen: tc.listen, Adapters: map[string]AdapterConfig{"simulator": {Enabled: true}}}
		if got := cfg.SimulatorExposed(); got != tc.want {
			t.Errorf("SimulatorExposed() for %q = %v, want %v", tc.listen, got, tc.want)
		}
	}

	off := &Config{Listen: ":8080", Adapters: map[string]AdapterConfig{"simulator": {Enabled: false}}}
	if off.SimulatorExposed() {
		t.Error("a disabled simulator was reported as exposed")
	}
}

// TestExampleConfigLoads keeps the shipped example honest: it is the first
// thing anyone runs, and a broken one wastes their first ten minutes.
func TestExampleConfigLoads(t *testing.T) {
	t.Setenv("FEDIVERSE_WEBHOOK_SECRET", "s3cret")

	cfg, err := Load("../../gateway.example.yaml")
	if err != nil {
		t.Fatalf("the shipped example config does not load: %v", err)
	}
	if len(cfg.Tenants) == 0 || cfg.Tenants[0].Secret != "s3cret" {
		t.Errorf("example config did not expand its secret: %+v", cfg.Tenants)
	}
	if cfg.Session.TTL != 180*time.Second {
		t.Errorf("example TTL = %v, want 180s", cfg.Session.TTL)
	}
}

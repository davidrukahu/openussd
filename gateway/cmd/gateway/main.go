// Command gateway runs the OpenUSSD gateway: it accepts USSD callbacks from
// one or more networks, keeps dialogues continuous, and forwards canonical
// session events to tenant applications over signed webhooks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidrukahu/openussd/gateway/internal/adapter"
	"github.com/davidrukahu/openussd/gateway/internal/adapter/africastalking"
	"github.com/davidrukahu/openussd/gateway/internal/adapter/simulator"
	"github.com/davidrukahu/openussd/gateway/internal/config"
	"github.com/davidrukahu/openussd/gateway/internal/httpx"
	"github.com/davidrukahu/openussd/gateway/internal/session"
	"github.com/davidrukahu/openussd/gateway/internal/tenant"
)

// shutdownGrace bounds how long in-flight dialogues have to finish on
// shutdown. USSD turns are sub-second; anything still running after this
// has already lost its handset.
const shutdownGrace = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "gateway.yaml", "path to the gateway configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := newSessionStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	router, err := tenant.NewRouter(cfg.Tenants)
	if err != nil {
		return err
	}

	registry, err := newRegistry(cfg)
	if err != nil {
		return err
	}

	if cfg.SimulatorExposed() {
		log.Warn("the simulator adapter is enabled on a non-loopback address; " +
			"it performs no authenticity checks and must not be reachable in production")
	}

	mux := http.NewServeMux()
	dispatcher := tenant.NewDispatcher(nil)
	for _, name := range registry.Names() {
		a, _ := registry.Lookup(name)
		path := "/ussd/" + name
		mux.Handle(path, httpx.NewInbound(a, store, router, dispatcher, log))
		log.Info("mounted inbound endpoint", "adapter", name, "path", path)
	}
	mux.HandleFunc("/healthz", httpx.Health)
	mux.Handle("/readyz", httpx.Ready(registry.Names(), router.Tenants()))

	for _, t := range router.Tenants() {
		log.Info("routing tenant", "tenant", t.Name, "shortcode", t.Shortcode, "prefix", t.Prefix)
	}

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
		// USSD turns are short. Generous timeouts here only hold sockets
		// open for dialogues the network has already given up on.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("gateway listening", "addr", cfg.Listen, "sessions", cfg.Session.Backend)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

func newSessionStore(ctx context.Context, cfg *config.Config) (session.Store, error) {
	if cfg.Session.Backend == "redis" {
		return session.NewRedis(ctx, cfg.Session.RedisURL, session.WithRedisTTL(cfg.Session.TTL))
	}
	return session.NewMemory(session.WithTTL(cfg.Session.TTL)), nil
}

// newRegistry builds the adapter set from config, wrapping any adapter that
// has an allowlist in adapter.TrustedProxy.
func newRegistry(cfg *config.Config) (*adapter.Registry, error) {
	available := map[string]adapter.Adapter{
		africastalking.Name: africastalking.New(),
		simulator.Name:      simulator.New(),
	}

	registry := adapter.NewRegistry()
	for name, ac := range cfg.Adapters {
		if !ac.Enabled {
			continue
		}
		base, ok := available[name]
		if !ok {
			return nil, fmt.Errorf("config: unknown adapter %q", name)
		}

		if adapter.NeedsAllowlist(base) && len(ac.AllowedSources) == 0 {
			// Failing closed: this adapter's provider authenticates
			// nothing, so without an allowlist the endpoint would accept
			// forged input from anywhere that can reach it.
			return nil, fmt.Errorf(
				"adapter %q is enabled with no allowed_sources: it would accept forged callbacks from anywhere", name)
		}

		guarded := base
		if len(ac.AllowedSources) > 0 {
			allowed, err := adapter.ParsePrefixes(ac.AllowedSources)
			if err != nil {
				return nil, fmt.Errorf("adapter %q: allowed_sources: %w", name, err)
			}
			forwarders, err := adapter.ParsePrefixes(ac.TrustedForwarders)
			if err != nil {
				return nil, fmt.Errorf("adapter %q: trusted_forwarders: %w", name, err)
			}
			guarded = &adapter.TrustedProxy{Adapter: base, Allowed: allowed, Forwarders: forwarders}
		}

		if err := registry.Register(guarded); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

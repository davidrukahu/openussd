// Package config loads and validates the gateway's configuration.
//
// Configuration is a YAML file with environment-variable expansion, so
// secrets stay in the environment and the file itself is safe to commit as
// an example. A missing variable is an error rather than an empty string:
// silently starting with an empty webhook secret is the failure mode this
// package exists to prevent.
//
// Expansion happens on parsed values, not on the file's text. Substituting
// before parsing is simpler but also blind to comments, so documenting the
// syntax in a comment would make the file fail to load.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/davidrukahu/openussd/gateway/internal/session"
	"github.com/davidrukahu/openussd/gateway/internal/tenant"
)

// Config is the whole gateway configuration.
type Config struct {
	// Listen is the address the HTTP server binds, e.g. ":8080".
	Listen string `yaml:"listen"`

	Session  SessionConfig            `yaml:"session"`
	Adapters map[string]AdapterConfig `yaml:"adapters"`
	Tenants  []tenant.Tenant          `yaml:"tenants"`

	// LogLevel is one of debug, info, warn, error.
	LogLevel string `yaml:"log_level"`
}

// SessionConfig selects and tunes the session store.
type SessionConfig struct {
	// Backend is "memory" (default) or "redis". Memory is correct only for
	// a single replica, because a dialogue pinned to one process cannot be
	// continued by another.
	Backend string `yaml:"backend"`
	// RedisURL is required when Backend is "redis".
	RedisURL string `yaml:"redis_url"`
	// TTL is the idle session lifetime. Zero means session.DefaultTTL.
	TTL time.Duration `yaml:"ttl"`
}

// AdapterConfig enables and guards one telco adapter.
type AdapterConfig struct {
	Enabled bool `yaml:"enabled"`
	// AllowedSources are the CIDRs permitted to deliver callbacks. Required
	// for adapters whose provider offers no authenticity signal of its own;
	// see adapter.TrustedProxy.
	AllowedSources []string `yaml:"allowed_sources"`
	// TrustedForwarders are the CIDRs of your own proxies, the only hops
	// permitted to set X-Forwarded-For.
	TrustedForwarders []string `yaml:"trusted_forwarders"`
}

// envRef matches ${NAME} references in the YAML source.
var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads, expands, parses, and validates a configuration file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	var missing []string
	expandNode(&doc, &missing)
	if len(missing) > 0 {
		return nil, fmt.Errorf("config: %s: unset environment variables: %s",
			path, strings.Join(missing, ", "))
	}

	var cfg Config
	if err := doc.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: decoding %s: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	return &cfg, nil
}

// expandNode substitutes ${NAME} in every scalar value of the document,
// collecting the names of unset variables so one run reports all of them
// rather than one per restart.
func expandNode(n *yaml.Node, missing *[]string) {
	if n.Kind == yaml.ScalarNode {
		n.Value = envRef.ReplaceAllStringFunc(n.Value, func(match string) string {
			name := match[2 : len(match)-1]
			val, ok := os.LookupEnv(name)
			if !ok {
				*missing = append(*missing, name)
				return ""
			}
			return val
		})
		return
	}
	for _, child := range n.Content {
		expandNode(child, missing)
	}
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.Session.Backend == "" {
		c.Session.Backend = "memory"
	}
	if c.Session.TTL == 0 {
		c.Session.TTL = session.DefaultTTL
	}
}

// Validate rejects configurations that would start but not work.
func (c *Config) Validate() error {
	switch c.Session.Backend {
	case "memory":
	case "redis":
		if c.Session.RedisURL == "" {
			return fmt.Errorf("session backend is redis but no redis_url is set")
		}
	default:
		return fmt.Errorf("unknown session backend %q, want memory or redis", c.Session.Backend)
	}

	if len(c.Tenants) == 0 {
		return fmt.Errorf("no tenants configured: the gateway would answer nothing")
	}

	var enabled int
	for name, a := range c.Adapters {
		if !a.Enabled {
			continue
		}
		enabled++
		if name == "africastalking" && len(a.AllowedSources) == 0 {
			// The provider signs nothing, so an empty allowlist means
			// anyone who finds the URL can forge any subscriber's input.
			return fmt.Errorf("adapter %q is enabled with no allowed_sources: it would accept forged callbacks from anywhere", name)
		}
	}
	if enabled == 0 {
		return fmt.Errorf("no adapters enabled: the gateway would have no inbound endpoint")
	}
	return nil
}

// SimulatorExposed reports whether the local simulator endpoint is enabled
// on an address that is not loopback. Callers warn rather than refuse: the
// combination is wrong in production but normal inside a container network,
// where the listen address is necessarily not loopback.
func (c *Config) SimulatorExposed() bool {
	a, ok := c.Adapters["simulator"]
	if !ok || !a.Enabled {
		return false
	}
	return !strings.HasPrefix(c.Listen, "127.0.0.1:") && !strings.HasPrefix(c.Listen, "localhost:")
}

// Package config loads runtime settings from environment variables and
// applies safe defaults for local development.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds every knob the server needs at boot time.
type Config struct {
	// Addr is the TCP listen address, e.g. ":8080".
	Addr string
	// DBPath is the SQLite database file location.
	DBPath string
	// WebDir points at the directory holding the static frontend.
	WebDir string
	// Env is one of "development", "test", "production".
	Env string
	// SeedDemo inserts the clearly-labelled demo dataset on first boot.
	SeedDemo bool
}

// Load reads the process environment. Every value has a working default so
// `go run ./cmd/server` works in a fresh checkout with no setup.
func Load() Config {
	cfg := Config{
		Addr:   getenv("GENEPATH_ADDR", ":8080"),
		DBPath: getenv("GENEPATH_DB", "data/genepath.db"),
		WebDir: getenv("GENEPATH_WEB", "web"),
		Env:    getenv("GENEPATH_ENV", "development"),
		// Demo rows are useful for UI work, but they must never show up by
		// accident: a fresh install starts with an empty workspace and the
		// designed empty states instead of fabricated statistics.
		SeedDemo: getenvBool("GENEPATH_SEED_DEMO", false),
	}
	if !strings.Contains(cfg.Addr, ":") {
		cfg.Addr = ":" + cfg.Addr
	}
	return cfg
}

// IsProduction reports whether the server runs in production mode.
func (c Config) IsProduction() bool { return c.Env == "production" }

// DemoEnabled reports whether demo seeding is on.
func (c Config) DemoEnabled() bool { return c.SeedDemo && !c.IsProduction() }

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

package main

// config.go — Config struct and loadConfig.
// The config file holds only non-secret values. Secrets (OP_SERVICE_ACCOUNT_TOKEN)
// come from the environment; the private key is read in-process via 1Password.

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds non-secret configuration loaded from ~/.config/sigil/config.toml.
type Config struct {
	OPKeyRef      string           `toml:"op_key_ref"`
	AppID         int64            `toml:"app_id"`
	Installations map[string]int64 `toml:"installations"`
}

// loadConfig reads and parses the TOML file at path.
// Returns a non-nil *Config on success. Config.Installations is guaranteed
// non-nil even when the [installations] section is absent in the file.
func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if _, err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	// Ensure Installations is never nil so callers can map-look without a nil-guard.
	if cfg.Installations == nil {
		cfg.Installations = make(map[string]int64)
	}

	return &cfg, nil
}

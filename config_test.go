package main

// Config: valid TOML parses to the struct; missing/malformed → clear error + non-zero exit.
//
// These are white-box unit tests. They call loadConfig and inspect the Config struct.

import (
	"strings"
	"testing"
)

// TestLoadConfig_ValidTOML verifies that a well-formed config file with all required
// fields parses to a fully-populated Config struct.
func TestLoadConfig_ValidTOML(t *testing.T) {
	// Given: a valid config file at testdata/config_valid.toml
	// When: loadConfig is called with its path
	cfg, err := loadConfig("testdata/config_valid.toml")

	// Then: no error and all fields are populated correctly
	if err != nil {
		t.Fatalf("expected no error parsing valid TOML, got: %v", err)
	}
	if cfg.OPKeyRef != "op://vault/item/private-key.pem" {
		t.Errorf("OPKeyRef: got %q, want %q", cfg.OPKeyRef, "op://vault/item/private-key.pem")
	}
	if cfg.AppID != 42 {
		t.Errorf("AppID: got %d, want 42", cfg.AppID)
	}
	if len(cfg.Installations) != 2 {
		t.Errorf("Installations: got %d entries, want 2", len(cfg.Installations))
	}
	if got := cfg.Installations["owner-a"]; got != 100 {
		t.Errorf("Installations[owner-a]: got %d, want 100", got)
	}
	if got := cfg.Installations["owner-b"]; got != 200 {
		t.Errorf("Installations[owner-b]: got %d, want 200", got)
	}
}

// TestLoadConfig_MissingFile verifies that a non-existent config path returns a
// clear, non-nil error rather than silently returning a zero-value struct.
func TestLoadConfig_MissingFile(t *testing.T) {
	// Given: a path that does not exist
	// When: loadConfig is called
	_, err := loadConfig("testdata/does_not_exist.toml")

	// Then: error is non-nil
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}

// TestLoadConfig_MalformedTOML verifies that syntactically invalid TOML returns a
// parse error rather than silently returning a zero-value or partial struct.
func TestLoadConfig_MalformedTOML(t *testing.T) {
	// Given: testdata/config_malformed.toml contains invalid TOML
	// When: loadConfig is called
	_, err := loadConfig("testdata/config_malformed.toml")

	// Then: a parse error is returned
	if err == nil {
		t.Fatal("expected parse error for malformed TOML, got nil")
	}
	// And: the error message is meaningful (not an empty string)
	if strings.TrimSpace(err.Error()) == "" {
		t.Error("expected a non-empty error message for malformed TOML")
	}
}

// TestLoadConfig_NoInstallationsSection verifies that a config without an
// [installations] table parses successfully and returns a non-nil (empty) map,
// preventing nil-pointer panics on map lookups.
func TestLoadConfig_NoInstallationsSection(t *testing.T) {
	// Given: a valid config with no [installations] entries
	// When: loadConfig is called
	cfg, err := loadConfig("testdata/config_no_installations.toml")

	// Then: no error; Installations is a non-nil empty map
	if err != nil {
		t.Fatalf("expected no error for config without installations, got: %v", err)
	}
	if cfg.Installations == nil {
		t.Error("Installations should be a non-nil empty map to prevent nil-map panics")
	}
	if len(cfg.Installations) != 0 {
		t.Errorf("expected empty installations map, got %d entries", len(cfg.Installations))
	}
}

// TestLoadConfig_ConfigHoldsNoSecrets checks, on a loaded fixture, that the
// config carries an op:// key reference and not key material. It inspects the
// loaded values only; it does not inspect the struct's field set, so a new
// secret-holding field would not fail it.
func TestLoadConfig_ConfigHoldsNoSecrets(t *testing.T) {
	// Given: a loaded config
	cfg, err := loadConfig("testdata/config_valid.toml")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	// Then: the op_key_ref is a pointer string (starts with "op://"), not key material
	if cfg.OPKeyRef == "" {
		t.Error("OPKeyRef should not be empty")
	}
	if !strings.HasPrefix(cfg.OPKeyRef, "op://") {
		t.Errorf("OPKeyRef must be an op:// reference, not key material; got %q", cfg.OPKeyRef)
	}
}

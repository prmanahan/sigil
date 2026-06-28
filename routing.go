package main

// routing.go — resolveInstallation maps a repo path to an installation ID.
// Fails closed: any unknown or empty path returns an error; no default is guessed.

import (
	"errors"
	"fmt"
	"strings"
)

// resolveInstallation extracts the owner from repoPath (the first "/" segment, or
// the full string if there is no slash) and returns the corresponding installation
// ID from cfg.Installations. Returns an error for an empty path or unknown owner.
func resolveInstallation(cfg *Config, repoPath string) (int64, error) {
	if repoPath == "" {
		return 0, errors.New("resolveInstallation: repoPath is empty")
	}

	owner := repoPath
	if idx := strings.IndexByte(repoPath, '/'); idx >= 0 {
		owner = repoPath[:idx]
	}

	id, ok := cfg.Installations[owner]
	if !ok {
		return 0, fmt.Errorf("resolveInstallation: unknown owner %q (not in installations map)", owner)
	}

	return id, nil
}

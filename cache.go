package main

// cache.go — per-installation token cache under ~/.cache/sigil/.
//
// Security invariants enforced here:
//   - Cache files are created with mode 0600 (owner-only read/write).
//   - CachedToken holds only the short-lived installation token and its expiry.
//     Private key material and the SA token are never stored here.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CachedToken is the on-disk representation of a cached installation token.
// Never contains key material or the service-account token.
type CachedToken struct {
	Token  string    `json:"token"`
	Expiry time.Time `json:"expiry"`
}

// cacheFilePath returns the path for the JSON cache file for installID.
func cacheFilePath(cacheDir string, installID int64) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%d.json", installID))
}

// cacheGet returns the CachedToken for installID, or nil if the file does not
// exist, the token is expired, or the entry holds no token. A missing file is
// not an error.
func cacheGet(cacheDir string, installID int64) (*CachedToken, error) {
	path := cacheFilePath(cacheDir, installID)

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cacheGet: read %q: %w", path, err)
	}

	var ct CachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		// Corrupt cache file is treated as a miss so the caller re-mints.
		// Do not propagate the decode error — a stale or truncated file is
		// recoverable by fetching a fresh token from the API.
		return nil, nil
	}

	if ct.Expiry.Before(time.Now()) {
		return nil, nil
	}

	// An entry with no token is never a credential: serving it would emit an
	// empty password with exit 0. Treat it as a miss so the caller re-mints.
	if ct.Token == "" {
		return nil, nil
	}

	return &ct, nil
}

// cachePut writes ct to the cache file for installID with permissions 0600.
// Overwrites any existing entry for that installID.
func cachePut(cacheDir string, installID int64, ct CachedToken) error {
	path := cacheFilePath(cacheDir, installID)

	data, err := json.Marshal(ct)
	if err != nil {
		return fmt.Errorf("cachePut: marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("cachePut: write %q: %w", path, err)
	}
	// Re-enforce 0600 on a pre-existing file: POSIX ignores the perm argument
	// to open(O_CREAT|O_TRUNC) when the file already exists, so a file that was
	// created with 0644 would otherwise remain 0644 after WriteFile.
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("cachePut: chmod %q: %w", path, err)
	}

	return nil
}

package main

// hardening_test.go — red-phase hardening tests (Warden M1, L2, L4).
//
// N2 and N3 findings are appended to their respective existing test files.
// L1 is in cli_test.go (package main_test, black-box).
// AC#7 (writeCredentialResponse) and L3 (defaultCacheDir / ensureCacheDir with
// explicit dir arg) live in hardening_compilefail_test.go — they reference
// not-yet-extracted functions and produce compile-fail red on purpose.
//
// Sequencing: this file contains only runtime tests so the package compiles for the
// first evidence run. Compile-fail tests are isolated so we can capture per-finding
// evidence in two separate `go test` runs.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- M1: context deadline aborts unbounded API hang ---

// TestGetInstallationToken_ContextDeadlineAborts verifies that getInstallationToken
// respects a short context deadline and returns an error rather than blocking
// when the GitHub API is unresponsive.
//
// M1 status: GREEN with current implementation (exchange.go already uses
// http.NewRequestWithContext so the context deadline propagates to the HTTP
// transport). This test documents the correct behaviour and guards against
// regression to a bare http.NewRequest.
//
// Remaining M1 gap: cmdGet and cmdToken (main.go:97, 125) both pass
// context.Background() — an unbounded context. Forge must thread a bounded
// context (or add an http.Client with Timeout) through the CLI entry points.
// That gap is not testable here without real credentials; document in Forge's
// brief.
func TestGetInstallationToken_ContextDeadlineAborts(t *testing.T) {
	// Given: an API server whose handler sleeps longer than the test deadline.
	const installID = int64(12345)
	cfg := newTestConfig(installID)
	key := newTestKey(t) // key generated before the timed context so keygen doesn't eat deadline
	cacheDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // longer than the 50 ms deadline
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      "ghs_too_late",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: getInstallationToken is called with a 50 ms deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := getInstallationToken(ctx, cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: an error is returned (not a hang).
	if err == nil {
		t.Fatal("expected timeout error when API hangs beyond deadline, got nil")
	}
	// And: the error chain contains context.DeadlineExceeded.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected errors.Is(err, context.DeadlineExceeded); got: %v", err)
	}
}

// --- L2: cachePut re-enforces 0600 on a pre-existing file ---

// TestCachePut_ReenforcesPermsOnExistingFile verifies that cachePut tightens
// the cache file to 0600 even when the file already exists with looser permissions.
//
// RED: os.WriteFile opens an existing file with O_CREAT|O_TRUNC; the perm
// argument is ignored by the kernel when the file already exists (POSIX: mode
// only applies at creation). The existing 0644 mode is preserved. Forge must
// add an explicit os.Chmod(path, 0600) after WriteFile.
func TestCachePut_ReenforcesPermsOnExistingFile(t *testing.T) {
	// Given: a pre-existing cache file with mode 0644 (too permissive).
	dir := t.TempDir()
	path := filepath.Join(dir, "99.json")
	if err := os.WriteFile(path, []byte(`{"token":"old","expiry":"2000-01-01T00:00:00Z"}`), 0644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}
	// Explicit chmod after WriteFile defeats umask so the setup mode is
	// deterministically 0644, not whatever (umask & 0644) happens to be.
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("setup Chmod: %v", err)
	}

	// When: cachePut writes a new token to the same path.
	if err := cachePut(dir, 99, CachedToken{
		Token:  "new_token",
		Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("cachePut: %v", err)
	}

	// Then: the file mode is 0600 (tightened from 0644).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file perms: got %04o, want 0600 (cachePut must re-enforce perms on pre-existing file)", mode)
	}
}

// --- L4: corrupt cache file → re-mint, not hard-fail ---

// TestCacheGet_CorruptFileIsMiss verifies that a cache file containing invalid JSON
// is treated as a cache miss — (nil, nil) — rather than returning an unmarshal error.
//
// RED: current cacheGet returns (nil, fmt.Errorf("cacheGet: unmarshal ...")) on
// JSON decode failure. Forge must change that path to return (nil, nil) so the
// caller re-mints instead of failing closed on corrupt data.
func TestCacheGet_CorruptFileIsMiss(t *testing.T) {
	// Given: a corrupt (non-JSON) cache file for installID 12345.
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "12345.json")
	if err := os.WriteFile(cachePath, []byte("not valid json !@#$%"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// When: cacheGet is called.
	entry, err := cacheGet(cacheDir, 12345)

	// Then: no error (corrupt = miss, not hard-fail).
	if err != nil {
		t.Errorf("expected nil error for corrupt cache file, got: %v", err)
	}
	// And: nil entry (cache miss).
	if entry != nil {
		t.Errorf("expected nil entry for corrupt cache file, got: %+v", entry)
	}
}

// TestGetInstallationToken_CorruptCacheRemints verifies the end-to-end effect:
// a corrupt cache file causes getInstallationToken to re-mint via the API and
// return a fresh token, not an error.
//
// RED: current getInstallationToken propagates the cacheGet unmarshal error as
// "getInstallationToken: cache read: ...". After Forge changes cacheGet to return
// (nil, nil) on corrupt data, this test becomes green.
func TestGetInstallationToken_CorruptCacheRemints(t *testing.T) {
	// Given: a corrupt cache file and a working mock API.
	const installID = int64(12345)
	const freshToken = "ghs_fresh_after_corrupt_cache"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	// cacheFilePath generates "<installID>.json" — match that path here.
	cachePath := filepath.Join(cacheDir, "12345.json")
	if err := os.WriteFile(cachePath, []byte("{corrupt json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	apiCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCallCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      freshToken,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: getInstallationToken is called with the corrupt cache in place.
	token, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: no error (corrupt cache is a miss, not a failure).
	if err != nil {
		t.Fatalf("expected nil error when cache is corrupt, got: %v", err)
	}
	// And: the fresh token from the mock API is returned.
	if token != freshToken {
		t.Errorf("token: got %q, want %q", token, freshToken)
	}
	// And: the mock API was called exactly once (re-mint happened).
	if apiCallCount != 1 {
		t.Errorf("API call count: got %d, want 1 (corrupt cache should trigger re-mint)", apiCallCount)
	}
}

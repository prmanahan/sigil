package main

// AC#6 (cache layer): a second call within TTL reuses the cache (no second mint);
// expired → re-mint; cache file perms are 0600.
//
// These tests cover the low-level cache primitives (cacheGet/cachePut).
// The higher-level cache reuse and re-mint paths are tested in token_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCachePut_WritesReadableEntry verifies that cachePut stores an entry that
// cacheGet can subsequently retrieve.
func TestCachePut_WritesReadableEntry(t *testing.T) {
	// Given: a fresh temp cache directory
	dir := t.TempDir()
	want := CachedToken{
		Token:  "ghs_abc123",
		Expiry: time.Now().Add(55 * time.Minute).UTC().Truncate(time.Second),
	}

	// When: cachePut stores an entry for installation 12345
	err := cachePut(dir, 12345, want)

	// Then: no error
	if err != nil {
		t.Fatalf("cachePut failed: %v", err)
	}

	// And: cacheGet returns the stored entry
	got, err := cacheGet(dir, 12345)
	if err != nil {
		t.Fatalf("cacheGet after put failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected cached entry, got nil")
	}
	if got.Token != want.Token {
		t.Errorf("token: got %q, want %q", got.Token, want.Token)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Errorf("expiry: got %v, want %v", got.Expiry, want.Expiry)
	}
}

// TestCacheGet_MissOnEmpty verifies that cacheGet returns nil (no error) when
// there is no cached entry for an installation ID.
func TestCacheGet_MissOnEmpty(t *testing.T) {
	// Given: a fresh cache directory with no entries
	dir := t.TempDir()

	// When: cacheGet is called for a never-cached installation ID
	entry, err := cacheGet(dir, 12345)

	// Then: nil entry, no error (cache miss is not an error)
	if err != nil {
		t.Fatalf("unexpected error on cache miss: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil on cache miss, got: %+v", entry)
	}
}

// TestCachePut_FilePermsAre0600 enforces the security invariant that cache files
// are created with mode 0600 (owner read/write only, no group/world access).
func TestCachePut_FilePermsAre0600(t *testing.T) {
	// Given: a fresh cache directory
	dir := t.TempDir()

	// When: cachePut writes a token for installation 99
	err := cachePut(dir, 99, CachedToken{
		Token:  "secret_tok",
		Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("cachePut failed: %v", err)
	}

	// Then: the cache file(s) in dir have mode 0600
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one cache file to be created")
	}
	for _, e := range entries {
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("Stat %q: %v", e.Name(), err)
		}
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Errorf("cache file %q: perms %04o, want 0600", e.Name(), mode)
		}
	}
}

// TestCacheGet_ExpiredEntryMisses verifies that a cached token whose Expiry is in
// the past is treated as a cache miss (returned as nil).
func TestCacheGet_ExpiredEntryMisses(t *testing.T) {
	// Given: a cache entry written with an already-expired expiry
	dir := t.TempDir()
	expired := CachedToken{
		Token:  "old_token",
		Expiry: time.Now().Add(-1 * time.Minute), // expired 1 minute ago
	}
	if err := cachePut(dir, 12345, expired); err != nil {
		t.Fatalf("cachePut: %v", err)
	}

	// When: cacheGet is called
	entry, err := cacheGet(dir, 12345)

	// Then: nil is returned (expired = miss), no error
	if err != nil {
		t.Fatalf("unexpected error for expired entry: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil for expired cache entry, got: %+v", entry)
	}
}

// TestCacheGet_ValidEntryWithinTTLHits verifies that a token with a future expiry
// is returned as a cache hit.
func TestCacheGet_ValidEntryWithinTTLHits(t *testing.T) {
	// Given: a token cached with 55 minutes remaining
	dir := t.TempDir()
	valid := CachedToken{
		Token:  "fresh_token_xyz",
		Expiry: time.Now().Add(55 * time.Minute),
	}
	if err := cachePut(dir, 12345, valid); err != nil {
		t.Fatalf("cachePut: %v", err)
	}

	// When: cacheGet is called
	entry, err := cacheGet(dir, 12345)

	// Then: the token is returned (cache hit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected cache hit (token within TTL), got nil")
	}
	if entry.Token != valid.Token {
		t.Errorf("token: got %q, want %q", entry.Token, valid.Token)
	}
}

// TestCacheGet_InstallationIDsAreSeparate verifies that tokens for different
// installation IDs are stored and retrieved independently (no cross-contamination).
func TestCacheGet_InstallationIDsAreSeparate(t *testing.T) {
	// Given: two different installation IDs cached in the same dir
	dir := t.TempDir()
	tok1 := CachedToken{Token: "token_for_1", Expiry: time.Now().Add(time.Hour)}
	tok2 := CachedToken{Token: "token_for_2", Expiry: time.Now().Add(time.Hour)}

	if err := cachePut(dir, 1, tok1); err != nil {
		t.Fatalf("cachePut(1): %v", err)
	}
	if err := cachePut(dir, 2, tok2); err != nil {
		t.Fatalf("cachePut(2): %v", err)
	}

	// When: each ID is retrieved
	got1, err := cacheGet(dir, 1)
	if err != nil || got1 == nil {
		t.Fatalf("cacheGet(1): err=%v, got=%v", err, got1)
	}
	got2, err := cacheGet(dir, 2)
	if err != nil || got2 == nil {
		t.Fatalf("cacheGet(2): err=%v, got=%v", err, got2)
	}

	// Then: each retrieval returns the correct token
	if got1.Token != tok1.Token {
		t.Errorf("install 1: got %q, want %q", got1.Token, tok1.Token)
	}
	if got2.Token != tok2.Token {
		t.Errorf("install 2: got %q, want %q", got2.Token, tok2.Token)
	}
}

// TestCachePut_OverwritesExistingEntry verifies that writing a new token for the
// same installation ID replaces the old one rather than appending.
func TestCachePut_OverwritesExistingEntry(t *testing.T) {
	// Given: an existing cached token
	dir := t.TempDir()
	old := CachedToken{Token: "old_token", Expiry: time.Now().Add(time.Hour)}
	if err := cachePut(dir, 42, old); err != nil {
		t.Fatalf("first cachePut: %v", err)
	}

	// When: a new token is written for the same installation ID
	updated := CachedToken{Token: "new_token", Expiry: time.Now().Add(55 * time.Minute)}
	if err := cachePut(dir, 42, updated); err != nil {
		t.Fatalf("second cachePut: %v", err)
	}

	// Then: cacheGet returns the new token, not the old one
	got, err := cacheGet(dir, 42)
	if err != nil || got == nil {
		t.Fatalf("cacheGet after overwrite: err=%v, got=%v", err, got)
	}
	if got.Token != updated.Token {
		t.Errorf("token: got %q, want %q (old was %q)", got.Token, updated.Token, old.Token)
	}
}

// TestCachePut_NeverWritesKeyMaterial verifies that the cache file does not
// contain PEM key markers, enforcing the "key never written to disk" invariant.
func TestCachePut_NeverWritesKeyMaterial(t *testing.T) {
	// Given: a cache dir and a token to store
	dir := t.TempDir()
	ct := CachedToken{
		Token:  "ghs_sometoken",
		Expiry: time.Now().Add(55 * time.Minute),
	}

	// When: the token is cached
	if err := cachePut(dir, 1, ct); err != nil {
		t.Fatalf("cachePut: %v", err)
	}

	// Then: no cache file contains PEM markers or private-key indicators
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile %q: %v", e.Name(), err)
		}
		content := string(data)
		pemMarkers := []string{
			"BEGIN RSA PRIVATE KEY",
			"END RSA PRIVATE KEY",
			"BEGIN PRIVATE KEY",
			"END PRIVATE KEY",
		}
		for _, marker := range pemMarkers {
			if strings.Contains(content, marker) {
				t.Errorf("cache file %q contains PEM marker %q — key material on disk", e.Name(), marker)
			}
		}
	}
}

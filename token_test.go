package main

// AC#4 (re-mint path) + AC#6 (cache reuse) at the getInstallationToken level.
//
// getInstallationToken is the high-level orchestrator:
//   1. Check cache (cacheGet) — return cached token if valid.
//   2. Mint App JWT (mintAppJWT).
//   3. Exchange for installation token (exchangeInstallationToken).
//   4. On 401 error: re-mint a fresh JWT and retry the exchange once.
//   5. Write the new token to cache (cachePut).
//
// Signature (Forge must implement this exactly):
//
//	func getInstallationToken(
//	    ctx context.Context,
//	    cfg *Config,
//	    key *rsa.PrivateKey,
//	    repoPath string,
//	    apiBaseURL string,
//	    cacheDir string,
//	) (string, error)

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestConfig returns a minimal Config for use in integration-style unit tests.
func newTestConfig(installID int64) *Config {
	return &Config{
		AppID: 1,
		Installations: map[string]int64{
			"test-owner": installID,
		},
	}
}

// newTestKey generates an RSA key for tests. Fatals on failure.
func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return k
}

// okInstallServer returns an httptest.Server that always responds 201 with the
// given token and a 1-hour expiry. The caller must close the server.
func okInstallServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      token,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
}

// TestGetInstallationToken_FreshMintAndCache verifies the baseline path:
// no cache entry → mint JWT → exchange → cache the result → return the token.
func TestGetInstallationToken_FreshMintAndCache(t *testing.T) {
	// Given: a mock GitHub API server, a key, and a config with one installation
	const installID = int64(12345)
	const wantToken = "ghs_fresh_token"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	srv := okInstallServer(t, wantToken)
	defer srv.Close()

	// When: getInstallationToken is called with an empty cache
	token, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: the token from the mock server is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token: got %q, want %q", token, wantToken)
	}
}

// TestGetInstallationToken_CacheHitSkipsAPI verifies AC#6:
// if the cache has a valid, non-expired token for the installation, the API is
// NOT called again (no second mint — assert via mock server request count).
func TestGetInstallationToken_CacheHitSkipsAPI(t *testing.T) {
	// Given: a cache already containing a valid token
	const installID = int64(12345)
	const cachedToken = "ghs_cached_token"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	// Pre-seed the cache with a valid (non-expired) token
	if err := cachePut(cacheDir, installID, CachedToken{
		Token:  cachedToken,
		Expiry: time.Now().Add(55 * time.Minute),
	}); err != nil {
		t.Fatalf("cachePut seed: %v", err)
	}

	// Track how many times the mock API is called
	apiCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCallCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      "ghs_api_token",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: getInstallationToken is called
	token, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: the cached token is returned without any API call
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != cachedToken {
		t.Errorf("token: got %q, want cached %q", token, cachedToken)
	}
	if apiCallCount != 0 {
		t.Errorf("API call count: got %d, want 0 (cache hit should skip the API)", apiCallCount)
	}
}

// TestGetInstallationToken_ExpiredCacheRemints verifies AC#6 (expired → re-mint):
// if the cached token is expired, the API is called to mint a fresh one.
func TestGetInstallationToken_ExpiredCacheRemints(t *testing.T) {
	// Given: a cache with an expired token
	const installID = int64(12345)
	const freshToken = "ghs_fresh_after_expiry"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	// Seed the cache with an already-expired token
	if err := cachePut(cacheDir, installID, CachedToken{
		Token:  "ghs_old_expired_token",
		Expiry: time.Now().Add(-5 * time.Minute), // expired
	}); err != nil {
		t.Fatalf("cachePut seed: %v", err)
	}

	srv := okInstallServer(t, freshToken)
	defer srv.Close()

	// When: getInstallationToken is called
	token, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: the fresh token is returned (expired cache is not used)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != freshToken {
		t.Errorf("token: got %q, want fresh %q", token, freshToken)
	}
}

// TestGetInstallationToken_401TriggersRemint verifies AC#4:
// when GitHub returns 401 (JWT rejected), getInstallationToken re-mints a fresh
// JWT and retries the exchange exactly once.
func TestGetInstallationToken_401TriggersRemint(t *testing.T) {
	// Given: a mock server that returns 401 on the first call, then 201 on the second
	const installID = int64(12345)
	const retryToken = "ghs_token_after_remint"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: 401 (JWT rejected by GitHub — triggers re-mint)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
			return
		}
		// Subsequent calls: 201 with a fresh token
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      retryToken,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: getInstallationToken is called
	token, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: the token from the retry is returned (re-mint path succeeded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != retryToken {
		t.Errorf("token: got %q, want %q (from re-mint retry)", token, retryToken)
	}
	// And: exactly two API calls were made (original + one retry after re-mint)
	if callCount != 2 {
		t.Errorf("API call count: got %d, want 2 (initial 401 + one retry)", callCount)
	}
}

// TestGetInstallationToken_Persistent401FailsClosed verifies that if both the
// original exchange and the re-mint retry return 401, an error is returned
// (fail-closed, never returns a partial or empty credential).
func TestGetInstallationToken_Persistent401FailsClosed(t *testing.T) {
	// Given: a mock server that always returns 401
	const installID = int64(12345)

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
	}))
	defer srv.Close()

	// When: getInstallationToken is called
	_, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)

	// Then: an error is returned — fail closed, no token emitted
	if err == nil {
		t.Fatal("expected error when GitHub returns persistent 401 (fail-closed), got nil")
	}
}

// TestGetInstallationToken_UnknownOwnerFailsClosed verifies that an unknown repo
// owner causes a failure before any API call is made.
func TestGetInstallationToken_UnknownOwnerFailsClosed(t *testing.T) {
	// Given: a config that does not include the requested owner
	cfg := &Config{
		AppID:         1,
		Installations: map[string]int64{"known-owner": 12345},
	}
	key := newTestKey(t)
	cacheDir := t.TempDir()

	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	// When: getInstallationToken is called for an unknown owner
	_, err := getInstallationToken(context.Background(), cfg, key, "unknown-owner/repo", srv.URL, cacheDir)

	// Then: error is returned and the API was not called
	if err == nil {
		t.Fatal("expected error for unknown owner (fail-closed), got nil")
	}
	if apiCalled {
		t.Error("GitHub API should not be called for an unknown owner")
	}
}

// TestGetInstallationToken_ResultIsCached verifies that after a successful mint,
// the token is written to the cache so subsequent calls can hit it.
func TestGetInstallationToken_ResultIsCached(t *testing.T) {
	// Given: an empty cache and a mock API that counts calls
	const installID = int64(12345)
	const wantToken = "ghs_to_be_cached"

	cfg := newTestConfig(installID)
	key := newTestKey(t)
	cacheDir := t.TempDir()

	apiCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCallCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      wantToken,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: getInstallationToken is called twice
	tok1, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	tok2, err := getInstallationToken(context.Background(), cfg, key, "test-owner/repo", srv.URL, cacheDir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	// Then: both calls return the same token
	if tok1 != wantToken || tok2 != wantToken {
		t.Errorf("tokens: first=%q second=%q want=%q", tok1, tok2, wantToken)
	}
	// And: the API was called exactly once (second call hit the cache)
	if apiCallCount != 1 {
		t.Errorf("API call count: got %d, want 1 (second call should hit cache)", apiCallCount)
	}
}

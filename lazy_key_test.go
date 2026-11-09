package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Order under test: routing, then cache lookup, then (only on a miss) the key
// load. A loadKey stub that counts calls is the oracle; no 1Password is involved.

type keyLoader struct {
	calls int
	key   *rsa.PrivateKey
	err   error
}

func (k *keyLoader) load(context.Context) (*rsa.PrivateKey, error) {
	k.calls++
	return k.key, k.err
}

func TestGetInstallationTokenLazy_CacheHit_DoesNotLoadKey(t *testing.T) {
	const installID = int64(12345)
	cacheDir := t.TempDir()
	if err := cachePut(cacheDir, installID, CachedToken{Token: "ghs_cached", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("cachePut: %v", err)
	}
	kl := &keyLoader{err: errors.New("key must not be read on a cache hit")}

	token, err := getInstallationTokenLazy(context.Background(), newTestConfig(installID), kl.load, "test-owner/repo", "http://127.0.0.1:1", cacheDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghs_cached" {
		t.Errorf("token: got %q, want the cached token", token)
	}
	if kl.calls != 0 {
		t.Errorf("loadKey calls on a cache hit: got %d, want 0", kl.calls)
	}
}

func TestGetInstallationTokenLazy_UnknownOwner_DoesNotLoadKey(t *testing.T) {
	kl := &keyLoader{err: errors.New("key must not be read for an unknown owner")}

	_, err := getInstallationTokenLazy(context.Background(), newTestConfig(12345), kl.load, "other-owner/repo", "http://127.0.0.1:1", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unknown owner") {
		t.Fatalf("expected the routing error, got: %v", err)
	}
	if kl.calls != 0 {
		t.Errorf("loadKey calls for an unknown owner: got %d, want 0", kl.calls)
	}
}

func TestGetInstallationTokenLazy_ExpiredEntry_LoadsKey(t *testing.T) {
	const installID = int64(12345)
	cacheDir := t.TempDir()
	if err := cachePut(cacheDir, installID, CachedToken{Token: "ghs_stale", Expiry: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("cachePut: %v", err)
	}
	kl := &keyLoader{err: errors.New("key read reached")}

	token, err := getInstallationTokenLazy(context.Background(), newTestConfig(installID), kl.load, "test-owner/repo", "http://127.0.0.1:1", cacheDir)
	if err == nil || !strings.Contains(err.Error(), "key read reached") {
		t.Fatalf("expected the key-load error, got: %v", err)
	}
	if token != "" {
		t.Errorf("an expired token was served: %q", token)
	}
	if kl.calls != 1 {
		t.Errorf("loadKey calls: got %d, want 1", kl.calls)
	}
}

// TestGetInstallationTokenLazy_401Retry_LoadsKeyOnce proves the retry reuses the
// key loaded for the first mint rather than reading it again or going without.
func TestGetInstallationTokenLazy_401Retry_LoadsKeyOnce(t *testing.T) {
	const installID = int64(12345)
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		if posts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      "ghs_after_retry",
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()
	kl := &keyLoader{key: newTestKey(t)}

	token, err := getInstallationTokenLazy(context.Background(), newTestConfig(installID), kl.load, "test-owner/repo", srv.URL, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghs_after_retry" || posts != 2 {
		t.Errorf("token %q after %d POSTs, want ghs_after_retry after 2", token, posts)
	}
	if kl.calls != 1 {
		t.Errorf("loadKey calls: got %d, want exactly 1 across the retry", kl.calls)
	}
}

// TestGetInstallationToken_CachesWithFiveMinuteMargin pins the write side of the
// margin: the stored expiry is GitHub's expires_at minus five minutes, so
// cacheGet (which compares against now) never serves a token within five
// minutes of death.
func TestGetInstallationToken_CachesWithFiveMinuteMargin(t *testing.T) {
	const installID = int64(12345)
	ghExpiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": "ghs_t", "expires_at": ghExpiry.Format(time.RFC3339)})
	}))
	defer srv.Close()
	cacheDir := t.TempDir()

	if _, err := getInstallationToken(context.Background(), newTestConfig(installID), newTestKey(t), "test-owner/repo", srv.URL, cacheDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := cacheGet(cacheDir, installID)
	if err != nil || got == nil {
		t.Fatalf("cacheGet: %v, %v", got, err)
	}
	if want := ghExpiry.Add(-5 * time.Minute); !got.Expiry.Equal(want) {
		t.Errorf("cached expiry: got %v, want %v (expires_at - 5m)", got.Expiry, want)
	}
}

type ctxProbeKey struct{}

// TestGetInstallationTokenLazy_LoadKeyReceivesCallerContext: the context handed
// to loadKey is the caller's, so the one deadline the CLI sets also bounds the
// key read (readKey passes it on to the 1Password client).
func TestGetInstallationTokenLazy_LoadKeyReceivesCallerContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), ctxProbeKey{}, "probe"), time.Minute)
	defer cancel()
	var got context.Context
	loader := func(c context.Context) (*rsa.PrivateKey, error) {
		got = c
		return nil, errors.New("stop here")
	}

	_, _ = getInstallationTokenLazy(ctx, newTestConfig(12345), loader, "test-owner/repo", "http://127.0.0.1:1", t.TempDir())

	if got == nil {
		t.Fatal("loadKey was not called")
	}
	if got.Value(ctxProbeKey{}) != "probe" {
		t.Error("loadKey did not receive the caller's context")
	}
	if _, ok := got.Deadline(); !ok {
		t.Error("loadKey's context carries no deadline")
	}
}

// TestGetInstallationTokenLazy_EmptyCachedToken_IsMiss: an entry holding no
// token must not be served as an empty password; it falls through to the key
// load like any other miss.
func TestGetInstallationTokenLazy_EmptyCachedToken_IsMiss(t *testing.T) {
	const installID = int64(12345)
	cacheDir := t.TempDir()
	if err := cachePut(cacheDir, installID, CachedToken{Token: "", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("cachePut: %v", err)
	}
	kl := &keyLoader{err: errors.New("key read reached")}

	token, err := getInstallationTokenLazy(context.Background(), newTestConfig(installID), kl.load, "test-owner/repo", "http://127.0.0.1:1", cacheDir)

	if token != "" || err == nil || !strings.Contains(err.Error(), "key read reached") {
		t.Fatalf("got token %q, err %v; want the key-load error and no token", token, err)
	}
	if kl.calls != 1 {
		t.Errorf("loadKey calls: got %d, want 1", kl.calls)
	}
}

// TestGetInstallationTokenLazy_UnreadableCache_SurfacesErrorWithoutKeyRead: a
// cache entry that cannot be read (here, a directory where the file should be)
// is an error, not a miss, and the key is not loaded.
func TestGetInstallationTokenLazy_UnreadableCache_SurfacesErrorWithoutKeyRead(t *testing.T) {
	const installID = int64(12345)
	cacheDir := t.TempDir()
	if err := os.Mkdir(cacheFilePath(cacheDir, installID), 0700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	kl := &keyLoader{err: errors.New("key must not be read")}

	_, err := getInstallationTokenLazy(context.Background(), newTestConfig(installID), kl.load, "test-owner/repo", "http://127.0.0.1:1", cacheDir)

	if err == nil || !strings.Contains(err.Error(), "cache read") {
		t.Fatalf("expected the cache read error to surface, got: %v", err)
	}
	if kl.calls != 0 {
		t.Errorf("loadKey calls after an unreadable cache: got %d, want 0", kl.calls)
	}
}

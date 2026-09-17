package main

// #3647: unit and httptest-level coverage for the --rate-limit-only path.
// None of these tests need network or 1Password — GitHub is always an
// httptest mock here, matching exchange_test.go / token_test.go's convention.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

// --- parseOwnerRepo: argument-shape unit tests ---

// TestParseOwnerRepo_Table covers the accepted shape and every rejected shape
// #3647 names: a/b/c (too many slashes), /repo and owner/ (empty side), and
// no slash at all.
func TestParseOwnerRepo_Table(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{name: "accepted", in: "mnemra/mnemra-core", wantOwner: "mnemra", wantRepo: "mnemra-core"},
		{name: "no slash", in: "mnemra", wantErr: true},
		{name: "two slashes", in: "a/b/c", wantErr: true},
		{name: "empty owner", in: "/x", wantErr: true},
		{name: "empty repo", in: "x/", wantErr: true},
		{name: "empty string", in: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseOwnerRepo(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseOwnerRepo(%q): expected error, got owner=%q repo=%q", tc.in, owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOwnerRepo(%q): unexpected error: %v", tc.in, err)
			}
			if owner != tc.wantOwner || repo != tc.wantRepo {
				t.Errorf("parseOwnerRepo(%q) = (%q, %q), want (%q, %q)", tc.in, owner, repo, tc.wantOwner, tc.wantRepo)
			}
		})
	}
}

// --- dispatchToken: argument-polarity unit tests ---
// These check the parser's own routing decision (does it call cmdToken or
// cmdTokenNarrowed, or reject before either?) without exercising the network
// or config-load side effects of the cmd* functions themselves — that
// end-to-end property is covered separately at the CLI level in cli_test.go.

// TestDispatchToken_RejectsBeforeDelegating verifies every #3647-named
// rejected shape returns an error without needing a config or network mock —
// i.e. the rejection happens in argument parsing, not downstream.
func TestDispatchToken_RejectsBeforeDelegating(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--foo", "owner/repo"}},
		{name: "typo'd flag", args: []string{"--rate-limt-only", "owner/repo"}},
		{name: "equals-form flag", args: []string{"--rate-limit-only=owner/repo"}},
		{name: "flag after positional", args: []string{"owner/repo", "--rate-limit-only"}},
		{name: "extra positional (plain form)", args: []string{"owner", "extra"}},
		{name: "extra positional (flag form)", args: []string{"--rate-limit-only", "owner/repo", "extra"}},
		{name: "missing repo", args: []string{"--rate-limit-only"}},
		{name: "no args", args: []string{}},
		{name: "two slashes", args: []string{"--rate-limit-only", "a/b/c"}},
		{name: "empty owner", args: []string{"--rate-limit-only", "/x"}},
		{name: "empty repo", args: []string{"--rate-limit-only", "x/"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := dispatchToken(tc.args)
			if err == nil {
				t.Fatalf("dispatchToken(%v): expected error, got nil", tc.args)
			}
		})
	}
}

// --- verifyNarrowedGrant: grant-check unit tests ---
// Response shapes below mirror #3647's live measurement (Peter's run,
// 2026-09-17, mnemra installation 143101545): an access_tokens request body
// of {"permissions":{}} was granted all seven installation permissions
// including write, not none, and a request body of
// {"permissions":{"metadata":"read"},"repositories":[]} was granted
// repository_selection "all", not zero repositories.

func repoRef(name string, private bool) struct {
	Name    string `json:"name"`
	Private bool   `json:"private"`
} {
	return struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}{Name: name, Private: private}
}

func TestVerifyNarrowedGrant_Table(t *testing.T) {
	const repo = "mnemra-core"

	tests := []struct {
		name    string
		resp    *narrowedTokenResponse
		wantErr bool
	}{
		{
			name: "accept: metadata:read, selected, exactly the one public repo",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{repoRef(repo, false)},
			},
			wantErr: false,
		},
		{
			name: "reject: an empty permissions object was granted all seven permissions, incl. write",
			resp: &narrowedTokenResponse{
				Permissions: map[string]string{
					"actions": "write", "checks": "write", "contents": "write",
					"issues": "write", "metadata": "read", "pull_requests": "write", "workflows": "write",
				},
				RepositorySelection: "all",
			},
			wantErr: true,
		},
		{
			name: "reject: metadata:read granted, but an empty repositories array reverted selection to all",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "all",
			},
			wantErr: true,
		},
		{
			name: "reject: metadata:read granted, selection=all, no repositories array at all",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "all",
				Repositories:        nil,
			},
			wantErr: true,
		},
		{
			name: "reject: extra permission beyond metadata:read",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read", "contents": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{repoRef(repo, false)},
			},
			wantErr: true,
		},
		{
			name: "reject: metadata:write instead of metadata:read",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "write"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{repoRef(repo, false)},
			},
			wantErr: true,
		},
		{
			name: "reject: repository name mismatch",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{repoRef("some-other-repo", false)},
			},
			wantErr: true,
		},
		{
			name: "reject: repository is private",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{repoRef(repo, true)},
			},
			wantErr: true,
		},
		{
			name: "reject: two repositories instead of one",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{
					repoRef(repo, false), repoRef("another-repo", false),
				},
			},
			wantErr: true,
		},
		{
			name: "reject: selection=selected but empty repositories array",
			resp: &narrowedTokenResponse{
				Permissions:         map[string]string{"metadata": "read"},
				RepositorySelection: "selected",
				Repositories: []struct {
					Name    string `json:"name"`
					Private bool   `json:"private"`
				}{},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyNarrowedGrant(tc.resp, repo)
			if tc.wantErr && err == nil {
				t.Fatalf("verifyNarrowedGrant: expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("verifyNarrowedGrant: unexpected error: %v", err)
			}
			if tc.wantErr && !errors.Is(err, errGrantMismatch) {
				t.Errorf("verifyNarrowedGrant: error %v does not wrap errGrantMismatch", err)
			}
		})
	}
}

// --- exchangeNarrowedToken: exact request body ---

// TestExchangeNarrowedToken_RequestBodyExact asserts the request body by
// semantic JSON equality (decode both sides, compare structurally) rather
// than a string/substring match, and that the repo name sent is bare — never
// "owner/repo".
func TestExchangeNarrowedToken_RequestBodyExact(t *testing.T) {
	const repo = "mnemra-core"
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token":                "ghs_narrow_test",
			"permissions":          map[string]string{"metadata": "read"},
			"repository_selection": "selected",
			"repositories":         []map[string]any{{"name": repo, "private": false}},
		})
	}))
	defer srv.Close()

	_, err := exchangeNarrowedToken(context.Background(), srv.URL, "test.jwt", 12345, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantBody := fmt.Sprintf(`{"permissions":{"metadata":"read"},"repositories":[%q]}`, repo)

	var gotDecoded, wantDecoded any
	if err := json.Unmarshal(gotBody, &gotDecoded); err != nil {
		t.Fatalf("decode actual body %s: %v", gotBody, err)
	}
	if err := json.Unmarshal([]byte(wantBody), &wantDecoded); err != nil {
		t.Fatalf("decode expected body: %v", err)
	}
	if !reflect.DeepEqual(gotDecoded, wantDecoded) {
		t.Errorf("request body: got %s, want %s (semantic JSON mismatch)", gotBody, wantBody)
	}
}

// --- getRateLimitOnlyToken: grant mismatch -> revoke, and cache isolation ---

// narrowServerRecorder is populated by newNarrowServer's mock handlers: it
// records whether DELETE /installation/token was called and with what
// Authorization header, so tests can assert the revoked token matches the
// minted one.
type narrowServerRecorder struct {
	deleteCalled bool
	deleteAuth   string
}

// newNarrowServer builds an httptest server that answers a POST
// access_tokens with mintBody at mintStatus, and a DELETE
// /installation/token with revokeStatus. The caller must close the server.
func newNarrowServer(t *testing.T, mintStatus int, mintBody any, revokeStatus int) (*httptest.Server, *narrowServerRecorder) {
	t.Helper()
	rec := &narrowServerRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(mintStatus)
		json.NewEncoder(w).Encode(mintBody)
	})
	mux.HandleFunc("/installation/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE on /installation/token, got %s", r.Method)
		}
		rec.deleteCalled = true
		rec.deleteAuth = r.Header.Get("Authorization")
		w.WriteHeader(revokeStatus)
	})
	return httptest.NewServer(mux), rec
}

// TestGetRateLimitOnlyToken_MismatchRevokesAndErrors verifies: a grant that
// does not match the request (all seven installation permissions granted,
// including write, instead of the requested metadata:read alone) causes the
// token to be revoked, an error returned, and no token handed back to the
// caller.
func TestGetRateLimitOnlyToken_MismatchRevokesAndErrors(t *testing.T) {
	const installID = int64(12345)
	const mintedToken = "ghs_widened_token_should_be_revoked"

	cfg := newTestConfig(installID)
	key := newTestKey(t)

	srv, rec := newNarrowServer(t, http.StatusCreated, map[string]any{
		"token": mintedToken,
		"permissions": map[string]string{
			"actions": "write", "checks": "write", "contents": "write",
			"issues": "write", "metadata": "read", "pull_requests": "write", "workflows": "write",
		},
		"repository_selection": "all",
	}, http.StatusNoContent)
	defer srv.Close()

	token, err := getRateLimitOnlyToken(context.Background(), cfg, key, "test-owner", "some-repo", srv.URL)

	if err == nil {
		t.Fatal("expected error on grant mismatch, got nil")
	}
	if !errors.Is(err, errGrantMismatch) {
		t.Errorf("error %v does not wrap errGrantMismatch", err)
	}
	if token != "" {
		t.Errorf("expected empty token on grant mismatch, got %q", token)
	}
	if !rec.deleteCalled {
		t.Fatal("expected DELETE /installation/token to be called on grant mismatch")
	}
	if rec.deleteAuth != "Bearer "+mintedToken {
		t.Errorf("revoke Authorization header: got %q, want %q", rec.deleteAuth, "Bearer "+mintedToken)
	}
}

// TestGetRateLimitOnlyToken_RevokeFailureStillErrors verifies that if the
// revoke call itself fails (non-204), getRateLimitOnlyToken still returns an
// error rather than silently succeeding or returning the mismatched token.
func TestGetRateLimitOnlyToken_RevokeFailureStillErrors(t *testing.T) {
	const installID = int64(12345)

	cfg := newTestConfig(installID)
	key := newTestKey(t)

	srv, rec := newNarrowServer(t, http.StatusCreated, map[string]any{
		"token":                "ghs_should_still_be_revoked_attempt",
		"permissions":          map[string]string{"metadata": "read"},
		"repository_selection": "all", // mismatch: not "selected"
	}, http.StatusInternalServerError) // revoke itself fails
	defer srv.Close()

	token, err := getRateLimitOnlyToken(context.Background(), cfg, key, "test-owner", "some-repo", srv.URL)

	if err == nil {
		t.Fatal("expected error when grant mismatch AND revoke both fail, got nil")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
	if !rec.deleteCalled {
		t.Fatal("expected a revoke attempt even though it was going to fail")
	}
}

// TestGetRateLimitOnlyToken_401RetryStillVerifiesGrant verifies #3647 Warden
// finding #1 (MEDIUM, reproduced): nothing previously exercised the 401
// re-mint-and-retry path far enough to prove the *retried* response is still
// run through verifyNarrowedGrant rather than trusted outright. Mirrors
// TestGetInstallationToken_401TriggersRemint's shape (token_test.go): 401 on
// the first mint, then 201 on the retry — except the retried grant here is a
// full-access widen, so the correct behaviour is still an error, not a token.
func TestGetRateLimitOnlyToken_401RetryStillVerifiesGrant(t *testing.T) {
	const installID = int64(12345)
	const retriedToken = "ghs_widened_after_401_retry"

	cfg := newTestConfig(installID)
	key := newTestKey(t)

	var mintCalls int
	var deleteCalled bool
	var deleteAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations/", func(w http.ResponseWriter, r *http.Request) {
		mintCalls++
		if mintCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token": retriedToken,
			"permissions": map[string]string{
				"actions": "write", "checks": "write", "contents": "write",
				"issues": "write", "metadata": "read", "pull_requests": "write", "workflows": "write",
			},
			"repository_selection": "all",
		})
	})
	mux.HandleFunc("/installation/token", func(w http.ResponseWriter, r *http.Request) {
		deleteCalled = true
		deleteAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token, err := getRateLimitOnlyToken(context.Background(), cfg, key, "test-owner", "some-repo", srv.URL)

	if err == nil {
		t.Fatal("expected error: the retried mint still returned a widened grant")
	}
	if !errors.Is(err, errGrantMismatch) {
		t.Errorf("error %v does not wrap errGrantMismatch", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
	if mintCalls != 2 {
		t.Fatalf("mint calls: got %d, want 2 (initial 401 + one retry)", mintCalls)
	}
	if !deleteCalled {
		t.Fatal("expected DELETE /installation/token to fire against the retried token")
	}
	if deleteAuth != "Bearer "+retriedToken {
		t.Errorf("revoke Authorization header: got %q, want %q", deleteAuth, "Bearer "+retriedToken)
	}
}

// TestGetRateLimitOnlyToken_EmptyTokenFailsClosed verifies #3647 Warden
// finding #4 (MEDIUM): a 201 with an otherwise-correct grant but an empty
// token must not pass through — cmdTokenNarrowed would otherwise print a
// blank line to stdout with exit 0.
func TestGetRateLimitOnlyToken_EmptyTokenFailsClosed(t *testing.T) {
	const installID = int64(12345)
	const repo = "mnemra-core"

	cfg := newTestConfig(installID)
	key := newTestKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token":                "", // correct grant, but the token itself is empty
			"permissions":          map[string]string{"metadata": "read"},
			"repository_selection": "selected",
			"repositories":         []map[string]any{{"name": repo, "private": false}},
		})
	}))
	defer srv.Close()

	token, err := getRateLimitOnlyToken(context.Background(), cfg, key, "test-owner", repo, srv.URL)

	if err == nil {
		t.Fatal("expected error for an empty token even with an otherwise-correct grant, got nil")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

// TestRevokeNarrowedToken_SurvivesAlreadyCancelledContext and
// TestRevokeNarrowedToken_SurvivesExpiredDeadlineContext verify #3647 Warden
// finding #3 (MEDIUM): the revoke leg has its own 5s budget, detached from
// the caller's ctx — a cancelled or already-expired parent must not stop the
// DELETE from reaching the network, since a mismatch discovered right at the
// caller's deadline is exactly when the widened token most needs revoking.

func TestRevokeNarrowedToken_SurvivesAlreadyCancelledContext(t *testing.T) {
	var deleteCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleteCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before revokeNarrowedToken is even called

	status, err := revokeNarrowedToken(ctx, srv.URL, "some-token")
	if err != nil {
		t.Fatalf("unexpected error with an already-cancelled parent context: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", status)
	}
	if !deleteCalled {
		t.Fatal("expected DELETE to reach the mock server despite the cancelled parent context")
	}
}

func TestRevokeNarrowedToken_SurvivesExpiredDeadlineContext(t *testing.T) {
	var deleteCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deleteCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	status, err := revokeNarrowedToken(ctx, srv.URL, "some-token")
	if err != nil {
		t.Fatalf("unexpected error with an already-expired parent deadline: %v", err)
	}
	if status != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", status)
	}
	if !deleteCalled {
		t.Fatal("expected DELETE to reach the mock server despite the expired parent deadline")
	}
}

// TestGetRateLimitOnlyToken_CacheIsolation verifies #3647's cache-isolation
// requirement. getRateLimitOnlyToken's signature (ctx, cfg, key, owner, repo,
// apiBaseURL) has no cacheDir parameter, so it cannot call cacheGet/cachePut
// against a directory it was never given — but that alone would make a test
// against an arbitrary t.TempDir() tautological: it can't observe the
// realistic regression, which is someone adding a call to
// defaultCacheDir()+cachePut inside this function, matching cmdToken's own
// convention. t.Setenv("HOME", ...) makes defaultCacheDir() resolve into the
// directory this test seeds and watches, so that regression — if
// introduced — would be caught here.
func TestGetRateLimitOnlyToken_CacheIsolation(t *testing.T) {
	const installID = int64(12345)
	const sentinelToken = "ghs_SENTINEL_full_token_must_not_be_touched"
	const narrowedToken = "ghs_narrow_token_from_mock"
	const repo = "mnemra-core"

	home := t.TempDir()
	t.Setenv("HOME", home) // anchors defaultCacheDir() to a directory this test watches

	cfg := newTestConfig(installID)
	key := newTestKey(t)

	cacheDir := defaultCacheDir()
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("MkdirAll cache dir: %v", err)
	}
	if err := cachePut(cacheDir, installID, CachedToken{
		Token:  sentinelToken,
		Expiry: time.Now().Add(55 * time.Minute),
	}); err != nil {
		t.Fatalf("cachePut seed: %v", err)
	}

	cachePath := cacheFilePath(cacheDir, installID)
	before, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat seeded cache file: %v", err)
	}
	beforeBytes, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read seeded cache file: %v", err)
	}
	entriesBefore, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir before: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"token":                narrowedToken,
			"permissions":          map[string]string{"metadata": "read"},
			"repository_selection": "selected",
			"repositories":         []map[string]any{{"name": repo, "private": false}},
		})
	}))
	defer srv.Close()

	token, err := getRateLimitOnlyToken(context.Background(), cfg, key, "test-owner", repo, srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != narrowedToken {
		t.Errorf("token: got %q, want mock's narrowed token %q (not the cached sentinel)", token, narrowedToken)
	}

	after, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat cache file after call: %v", err)
	}
	afterBytes, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache file after call: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("cache file mtime changed: before=%v after=%v", before.ModTime(), after.ModTime())
	}
	if string(afterBytes) != string(beforeBytes) {
		t.Errorf("cache file bytes changed:\nbefore=%s\nafter=%s", beforeBytes, afterBytes)
	}

	entriesAfter, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir after: %v", err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("cache dir entry count changed: before=%d after=%d", len(entriesBefore), len(entriesAfter))
	}
	for i := range entriesBefore {
		if entriesBefore[i].Name() != entriesAfter[i].Name() {
			t.Errorf("cache dir contents changed: before=%q after=%q", entriesBefore[i].Name(), entriesAfter[i].Name())
		}
	}
}

package main

// AC#4 (exchange layer): Installation-token exchange against an httptest mock GitHub server.
// These tests cover the raw HTTP exchange function in isolation.
// The re-mint-on-401 path (higher level) is tested in token_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// githubInstallationResponse matches the relevant subset of GitHub's
// POST /app/installations/{id}/access_tokens response.
type githubInstallationResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// TestExchangeInstallationToken_HappyPath verifies that a successful GitHub API
// response is parsed into the correct token string and expiry time.
func TestExchangeInstallationToken_HappyPath(t *testing.T) {
	// Given: a mock GitHub API server that returns a valid installation token
	wantToken := "ghs_test_token_abc123"
	wantExpiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	installID := int64(12345)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the correct endpoint is called
		wantPath := fmt.Sprintf("/app/installations/%d/access_tokens", installID)
		if r.URL.Path != wantPath {
			t.Errorf("path: got %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q, want POST", r.Method)
		}
		// Verify Authorization header contains a Bearer token
		auth := r.Header.Get("Authorization")
		if len(auth) < len("Bearer ") || auth[:7] != "Bearer " {
			t.Errorf("Authorization header: got %q, want Bearer <jwt>", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(githubInstallationResponse{
			Token:     wantToken,
			ExpiresAt: wantExpiry.Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// When: exchangeInstallationToken is called with the mock server as apiBaseURL
	token, expiry, err := exchangeInstallationToken(context.Background(), srv.URL, "test.jwt.token", installID)

	// Then: no error and the token matches the mock response
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token: got %q, want %q", token, wantToken)
	}
	if !expiry.Equal(wantExpiry) {
		t.Errorf("expiry: got %v, want %v", expiry, wantExpiry)
	}
}

// TestExchangeInstallationToken_401ReturnsError verifies that a 401 Unauthorized
// response causes exchangeInstallationToken to return an error. The caller
// (getInstallationToken) is responsible for re-minting the JWT and retrying.
func TestExchangeInstallationToken_401ReturnsError(t *testing.T) {
	// Given: a mock GitHub API that returns 401 Unauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
	}))
	defer srv.Close()

	// When: exchangeInstallationToken is called
	_, _, err := exchangeInstallationToken(context.Background(), srv.URL, "expired.jwt", 12345)

	// Then: an error is returned (caller will handle re-mint)
	if err == nil {
		t.Fatal("expected error on 401 Unauthorized, got nil")
	}
}

// TestExchangeInstallationToken_ServerErrorReturnsError verifies that any non-2xx
// response (e.g. 500 Internal Server Error) causes an error to be returned.
func TestExchangeInstallationToken_ServerErrorReturnsError(t *testing.T) {
	// Given: a mock server that returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"message":"Internal Server Error"}`)
	}))
	defer srv.Close()

	// When: exchangeInstallationToken is called
	_, _, err := exchangeInstallationToken(context.Background(), srv.URL, "test.jwt", 12345)

	// Then: an error is returned
	if err == nil {
		t.Fatal("expected error on 500 Internal Server Error, got nil")
	}
}

// TestExchangeInstallationToken_404ReturnsError verifies that a 404 (installation
// not found) causes an error, enabling callers to distinguish it from auth errors.
func TestExchangeInstallationToken_404ReturnsError(t *testing.T) {
	// Given: a mock server returning 404 (installation ID not found)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
	}))
	defer srv.Close()

	// When: exchangeInstallationToken is called
	_, _, err := exchangeInstallationToken(context.Background(), srv.URL, "test.jwt", 99999)

	// Then: an error is returned
	if err == nil {
		t.Fatal("expected error on 404 Not Found, got nil")
	}
}

// TestExchangeInstallationToken_401ReturnsErrUnauthorizedSentinel strengthens the
// basic 401 test by asserting the specific sentinel error (errUnauthorized) is in
// the error chain. errors.Is catches regressions that return a generic error for
// 401, which would silently break the re-mint path in getInstallationToken.
//
// N2 status: GREEN with current implementation (exchange.go:49-50 already returns
// errUnauthorized). This test documents the sentinel contract and guards against
// accidental replacement with a generic fmt.Errorf.
func TestExchangeInstallationToken_401ReturnsErrUnauthorizedSentinel(t *testing.T) {
	// Given: a mock server that returns 401 Unauthorized.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// When: exchangeInstallationToken is called.
	_, _, err := exchangeInstallationToken(context.Background(), srv.URL, "some.jwt", 12345)

	// Then: the error is non-nil.
	if err == nil {
		t.Fatal("expected errUnauthorized on 401, got nil")
	}
	// And: the specific sentinel is in the chain (not just any error).
	if !errors.Is(err, errUnauthorized) {
		t.Errorf("expected errors.Is(err, errUnauthorized); got: %v", err)
	}
}

// TestExchangeInstallationToken_CorrectInstallID verifies that the installation ID
// appears in the API request URL path as required by the GitHub API.
func TestExchangeInstallationToken_CorrectInstallID(t *testing.T) {
	// Given: two distinct installation IDs to test routing
	tests := []struct {
		installID int64
	}{
		{12345},
		{99999},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("installID=%d", tc.installID), func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(githubInstallationResponse{
					Token:     "tok",
					ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				})
			}))
			defer srv.Close()

			// When: exchangeInstallationToken is called with a specific installID
			_, _, err := exchangeInstallationToken(context.Background(), srv.URL, "jwt", tc.installID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Then: the path contains the correct installation ID
			wantPath := fmt.Sprintf("/app/installations/%d/access_tokens", tc.installID)
			if gotPath != wantPath {
				t.Errorf("URL path: got %q, want %q", gotPath, wantPath)
			}
		})
	}
}

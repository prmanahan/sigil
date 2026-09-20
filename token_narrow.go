package main

// token_narrow.go — getRateLimitOnlyToken mints a "rate-limit-only" installation
// token: permissions narrowed to metadata:read, scoped to exactly one repository.
//
// Sibling to token.go's getInstallationToken, not a variant of it: this path
// never calls cacheGet or cachePut. #3647's measurement is why — a token this
// narrow is a one-shot credential for a single unattended rate-limit bump, not
// a long-lived one worth persisting, and caching it would need a second cache
// key (installID alone collides with the full-permission cache entry).
//
// #3647 also measured that GitHub's grant does not always match the request:
// an empty `permissions: {}` body was granted all seven installation
// permissions including write, and an empty `repositories: []` body reverted
// `repository_selection` to "all". verifyNarrowedGrant exists to catch exactly
// that class before a token is trusted or printed.

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// errGrantMismatch is returned when GitHub's actual grant on a narrowed token
// does not exactly match what was requested. What GitHub granted counts, not
// what was asked for.
var errGrantMismatch = errors.New("sigil: rate-limit-only token grant did not match the request")

// narrowedTokenRequest is the fixed shape of the access_tokens POST body for a
// rate-limit-only token. Permissions and Repositories are never built from
// caller input beyond the single repo name.
type narrowedTokenRequest struct {
	Permissions  map[string]string `json:"permissions"`
	Repositories []string          `json:"repositories"`
}

// narrowedTokenResponse decodes the fields of a POST .../access_tokens
// response needed to verify the grant.
type narrowedTokenResponse struct {
	Token               string            `json:"token"`
	Permissions         map[string]string `json:"permissions"`
	RepositorySelection string            `json:"repository_selection"`
	Repositories        []struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	} `json:"repositories"`
}

// getRateLimitOnlyToken resolves the installation for owner (fail-closed, same
// routing as the cached path), mints a token narrowed to metadata:read on
// exactly repo, and verifies GitHub's actual grant before returning it. On a
// grant mismatch it revokes the token and returns an error instead. Never
// reads or writes the token cache.
func getRateLimitOnlyToken(ctx context.Context, cfg *Config, key *rsa.PrivateKey, owner, repo, apiBaseURL string) (string, error) {
	installID, err := resolveInstallation(cfg, owner)
	if err != nil {
		return "", fmt.Errorf("getRateLimitOnlyToken: %w", err)
	}

	appJWT, err := mintAppJWT(key, cfg.AppID, time.Now())
	if err != nil {
		return "", fmt.Errorf("getRateLimitOnlyToken: mint JWT: %w", err)
	}

	resp, err := exchangeNarrowedToken(ctx, apiBaseURL, appJWT, installID, repo)
	if err != nil {
		if !errors.Is(err, errUnauthorized) {
			return "", fmt.Errorf("getRateLimitOnlyToken: exchange: %w", err)
		}
		// Mirrors getInstallationToken's re-mint-on-401 retry: a JWT can be
		// rejected as stale for the same reasons on either path.
		appJWT, err = mintAppJWT(key, cfg.AppID, time.Now())
		if err != nil {
			return "", fmt.Errorf("getRateLimitOnlyToken: re-mint JWT: %w", err)
		}
		resp, err = exchangeNarrowedToken(ctx, apiBaseURL, appJWT, installID, repo)
		if err != nil {
			return "", fmt.Errorf("getRateLimitOnlyToken: exchange (retry): %w", err)
		}
	}

	// Warden review (#3647): checked before the grant match, not after — a
	// 201 with an empty token is never revocable (an empty bearer credential
	// is not a live grant to undo) regardless of whether the grant would
	// otherwise have matched, so there is nothing the mismatch branch below
	// could usefully attempt against it. Checking it first also keeps that
	// branch's revoke-failure error meaning what it says: if the mismatch
	// branch's own revoke fails, it's because the DELETE call failed, not
	// because there was never a token to send.
	if resp.Token == "" {
		return "", errors.New("getRateLimitOnlyToken: access_tokens response had an empty token")
	}

	if err := verifyNarrowedGrant(resp, repo); err != nil {
		revokeStatus, revokeErr := revokeNarrowedToken(ctx, apiBaseURL, resp.Token)
		if revokeErr != nil {
			return "", fmt.Errorf("getRateLimitOnlyToken: grant mismatch (%w), and revoke failed: %v", err, revokeErr)
		}
		return "", fmt.Errorf("getRateLimitOnlyToken: grant mismatch (revoked, status %d): %w", revokeStatus, err)
	}

	return resp.Token, nil
}

// exchangeNarrowedToken POSTs a fixed, narrowed body to the access_tokens
// endpoint: {"permissions":{"metadata":"read"},"repositories":["<repo>"]}.
// Reuses exchange.go's shared HTTP client and errUnauthorized sentinel for the
// same 401 semantics as the full-permission path; does not call
// exchangeInstallationToken itself, which always sends a nil body.
func exchangeNarrowedToken(ctx context.Context, apiBaseURL, appJWT string, installID int64, repo string) (*narrowedTokenResponse, error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", apiBaseURL, installID)

	reqBody := narrowedTokenRequest{
		Permissions:  map[string]string{"metadata": "read"},
		Repositories: []string{repo},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("exchangeNarrowedToken: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("exchangeNarrowedToken: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := exchangeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchangeNarrowedToken: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("exchangeNarrowedToken: unexpected status %d for installation %d", resp.StatusCode, installID)
	}

	var out narrowedTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("exchangeNarrowedToken: decode response: %w", err)
	}
	return &out, nil
}

// permissionKeysSorted returns the sorted key names of a permissions map, for
// error messages. Warden review (#3647): reporting the raw map via %v put the
// installation's actual granted permission *levels* (read/write/admin) on
// stderr, against this file's own "types, status codes and lengths — never
// bytes" convention. Key names alone are enough to diagnose a mismatch
// (extra key, wrong count) without repeating what GitHub granted.
func permissionKeysSorted(perms map[string]string) []string {
	keys := make([]string, 0, len(perms))
	for k := range perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// verifyNarrowedGrant checks GitHub's actual grant against the exact request
// that was made — all four conditions ANDed, exact match, never inferred.
// #3647 measured that permissions["metadata"]=="read" alone is not sufficient:
// it also holds for a full-access grant that additionally includes write.
func verifyNarrowedGrant(resp *narrowedTokenResponse, repo string) error {
	if len(resp.Permissions) != 1 || resp.Permissions["metadata"] != "read" {
		return fmt.Errorf("%w: granted permission key(s) %q, want exactly %q (metadata:read)",
			errGrantMismatch, permissionKeysSorted(resp.Permissions), []string{"metadata"})
	}
	if resp.RepositorySelection != "selected" {
		return fmt.Errorf("%w: repository_selection=%q, want %q", errGrantMismatch, resp.RepositorySelection, "selected")
	}
	if len(resp.Repositories) != 1 {
		return fmt.Errorf("%w: got %d repositories, want exactly 1", errGrantMismatch, len(resp.Repositories))
	}
	if resp.Repositories[0].Name != repo {
		return fmt.Errorf("%w: repository name %q, want %q", errGrantMismatch, resp.Repositories[0].Name, repo)
	}
	if resp.Repositories[0].Private {
		return fmt.Errorf("%w: repository %q is private, want public", errGrantMismatch, repo)
	}
	return nil
}

// revokeNarrowedToken calls DELETE /installation/token, authenticated with the
// token itself (GitHub's self-revoke endpoint). Used only on a grant mismatch,
// so a rejected token never outlives this call. Returns the status only —
// never the token.
//
// Runs on its own 5s budget, detached from the caller's cancellation and
// deadline (Warden review, #3647): the caller's ctx may already be cancelled
// or past its deadline by the time a grant mismatch is discovered — e.g. the
// mint plus one 401 retry burned the full 30s budget — and that is exactly
// when a widened token most needs revoking, not when the revoke should be
// allowed to give up without reaching the network.
func revokeNarrowedToken(ctx context.Context, apiBaseURL, token string) (int, error) {
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(revokeCtx, http.MethodDelete, apiBaseURL+"/installation/token", nil)
	if err != nil {
		return 0, fmt.Errorf("revokeNarrowedToken: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := exchangeHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("revokeNarrowedToken: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return resp.StatusCode, fmt.Errorf("revokeNarrowedToken: unexpected status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

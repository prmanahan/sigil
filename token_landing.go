package main

// token_landing.go — getLandingToken mints the lead-session "landing" token:
// the fixed five-permission set of the spec's API Contract, scoped to exactly
// one repository, verified against GitHub's actual grant before it is trusted
// or printed. `sigil token --landing <owner>/<repo>` is its only caller.
//
// Sibling to token_narrow.go, not a variant of it: the narrowed path's three
// function bodies are frozen (spec R-6), so the exchange and the verifier are
// copied here rather than parameterised. Two copies is below the extract
// threshold. It shares token_narrow.go's request/response types and
// revokeNarrowedToken, unchanged.
//
// Like token_narrow.go it never imports cache.go: a landing token is minted
// per call and never stored (spec R-5, R-7).
//
// Two departures from the narrowed verifier, both by the spec (R-4): the
// permission constant is the five-key landing set, and a private repository
// is accepted. The repository name is still matched exactly, letter case
// included.

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

// errLandingGrantMismatch is returned when GitHub's actual grant on a landing
// token does not match the request. What GitHub granted counts, not what was
// asked for.
var errLandingGrantMismatch = errors.New("sigil: landing token grant did not match the request")

// landingPermissions returns the API Contract's permission constant. A fresh
// map per call, so no caller can widen the set for a later one. Nothing takes
// a permissions input from the caller (spec R-7).
func landingPermissions() map[string]string {
	return map[string]string{
		"contents":      "write",
		"pull_requests": "write",
		"checks":        "read",
		"workflows":     "write",
		"metadata":      "read",
	}
}

// getLandingToken resolves the installation for owner (fail-closed), mints a
// token with landingPermissions on exactly repo, and verifies GitHub's actual
// grant before returning it. On a mismatch it revokes the token and returns an
// error instead. Never reads or writes the token cache, and writes to neither
// os.Stdout nor os.Stderr.
func getLandingToken(ctx context.Context, cfg *Config, key *rsa.PrivateKey, owner, repo, apiBaseURL string) (string, error) {
	installID, err := resolveInstallation(cfg, owner)
	if err != nil {
		return "", fmt.Errorf("getLandingToken: %w", err)
	}

	appJWT, err := mintAppJWT(key, cfg.AppID, time.Now())
	if err != nil {
		return "", fmt.Errorf("getLandingToken: mint JWT: %w", err)
	}

	resp, err := exchangeLandingToken(ctx, apiBaseURL, appJWT, installID, repo)
	if err != nil {
		if !errors.Is(err, errUnauthorized) {
			return "", fmt.Errorf("getLandingToken: exchange: %w", err)
		}
		// One re-mint on 401, as on the other token paths: a JWT can be
		// rejected as stale. A second 401 is an error, not a third attempt.
		appJWT, err = mintAppJWT(key, cfg.AppID, time.Now())
		if err != nil {
			return "", fmt.Errorf("getLandingToken: re-mint JWT: %w", err)
		}
		resp, err = exchangeLandingToken(ctx, apiBaseURL, appJWT, installID, repo)
		if err != nil {
			return "", fmt.Errorf("getLandingToken: exchange (retry): %w", err)
		}
	}

	// An empty token is checked before the grant: there is no live credential
	// to revoke, so the mismatch branch's revoke could only fail for the wrong
	// reason.
	if resp.Token == "" {
		return "", errors.New("getLandingToken: access_tokens response had an empty token")
	}

	if err := verifyLandingGrant(resp, repo); err != nil {
		revokeStatus, revokeErr := revokeNarrowedToken(ctx, apiBaseURL, resp.Token)
		if revokeErr != nil {
			return "", fmt.Errorf("getLandingToken: grant mismatch (%w), and revoke failed: %v", err, revokeErr)
		}
		return "", fmt.Errorf("getLandingToken: grant mismatch (revoked, status %d): %w", revokeStatus, err)
	}

	return resp.Token, nil
}

// exchangeLandingToken POSTs the fixed landing body to the access_tokens
// endpoint: the five-permission constant and repositories [repo]. The repo
// name is the only caller input. Reuses exchange.go's shared HTTP client and
// errUnauthorized sentinel.
func exchangeLandingToken(ctx context.Context, apiBaseURL, appJWT string, installID int64, repo string) (*narrowedTokenResponse, error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", apiBaseURL, installID)

	bodyBytes, err := json.Marshal(narrowedTokenRequest{
		Permissions:  landingPermissions(),
		Repositories: []string{repo},
	})
	if err != nil {
		return nil, fmt.Errorf("exchangeLandingToken: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("exchangeLandingToken: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := exchangeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchangeLandingToken: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("exchangeLandingToken: unexpected status %d for installation %d", resp.StatusCode, installID)
	}

	var out narrowedTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		// A 201 is a live credential. If a token was decoded before the failure
		// (encoding/json keeps decoding after a type error, so the token can be
		// filled whatever the field order), it must not be left live: the grant
		// was never verified. Revoke it, and report both failures.
		if out.Token != "" {
			if _, revokeErr := revokeNarrowedToken(ctx, apiBaseURL, out.Token); revokeErr != nil {
				return nil, fmt.Errorf("exchangeLandingToken: decode response: %w, and revoke failed: %v", err, revokeErr)
			}
		}
		return nil, fmt.Errorf("exchangeLandingToken: decode response: %w", err)
	}
	return &out, nil
}

// verifyLandingGrant checks GitHub's actual grant against the request, all
// conditions ANDed and exact: the permission map equals landingPermissions in
// both directions (keys and levels), repository_selection is "selected",
// exactly one repository is listed, and its name equals repo.
//
// Error text carries permission key names only, never a granted level, so
// stderr does not repeat what GitHub granted (the convention in
// permissionKeysSorted). Neither `private` nor the owner is checked.
func verifyLandingGrant(resp *narrowedTokenResponse, repo string) error {
	want := landingPermissions()
	var extra, missing, differing []string
	for k := range resp.Permissions {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	for k, level := range want {
		got, ok := resp.Permissions[k]
		switch {
		case !ok:
			missing = append(missing, k)
		case got != level:
			differing = append(differing, k)
		}
	}
	if len(extra)+len(missing)+len(differing) > 0 {
		sort.Strings(extra)
		sort.Strings(missing)
		sort.Strings(differing)
		return fmt.Errorf("%w: permissions differ from the contract (extra key(s) %q, missing key(s) %q, level differs for key(s) %q)",
			errLandingGrantMismatch, extra, missing, differing)
	}
	if resp.RepositorySelection != "selected" {
		return fmt.Errorf("%w: repository_selection=%q, want %q", errLandingGrantMismatch, resp.RepositorySelection, "selected")
	}
	if len(resp.Repositories) != 1 {
		return fmt.Errorf("%w: got %d repositories, want exactly 1", errLandingGrantMismatch, len(resp.Repositories))
	}
	if resp.Repositories[0].Name != repo {
		return fmt.Errorf("%w: repository name %q, want %q", errLandingGrantMismatch, resp.Repositories[0].Name, repo)
	}
	return nil
}

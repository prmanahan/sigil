package main

// token.go — getInstallationToken is the top-level orchestrator that implements
// the full mint-cache-retry flow.
//
// Orchestration order (enforced by tests):
//  1. resolveInstallation — fail closed before any network call for unknown owners
//  2. cacheGet(installID) — return cached token on hit
//  3. mintAppJWT
//  4. exchangeInstallationToken
//  5. On 401: re-mint a fresh JWT and retry the exchange exactly once
//  6. cachePut the new token
//  7. Return the token string

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"
)

// getInstallationToken returns a valid installation access token for the repo
// owner extracted from repoPath. It uses the cache when possible, mints a new
// token when the cache is empty or expired, and retries once on a 401.
func getInstallationToken(
	ctx context.Context,
	cfg *Config,
	key *rsa.PrivateKey,
	repoPath string,
	apiBaseURL string,
	cacheDir string,
) (string, error) {
	// Step 1: resolve installation ID first — fail closed before any network call.
	installID, err := resolveInstallation(cfg, repoPath)
	if err != nil {
		return "", fmt.Errorf("getInstallationToken: %w", err)
	}

	// Step 2: cache hit — return without minting.
	cached, err := cacheGet(cacheDir, installID)
	if err != nil {
		return "", fmt.Errorf("getInstallationToken: cache read: %w", err)
	}
	if cached != nil {
		return cached.Token, nil
	}

	// Step 3: mint App JWT.
	appJWT, err := mintAppJWT(key, cfg.AppID, time.Now())
	if err != nil {
		return "", fmt.Errorf("getInstallationToken: mint JWT: %w", err)
	}

	// Step 4: exchange for installation token.
	token, expiry, err := exchangeInstallationToken(ctx, apiBaseURL, appJWT, installID)
	if err != nil {
		// Step 5: on 401, re-mint a fresh JWT and retry exactly once.
		if !errors.Is(err, errUnauthorized) {
			return "", fmt.Errorf("getInstallationToken: exchange: %w", err)
		}

		appJWT, err = mintAppJWT(key, cfg.AppID, time.Now())
		if err != nil {
			return "", fmt.Errorf("getInstallationToken: re-mint JWT: %w", err)
		}
		token, expiry, err = exchangeInstallationToken(ctx, apiBaseURL, appJWT, installID)
		if err != nil {
			return "", fmt.Errorf("getInstallationToken: exchange (retry): %w", err)
		}
	}

	// Step 6: cache the token with a 5-minute buffer before the GitHub expiry.
	ct := CachedToken{
		Token:  token,
		Expiry: expiry.Add(-5 * time.Minute),
	}
	// Cache failure is non-fatal: we already have the token and can return it.
	// The next call will simply re-mint rather than hitting cache.
	_ = cachePut(cacheDir, installID, ct)

	// Step 7: return the token.
	return token, nil
}

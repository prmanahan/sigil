package main

// token.go — getInstallationTokenLazy is the top-level orchestrator that
// implements the full cache-mint-retry flow for the full-permission path.
//
// Orchestration order, and what the tests pin:
//  1. resolveInstallation — fail closed before any network call or key read
//     for unknown owners (pinned: lazy_key_test.go, cli_cache_test.go)
//  2. cacheGet(installID) — return a live cached token with no key read
//     (pinned: lazy_key_test.go, cli_cache_test.go)
//  3. load the private key (first miss only), then mintAppJWT
//     (pinned: loadKey is called exactly once across a 401 retry)
//  4. exchangeInstallationToken
//  5. On 401: re-mint a fresh JWT and retry the exchange exactly once.
//     Pinned: the retry happens, the key is not loaded a second time, and the
//     retry's JWT is freshly minted (token_test.go holds the first 401 until the
//     wall-clock second advances, so the two Authorization headers must differ;
//     no clock seam is needed).
//  6. cachePut the new token, expiry minus five minutes (pinned)
//  7. Return the token string
//
// getInstallationTokenLazy's own error messages carry its name as a prefix.

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"
)

// getInstallationTokenLazy returns a valid installation access token for the
// repo owner extracted from repoPath. It uses the cache when possible, mints a
// new token when the cache is empty or expired, and retries once on a 401.
//
// loadKey is called at most once, and only after routing and the cache lookup
// have both failed to answer: an unknown owner or a cache hit never reads the
// private key (and so never touches 1Password). It receives ctx, so one
// deadline bounds the key read and the exchange alike. The loaded key is
// reused for the 401 retry.
func getInstallationTokenLazy(
	ctx context.Context,
	cfg *Config,
	loadKey func(context.Context) (*rsa.PrivateKey, error),
	repoPath string,
	apiBaseURL string,
	cacheDir string,
) (string, error) {
	// Step 1: resolve installation ID first — fail closed before any network call.
	installID, err := resolveInstallation(cfg, repoPath)
	if err != nil {
		return "", fmt.Errorf("getInstallationTokenLazy: %w", err)
	}

	// Step 2: cache hit — return without minting.
	cached, err := cacheGet(cacheDir, installID)
	if err != nil {
		return "", fmt.Errorf("getInstallationTokenLazy: cache read: %w", err)
	}
	if cached != nil {
		return cached.Token, nil
	}

	// Step 3: cache miss — only now is the private key needed.
	key, err := loadKey(ctx)
	if err != nil {
		return "", err
	}

	appJWT, err := mintAppJWT(key, cfg.AppID, time.Now())
	if err != nil {
		return "", fmt.Errorf("getInstallationTokenLazy: mint JWT: %w", err)
	}

	// Step 4: exchange for installation token.
	token, expiry, err := exchangeInstallationToken(ctx, apiBaseURL, appJWT, installID)
	if err != nil {
		// Step 5: on 401, re-mint a fresh JWT and retry exactly once.
		if !errors.Is(err, errUnauthorized) {
			return "", fmt.Errorf("getInstallationTokenLazy: exchange: %w", err)
		}

		appJWT, err = mintAppJWT(key, cfg.AppID, time.Now())
		if err != nil {
			return "", fmt.Errorf("getInstallationTokenLazy: re-mint JWT: %w", err)
		}
		token, expiry, err = exchangeInstallationToken(ctx, apiBaseURL, appJWT, installID)
		if err != nil {
			return "", fmt.Errorf("getInstallationTokenLazy: exchange (retry): %w", err)
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

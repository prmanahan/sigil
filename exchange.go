package main

// exchange.go — exchangeInstallationToken calls the GitHub API to obtain a
// short-lived installation access token authenticated with the App JWT.
//
// Security invariant: the returned token leaves this function only as a return
// value; it is never logged or written to stderr here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// errUnauthorized is the sentinel returned when GitHub responds with 401.
// getInstallationTokenLazy uses errors.Is on this to trigger the re-mint retry path.
var errUnauthorized = errors.New("github API: 401 Unauthorized")

// exchangeHTTPClient is used for all GitHub API calls. Its 30-second timeout is
// a backstop for a caller that passes a context with no deadline; the CLI entry
// points pass their own 30 s context. A context deadline shorter than 30 s takes
// precedence because http.NewRequestWithContext propagates it through the transport.
var exchangeHTTPClient = &http.Client{Timeout: 30 * time.Second}

// installationTokenResponse is the subset of GitHub's access_tokens response
// that Sigil uses.
type installationTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// exchangeInstallationToken calls POST {apiBaseURL}/app/installations/{installID}/access_tokens
// authenticated with the provided App JWT. Returns the installation token and its
// expiry on 201 Created. Returns errUnauthorized for 401; a generic error for any
// other non-2xx response.
func exchangeInstallationToken(ctx context.Context, apiBaseURL, appJWT string, installID int64) (token string, expiry time.Time, err error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", apiBaseURL, installID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("exchangeInstallationToken: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := exchangeHTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("exchangeInstallationToken: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", time.Time{}, errUnauthorized
	}

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("exchangeInstallationToken: unexpected status %d for installation %d", resp.StatusCode, installID)
	}

	var body installationTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", time.Time{}, fmt.Errorf("exchangeInstallationToken: decode response: %w", err)
	}

	exp, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		// Neither the value nor err is quoted: time.Parse's error embeds the
		// offending response text, and error messages never carry response bytes.
		return "", time.Time{}, errors.New("exchangeInstallationToken: expires_at is not an RFC 3339 timestamp")
	}

	// A 201 with an empty token must not pass as a credential.
	if body.Token == "" {
		return "", time.Time{}, errors.New("exchangeInstallationToken: access_tokens response had an empty token")
	}

	return body.Token, exp, nil
}

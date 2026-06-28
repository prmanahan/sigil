package main

// jwt.go — mintAppJWT signs an RS256 GitHub App JWT.
//
// GitHub App JWT requirements:
//   - alg: RS256
//   - iat: now − 60 s  (compensates for clock skew between caller and GitHub)
//   - exp: ≤ now + 10 min  (GitHub hard limit)
//   - iss: App ID (numeric)

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// mintAppJWT signs an RS256 GitHub App JWT with the given private key.
// The now parameter makes claim bounds testable without real-clock dependency.
func mintAppJWT(key *rsa.PrivateKey, appID int64, now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("mintAppJWT: sign: %w", err)
	}

	return signed, nil
}

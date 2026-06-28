package main

// AC#3: JWT: valid RS256 with iss=app_id and iat/exp in bounds.
// Verify with the public key in-test (the RSA keypair is generated here).
//
// White-box unit tests for mintAppJWT. The keypair is generated in each test so
// these tests exercise the cryptographic contract without touching 1Password.

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateTestKey creates a fresh 2048-bit RSA key for use in a single test.
// Panics on failure so tests that call it always get a usable key.
func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("RSA key generation failed: %v", err)
	}
	return key
}

// parseAndVerifyJWT parses a JWT string and verifies its RS256 signature with
// the provided public key. Returns the parsed token or fails the test.
// Time-claim validation is intentionally disabled: iat/exp bounds are asserted
// separately in each test via ParseUnverified + explicit arithmetic, so this
// helper's sole job is signature verification.
func parseAndVerifyJWT(t *testing.T, tokenStr string, pub *rsa.PublicKey) *jwt.Token {
	t.Helper()
	parsed, err := jwt.NewParser(jwt.WithoutClaimsValidation()).Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return pub, nil
	})
	if err != nil {
		t.Fatalf("JWT parse/verify failed: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("JWT is not valid after parse")
	}
	return parsed
}

// TestMintAppJWT_SignatureVerifiesWithPublicKey verifies that the JWT produced by
// mintAppJWT can be verified using the corresponding public key (RS256 round-trip).
func TestMintAppJWT_SignatureVerifiesWithPublicKey(t *testing.T) {
	// Given: a freshly generated RSA key pair
	key := generateTestKey(t)
	now := time.Now()

	// When: a JWT is minted for app ID 99
	tokenStr, err := mintAppJWT(key, 99, now)

	// Then: no error
	if err != nil {
		t.Fatalf("mintAppJWT failed: %v", err)
	}
	// And: the JWT verifies with the corresponding public key (AC#3)
	parseAndVerifyJWT(t, tokenStr, &key.PublicKey)
}

// TestMintAppJWT_AlgorithmIsRS256 verifies that the JWT header declares alg=RS256,
// which is required by the GitHub App authentication spec.
func TestMintAppJWT_AlgorithmIsRS256(t *testing.T) {
	// Given: a key and now
	key := generateTestKey(t)
	now := time.Now()

	// When: a JWT is minted
	tokenStr, err := mintAppJWT(key, 1, now)
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	// Then: parse header without verification to check alg
	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	alg, ok := parsed.Header["alg"].(string)
	if !ok || alg != "RS256" {
		t.Errorf("alg header: got %v, want RS256", parsed.Header["alg"])
	}
}

// TestMintAppJWT_IssIsAppID verifies that the iss (issuer) claim equals the AppID,
// as required by the GitHub App JWT spec (iss = App ID).
func TestMintAppJWT_IssIsAppID(t *testing.T) {
	// Given: a key and a specific app ID
	key := generateTestKey(t)
	appID := int64(12345)
	now := time.Now()

	// When: a JWT is minted
	tokenStr, err := mintAppJWT(key, appID, now)
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	// Parse with verification
	parsed := parseAndVerifyJWT(t, tokenStr, &key.PublicKey)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected jwt.MapClaims")
	}

	// Then: iss claim equals the appID
	// GitHub accepts iss as either a numeric string or a JSON number.
	issRaw := claims["iss"]
	switch v := issRaw.(type) {
	case float64:
		if int64(v) != appID {
			t.Errorf("iss (numeric): got %v, want %d", v, appID)
		}
	case string:
		want := fmt.Sprintf("%d", appID)
		if v != want {
			t.Errorf("iss (string): got %q, want %q", v, want)
		}
	default:
		t.Errorf("iss: unexpected type %T, value %v", issRaw, issRaw)
	}
}

// TestMintAppJWT_IATIs60SecondsBeforeNow verifies the iat (issued-at) clock skew:
// iat must be set to now − 60 seconds to compensate for clock drift between the
// caller and GitHub's servers, as required by the GitHub App JWT spec.
func TestMintAppJWT_IATIs60SecondsBeforeNow(t *testing.T) {
	// Given: a key and a pinned "now"
	key := generateTestKey(t)
	now := time.Now()

	// When: a JWT is minted
	tokenStr, err := mintAppJWT(key, 1, now)
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	// Parse without time validation so we can inspect the raw iat value
	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)

	// Then: iat = now − 60s (±2s test tolerance)
	iatRaw, ok := claims["iat"].(float64)
	if !ok {
		t.Fatalf("iat claim missing or wrong type: %T %v", claims["iat"], claims["iat"])
	}
	iat := time.Unix(int64(iatRaw), 0)
	wantIAT := now.Add(-60 * time.Second)
	if diff := iat.Sub(wantIAT); diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("iat skew: got %v (want now−60s = %v), diff %v", iat, wantIAT, diff)
	}
}

// TestMintAppJWT_EXPAtMostTenMinutesFromNow verifies that the exp (expiration) is no
// more than 10 minutes in the future from now, enforcing the GitHub App JWT limit.
func TestMintAppJWT_EXPAtMostTenMinutesFromNow(t *testing.T) {
	// Given: a key and a pinned "now"
	key := generateTestKey(t)
	now := time.Now()

	// When: a JWT is minted
	tokenStr, err := mintAppJWT(key, 1, now)
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	// Parse without time validation to inspect raw exp
	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)

	// Then: exp ≤ now + 10 minutes
	expRaw, ok := claims["exp"].(float64)
	if !ok {
		t.Fatalf("exp claim missing or wrong type: %T %v", claims["exp"], claims["exp"])
	}
	exp := time.Unix(int64(expRaw), 0)
	maxExp := now.Add(10 * time.Minute)
	if exp.After(maxExp) {
		t.Errorf("exp exceeds 10-minute limit: got %v, max allowed %v", exp, maxExp)
	}
}

// TestMintAppJWT_EXPIsAfterIAT verifies the basic sanity that exp > iat so the
// token has a positive validity window.
func TestMintAppJWT_EXPIsAfterIAT(t *testing.T) {
	// Given: a key
	key := generateTestKey(t)
	now := time.Now()

	// When: a JWT is minted
	tokenStr, err := mintAppJWT(key, 1, now)
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	parsed, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	iat := time.Unix(int64(claims["iat"].(float64)), 0)
	exp := time.Unix(int64(claims["exp"].(float64)), 0)

	// Then: exp is strictly after iat
	if !exp.After(iat) {
		t.Errorf("exp %v is not after iat %v", exp, iat)
	}
}

// TestMintAppJWT_WrongKeyFailsVerification verifies that a JWT minted with one RSA
// key cannot be verified with a different key — basic tamper-resistance.
//
// N3b status: GREEN with current implementation. jwt.Parse with a wrong public key
// always returns a signature-verification error. This test documents the property
// and guards against a regression that disables signature checking.
func TestMintAppJWT_WrongKeyFailsVerification(t *testing.T) {
	// Given: two distinct RSA key pairs.
	signingKey := generateTestKey(t)
	wrongKey := generateTestKey(t)

	// When: a JWT is minted with the signing key.
	tokenStr, err := mintAppJWT(signingKey, 1, time.Now())
	if err != nil {
		t.Fatalf("mintAppJWT: %v", err)
	}

	// Then: attempting to verify with the wrong public key fails.
	_, err = jwt.NewParser(jwt.WithoutClaimsValidation()).Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &wrongKey.PublicKey, nil
	})
	if err == nil {
		t.Fatal("expected signature verification to fail with a different public key, got nil")
	}
}

// TestMintAppJWT_TableDriven verifies claims across different app IDs and time values.
func TestMintAppJWT_TableDriven(t *testing.T) {
	key := generateTestKey(t)
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		appID int64
		now   time.Time
	}{
		{"small app ID", 1, base},
		{"large app ID", 9_999_999, base},
		{"different time", 42, base.Add(24 * time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: app ID and now from the table
			// When: JWT is minted
			tokenStr, err := mintAppJWT(key, tc.appID, tc.now)
			if err != nil {
				t.Fatalf("mintAppJWT: %v", err)
			}

			// Then: signature verifies
			parseAndVerifyJWT(t, tokenStr, &key.PublicKey)

			// And: iat and exp are in bounds
			parsed, _, _ := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
			claims := parsed.Claims.(jwt.MapClaims)
			iat := time.Unix(int64(claims["iat"].(float64)), 0)
			exp := time.Unix(int64(claims["exp"].(float64)), 0)

			wantIAT := tc.now.Add(-60 * time.Second)
			maxExp := tc.now.Add(10 * time.Minute)

			if diff := iat.Sub(wantIAT); diff < -2*time.Second || diff > 2*time.Second {
				t.Errorf("iat skew out of bounds: got %v, want %v", iat, wantIAT)
			}
			if exp.After(maxExp) {
				t.Errorf("exp exceeds 10min: got %v, max %v", exp, maxExp)
			}
		})
	}
}

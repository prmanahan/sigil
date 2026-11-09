package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"strconv"
	"strings"
	"testing"
)

// Keys are generated in-process: .gitignore blocks *.pem, so no fixture file
// can be committed, and a committed key would be a hardcoded secret anyway.

func pemEncode(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

// assertNoKeyMaterial fails if err's text contains any of the given key bytes
// (der: whatever bytes actually reached the function under test) in a printable
// form: raw, hex (either case), %q-escaped, or a window of its base64. It checks
// three windows spread across der, so a quote that spans a whole window is
// caught wherever it falls, not only at the first bytes. A quote of a shorter
// fragment that fits between windows is not caught. Error messages may quote types and block names,
// never bytes.
func assertNoKeyMaterial(t *testing.T, err error, der []byte) {
	t.Helper()
	msg := err.Error()
	const win = 12 // a multiple of 3, so the base64 window is alignment-independent
	if len(der) < 3*win {
		t.Fatalf("assertNoKeyMaterial: %d bytes is too short for three %d-byte windows", len(der), win)
	}
	for _, off := range []int{0, len(der) / 3, 2 * len(der) / 3} {
		off -= off % 3 // base64 groups align on 3-byte boundaries
		w := der[off : off+win]
		forms := map[string]string{
			"raw":     string(w),
			"hex":     hex.EncodeToString(w),
			"HEX":     strings.ToUpper(hex.EncodeToString(w)),
			"%q":      strings.Trim(strconv.Quote(string(w)), `"`),
			"base64":  base64.StdEncoding.EncodeToString(w),
			"url-b64": base64.URLEncoding.EncodeToString(w),
		}
		for name, f := range forms {
			if strings.Contains(msg, f) {
				t.Errorf("error leaks key material (%s window at %d): %q", name, off, msg)
			}
		}
	}
}

func TestParseRSAPrivateKey_ValidPKCS1(t *testing.T) {
	key := generateTestKey(t)
	got, err := parseRSAPrivateKey(pemEncode("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)))
	if err != nil {
		t.Fatalf("parseRSAPrivateKey: %v", err)
	}
	if !got.Equal(key) {
		t.Error("parsed key does not equal the generated key")
	}
}

func TestParseRSAPrivateKey_ValidPKCS8(t *testing.T) {
	key := generateTestKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	got, err := parseRSAPrivateKey(pemEncode("PRIVATE KEY", der))
	if err != nil {
		t.Fatalf("parseRSAPrivateKey: %v", err)
	}
	if !got.Equal(key) {
		t.Error("parsed key does not equal the generated key")
	}
}

// TestParseRSAPrivateKey_PKCS8NonRSA_Errors covers the type-assertion branch:
// a well-formed PKCS#8 key that is not RSA must be rejected, never returned
// as a nil or mistyped *rsa.PrivateKey.
func TestParseRSAPrivateKey_PKCS8NonRSA_Errors(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	cases := map[string]any{"ecdsa": ecKey, "ed25519": edKey}
	for name, k := range cases {
		t.Run(name, func(t *testing.T) {
			der, err := x509.MarshalPKCS8PrivateKey(k)
			if err != nil {
				t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
			}
			got, err := parseRSAPrivateKey(pemEncode("PRIVATE KEY", der))
			if err == nil {
				t.Fatalf("expected an error for a non-RSA PKCS#8 key, got key %v", got)
			}
			if got != nil {
				t.Errorf("expected nil key on error, got %v", got)
			}
			if !strings.Contains(err.Error(), "not *rsa.PrivateKey") {
				t.Errorf("error should name the type mismatch, got: %v", err)
			}
			assertNoKeyMaterial(t, err, der)
		})
	}
}

func TestParseRSAPrivateKey_NoPEMBlock_Errors(t *testing.T) {
	for name, in := range map[string][]byte{
		"empty":   nil,
		"garbage": []byte("this is not a pem block"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseRSAPrivateKey(in)
			if err == nil {
				t.Fatalf("expected error, got key %v", got)
			}
			if !strings.Contains(err.Error(), "no PEM block found") {
				t.Errorf("error should say no PEM block was found, got: %v", err)
			}
		})
	}
}

// TestParseRSAPrivateKey_StrippedNewlines_Errors reproduces the real field
// failure: a 1Password concealed field strips the newlines from the PEM, and
// the key then reads as "no PEM block found" with no hint about field type.
// Both the PKCS#1 and PKCS#8 encodings are covered.
func TestParseRSAPrivateKey_StrippedNewlines_Errors(t *testing.T) {
	key := generateTestKey(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	cases := map[string]struct {
		blockType string
		der       []byte
	}{
		"pkcs1": {"RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)},
		"pkcs8": {"PRIVATE KEY", pkcs8},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			intact := pemEncode(c.blockType, c.der)
			// Control: the same key with newlines intact parses, so the
			// failure below is attributable to the stripping alone.
			if _, err := parseRSAPrivateKey(intact); err != nil {
				t.Fatalf("control (newlines intact) failed to parse: %v", err)
			}

			stripped := []byte(strings.ReplaceAll(string(intact), "\n", ""))
			got, err := parseRSAPrivateKey(stripped)
			if err == nil {
				t.Fatalf("expected error for newline-stripped PEM, got key %v", got)
			}
			if !strings.Contains(err.Error(), "no PEM block found") {
				t.Errorf("error should say no PEM block was found, got: %v", err)
			}
			assertNoKeyMaterial(t, err, c.der)
		})
	}
}

// TestParseRSAPrivateKey_UnsupportedBlockType_Errors and the corrupt-DER test
// cover the remaining error arms; both quote only a type name or a static
// stdlib message, never bytes.
func TestParseRSAPrivateKey_UnsupportedBlockType_Errors(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	_, err = parseRSAPrivateKey(pemEncode("EC PRIVATE KEY", der))
	if err == nil {
		t.Fatal("expected error for an unsupported PEM block type")
	}
	if !strings.Contains(err.Error(), `"EC PRIVATE KEY"`) {
		t.Errorf("error should name the block type, got: %v", err)
	}
	assertNoKeyMaterial(t, err, der)
}

func TestParseRSAPrivateKey_CorruptDER_Errors(t *testing.T) {
	key := generateTestKey(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	cases := map[string]struct {
		blockType string
		der       []byte
	}{
		"pkcs1": {"RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)},
		"pkcs8": {"PRIVATE KEY", pkcs8},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			truncated := c.der[:len(c.der)/2]
			got, err := parseRSAPrivateKey(pemEncode(c.blockType, truncated))
			if err == nil {
				t.Fatalf("expected error for truncated DER, got key %v", got)
			}
			// The bytes that reached the function are the truncated ones; checking
			// the full DER would test windows the function never saw.
			assertNoKeyMaterial(t, err, truncated)
		})
	}
}

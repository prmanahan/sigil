package main

// main.go — Sigil CLI dispatcher: git credential helper + utility subcommands.
//
// Subcommands:
//   sigil (bare) / sigil get   — git credential helper mode (reads stdin key=value)
//   sigil store                — no-op (git protocol requirement), exit 0
//   sigil erase                — no-op (git protocol requirement), exit 0
//   sigil token <owner>        — mint and print installation token to stdout
//   sigil doctor               — verify config + key + one mint; print safe summary
//
// Security invariants:
//   - Private key: never written to disk; read in-process from 1Password.
//   - Installation token: emitted only via stdout (password= line or sigil token).
//   - SA token: read from env only; never logged or persisted.
//   - All error paths exit non-zero with a message to stderr; no key/token material
//     appears in any error message.

import (
	"bufio"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	onepassword "github.com/1password/onepassword-sdk-go"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sigil:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Bare invocation (git calls the helper with no args) is identical to "get".
	subcommand := "get"
	if len(args) > 0 {
		subcommand = args[0]
	}

	switch subcommand {
	case "get":
		return cmdGet()
	case "store", "erase":
		// No-op: git credential-helper protocol requires these to be handled
		// silently with exit 0.
		return nil
	case "token":
		if len(args) < 2 {
			return errors.New("usage: sigil token <owner>")
		}
		return cmdToken(args[1])
	case "doctor":
		return cmdDoctor()
	default:
		// Unknown subcommand — treat as get (defensive; git may send variants).
		return cmdGet()
	}
}

// cmdGet implements the git credential-helper "get" action.
// Reads key=value pairs from stdin; for GitHub HTTPS requests, emits
// username=x-access-token and password=<token> to stdout. Non-GitHub hosts
// and non-https protocols are no-ops (exit 0, no output).
func cmdGet() error {
	attrs := parseCredentialInput(os.Stdin)

	// Only handle HTTPS requests to github.com; all others are pass-through no-ops.
	if attrs["protocol"] != "https" || attrs["host"] != "github.com" {
		return nil
	}

	cfg, err := loadConfigDefault()
	if err != nil {
		return err
	}

	// L1: validate API base URL scheme before any credential read (fail closed,
	// no credentials required to detect a misconfiguration).
	if err := validateAPIBase(apiBase()); err != nil {
		return err
	}

	key, err := readKey(cfg.OPKeyRef)
	if err != nil {
		return err
	}

	cacheDir := defaultCacheDir()
	if err := ensureCacheDir(cacheDir); err != nil {
		return err
	}

	// M1: bounded context prevents an unbounded hang on an unresponsive API.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := getInstallationToken(ctx, cfg, key, attrs["path"], apiBase(), cacheDir)
	if err != nil {
		return err
	}

	// Token leaves Sigil only via this sanctioned stdout channel.
	writeCredentialResponse(os.Stdout, token)
	return nil
}

// cmdToken mints and prints an installation token to stdout for the given owner.
// Intended for the per-run URL-rewrite fallback and debugging.
func cmdToken(owner string) error {
	cfg, err := loadConfigDefault()
	if err != nil {
		return err
	}

	// L1: validate API base URL scheme before any credential read (fail closed,
	// no credentials required to detect a misconfiguration).
	if err := validateAPIBase(apiBase()); err != nil {
		return err
	}

	key, err := readKey(cfg.OPKeyRef)
	if err != nil {
		return err
	}

	cacheDir := defaultCacheDir()
	if err := ensureCacheDir(cacheDir); err != nil {
		return err
	}

	// M1: bounded context prevents an unbounded hang on an unresponsive API.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := getInstallationToken(ctx, cfg, key, owner, apiBase(), cacheDir)
	if err != nil {
		return err
	}

	// Token to stdout is sanctioned for this subcommand.
	fmt.Println(token)
	return nil
}

// cmdDoctor verifies config + key read + one JWT mint, printing only safe
// confirmations (sizes, expiry, "OK") to stderr. Never prints the key or token.
func cmdDoctor() error {
	cfg, err := loadConfigDefault()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "config: OK (app_id=%d, %d installation(s))\n", cfg.AppID, len(cfg.Installations))

	saToken := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if saToken == "" {
		return errors.New("OP_SERVICE_ACCOUNT_TOKEN is not set")
	}
	fmt.Fprintln(os.Stderr, "OP_SERVICE_ACCOUNT_TOKEN: present")

	key, err := readKey(cfg.OPKeyRef)
	if err != nil {
		return fmt.Errorf("key read: %w", err)
	}
	fmt.Fprintf(os.Stderr, "private key: OK (%d-bit RSA)\n", key.N.BitLen())

	jwtStr, err := mintAppJWT(key, cfg.AppID, time.Now())
	if err != nil {
		return fmt.Errorf("JWT mint: %w", err)
	}
	fmt.Fprintf(os.Stderr, "JWT mint: OK (%d bytes)\n", len(jwtStr))

	return nil
}

// --- helpers ---

// loadConfigDefault resolves the default config path via $HOME and calls loadConfig.
// Using $HOME explicitly (not os.UserConfigDir) so the path is predictable in tests
// that set HOME to a temp dir without setting XDG_CONFIG_HOME.
func loadConfigDefault() (*Config, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return nil, errors.New("HOME environment variable is not set")
	}
	path := filepath.Join(home, ".config", "sigil", "config.toml")
	return loadConfig(path)
}

// readKey reads the RSA private key from 1Password using the service-account token
// from the environment. Fails closed if OP_SERVICE_ACCOUNT_TOKEN is absent.
// The key is never written to disk; it is held in memory for the duration of the call.
func readKey(opKeyRef string) (*rsa.PrivateKey, error) {
	saToken := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if saToken == "" {
		return nil, errors.New("OP_SERVICE_ACCOUNT_TOKEN is not set")
	}

	client, err := onepassword.NewClient(
		context.Background(),
		onepassword.WithServiceAccountToken(saToken),
		onepassword.WithIntegrationInfo("sigil", "v1.0.0"),
	)
	if err != nil {
		return nil, fmt.Errorf("1Password client init: %w", err)
	}

	secret, err := client.Secrets().Resolve(context.Background(), opKeyRef)
	if err != nil {
		return nil, fmt.Errorf("resolve key ref: %w", err)
	}

	key, err := parseRSAPrivateKey([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return key, nil
}

// parseRSAPrivateKey decodes a PEM block and parses it as an RSA private key.
// Supports PKCS#1 ("RSA PRIVATE KEY") and PKCS#8 ("PRIVATE KEY") encodings.
// The raw key material in pemData is used only in-memory and never logged.
func parseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("parseRSAPrivateKey: no PEM block found")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("parseRSAPrivateKey: PKCS#8 key is %T, not *rsa.PrivateKey", k)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("parseRSAPrivateKey: unsupported PEM block type %q", block.Type)
	}
}

// parseCredentialInput reads git credential helper key=value input from r,
// stopping at the first blank line or EOF. Returns a map of the parsed pairs.
func parseCredentialInput(r io.Reader) map[string]string {
	attrs := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			attrs[line[:idx]] = line[idx+1:]
		}
	}
	return attrs
}

// apiBase returns the GitHub API base URL, overridable via SIGIL_GITHUB_API_URL
// for tests and staging environments.
func apiBase() string {
	if u := os.Getenv("SIGIL_GITHUB_API_URL"); u != "" {
		return u
	}
	return "https://api.github.com"
}

// defaultCacheDir returns the default cache directory path: $HOME/.cache/sigil.
// Uses $HOME explicitly (not os.UserCacheDir) so the path is predictable in tests
// that set HOME to a temp dir without also setting XDG_CACHE_HOME.
func defaultCacheDir() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "sigil")
}

// ensureCacheDir creates dir with mode 0700 if it does not exist, and
// re-enforces 0700 on an existing directory (in case it was created with
// looser permissions by another process or a stale install).
func ensureCacheDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cache dir %q: %w", dir, err)
	}
	// Chmod corrects a pre-existing dir whose permissions may be looser than 0700.
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("chmod cache dir %q: %w", dir, err)
	}
	return nil
}

// validateAPIBase checks that the API base URL uses https (or http:// on loopback
// for tests and local staging). Loopback hosts: 127.0.0.1, localhost, ::1.
// Returns an error mentioning "https" or "scheme" for any other scheme, so that
// the caller fails closed before reading any credential.
func validateAPIBase(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("API base URL: parse %q: %w", rawURL, err)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "127.0.0.1" || host == "localhost" || host == "::1" {
			return nil
		}
	}
	return fmt.Errorf("API base URL %q: https scheme required (http:// loopback is allowed for testing)", rawURL)
}

// writeCredentialResponse writes the git credential helper response lines to w.
// Accepts exactly one io.Writer — the only sanctioned output channel — so there
// is no code path by which the token can reach os.Stderr or any other stream.
func writeCredentialResponse(w io.Writer, token string) {
	fmt.Fprintf(w, "username=x-access-token\npassword=%s\n", token)
}

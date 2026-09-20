package main

// main.go — Sigil CLI dispatcher: git credential helper + utility subcommands.
//
// Subcommands:
//   sigil (bare) / sigil get               — git credential helper mode (reads stdin key=value)
//   sigil store                             — no-op (git protocol requirement), exit 0
//   sigil erase                             — no-op (git protocol requirement), exit 0
//   sigil token <owner>                     — mint and print a full-permission installation
//                                              token to stdout (cached; unchanged since #1880).
//                                              Refuses to run when stdout is not a terminal
//                                              (#3667) — see requireTerminalStdout
//   sigil token --rate-limit-only <owner>/<repo>
//                                            — mint and print a token narrowed to
//                                              metadata:read on exactly one repo; verified
//                                              against GitHub's actual grant before being
//                                              trusted or printed; never cached (#3647)
//   sigil doctor                            — verify config + key + one mint; print safe summary
//
// Security invariants:
//   - Private key: never written to disk; read in-process from 1Password.
//   - Installation token: emitted only via stdout (password= line or sigil token).
//   - SA token: read from env only; never logged or persisted.
//   - All error paths exit non-zero with a message to stderr; no key/token material
//     appears in any error message.
//   - Rate-limit-only token (--rate-limit-only): the access_tokens request body is fixed
//     in code — {"metadata":"read"} on exactly one repo, never built from unvalidated
//     input. GitHub's actual grant (permissions, repository_selection, repositories) is
//     verified against that exact request before the token is trusted or printed; any
//     mismatch revokes the token and returns an error instead. Never cached: it is a
//     one-shot credential, not a long-lived one. ⇒ Breaking the verification lets a
//     silently-widened grant (e.g. an empty permissions object granting everything,
//     write included — measured #3647) reach stdout unnoticed.
//   - Full-permission token (sigil token <owner>): refused when stdout is not a
//     terminal, before the key read and before any network call (#3667). This is a
//     speed bump and not a boundary — a pty defeats it, and `sigil get` plus the
//     on-disk cache sit outside it entirely — so it narrows the ordinary
//     unattended case and does not replace the agent permission rule. The honest
//     limits are enumerated at requireTerminalStdout; do not restate them more
//     strongly than they are written there.

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
		return dispatchToken(args[1:])
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

	// #3667: this form mints a FULL-PERMISSION token, so it runs interactively
	// only. The check sits in the credential-free preflight band — after the
	// scheme check, before readKey — so a refused run reads no secret and makes
	// no network call. Ordered after the scheme check deliberately: a
	// misconfigured API base is an operator-facing defect that should surface
	// whether or not stdout is a terminal.
	//
	// Only this path is gated. cmdGet is driven by git over pipes and is non-tty
	// on every invocation; cmdTokenNarrowed is the sanctioned unattended form.
	if err := requireTerminalStdout(os.Stdout); err != nil {
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

// rateLimitOnlyFlag selects the narrowed form of `sigil token`. Checked by exact
// string equality only — see dispatchToken for why a substring or prefix match
// is not used.
const rateLimitOnlyFlag = "--rate-limit-only"

// dispatchToken enumerates the exactly two permitted shapes of `sigil token` and
// rejects everything else — before any config load, key read, or network call.
// args is everything after the "token" subcommand word itself.
//
//	token <owner-or-path>                    (1 arg)  — unchanged, delegates to cmdToken
//	token --rate-limit-only <owner>/<repo>   (2 args)  — delegates to cmdTokenNarrowed
//
// ⛔ This enumerates the permitted forms rather than scanning args for the flag
// and taking the last element: that shape would let a typo'd or misplaced flag
// (e.g. --rate-limt-only, or the flag placed after the positional) silently fall
// through to a full-permission mint — the same class of silent widening
// verifyNarrowedGrant guards against on GitHub's side (#3647: an empty
// permissions object in the request was granted every installation
// permission, write included), moved into this parser instead.
func dispatchToken(args []string) error {
	const usage = "usage: sigil token <owner> | sigil token --rate-limit-only <owner>/<repo>"

	switch len(args) {
	case 1:
		if args[0] == rateLimitOnlyFlag {
			return fmt.Errorf("%s (missing <owner>/<repo>)", usage)
		}
		// The flag takes a separate positional, never --rate-limit-only=value —
		// reject the equals-form explicitly rather than letting it fall through
		// to cmdToken as a literal (and harmless-but-wrong) owner name.
		if strings.HasPrefix(args[0], rateLimitOnlyFlag+"=") {
			return fmt.Errorf("%s (--rate-limit-only takes a separate <owner>/<repo> argument, not %s=value)", usage, rateLimitOnlyFlag)
		}
		return cmdToken(args[0])
	case 2:
		if args[0] != rateLimitOnlyFlag {
			return errors.New(usage)
		}
		owner, repo, err := parseOwnerRepo(args[1])
		if err != nil {
			return fmt.Errorf("%s: %w", usage, err)
		}
		return cmdTokenNarrowed(owner, repo)
	default:
		return errors.New(usage)
	}
}

// parseOwnerRepo splits "<owner>/<repo>" strictly: exactly one slash, both sides
// non-empty. Anything else (a/b/c, /repo, owner/, no slash at all) is rejected
// here, before the value reaches config load or any network call.
func parseOwnerRepo(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected exactly one %q in <owner>/<repo>, got %q", "/", s)
	}
	owner, repo = parts[0], parts[1]
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("<owner>/<repo> must have non-empty owner and repo, got %q", s)
	}
	return owner, repo, nil
}

// cmdTokenNarrowed mints and prints a rate-limit-only installation token for
// owner/repo to stdout. The token is verified against GitHub's actual grant
// before being trusted or printed (see getRateLimitOnlyToken); never cached.
func cmdTokenNarrowed(owner, repo string) error {
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

	// M1: bounded context prevents an unbounded hang on an unresponsive API.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := getRateLimitOnlyToken(ctx, cfg, key, owner, repo, apiBase())
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

// errStdoutNotTerminal is the refusal text for the full-permission token form.
// It is a constant with no interpolation, and requireTerminalStdout runs before
// the key read and before any network call — so there is no token, key or
// response body in the process to quote even by accident (#3667 AC#3).
const errStdoutNotTerminal = "stdout is not a terminal: `sigil token <owner>` mints a full-permission installation token and runs interactively only (unattended use: `sigil token --rate-limit-only <owner>/<repo>`)"

// requireTerminalStdout permits f only when it is a character device, and
// refuses everything else — pipes, regular files, sockets. Called from cmdToken
// alone (#3667). No CONFIGURED bypass reaches this decision: no flag, no
// environment variable, nothing in config. That is a claim about this function
// and nothing wider — the control has the bypasses enumerated below, and none
// of them go through it.
//
// ⛔ Do NOT call this from cmdGet or from anything cmdGet calls. Git drives the
// credential helper over pipes, so that path is non-tty on every single
// invocation; gating it breaks every git operation on every wired repository,
// and breaks it silently from git's point of view.
//
// ⚠ Honest limit — this is a SPEED BUMP, NOT A BOUNDARY, and it does not remove
// the need for the agent permission rule that keeps this command out of
// unattended runs. Four ways a full-permission token still reaches a
// non-interactive caller with this gate in place, deliberately NOT ranked —
// the first one dominates the rest and no "cheapest gap" claim survives it:
//
//   - ⛔ `sigil get` is ITSELF an on-demand, non-tty, full-permission token
//     printer, and is supposed to be. Fed `protocol=https`, `host=github.com`
//     and `path=<owner>/<anything>` on stdin, it writes `password=<token>` to a
//     pipe (cmdGet → writeCredentialResponse) — no pty, no prior git operation,
//     no cache read, nothing this gate is anywhere near. resolveInstallation
//     reads only the first path segment, so the repo half is arbitrary. #3667
//     AC#5 mandates leaving it alone and it is the entire purpose of the binary,
//     so this is neither a defect nor fixable here; #3673 is the filed follow-up
//     to narrow the grant the helper requests. It is named because a reader who
//     sees only the gate will otherwise take the control for more than it is.
//   - ⛔ A full-permission token is already ON DISK. cmdGet calls
//     getInstallationToken with the same cacheDir, so every ordinary git
//     operation through the helper writes ~/.cache/sigil/<installID>.json —
//     0600, owner-readable, live for up to ~55 minutes (cache.go; expiry is
//     GitHub's minus five). Reading it needs no pty and no subcommand. This gate
//     stops the full form from MINTING AND PRINTING a token on demand; it does
//     not stop anything running as this user from reading one git just minted.
//   - A pty walks straight through it, and one need not even be allocated.
//     Allocating: `script -q /dev/null sigil token <owner> | cat` reports a
//     terminal AND the output is still captured — the pty survives being piped
//     onward. Not allocating: this user already owns live pty slaves attached to
//     running shells (12 of them on 2026-09-20), so
//     `sigil token <owner> > /dev/ttysNNN` is a bare redirect into one that
//     already exists, and when that pane belongs to tmux the bytes come straight
//     back out with `tmux capture-pane -p`. Both measured 2026-09-20 on macOS,
//     the second end-to-end against a live pane. ⚠ NNN is whatever tmux
//     allocated that run (ttys012 on the day); it is not a fixed path, so do not
//     read the device number as the property — "a pty this user already owns" is.
//   - The character-device test is coarser than isatty(3): `/dev/null` is a
//     character device, so `sigil token <owner> > /dev/null` is permitted where
//     isatty(3) would refuse. ⚠ That run discards the PRINTED BYTES but NOT the
//     token — the gate sits before the mint, and getInstallationToken calls
//     cachePut (token.go) before cmdToken ever reaches fmt.Println, so the token
//     lands in the cache file described two bullets up and is readable from
//     there. `sigil token <owner> > /dev/null; cat ~/.cache/sigil/<id>.json` is
//     a working bypass needing no pty at all. Stdlib-only is why the check is
//     coarse: an exact isatty needs an ioctl and a dependency this repo does not
//     carry.
//
// What it does buy, and the whole reason it is here: the ordinary unattended
// invocation — a pipe, a captured file, an agent shell — fails loudly and
// immediately, before any secret is read. A loud failure in the accidental case,
// not prevention of a determined one.
//
// Fails closed: an f whose mode cannot be read is refused, not permitted.
func requireTerminalStdout(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		// Cannot evaluate ⇒ deny. The Stat error is an fs.PathError (a path and
		// an errno); nothing secret exists in the process at this point.
		return fmt.Errorf("%s [stdout mode unreadable: %v]", errStdoutNotTerminal, err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return errors.New(errStdoutNotTerminal)
	}
	return nil
}

// writeCredentialResponse writes the git credential helper response lines to w.
// Accepts exactly one io.Writer — the only sanctioned output channel — so there
// is no code path by which the token can reach os.Stderr or any other stream.
func writeCredentialResponse(w io.Writer, token string) {
	fmt.Fprintf(w, "username=x-access-token\npassword=%s\n", token)
}

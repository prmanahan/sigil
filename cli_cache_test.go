package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These tests drive the built binary with a seeded cache and no
// OP_SERVICE_ACCOUNT_TOKEN, so none needs 1Password. They pin the order
// routing → cache lookup → (miss only) key read, and the terminal gate ahead of
// the cache. Config load and the scheme check are not exercised here.

const (
	cacheTestOwner = "myorg"
	getStdin       = "protocol=https\nhost=github.com\npath=myorg/repo\n\n"
)

// cacheTestEnv returns an environment whose HOME holds a valid config and
// nothing that could reach 1Password or redirect the API, plus the cache dir
// entries go in.
func cacheTestEnv(t *testing.T) (env []string, cacheDir string) {
	t.Helper()
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, cacheTestOwner))
	cacheDir = filepath.Join(home, ".cache", "sigil")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("MkdirAll cache dir: %v", err)
	}
	for _, e := range envWithout("OP_SERVICE_ACCOUNT_TOKEN") {
		if !strings.HasPrefix(e, "SIGIL_GITHUB_API_URL=") {
			env = append(env, e)
		}
	}
	return append(env, "HOME="+home), cacheDir
}

func seedCache(t *testing.T, cacheDir, token string, expiry time.Time) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"token": token, "expiry": expiry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "9999.json"), data, 0600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
}

// TestCLI_Get_CacheHit_NoKeyRead_TokenOnStdoutOnly: a live cache entry is served
// with no service-account token in the environment. It also checks channel
// separation through the real binary on this path: the token is on stdout, and
// stderr is empty.
func TestCLI_Get_CacheHit_NoKeyRead_TokenOnStdoutOnly(t *testing.T) {
	const token = "ghs_seeded_cache_hit_token"
	env, cacheDir := cacheTestEnv(t)
	seedCache(t, cacheDir, token, time.Now().Add(time.Hour))

	result := runSigilWithEnv(t, []string{"get"}, getStdin, env)

	if result.exitCode != 0 {
		t.Fatalf("exit code: got %d, want 0\nstderr: %s", result.exitCode, result.stderr)
	}
	if want := "username=x-access-token\npassword=" + token + "\n"; result.stdout != want {
		t.Errorf("stdout: got %q, want %q", result.stdout, want)
	}
	if result.stderr != "" {
		t.Errorf("stderr must be empty on a cache hit, got %q", result.stderr)
	}
}

// TestCLI_Get_UnknownOwner_FailsAtRouting_NotKeyRead: the error names the
// routing failure and never the key read, with a live entry for another owner
// present and no service-account token.
func TestCLI_Get_UnknownOwner_FailsAtRouting_NotKeyRead(t *testing.T) {
	env, cacheDir := cacheTestEnv(t)
	seedCache(t, cacheDir, "ghs_must_not_be_served", time.Now().Add(time.Hour))
	stdin := "protocol=https\nhost=github.com\npath=unknown-owner/repo\n\n"

	result := runSigilWithEnv(t, []string{"get"}, stdin, env)

	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit for an unknown owner, got 0\nstdout: %s", result.stdout)
	}
	if !strings.Contains(result.stderr, "unknown owner") {
		t.Errorf("stderr: got %q, want the routing error (unknown owner)", result.stderr)
	}
	if strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr shows the key read ran before routing: %s", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty", result.stdout)
	}
}

// TestCLI_Get_ExpiredOrMarginEntry_GoesToKeyRead: the cache stores GitHub's
// expires_at minus five minutes, so an entry whose stored expiry is in the past
// is either expired or was inside the five-minute margin when written. Neither
// may be served; both must fall through to the key read.
func TestCLI_Get_ExpiredOrMarginEntry_GoesToKeyRead(t *testing.T) {
	cases := map[string]time.Time{
		"expired": time.Now().Add(-time.Hour),
		// GitHub's expiry was 3 minutes out when this was stored (3 - 5 = -2).
		"inside five-minute margin": time.Now().Add(-2 * time.Minute),
	}
	for name, expiry := range cases {
		t.Run(name, func(t *testing.T) {
			env, cacheDir := cacheTestEnv(t)
			seedCache(t, cacheDir, "ghs_stale_must_not_be_served", expiry)

			result := runSigilWithEnv(t, []string{"get"}, getStdin, env)

			if result.exitCode == 0 {
				t.Fatalf("expected non-zero exit (key read unavailable), got 0\nstdout: %s", result.stdout)
			}
			if !strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
				t.Errorf("stderr: got %q, want the key-read failure", result.stderr)
			}
			if strings.Contains(result.stdout, "ghs_stale") || strings.Contains(result.stderr, "ghs_stale") {
				t.Errorf("a stale token was emitted\nstdout: %s\nstderr: %s", result.stdout, result.stderr)
			}
		})
	}
}

// TestCLI_Token_CacheHit_NoKeyRead covers the full-permission `token <owner>`
// path under a pty (it refuses a non-terminal stdout): a live entry is printed
// with no key read. Same `script` mechanics as
// TestCLI_TokenFullForm_TerminalStdout_ReachesKeyRead.
func TestCLI_Token_CacheHit_NoKeyRead(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("pty control uses BSD `script` argument form; GOOS=%s", runtime.GOOS)
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skipf("`script` not available (%v) — cannot allocate a pty", err)
	}
	const token = "ghs_seeded_token_cmd_token"
	env, cacheDir := cacheTestEnv(t)
	seedCache(t, cacheDir, token, time.Now().Add(time.Hour))

	cmd := exec.Command("script", "-q", "/dev/null", binaryPath, "token", cacheTestOwner)
	cmd.Env = env
	out, _ := cmd.CombinedOutput()

	if !strings.Contains(string(out), token) {
		t.Errorf("output: got %q, want the cached token", string(out))
	}
	if strings.Contains(string(out), "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("output shows a key read on a cache hit: %q", string(out))
	}
}

// TestCLI_Get_EmptyCachedToken_NotServed: a live entry with an empty token must
// not be emitted as `password=` with exit 0; it is a miss and goes to the key read.
func TestCLI_Get_EmptyCachedToken_NotServed(t *testing.T) {
	env, cacheDir := cacheTestEnv(t)
	seedCache(t, cacheDir, "", time.Now().Add(time.Hour))

	result := runSigilWithEnv(t, []string{"get"}, getStdin, env)

	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit, got 0\nstdout: %q", result.stdout)
	}
	if strings.Contains(result.stdout, "password=") {
		t.Errorf("an empty cached token was emitted: %q", result.stdout)
	}
	if !strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr: got %q, want the key-read failure (the entry is a miss)", result.stderr)
	}
}

// TestCLI_Token_NonTerminal_LiveCache_StillRefuses: the terminal gate must hold
// when a live cache entry exists. If the gate ran only on the mint path (after
// the cache lookup), a pipe would be handed the cached full-permission token.
func TestCLI_Token_NonTerminal_LiveCache_StillRefuses(t *testing.T) {
	const token = "ghs_live_entry_must_not_be_printed"
	env, cacheDir := cacheTestEnv(t)
	seedCache(t, cacheDir, token, time.Now().Add(time.Hour))

	result := runSigilWithEnv(t, []string{"token", cacheTestOwner}, "", env)

	if result.exitCode == 0 {
		t.Errorf("exit code: got 0, want non-zero under a pipe")
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty — a refused run prints no token", result.stdout)
	}
	if !strings.Contains(result.stderr, stdoutNotATerminalMarker) {
		t.Errorf("stderr: got %q, want the terminal refusal (%q)", result.stderr, stdoutNotATerminalMarker)
	}
	if strings.Contains(result.stderr, token) {
		t.Errorf("token reached stderr: %q", result.stderr)
	}
}

// TestCLI_TokenRateLimitOnly_UnknownOwner_FailsAtRouting_NotKeyRead: the narrowed
// form routes before it reads the key, like the other two paths.
func TestCLI_TokenRateLimitOnly_UnknownOwner_FailsAtRouting_NotKeyRead(t *testing.T) {
	env, _ := cacheTestEnv(t)

	result := runSigilWithEnv(t, []string{"token", "--rate-limit-only", "unknown-owner/repo"}, "", env)

	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit for an unknown owner, got 0")
	}
	if !strings.Contains(result.stderr, "unknown owner") {
		t.Errorf("stderr: got %q, want the routing error (unknown owner)", result.stderr)
	}
	if strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr shows the key read ran before routing: %s", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty", result.stdout)
	}
}

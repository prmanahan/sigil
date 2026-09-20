package main_test

// AC#5 + AC#7: Black-box CLI tests for Sigil's credential-helper protocol and leak scan.
//
// These tests build the sigil binary and exec it, testing only its externally
// observable behaviour (stdin/stdout/stderr/exit-code). No implementation details
// are assumed. The SIGIL_GITHUB_API_URL env var is the sole testable seam for
// redirecting GitHub API calls.
//
// Red state: tests compile fine but fail at runtime because the current binary
// is the Layer-B 1Password smoke test, not the real credential helper. That is
// the expected red.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// binaryPath is set by TestMain after building the binary.
var binaryPath string

// TestMain builds the sigil binary once for all CLI tests.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "sigil-cli-test-*")
	if err != nil {
		panic("MkdirTemp: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "sigil")

	// go build runs in the current working directory, which go test sets to the
	// package directory (the worktree root).
	out, err := exec.Command("go", "build", "-o", binaryPath, ".").CombinedOutput()
	if err != nil {
		panic("build failed: " + string(out))
	}

	os.Exit(m.Run())
}

// runResult holds the output of a sigil invocation.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runSigil executes the sigil binary with the given args, stdin, and additional
// environment entries (appended to the current process env).
func runSigil(t *testing.T, args []string, stdin string, extraEnv []string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	cmd.Env = append(os.Environ(), extraEnv...)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("exec.Command(%v) failed unexpectedly: %v", args, err)
		}
	}
	return runResult{
		stdout:   outBuf.String(),
		stderr:   errBuf.String(),
		exitCode: code,
	}
}

// envWithout returns a copy of os.Environ() without entries whose key matches key.
func envWithout(key string) []string {
	prefix := key + "="
	env := os.Environ()
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// writeTempConfig creates a temporary $HOME directory with the sigil config at
// $HOME/.config/sigil/config.toml. Returns the HOME path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "sigil")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("MkdirAll config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	return home
}

// skipIfNoOPToken skips the test if OP_SERVICE_ACCOUNT_TOKEN is not set.
// Used for integration tests that require a real 1Password service account.
func skipIfNoOPToken(t *testing.T) {
	t.Helper()
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		t.Skip("OP_SERVICE_ACCOUNT_TOKEN not set — skipping 1Password integration test")
	}
}

// minimalConfig returns a config file body suitable for tests that need a valid
// config but whose code paths don't actually reach 1Password.
func minimalConfig(opKeyRef string, appID int, installID int, owner string) string {
	return fmt.Sprintf(`op_key_ref = %q
app_id = %d

[installations]
%q = %d
`, opKeyRef, appID, owner, installID)
}

// --- AC#5: Credential-helper protocol ---

// TestCLI_HelperGet_NonGitHubHost verifies that a credential request for a
// non-GitHub host produces no output and exits 0 (no-op, per the spec).
func TestCLI_HelperGet_NonGitHubHost(t *testing.T) {
	// Given: stdin with a non-GitHub host
	stdin := "protocol=https\nhost=example.com\npath=owner/repo\n\n"
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "owner"))

	// When: sigil get is called
	result := runSigil(t, []string{"get"}, stdin, []string{
		"HOME=" + home,
	})

	// Then: no output, exit 0
	if result.exitCode != 0 {
		t.Errorf("exit code: got %d, want 0 for non-GitHub host", result.exitCode)
	}
	if result.stdout != "" {
		t.Errorf("stdout: expected empty for non-GitHub host, got %q", result.stdout)
	}
}

// TestCLI_HelperStore_NoOp verifies that the `store` action is a no-op: no output,
// exit 0, as required by the git credential-helper protocol.
func TestCLI_HelperStore_NoOp(t *testing.T) {
	// Given: stdin with a store request
	stdin := "protocol=https\nhost=github.com\nusername=x-access-token\npassword=ghs_fake\n\n"
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "owner"))

	// When: sigil store is called
	result := runSigil(t, []string{"store"}, stdin, []string{"HOME=" + home})

	// Then: no output, exit 0
	if result.exitCode != 0 {
		t.Errorf("exit code: got %d, want 0 for store no-op", result.exitCode)
	}
	if result.stdout != "" {
		t.Errorf("stdout: expected empty for store, got %q", result.stdout)
	}
}

// TestCLI_HelperErase_NoOp verifies that the `erase` action is a no-op: no output,
// exit 0, as required by the git credential-helper protocol.
func TestCLI_HelperErase_NoOp(t *testing.T) {
	// Given: stdin with an erase request
	stdin := "protocol=https\nhost=github.com\npath=owner/repo\n\n"
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "owner"))

	// When: sigil erase is called
	result := runSigil(t, []string{"erase"}, stdin, []string{"HOME=" + home})

	// Then: no output, exit 0
	if result.exitCode != 0 {
		t.Errorf("exit code: got %d, want 0 for erase no-op", result.exitCode)
	}
	if result.stdout != "" {
		t.Errorf("stdout: expected empty for erase, got %q", result.stdout)
	}
}

// TestCLI_HelperGet_ValidGitHubPath verifies the full AC#5 happy path:
// stdin with a GitHub HTTPS path → stdout containing username and password lines.
// Requires a real 1Password service account token (OP_SERVICE_ACCOUNT_TOKEN).
func TestCLI_HelperGet_ValidGitHubPath(t *testing.T) {
	skipIfNoOPToken(t)

	// Given: a mock GitHub API server and a valid config
	const installID = 12345
	const fakeToken = "ghs_mock_installation_token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"token":      fakeToken,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer srv.Close()

	// Use real op_key_ref from the environment (as in actual usage)
	opKeyRef := os.Getenv("SIGIL_TEST_OP_KEY_REF")
	if opKeyRef == "" {
		t.Skip("SIGIL_TEST_OP_KEY_REF not set — skipping full integration test")
	}
	home := writeTempConfig(t, minimalConfig(opKeyRef, 42, installID, "test-owner"))

	stdin := "protocol=https\nhost=github.com\npath=test-owner/repo\n\n"

	// When: sigil get is called with the mock API URL
	result := runSigil(t, []string{"get"}, stdin, []string{
		"HOME=" + home,
		"SIGIL_GITHUB_API_URL=" + srv.URL,
	})

	// Then: exit 0
	if result.exitCode != 0 {
		t.Errorf("exit code: got %d, want 0\nstderr: %s", result.exitCode, result.stderr)
	}

	// And: stdout contains username=x-access-token
	if !strings.Contains(result.stdout, "username=x-access-token") {
		t.Errorf("stdout missing 'username=x-access-token':\n%s", result.stdout)
	}

	// And: stdout contains password=<some token>
	if !strings.Contains(result.stdout, "password=") {
		t.Errorf("stdout missing 'password=' line:\n%s", result.stdout)
	}
}

// TestCLI_BareInvocation_ActsAsGet verifies that calling sigil with no subcommand
// (which is how git invokes credential helpers) behaves identically to `sigil get`.
func TestCLI_BareInvocation_NonGitHubHostIsNoOp(t *testing.T) {
	// Given: stdin with a non-GitHub host and no subcommand
	stdin := "protocol=ssh\nhost=bitbucket.org\npath=owner/repo\n\n"
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "owner"))

	// When: sigil is called with no args (bare invocation)
	result := runSigil(t, []string{}, stdin, []string{"HOME=" + home})

	// Then: no output, exit 0 (non-GitHub, no-op)
	if result.exitCode != 0 {
		t.Errorf("exit code: got %d, want 0 for bare invocation / non-GitHub host", result.exitCode)
	}
	if result.stdout != "" {
		t.Errorf("stdout: expected empty, got %q", result.stdout)
	}
}

// --- Error / exit-code paths ---

// TestCLI_MissingConfig_ExitsNonZero verifies that a missing config file causes
// sigil to exit non-zero rather than silently emitting no credentials.
func TestCLI_MissingConfig_ExitsNonZero(t *testing.T) {
	// Given: a HOME that has no sigil config
	emptyHome := t.TempDir()
	stdin := "protocol=https\nhost=github.com\npath=owner/repo\n\n"

	// When: sigil get is called
	result := runSigil(t, []string{"get"}, stdin, []string{
		"HOME=" + emptyHome,
	})

	// Then: exits non-zero (fail closed)
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit when config is missing, got 0\nstdout: %s", result.stdout)
	}
}

// TestCLI_UnknownOwner_ExitsNonZero verifies that a request for a repo whose owner
// is not in the installations map causes sigil to fail closed with non-zero exit.
func TestCLI_UnknownOwner_ExitsNonZero(t *testing.T) {
	// Given: a config that only knows about "known-owner"
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "known-owner"))
	stdin := "protocol=https\nhost=github.com\npath=unknown-owner/repo\n\n"

	// When: sigil get is called for an unknown owner
	result := runSigil(t, []string{"get"}, stdin, []string{
		"HOME=" + home,
	})

	// Then: exits non-zero (fail closed — unknown owner is an error)
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit for unknown owner (fail-closed), got 0\nstdout: %s", result.stdout)
	}
}

// TestCLI_MissingOPServiceAccountToken_ExitsNonZero verifies that if
// OP_SERVICE_ACCOUNT_TOKEN is not set, sigil fails closed with non-zero exit
// for a GitHub credential request (requires the token to mint a JWT).
func TestCLI_MissingOPServiceAccountToken_ExitsNonZero(t *testing.T) {
	// Given: a valid config but no OP_SERVICE_ACCOUNT_TOKEN in the environment
	home := writeTempConfig(t, minimalConfig("op://vault/item/key.pem", 42, 12345, "myorg"))
	stdin := "protocol=https\nhost=github.com\npath=myorg/repo\n\n"

	// Build an environment without OP_SERVICE_ACCOUNT_TOKEN
	env := envWithout("OP_SERVICE_ACCOUNT_TOKEN")
	env = append(env, "HOME="+home)

	// When: sigil get is called with env that has no OP_SERVICE_ACCOUNT_TOKEN
	// (exec.Command.Env replaces the full env, unlike runSigil which appends)
	cmd := exec.Command(binaryPath, "get")
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = env
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	result := runResult{stdout: outBuf.String(), stderr: errBuf.String(), exitCode: exitCode}

	// Then: exits non-zero (fail closed)
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit with OP_SERVICE_ACCOUNT_TOKEN unset, got 0\nstdout: %s", result.stdout)
	}
}

// --- sigil token subcommand ---

// TestCLI_TokenSubcommand_MissingConfig_ExitsNonZero verifies that `sigil token`
// fails closed when no config is present.
func TestCLI_TokenSubcommand_MissingConfig_ExitsNonZero(t *testing.T) {
	// Given: an empty HOME with no config
	emptyHome := t.TempDir()

	// When: sigil token <owner> is called
	result := runSigil(t, []string{"token", "myorg"}, "", []string{"HOME=" + emptyHome})

	// Then: exits non-zero
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit for missing config in 'sigil token', got 0")
	}
}

// --- sigil token --rate-limit-only argument polarity (#3647) ---
//
// These reject every disallowed shape BEFORE config load: HOME points at an
// empty temp dir with no config file, so if dispatch fell through to
// cmdToken/cmdTokenNarrowed, the error would be a "open config ...: no such
// file" message (loadConfig's wording), which is distinguishable from the
// "usage: ..." argument error. Asserting stderr does NOT contain "open config"
// is the observable proof that rejection happened in argument parsing, not
// downstream of it.

// TestCLI_TokenRateLimitOnly_RejectedShapes_NoConfigLoad table-drives every
// disallowed `sigil token` shape named in #3647: an unknown flag, a typo'd
// flag, the flag placed after the positional, extra positionals (both the
// plain and flag forms), a missing repo, and a malformed <owner>/<repo>
// (two slashes, empty owner, empty repo).
func TestCLI_TokenRateLimitOnly_RejectedShapes_NoConfigLoad(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"token", "--foo", "owner/repo"}},
		{name: "typo'd flag", args: []string{"token", "--rate-limt-only", "owner/repo"}},
		{name: "equals-form flag", args: []string{"token", "--rate-limit-only=owner/repo"}},
		{name: "flag after positional", args: []string{"token", "owner/repo", "--rate-limit-only"}},
		{name: "extra positional, plain form", args: []string{"token", "myorg", "extra"}},
		{name: "extra positional, flag form", args: []string{"token", "--rate-limit-only", "owner/repo", "extra"}},
		{name: "missing repo", args: []string{"token", "--rate-limit-only"}},
		{name: "two slashes", args: []string{"token", "--rate-limit-only", "a/b/c"}},
		{name: "empty owner", args: []string{"token", "--rate-limit-only", "/x"}},
		{name: "empty repo", args: []string{"token", "--rate-limit-only", "x/"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: an empty HOME — no config file at all
			emptyHome := t.TempDir()
			env := envWithout("OP_SERVICE_ACCOUNT_TOKEN")
			env = append(env, "HOME="+emptyHome)

			// When: sigil is invoked with the rejected argument shape
			cmd := exec.Command(binaryPath, tc.args...)
			cmd.Env = env
			var outBuf, errBuf strings.Builder
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf
			runErr := cmd.Run()
			exitCode := 0
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}

			// Then: non-zero exit, empty stdout
			if exitCode == 0 {
				t.Errorf("exit code: got 0, want non-zero for args %v", tc.args)
			}
			if outBuf.String() != "" {
				t.Errorf("stdout: got %q, want empty for args %v", outBuf.String(), tc.args)
			}
			// And: the error is the argument-polarity rejection, not a config-load
			// failure — proving rejection happened before config load / key read /
			// any network call.
			if strings.Contains(errBuf.String(), "open config") {
				t.Errorf("stderr shows config load was attempted for a rejected shape %v: %s", tc.args, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), "usage") {
				t.Errorf("stderr: got %q, want it to mention \"usage\" for args %v", errBuf.String(), tc.args)
			}
		})
	}
}

// TestCLI_TokenRateLimitOnly_AcceptedShape_ReachesConfigLoad is the positive
// control for the table above: a well-formed --rate-limit-only invocation
// must NOT be rejected by argument parsing, and must reach (and fail at)
// config load — proving the rejected-shapes test isn't green because
// argument parsing rejects everything indiscriminately.
func TestCLI_TokenRateLimitOnly_AcceptedShape_ReachesConfigLoad(t *testing.T) {
	emptyHome := t.TempDir()
	env := envWithout("OP_SERVICE_ACCOUNT_TOKEN")
	env = append(env, "HOME="+emptyHome)

	cmd := exec.Command(binaryPath, "token", "--rate-limit-only", "myorg/myrepo")
	cmd.Env = env
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	if exitCode == 0 {
		t.Errorf("exit code: got 0, want non-zero (no config present)")
	}
	if outBuf.String() != "" {
		t.Errorf("stdout: got %q, want empty", outBuf.String())
	}
	if !strings.Contains(errBuf.String(), "open config") {
		t.Errorf("stderr: got %q, want it to show a config-load failure (proves the well-formed shape was accepted by argument parsing)", errBuf.String())
	}
}

// --- #3667: `sigil token <owner>` refuses a non-terminal stdout ---
//
// These tests pin WHICH path is gated, which is the whole risk of the change.
// Every one of them runs with stdout captured through a pipe (what runSigil and
// exec.Command do), i.e. exactly the non-tty condition the gate fires on.
//
// The ordering proof is the identity of the error, the same technique the
// --rate-limit-only tests above use with "open config": each test below runs
// with a VALID config and with OP_SERVICE_ACCOUNT_TOKEN stripped, so
//
//	stderr says "OP_SERVICE_ACCOUNT_TOKEN"  ⇒ the run reached readKey — NOT gated
//	stderr says "stdout is not a terminal"  ⇒ the run was refused before readKey
//
// and the two are mutually exclusive. No network assertion is needed for AC#2:
// the access_tokens POST is strictly downstream of readKey in cmdToken's
// straight-line code, and readKey demonstrably did not run.

// stdoutNotATerminalMarker is the distinctive fragment of the refusal message.
// Hardcoded rather than imported: cli_test.go is package main_test and a Go main
// package cannot be imported, so every expectation here is black-box by
// construction.
const stdoutNotATerminalMarker = "stdout is not a terminal"

// runSigilWithEnv executes the sigil binary with an EXACT environment rather
// than one appended to the test process's own, so a test can strip
// OP_SERVICE_ACCOUNT_TOKEN and behave the same on a developer machine where it
// is set as on one where it is not.
func runSigilWithEnv(t *testing.T, args []string, stdin string, env []string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("exec.Command(%v) failed unexpectedly: %v", args, err)
		}
	}
	return runResult{stdout: outBuf.String(), stderr: errBuf.String(), exitCode: code}
}

// envForGateTest returns an environment with a valid config at HOME and
// OP_SERVICE_ACCOUNT_TOKEN removed.
func envForGateTest(t *testing.T) []string {
	t.Helper()
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "myorg"))
	env := envWithout("OP_SERVICE_ACCOUNT_TOKEN")
	return append(env, "HOME="+home)
}

// TestCLI_TokenFullForm_NonTerminalStdout_Refuses covers AC#1, AC#2 and AC#3:
// the full-permission form with stdout on a pipe exits non-zero, prints nothing
// to stdout, names the reason, and is refused BEFORE the 1Password key read
// (hence before any network call).
func TestCLI_TokenFullForm_NonTerminalStdout_Refuses(t *testing.T) {
	result := runSigilWithEnv(t, []string{"token", "myorg"}, "", envForGateTest(t))

	if result.exitCode == 0 {
		t.Errorf("exit code: got 0, want non-zero for `token myorg` with stdout on a pipe")
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty — a refused run prints no token", result.stdout)
	}
	if !strings.Contains(result.stderr, stdoutNotATerminalMarker) {
		t.Errorf("stderr: got %q, want it to name the reason (%q)", result.stderr, stdoutNotATerminalMarker)
	}
	// Ordering: if the gate ran after readKey, this is the error we would see
	// instead, because the environment has no service-account token.
	if strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr shows the key read was attempted on a refused run: %s", result.stderr)
	}
}

// TestCLI_TokenFullForm_TerminalStdout_ReachesKeyRead is the CLI-level positive
// control: under an allocated pty the SAME binary proceeds past the gate to the
// key read. Without it, a gate that refused unconditionally — or a binary that
// failed early for an unrelated reason — would satisfy every refusal assertion
// above.
//
// `script -q /dev/null <cmd>` is how the pty is allocated; it was measured on
// 2026-09-20 to report a terminal, and to keep reporting one when its own output
// is piped onward. That second measurement is also why the gate is documented as
// a speed bump: this test is simultaneously the positive control and the
// demonstrated bypass.
//
// macOS-only: BSD and GNU `script` take their command argument differently, and
// this repo is built and run on macOS.
func TestCLI_TokenFullForm_TerminalStdout_ReachesKeyRead(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("pty control uses BSD `script` argument form; GOOS=%s", runtime.GOOS)
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skipf("`script` not available (%v) — cannot allocate a pty", err)
	}

	cmd := exec.Command("script", "-q", "/dev/null", binaryPath, "token", "myorg")
	cmd.Env = envForGateTest(t)
	// script merges the child's stdout and stderr onto the pty, so both arrive
	// here as one stream.
	out, _ := cmd.CombinedOutput()

	if strings.Contains(string(out), stdoutNotATerminalMarker) {
		t.Errorf("output: got %q, want no refusal — a pty must be permitted", string(out))
	}
	if !strings.Contains(string(out), "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("output: got %q, want the key-read failure, which is the proof the run got past the gate", string(out))
	}
}

// TestCLI_TokenRateLimitOnly_NonTerminalStdout_Unaffected covers AC#4: the
// narrowed form is the sanctioned unattended path and must be completely
// unaffected by stdout being a pipe. It reaches the key read, so it was not
// gated.
//
// This is a distinct test from TestCLI_TokenRateLimitOnly_AcceptedShape_ReachesConfigLoad
// above, which uses an EMPTY home and so would stop at config load before a
// leaked gate could fire — it cannot catch this regression.
func TestCLI_TokenRateLimitOnly_NonTerminalStdout_Unaffected(t *testing.T) {
	result := runSigilWithEnv(t, []string{"token", "--rate-limit-only", "myorg/myrepo"}, "", envForGateTest(t))

	if strings.Contains(result.stderr, stdoutNotATerminalMarker) {
		t.Errorf("stderr: got %q, want no refusal — --rate-limit-only must not be gated", result.stderr)
	}
	if !strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr: got %q, want the key-read failure, which proves the run got past every preflight check", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty", result.stdout)
	}
}

// TestCLI_HelperGet_NonTerminalStdout_Unaffected covers AC#5, the catastrophic
// failure mode: git drives `sigil get` over pipes on every invocation, so a gate
// reaching that handler — directly or through a shared helper — breaks every git
// operation on every wired repository, silently from git's point of view.
//
// A GitHub HTTPS request with stdout on a pipe must run the ordinary path all
// the way to the key read.
func TestCLI_HelperGet_NonTerminalStdout_Unaffected(t *testing.T) {
	stdin := "protocol=https\nhost=github.com\npath=myorg/repo\n\n"

	result := runSigilWithEnv(t, []string{"get"}, stdin, envForGateTest(t))

	if strings.Contains(result.stderr, stdoutNotATerminalMarker) {
		t.Errorf("stderr: got %q — the credential-helper path MUST NOT be gated on a terminal", result.stderr)
	}
	if !strings.Contains(result.stderr, "OP_SERVICE_ACCOUNT_TOKEN") {
		t.Errorf("stderr: got %q, want the key-read failure, which proves `get` ran its ordinary path under a pipe", result.stderr)
	}
	if result.stdout != "" {
		t.Errorf("stdout: got %q, want empty (no token available in this environment)", result.stdout)
	}
}

// --- sigil doctor subcommand ---

// TestCLI_DoctorSubcommand_MissingConfig_ExitsNonZero verifies that `sigil doctor`
// exits non-zero when no config is present.
func TestCLI_DoctorSubcommand_MissingConfig_ExitsNonZero(t *testing.T) {
	// Given: an empty HOME
	emptyHome := t.TempDir()

	// When: sigil doctor is called
	result := runSigil(t, []string{"doctor"}, "", []string{"HOME=" + emptyHome})

	// Then: exits non-zero (can't verify setup without a config)
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit for 'sigil doctor' with no config, got 0")
	}
}

// --- AC#7: Leak scan ---

// TestCLI_InsecureAPIBaseURL_ExitsNonZero verifies that SIGIL_GITHUB_API_URL with a
// non-https non-loopback scheme causes sigil to fail closed before reading any
// credential, with an error message that mentions the scheme or https requirement.
//
// Loopback addresses (http://127.0.0.1) MUST remain allowed — existing httptest
// tests rely on them. Only cleartext to a remote host is rejected.
//
// L1 status: RED. Current code validates no scheme: cmdToken calls readKey before
// any URL inspection, so the error is "OP_SERVICE_ACCOUNT_TOKEN is not set" (stderr
// does not mention https/scheme). Forge must validate the API base URL scheme
// BEFORE readKey so the check is credential-free.
//
// OP_SERVICE_ACCOUNT_TOKEN is stripped from the environment to make the test
// deterministic regardless of the CI/developer environment.
func TestCLI_InsecureAPIBaseURL_ExitsNonZero(t *testing.T) {
	// Given: a valid config with a known owner, and a cleartext remote API URL.
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "myorg"))

	// Strip OP_SERVICE_ACCOUNT_TOKEN so behaviour is independent of the environment
	// (scheme validation must fire before the key-read — testable without credentials).
	env := envWithout("OP_SERVICE_ACCOUNT_TOKEN")
	env = append(env,
		"HOME="+home,
		"SIGIL_GITHUB_API_URL=http://evil.example.com",
	)

	// When: sigil token is called with the insecure API URL.
	cmd := exec.Command(binaryPath, "token", "myorg")
	cmd.Env = env
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	result := runResult{stdout: outBuf.String(), stderr: errBuf.String(), exitCode: exitCode}

	// Then: non-zero exit (fail closed before any credential read).
	if result.exitCode == 0 {
		t.Errorf("expected non-zero exit for insecure API URL, got 0\nstdout: %s\nstderr: %s",
			result.stdout, result.stderr)
	}
	// And: the error message mentions the scheme or https requirement.
	if !strings.Contains(result.stderr, "https") && !strings.Contains(result.stderr, "scheme") {
		t.Errorf("stderr must mention 'https' or 'scheme' for insecure-URL error; got: %s", result.stderr)
	}
}

// TestCLI_LeakScan_NoPEMMarkersInOutput verifies that PEM private-key markers never
// appear in stdout or stderr across error-path invocations.
// These paths do not need 1Password credentials.
func TestCLI_LeakScan_NoPEMMarkersInOutput(t *testing.T) {
	home := writeTempConfig(t, minimalConfig("op://v/i/k.pem", 1, 9999, "owner"))
	pemMarkers := []string{
		"BEGIN RSA PRIVATE KEY",
		"END RSA PRIVATE KEY",
		"BEGIN PRIVATE KEY",
		"END PRIVATE KEY",
		"BEGIN EC PRIVATE KEY",
	}

	// Run several invocations that are expected to fail (error paths)
	invocations := []struct {
		name  string
		args  []string
		stdin string
		env   []string
	}{
		{
			name:  "missing config",
			args:  []string{"get"},
			stdin: "protocol=https\nhost=github.com\npath=owner/repo\n\n",
			env:   []string{"HOME=" + t.TempDir()},
		},
		{
			name:  "unknown owner",
			args:  []string{"get"},
			stdin: "protocol=https\nhost=github.com\npath=nobody/repo\n\n",
			env:   []string{"HOME=" + home},
		},
		{
			name:  "non-github host",
			args:  []string{"get"},
			stdin: "protocol=https\nhost=gitlab.com\npath=owner/repo\n\n",
			env:   []string{"HOME=" + home},
		},
		{
			name:  "store no-op",
			args:  []string{"store"},
			stdin: "protocol=https\nhost=github.com\npassword=ghs_whatever\n\n",
			env:   []string{"HOME=" + home},
		},
	}

	for _, inv := range invocations {
		t.Run(inv.name, func(t *testing.T) {
			// Given: an invocation expected to produce output (or error output)
			result := runSigil(t, inv.args, inv.stdin, inv.env)
			combined := result.stdout + result.stderr

			// Then: no PEM markers appear in any output
			for _, marker := range pemMarkers {
				if strings.Contains(combined, marker) {
					t.Errorf("invocation %q: PEM marker %q found in output — key material leak\nstdout: %s\nstderr: %s",
						inv.name, marker, result.stdout, result.stderr)
				}
			}
		})
	}
}

// TestCLI_LeakScan_TokenNotInStderr verifies that if a token is returned via a
// sanctioned stdout channel, the same token value does NOT appear in stderr.
// This is the channel-separation invariant, checked at both of Sigil's
// stdout-emission sites: `sigil get`'s `password=` line (writeCredentialResponse)
// and `sigil token --rate-limit-only`'s fmt.Println (cmdTokenNarrowed, #3647
// Warden finding #2 — this second site previously had no leak-scan coverage
// at all). Requires 1Password credentials; stays gated on that exactly as
// before — this does not run under `just check`.
func TestCLI_LeakScan_TokenNotInStderr(t *testing.T) {
	skipIfNoOPToken(t)

	opKeyRef := os.Getenv("SIGIL_TEST_OP_KEY_REF")
	if opKeyRef == "" {
		t.Skip("SIGIL_TEST_OP_KEY_REF not set")
	}

	const installID = 12345
	const issuedToken = "ghs_issued_token_for_leak_check"
	const narrowedIssuedToken = "ghs_narrowed_issued_token_for_leak_check"

	// The full-permission path (exchangeInstallationToken) sends a nil request
	// body; the narrowed path (exchangeNarrowedToken) always sends a JSON
	// body. Both hit the same access_tokens URL under this one mock server, so
	// body presence is what distinguishes which response shape to return.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if len(body) == 0 {
			json.NewEncoder(w).Encode(map[string]string{
				"token":      issuedToken,
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"token":                narrowedIssuedToken,
			"permissions":          map[string]string{"metadata": "read"},
			"repository_selection": "selected",
			"repositories":         []map[string]any{{"name": "repo", "private": false}},
		})
	}))
	defer srv.Close()

	home := writeTempConfig(t, minimalConfig(opKeyRef, 42, installID, "leak-test-owner"))
	stdin := "protocol=https\nhost=github.com\npath=leak-test-owner/repo\n\n"

	result := runSigil(t, []string{"get"}, stdin, []string{
		"HOME=" + home,
		"SIGIL_GITHUB_API_URL=" + srv.URL,
	})

	// Given: the token may appear in the sanctioned stdout "password=" line
	// Then: the token must NOT appear in stderr (unsanctioned channel)
	if strings.Contains(result.stderr, issuedToken) {
		t.Errorf("installation token appeared in stderr — unsanctioned channel leak\nstderr: %s", result.stderr)
	}

	// And: no PEM markers in any channel
	combined := result.stdout + result.stderr
	for _, marker := range []string{"BEGIN RSA PRIVATE KEY", "BEGIN PRIVATE KEY"} {
		if strings.Contains(combined, marker) {
			t.Errorf("PEM marker %q in output — key material leak", marker)
		}
	}

	// --- second stdout-emission site: sigil token --rate-limit-only ---

	narrowResult := runSigil(t, []string{"token", "--rate-limit-only", "leak-test-owner/repo"}, "", []string{
		"HOME=" + home,
		"SIGIL_GITHUB_API_URL=" + srv.URL,
	})

	// Positive control: the narrowed token must actually reach stdout, or the
	// negative check below (absent from stderr) would be vacuously true.
	if !strings.Contains(narrowResult.stdout, narrowedIssuedToken) {
		t.Fatalf("narrowed token never reached stdout — positive control failed\nstdout: %s\nstderr: %s", narrowResult.stdout, narrowResult.stderr)
	}
	if strings.Contains(narrowResult.stderr, narrowedIssuedToken) {
		t.Errorf("narrowed token appeared in stderr — unsanctioned channel leak\nstderr: %s", narrowResult.stderr)
	}

	combinedNarrow := narrowResult.stdout + narrowResult.stderr
	for _, marker := range []string{"BEGIN RSA PRIVATE KEY", "BEGIN PRIVATE KEY"} {
		if strings.Contains(combinedNarrow, marker) {
			t.Errorf("PEM marker %q in narrowed-path output — key material leak", marker)
		}
	}
}

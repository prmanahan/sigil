package main_test

// Black-box tests for .githooks/pre-commit, the staged-change secret scan.
//
// External test package, like cli_test.go: the hook is a shell script, not Go
// code, so nothing here touches package main's identifiers. The tests run the
// script with stub `git` and `gitleaks` as the only things on PATH, so they need
// no real gitleaks and pin only the hook's own decision logic: a change counts as
// scanned only when the staged set was readable, gitleaks ran, exited 0 and
// logged no ERR line. Anything else must refuse, with a message that says which.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const hookPath = ".githooks/pre-commit"

// runHook runs the hook under /bin/sh with PATH set to pathDirs only, so the
// real gitleaks (if installed) is never reachable. It returns the exit code and
// combined output.
func runHook(t *testing.T, pathDirs string) (int, string) {
	t.Helper()
	cmd := exec.Command("/bin/sh", hookPath)
	cmd.Env = []string{"PATH=" + pathDirs}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %s failed unexpectedly: %v", hookPath, err)
		}
		code = exitErr.ExitCode()
	}
	return code, string(out)
}

// writeStub writes an executable shell script named name into dir.
func writeStub(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
}

// stubs builds a PATH dir holding a fake git (the preflight read exits gitExit)
// and, unless noGitleaks, a fake gitleaks that records its arguments to the
// returned file, writes stderrText to stderr and exits gitleaksExit.
func stubs(t *testing.T, gitExit, gitleaksExit int, stderrText string, noGitleaks bool) (dir, argsFile string) {
	t.Helper()
	dir = t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	writeStub(t, dir, "git", "echo 'stub git: fatal: bad index' >&2\nexit "+strconv.Itoa(gitExit)+"\n")
	if gitExit == 0 {
		writeStub(t, dir, "git", "exit 0\n")
	}
	if !noGitleaks {
		writeStub(t, dir, "gitleaks",
			"printf '%s\\n' \"$*\" > '"+argsFile+"'\n"+
				"printf '%s\\n' '"+stderrText+"' >&2\n"+
				"exit "+strconv.Itoa(gitleaksExit)+"\n")
	}
	return dir, argsFile
}

func skipWithoutPOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook missing: %v", err)
	}
}

func TestPreCommitHook_DecisionByGitleaksOutcome(t *testing.T) {
	skipWithoutPOSIXShell(t)

	const inf = "12:00PM INF no leaks found"
	cases := []struct {
		name       string
		stubExit   int
		stderrText string
		wantCode   int
		wantMsg    string // empty: assert no REFUSED line instead
	}{
		{"clean scan passes", 0, inf, 0, ""},
		{"exit 0 with INF lines passes", 0, "12:00PM INF 0 commits scanned.\n12:00PM INF scanned ~6 bytes", 0, ""},
		{"exit 0 with an ERR line refuses", 0, "12:00PM ERR [git] fatal: index file smaller than expected\n" + inf, 1, "ERR line on stderr despite exit 0"},
		{"finding (42) refuses", 42, "12:00PM WRN leaks found: 1", 1, "found a secret"},
		{"scan error (1) refuses", 1, "12:00PM FTL unable to load config", 1, "failed to scan (exit 1)"},
		{"other nonzero (2) refuses", 2, inf, 1, "failed to scan (exit 2)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, argsFile := stubs(t, 0, tc.stubExit, tc.stderrText, false)
			code, out := runHook(t, dir)
			if code != tc.wantCode {
				t.Errorf("exit = %d, want %d; output: %q", code, tc.wantCode, out)
			}
			if tc.wantMsg == "" {
				if strings.Contains(out, "REFUSED") {
					t.Errorf("clean scan printed a refusal: %q", out)
				}
			} else if !strings.Contains(out, "REFUSED") || !strings.Contains(out, tc.wantMsg) {
				t.Errorf("output %q lacks REFUSED and %q", out, tc.wantMsg)
			}
			// gitleaks' own stderr must still reach the user.
			if first := strings.SplitN(tc.stderrText, "\n", 2)[0]; !strings.Contains(out, first) {
				t.Errorf("output %q lost gitleaks stderr %q", out, first)
			}
			// The hook must ask for the staged scan and the verbose, redacted report.
			// --no-color is load-bearing: gitleaks 8.30.1 colours its log lines
			// even when stderr is a pipe, wrapping the level token as ESC[31mERR
			// ESC[0m, which the hook's whitespace-delimited ERR match does not
			// see, so an unscanned commit would pass. Dropping the flag is a
			// silent fail-open; this assertion makes it loud.
			args, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("stub was never invoked: %v", err)
			}
			for _, want := range []string{"git", "--staged", "--redact", "--verbose", "--no-color"} {
				if !strings.Contains(string(args), want) {
					t.Errorf("gitleaks args %q lack %q", args, want)
				}
			}
		})
	}
}

func TestPreCommitHook_FailingGitPreflightRefuses(t *testing.T) {
	skipWithoutPOSIXShell(t)
	// gitleaks would happily report clean; the staged set is unreadable.
	dir, argsFile := stubs(t, 128, 0, "12:00PM INF no leaks found", false)
	code, out := runHook(t, dir)
	if code != 1 {
		t.Errorf("exit = %d, want 1; output: %q", code, out)
	}
	if !strings.Contains(out, "REFUSED") || !strings.Contains(out, "could not read the staged set") {
		t.Errorf("output %q lacks the preflight refusal", out)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("gitleaks ran although the git preflight failed")
	}
}

func TestPreCommitHook_MissingGitleaksRefuses(t *testing.T) {
	skipWithoutPOSIXShell(t)
	// git present, gitleaks nowhere on PATH.
	dir, _ := stubs(t, 0, 0, "", true)
	code, out := runHook(t, dir)
	if code != 1 {
		t.Errorf("exit = %d, want 1; output: %q", code, out)
	}
	if !strings.Contains(out, "REFUSED") || !strings.Contains(out, "not found on PATH") {
		t.Errorf("output %q lacks the missing-binary refusal", out)
	}
}

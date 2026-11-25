package main_test

// Tests of the gh shim's real-gh path (R-10, R-39), subcommand refusal (R-11),
// mint (R-12), environment (R-13), argv and exit status (R-15) and per-call
// minting (R-16).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func (f *shimFixture) writeInstallTimePath(content string) {
	f.t.Helper()
	dir := filepath.Join(f.home, ".config", "sigil")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gh-real-path"), []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// R-10: the real gh path comes from SIGIL_GH_REAL, else from the install-time
// file. Anything that is not an absolute path to an executable file fails
// class real-gh with no helper call.
func TestShim_R10_RealGHPathRejected(t *testing.T) {
	bad := map[string]func(f *shimFixture) string{
		"relative path": func(f *shimFixture) string { return "relative/gh" },
		"missing file":  func(f *shimFixture) string { return filepath.Join(f.fx, "no-such-gh") },
		"non-executable file": func(f *shimFixture) string {
			p := filepath.Join(f.fx, "plain-file")
			if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
				f.t.Fatal(err)
			}
			return p
		},
		"directory": func(f *shimFixture) string { return f.fx },
	}
	for name, mk := range bad {
		t.Run("override "+name, func(t *testing.T) {
			f := newShimFixture(t)
			f.setEnv("SIGIL_GH_REAL", mk(f))
			res := f.run("api", "user")
			f.expectRefused(res, "real-gh")
			f.expectNoHelperCall()
		})
		t.Run("install-time "+name, func(t *testing.T) {
			f := newShimFixture(t)
			f.unsetEnv("SIGIL_GH_REAL")
			f.writeInstallTimePath(mk(f) + "\n")
			res := f.run("api", "user")
			f.expectRefused(res, "real-gh")
			f.expectNoHelperCall()
		})
	}
	t.Run("no value from either source", func(t *testing.T) {
		f := newShimFixture(t)
		f.unsetEnv("SIGIL_GH_REAL")
		res := f.run("api", "user")
		f.expectRefused(res, "real-gh")
		f.expectNoHelperCall()
	})
	t.Run("an empty override falls back to the install-time file, which is the control", func(t *testing.T) {
		f := newShimFixture(t)
		f.writeInstallTimePath(filepath.Join(f.fx, "gh-real") + "\n")
		f.setEnv("SIGIL_GH_REAL", "")
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Errorf("a valid install-time path should run gh: exit %d, runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
	})
	t.Run("a set override wins over a broken install-time file", func(t *testing.T) {
		f := newShimFixture(t)
		f.writeInstallTimePath("relative/gh\n")
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Errorf("the override should win: exit %d, runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
	})
}

// R-39: a real gh path that is the shim itself, by path or by file identity,
// fails class real-gh before any mint.
func TestShim_R39_RealGHPathIsTheShimItself(t *testing.T) {
	mk := map[string]func(f *shimFixture) string{
		"its own path": func(f *shimFixture) string { return shimTrackedPath(f.t) },
		"a symbolic link to it": func(f *shimFixture) string {
			p := filepath.Join(f.fx, "gh-link")
			if err := os.Symlink(shimTrackedPath(f.t), p); err != nil {
				f.t.Fatal(err)
			}
			return p
		},
	}
	for name, path := range mk {
		t.Run("override "+name, func(t *testing.T) {
			f := newShimFixture(t)
			f.setEnv("SIGIL_GH_REAL", path(f))
			res := f.run("api", "user")
			f.expectRefused(res, "real-gh")
			f.expectNoHelperCall()
		})
		t.Run("install-time "+name, func(t *testing.T) {
			f := newShimFixture(t)
			f.unsetEnv("SIGIL_GH_REAL")
			f.writeInstallTimePath(path(f) + "\n")
			res := f.run("api", "user")
			f.expectRefused(res, "real-gh")
			f.expectNoHelperCall()
		})
	}
}

// R-11: auth, extension, extensions and ext are refused, with neither fake
// started and the outcome recorded as refused.
func TestShim_R11_AuthAndExtensionSubcommandsRefused(t *testing.T) {
	calls := [][]string{
		{"auth", "setup-git"},
		{"auth", "token"},
		{"extension", "install", "o/r"},
		{"extensions"},
		{"ext"},
	}
	for _, argv := range calls {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			f := newShimFixture(t)
			res := f.run(argv...)
			f.expectRefused(res, "refused")
			f.expectNoHelperCall()
			f.expectOneRecord("refused", "fixowner/fixrepo")
		})
	}
	t.Run("gh api user is not refused, the control", func(t *testing.T) {
		f := newShimFixture(t)
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Errorf("gh api user should run: exit %d, runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
	})
}

// R-12: the helper is asked for exactly `token --landing <owner/repo>`; its
// output is accepted only as one non-empty line without space, tab or CR.
func TestShim_R12_MintOutputValidation(t *testing.T) {
	t.Run("the helper receives exactly token --landing owner/repo", func(t *testing.T) {
		f := newShimFixture(t)
		res := f.run("api", "user")
		if res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		h := f.helperCalls()
		if len(h) != 1 || len(h[0]) != 3 || h[0][0] != "token" || h[0][1] != "--landing" || h[0][2] != "fixowner/fixrepo" {
			t.Errorf("helper argv = %q, want exactly [token --landing fixowner/fixrepo]", h)
		}
	})
	rejected := map[string]string{
		"a token then a non-zero exit": "printf '%s\\n' TOKEN\nexit 1",
		"no token and a non-zero exit": "exit 1",
		"empty output, exit 0":         "exit 0",
		"two lines":                    "printf 'first\\nsecond\\n'",
		"a token ending in a CR":       "printf '%s\\r\\n' TOKEN",
		"a space inside":               "printf 'tok en\\n'",
		"a tab inside":                 "printf 'tok\\ten\\n'",
	}
	for name, behaviour := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			f := newShimFixture(t)
			f.setHelper("echo helper-stderr-line >&2\n" + strings.ReplaceAll(behaviour, "TOKEN", f.token))
			res := f.run("api", "user")
			f.expectRefused(res, "mint-failed")
			if !strings.Contains(res.stderr, "helper-stderr-line") {
				t.Errorf("the helper's stderr should pass through, got %q", res.stderr)
			}
			r := f.expectOneRecord("mint-failed", "fixowner/fixrepo")
			if _, err := strconv.Atoi(r.mintSeconds); err != nil {
				t.Errorf("a mint-failed record carries the mint time, got %q", r.mintSeconds)
			}
			if strings.Contains(res.stdout+res.stderr, f.token) {
				t.Errorf("a rejected token must not be printed")
			}
		})
	}
	t.Run("accepts a token with no trailing newline, the control", func(t *testing.T) {
		f := newShimFixture(t)
		f.setHelper("printf '%s' " + f.token)
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Fatalf("exit %d, gh runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
		if got := f.ghEnv()["GH_TOKEN"]; got != f.token {
			t.Errorf("GH_TOKEN = %q, want the helper's output", got)
		}
	})
}

func shimClean(p string) string { return filepath.Clean(p) }

// R-13: the environment the real gh receives.
func TestShim_R13_EnvironmentSetForTheRealGH(t *testing.T) {
	t.Run("dedicated variables set, seeded variables removed or overwritten", func(t *testing.T) {
		f := newShimFixture(t)
		seed := shimRandomToken(t)
		for _, k := range []string{"GITHUB_TOKEN", "GH_HOST", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
			f.setEnv(k, "seed-"+seed)
		}
		f.setEnv("GH_TOKEN", "seed-"+seed)
		f.setEnv("GH_CONFIG_DIR", filepath.Join(f.home, "seeded-config"))
		f.setEnv("XDG_DATA_HOME", filepath.Join(f.home, "seeded-data"))
		f.setEnv("GH_PROMPT_DISABLED", "0")
		res := f.run("api", "user")
		if res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		env := f.ghEnv()
		if env["GH_TOKEN"] != f.token {
			t.Errorf("GH_TOKEN = %q, want exactly the helper's line", env["GH_TOKEN"])
		}
		for _, k := range []string{"GITHUB_TOKEN", "GH_HOST", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
			if v, ok := env[k]; ok {
				t.Errorf("%s must be removed, gh saw %q", k, v)
			}
		}
		if got, want := env["GH_CONFIG_DIR"], filepath.Join(f.home, ".config", "sigil", "gh"); got != want {
			t.Errorf("GH_CONFIG_DIR = %q, want %q", got, want)
		}
		xdg := env["XDG_DATA_HOME"]
		if want := filepath.Join(f.home, ".local", "share", "sigil", "gh-data"); xdg != want {
			t.Errorf("XDG_DATA_HOME = %q, want %q", xdg, want)
		}
		for _, other := range []string{filepath.Join(f.home, "seeded-data"), filepath.Join(f.home, ".local", "share"), filepath.Join(f.home, ".local", "share", "gh")} {
			if shimClean(xdg) == shimClean(other) {
				t.Errorf("XDG_DATA_HOME must differ from %q", other)
			}
		}
		if env["GH_PROMPT_DISABLED"] != "1" {
			t.Errorf("GH_PROMPT_DISABLED = %q, want 1", env["GH_PROMPT_DISABLED"])
		}
	})
	t.Run("BASHOPTS and BASH_XTRACEFD reach neither fake, and the copy without the unset shows both", func(t *testing.T) {
		f := newShimFixture(t)
		f.setEnv("BASHOPTS", "extglob")
		f.setEnv("BASH_XTRACEFD", "7")
		res := f.run("api", "user")
		if res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, env := range []map[string]string{f.helperEnv(), f.ghEnv()} {
			for _, k := range []string{"BASHOPTS", "BASH_XTRACEFD"} {
				if v, ok := env[k]; ok {
					t.Errorf("%s must not reach a child, saw %q", k, v)
				}
			}
		}
		// Control: without that unset, both reach the fake helper.
		f.shim = f.editedCopy("no-bash-unset", [2]string{"unset BASHOPTS BASH_XTRACEFD GITHUB_TOKEN", "unset GITHUB_TOKEN"})
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("control copy exit %d, stderr=%q", res.exit, res.stderr)
		}
		ctl := f.helperEnv()
		if ctl["BASHOPTS"] != "extglob" || ctl["BASH_XTRACEFD"] != "7" {
			t.Errorf("the control copy should show both variables in the helper's environment, got BASHOPTS=%q BASH_XTRACEFD=%q", ctl["BASHOPTS"], ctl["BASH_XTRACEFD"])
		}
	})
}

// R-15: the argv reaches gh unchanged, and the shim's exit status is gh's.
func TestShim_R15_ArgvForwardedAndExitStatusPassedThrough(t *testing.T) {
	argv := []string{"pr", "view", "--repo=o/other", "a b", "-R", "o/other", "https://github.com/o/other", "repos/o/other/pulls"}
	for _, code := range []int{0, 1, 7, 42} {
		t.Run("exit "+strconv.Itoa(code), func(t *testing.T) {
			f := newShimFixture(t)
			f.setGH(code)
			res := f.run(argv...)
			if res.exit != code {
				t.Errorf("shim exit = %d, want the fake gh's %d", res.exit, code)
			}
			if got := f.ghArgv(); strings.Join(got, "\x00") != strings.Join(argv, "\x00") {
				t.Errorf("gh argv = %q, want %q", got, argv)
			}
		})
	}
}

// R-16: each call mints its own token, and none is stored.
func TestShim_R16_EachCallMintsAndNothingIsStored(t *testing.T) {
	f := newShimFixture(t)
	seq := filepath.Join(f.fx, "seq")
	f.setHelper("n=0\n[ -f '" + seq + "' ] && read n < '" + seq + "'\nn=$((n+1))\necho $n > '" + seq + "'\nprintf '%s\\n' " + f.token + "-$n")
	t1, t2 := f.token+"-1", f.token+"-2"

	if res := f.run("api", "user"); res.exit != 0 {
		t.Fatalf("first call: exit %d, stderr=%q", res.exit, res.stderr)
	}
	first := f.ghEnv()["GH_TOKEN"]
	if res := f.run("api", "user"); res.exit != 0 {
		t.Fatalf("second call: exit %d, stderr=%q", res.exit, res.stderr)
	}
	second := f.ghEnv()["GH_TOKEN"]

	if len(f.helperCalls()) != 2 {
		t.Errorf("the helper should run once per call, calls: %q", f.helperCalls())
	}
	if first != t1 || second != t2 || first == second {
		t.Errorf("the two calls should hand over two different tokens, got %q and %q", first, second)
	}
	// The scan, with its control: it finds a token planted under TMPDIR.
	planted := filepath.Join(f.tmp, "planted")
	if err := os.WriteFile(planted, []byte(t1), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := shimScanTree(t, f.root, t1); len(hits) != 1 || hits[0] != planted {
		t.Fatalf("the scan control should find exactly the planted file, found %q", hits)
	}
	if err := os.Remove(planted); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{t1, t2} {
		if hits := shimScanTree(t, f.root, tok); len(hits) != 0 {
			t.Errorf("a minted token must not be stored, found it in %q", hits)
		}
	}
}

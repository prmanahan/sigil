package main_test

// Tests of the gh shim's first steps: the guard (R-8), the failure classes and
// their order (R-9), HOME (R-17), the session's repository (R-18 to R-20).
// Each test is one scenario, named for the requirement it checks.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// R-8: the tracked file exists, is executable and starts `#!/bin/sh`. The test
// fails, naming the path, when the shim is missing; it never skips.
func TestShim_R8_TrackedFileExistsExecutableAndShebang(t *testing.T) {
	p := shimTrackedPath(t)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("the tracked shim %s is missing: %v", shimRel, err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the tracked shim %s is not executable (mode %v)", shimRel, fi.Mode())
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", shimRel, err)
	}
	first, _, _ := strings.Cut(string(b), "\n")
	if first != "#!/bin/sh" {
		t.Errorf("first line of %s = %q, want exactly %q", shimRel, first, "#!/bin/sh")
	}
}

// R-9: each failure class has its own stderr prefix, and the steps run in
// order.
func TestShim_R9_FailureClassPrefixesAndStepOrder(t *testing.T) {
	t.Run("outside a clone and gh auth records no-repo, not refused", func(t *testing.T) {
		// Given a harness process outside any clone
		f := newShimFixture(t)
		f.startHarness(f.callDir)
		// When `gh auth token` runs
		res := f.run("auth", "token")
		// Then no-repo wins over the subcommand refusal, and the record says so
		f.expectRefused(res, "no-repo")
		f.expectOneRecord("no-repo", "")
		f.expectNoHelperCall()
	})
	t.Run("empty HOME and no real gh path is class home", func(t *testing.T) {
		f := newShimFixture(t)
		f.setEnv("HOME", "")
		f.unsetEnv("SIGIL_GH_REAL")
		res := f.run("api", "user")
		f.expectRefused(res, "home")
		f.expectNoHelperCall()
	})
	t.Run("HOME set and no real gh path is class real-gh", func(t *testing.T) {
		f := newShimFixture(t)
		f.unsetEnv("SIGIL_GH_REAL")
		res := f.run("api", "user")
		f.expectRefused(res, "real-gh")
		f.expectNoHelperCall()
	})
	t.Run("no real gh path and a harness outside a clone is real-gh, and the swapped copy gives no-repo", func(t *testing.T) {
		f := newShimFixture(t)
		f.unsetEnv("SIGIL_GH_REAL")
		f.startHarness(f.callDir)
		res := f.run("api", "user")
		f.expectRefused(res, "real-gh")

		// Control: a temporary copy with steps 2 and 3 swapped gives no-repo on
		// the same call, so this check can tell the two orders apart.
		text := shimTrackedText(t)
		i2 := strings.Index(text, "# Step 2:")
		i3 := strings.Index(text, "# Step 3:")
		i4 := strings.Index(text, "# Step 4:")
		if i2 < 0 || i3 < i2 || i4 < i3 {
			t.Fatalf("step markers not found in order in the tracked shim (%d, %d, %d)", i2, i3, i4)
		}
		swapped := text[:i2] + text[i3:i4] + text[i2:i3] + text[i4:]
		f.shim = f.copyOf("steps-2-3-swapped", swapped)
		ctl := f.run("api", "user")
		if !strings.HasPrefix(ctl.stderr, "gh shim: no-repo:") {
			t.Errorf("the swapped control copy should give no-repo, stderr=%q", ctl.stderr)
		}
	})
	t.Run("a refused subcommand in a resolving clone is class refused", func(t *testing.T) {
		f := newShimFixture(t)
		res := f.run("auth", "setup-git")
		f.expectRefused(res, "refused")
		f.expectNoHelperCall()
	})
}

// R-17: HOME unset, empty, relative or multi-line fails class home, with no
// helper call and no record.
func TestShim_R17_HomeValuesRefused(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *shimFixture)
	}{
		{"unset", func(f *shimFixture) { f.unsetEnv("HOME") }},
		{"empty", func(f *shimFixture) { f.setEnv("HOME", "") }},
		{"relative", func(f *shimFixture) { f.setEnv("HOME", "relative/home") }},
		{"two lines, first absolute", func(f *shimFixture) { f.setEnv("HOME", f.home+"\nsecond") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newShimFixture(t)
			c.set(f)
			res := f.run("api", "user")
			f.expectRefused(res, "home")
			f.expectNoHelperCall()
			if _, err := os.Stat(filepath.Join(f.home, ".local")); err == nil {
				t.Errorf("a refused HOME must write no record, but %s/.local exists", f.home)
			}
			if _, err := os.Stat(f.home + "\nsecond"); err == nil {
				t.Errorf("a two-line HOME must not be used as a directory")
			}
		})
	}
}

func shimExpectResolves(t *testing.T, f *shimFixture, want string) {
	t.Helper()
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("expected the call to succeed, exit %d, stderr=%q", res.exit, res.stderr)
	}
	calls := f.helperCalls()
	if len(calls) != 1 || strings.Join(calls[0], " ") != "token --landing "+want {
		t.Errorf("helper calls = %q, want exactly one: token --landing %s", calls, want)
	}
	if f.ghRuns() != 1 {
		t.Errorf("the fake gh should run once, ran %d", f.ghRuns())
	}
}

// R-18, resolving cases: the fake helper receives the fixture's owner/repo.
func TestShim_R18_SessionRepositoryResolves(t *testing.T) {
	t.Run("from another directory, as the call's own directory is not read", func(t *testing.T) {
		f := newShimFixture(t)
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("from a second clone with a different origin", func(t *testing.T) {
		f := newShimFixture(t)
		f.callDir = f.newClone("second", "https://github.com/other/other")
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("a .git suffixed origin resolves to the same value", func(t *testing.T) {
		f := newShimFixture(t)
		shimGitRun(t, f.clone, "config", "remote.origin.url", "https://github.com/fixowner/fixrepo.git")
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("a harness in a subdirectory of the clone", func(t *testing.T) {
		f := newShimFixture(t)
		sub := filepath.Join(f.clone, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		f.startHarness(sub)
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("a url insteadOf rule does not change the configured value", func(t *testing.T) {
		f := newShimFixture(t)
		shimGitRun(t, f.clone, "config", "url.https://example.invalid/.insteadOf", "https://github.com/")
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("an owner/.github origin resolves, the control for the name checks", func(t *testing.T) {
		f := newShimFixture(t)
		shimGitRun(t, f.clone, "config", "remote.origin.url", "https://github.com/fixowner/.github")
		shimExpectResolves(t, f, "fixowner/.github")
	})
	t.Run("injected configuration variables do not re-point the repository", func(t *testing.T) {
		f := newShimFixture(t)
		f.setEnv("GIT_CONFIG_COUNT", "1")
		f.setEnv("GIT_CONFIG_KEY_0", "remote.origin.url")
		f.setEnv("GIT_CONFIG_VALUE_0", "https://github.com/other/other")
		f.setEnv("GIT_CONFIG_PARAMETERS", "'remote.origin.url'='https://github.com/other/other'")
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
	t.Run("a GIT_DIR naming another clone does not re-point the repository", func(t *testing.T) {
		f := newShimFixture(t)
		second := f.newClone("second", "https://github.com/other/other")
		f.setEnv("GIT_DIR", filepath.Join(second, ".git"))
		shimExpectResolves(t, f, "fixowner/fixrepo")
	})
}

// R-18, refusing cases: each exits non-zero with outcome no-repo and an empty
// repository field, and starts neither the fake helper nor the fake gh.
func TestShim_R18_SessionRepositoryRefuses(t *testing.T) {
	setOrigin := func(v string) func(f *shimFixture) {
		return func(f *shimFixture) { shimGitRun(t, f.clone, "config", "remote.origin.url", v) }
	}
	cases := []struct {
		name string
		set  func(f *shimFixture)
	}{
		{"CLAUDE_PID unset", func(f *shimFixture) { f.unsetEnv("CLAUDE_PID") }},
		{"CLAUDE_PID empty", func(f *shimFixture) { f.setEnv("CLAUDE_PID", "") }},
		{"CLAUDE_PID not numeric", func(f *shimFixture) { f.setEnv("CLAUDE_PID", "abc") }},
		{"CLAUDE_PID with a comma", func(f *shimFixture) { f.setEnv("CLAUDE_PID", "1,"+f.env["CLAUDE_PID"]) }},
		{"CLAUDE_PID with a space", func(f *shimFixture) { f.setEnv("CLAUDE_PID", f.env["CLAUDE_PID"]+" 1") }},
		{"CLAUDE_PID two lines, first valid", func(f *shimFixture) { f.setEnv("CLAUDE_PID", f.env["CLAUDE_PID"]+"\n"+f.env["CLAUDE_PID"]) }},
		{"CLAUDE_PID names no running process", func(f *shimFixture) { f.setEnv("CLAUDE_PID", f.deadPID()) }},
		{"reader absent from PATH", func(f *shimFixture) { f.removeLsof() }},
		{"reader exits non-zero", func(f *shimFixture) { f.setLsof("exit 3\n") }},
		{"reader prints nothing", func(f *shimFixture) { f.setLsof("exit 0\n") }},
		{"reader prints two paths", func(f *shimFixture) { f.setLsof("printf 'p1\\nfcwd\\nn/tmp\\nn/var\\n'\n") }},
		{"reader prints a relative path", func(f *shimFixture) { f.setLsof("printf 'p1\\nfcwd\\nnrelative/dir\\n'\n") }},
		{"harness outside any clone", func(f *shimFixture) { f.startHarness(f.callDir) }},
		{"clone without an origin", func(f *shimFixture) { shimGitRun(t, f.clone, "remote", "remove", "origin") }},
		{"scp-style ssh origin", setOrigin("git@github.com:fixowner/fixrepo.git")},
		{"ssh url origin", setOrigin("ssh://git@github.com/fixowner/fixrepo")},
		{"non-GitHub https origin", setOrigin("https://example.com/fixowner/fixrepo")},
		{"http origin", setOrigin("http://github.com/fixowner/fixrepo")},
		{"owner beginning with a dash", setOrigin("https://github.com/-x/y")},
		{"repo beginning with a dash", setOrigin("https://github.com/x/-y")},
		{"owner is a dot", setOrigin("https://github.com/./x")},
		{"repo is two dots", setOrigin("https://github.com/x/..")},
		{"owner is two dots", setOrigin("https://github.com/../x")},
		{"owner/repo with a third segment", setOrigin("https://github.com/a/b/c")},
		{"repo with a character outside the set", setOrigin("https://github.com/a/b%20c")},
		{"empty repo after the suffix is stripped", setOrigin("https://github.com/a/.git")},
		{"two-line origin whose second line is valid", setOrigin("https://github.com/a/b\nhttps://github.com/c/d")},
		{"origin ending in a carriage return", setOrigin("https://github.com/a/b\r")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newShimFixture(t)
			c.set(f)
			res := f.run("api", "user")
			f.expectRefused(res, "no-repo")
			f.expectNoHelperCall()
			f.expectOneRecord("no-repo", "")
		})
	}
	t.Run("injected variables with a harness outside any clone still refuse", func(t *testing.T) {
		f := newShimFixture(t)
		f.startHarness(f.callDir)
		second := f.newClone("second", "https://github.com/other/other")
		f.setEnv("GIT_CONFIG_COUNT", "1")
		f.setEnv("GIT_CONFIG_KEY_0", "remote.origin.url")
		f.setEnv("GIT_CONFIG_VALUE_0", "https://github.com/other/other")
		f.setEnv("GIT_CONFIG_PARAMETERS", "'remote.origin.url'='https://github.com/other/other'")
		f.setEnv("GIT_DIR", filepath.Join(second, ".git"))
		res := f.run("api", "user")
		f.expectRefused(res, "no-repo")
		f.expectNoHelperCall()
	})
}

// R-19: the shim reads no repository from the call. The helper gets the
// session's repository, and the fake gh gets the argv unchanged.
func TestShim_R19_CallAimedAtAnotherRepositoryIsNotRead(t *testing.T) {
	calls := [][]string{
		{"pr", "list", "-R", "other/x"},
		{"pr", "list", "--repo=other/x"},
		{"repo", "view", "https://github.com/other/x"},
		{"api", "-X", "POST", "repos/other/x/git/refs"},
	}
	for _, argv := range calls {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			f := newShimFixture(t)
			res := f.run(argv...)
			if res.exit != 0 {
				t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
			}
			if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing fixowner/fixrepo" {
				t.Errorf("helper calls = %q, want the session's repository only", h)
			}
			if got := f.ghArgv(); strings.Join(got, "\x00") != strings.Join(argv, "\x00") {
				t.Errorf("gh argv = %q, want %q unchanged", got, argv)
			}
		})
	}
	t.Run("GH_REPO in the environment", func(t *testing.T) {
		f := newShimFixture(t)
		f.setEnv("GH_REPO", "other/x")
		res := f.run("pr", "list")
		if res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing fixowner/fixrepo" {
			t.Errorf("helper calls = %q, want the session's repository only", h)
		}
		if got := f.ghEnv()["GH_REPO"]; got != "other/x" {
			t.Errorf("gh GH_REPO = %q, want it passed through unchanged", got)
		}
	})
	t.Run("a call made from another clone's directory", func(t *testing.T) {
		f := newShimFixture(t)
		f.callDir = f.newClone("second", "https://github.com/other/x")
		res := f.run("pr", "list")
		if res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing fixowner/fixrepo" {
			t.Errorf("helper calls = %q, want the session's repository only", h)
		}
	})
}

// R-39: the real-gh comparison is by file identity, and `-ef` answers false, not
// an error, when a path cannot be stat'd. A shim whose own path cannot be stat'd
// (here, run as `sh -c <text> /no/such/path`) therefore refuses as real-gh
// instead of comparing against nothing and going on.
func TestShim_R39_UnstattableOwnPathRefusesAsRealGH(t *testing.T) {
	f := newShimFixture(t)
	cmd := exec.Command("/bin/sh", "-c", shimTrackedText(t), filepath.Join(f.root, "no-such-dir", "gh"))
	cmd.Dir = f.callDir
	cmd.Env = f.environ()
	var errb strings.Builder
	cmd.Stderr = &errb
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() == 0 {
		t.Fatalf("expected a non-zero exit, got %v; stderr=%q", err, errb.String())
	}
	if !strings.HasPrefix(errb.String(), "gh shim: real-gh:") {
		t.Errorf("stderr should begin %q, got %q", "gh shim: real-gh:", errb.String())
	}
	if f.ghRuns() != 0 {
		t.Errorf("the fake gh must not start")
	}
	f.expectNoHelperCall()
}

// R-20, shim half: a mixed-case origin is minted for as written, and a failing
// helper leaves the fake gh uninvoked.
func TestShim_R20_MixedCaseOriginReachesHelperAsWritten(t *testing.T) {
	f := newShimFixture(t)
	shimGitRun(t, f.clone, "config", "remote.origin.url", "https://github.com/FixOwner/FixRepo")
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing FixOwner/FixRepo" {
		t.Errorf("helper calls = %q, want the origin's case unchanged", h)
	}

	g := newShimFixture(t)
	shimGitRun(t, g.clone, "config", "remote.origin.url", "https://github.com/FixOwner/FixRepo")
	g.setHelper("echo 'grant mismatch' >&2\nexit 1")
	res2 := g.run("api", "user")
	g.expectRefused(res2, "mint-failed")
}

// R-19 and R-14's git environment: the GIT_ names come from awk. When awk
// fails, the shim must refuse as no-repo rather than run git with the caller's
// GIT_DIR in place, which would read a different clone's origin and mint for it.
func TestShim_GitEnvironment_FailingAwkRefusesAsNoRepo(t *testing.T) {
	// Two ways awk fails: a non-zero status, and a zero status with no sentinel
	// (output cut short). Each has a control copy lacking the one check that
	// catches it, which then reads the other clone.
	cases := []struct {
		name, awk string
		anchor    [2]string
	}{
		{"a non-zero status after the sentinel", "printf '%s\\n' --end--\nexit 1\n", [2]string{"print \"--end--\" }') || exit 1", "print \"--end--\" }')"}},
		{"no sentinel, status zero", "exit 0\n", [2]string{"      *) exit 1 ;;\n    esac", "      *) names= ;;\n    esac"}},
	}
	for _, c := range cases {
		setup := func(t *testing.T) *shimFixture {
			f := newShimFixture(t)
			other := f.newClone("second", "https://github.com/other/x")
			f.setEnv("GIT_DIR", filepath.Join(other, ".git"))
			shimWriteExec(t, filepath.Join(f.bin, "awk"), c.awk)
			return f
		}
		t.Run(c.name+": the shim refuses", func(t *testing.T) {
			f := setup(t)
			res := f.run("api", "user")
			f.expectRefused(res, "no-repo")
			f.expectNoHelperCall()
			f.expectOneRecord("no-repo", "")
		})
		t.Run(c.name+": control, a copy without the check reads the other clone", func(t *testing.T) {
			f := setup(t)
			f.shim = f.editedCopy("no-awk-check", c.anchor)
			res := f.run("api", "user")
			if res.exit != 0 {
				t.Fatalf("control copy exit %d, stderr=%q", res.exit, res.stderr)
			}
			if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing other/x" {
				t.Errorf("the control copy should mint for the other clone, helper calls = %q", h)
			}
		})
	}
}

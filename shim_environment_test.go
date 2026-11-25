package main_test

// Tests of what the gh shim's own environment handling lets through: variables
// the helper must not inherit (R-13), shell functions imported from the
// environment, and the shim's internal variable names.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	shimAssignedNameRe = regexp.MustCompile(`(?:^|[\s;|&(])([a-z_][a-z0-9_]*)=`)
	shimForNameRe      = regexp.MustCompile(`\bfor ([a-z_][a-z0-9_]*) in\b`)
)

// shimAssignedNames returns the lower-case variable names the tracked script
// assigns, read from its non-comment lines.
func shimAssignedNames(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, line := range strings.Split(shimTrackedText(t), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, m := range shimAssignedNameRe.FindAllStringSubmatch(line, -1) {
			seen[m[1]] = true
		}
		for _, m := range shimForNameRe.FindAllStringSubmatch(line, -1) {
			seen[m[1]] = true
		}
	}
	var names []string
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The shim's own variables never reach gh. A variable of the same name that
// arrived exported would keep its export mark through the shim's assignment and
// hand gh the shim's value, so each name the script assigns is unset first.
// Control: a copy without that unset hands gh the shim's `repo`.
func TestShim_InternalNamesAreNotExportedToGH(t *testing.T) {
	names := shimAssignedNames(t)
	for _, n := range names {
		if !strings.HasPrefix(n, shimPrivatePrefix) {
			t.Errorf("the shim assigns %q, which lacks the private prefix %q", n, shimPrivatePrefix)
		}
	}
	for _, must := range []string{"_sgs_repo", "_sgs_origin", "_sgs_real", "_sgs_url", "_sgs_owner", "_sgs_name"} {
		i := sort.SearchStrings(names, must)
		if i == len(names) || names[i] != must {
			t.Fatalf("the name scan missed %q in %q; it would be vacuous", must, names)
		}
	}
	seeded := func(f *shimFixture) {
		for _, n := range names {
			f.setEnv(n, "seed-"+n)
		}
	}
	f := newShimFixture(t)
	seeded(f)
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	env := f.ghEnv()
	for _, n := range names {
		if v, ok := env[n]; ok {
			t.Errorf("gh's environment holds the shim's internal name %s=%q", n, v)
		}
	}

	var kept []string
	for _, l := range strings.Split(shimTrackedText(t), "\n") {
		if !strings.HasPrefix(l, "unset _sgs_nl ") {
			kept = append(kept, l)
		}
	}
	if len(kept) != len(strings.Split(shimTrackedText(t), "\n"))-1 {
		t.Fatalf("expected exactly one `unset _sgs_nl ...` line in the tracked shim")
	}
	g := newShimFixture(t)
	seeded(g)
	g.shim = g.copyOf("no-internal-unset", strings.Join(kept, "\n"))
	if c := g.run("api", "user"); c.exit != 0 {
		t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
	}
	if got := g.ghEnv()["_sgs_repo"]; got != "fixowner/fixrepo" {
		t.Errorf("the control copy should hand gh the shim's repo, got %q", got)
	}
}

// shimPrivatePrefix is the prefix of every variable the shim assigns. A caller
// will not plausibly export a name that starts with it, so the shim's unset of
// its own names cannot remove a variable the caller meant gh to see.
const shimPrivatePrefix = "_sgs_"

// FD-7 (gh gets the caller's environment unchanged apart from FD-4's list): a
// caller-exported variable whose name the shim once used internally reaches gh
// with the caller's value. Control: a copy that unsets those names (the shim's
// former behaviour) hides them from gh, so the assertion can fail.
func TestShim_FD7_CallerExportedGenericNamesReachGHUnchanged(t *testing.T) {
	generic := []string{"name", "url", "file", "dir", "repo", "class", "now", "top", "origin", "owner", "real", "rest", "cwd", "same", "tab", "nl", "cr"}
	seed := func(f *shimFixture) {
		for _, n := range generic {
			f.setEnv(n, "caller-"+n)
		}
	}
	f := newShimFixture(t)
	seed(f)
	if res := f.run("api", "user"); res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	env := f.ghEnv()
	for _, n := range generic {
		if got := env[n]; got != "caller-"+n {
			t.Errorf("gh should see the caller's %s=%q, got %q", n, "caller-"+n, got)
		}
	}

	g := newShimFixture(t)
	seed(g)
	g.shim = g.editedCopy("unsets-generic-names", [2]string{shimUnsetLine, shimUnsetLine + "unset " + strings.Join(generic, " ") + "\n"})
	if c := g.run("api", "user"); c.exit != 0 {
		t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
	}
	for _, n := range generic {
		if v, ok := g.ghEnv()[n]; ok {
			t.Errorf("the control copy should hide %s from gh, got %q", n, v)
		}
	}
}

// R-13, helper environment (maintainer's level-4 decision, review round 1): the
// helper holds the App JWT while it runs. GODEBUG=http2debug=2 makes Go's HTTP
// client log requests and responses to stderr, and SIGIL_GITHUB_API_URL points
// the JWT at a caller-chosen host, so neither reaches the helper. Control: a
// copy without the two names in its unset line shows both.
func TestShim_R13_HelperEnvironment_DebugAndAPIURLAreScrubbed(t *testing.T) {
	seed := map[string]string{"GODEBUG": "http2debug=2", "SIGIL_GITHUB_API_URL": "https://api.invalid.example"}
	f := newShimFixture(t)
	for k, v := range seed {
		f.setEnv(k, v)
	}
	if res := f.run("api", "user"); res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	env := f.helperEnv()
	for k := range seed {
		if v, ok := env[k]; ok {
			t.Errorf("the helper's environment holds %s=%q", k, v)
		}
	}

	f.shim = f.editedCopy("no-helper-env-scrub", [2]string{" GODEBUG SIGIL_GITHUB_API_URL", ""})
	if c := f.run("api", "user"); c.exit != 0 {
		t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
	}
	env = f.helperEnv()
	for k, v := range seed {
		if env[k] != v {
			t.Errorf("the control copy should pass %s=%q to the helper, got %q", k, v, env[k])
		}
	}
}

// The lines that put GODEBUG and SIGIL_GITHUB_API_URL back for gh after the
// helper has run. The controls below edit their conditions out or force them on.
const (
	shimRestoreGodebugCond = `command [ -n "$_sgs_godebug_set" ]`
	shimRestoreAPIURLCond  = `command [ -n "$_sgs_api_url_set" ]`
)

// FD-7 (gh gets the caller's environment unchanged apart from FD-4's list):
// GODEBUG and SIGIL_GITHUB_API_URL are scrubbed from the helper's environment
// (the test above) but are not on FD-4's list, so gh gets each exactly as it was
// inherited: set and exported with the same value, an empty value included, or
// unset when it was unset.
func TestShim_FD7_GodebugAndAPIURLReachGHAsInherited(t *testing.T) {
	names := []string{"GODEBUG", "SIGIL_GITHUB_API_URL"}
	t.Run("an inherited value reaches gh unchanged and never the helper", func(t *testing.T) {
		f := newShimFixture(t)
		for _, n := range names {
			f.setEnv(n, "inherited-"+n)
		}
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, n := range names {
			if got, ok := f.ghEnv()[n]; !ok || got != "inherited-"+n {
				t.Errorf("gh should see %s=%q exported, got %q (present %v)", n, "inherited-"+n, got, ok)
			}
			if v, ok := f.helperEnv()[n]; ok {
				t.Errorf("the helper's environment holds %s=%q", n, v)
			}
		}
	})
	t.Run("an inherited empty value stays set and empty", func(t *testing.T) {
		f := newShimFixture(t)
		for _, n := range names {
			f.setEnv(n, "")
		}
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, n := range names {
			if got, ok := f.ghEnv()[n]; !ok || got != "" {
				t.Errorf("gh should see %s set and empty, got %q (present %v)", n, got, ok)
			}
		}
	})
	t.Run("an unset variable stays unset for gh", func(t *testing.T) {
		f := newShimFixture(t)
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, n := range names {
			if v, ok := f.ghEnv()[n]; ok {
				t.Errorf("%s was not inherited, gh should not see it, got %q", n, v)
			}
		}
	})
	t.Run("control: a copy without the restore hides both from gh", func(t *testing.T) {
		f := newShimFixture(t)
		for _, n := range names {
			f.setEnv(n, "x-"+n)
		}
		f.shim = f.editedCopy("no-restore", [2]string{shimRestoreGodebugCond, `command [ -n "" ]`}, [2]string{shimRestoreAPIURLCond, `command [ -n "" ]`})
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("control copy exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, n := range names {
			if v, ok := f.ghEnv()[n]; ok {
				t.Errorf("the control copy should hide %s from gh, got %q", n, v)
			}
		}
	})
	t.Run("control: a copy that restores unconditionally exports an unset variable as empty", func(t *testing.T) {
		f := newShimFixture(t)
		f.shim = f.editedCopy("restore-always", [2]string{shimRestoreGodebugCond, "command [ 0 -eq 0 ]"}, [2]string{shimRestoreAPIURLCond, "command [ 0 -eq 0 ]"})
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("control copy exit %d, stderr=%q", res.exit, res.stderr)
		}
		for _, n := range names {
			if v, ok := f.ghEnv()[n]; !ok || v != "" {
				t.Errorf("the control copy should export %s as empty, got %q (present %v)", n, v, ok)
			}
		}
	})
}

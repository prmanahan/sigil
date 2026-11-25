package main_test

// R-14 and R-13's runtime tests of how the shim handles the minted token and
// the helper's own secret: argv, output, files, shell tracing, allexport and
// an inherited export of the token variable.
//
// The three tests tagged macOS runtime are top-level tests whose names carry,
// each, exactly one of the segments the spec names. No other top-level test
// name in this package carries any of those segments. Each runs its own
// control inside the test: the leak must show on the control copy, or the test
// fails.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// shimNamesHolding returns the sorted names whose value contains token.
func shimNamesHolding(env map[string]string, token string) []string {
	var names []string
	for k, v := range env {
		if strings.Contains(v, token) {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names
}

// R-14: the token is in neither fake's argv and not in the shim's own stdout or
// stderr. The shared seeded control: a copy that also prints the token on a
// path makes that path's scan fail.
func TestShim_R14_TokenNotInArgvOrShimOutput(t *testing.T) {
	f := newShimFixture(t)
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if got := f.ghEnv()["GH_TOKEN"]; got != f.token {
		t.Fatalf("the call did not hand the token over (GH_TOKEN=%q); the scans below would be vacuous", got)
	}
	argv := strings.Join(shimReadLines(f.helperArgsFile()), "\n") + "\n" + strings.Join(f.ghArgv(), "\n")
	if strings.Contains(argv, f.token) {
		t.Errorf("the token is in a fake's argv")
	}
	if strings.Contains(res.stdout, f.token) {
		t.Errorf("the token is on the shim's stdout")
	}
	if strings.Contains(res.stderr, f.token) {
		t.Errorf("the token is on the shim's stderr")
	}

	// Control: copies that print the token on stdout, then on stderr.
	tok := `"$` + shimTokenVar + `"`
	f.shim = f.editedCopy("stdout-leak", [2]string{shimHandoffLine, "printf '%s\\n' " + tok + "\n" + shimHandoffLine})
	if c := f.run("api", "user"); !strings.Contains(c.stdout, f.token) {
		t.Errorf("the stdout control should show the token on stdout, got %q", c.stdout)
	}
	f.shim = f.editedCopy("stderr-leak", [2]string{shimHandoffLine, "printf '%s\\n' " + tok + " >&2\n" + shimHandoffLine})
	if c := f.run("api", "user"); !strings.Contains(c.stderr, f.token) {
		t.Errorf("the stderr control should show the token on stderr, got %q", c.stderr)
	}
}

// R-14, S9: after a call no file under the test's temporary root holds the
// token, with TMPDIR set inside that root. The same scan finds a token the test
// plants under TMPDIR.
func TestShim_R14_NoFileHoldsTheToken(t *testing.T) {
	f := newShimFixture(t)
	if res := f.run("api", "user"); res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if got := f.ghEnv()["GH_TOKEN"]; got != f.token {
		t.Fatalf("the call did not hand the token over (GH_TOKEN=%q)", got)
	}
	if hits := shimScanTree(t, f.root, f.token); len(hits) != 0 {
		t.Errorf("the token was written to %q", hits)
	}
	planted := filepath.Join(f.tmp, "planted")
	if err := os.WriteFile(planted, []byte("x "+f.token+" x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := shimScanTree(t, f.root, f.token); len(hits) != 1 || hits[0] != planted {
		t.Errorf("the control scan should find exactly the planted file, found %q", hits)
	}
}

// R-14: a variable of the token variable's name that arrived already exported
// must not carry the token. The fake gh sees it only as GH_TOKEN. Runs on every
// platform. Control: a copy without the unset shows the token under that name.
func TestShim_R14_InheritedExportOfTheTokenVariable(t *testing.T) {
	f := newShimFixture(t)
	f.setEnv(shimTokenVar, "preset-"+shimRandomToken(t))
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if names := shimNamesHolding(f.ghEnv(), f.token); strings.Join(names, ",") != "GH_TOKEN" {
		t.Errorf("the fake gh should hold the token only as GH_TOKEN, found it under %q", names)
	}
	if _, ok := f.helperEnv()[shimTokenVar]; ok {
		t.Errorf("the inherited export must not reach the helper, which starts before the capture")
	}

	f.shim = f.editedCopy("no-token-var-unset", [2]string{"unset " + shimTokenVar + "\n", ""})
	if c := f.run("api", "user"); c.exit != 0 {
		t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
	}
	names := shimNamesHolding(f.ghEnv(), f.token)
	if strings.Join(names, ",") != "GH_TOKEN,"+shimTokenVar {
		t.Errorf("the control copy should show the token under %s as well, found %q", shimTokenVar, names)
	}
}

// R-13, macOS runtime: with SHELLOPTS=xtrace exported, a helper's own secret
// stays off the shim's stderr. Control: the copy without the first statement
// shows it.
func TestShim_R13_Xtrace_HelperSecretNotOnStderr(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS runtime test: Linux dash ignores SHELLOPTS")
	}
	f := newShimFixture(t)
	secret := shimRandomToken(t)
	f.setHelper("helper_secret=" + secret + "\nprintf '%s\\n' " + f.token)
	f.setEnv("SHELLOPTS", "xtrace")
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if strings.Contains(res.stderr, secret) {
		t.Errorf("the helper's secret reached the shim's stderr under SHELLOPTS=xtrace")
	}
	f.shim = f.editedCopy("no-first-statement", [2]string{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\n"})
	ctl := f.run("api", "user")
	if !strings.Contains(ctl.stderr, secret) {
		t.Fatalf("the control copy shows no leak, so this test cannot detect one; stderr=%q", ctl.stderr)
	}
}

// R-14, macOS runtime: with SHELLOPTS=xtrace exported, the token stays off the
// shim's stderr. Control: the copy without the first statement shows it.
func TestShim_R14_Xtrace_TokenNotOnStderr(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS runtime test: Linux dash ignores SHELLOPTS")
	}
	f := newShimFixture(t)
	f.setEnv("SHELLOPTS", "xtrace")
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if strings.Contains(res.stderr, f.token) {
		t.Errorf("the token reached the shim's stderr under SHELLOPTS=xtrace")
	}
	f.shim = f.editedCopy("no-first-statement", [2]string{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\n"})
	ctl := f.run("api", "user")
	if !strings.Contains(ctl.stderr, f.token) {
		t.Fatalf("the control copy shows no leak, so this test cannot detect one; stderr=%q", ctl.stderr)
	}
}

// R-14, macOS runtime: with SHELLOPTS=allexport exported, the fake gh holds the
// token only as the value of GH_TOKEN. Control: a copy whose first statement is
// `set +x +v` shows it under another name.
func TestShim_R14_Allexport_TokenOnlyAsGHToken(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS runtime test: Linux dash ignores SHELLOPTS")
	}
	f := newShimFixture(t)
	f.setEnv("SHELLOPTS", "allexport")
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if names := shimNamesHolding(f.ghEnv(), f.token); strings.Join(names, ",") != "GH_TOKEN" {
		t.Errorf("the fake gh should hold the token only as GH_TOKEN, found it under %q", names)
	}
	f.shim = f.editedCopy("no-plus-a", [2]string{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\nset +x +v\n"})
	if c := f.run("api", "user"); c.exit != 0 {
		t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
	}
	names := shimNamesHolding(f.ghEnv(), f.token)
	if len(names) < 2 || names[0] == "" {
		t.Fatalf("the control copy shows the token only as GH_TOKEN (%q), so this test cannot detect a leak", names)
	}
	foundOther := false
	for _, n := range names {
		if n != "GH_TOKEN" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Fatalf("the control copy shows no second name for the token: %q", names)
	}
}

// The macOS runtime tests are recorded by name, and the spec's -v read counts
// them. shimSegmentProblems reconciles the declared set against test sources:
// exactly three top-level names, each carrying exactly one of the three
// segments, each segment carried by exactly one name.
func shimSegmentProblems(sources []string) []string {
	segments := []string{"R13_Xtrace", "R14_Xtrace", "R14_Allexport"}
	nameRe := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)
	carriers := map[string][]string{}
	var carrying, problems []string
	for _, src := range sources {
		for _, m := range nameRe.FindAllStringSubmatch(src, -1) {
			hit := 0
			for _, s := range segments {
				if strings.Contains(m[1], s) {
					carriers[s] = append(carriers[s], m[1])
					hit++
				}
			}
			if hit > 1 {
				problems = append(problems, m[1]+" carries more than one segment")
			}
			if hit > 0 {
				carrying = append(carrying, m[1])
			}
		}
	}
	if len(carrying) != 3 {
		problems = append(problems, fmt.Sprintf("expected exactly three names carrying a segment, found %d: %q", len(carrying), carrying))
	}
	for _, s := range segments {
		if len(carriers[s]) != 1 {
			problems = append(problems, fmt.Sprintf("segment %s must be carried by exactly one name, found %q", s, carriers[s]))
		}
	}
	return problems
}

func TestShim_MacOSRuntimeNames_ThreeNamesOneSegmentEach(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files found (%v)", err)
	}
	var sources []string
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, string(b))
	}
	if probs := shimSegmentProblems(sources); len(probs) != 0 {
		t.Errorf("the macOS runtime names do not reconcile:\n  %s", strings.Join(probs, "\n  "))
	}
	// Seeded violations: the reconciliation must fail on each.
	three := "func TestA_R13_Xtrace_x(t *testing.T) {}\nfunc TestB_R14_Xtrace_x(t *testing.T) {}\nfunc TestC_R14_Allexport_x(t *testing.T) {}\n"
	if probs := shimSegmentProblems([]string{three}); len(probs) != 0 {
		t.Fatalf("a correct set of three names should reconcile, got %q", probs)
	}
	seeded := map[string]string{
		"a fourth name":            three + "func TestD_R14_Xtrace_dup(t *testing.T) {}\n",
		"a missing segment":        "func TestA_R13_Xtrace_x(t *testing.T) {}\nfunc TestB_R14_Xtrace_x(t *testing.T) {}\n",
		"a name with two segments": "func TestA_R13_Xtrace_R14_Allexport(t *testing.T) {}\nfunc TestB_R14_Xtrace_x(t *testing.T) {}\nfunc TestC_R14_Allexport_x(t *testing.T) {}\n",
	}
	for name, src := range seeded {
		if probs := shimSegmentProblems([]string{src}); len(probs) == 0 {
			t.Errorf("seeded violation %q was not detected", name)
		}
	}
}

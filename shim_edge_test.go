package main_test

// Edge tests of the tracked gh shim (shim/gh), task T6 of task #4741 in the
// maintainer's tracker. Each test is one scenario of docs/plans/
// 2026-11-25-lead-session-gh-token-test-plan.md, section 3.2 (TP-S), and
// carries its TP id and the spec requirement it checks.
//
// Black-box. The shim is run in place as a process; the assertions read its
// exit status, stdout, stderr, the record and marker files it writes under a
// temporary HOME, and what the edge fakes below recorded.
//
// Independence (plan T6, D15, D17, D18). This file brings its own recording fake
// gh (edgeInstallGH), its own recording fake sigil-helper (edgeInstallHelper),
// its own Linux lsof reader, and its own assertions for the class line
// (edgeAssertClass), the record line (edgeReadRecords, edgeAssertRecord) and the
// environment handed to gh (edgeAssertEnv), all written from the spec. None of
// the implementer's setGH, setHelper, expectRefused, expectOneRecord, records(),
// shimToday, recordDir or helper/gh readers is called here.
//
// Process plumbing still reused from shim_harness_test.go, and nothing else:
// newShimFixture (the two temporary roots, the PATH of wrapper scripts for awk,
// grep, date, git and the rest, the default environment, the clone and the fake
// harness process it starts), startHarness, newClone, setEnv/unsetEnv, run, the
// fixture's path fields (root, fx, bin, home, tmp, callDir, shim, token),
// shimTrackedPath, shimRandomToken and shimScanTree. newShimFixture also installs
// the implementer's default fake helper, fake gh and (on Linux) fake lsof; the
// edge fakes overwrite all three at the same paths, before any call runs.
//
// Declared reads. Spec and test plan only. For the seeded checks of the report,
// shim/gh was read to choose a scratch edit; no assertion was shaped on it.
//
// Assertions follow D17 and D18 of the plan: the record's time is asserted as
// ISO 8601 UTC with a Z suffix, mint_seconds only as a decimal number or empty,
// the marker's time only as non-empty. The warning's wording is open beyond what
// R-24 names: the count, the assumed limit, reads-per-mint and the word
// maintainer; the limit and reads-per-mint are accepted as 1000 or 1,000 and 3
// or 3.0 (the shipped constants, R-23).
//
// Fixture tokens come from the harness (runtime, not ghs_-shaped). No scanner
// allow entry is added.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	edgeTimeRE    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
	edgeDecimalRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	edgeLogNameRE = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2})\.log$`)
	edgeLimitRE   = regexp.MustCompile(`(^|[^0-9,.])1,?000([^0-9,]|$)`)
	edgeReadsRE   = regexp.MustCompile(`(^|[^0-9.,])3(\.0)?([^0-9.]|$)`)
	edgeCountRE   = regexp.MustCompile(`(^|[^0-9,.])182([^0-9,]|$)`)
)

func edgeToday() string { return time.Now().UTC().Format("2006-01-02") }

// edgeSameDay skips (inconclusive, the plan's clock trap) when the UTC date
// moved during the call.
func edgeSameDay(t *testing.T, before string) {
	t.Helper()
	if after := edgeToday(); after != before {
		t.Skipf("inconclusive: the UTC date moved from %s to %s during the call; re-run", before, after)
	}
}

// edgeStateDir is R-21's directory, from HOME as the spec spells it.
func (f *shimFixture) edgeStateDir() string {
	return filepath.Join(f.home, ".local", "state", "sigil", "gh-shim")
}

func (f *shimFixture) edgeLogPath() string {
	return filepath.Join(f.edgeStateDir(), edgeToday()+".log")
}

func (f *shimFixture) edgeMarkerPath() string {
	return filepath.Join(f.edgeStateDir(), edgeToday()+".warned")
}

func edgeReadLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func edgeQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func edgeWriteScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func edgeEnvBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("env")
	if err != nil {
		t.Fatalf("required tool env is not installed: %v", err)
	}
	return p
}

// ---- edge fake: recording gh -------------------------------------------------

func (f *shimFixture) edgeGHPath() string     { return filepath.Join(f.fx, "gh-real") }
func (f *shimFixture) edgeGHRuns() string     { return filepath.Join(f.fx, "edge-gh.runs") }
func (f *shimFixture) edgeGHArgvPath() string { return filepath.Join(f.fx, "edge-gh.argv") }
func (f *shimFixture) edgeGHEnvPath() string  { return filepath.Join(f.fx, "edge-gh.env") }

// edgeInstallGH writes the recording fake gh at SIGIL_GH_REAL's path. On each
// run it appends one line to the runs file naming the exit status it will
// return, writes its argv (one `arg <value>` line each) and its environment,
// prints a line to stdout, and exits with status.
func (f *shimFixture) edgeInstallGH(status int) {
	f.t.Helper()
	body := "echo \"run exit=" + strconv.Itoa(status) + "\" >> " + edgeQuote(f.edgeGHRuns()) + "\n" +
		"for a in \"$@\"; do printf 'arg %s\\n' \"$a\"; done > " + edgeQuote(f.edgeGHArgvPath()) + "\n" +
		edgeQuote(edgeEnvBin(f.t)) + " > " + edgeQuote(f.edgeGHEnvPath()) + "\n" +
		"echo edge-gh-output\nexit " + strconv.Itoa(status) + "\n"
	edgeWriteScript(f.t, f.edgeGHPath(), body)
}

func (f *shimFixture) edgeGHRunCount() int { return len(edgeReadLines(f.edgeGHRuns())) }

func (f *shimFixture) edgeGHArgv() []string {
	var out []string
	for _, l := range edgeReadLines(f.edgeGHArgvPath()) {
		out = append(out, strings.TrimPrefix(l, "arg "))
	}
	return out
}

func edgeParseEnv(lines []string) map[string]string {
	m := map[string]string{}
	for _, l := range lines {
		if i := strings.Index(l, "="); i > 0 {
			m[l[:i]] = l[i+1:]
		}
	}
	return m
}

func (f *shimFixture) edgeGHEnv() map[string]string {
	return edgeParseEnv(edgeReadLines(f.edgeGHEnvPath()))
}

// ---- edge fake: recording sigil-helper ---------------------------------------

func (f *shimFixture) edgeHelperArgvPath() string { return filepath.Join(f.fx, "edge-helper.argv") }
func (f *shimFixture) edgeHelperEnvPath() string  { return filepath.Join(f.fx, "edge-helper.env") }

// edgeInstallHelper writes the recording fake sigil-helper into the fake PATH.
// Each call appends `call` then one `arg <value>` line per argument to the argv
// file and overwrites the env file with its environment, then runs behaviour, a
// shell fragment that sets its output and exit.
func (f *shimFixture) edgeInstallHelper(behaviour string) {
	f.t.Helper()
	body := "{ echo call; for a in \"$@\"; do printf 'arg %s\\n' \"$a\"; done; } >> " + edgeQuote(f.edgeHelperArgvPath()) + "\n" +
		edgeQuote(edgeEnvBin(f.t)) + " > " + edgeQuote(f.edgeHelperEnvPath()) + "\n" + behaviour + "\n"
	edgeWriteScript(f.t, filepath.Join(f.bin, "sigil-helper"), body)
}

// edgeHelperPrints installs a helper that prints stdout (with a trailing
// newline), prints stderr to its stderr when non-empty, and exits with status.
func (f *shimFixture) edgeHelperPrints(stdout, stderr string, status int) {
	f.t.Helper()
	b := "printf '%s\\n' " + edgeQuote(stdout) + "\n"
	if stderr != "" {
		b += "printf '%s\\n' " + edgeQuote(stderr) + " >&2\n"
	}
	f.edgeInstallHelper(b + "exit " + strconv.Itoa(status))
}

// edgeHelperCalls returns the argv of each helper invocation, in order.
func (f *shimFixture) edgeHelperCalls() [][]string {
	var calls [][]string
	for _, l := range edgeReadLines(f.edgeHelperArgvPath()) {
		if l == "call" {
			calls = append(calls, []string{})
			continue
		}
		calls[len(calls)-1] = append(calls[len(calls)-1], strings.TrimPrefix(l, "arg "))
	}
	return calls
}

func (f *shimFixture) edgeHelperEnv() map[string]string {
	return edgeParseEnv(edgeReadLines(f.edgeHelperEnvPath()))
}

func (f *shimFixture) edgeExpectNoHelperCall() {
	f.t.Helper()
	if c := f.edgeHelperCalls(); len(c) != 0 {
		f.t.Errorf("the fake helper must not be called; calls: %q", c)
	}
}

// edgeNewFixture is newShimFixture with the edge fakes in place of the
// implementer's, and, off macOS, the edge Linux lsof reader.
func edgeNewFixture(t *testing.T) *shimFixture {
	t.Helper()
	f := newShimFixture(t)
	f.edgeInstallGH(0)
	f.edgeHelperPrints(f.token, "", 0)
	if runtime.GOOS != "darwin" {
		// R-18's reader on PATH: `lsof -a -p <pid> -d cwd -Fn`, answering from
		// /proc/<pid>/cwd in lsof's field format.
		edgeWriteScript(t, filepath.Join(f.bin, "lsof"),
			"pid=\"$3\"\ncase \"$pid\" in ''|*[!0-9]*) exit 1 ;; esac\n"+
				"d=$(readlink \"/proc/$pid/cwd\") || exit 1\nprintf 'p%s\\nfcwd\\nn%s\\n' \"$pid\" \"$d\"\n")
	}
	return f
}

// ---- edge assertions ---------------------------------------------------------

// edgeAssertClass asserts R-9 and D18 for one failed call of the given class:
// non-zero exit; exactly one stderr line beginning `gh shim: <class>:`; that line
// is the first line beginning `gh shim: `; for every class except mint-failed
// (whose helper stderr passes through first, R-12) stderr begins with it; and
// the fake gh never started.
func (f *shimFixture) edgeAssertClass(res shimResult, class string) {
	f.t.Helper()
	prefix := "gh shim: " + class + ":"
	if res.exit == 0 {
		f.t.Errorf("class %s: expected a non-zero exit, got 0; stderr %q", class, res.stderr)
	}
	var shimLines []string
	n := 0
	for _, l := range strings.Split(res.stderr, "\n") {
		if strings.HasPrefix(l, "gh shim: ") {
			shimLines = append(shimLines, l)
		}
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	if n != 1 {
		f.t.Errorf("class %s: want exactly one stderr line beginning %q, got %d in %q", class, prefix, n, res.stderr)
	}
	if len(shimLines) == 0 || !strings.HasPrefix(shimLines[0], prefix) {
		f.t.Errorf("class %s: the first line beginning `gh shim: ` must be the class line; got %q", class, shimLines)
	}
	if class != "mint-failed" && !strings.HasPrefix(res.stderr, prefix) {
		f.t.Errorf("class %s: stderr must begin %q, got %q", class, prefix, res.stderr)
	}
	if got := f.edgeGHRunCount(); got != 0 {
		f.t.Errorf("class %s: the fake gh must not start on a failed call; it ran %d time(s)", class, got)
	}
}

type edgeRecord struct {
	file                                    string
	raw                                     string
	time, repo, class, outcome, mintSeconds string
}

// edgeReadRecords reads every line of every file in the state directory ending
// .log, with its file name. A line that does not hold five fields keeps raw only.
func (f *shimFixture) edgeReadRecords() []edgeRecord {
	f.t.Helper()
	files, _ := filepath.Glob(filepath.Join(f.edgeStateDir(), "*.log"))
	sort.Strings(files)
	var out []edgeRecord
	for _, p := range files {
		b, _ := os.ReadFile(p)
		if len(b) > 0 && b[len(b)-1] != '\n' {
			f.t.Errorf("record file %s does not end with a newline", p)
		}
		for _, l := range edgeReadLines(p) {
			r := edgeRecord{file: filepath.Base(p), raw: l}
			if parts := strings.Split(l, "\t"); len(parts) == 5 {
				r.time, r.repo, r.class, r.outcome, r.mintSeconds = parts[0], parts[1], parts[2], parts[3], parts[4]
			}
			out = append(out, r)
		}
	}
	return out
}

func (f *shimFixture) edgeExpectNoRecord() {
	f.t.Helper()
	if recs := f.edgeReadRecords(); len(recs) != 0 {
		f.t.Errorf("expected no record line, got %q", recs)
	}
}

// edgeAssertRecord asserts the Data Model's call record line, R-21: exactly one
// line, five tab-separated fields in order, the time ISO 8601 UTC to the second
// with Z (and on the file's UTC date), the repository as given, class narrowed
// exactly when the outcome is minted, the outcome in the closed set and as
// given, and mint_seconds a decimal number for minted, empty for no-repo and
// refused, and empty or a decimal for mint-failed (empty when the shim refused
// before attempting a mint, as on a clock refusal).
func (f *shimFixture) edgeAssertRecord(repo, outcome string) {
	f.t.Helper()
	recs := f.edgeReadRecords()
	if len(recs) != 1 {
		f.t.Fatalf("expected exactly one record line, got %d: %q", len(recs), recs)
	}
	r := recs[0]
	if n := len(strings.Split(r.raw, "\t")); n != 5 {
		f.t.Fatalf("record line has %d tab-separated fields, want 5: %q", n, r.raw)
	}
	m := edgeLogNameRE.FindStringSubmatch(r.file)
	if m == nil {
		f.t.Errorf("record file %q is not <YYYY-MM-DD>.log", r.file)
	}
	if !edgeTimeRE.MatchString(r.time) {
		f.t.Errorf("time %q is not ISO 8601 UTC to the second with Z", r.time)
	} else if m != nil && !strings.HasPrefix(r.time, m[1]) {
		f.t.Errorf("time %q is not on the file's UTC date %s", r.time, m[1])
	}
	if r.repo != repo {
		f.t.Errorf("repo = %q, want %q", r.repo, repo)
	}
	switch r.outcome {
	case "minted", "mint-failed", "no-repo", "refused":
	default:
		f.t.Errorf("outcome %q is outside the closed set", r.outcome)
	}
	if r.outcome != outcome {
		f.t.Errorf("outcome = %q, want %q", r.outcome, outcome)
	}
	wantClass := "none"
	if outcome == "minted" {
		wantClass = "narrowed"
	}
	if r.class != wantClass {
		f.t.Errorf("class = %q, want %q for outcome %q", r.class, wantClass, outcome)
	}
	switch outcome {
	case "minted":
		if !edgeDecimalRE.MatchString(r.mintSeconds) {
			f.t.Errorf("mint_seconds %q should be a non-empty decimal number", r.mintSeconds)
		}
	case "mint-failed":
		if r.mintSeconds != "" && !edgeDecimalRE.MatchString(r.mintSeconds) {
			f.t.Errorf("mint_seconds %q should be empty or a decimal number", r.mintSeconds)
		}
	default:
		if r.mintSeconds != "" {
			f.t.Errorf("mint_seconds %q should be empty where no mint was attempted", r.mintSeconds)
		}
	}
}

// edgeAssertEnv asserts R-13 on the environment the fake gh recorded, and that
// the two variables unset before the helper runs are absent from the helper's
// too.
func (f *shimFixture) edgeAssertEnv(wantToken string) {
	f.t.Helper()
	env := f.edgeGHEnv()
	if len(env) == 0 {
		f.t.Fatalf("the fake gh recorded no environment")
	}
	if env["GH_TOKEN"] != wantToken {
		f.t.Errorf("GH_TOKEN = %q, want the helper's line %q", env["GH_TOKEN"], wantToken)
	}
	for _, name := range []string{"GITHUB_TOKEN", "GH_HOST", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "BASHOPTS", "BASH_XTRACEFD"} {
		if v, ok := env[name]; ok {
			f.t.Errorf("%s must be absent from gh's environment, got %q", name, v)
		}
	}
	for _, name := range []string{"BASHOPTS", "BASH_XTRACEFD"} {
		if v, ok := f.edgeHelperEnv()[name]; ok {
			f.t.Errorf("%s must be absent from the helper's environment, got %q", name, v)
		}
	}
	if want := f.home + "/.config/sigil/gh"; env["GH_CONFIG_DIR"] != want {
		f.t.Errorf("GH_CONFIG_DIR = %q, want %q", env["GH_CONFIG_DIR"], want)
	}
	if want := f.home + "/.local/share/sigil/gh-data"; env["XDG_DATA_HOME"] != want {
		f.t.Errorf("XDG_DATA_HOME = %q, want %q", env["XDG_DATA_HOME"], want)
	}
	if env["GH_PROMPT_DISABLED"] != "1" {
		f.t.Errorf("GH_PROMPT_DISABLED = %q, want 1", env["GH_PROMPT_DISABLED"])
	}
}

func edgeLinesWithPrefix(stderr, prefix string) []string {
	var out []string
	for _, l := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// edgeSeedMinted writes n well-formed `minted` lines into today's record, in
// the Data Model's five-field form.
func (f *shimFixture) edgeSeedMinted(n int) {
	f.t.Helper()
	if err := os.MkdirAll(f.edgeStateDir(), 0o700); err != nil {
		f.t.Fatalf("mkdir record dir: %v", err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(edgeToday() + "T00:00:00Z\tfixowner/fixrepo\tnarrowed\tminted\t1.0\n")
	}
	if err := os.WriteFile(f.edgeLogPath(), []byte(b.String()), 0o600); err != nil {
		f.t.Fatalf("seed record: %v", err)
	}
}

// edgeBlockStateDir occupies the state directory's path with a regular file, so
// the record cannot be written (R-22).
func (f *shimFixture) edgeBlockStateDir() {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.edgeStateDir()), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.edgeStateDir(), []byte("in the way"), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// TP-S0 (R-8): the tracked shim exists and is executable; the tests run it in
// place. Fails, never skips, when it is missing.
func TestShimEdge_S0_TrackedShimExistsAndIsExecutable(t *testing.T) {
	p := shimTrackedPath(t)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("the tracked shim %s is missing: %v", p, err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the tracked shim %s is not executable (mode %v)", p, fi.Mode())
	}
}

// TP-S1 (R-9, R-17): Given an empty HOME, When a call runs, Then class home.
func TestShimEdge_S1_EmptyHomeIsHomeClass(t *testing.T) {
	f := edgeNewFixture(t)
	f.setEnv("HOME", "")
	res := f.run("api", "user")
	f.edgeAssertClass(res, "home")
	f.edgeExpectNoHelperCall()
	f.edgeExpectNoRecord()
}

// TP-S2 (R-9, R-10): Given HOME and no real-gh path, Then class real-gh.
func TestShimEdge_S2_NoRealGHPathIsRealGHClass(t *testing.T) {
	f := edgeNewFixture(t)
	f.unsetEnv("SIGIL_GH_REAL")
	res := f.run("api", "user")
	f.edgeAssertClass(res, "real-gh")
	f.edgeExpectNoHelperCall()
}

// TP-S3 (R-9, R-39): Given the override names the shim itself, Then real-gh.
func TestShimEdge_S3_RealGHIsTheShimItself(t *testing.T) {
	f := edgeNewFixture(t)
	f.setEnv("SIGIL_GH_REAL", f.shim)
	res := f.run("api", "user")
	f.edgeAssertClass(res, "real-gh")
	f.edgeExpectNoHelperCall()
}

// TP-S4 (R-9, R-18): Given a harness process outside any clone, Then no-repo,
// and the record's one line has outcome no-repo and an empty repository.
func TestShimEdge_S4_OutsideAClone_NoRepo(t *testing.T) {
	f := edgeNewFixture(t)
	f.startHarness(f.callDir)
	res := f.run("api", "user")
	f.edgeAssertClass(res, "no-repo")
	f.edgeExpectNoHelperCall()
	f.edgeAssertRecord("", "no-repo")
}

// TP-S5 (R-9, R-11): Given a resolving clone, `gh auth setup-git` is refused.
func TestShimEdge_S5_AuthSubcommandRefused(t *testing.T) {
	f := edgeNewFixture(t)
	res := f.run("auth", "setup-git")
	f.edgeAssertClass(res, "refused")
	f.edgeExpectNoHelperCall()
	f.edgeAssertRecord("fixowner/fixrepo", "refused")
}

// TP-S6 (R-9, R-12): Given a helper that prints a token and exits non-zero,
// Then mint-failed, the helper's stderr is passed through, and the helper's
// argv was exactly `token --landing <owner/repo>`.
func TestShimEdge_S6_HelperFailsAfterPrintingAToken_MintFailed(t *testing.T) {
	f := edgeNewFixture(t)
	complaint := "helper-complaint-" + f.token[:6]
	f.edgeHelperPrints(f.token, complaint, 3)
	res := f.run("api", "user")
	f.edgeAssertClass(res, "mint-failed")
	if !strings.Contains(res.stderr, complaint) {
		t.Errorf("the helper's stderr was not passed through: %q", res.stderr)
	}
	calls := f.edgeHelperCalls()
	want := []string{"token", "--landing", "fixowner/fixrepo"}
	if len(calls) != 1 || strings.Join(calls[0], "\x00") != strings.Join(want, "\x00") {
		t.Errorf("helper argv = %q, want exactly %q", calls, want)
	}
	f.edgeAssertRecord("fixowner/fixrepo", "mint-failed")
	if strings.Contains(res.stdout, f.token) || strings.Contains(res.stderr, f.token) {
		t.Errorf("the rejected token reached the shim's output")
	}
}

// TP-S7 (R-9): step order, real-gh before no-repo.
func TestShimEdge_S7_RealGHCheckRunsBeforeTheRepoCheck(t *testing.T) {
	f := edgeNewFixture(t)
	f.unsetEnv("SIGIL_GH_REAL")
	f.startHarness(f.callDir)
	res := f.run("api", "user")
	f.edgeAssertClass(res, "real-gh")
}

// TP-S8 (R-9): step order, no-repo before refused.
func TestShimEdge_S8_RepoCheckRunsBeforeTheSubcommandRefusal(t *testing.T) {
	f := edgeNewFixture(t)
	f.startHarness(f.callDir)
	res := f.run("auth", "token")
	f.edgeAssertClass(res, "no-repo")
	f.edgeAssertRecord("", "no-repo")
}

// TP-S9 (R-9): step order, home before real-gh.
func TestShimEdge_S9_HomeCheckRunsBeforeTheRealGHCheck(t *testing.T) {
	f := edgeNewFixture(t)
	f.setEnv("HOME", "")
	f.unsetEnv("SIGIL_GH_REAL")
	res := f.run("api", "user")
	f.edgeAssertClass(res, "home")
}

// TP-S10 (R-13): the fake gh's environment after a successful call.
func TestShimEdge_S10_EnvironmentHandedToTheRealGH(t *testing.T) {
	f := edgeNewFixture(t)
	other := shimRandomToken(t)
	f.setEnv("GITHUB_TOKEN", other)
	f.setEnv("GH_HOST", "example.invalid")
	f.setEnv("GH_ENTERPRISE_TOKEN", other)
	f.setEnv("GITHUB_ENTERPRISE_TOKEN", other)
	f.setEnv("GH_TOKEN", other)
	f.setEnv("GH_CONFIG_DIR", filepath.Join(f.root, "other-config"))
	f.setEnv("XDG_DATA_HOME", filepath.Join(f.root, "other-data"))
	f.setEnv("GH_PROMPT_DISABLED", "0")
	f.setEnv("BASHOPTS", "extglob")
	f.setEnv("BASH_XTRACEFD", "7")
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr %q", res.exit, res.stderr)
	}
	f.edgeAssertEnv(f.token)
}

// TP-S11 (R-15): argv forwarded unchanged, exit status equal to the fake's.
func TestShimEdge_S11_ArgvForwardedAndExitStatusEqualsTheRealGHs(t *testing.T) {
	f := edgeNewFixture(t)
	f.edgeInstallGH(7)
	argv := []string{"pr", "view", "-R", "o/other", "--repo=o/other", "https://github.com/o/other/pull/1", "repos/o/other/pulls", "--json", "a b"}
	res := f.run(argv...)
	if res.exit != 7 {
		t.Errorf("exit = %d, want the fake gh's 7; stderr %q", res.exit, res.stderr)
	}
	if got := f.edgeGHArgv(); strings.Join(got, "\x00") != strings.Join(argv, "\x00") {
		t.Errorf("fake gh argv = %q, want %q", got, argv)
	}
}

// edgeFileHolds reports whether the file at path holds needle.
func edgeFileHolds(path, needle string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), needle)
}

// TP-S12 (R-14): the token is in neither fake's argv, not in the shim's own
// output, and in no file under the test root. Control: the same scan finds a
// planted token.
func TestShimEdge_S12_TokenLeavesOnlyThroughTheEnvironment(t *testing.T) {
	f := edgeNewFixture(t)
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit = %d, stderr %q", res.exit, res.stderr)
	}
	if got := f.edgeGHEnv()["GH_TOKEN"]; got != f.token {
		t.Fatalf("precondition: the fake gh's GH_TOKEN = %q, want the helper's line", got)
	}
	for _, p := range []string{f.edgeGHArgvPath(), f.edgeHelperArgvPath()} {
		if edgeFileHolds(p, f.token) {
			t.Errorf("the token is in an argv record: %s", p)
		}
	}
	if len(f.edgeGHArgv()) != 2 {
		t.Errorf("precondition: fake gh argv should be the call's two arguments, got %q", f.edgeGHArgv())
	}
	if strings.Contains(res.stdout, f.token) || strings.Contains(res.stderr, f.token) {
		t.Errorf("the token is in the shim's own output: stdout %q stderr %q", res.stdout, res.stderr)
	}
	if hits := shimScanTree(t, f.root, f.token); len(hits) != 0 {
		t.Errorf("files under the test root hold the token: %v", hits)
	}
	// Control: the scan finds a token planted under TMPDIR.
	planted := shimRandomToken(t)
	if err := os.WriteFile(filepath.Join(f.tmp, "planted"), []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := shimScanTree(t, f.root, planted); len(hits) != 1 {
		t.Errorf("control: the scan should find the planted token once, got %v", hits)
	}
}

// TP-S13 (R-16): two consecutive calls run the helper twice with two tokens;
// neither is under the test root afterwards. Control as TP-S12.
func TestShimEdge_S13_EachCallMintsItsOwnToken(t *testing.T) {
	f := edgeNewFixture(t)
	tokFile := filepath.Join(f.fx, "edge-tokens")
	prefix := shimRandomToken(t)
	f.edgeInstallHelper(fmt.Sprintf("t=%s$$\nprintf '%%s\\n' \"$t\" >> %s\nprintf '%%s\\n' \"$t\"", prefix, edgeQuote(tokFile)))
	for i := 0; i < 2; i++ {
		if res := f.run("api", "user"); res.exit != 0 {
			t.Fatalf("call %d: exit %d, stderr %q", i, res.exit, res.stderr)
		}
	}
	if n := len(f.edgeHelperCalls()); n != 2 {
		t.Fatalf("the helper ran %d times, want 2", n)
	}
	toks := edgeReadLines(tokFile)
	if len(toks) != 2 || toks[0] == toks[1] {
		t.Fatalf("expected two different tokens, got %q", toks)
	}
	for _, tok := range toks {
		if hits := shimScanTree(t, f.root, tok); len(hits) != 0 {
			t.Errorf("files under the test root hold a token: %v", hits)
		}
	}
	planted := toks[0]
	if err := os.WriteFile(filepath.Join(f.tmp, "planted"), []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := shimScanTree(t, f.root, planted); len(hits) != 1 {
		t.Errorf("control: the scan should find the planted token once, got %v", hits)
	}
}

// TP-S14 (R-19): the shim never reads a repository from the call.
func TestShimEdge_S14_RepositoryIsNeverReadFromTheCall(t *testing.T) {
	f := edgeNewFixture(t)
	other := f.newClone("other-clone", "https://github.com/otherowner/otherrepo")
	cases := []struct {
		name string
		dir  string
		env  map[string]string
		argv []string
	}{
		{"GH_REPO", f.callDir, map[string]string{"GH_REPO": "other/x"}, []string{"issue", "list"}},
		{"another clone's directory", other, nil, []string{"issue", "list"}},
		{"-R", f.callDir, nil, []string{"issue", "list", "-R", "other/x"}},
	}
	for i, c := range cases {
		f.callDir = c.dir
		for k, v := range c.env {
			f.setEnv(k, v)
		}
		res := f.run(c.argv...)
		if res.exit != 0 {
			t.Fatalf("%s: exit %d, stderr %q", c.name, res.exit, res.stderr)
		}
		calls := f.edgeHelperCalls()
		if len(calls) != i+1 {
			t.Fatalf("%s: helper ran %d times, want %d", c.name, len(calls), i+1)
		}
		if got := strings.Join(calls[i], " "); got != "token --landing fixowner/fixrepo" {
			t.Errorf("%s: helper argv = %q, want the session's fixowner/fixrepo", c.name, got)
		}
		if got := f.edgeGHArgv(); strings.Join(got, "\x00") != strings.Join(c.argv, "\x00") {
			t.Errorf("%s: fake gh argv = %q, want %q", c.name, got, c.argv)
		}
		for k := range c.env {
			f.unsetEnv(k)
		}
	}
}

// TP-S15 (R-21): a minted call.
func TestShimEdge_S15_RecordLine_Minted(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	res := f.run("api", "user")
	edgeSameDay(t, day)
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr %q", res.exit, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(f.edgeStateDir(), day+".log")); err != nil {
		t.Fatalf("the record file for UTC %s is missing: %v", day, err)
	}
	f.edgeAssertRecord("fixowner/fixrepo", "minted")
}

// TP-S16 (R-21): a mint-failed call.
func TestShimEdge_S16_RecordLine_MintFailed(t *testing.T) {
	f := edgeNewFixture(t)
	f.edgeHelperPrints("", "", 1)
	f.run("api", "user")
	f.edgeAssertRecord("fixowner/fixrepo", "mint-failed")
}

// TP-S17 (R-21): a no-repo call.
func TestShimEdge_S17_RecordLine_NoRepo(t *testing.T) {
	f := edgeNewFixture(t)
	f.startHarness(f.callDir)
	f.run("api", "user")
	f.edgeAssertRecord("", "no-repo")
}

// TP-S18 (R-21): a refused call.
func TestShimEdge_S18_RecordLine_Refused(t *testing.T) {
	f := edgeNewFixture(t)
	f.run("extension", "install", "o/r")
	f.edgeAssertRecord("fixowner/fixrepo", "refused")
}

// TP-S19 (R-21): a pre-existing 0755 directory and 0644 file read back 0700
// and 0600.
func TestShimEdge_S19_RecordModesAreReappliedAfterACall(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	if err := os.MkdirAll(f.edgeStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.edgeStateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	logp := filepath.Join(f.edgeStateDir(), day+".log")
	if err := os.WriteFile(logp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(logp, 0o644); err != nil {
		t.Fatal(err)
	}
	res := f.run("api", "user")
	edgeSameDay(t, day)
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr %q", res.exit, res.stderr)
	}
	di, err := os.Stat(f.edgeStateDir())
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(logp)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("state directory mode = %v, want 0700", di.Mode().Perm())
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("record file mode = %v, want 0600", fi.Mode().Perm())
	}
}

// TP-S20 (R-22): with the state directory's path occupied by a regular file,
// the fake gh still runs and stderr holds exactly one `gh shim: record:` line,
// without the token.
func TestShimEdge_S20_UnwritableRecordWarnsOnceAndContinues(t *testing.T) {
	f := edgeNewFixture(t)
	f.edgeBlockStateDir()
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Errorf("exit = %d, want 0 (the fake gh exits 0); stderr %q", res.exit, res.stderr)
	}
	if got := f.edgeGHRunCount(); got != 1 {
		t.Errorf("the fake gh ran %d times, want 1", got)
	}
	if lines := edgeLinesWithPrefix(res.stderr, "gh shim: record:"); len(lines) != 1 {
		t.Fatalf("want exactly one stderr line beginning `gh shim: record:`, got %q", res.stderr)
	}
	if strings.Contains(res.stderr, f.token) || strings.Contains(res.stdout, f.token) {
		t.Errorf("the token is in the shim's output")
	}
}

// TP-S21 (R-23, R-24): today's record seeded with 181 minted lines; a minting
// call (N = 182) prints one volume warning holding the count, both constants
// and the word maintainer, and writes one marker.
func TestShimEdge_S21_VolumeWarningAtTheLine(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	f.edgeSeedMinted(181)
	res := f.run("api", "user")
	edgeSameDay(t, day)
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr %q", res.exit, res.stderr)
	}
	lines := edgeLinesWithPrefix(res.stderr, "gh shim: volume warning:")
	if len(lines) != 1 {
		t.Fatalf("want exactly one `gh shim: volume warning:` line, got %q", res.stderr)
	}
	w := lines[0]
	if !edgeCountRE.MatchString(w) {
		t.Errorf("warning must state today's minted count, 182: %q", w)
	}
	if !edgeLimitRE.MatchString(w) {
		t.Errorf("warning must state the assumed daily limit, 1000 (or 1,000): %q", w)
	}
	if !edgeReadsRE.MatchString(w) {
		t.Errorf("warning must state reads-per-mint, 3.0 (or 3): %q", w)
	}
	if !strings.Contains(w, "maintainer") {
		t.Errorf("warning must tell the session to report it to the maintainer: %q", w)
	}
	if strings.Contains(res.stderr, f.token) {
		t.Errorf("the token is in the warning's stderr")
	}
	mlines := edgeReadLines(f.edgeMarkerPath())
	if len(mlines) != 1 {
		t.Fatalf("marker %s should hold one line, got %q", f.edgeMarkerPath(), mlines)
	}
	fields := strings.Split(mlines[0], "\t")
	if len(fields) != 2 || fields[0] == "" || fields[1] != "182" {
		t.Errorf("marker line = %q, want <creation time> TAB 182", mlines[0])
	}
}

// TP-S22 (R-23): a second minting call the same UTC day prints no warning.
func TestShimEdge_S22_NoSecondWarningTheSameDay(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	f.edgeSeedMinted(181)
	first := f.run("api", "user")
	if len(edgeLinesWithPrefix(first.stderr, "gh shim: volume warning:")) != 1 {
		t.Fatalf("precondition: the first call should warn once, stderr %q", first.stderr)
	}
	second := f.run("api", "user")
	edgeSameDay(t, day)
	if second.exit != 0 {
		t.Fatalf("second call exit %d, stderr %q", second.exit, second.stderr)
	}
	if strings.Contains(second.stderr, "volume warning") {
		t.Errorf("the second call that day printed a warning: %q", second.stderr)
	}
}

// TP-S23 (R-23): with 180 seeded lines, N = 181 prints no warning and writes no
// marker. The control for TP-S21.
func TestShimEdge_S23_NoWarningBelowTheLine(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	f.edgeSeedMinted(180)
	res := f.run("api", "user")
	edgeSameDay(t, day)
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr %q", res.exit, res.stderr)
	}
	if strings.Contains(res.stderr, "gh shim: volume") {
		t.Errorf("no volume line expected at N = 181: %q", res.stderr)
	}
	if _, err := os.Stat(f.edgeMarkerPath()); err == nil {
		t.Errorf("no marker should exist at N = 181")
	}
}

// TP-S24 (R-23): with the marker's path occupied by a directory and the record
// past the line, a minting call prints exactly one `gh shim: volume:` line and
// the fake gh still runs.
func TestShimEdge_S24_UncreatableMarkerPrintsVolumeLineAndContinues(t *testing.T) {
	f := edgeNewFixture(t)
	day := edgeToday()
	f.edgeSeedMinted(181)
	if err := os.Mkdir(f.edgeMarkerPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	res := f.run("api", "user")
	edgeSameDay(t, day)
	if n := len(edgeLinesWithPrefix(res.stderr, "gh shim: volume:")); n != 1 {
		t.Errorf("want exactly one stderr line beginning `gh shim: volume:`, got %d in %q", n, res.stderr)
	}
	if got := f.edgeGHRunCount(); got != 1 {
		t.Errorf("the fake gh ran %d times, want 1", got)
	}
	if res.exit != 0 {
		t.Errorf("exit = %d, want 0", res.exit)
	}
}

// TP-S25 (R-9, R-22, D18): a refused call whose record also fails prints the
// class line first; the `gh shim: record:` line may follow, never precede it.
func TestShimEdge_S25_ClassLinePrecedesRecordLine_Refused(t *testing.T) {
	f := edgeNewFixture(t)
	f.edgeBlockStateDir()
	res := f.run("auth", "setup-git")
	f.edgeAssertClass(res, "refused")
	if n := len(edgeLinesWithPrefix(res.stderr, "gh shim: record:")); n != 1 {
		t.Errorf("precondition: the unwritable record should print one `gh shim: record:` line, got %d in %q", n, res.stderr)
	}
}

// TP-S26 (R-9, R-12, R-22, D18): a mint-failed call whose record also fails: the
// helper's stderr may come first, but the first `gh shim: ` line is the class line.
func TestShimEdge_S26_ClassLinePrecedesRecordLine_MintFailed(t *testing.T) {
	f := edgeNewFixture(t)
	f.edgeBlockStateDir()
	f.edgeHelperPrints("", "helper-complaint", 1)
	res := f.run("api", "user")
	f.edgeAssertClass(res, "mint-failed")
	if n := len(edgeLinesWithPrefix(res.stderr, "gh shim: record:")); n != 1 {
		t.Errorf("precondition: the unwritable record should print one `gh shim: record:` line, got %d in %q", n, res.stderr)
	}
}

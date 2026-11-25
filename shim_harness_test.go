package main_test

// Harness for the tests of the tracked gh shim (shim/gh).
//
// The shim is a POSIX shell script run in place, as a process, so these tests
// are black-box: they drive argv, environment and the process table, and read
// the exit status, stdout, stderr and the files the shim writes. Nothing here
// touches package main.
//
// Two temporary roots per test, deliberately:
//
//	root  holds what the shim may touch: HOME, TMPDIR, the fixture clones. The
//	      R-14 and R-16 scans walk it and must find no token.
//	fx    holds the apparatus: the fake sigil-helper (its script holds the token
//	      it prints), the recording fake gh (its dump holds GH_TOKEN) and the
//	      PATH wrappers. It is never scanned, because it holds the token by
//	      design.
//
// The shim's PATH is built from scratch (wrappers for the tools it needs, the
// fakes first), and its environment is built from scratch, so the developer's
// real sigil-helper, SHELLOPTS and GIT_ variables never reach it.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const shimRel = "shim/gh"

// shimTools are the base tools the shim and the fakes may use. Each gets a
// wrapper in the fake PATH; the first seven are required (readlink is the Linux fake lsof's reader), the rest are wrapped
// when present so later steps of the shim can use them.
var shimRequiredTools = []string{"awk", "grep", "date", "mkdir", "chmod", "git", "readlink"}
var shimOptionalTools = []string{"sed", "cat", "touch", "ln", "mv", "rm", "tr", "wc", "sort", "head", "dd", "uniq"}

type shimResult struct {
	exit   int
	stdout string
	stderr string
}

type shimFixture struct {
	t       *testing.T
	root    string
	fx      string
	home    string
	tmp     string
	bin     string
	callDir string
	clone   string
	token   string
	shim    string
	env     map[string]string
	unset   map[string]bool
	harness *exec.Cmd
}

func shimTrackedPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(shimRel)
	if err != nil {
		t.Fatalf("resolve %s: %v", shimRel, err)
	}
	return p
}

// shimRandomToken returns a per-run fixture token. It is not ghs_-shaped and is
// never written into this source, so no scanner rule matches it.
func shimRandomToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random token: %v", err)
	}
	return "fx" + hex.EncodeToString(b)
}

func shimWriteExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func shimMustLook(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("required tool %q is not installed: %v", name, err)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("resolve %s: %v", p, err)
	}
	return abs
}

func shimGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitBin := shimMustLook(t, "git")
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	// Scrubbed environment: no inherited GIT_ variable, no global configuration.
	cmd.Env = []string{"PATH=" + filepath.Dir(gitBin) + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func newShimFixture(t *testing.T) *shimFixture {
	t.Helper()
	f := &shimFixture{t: t, root: t.TempDir(), fx: t.TempDir(), env: map[string]string{}, unset: map[string]bool{}}
	f.home = filepath.Join(f.root, "home")
	f.tmp = filepath.Join(f.root, "tmp")
	f.callDir = filepath.Join(f.root, "elsewhere")
	f.bin = filepath.Join(f.fx, "bin")
	for _, d := range []string{f.home, f.tmp, f.callDir, f.bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	f.token = shimRandomToken(t)
	f.shim = shimTrackedPath(t)

	for _, name := range shimRequiredTools {
		f.wrapTool(name, true)
	}
	for _, name := range shimOptionalTools {
		f.wrapTool(name, false)
	}
	if runtime.GOOS == "darwin" {
		// The real lsof is the reader on macOS; its absence fails, never skips.
		f.wrapTool("lsof", true)
	} else {
		f.writeProcLsof()
	}

	f.setHelper(fmt.Sprintf("printf '%%s\\n' %s", f.token))
	f.setGH(0)
	f.env["SIGIL_GH_REAL"] = filepath.Join(f.fx, "gh-real")

	f.clone = f.newClone("clone", "https://github.com/fixowner/fixrepo")
	f.startHarness(f.clone)

	f.env["HOME"] = f.home
	f.env["PATH"] = f.bin
	f.env["TMPDIR"] = f.tmp
	return f
}

func (f *shimFixture) wrapTool(name string, required bool) {
	f.t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		if required {
			f.t.Fatalf("required tool %q is not installed: %v", name, err)
		}
		return
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		f.t.Fatalf("resolve %s: %v", p, err)
	}
	shimWriteExec(f.t, filepath.Join(f.bin, name), "exec '"+abs+"' \"$@\"\n")
}

// writeProcLsof is the Linux reader: it answers lsof -a -p PID -d cwd -Fn from
// /proc, in lsof's field format.
func (f *shimFixture) writeProcLsof() { f.writeProcLsofAt("/proc") }

// writeProcLsofAt is writeProcLsof with the proc root injectable, so the fake's
// own argv handling can be tested on a platform with no /proc.
func (f *shimFixture) writeProcLsofAt(procRoot string) {
	f.t.Helper()
	shimWriteExec(f.t, filepath.Join(f.bin, "lsof"),
		// Exactly the shim's argv or nothing: a change to the shim's flags must
		// fail here, on Linux, as it would against the real lsof's contract.
		"[ \"$#\" -eq 6 ] && [ \"$1\" = -a ] && [ \"$2\" = -p ] && [ \"$4\" = -d ] && [ \"$5\" = cwd ] && [ \"$6\" = -Fn ] || exit 1\n"+
			"pid=$3\ncase $pid in ''|*[!0123456789]*) exit 1 ;; esac\n"+
			"d=$(readlink \""+procRoot+"/$pid/cwd\") || exit 1\nprintf 'p%s\\nfcwd\\nn%s\\n' \"$pid\" \"$d\"\n")
}

func (f *shimFixture) setLsof(body string) {
	f.t.Helper()
	shimWriteExec(f.t, filepath.Join(f.bin, "lsof"), body)
}

func (f *shimFixture) removeLsof() {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.bin, "lsof")); err != nil {
		f.t.Fatalf("remove fake lsof: %v", err)
	}
}

func (f *shimFixture) helperArgsFile() string { return filepath.Join(f.fx, "helper.args") }
func (f *shimFixture) helperEnvFile() string  { return filepath.Join(f.fx, "helper.env") }
func (f *shimFixture) ghArgvFile() string     { return filepath.Join(f.fx, "gh.argv") }
func (f *shimFixture) ghEnvFile() string      { return filepath.Join(f.fx, "gh.env") }
func (f *shimFixture) ghCountFile() string    { return filepath.Join(f.fx, "gh.count") }

// setHelper installs the fake sigil-helper. It records its argv and its
// environment, then runs behaviour (a shell fragment that prints the token or
// fails).
func (f *shimFixture) setHelper(behaviour string) {
	f.t.Helper()
	envBin := shimMustLook(f.t, "env")
	body := "{ for a in \"$@\"; do printf '%s\\n' \"$a\"; done; echo --; } >> '" + f.helperArgsFile() + "'\n" +
		"'" + envBin + "' > '" + f.helperEnvFile() + "'\n" + behaviour + "\n"
	shimWriteExec(f.t, filepath.Join(f.bin, "sigil-helper"), body)
}

// setGH installs the recording fake real gh, which exits with status code.
func (f *shimFixture) setGH(code int) {
	f.t.Helper()
	envBin := shimMustLook(f.t, "env")
	body := "echo run >> '" + f.ghCountFile() + "'\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + f.ghArgvFile() + "'\n" +
		"'" + envBin + "' > '" + f.ghEnvFile() + "'\n" +
		"echo gh-output\nexit " + strconv.Itoa(code) + "\n"
	shimWriteExec(f.t, filepath.Join(f.fx, "gh-real"), body)
}

func shimReadLines(path string) []string {
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

// helperCalls returns the argv of each fake-helper invocation, in order.
func (f *shimFixture) helperCalls() [][]string {
	var calls [][]string
	var cur []string
	for _, l := range shimReadLines(f.helperArgsFile()) {
		if l == "--" {
			calls = append(calls, cur)
			cur = nil
			continue
		}
		cur = append(cur, l)
	}
	return calls
}

func (f *shimFixture) ghRuns() int              { return len(shimReadLines(f.ghCountFile())) }
func (f *shimFixture) ghArgv() []string         { return shimReadLines(f.ghArgvFile()) }
func (f *shimFixture) ghEnv() map[string]string { return shimParseEnv(shimReadLines(f.ghEnvFile())) }
func (f *shimFixture) helperEnv() map[string]string {
	return shimParseEnv(shimReadLines(f.helperEnvFile()))
}

func shimParseEnv(lines []string) map[string]string {
	m := map[string]string{}
	for _, l := range lines {
		if i := strings.Index(l, "="); i > 0 {
			m[l[:i]] = l[i+1:]
		}
	}
	return m
}

// newClone makes a fixture clone under root with the given origin (empty: none).
func (f *shimFixture) newClone(name, origin string) string {
	f.t.Helper()
	dir := filepath.Join(f.root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatalf("mkdir %s: %v", dir, err)
	}
	shimGitRun(f.t, dir, "init", "-q")
	if origin != "" {
		shimGitRun(f.t, dir, "remote", "add", "origin", origin)
	}
	return dir
}

// startHarness starts the fake harness process with dir as its working
// directory and points CLAUDE_PID at it. Any earlier one is stopped.
func (f *shimFixture) startHarness(dir string) {
	f.t.Helper()
	if f.harness != nil {
		_ = f.harness.Process.Kill()
		_ = f.harness.Wait()
	}
	cmd := exec.Command(shimMustLook(f.t, "sleep"), "600")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if err := cmd.Start(); err != nil {
		f.t.Fatalf("start fake harness process: %v", err)
	}
	f.harness = cmd
	f.t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	f.env["CLAUDE_PID"] = strconv.Itoa(cmd.Process.Pid)
}

// deadPID returns the pid of a process that has exited.
func (f *shimFixture) deadPID() string {
	f.t.Helper()
	cmd := exec.Command(shimMustLook(f.t, "sleep"), "0")
	if err := cmd.Run(); err != nil {
		f.t.Fatalf("run short process: %v", err)
	}
	return strconv.Itoa(cmd.Process.Pid)
}

func (f *shimFixture) environ() []string {
	var keys []string
	for k := range f.env {
		if !f.unset[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+f.env[k])
	}
	return out
}

func (f *shimFixture) setEnv(k, v string) { f.env[k] = v; delete(f.unset, k) }
func (f *shimFixture) unsetEnv(k string)  { f.unset[k] = true }

func (f *shimFixture) runErr(args ...string) (shimResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.shim, args...)
	cmd.Dir = f.callDir
	cmd.Env = f.environ()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := shimResult{stdout: out.String(), stderr: errb.String()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.exit = ee.ExitCode()
		return res, nil
	}
	return res, err
}

func (f *shimFixture) run(args ...string) shimResult {
	f.t.Helper()
	res, err := f.runErr(args...)
	if err != nil {
		f.t.Fatalf("run %s: %v", f.shim, err)
	}
	return res
}

// expectRefused asserts a failed call of the given class: non-zero exit,
// stderr beginning with one `gh shim: <class>:` line, and no gh started.
func (f *shimFixture) expectRefused(res shimResult, class string) {
	f.t.Helper()
	prefix := "gh shim: " + class + ":"
	if res.exit == 0 {
		f.t.Errorf("expected a non-zero exit, got 0; stderr=%q", res.stderr)
	}
	// A mint-failed call passes the helper's own stderr through ahead of the
	// shim's line (R-12, D18), so only the other classes pin the line to the
	// start of stderr. Every class pins the first `gh shim: ` line to the class
	// line: a record or volume line ahead of it is a defect.
	if class != "mint-failed" && !strings.HasPrefix(res.stderr, prefix) {
		f.t.Errorf("stderr should begin %q, got %q", prefix, res.stderr)
	}
	for _, l := range strings.Split(res.stderr, "\n") {
		if strings.HasPrefix(l, "gh shim: ") {
			if !strings.HasPrefix(l, prefix) {
				f.t.Errorf("the first `gh shim: ` line should be the %q class line, got %q", class, l)
			}
			break
		}
	}
	n := 0
	for _, l := range strings.Split(res.stderr, "\n") {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	if n != 1 {
		f.t.Errorf("expected exactly one stderr line beginning %q, got %d in %q", prefix, n, res.stderr)
	}
	if got := f.ghRuns(); got != 0 {
		f.t.Errorf("the fake gh must not start on a failed call; it ran %d time(s)", got)
	}
}

// shimRecordLine is one parsed line of the call record.
type shimRecordLine struct {
	time, repo, class, outcome, mintSeconds string
	raw                                     string
}

func (f *shimFixture) recordDir() string {
	return filepath.Join(f.home, ".local", "state", "sigil", "gh-shim")
}

func (f *shimFixture) records() []shimRecordLine {
	f.t.Helper()
	files, _ := filepath.Glob(filepath.Join(f.recordDir(), "*.log"))
	sort.Strings(files)
	var out []shimRecordLine
	for _, p := range files {
		for _, l := range shimReadLines(p) {
			parts := strings.Split(l, "\t")
			r := shimRecordLine{raw: l}
			if len(parts) == 5 {
				r.time, r.repo, r.class, r.outcome, r.mintSeconds = parts[0], parts[1], parts[2], parts[3], parts[4]
			}
			out = append(out, r)
		}
	}
	return out
}

// expectOneRecord asserts the call record holds exactly one line with this
// outcome and repository.
func (f *shimFixture) expectOneRecord(outcome, repo string) shimRecordLine {
	f.t.Helper()
	recs := f.records()
	if len(recs) != 1 {
		f.t.Fatalf("expected one record line, got %d: %q", len(recs), recs)
	}
	r := recs[0]
	if r.outcome != outcome || r.repo != repo {
		f.t.Errorf("record outcome/repo = %q/%q, want %q/%q (line %q)", r.outcome, r.repo, outcome, repo, r.raw)
	}
	return r
}

func (f *shimFixture) expectNoRecord() {
	f.t.Helper()
	if recs := f.records(); len(recs) != 0 {
		f.t.Errorf("expected no record line, got %q", recs)
	}
}

func (f *shimFixture) expectNoHelperCall() {
	f.t.Helper()
	if c := f.helperCalls(); len(c) != 0 {
		f.t.Errorf("the fake helper must not be called; calls: %q", c)
	}
}

// scanTree returns the files under root that hold needle.
func shimScanTree(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if bytes.Contains(b, []byte(needle)) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan %s: %v", root, err)
	}
	return hits
}

// copyOf writes text as an executable shim copy in the apparatus root and
// returns its path. Controls run these in place of the tracked script.
func (f *shimFixture) copyOf(name, text string) string {
	f.t.Helper()
	p := filepath.Join(f.fx, "copies", name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatalf("mkdir copies: %v", err)
	}
	if err := os.WriteFile(p, []byte(text), 0o755); err != nil {
		f.t.Fatalf("write copy: %v", err)
	}
	return p
}

// shimTrackedText reads the tracked shim.
func shimTrackedText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(shimTrackedPath(t))
	if err != nil {
		t.Fatalf("read tracked shim: %v", err)
	}
	return string(b)
}

// shimEdit applies each replacement once. A replacement whose anchor is absent
// fails the test: a control that mutated nothing would pass for the wrong
// reason.
func shimEdit(t *testing.T, text string, edits ...[2]string) string {
	t.Helper()
	for _, e := range edits {
		if !strings.Contains(text, e[0]) {
			t.Fatalf("seed anchor not found in the tracked shim: %q", e[0])
		}
		text = strings.Replace(text, e[0], e[1], 1)
	}
	return text
}

func (f *shimFixture) editedCopy(name string, edits ...[2]string) string {
	f.t.Helper()
	return f.copyOf(name, shimEdit(f.t, shimTrackedText(f.t), edits...))
}

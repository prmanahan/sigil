package main_test

// Tests of the harness's Linux reader, the fake lsof that answers from /proc.
// CI's ubuntu job is the only place the real /proc is read; these run the same
// fake on any platform against a /proc-like fixture directory, so a mistake in
// its argv handling shows on macOS too.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// procFixture makes a /proc-like root in which pid's cwd is a symlink to dir.
func procFixture(t *testing.T, pid, dir string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, pid), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, filepath.Join(root, pid, "cwd")); err != nil {
		t.Fatal(err)
	}
	return root
}

func (f *shimFixture) runFakeLsof(args ...string) (string, error) {
	f.t.Helper()
	cmd := exec.Command(filepath.Join(f.bin, "lsof"), args...)
	cmd.Env = []string{"PATH=" + f.bin}
	out, err := cmd.Output()
	return string(out), err
}

// The fake answers `lsof -a -p <pid> -d cwd -Fn` with the pid from the -p
// argument, in lsof's field format.
func TestShimHarness_ProcLsof_AnswersForThePidInTheFlagArgument(t *testing.T) {
	f := newShimFixture(t)
	pid := strconv.Itoa(40000 + os.Getpid()%1000)
	f.writeProcLsofAt(procFixture(t, pid, f.clone))
	out, err := f.runFakeLsof("-a", "-p", pid, "-d", "cwd", "-Fn")
	if err != nil {
		t.Fatalf("the fake failed on the shim's exact argv: %v", err)
	}
	if want := "p" + pid + "\nfcwd\nn" + f.clone + "\n"; out != want {
		t.Errorf("fake lsof output = %q, want %q", out, want)
	}
}

// The fake exits non-zero on any argv other than exactly
// `-a -p <pid> -d cwd -Fn`, so a change to the shim's flags fails on Linux.
func TestShimHarness_ProcLsof_RejectsAnyOtherArgv(t *testing.T) {
	f := newShimFixture(t)
	pid := strconv.Itoa(40000 + os.Getpid()%1000)
	f.writeProcLsofAt(procFixture(t, pid, f.clone))
	bad := map[string][]string{
		"no arguments":        {},
		"no -a":               {"-p", pid, "-d", "cwd", "-Fn"},
		"-p after -d":         {"-a", "-d", "cwd", "-p", pid, "-Fn"},
		"no -d cwd":           {"-a", "-p", pid, "-Fn"},
		"a different fd":      {"-a", "-p", pid, "-d", "txt", "-Fn"},
		"no -Fn":              {"-a", "-p", pid, "-d", "cwd"},
		"an extra flag":       {"-a", "-p", pid, "-d", "cwd", "-Fn", "-w"},
		"a non-numeric pid":   {"-a", "-p", "abc", "-d", "cwd", "-Fn"},
		"a flag in the pid":   {"-a", "-p", "-p", "-d", "cwd", "-Fn"},
		"a pid with no /proc": {"-a", "-p", "999999", "-d", "cwd", "-Fn"},
		"a pid with a slash":  {"-a", "-p", "../" + pid, "-d", "cwd", "-Fn"},
	}
	for name, argv := range bad {
		t.Run(name, func(t *testing.T) {
			out, err := f.runFakeLsof(argv...)
			if err == nil {
				t.Errorf("the fake should exit non-zero on %q, printed %q", argv, out)
			}
		})
	}
}

// End to end: the shim, with the fake lsof reading a /proc-like fixture,
// resolves the harness process's clone.
func TestShimHarness_ProcLsof_ShimResolvesThroughTheFake(t *testing.T) {
	f := newShimFixture(t)
	pid := f.env["CLAUDE_PID"]
	f.writeProcLsofAt(procFixture(t, pid, f.clone))
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
	}
	if h := f.helperCalls(); len(h) != 1 || strings.Join(h[0], " ") != "token --landing fixowner/fixrepo" {
		t.Errorf("helper calls = %q, want the fixture clone's repository", h)
	}
}

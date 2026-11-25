package main_test

// Tests of the gh shim's refusal to open a path that is not a plain file: today's
// record log (R-21, R-22) and today's warning marker (R-23). Opening a FIFO for
// writing blocks until a reader arrives, which would hang the call that is about
// to start gh; a symlink to /dev/null makes a write look like a record while
// keeping nothing. Each refusal is the step's own failure line (`gh shim: record:`
// or `gh shim: volume:`), which never stops the call.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// shimNotRegularFunc is the shim's one definition of "not a plain file"; the
// controls replace its body so that the shim opens whatever is there.
const shimNotRegularFunc = `not_regular() { command [ -h "$1" ] || { command [ -e "$1" ] && command [ ! -f "$1" ]; }; }`

// runWithin is run with a short bound, for calls that may hang. It returns
// whether the bound was hit. The shim's whole process group is killed at the
// bound, and the pipes are closed shortly after, so a blocked grandchild cannot
// hold the test.
func (f *shimFixture) runWithin(d time.Duration, args ...string) (shimResult, bool) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.shim, args...)
	cmd.Dir = f.callDir
	cmd.Env = f.environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return shimResult{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}, ctx.Err() != nil
}

// nonRegularKind builds, at path, a thing that is not a plain file, and returns
// the regular file a symlink points at (empty for the other kinds).
type nonRegularKind struct {
	name  string
	build func(t *testing.T, path, targetDir string) (target string)
}

func shimNonRegularKinds() []nonRegularKind {
	return []nonRegularKind{
		{"a FIFO", func(t *testing.T, path, _ string) string {
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatalf("mkfifo: %v", err)
			}
			// Release any writer left blocked on the FIFO when the test ends.
			t.Cleanup(func() {
				if fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
					_ = syscall.Close(fd)
				}
			})
			return ""
		}},
		{"a symlink to /dev/null", func(t *testing.T, path, _ string) string {
			if err := os.Symlink("/dev/null", path); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			return ""
		}},
		{"a symlink to a regular file", func(t *testing.T, path, dir string) string {
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(target, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			return target
		}},
		{"a dangling symlink", func(t *testing.T, path, dir string) string {
			if err := os.Symlink(filepath.Join(dir, "absent"), path); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			return ""
		}},
		{"a directory", func(t *testing.T, path, _ string) string {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
	}
}

func (f *shimFixture) expectTargetUntouched(target string) {
	f.t.Helper()
	if target == "" {
		return
	}
	if b, err := os.ReadFile(target); err != nil || len(b) != 0 {
		f.t.Errorf("nothing may be written through the symlink: target holds %q (err %v)", b, err)
	}
}

// R-22 with a record path that is not a plain file: the call still runs gh and
// prints one `gh shim: record:` line, the volume check is skipped, and nothing
// is written through a link or waited on.
func TestShim_R21_RecordPathThatIsNotAPlainFileIsRefused(t *testing.T) {
	for _, kind := range shimNonRegularKinds() {
		t.Run(kind.name, func(t *testing.T) {
			f := newShimFixture(t)
			if err := os.MkdirAll(f.recordDir(), 0o700); err != nil {
				t.Fatal(err)
			}
			target := kind.build(t, filepath.Join(f.recordDir(), shimToday()+".log"), f.fx)
			res, hung := f.runWithin(10*time.Second, "api", "user")
			if hung {
				t.Fatalf("the shim hung on the record path; stderr=%q", res.stderr)
			}
			if res.exit != 0 || f.ghRuns() != 1 {
				t.Fatalf("an unwritable record never stops the call: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
			}
			if lines := shimLinesWith(res.stderr, "gh shim: record:"); len(lines) != 1 {
				t.Errorf("expected exactly one `gh shim: record:` line, got %q in %q", lines, res.stderr)
			}
			if strings.Contains(res.stderr, "gh shim: volume") {
				t.Errorf("the volume check is skipped when the record is unwritable, got %q", res.stderr)
			}
			if strings.Contains(res.stderr, f.token) || strings.Contains(res.stdout, f.token) {
				t.Errorf("the token must not appear in the shim's output")
			}
			f.expectTargetUntouched(target)
		})
	}
}

// R-23's marker path that is not a plain file: the warning is not recorded, one
// `gh shim: volume:` line says so, and no `volume warning:` line claims a
// recorded warning. The call still runs gh.
func TestShim_R23_MarkerPathThatIsNotAPlainFileIsRefused(t *testing.T) {
	for _, kind := range shimNonRegularKinds() {
		t.Run(kind.name, func(t *testing.T) {
			f := newShimFixture(t)
			f.seedMinted(181)
			target := kind.build(t, f.markerPath(), f.fx)
			res, hung := f.runWithin(10*time.Second, "api", "user")
			if hung {
				t.Fatalf("the shim hung on the marker path; stderr=%q", res.stderr)
			}
			if res.exit != 0 || f.ghRuns() != 1 {
				t.Fatalf("a volume failure never stops the call: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
			}
			if lines := shimLinesWith(res.stderr, shimVolPrefix); len(lines) != 1 {
				t.Errorf("expected exactly one %q line, got %q in %q", shimVolPrefix, lines, res.stderr)
			}
			if lines := shimLinesWith(res.stderr, shimWarnPrefix); len(lines) != 0 {
				t.Errorf("no warning is claimed when the marker cannot be made, got %q", lines)
			}
			f.expectTargetUntouched(target)
		})
	}
}

// Controls: with the helper function made to report "a plain file" for
// everything, the same setups hang or write through the link, so the tests above
// can fail.
func TestShim_R21_R23_NonRegularControls_WithoutTheCheckTheShimOpensIt(t *testing.T) {
	noCheck := func(f *shimFixture) {
		f.shim = f.editedCopy("no-not-regular", [2]string{shimNotRegularFunc, "not_regular() { return 1; }"})
	}
	t.Run("a FIFO at the record path hangs the call", func(t *testing.T) {
		f := newShimFixture(t)
		noCheck(f)
		if err := os.MkdirAll(f.recordDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		shimNonRegularKinds()[0].build(t, filepath.Join(f.recordDir(), shimToday()+".log"), f.fx)
		if _, hung := f.runWithin(5*time.Second, "api", "user"); !hung {
			t.Errorf("the control copy should block on the FIFO")
		}
	})
	t.Run("a FIFO at the marker path hangs the call", func(t *testing.T) {
		if !shShellImportsFunctions(t) {
			// dash's noclobber redirection is O_EXCL and fails on a FIFO instead of
			// blocking; only bash blocks, so only bash can show the control.
			t.Skip("the interpreter is not bash, whose noclobber redirection opens an existing FIFO")
		}
		f := newShimFixture(t)
		noCheck(f)
		f.seedMinted(181)
		shimNonRegularKinds()[0].build(t, f.markerPath(), f.fx)
		if _, hung := f.runWithin(5*time.Second, "api", "user"); !hung {
			t.Errorf("the control copy should block on the FIFO")
		}
	})
	t.Run("a symlink to a regular file at the record path is written through", func(t *testing.T) {
		f := newShimFixture(t)
		noCheck(f)
		if err := os.MkdirAll(f.recordDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		target := shimNonRegularKinds()[2].build(t, filepath.Join(f.recordDir(), shimToday()+".log"), f.fx)
		if res, hung := f.runWithin(10*time.Second, "api", "user"); hung || res.exit != 0 {
			t.Fatalf("control copy: hung %v exit %d stderr=%q", hung, res.exit, res.stderr)
		}
		if b, _ := os.ReadFile(target); len(b) == 0 {
			t.Errorf("the control copy should write the record through the link")
		}
	})
}

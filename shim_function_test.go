package main_test

// Tests of the gh shim after the token capture: an exported shell function must
// not replace a command the shim runs while the token is in scope (review round
// 1, Warden L-1), and a clock that prints something other than whole seconds
// must not reach shell arithmetic.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shShellImportsFunctions reports whether the interpreter named on the tracked
// shim's shebang line imports an exported function from the environment (bash
// does, as `BASH_FUNC_name%%`; dash does not).
func shShellImportsFunctions(t *testing.T) bool {
	t.Helper()
	first, _, _ := strings.Cut(shimTrackedText(t), "\n")
	cmd := exec.Command(strings.TrimPrefix(first, "#!"), "-c", "probe")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "BASH_FUNC_probe%%=() { echo imported; }"}
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == "imported"
}

// Commands the shim runs after the capture, with the token variable in scope.
// Each is replaced, one at a time, by an exported function that writes the token
// variable to a file and then runs the real command.
// `command` itself is on the list: bash lets an exported function replace it,
// and the shim's `command` prefixes would then run that function instead. The
// shim removes any such function with `unset -f command` before its first use.
var shimPostCaptureCommands = []string{"date", "mkdir", "chmod", "printf", "umask", "awk", "[", "command"}

func TestShim_L1_ExportedFunctionCannotReplaceACommandWhileTheTokenIsInScope(t *testing.T) {
	if !shShellImportsFunctions(t) {
		t.Skip("/bin/sh does not import exported functions (dash), so there is nothing to replace")
	}
	hijack := func(f *shimFixture, name string) string {
		marker := filepath.Join(f.fx, "hijack-"+strings.NewReplacer("[", "bracket").Replace(name))
		// A function named command cannot call itself by that name; builtin reaches
		// the real one.
		runReal := "command " + name
		if name == "command" {
			runReal = "builtin command"
		}
		f.setEnv("BASH_FUNC_"+name+"%%", "() { echo \"${"+shimTokenVar+"-}\" >> '"+marker+"'; "+runReal+" \"$@\"; }")
		return marker
	}
	for _, name := range shimPostCaptureCommands {
		t.Run(name, func(t *testing.T) {
			f := newShimFixture(t)
			marker := hijack(f, name)
			res := f.run("api", "user")
			if res.exit != 0 {
				t.Fatalf("exit %d, stderr=%q", res.exit, res.stderr)
			}
			b, _ := os.ReadFile(marker)
			if strings.Contains(string(b), f.token) {
				t.Errorf("an exported function %q ran while the token was in scope and read it", name)
			}

			// Control: with every `command ` prefix removed, the same function
			// reads the token, so the scan above can see a leak. For a function
			// named command there is no prefix to remove: the control removes the
			// `unset -f command` that deletes it.
			g := newShimFixture(t)
			cm := hijack(g, name)
			controlText := strings.ReplaceAll(shimTrackedText(t), "command ", "")
			if name == "command" {
				controlText = shimEdit(t, shimTrackedText(t), [2]string{"unset -f command\n", ""})
			}
			g.shim = g.copyOf("no-command-prefix", controlText)
			if c := g.run("api", "user"); c.exit != 0 {
				t.Fatalf("control copy exit %d, stderr=%q", c.exit, c.stderr)
			}
			cb, _ := os.ReadFile(cm)
			if !strings.Contains(string(cb), g.token) {
				t.Errorf("the control copy should let the function read the token, marker holds %q", cb)
			}
		})
	}
}

// A date that prints something other than digits must not reach the shell's
// arithmetic, where a value such as a[$(cmd)] runs cmd. The call fails closed as
// mint-failed, and the recorded line says so.
func TestShim_L1_ClockOutputThatIsNotDigitsIsRefusedBeforeArithmetic(t *testing.T) {
	for _, call := range []string{"1", "2"} {
		for pname, payload := range map[string]func(touch, marker string) string{
			"nothing":                         func(touch, marker string) string { return "" },
			"two numbers":                     func(touch, marker string) string { return "12 34" },
			"a subscript that runs a command": func(touch, marker string) string { return "x[$(" + touch + " " + marker + ")]" },
		} {
			t.Run("call "+call+" prints "+pname, func(t *testing.T) {
				f := newShimFixture(t)
				marker := filepath.Join(f.fx, "arith-marker")
				counter := filepath.Join(f.fx, "date.count")
				realDate := shimMustLook(t, "date")
				touch := shimMustLook(t, "touch")
				body := "n=0\n[ -f '" + counter + "' ] && read n < '" + counter + "'\n" +
					"if [ \"$1\" = '+%s' ]; then\n  n=$((n + 1))\n  echo \"$n\" > '" + counter + "'\n" +
					"  if [ \"$n\" -eq " + call + " ]; then\n" +
					"    printf '%s\\n' '" + payload(touch, marker) + "'\n    exit 0\n  fi\nfi\n" +
					"exec '" + realDate + "' \"$@\"\n"
				shimWriteExec(t, filepath.Join(f.bin, "date"), body)
				res := f.run("api", "user")
				f.expectRefused(res, "mint-failed")
				if _, err := os.Stat(marker); err == nil {
					t.Errorf("the clock output ran a command through shell arithmetic")
				}
				rec := f.expectOneRecord("mint-failed", "fixowner/fixrepo")
				if rec.mintSeconds != "" {
					t.Errorf("a refused clock reading records no mint_seconds, got %q", rec.mintSeconds)
				}
				// A bad starting reading refuses before the helper runs, so no token is
				// minted and dropped. A bad ending reading comes after the mint.
				if call == "1" {
					f.expectNoHelperCall()
				} else if n := len(f.helperCalls()); n != 1 {
					t.Errorf("a bad ending reading follows one helper call, got %d", n)
				}
			})
		}
	}
}

// installClock makes the fake date print payload for its call-th `+%s` reading
// and pass everything else to the real date.
func (f *shimFixture) installClock(call, payload string) {
	f.t.Helper()
	counter := filepath.Join(f.fx, "date.count")
	body := "n=0\n[ -f '" + counter + "' ] && read n < '" + counter + "'\n" +
		"if [ \"$1\" = '+%s' ]; then\n  n=$((n + 1))\n  echo \"$n\" > '" + counter + "'\n" +
		"  if [ \"$n\" -eq " + call + " ]; then\n    printf '%s\\n' '" + payload + "'\n    exit 0\n  fi\nfi\n" +
		"exec '" + shimMustLook(f.t, "date") + "' \"$@\"\n"
	shimWriteExec(f.t, filepath.Join(f.bin, "date"), body)
}

// installMintedCount makes the fake awk print payload for the volume check's
// count of minted lines, and run the real awk for anything else (the shim's
// GIT_ name read uses it too).
func (f *shimFixture) installMintedCount(payload string) {
	f.t.Helper()
	body := "case \"$*\" in\n  *minted*) printf '%s\\n' '" + payload + "'; exit 0 ;;\nesac\n" +
		"exec '" + shimMustLook(f.t, "awk") + "' \"$@\"\n"
	shimWriteExec(f.t, filepath.Join(f.bin, "awk"), body)
}

// A reading or a count with a leading zero is made of digits, but the shell
// reads 08 as an invalid octal number and aborts the arithmetic, with no class
// line. Each digit check refuses it: the two clock readings as mint-failed, the
// count as a volume line that lets the call go on. Control: a copy without the
// refusal arms aborts instead.
func TestShim_L1_LeadingZeroIsRefusedWithItsClassLine(t *testing.T) {
	for _, call := range []string{"1", "2"} {
		t.Run("clock reading "+call+" is 08", func(t *testing.T) {
			f := newShimFixture(t)
			f.installClock(call, "08")
			res := f.run("api", "user")
			f.expectRefused(res, "mint-failed")
			if rec := f.expectOneRecord("mint-failed", "fixowner/fixrepo"); rec.mintSeconds != "" {
				t.Errorf("a refused clock reading records no mint_seconds, got %q", rec.mintSeconds)
			}
		})
	}
	t.Run("the minted count is 08", func(t *testing.T) {
		f := newShimFixture(t)
		f.installMintedCount("08")
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Fatalf("a volume failure is operations data and gh still runs: exit %d, runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
		if lines := shimLinesWith(res.stderr, shimVolPrefix); len(lines) != 1 {
			t.Errorf("expected exactly one %q line, got %q in %q", shimVolPrefix, lines, res.stderr)
		}
		if lines := shimLinesWith(res.stderr, shimWarnPrefix); len(lines) != 0 {
			t.Errorf("no volume warning on an unreadable count, got %q", lines)
		}
	})

	control := func(t *testing.T) *shimFixture {
		f := newShimFixture(t)
		text := shimTrackedText(t)
		if n := strings.Count(text, "|0?*|"); n != 3 {
			t.Fatalf("expected the leading-zero arm in three digit checks, found %d", n)
		}
		f.shim = f.copyOf("no-leading-zero-arm", strings.ReplaceAll(text, "|0?*|", "|"))
		return f
	}
	for _, call := range []string{"1", "2"} {
		t.Run("control: without the arm, clock reading "+call+" of 08 aborts with no class line", func(t *testing.T) {
			f := control(t)
			f.installClock(call, "08")
			res := f.run("api", "user")
			if res.exit == 0 || strings.Contains(res.stderr, "gh shim: mint-failed:") {
				t.Errorf("the control copy should abort on the arithmetic, got exit %d stderr=%q", res.exit, res.stderr)
			}
		})
	}
	t.Run("control: without the arm, a count of 08 aborts with no volume line", func(t *testing.T) {
		f := control(t)
		f.installMintedCount("08")
		res := f.run("api", "user")
		if f.ghRuns() != 0 || strings.Contains(res.stderr, shimVolPrefix) {
			t.Errorf("the control copy should abort before gh with no volume line, got exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
	})
}

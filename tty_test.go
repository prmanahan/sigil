package main

// tty_test.go — unit coverage for requireTerminalStdout (defined in main.go
// beside validateAPIBase, its sibling credential-free preflight check).
//
// `sigil token <owner>` mints a full-permission token and must refuse to
// run when stdout is not a terminal. These tests exercise the decision function
// directly; the wired-up binary is covered black-box in cli_test.go, including
// the two paths that MUST NOT be gated (`sigil get`, `token --rate-limit-only`).
//
// Note on the environment: under `go test` the test binary's own os.Stdout is a
// pipe, so a test must never assume a terminal is available. The positive
// control below opens /dev/ptmx to obtain a character device of its own.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequireTerminalStdout_RefusesNonTerminals covers the decision half: a
// pipe (what os/exec hands a child whose output is captured), and a regular
// file (what an agent shell's redirected stdout is — measured 2026-09-20) are
// both refused, with exactly the refusal message and nothing appended.
func TestRequireTerminalStdout_RefusesNonTerminals(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	regular, err := os.Create(filepath.Join(t.TempDir(), "captured-output"))
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	defer regular.Close()

	tests := []struct {
		name string
		f    *os.File
	}{
		{name: "pipe write end", f: w},
		{name: "pipe read end", f: r},
		{name: "regular file", f: regular},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireTerminalStdout(tc.f)
			if err == nil {
				t.Fatalf("requireTerminalStdout(%s): got nil, want a refusal", tc.name)
			}
			if err.Error() != errStdoutNotTerminal {
				t.Errorf("requireTerminalStdout(%s) message:\n got: %q\nwant: %q",
					tc.name, err.Error(), errStdoutNotTerminal)
			}
		})
	}
}

// TestRequireTerminalStdout_PermitsCharacterDevice is the positive control for
// the table above: without it, a requireTerminalStdout that refused everything
// unconditionally — a gate that has stopped discriminating — would pass every
// refusal test while breaking the interactive case.
//
// /dev/ptmx allocates a pseudo-terminal master, which is the same character
// device a terminal emulator hands a process (verified 2026-09-20: char device,
// as is the pty slave under `script -q /dev/null`).
func TestRequireTerminalStdout_PermitsCharacterDevice(t *testing.T) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("cannot open /dev/ptmx (%v) — no pty available; the CLI-level pty control in cli_test.go still applies", err)
	}
	defer ptmx.Close()

	if err := requireTerminalStdout(ptmx); err != nil {
		t.Errorf("requireTerminalStdout(/dev/ptmx): got %v, want nil (a pty must be permitted)", err)
	}
}

// TestRequireTerminalStdout_CharacterDeviceDivergenceFromIsatty pins the known,
// documented gap between this check and isatty(3): /dev/null is a character
// device and is therefore permitted, where isatty(3) would refuse.
//
// This test asserts the weakness on purpose, so that it cannot change silently
// in either direction. ⚠ Do NOT read the permitted /dev/null case as harmless:
// the run discards the PRINTED BYTES but not the token. The gate sits before the
// mint, and getInstallationTokenLazy calls cachePut before cmdToken reaches
// fmt.Println, so `sigil token <owner> > /dev/null` still leaves a live
// full-permission token in ~/.cache/sigil/<installID>.json. See
// requireTerminalStdout's comment for the full list of gaps. If this check is
// ever tightened to a real isatty, this test fails and the decision is made
// deliberately rather than drifting.
func TestRequireTerminalStdout_CharacterDeviceDivergenceFromIsatty(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	if err := requireTerminalStdout(devNull); err != nil {
		t.Errorf("requireTerminalStdout(%s): got %v, want nil — the character-device "+
			"test permits /dev/null, and the comment documenting that must stay true", os.DevNull, err)
	}
}

// TestRequireTerminalStdout_FailsClosedOnUnreadableMode covers the guard's own
// error path: when the mode cannot be read at all, the operation is DENIED, not
// passed through. A control that cannot evaluate must not fall open.
func TestRequireTerminalStdout_FailsClosedOnUnreadableMode(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Stat on a closed *os.File returns os.ErrClosed — the guard cannot decide.
	err = requireTerminalStdout(f)
	if err == nil {
		t.Fatal("requireTerminalStdout(closed file): got nil, want a refusal (fail closed)")
	}
	if !strings.HasPrefix(err.Error(), errStdoutNotTerminal) {
		t.Errorf("requireTerminalStdout(closed file) message %q: want it to lead with the refusal text", err.Error())
	}
}

// TestErrStdoutNotTerminal_NamesReasonAndSanctionedAlternative covers the refusal text: the
// message names why the command refused, and points at the form that is meant
// to be used unattended, so the refusal is actionable without inviting a bypass
// (none is CONFIGURED — requireTerminalStdout enumerates the ones that exist
// regardless).
func TestErrStdoutNotTerminal_NamesReasonAndSanctionedAlternative(t *testing.T) {
	for _, want := range []string{"stdout is not a terminal", "full-permission", "--rate-limit-only"} {
		if !strings.Contains(errStdoutNotTerminal, want) {
			t.Errorf("errStdoutNotTerminal %q: want it to mention %q", errStdoutNotTerminal, want)
		}
	}
}

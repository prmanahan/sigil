package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStd swaps os.Stdout and os.Stderr for pipes, runs fn, and returns
// what was written to each. It mutates process-global state, so tests using it
// must not call t.Parallel (none in this package do).
func captureStd(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW
	restore := func() { os.Stdout, os.Stderr = origOut, origErr }
	t.Cleanup(restore)

	fn()

	restore()
	outW.Close()
	errW.Close()
	outB, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	errB, err := io.ReadAll(errR)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(outB), string(errB)
}

// TestWriteCredentialResponse_TokenReachesOnlyTheGivenWriter pins the
// channel-separation invariant for writeCredentialResponse and nothing wider:
// while it runs, the token must appear in the writer it was handed and
// nowhere on os.Stderr or os.Stdout. It fails if someone adds, say,
// fmt.Fprintln(os.Stderr, token) inside that function (seeded and measured).
//
// Scope, stated plainly: this guards that one function. It does not scan the
// rest of the binary — other code paths that print or log a token are not
// covered here. The end-to-end leak scan (TestCLI_LeakScan_TokenNotInStderr)
// still needs real 1Password credentials and is skipped in the default gate.
func TestWriteCredentialResponse_TokenReachesOnlyTheGivenWriter(t *testing.T) {
	const token = "ghs_channel_separation_probe_0123456789"

	var buf bytes.Buffer
	stdout, stderr := captureStd(t, func() {
		writeCredentialResponse(&buf, token)
	})

	if !strings.Contains(buf.String(), "password="+token) {
		t.Fatalf("token missing from the sanctioned writer:\n%q", buf.String())
	}
	if strings.Contains(stderr, token) {
		t.Errorf("token reached os.Stderr: %q", stderr)
	}
	if stderr != "" {
		t.Errorf("os.Stderr must stay empty, got: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("os.Stdout must stay empty (output belongs to the given writer), got: %q", stdout)
	}
}

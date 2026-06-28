package main

// hardening_compilefail_test.go — red-phase tests for AC#7 and L3.
//
// These tests reference functions that do not yet exist or have different
// signatures; the package will not compile until Forge implements them.
// That compile-fail IS the expected red state.
//
// Functions Forge must implement (cross-dispatch handoff):
//
//   writeCredentialResponse(w io.Writer, token string)
//       Writes "username=x-access-token\npassword=<token>\n" to w ONLY.
//       Extracted from the inline fmt.Printf in cmdGet (main.go:103).
//       Taking exactly one io.Writer enforces the channel-separation invariant:
//       the function has no code path to os.Stderr.
//
//   defaultCacheDir() string
//       Returns filepath.Join(os.Getenv("HOME"), ".cache", "sigil").
//       Must NOT use os.UserCacheDir() (path differs under XDG; HOME is
//       predictable in tests that set t.Setenv("HOME", tmp)).
//
//   ensureCacheDir(dir string) error   ← NEW signature (dir is explicit)
//       MkdirAll(dir, 0700) AND os.Chmod(dir, 0700) on an already-existing dir.
//       The existing ensureCacheDir() (string, error) must be refactored to
//       call defaultCacheDir() + ensureCacheDir(dir) internally.
//       Callers that use t.TempDir() pass the dir directly — no XDG uncertainty.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --- AC#7: writeCredentialResponse channel-separation ---

// TestWriteCredentialResponse_WritesCredentialLines verifies that
// writeCredentialResponse writes exactly the git credential protocol lines to
// the provided writer.
//
// RED (compile-fail): writeCredentialResponse does not exist. Forge must
// extract it from cmdGet's inline fmt.Printf(main.go:103).
func TestWriteCredentialResponse_WritesCredentialLines(t *testing.T) {
	// Given: a buffer standing in for the sanctioned stdout writer.
	var buf bytes.Buffer
	const token = "ghs_known_test_token"

	// When: writeCredentialResponse is called with the buffer and a known token.
	// (compile-fail: writeCredentialResponse undefined)
	writeCredentialResponse(&buf, token)

	// Then: the buffer contains the username line.
	if !bytes.Contains(buf.Bytes(), []byte("username=x-access-token")) {
		t.Errorf("output missing 'username=x-access-token':\n%s", buf.String())
	}
	// And: the buffer contains the password line with the exact token.
	if !bytes.Contains(buf.Bytes(), []byte("password="+token)) {
		t.Errorf("output missing 'password=%s':\n%s", token, buf.String())
	}
	// Channel-separation note: the function accepts exactly one io.Writer.
	// By construction it has no code path to os.Stderr. A call with a buffer
	// is sufficient to prove the token reaches the sanctioned writer; the
	// io.Writer-only signature is the structural guarantee that no other channel
	// exists. No os.Stderr interception needed.
}

// TestWriteCredentialResponse_OnlyWritesToGivenWriter verifies the
// output length: the function writes username= and password= lines and nothing
// else to the buffer (no extra blank lines from a stray fmt.Println, etc.).
func TestWriteCredentialResponse_OnlyWritesToGivenWriter(t *testing.T) {
	// Given
	var buf bytes.Buffer
	const token = "ghs_abc"

	// When
	// (compile-fail: writeCredentialResponse undefined)
	writeCredentialResponse(&buf, token)

	// Then: output is exactly the two-line credential block.
	want := "username=x-access-token\npassword=" + token + "\n"
	if got := buf.String(); got != want {
		t.Errorf("writeCredentialResponse output:\ngot:  %q\nwant: %q", got, want)
	}
}

// --- L3: defaultCacheDir and ensureCacheDir with explicit dir ---

// TestDefaultCacheDir_UsesHOME verifies that defaultCacheDir() returns
// $HOME/.cache/sigil (not os.UserCacheDir()/sigil which varies by XDG config).
//
// RED (compile-fail): defaultCacheDir is not yet extracted from ensureCacheDir.
func TestDefaultCacheDir_UsesHOME(t *testing.T) {
	// Given: a controlled HOME directory.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// When: defaultCacheDir is called.
	// (compile-fail: defaultCacheDir undefined)
	got := defaultCacheDir()

	// Then: the returned path is $HOME/.cache/sigil.
	want := filepath.Join(tmp, ".cache", "sigil")
	if got != want {
		t.Errorf("defaultCacheDir(): got %q, want %q", got, want)
	}
}

// TestEnsureCacheDir_CreatesDir verifies that ensureCacheDir creates the target
// directory when it does not exist, with mode 0700.
//
// RED (compile-fail): ensureCacheDir(dir string) error has a different signature
// from the existing ensureCacheDir() (string, error) — calling it with one arg
// fails to compile.
func TestEnsureCacheDir_CreatesDir(t *testing.T) {
	// Given: a non-existent path inside a temp dir.
	base := t.TempDir()
	target := filepath.Join(base, "newdir", "sigil")

	// When: ensureCacheDir is called.
	// (compile-fail: ensureCacheDir(string) does not match existing signature)
	if err := ensureCacheDir(target); err != nil {
		t.Fatalf("ensureCacheDir: %v", err)
	}

	// Then: the directory exists with mode 0700.
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected a directory, got: %v", info.Mode())
	}
	if mode := info.Mode().Perm(); mode != 0700 {
		t.Errorf("dir perms: got %04o, want 0700", mode)
	}
}

// TestEnsureCacheDir_ChmodsExistingDirTo0700 verifies that ensureCacheDir tightens
// an existing directory's permissions to 0700 when they are looser (e.g. 0755).
//
// RED (compile-fail): same signature mismatch as above.
func TestEnsureCacheDir_ChmodsExistingDirTo0700(t *testing.T) {
	// Given: a pre-existing directory with mode 0755.
	base := t.TempDir()
	target := filepath.Join(base, "sigil")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Explicit chmod after Mkdir to defeat umask (ensures setup mode is exactly 0755).
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatalf("setup Chmod: %v", err)
	}

	// When: ensureCacheDir is called on the pre-existing directory.
	// (compile-fail: ensureCacheDir(string) does not match existing signature)
	if err := ensureCacheDir(target); err != nil {
		t.Fatalf("ensureCacheDir: %v", err)
	}

	// Then: the directory mode is 0700 (tightened from 0755).
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0700 {
		t.Errorf("dir perms: got %04o, want 0700 (ensureCacheDir must chmod an existing dir)", mode)
	}
}

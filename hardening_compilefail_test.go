package main

// hardening_compilefail_test.go — tests for writeCredentialResponse's output
// shape, defaultCacheDir, and ensureCacheDir's permission enforcement. Ordinary
// tests, run by a plain `go test ./...`.
//
//   writeCredentialResponse(w io.Writer, token string)
//       Writes "username=x-access-token\npassword=<token>\n" to w only.
//       Taking exactly one io.Writer is the signature-level half of the
//       channel-separation invariant; credential_channel_test.go holds the
//       behavioural half.
//
//   defaultCacheDir() string
//       Returns filepath.Join(os.Getenv("HOME"), ".cache", "sigil"). It does not
//       use os.UserCacheDir(), whose result varies with XDG settings; HOME is
//       predictable in tests that set t.Setenv("HOME", tmp).
//
//   ensureCacheDir(dir string) error
//       MkdirAll(dir, 0700) and os.Chmod(dir, 0700), so a pre-existing looser
//       directory is tightened.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --- writeCredentialResponse output shape ---

// TestWriteCredentialResponse_WritesCredentialLines verifies that
// writeCredentialResponse writes exactly the git credential protocol lines to
// the provided writer.
func TestWriteCredentialResponse_WritesCredentialLines(t *testing.T) {
	// Given: a buffer standing in for the sanctioned stdout writer.
	var buf bytes.Buffer
	const token = "ghs_known_test_token"

	// When: writeCredentialResponse is called with the buffer and a known token.
	writeCredentialResponse(&buf, token)

	// Then: the buffer contains the username line.
	if !bytes.Contains(buf.Bytes(), []byte("username=x-access-token")) {
		t.Errorf("output missing 'username=x-access-token':\n%s", buf.String())
	}
	// And: the buffer contains the password line with the exact token.
	if !bytes.Contains(buf.Bytes(), []byte("password="+token)) {
		t.Errorf("output missing 'password=%s':\n%s", token, buf.String())
	}
	// This test checks only that the token reaches the given writer. That it
	// reaches no other stream is checked in credential_channel_test.go.
}

// TestWriteCredentialResponse_OnlyWritesToGivenWriter verifies the
// output length: the function writes username= and password= lines and nothing
// else to the buffer (no extra blank lines from a stray fmt.Println, etc.).
func TestWriteCredentialResponse_OnlyWritesToGivenWriter(t *testing.T) {
	// Given
	var buf bytes.Buffer
	const token = "ghs_abc"

	// When
	writeCredentialResponse(&buf, token)

	// Then: output is exactly the two-line credential block.
	want := "username=x-access-token\npassword=" + token + "\n"
	if got := buf.String(); got != want {
		t.Errorf("writeCredentialResponse output:\ngot:  %q\nwant: %q", got, want)
	}
}

// --- defaultCacheDir and ensureCacheDir with an explicit dir ---

// TestDefaultCacheDir_UsesHOME verifies that defaultCacheDir() returns
// $HOME/.cache/sigil (not os.UserCacheDir()/sigil which varies by XDG config).
func TestDefaultCacheDir_UsesHOME(t *testing.T) {
	// Given: a controlled HOME directory.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// When: defaultCacheDir is called.
	got := defaultCacheDir()

	// Then: the returned path is $HOME/.cache/sigil.
	want := filepath.Join(tmp, ".cache", "sigil")
	if got != want {
		t.Errorf("defaultCacheDir(): got %q, want %q", got, want)
	}
}

// TestEnsureCacheDir_CreatesDir verifies that ensureCacheDir creates the target
// directory when it does not exist, with mode 0700.
func TestEnsureCacheDir_CreatesDir(t *testing.T) {
	// Given: a non-existent path inside a temp dir.
	base := t.TempDir()
	target := filepath.Join(base, "newdir", "sigil")

	// When: ensureCacheDir is called.
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

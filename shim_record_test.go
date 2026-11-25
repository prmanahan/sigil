package main_test

// Tests of the gh shim's call record: its format and modes (R-21) and the
// behaviour when it cannot be written (R-22).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var shimRecordTime = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
var shimDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// shimExpectWellFormed asserts one record line matches the Data Model.
func shimExpectWellFormed(t *testing.T, r shimRecordLine) {
	t.Helper()
	if len(strings.Split(r.raw, "\t")) != 5 {
		t.Errorf("record line should have five tab-separated fields: %q", r.raw)
		return
	}
	if !shimRecordTime.MatchString(r.time) {
		t.Errorf("time field %q is not UTC ISO 8601 to the second with a Z suffix", r.time)
	}
	switch r.outcome {
	case "minted":
		if r.class != "narrowed" {
			t.Errorf("class = %q for outcome minted, want narrowed", r.class)
		}
	case "mint-failed", "no-repo", "refused":
		if r.class != "none" {
			t.Errorf("class = %q for outcome %s, want none", r.class, r.outcome)
		}
	default:
		t.Errorf("outcome %q is outside the closed set", r.outcome)
	}
}

// R-21: each outcome writes one line in the Data Model's format.
func TestShim_R21_EachOutcomeWritesOneWellFormedLine(t *testing.T) {
	t.Run("minted", func(t *testing.T) {
		f := newShimFixture(t)
		f.run("api", "user")
		r := f.expectOneRecord("minted", "fixowner/fixrepo")
		shimExpectWellFormed(t, r)
		if !shimDecimal.MatchString(r.mintSeconds) {
			t.Errorf("mint_seconds = %q, want a non-empty decimal number", r.mintSeconds)
		}
		if tm, err := time.Parse(time.RFC3339, r.time); err != nil || time.Since(tm) > 2*time.Minute || time.Until(tm) > time.Minute {
			t.Errorf("record time %q is not the current UTC time (%v)", r.time, err)
		}
		files, _ := filepath.Glob(filepath.Join(f.recordDir(), "*.log"))
		if len(files) != 1 || !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}\.log$`).MatchString(filepath.Base(files[0])) {
			t.Errorf("record file should be <UTC date>.log, got %q", files)
		}
		if len(files) == 1 && strings.TrimSuffix(filepath.Base(files[0]), ".log") != r.time[:10] {
			t.Errorf("record file %q does not carry the line's UTC date %q", filepath.Base(files[0]), r.time[:10])
		}
	})
	t.Run("mint-failed", func(t *testing.T) {
		f := newShimFixture(t)
		f.setHelper("exit 1")
		f.run("api", "user")
		r := f.expectOneRecord("mint-failed", "fixowner/fixrepo")
		shimExpectWellFormed(t, r)
		if !shimDecimal.MatchString(r.mintSeconds) {
			t.Errorf("mint_seconds = %q, want a non-empty decimal number", r.mintSeconds)
		}
	})
	t.Run("no-repo", func(t *testing.T) {
		f := newShimFixture(t)
		f.startHarness(f.callDir)
		f.run("api", "user")
		r := f.expectOneRecord("no-repo", "")
		shimExpectWellFormed(t, r)
		if r.mintSeconds != "" {
			t.Errorf("mint_seconds should be empty when no mint was attempted, got %q", r.mintSeconds)
		}
	})
	t.Run("refused", func(t *testing.T) {
		f := newShimFixture(t)
		f.run("auth", "token")
		r := f.expectOneRecord("refused", "fixowner/fixrepo")
		shimExpectWellFormed(t, r)
		if r.mintSeconds != "" {
			t.Errorf("mint_seconds should be empty when no mint was attempted, got %q", r.mintSeconds)
		}
	})
}

// R-21: the directory is 0700 and the file 0600, re-applied after creation.
func TestShim_R21_RecordModesAreReapplied(t *testing.T) {
	t.Run("a fresh record", func(t *testing.T) {
		f := newShimFixture(t)
		f.run("api", "user")
		shimExpectModes(t, f)
	})
	t.Run("a pre-existing 0755 directory and 0644 file", func(t *testing.T) {
		f := newShimFixture(t)
		if err := os.MkdirAll(f.recordDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(f.recordDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(f.recordDir(), time.Now().UTC().Format("2006-01-02")+".log")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(file, 0o644); err != nil {
			t.Fatal(err)
		}
		f.run("api", "user")
		shimExpectModes(t, f)
		if recs := f.records(); len(recs) != 1 {
			t.Errorf("the pre-existing file should have gained exactly one line, got %q", recs)
		}
	})
}

func shimExpectModes(t *testing.T, f *shimFixture) {
	t.Helper()
	di, err := os.Stat(f.recordDir())
	if err != nil {
		t.Fatalf("record directory: %v", err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("record directory mode = %v, want 0700", di.Mode().Perm())
	}
	files, _ := filepath.Glob(filepath.Join(f.recordDir(), "*.log"))
	if len(files) != 1 {
		t.Fatalf("expected one record file, got %q", files)
	}
	fi, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("record file mode = %v, want 0600", fi.Mode().Perm())
	}
}

// R-21: the record holds no token. Control: the same grep finds a token
// appended to a copy.
func TestShim_R21_RecordHoldsNoToken(t *testing.T) {
	f := newShimFixture(t)
	f.run("api", "user")
	f.setHelper("exit 1")
	f.run("api", "user")
	files, _ := filepath.Glob(filepath.Join(f.recordDir(), "*.log"))
	if len(files) != 1 {
		t.Fatalf("expected one record file, got %q", files)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	// The check and its control use the same scan, shimScanTree.
	if hits := shimScanTree(t, f.recordDir(), f.token); len(hits) != 0 {
		t.Errorf("the record holds a token: %q in %q", b, hits)
	}
	copyDir := filepath.Join(f.fx, "record-copy")
	if err := os.MkdirAll(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, "r.log"), append(b, []byte(f.token+"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := shimScanTree(t, copyDir, f.token); len(hits) != 1 {
		t.Errorf("the grep control should find the appended token, found %q", hits)
	}
}

// R-21: twenty concurrent calls produce twenty well-formed, non-interleaved
// lines.
func TestShim_R21_ConcurrentCallsDoNotInterleave(t *testing.T) {
	f := newShimFixture(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	exits := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := f.runErr("api", "user")
			errs[i], exits[i] = err, res.exit
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil || exits[i] != 0 {
			t.Fatalf("call %d: err=%v exit=%d", i, errs[i], exits[i])
		}
	}
	recs := f.records()
	if len(recs) != n {
		t.Fatalf("expected %d record lines, got %d", n, len(recs))
	}
	for _, r := range recs {
		shimExpectWellFormed(t, r)
		if r.outcome != "minted" || r.repo != "fixowner/fixrepo" {
			t.Errorf("unexpected line %q", r.raw)
		}
	}
	if len(f.helperCalls()) != n {
		t.Errorf("each call mints once: helper ran %d times", len(f.helperCalls()))
	}
}

// R-22: with the state directory's path occupied by a regular file, gh still
// runs and one `gh shim: record:` line says the count is unavailable.
func TestShim_R22_UnwritableRecordWarnsAndContinues(t *testing.T) {
	occupy := func(f *shimFixture) {
		parent := filepath.Dir(f.recordDir())
		if err := os.MkdirAll(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.recordDir(), []byte("occupied"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	recordLines := func(stderr string) []string {
		var out []string
		for _, l := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(l, "gh shim: record:") {
				out = append(out, l)
			}
		}
		return out
	}
	t.Run("the fake gh still runs and stderr holds exactly one record line with no token", func(t *testing.T) {
		f := newShimFixture(t)
		occupy(f)
		res := f.run("api", "user")
		if res.exit != 0 || f.ghRuns() != 1 {
			t.Fatalf("gh should still run: exit %d, runs %d, stderr=%q", res.exit, f.ghRuns(), res.stderr)
		}
		lines := recordLines(res.stderr)
		if len(lines) != 1 {
			t.Fatalf("expected exactly one `gh shim: record:` line, got %q in %q", lines, res.stderr)
		}
		if !strings.Contains(lines[0], "unavailable") {
			t.Errorf("the line should say the volume count is unavailable: %q", lines[0])
		}
		if strings.Contains(res.stderr, f.token) || strings.Contains(res.stdout, f.token) {
			t.Errorf("the token must not appear in the shim's output")
		}
	})
	t.Run("the shared seeded control: a copy that prints the token on that line fails the scan", func(t *testing.T) {
		f := newShimFixture(t)
		occupy(f)
		f.shim = f.editedCopy("record-line-leak", [2]string{
			`unavailable for this call\n' >&2`,
			`unavailable for this call %s\n' "$` + shimTokenVar + `" >&2`,
		})
		res := f.run("api", "user")
		lines := recordLines(res.stderr)
		if len(lines) != 1 || !strings.Contains(lines[0], f.token) {
			t.Errorf("the control copy should leak the token on the record line, got %q", lines)
		}
	})
	// The class line comes before the record line (D18). The classes that write
	// nothing ahead of the shim's own line begin stderr with it; a mint-failed
	// call may carry the helper's passthrough first, so expectRefused pins its
	// first `gh shim: ` line instead.
	failures := []struct {
		class string
		setup func(f *shimFixture)
		argv  []string
	}{
		{"refused", func(f *shimFixture) {}, []string{"auth", "token"}},
		{"no-repo", func(f *shimFixture) { f.startHarness(f.callDir) }, []string{"api", "user"}},
		{"mint-failed", func(f *shimFixture) { f.setHelper("exit 1") }, []string{"api", "user"}},
	}
	for _, c := range failures {
		t.Run("a "+c.class+" call with an unwritable record reports its class line first", func(t *testing.T) {
			f := newShimFixture(t)
			occupy(f)
			c.setup(f)
			res := f.run(c.argv...)
			f.expectRefused(res, c.class)
			lines := strings.Split(res.stderr, "\n")
			classAt, recordAt := -1, -1
			for i, l := range lines {
				if strings.HasPrefix(l, "gh shim: "+c.class+":") && classAt < 0 {
					classAt = i
				}
				if strings.HasPrefix(l, "gh shim: record:") && recordAt < 0 {
					recordAt = i
				}
			}
			if rl := recordLines(res.stderr); len(rl) != 1 {
				t.Fatalf("expected one record line as well, got %q", res.stderr)
			}
			if classAt < 0 || classAt > recordAt {
				t.Errorf("the class line (index %d) must come before the record line (index %d) in %q", classAt, recordAt, res.stderr)
			}
		})
	}
}

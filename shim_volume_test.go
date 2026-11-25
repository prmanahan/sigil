package main_test

// Tests of the gh shim's volume warning and its marker: R-23 (the threshold,
// once a day, the failure lines), R-24 (the warning line) and R-22's skip.
//
// The threshold is (N + 52) x reads-per-mint >= 70% of the daily limit. At the
// shipped constants (3.0 reads per mint, held as 30 tenths, and a limit of
// 1000) that is 7020 >= 7000 at N = 182 and 6990 < 7000 at N = 181. The
// oracle below restates the spec's inequality in integers; it never calls the
// shim to learn a boundary.
//
// D17: the marker's time format is pinned here, not by the edge tests. It is
// the record's own: ISO 8601 UTC to the second with a Z suffix.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	shimTenthsLine = "_sgs_reads_per_mint_tenths=30\n"
	shimWarnPrefix = "gh shim: volume warning:"
	shimVolPrefix  = "gh shim: volume:"
)

var shimMarkerLine = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z\t([0-9]+)$`)

// shimFires is the spec's firing condition, in integers.
func shimFires(n, tenths int) bool { return (n+52)*tenths >= 7*1000 }

func shimToday() string { return time.Now().UTC().Format("2006-01-02") }

// seedMinted writes today's record with n minted lines.
func (f *shimFixture) seedMinted(n int) {
	f.t.Helper()
	if err := os.MkdirAll(f.recordDir(), 0o700); err != nil {
		f.t.Fatal(err)
	}
	var b strings.Builder
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%s\tfixowner/fixrepo\tnarrowed\tminted\t1\n", ts)
	}
	p := filepath.Join(f.recordDir(), shimToday()+".log")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *shimFixture) markerPath() string {
	return filepath.Join(f.recordDir(), shimToday()+".warned")
}

func shimLinesWith(stderr, prefix string) []string {
	var out []string
	for _, l := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// expectWarning asserts exactly one R-24 line carrying n, the limit and
// reads-per-mint (as text), the word maintainer and no token, and a marker of
// the D17 form holding n.
func (f *shimFixture) expectWarning(res shimResult, n int, reads string) {
	f.t.Helper()
	lines := shimLinesWith(res.stderr, shimWarnPrefix)
	if len(lines) != 1 {
		f.t.Fatalf("expected exactly one %q line, got %q in %q", shimWarnPrefix, lines, res.stderr)
	}
	l := lines[0]
	if !regexp.MustCompile(`(^|[^0-9.])` + strconv.Itoa(n) + `([^0-9.]|$)`).MatchString(l) {
		f.t.Errorf("the line should state the count %d: %q", n, l)
	}
	for _, want := range []string{"1000", reads, "maintainer"} {
		if !strings.Contains(l, want) {
			f.t.Errorf("the line should hold %q: %q", want, l)
		}
	}
	if strings.Contains(res.stderr, f.token) || strings.Contains(res.stdout, f.token) {
		f.t.Errorf("the token must not appear in the shim's output")
	}
	b, err := os.ReadFile(f.markerPath())
	if err != nil {
		f.t.Fatalf("the marker should exist: %v", err)
	}
	m := shimMarkerLine.FindStringSubmatch(strings.TrimSuffix(string(b), "\n"))
	if m == nil || strings.Count(string(b), "\n") != 1 {
		f.t.Fatalf("the marker should be one line `<time>\\t<N>`, got %q", b)
	}
	if m[1] != strconv.Itoa(n) {
		f.t.Errorf("the marker should record N=%d, got %q", n, b)
	}
	if fi, err := os.Stat(f.markerPath()); err == nil && fi.Mode().Perm() != 0o600 {
		f.t.Errorf("the marker should be 0600, got %v", fi.Mode().Perm())
	}
}

func (f *shimFixture) expectNoVolumeOutput(res shimResult) {
	f.t.Helper()
	if strings.Contains(res.stderr, "gh shim: volume") {
		f.t.Errorf("no volume line expected, got %q", res.stderr)
	}
	if _, err := os.Stat(f.markerPath()); err == nil {
		f.t.Errorf("no marker expected")
	}
}

// R-23 scenario "the volume line is reached": 181 lines seeded, the call
// mints the 182nd, one warning prints with count 182 and the marker records
// 182; the next call that day prints none.
func TestShim_R23_WarnsAtN182AndOnceADay(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(181)
	res := f.run("api", "user")
	if res.exit != 0 || f.ghRuns() != 1 {
		t.Fatalf("gh should still run: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
	}
	f.expectWarning(res, 182, "3.0")
	before, _ := os.ReadFile(f.markerPath())
	next := f.run("api", "user")
	if strings.Contains(next.stderr, "gh shim: volume") {
		t.Errorf("a later call that day prints nothing, got %q", next.stderr)
	}
	after, _ := os.ReadFile(f.markerPath())
	if string(before) != string(after) {
		t.Errorf("the marker must not be rewritten: %q then %q", before, after)
	}
}

// R-23: not at N = 181 (180 seeded, the call mints the 181st).
func TestShim_R23_SilentAtN181(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(180)
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("exit %d stderr=%q", res.exit, res.stderr)
	}
	f.expectNoVolumeOutput(res)
}

// R-23, the seeded copy: reads-per-mint 3.4 fires at N = 154, not 153. A
// truncating shim (3) would wait for 182.
func TestShim_R23_CopyAtThreePointFour_FiresAt154NotAt153(t *testing.T) {
	copyAt := func(f *shimFixture) {
		f.shim = f.editedCopy("tenths-34", [2]string{shimTenthsLine, "_sgs_reads_per_mint_tenths=34\n"})
	}
	t.Run("154 fires", func(t *testing.T) {
		f := newShimFixture(t)
		copyAt(f)
		f.seedMinted(153)
		f.expectWarning(f.run("api", "user"), 154, "3.4")
	})
	t.Run("153 is silent", func(t *testing.T) {
		f := newShimFixture(t)
		copyAt(f)
		f.seedMinted(152)
		f.expectNoVolumeOutput(f.run("api", "user"))
	})
}

// R-23 as a property: over reads-per-mint values and N around and away from the
// boundary, the warning prints exactly when (N + 52) x tenths >= 7000.
func TestShim_R23_FiresExactlyWhenTheInequalityHolds(t *testing.T) {
	rng := rand.New(rand.NewSource(4741))
	for _, tenths := range []int{10, 20, 30, 34, 50, 70} {
		boundary := (7000+tenths-1)/tenths - 52 // smallest N that fires
		ns := []int{boundary - 1, boundary, boundary + 1 + rng.Intn(40), 1 + rng.Intn(boundary)}
		for _, n := range ns {
			if n < 1 {
				continue
			}
			t.Run(fmt.Sprintf("tenths=%d/N=%d", tenths, n), func(t *testing.T) {
				f := newShimFixture(t)
				f.shim = f.editedCopy("tenths", [2]string{shimTenthsLine, fmt.Sprintf("_sgs_reads_per_mint_tenths=%d\n", tenths)})
				f.seedMinted(n - 1)
				res := f.run("api", "user")
				got := len(shimLinesWith(res.stderr, shimWarnPrefix))
				want := 0
				if shimFires(n, tenths) {
					want = 1
				}
				if got != want {
					t.Errorf("(%d+52)*%d vs 7000: want %d warning line(s), got %d: %q", n, tenths, want, got, res.stderr)
				}
			})
		}
	}
}

// R-23: ten concurrent calls past the line print one warning, and the marker
// is one line.
func TestShim_R23_TenConcurrentCallsPrintOneWarning(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(190)
	const n = 10
	results := make([]shimResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.runErr("api", "user")
		}(i)
	}
	wg.Wait()
	warnings := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil || results[i].exit != 0 {
			t.Fatalf("call %d: err=%v exit=%d", i, errs[i], results[i].exit)
		}
		warnings += len(shimLinesWith(results[i].stderr, shimWarnPrefix))
		if v := shimLinesWith(results[i].stderr, shimVolPrefix); len(v) != 0 {
			t.Errorf("call %d printed a failure line: %q", i, v)
		}
	}
	if warnings != 1 {
		t.Fatalf("ten concurrent calls past the line should print one warning, got %d", warnings)
	}
	b, err := os.ReadFile(f.markerPath())
	if err != nil || len(shimMarkerLine.FindStringSubmatch(strings.TrimSuffix(string(b), "\n"))) == 0 {
		t.Errorf("the marker should be one well-formed line, got %q (%v)", b, err)
	}
}

// R-23 Failures: the marker's path occupied by a directory, a minting call past
// the line prints a `gh shim: volume:` line and continues.
func TestShim_R23_MarkerPathIsADirectory_PrintsVolumeLine(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(181)
	if err := os.Mkdir(f.markerPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	res := f.run("api", "user")
	if res.exit != 0 || f.ghRuns() != 1 {
		t.Fatalf("gh should still run: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
	}
	if v := shimLinesWith(res.stderr, shimVolPrefix); len(v) != 1 {
		t.Errorf("expected exactly one %q line, got %q in %q", shimVolPrefix, v, res.stderr)
	}
	if strings.Contains(res.stderr, f.token) {
		t.Errorf("the token must not appear on stderr")
	}
}

// R-23 Failures, other half: when N cannot be counted the shim prints a
// `gh shim: volume:` line, creates no marker and continues. The fake readers
// fail any use that names a record file.
func TestShim_R23_CountFailure_PrintsVolumeLine(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(181)
	for _, name := range []string{"awk", "grep", "wc", "sed", "cat", "tr", "sort", "uniq", "head"} {
		wrapper := filepath.Join(f.bin, name)
		if _, err := os.Stat(wrapper); err != nil {
			continue
		}
		renamed := filepath.Join(f.fx, "real-"+name)
		if err := os.Rename(wrapper, renamed); err != nil {
			t.Fatal(err)
		}
		shimWriteExec(t, wrapper, "for a in \"$@\"; do case $a in *.log) exit 2;; esac; done\nexec '"+renamed+"' \"$@\"\n")
	}
	res := f.run("api", "user")
	if res.exit != 0 || f.ghRuns() != 1 {
		t.Fatalf("gh should still run: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
	}
	if v := shimLinesWith(res.stderr, shimVolPrefix); len(v) != 1 {
		t.Errorf("expected exactly one %q line, got %q in %q", shimVolPrefix, v, res.stderr)
	}
	if len(shimLinesWith(res.stderr, shimWarnPrefix)) != 0 {
		t.Errorf("no warning without a count: %q", res.stderr)
	}
	if _, err := os.Stat(f.markerPath()); err == nil {
		t.Errorf("no marker without a count")
	}
}

// R-22: with the record unwritable the threshold check is skipped, so no
// marker, no warning and no `gh shim: volume` line, only the record line.
func TestShim_R22_UnwritableRecord_SkipsTheVolumeCheck(t *testing.T) {
	f := newShimFixture(t)
	parent := filepath.Dir(f.recordDir())
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.recordDir(), []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := f.run("api", "user")
	if res.exit != 0 || f.ghRuns() != 1 {
		t.Fatalf("gh should still run: exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
	}
	if len(shimLinesWith(res.stderr, "gh shim: record:")) != 1 {
		t.Errorf("the record line should print once: %q", res.stderr)
	}
	if strings.Contains(res.stderr, "gh shim: volume") {
		t.Errorf("the volume check must be skipped, got %q", res.stderr)
	}
}

// A call that records no mint (mint-failed) never reaches the volume check,
// however long today's record is.
func TestShim_R23_FailedMintDoesNotWarn(t *testing.T) {
	f := newShimFixture(t)
	f.seedMinted(300)
	f.setHelper("exit 1")
	res := f.run("api", "user")
	f.expectRefused(res, "mint-failed")
	f.expectNoVolumeOutput(res)
}

// R-24 shared seeded control: a copy that also prints the token on the
// volume-warning line makes the no-token scan fail.
func TestShim_R24_SeededControl_TokenOnTheWarningLineIsCaught(t *testing.T) {
	f := newShimFixture(t)
	f.shim = f.editedCopy("volume-line-leak", [2]string{
		`'gh shim: volume warning: `,
		`'gh shim: volume warning: '"$` + shimTokenVar + `"'`,
	})
	f.seedMinted(181)
	res := f.run("api", "user")
	lines := shimLinesWith(res.stderr, shimWarnPrefix)
	if len(lines) != 1 || !strings.Contains(lines[0], f.token) {
		t.Errorf("the control copy should leak the token on the warning line, got %q", lines)
	}
}

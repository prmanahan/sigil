package main_test

// R-21 and R-23: one UTC day per call. A call that crosses UTC midnight must
// write its record line and count today's minted lines in the same day's file,
// and name its marker for that day. The fake date below answers the first
// date-and-time question with one day and every later one with the next day, as
// a call that crosses midnight would see.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	shimDayA = "2031-01-01"
	shimDayB = "2031-01-02"
)

// installMidnightDate makes the fake date answer the first `+%Y...` question
// with shimDayA and every later one with shimDayB, in the format asked for, and
// pass every other question (the `+%s` clock) to the real date.
func (f *shimFixture) installMidnightDate() {
	f.t.Helper()
	counter := filepath.Join(f.fx, "midnight.count")
	body := "case \"$*\" in\n  *%Y*) ;;\n  *) exec '" + shimMustLook(f.t, "date") + "' \"$@\" ;;\nesac\n" +
		"n=0\n[ -f '" + counter + "' ] && read n < '" + counter + "'\nn=$((n + 1))\necho \"$n\" > '" + counter + "'\n" +
		"day=" + shimDayA + "\n[ \"$n\" -gt 1 ] && day=" + shimDayB + "\n" +
		"for a in \"$@\"; do\n  case $a in\n" +
		"    +%Y-%m-%dT%H:%M:%SZ) printf '%sT00:00:00Z\\n' \"$day\" ;;\n" +
		"    +%Y-%m-%d) printf '%s\\n' \"$day\" ;;\n" +
		"  esac\ndone\n"
	shimWriteExec(f.t, filepath.Join(f.bin, "date"), body)
}

func (f *shimFixture) seedMintedOn(day string, n int) {
	f.t.Helper()
	if err := os.MkdirAll(f.recordDir(), 0o700); err != nil {
		f.t.Fatal(err)
	}
	line := fmt.Sprintf("%sT00:00:00Z\tfixowner/fixrepo\tnarrowed\tminted\t1\n", day)
	if err := os.WriteFile(filepath.Join(f.recordDir(), day+".log"), []byte(strings.Repeat(line, n)), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// A call whose later date readings fall on the next day writes its line to the
// first day's file, counts that file, and warns with that day's marker: here the
// first day holds 181 minted lines, so this call's line is the 182nd and the
// warning fires. A second day's file or marker is never made.
func TestShim_R21_R23_ACallCrossingMidnightUsesOneDay(t *testing.T) {
	f := newShimFixture(t)
	f.installMidnightDate()
	f.seedMintedOn(shimDayA, 181)
	res := f.run("api", "user")
	if res.exit != 0 || f.ghRuns() != 1 {
		t.Fatalf("exit %d runs %d stderr=%q", res.exit, f.ghRuns(), res.stderr)
	}
	if lines := shimLinesWith(res.stderr, shimWarnPrefix); len(lines) != 1 || !strings.Contains(lines[0], " 182 ") {
		t.Errorf("the call should count 182 minted lines in the first day's file and warn once, got %q (all stderr %q)", lines, res.stderr)
	}
	b, err := os.ReadFile(filepath.Join(f.recordDir(), shimDayA+".log"))
	if err != nil || strings.Count(string(b), "\n") != 182 {
		t.Errorf("the first day's log should hold 182 lines, got %d (err %v)", strings.Count(string(b), "\n"), err)
	}
	if m, err := os.ReadFile(filepath.Join(f.recordDir(), shimDayA+".warned")); err != nil || !strings.HasSuffix(strings.TrimSpace(string(m)), "\t182") {
		t.Errorf("the first day's marker should record 182, got %q (err %v)", m, err)
	}
	for _, name := range []string{shimDayB + ".log", shimDayB + ".warned"} {
		if _, err := os.Stat(filepath.Join(f.recordDir(), name)); err == nil {
			t.Errorf("%s must not exist: the call used one day", name)
		}
	}
}

// Control: a copy that asks the clock for the day again at the count, as the
// shim did, reads the second day's file and misses the 182nd line.
func TestShim_R21_R23_MidnightControl_ASecondDayReadingCountsTheWrongFile(t *testing.T) {
	f := newShimFixture(t)
	f.installMidnightDate()
	f.seedMintedOn(shimDayA, 181)
	f.shim = f.editedCopy("second-day-reading", [2]string{`_sgs_vol_day=$_sgs_day`, `_sgs_vol_day=$(command date -u +%Y-%m-%d)`})
	res := f.run("api", "user")
	if res.exit != 0 {
		t.Fatalf("control copy exit %d stderr=%q", res.exit, res.stderr)
	}
	if lines := shimLinesWith(res.stderr, shimWarnPrefix); len(lines) != 0 {
		t.Errorf("the control copy should miss the count and not warn, got %q", lines)
	}
}

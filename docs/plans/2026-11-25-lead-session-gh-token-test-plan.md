---
test_plan: lead-session-gh-token
status: active
author: Glitch
derived_from:
  - docs/specs/lead-session-gh-token.md (locked, blob 725bf343a3ff06fb2136321c6050725033b32c27)
  - the locked Frame, wording-amended form (FD-n Observable lists and LT rows, read as the spec adopts them)
plan: docs/plans/2026-11-25-lead-session-gh-token-plan.md (T1; read by T3, T6 and T8)
code_read: none. Written from the spec and the Frame only, before the build.
---

# Test plan: narrowed GitHub token for lead sessions' `gh` work

## 1. What this document is

The edge areas of the feature, each traced to a requirement, written before any build. T3 (landing mint), T6 (the `gh` shim) and T8 (S7's command) write their edge tests from it. It is a plan for tests; it contains no test code.

Identifiers: `R-n` is the spec's requirement. `FD-n`, `LT-n`, `KG-n` and `RR-n` keep the Frame's meaning. `TP-<area>-<n>` is this plan's scenario id, and a test's name or header comment carries it together with its `R-n`.

Three ways in exist, and each gets edge scenarios:

| Way in | Artifact driven from outside | Area | Task |
|---|---|---|---|
| `sigil token --landing <owner>/<repo>` | the built `sigil` binary, as a process | TP-L | T3 |
| the `gh` shim, with its record and marker | the tracked shim script, run in place as a process | TP-S | T6 |
| S7's command | the tracked command script, run in place as a process | TP-C | T8 |

The wiring line (R-25), the README (R-27) and the run record (R-29 to R-38) are not edge-testable artifacts. Section 4 says where each runs.

## 2. Form and harness rules

**Form.** The spec's "Where acceptance runs" puts R-1 to R-5, R-8 to R-28 and R-39 in `just check`, and the plan puts the edge tests in Go test files beside the existing ones. The edge tests therefore drive each artifact through `os/exec` from Go tests, in `package main_test`. A stdlib Python `unittest` file would be the default form for a binary, but it cannot run in `just check`, and the spec forbids changing that recipe. Gap G-9 records this.

**Black-box.** An edge test spawns the artifact and asserts on its exit code, stdout, stderr, the files it writes under a temporary `HOME`, and the environment and argv a fake child records. It never imports or calls product code, and never ports a decision from it.

**Fails, not skips, when the artifact is missing.**
- Each area opens with a guard test (TP-L0, TP-S0, TP-C0): the artifact exists and is executable, and the test fails, with a message naming the path, when it is not.
- A liveness probe by `--help` is not available. A bare `sigil` run reads stdin as a credential helper, the shim would run its own steps, and the command's usage is unspecified. The guard checks existence and the executable bit, and the guard test's own failure is shown once by pointing it at a missing path.
- On macOS no test of these areas skips, except TP-L8 and TP-L9, which are skip-gated by the spec (R-5). A missing `sh`, `git` or `lsof` fails the test. On Linux the harness puts a fake `lsof` on `PATH` (Constraints).

**Fresh state per test.** Each test builds its own temporary root, with `HOME` and `TMPDIR` inside it, its own fixture clones made with `git` at test time, its own fake harness process, and its own fakes on `PATH`. No ordering dependency between tests. Tests that set environment variables do not run in parallel.

**Clean environment.** Each child starts from an explicit environment, not the runner's. In particular the harness removes `OP_SERVICE_ACCOUNT_TOKEN` for TP-L5 to TP-L7, because in the skip-gated environment it is set and would change the order those tests observe. The skip-gated tests TP-L8 and TP-L9 are the only ones that pass it on.

**Fakes** (shim and command areas only):
- a fake harness process, a child the test starts with its working directory in a fixture clone, named by `CLAUDE_PID`;
- a fake `sigil-helper` on `PATH` that records its argv, prints a runtime-generated token, and can be told to exit non-zero, print two lines, and so on;
- a recording fake `gh`, supplied through `SIGIL_GH_REAL`, that records argv, its environment and its own exit status;
- the real `lsof` on macOS, and a fake from `/proc` on Linux.

**Fixture tokens** are built at runtime, are not `ghs_`-shaped, and add no scanner allowlist entry or `gitleaks:allow` comment. The fixture owner and repository names are invented. No secret is committed.

**No fixed waits.** The 20-way concurrency and the fake harness process are synchronised on a condition with a timeout.

**Expectations are written by hand from the spec.** No overwrite or snapshot mode is used.

**Declared reads.** The edge author may read two things that are not the spec, and states each in the test file's header: the existing CLI test harness, to learn how the built binary's path is exposed (T3 only); and, if a test needs the exact text of a shim constant, the shim, with a spec-gap note (G-5). Nothing else of the implementation is read.

**Clock trap.** Record and marker files are keyed by the UTC date. A test reads the UTC date before and after its call and reports the run `inconclusive`, never failed, if they differ, which only happens within a second of 00:00 UTC. The test is then re-run, not widened.

## 3. Scenarios

Each scenario is one test function. `AC` says whether the plan's task acceptance criteria already name it; `plan-only` means the scenario traces to the spec and to T1's list of areas, but the build task's criteria do not list it (section 7).

### 3.1 TP-L: `sigil token --landing <owner>/<repo>` (T3)

| ID | Req | Scenario | AC |
|---|---|---|---|
| TP-L0 | R-1 | **Given** the built `sigil` path, **when** the test starts, **then** it exists and is executable, or the test fails naming it | T3 |
| TP-L1 | R-1 | **Given** an empty `HOME`, **when** `token --landing myorg/myrepo` runs, **then** it exits non-zero, stderr holds `open config` and not `usage`, stdout is empty. The positive control for TP-L2 to TP-L4 | T3 |
| TP-L2 | R-1 | **Given** an empty `HOME`, **when** `token --landing=o/r` runs, **then** non-zero, stdout empty, stderr holds `usage` and not `open config` | T3 |
| TP-L3 | R-1 | **Given** an empty `HOME`, **when** `token --landing a/b/c` runs, **then** the same as TP-L2 | T3 |
| TP-L4 | R-1 | **Given** an empty `HOME`, **when** `token -x` runs, **then** the same as TP-L2. This is R-1's one change to the existing single-argument form | T3 |
| TP-L5 | R-2 | **Given** a config that routes one known owner and no service-account token, **when** `token --landing unknown-owner/r` runs, **then** non-zero, stderr holds `unknown owner`, stdout empty | T3 |
| TP-L6 | R-2 | **Given** a config, an API base that is not https, and an owner that is also unknown, **when** `token --landing unknown-owner/r` runs, **then** stderr holds `https scheme required` and not `unknown owner`. Both faults at once, so the observed message shows the scheme check runs before routing | T3 (pins order) |
| TP-L7 | R-2 | **Given** a known owner, no service-account token and stdout a pipe, **when** `token --landing <owner>/r` runs, **then** stderr holds `OP_SERVICE_ACCOUNT_TOKEN is not set` and not `stdout is not a terminal`, stdout empty | T3 |
| TP-L8 | R-5 | *Skip-gated on `OP_SERVICE_ACCOUNT_TOKEN` and `SIGIL_TEST_OP_KEY_REF`.* **Given** a mock GitHub returning a grant equal to the contract, and stdout a pipe, **when** `token --landing <owner>/<repo>` runs, **then** exit 0, stdout is exactly the token and one newline, stderr is empty, and the temporary `HOME`'s cache directory holds no new file | T3 |
| TP-L9 | R-4, R-5 | *Skip-gated, same variables.* **Given** a mock GitHub whose response adds one permission to the contract, **when** the same command runs, **then** exit non-zero, stdout empty, stderr names no token, and the mock received exactly one revoke carrying the minted token | T3 |
| TP-L10 | R-1 | **Given** an empty `HOME`, **when** `token someowner` runs, **then** stderr holds `open config` and not `usage`: a single argument that does not begin with `-` still reaches the owner form | plan-only |

Count: 11 scenarios, of which 1 is a guard, 9 are in T3's criteria and 1 is plan-only. R-1 has 5 (L0 aside: L1 to L4 and L10), R-2 has 3, R-4 and R-5 have 2.

The "unattended landing mint" and "GitHub widens the grant" scenarios of the spec reach the built binary only through TP-L8 and TP-L9, which skip without real 1Password. They run live at `LT-1`.

### 3.2 TP-S: the `gh` shim (T6)

Every failure scenario also asserts: exit non-zero; the fake `gh` was not started; and, where the class is after step 2, the fake `sigil-helper` was not called. Every failure scenario asserts that stderr holds exactly one line beginning `gh shim: <class>:` (the helper's own passed-through stderr in TP-S6 is not counted).

| ID | Req | Scenario | AC |
|---|---|---|---|
| TP-S0 | R-8 | **Given** the tracked shim path, **when** the test starts, **then** it exists and is executable, or the test fails naming it. The tests run that file in place, never a copy | T6 |
| **Failure classes** | | | |
| TP-S1 | R-9, R-17 | **Given** `HOME` empty and a valid real-`gh` path, **when** a call runs, **then** stderr begins `gh shim: home:`, no helper call, no record file written | T6 |
| TP-S2 | R-9, R-10 | **Given** `HOME` set and no override or install-time path, **when** a call runs, **then** stderr begins `gh shim: real-gh:`, no helper call | T6 |
| TP-S3 | R-9, R-39 | **Given** `SIGIL_GH_REAL` set to the shim's own path, **when** a call runs, **then** `gh shim: real-gh:`, no helper call | T6 |
| TP-S4 | R-9, R-18 | **Given** a fake harness process whose working directory is outside any clone, **when** `gh api user` runs, **then** `gh shim: no-repo:`, no helper call, and the record's one new line has outcome `no-repo` with an empty repository field | T6 |
| TP-S5 | R-9, R-11 | **Given** a resolving clone, **when** `gh auth setup-git` runs, **then** `gh shim: refused:`, no helper call, the record's line has outcome `refused` | T6 |
| TP-S6 | R-9, R-12 | **Given** a fake helper that prints a token and then exits non-zero, **when** a call runs, **then** `gh shim: mint-failed:`, the helper's own stderr text is on the shim's stderr, the record's line has outcome `mint-failed`, and the helper's argv was exactly `token --landing <fixture owner/repo>` | T6 |
| **Step order** | | | |
| TP-S7 | R-9 | **Given** `HOME` set, no real-`gh` path, and a harness process outside a clone, **when** a call runs, **then** stderr begins `gh shim: real-gh:`, not `no-repo` | T6 (R-9's bullet) |
| TP-S8 | R-9 | **Given** a harness process outside a clone, **when** `gh auth token` runs, **then** `gh shim: no-repo:` and the record says `no-repo`, not `refused` | T6 (R-9's bullet) |
| TP-S9 | R-9 | **Given** `HOME` empty and no real-`gh` path, **when** a call runs, **then** `gh shim: home:` | T6 (R-9's bullet) |
| **Success path** | | | |
| TP-S10 | R-13 | **Given** a good fake helper and seeded `GITHUB_TOKEN`, `GH_HOST`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_TOKEN` (a different value), `GH_CONFIG_DIR`, `XDG_DATA_HOME` and `GH_PROMPT_DISABLED=0`, **when** a call runs, **then** the fake `gh`'s recorded environment has `GH_TOKEN` equal to the helper's line with no line ending, the four removed names absent, `GH_CONFIG_DIR` equal to `$HOME/.config/sigil/gh`, `XDG_DATA_HOME` equal to `$HOME/.local/share/sigil/gh-data`, `GH_PROMPT_DISABLED` equal to `1` | T6 |
| TP-S11 | R-15 | **Given** a fake `gh` that exits with a chosen status, **when** a call with `-R o/other`, `--repo=o/other`, a URL and a `repos/...` argument runs, **then** the fake records exactly that argv and the shim exits with the fake's status | T6 |
| TP-S12 | R-14 | **Given** a successful call, **when** it ends, **then** the token is in neither fake's argv, not in the shim's own stdout or stderr (the fake `gh`'s output excluded), and in no file under the test root with `TMPDIR` inside it. Control: the same file scan finds a token the test plants under `TMPDIR` | plan-only |
| TP-S13 | R-16 | **Given** two consecutive calls, **then** the fake helper ran twice, the two tokens differ, and neither is found under the test root afterwards, with TP-S12's scan and control | plan-only |
| TP-S14 | R-19 | **Given** a session in clone A, **when** calls run with `GH_REPO=other/x`, from another clone's directory, and with `-R other/x`, **then** each time the helper receives A's `owner/repo` and the fake `gh` receives the argv unchanged | plan-only |
| **Record** | | | |
| TP-S15 | R-21 | **Given** a `minted` call, **then** the day's file `$HOME/.local/state/sigil/gh-shim/<UTC date>.log` gains exactly one line of five tab-separated fields: a UTC ISO 8601 time with `Z`, the session's repository, `narrowed`, `minted`, a non-empty decimal mint time | T6 |
| TP-S16 | R-21 | **Given** a `mint-failed` call, **then** one line, repository the session's, class `none`, outcome `mint-failed`, mint time non-empty | T6 |
| TP-S17 | R-21 | **Given** a `no-repo` call, **then** one line, repository empty, class `none`, outcome `no-repo`, mint time empty | T6 |
| TP-S18 | R-21 | **Given** a `refused` call, **then** one line, repository the session's, class `none`, outcome `refused`, mint time empty | T6 |
| TP-S19 | R-21 | **Given** a pre-existing state directory at `0755` and a record file at `0644`, **when** a call runs, **then** they read back `0700` and `0600` | plan-only |
| TP-S20 | R-22 | **Given** the state directory's path occupied by a regular file, **when** a call runs, **then** the fake `gh` still runs, stderr holds exactly one line beginning `gh shim: record:`, and no token appears in it | plan-only |
| **Volume warning and marker** | | | |
| TP-S21 | R-23, R-24 | **Given** today's record seeded with 181 well-formed `minted` lines (the format is the spec's Data Model), **when** a minting call runs, **then** stderr holds exactly one line beginning `gh shim: volume warning:` that holds `182` and `maintainer` and no token, and the file `<UTC date>.warned` holds one line: a creation time, a tab, `182` | T6 |
| TP-S22 | R-23 | **Given** TP-S21's state after its call, **when** a second minting call runs the same UTC day, **then** no `volume warning` line is printed | T6 |
| TP-S23 | R-23 | **Given** today's record seeded with 180 `minted` lines, **when** a minting call runs, **then** (N is 181) no warning is printed and no marker exists. The control for TP-S21 | plan-only |
| TP-S24 | R-23 | **Given** the marker's path occupied by a directory and today's record seeded past the line, **when** a minting call runs, **then** stderr holds a line beginning `gh shim: volume:` and the fake `gh` still runs | plan-only |

Count: 25 scenarios. By way in: guard 1; failure classes 6 (`home` 1, `real-gh` 2, `no-repo` 1, `refused` 1, `mint-failed` 1); step order 3; success path 5 (env, argv and status, no leak, per-call mint, pin); record 6; volume 4. Of the 25, 18 are in T6's criteria (the guard included) and 7 are plan-only.

### 3.3 TP-C: S7's command (T8)

The command takes the expected session `owner/repo` and the path of a clone of a different repository (R-28). Its invocation form is not specified (G-1), so each test calls it as the builder's usage defines, recorded in the test header. Each case builds fakes on `PATH` and a deployed copy of the shim, and asserts that the command's stdout and stderr hold the fake helper's runtime token nowhere.

| ID | Req | Scenario | AC |
|---|---|---|---|
| TP-C0 | R-28 | **Given** the tracked command path, **when** the test starts, **then** it exists and is executable, or the test fails naming it | T8 |
| TP-C1 | R-28 | **Given** a deployed shim byte-equal to the tracked one, a fake `gh` that prints the expected repository on the canary call and something other than `User` on the login call, and a clone of a different repository as the working directory, **when** the command runs, **then** exit 0. The positive control | T8 |
| TP-C2 | R-28 | **Given** TP-C1's state with the deployed shim differing by one byte, **then** exit 1 and stderr names `byte-compare` | T8 |
| TP-C3 | R-28 | **Given** TP-C1's state with a fake `gh` that prints another repository on the canary call, **then** exit 1 and stderr names `pin-canary` | T8 |
| TP-C4 | R-28 | **Given** TP-C1's state with a fake `gh` that prints `User` for `api user --jq .type`, **then** exit 1 and stderr names `login-check` | T8 |
| TP-C5 | R-28 | **Given** a given clone whose `origin` resolves to the expected repository, **then** exit 2, stderr names `preconditions`, and the fake `gh` and fake helper were never invoked | T8 |

Count: 6 scenarios, 1 guard and 5 in T8's criteria, one per exit class and check name.

## 4. Where each criterion runs

The spec's "Where each criterion runs" gives four places, plus the *macOS runtime* condition. This is that assignment, by requirement, with the edge scenario that drives it. "Unit" means the build task's own tests (T2, T4, T5, T7), written by the implementer.

| Requirement | Runs in | Edge scenario |
|---|---|---|
| R-1 | `just check` (unit for dispatch; built binary) | TP-L1 to TP-L4, TP-L10 |
| R-2 | `just check` built binary for order; lead-run at the pull request for the single bounded context | TP-L5 to TP-L7 |
| R-3 | `just check`, unit only (recording mock) | none |
| R-4 | `just check`, function level only | TP-L9 reaches it through the built binary, skip-gated |
| R-5 | `just check` for `captureStd` and cache bytes (unit); **skip-gated** for the built binary | TP-L8, TP-L9 |
| R-6, R-7 | lead-run at the pull request; every pre-existing test passes in `just check` | none |
| R-8 | `just check`; lead-run reads that the tests run the tracked file in place | TP-S0 |
| R-9, R-10, R-11, R-12, R-17, R-18, R-39 | `just check` | TP-S1 to TP-S9 |
| R-13 | `just check`; the `SHELLOPTS=xtrace` helper-secret test is *macOS runtime* | TP-S10 |
| R-14 | `just check`; the static check and its 20 seeded copies are unit; the two `SHELLOPTS` tests are *macOS runtime*; the inherited-export test runs on every platform | TP-S12 |
| R-15 | `just check` | TP-S11 |
| R-16 | `just check` | TP-S13 |
| R-19 | `just check`; the call aimed at another repository is **live** at `LT-10` | TP-S14 |
| R-20 | `just check`, in two halves (shim half in T4, R-4's letter-case case in T2). The whole chain is not run anywhere in the gate | none |
| R-21, R-22 | `just check` | TP-S15 to TP-S20 |
| R-23, R-24 | `just check`; the human reader of the marker is parked | TP-S21 to TP-S24 |
| R-25 | **live** at `LT-6` | none |
| R-26 | **live**, with a seeded control | none |
| R-27 | lead-run at the pull request | none |
| R-28 | `just check` | TP-C1 to TP-C5 |
| R-29 | lead-run, once the second repository is chosen and at every later landing | none |
| R-30, R-31, R-32, R-33, R-34, R-35, R-36, R-37, R-38 | lead-run or **live**, read from the task record | none |

The three *macOS runtime* tests (R-13's `SHELLOPTS=xtrace`, R-14's `SHELLOPTS=xtrace`, R-14's `SHELLOPTS=allexport`) belong to T4. They are not duplicated at the edge. On Linux they skip by the spec's own rule. Their coverage claim is only the lead's anchored `-v` read.

The skip-gated tests (TP-L8, TP-L9) take the macOS skip count from 2 to 3. That count, by the repository's `grep -c '^--- SKIP'` method, is a check the lead makes at the build's close.

Live runs that touch these ways in: `LT-1` (the mint), `LT-5` (the failure classes and refusals with the login in place), `LT-6` (the wiring), `LT-7` (reads per mint and wall time), `LT-10` (the pin and the no-clone and SSH cases).

## 5. Edge cases routed to the unit tests

Per "each way in once", these are not repeated at the edge. They go to the build tasks' unit tests; this list is what the plan names for the implementers.

**To T2 (landing mint):**
- every one-field difference in R-4: an extra permission key, a missing key, a changed level, `repository_selection: all`, a repository count of 0 and of 2, a name differing only in letter case; each rejected with `grant mismatch` and exactly one revoke carrying the minted token;
- a matching public response and a matching private response each return the token;
- a revoke with a status other than 204, and a revoke failing in transport, give `revoke failed` (the "revoke fails" scenario is reachable only here);
- a 201 with an empty token errors with no revoke;
- a 401 retried exactly once with a fresh JWT;
- no returned error contains the token or a granted permission level, with the control that the same scan finds the token in the mock's body;
- R-3's request body equals the contract field for field and holds no other field;
- `captureStd` shows the mint function writes to neither stream, and the cache directory is byte-identical before and after;
- R-1's remaining shapes: `--landing` alone, no argument, extra arguments, a `--landing` second argument that fails `owner/repo` parsing;
- the usage text names all three forms (its wording is not pinned by the spec, so no edge test asserts it);
- R-6's existing tests pass unmodified.

**To T4 and T5 (shim):**
- each R-10 shape: relative path, missing file, non-executable file, directory; each exits non-zero with no helper call;
- R-39's symbolic link to the shim;
- `gh auth token`, `gh extension install o/r`, `gh extensions`, `gh ext`, each refused, with `gh api user` as the unrefused control;
- R-12's other rejected outputs: two lines, a carriage return, a space or tab, empty output; and the accepted token with no trailing newline;
- R-17's `HOME` unset, relative, and two-line;
- every R-18 and `FD-7` refusal: each bad `CLAUDE_PID` shape, the reader absent, failing, silent, two-line and relative, no `origin`, SSH and non-GitHub `origin`, each invalid name half, a `.git` suffix, a subdirectory working directory, `insteadOf`, and the injected `GIT_` variables with a clone and with no clone;
- R-20's shim half: a mixed-case `origin` reaches the helper unchanged;
- R-21's twenty concurrent calls producing twenty whole lines;
- R-13's `BASHOPTS` and `BASH_XTRACEFD` removal with its control copy;
- R-14's static check and its 20 seeded copies, the three *macOS runtime* tests, the inherited-export test;
- R-23's 3.4 reads-per-mint copy at N = 154 and 153, ten concurrent calls printing one warning, the property over N and reads-per-mint;
- R-24's seeded control on the warning line, R-22's on the `record:` line.

**To T7 (command):**
- the three causes of `pin-canary`: a fake `gh` printing another repository, a failing helper with a seeded expected-repository `minted` line, a record path occupied by a directory;
- the second precondition, a working directory outside the given clone, before any shim call;
- each check failing at its own position with the earlier checks passing;
- the seeded control copy of the command that prints a token.

## 6. Not reachable through the artifact

- The R-20 chain end to end (a mixed-case `origin` through the shim and the real mint to GitHub's refusal) needs a key-source seam that does not exist (R-5). It is checked in two halves only.
- R-4 as a mint through the built binary, outside the skip-gated TP-L9, and the "revoke fails" scenario, are function-level by the spec.
- The wiring line (R-25) is a line in the maintainer's shell start-up file. It is live only. Its two states, a harness session and the maintainer's terminal, are both observed at `LT-6`. No gate test reaches it.
- GitHub's own refusal of a write to another repository (`LT-10`) and every live row.
- Whether the first-statement and `exec` guards hold under a given shell's tracing option is reachable only on macOS (the *macOS runtime* tests); Linux's shell ignores the option.

## 7. Spec gaps

Each: requirement, why it cannot produce a testable scenario or is ambiguous, what would fix it. No requirement of R-1 to R-39 is unreachable by any scenario; the gaps below weaken, narrow or couple a test. The tier is the plan-deviation protocol's, for the lead.

- **G-1. R-28, the command's invocation.** "Take two inputs" names the two inputs and not how they are given (positional order, flags or environment). A black-box test cannot call the command without knowing. Fix: the spec or T7 pins `<owner/repo> <clone path>` positional, or T7 records its usage before T8 starts. T8 then declares the form in its header. Advisory tier; resolved by a one-line decision before T8.
- **G-2. R-23, the marker's creation time.** The Data Model says "the creation time, a tab, and N" and gives no time format. TP-S21 asserts two tab-separated fields, a non-empty first and `182` second, and nothing about the first. Fix: say "the same ISO 8601 form as the record's time field". Advisory.
- **G-3. Record field `mint_seconds`.** "At least whole-second resolution" bounds nothing: any decimal number passes. TP-S15 and TP-S16 assert a non-empty decimal number only. Fix: a lower bound, or an accepted range for a fake helper that sleeps a known time. Advisory.
- **G-4. R-9 and R-12 together.** R-9 says a failure writes "one stderr line beginning `gh shim: <class>:`", and R-12 passes the helper's stderr through, so a `mint-failed` call has more than one stderr line. The plan reads R-9 as "exactly one line begins `gh shim: <class>:`". Fix: say so in R-9. Advisory.
- **G-5. R-23 and R-13, the controls on a copy of the shim.** The spec's controls (reads-per-mint 3.4, the shim without its `unset`, without its first statement) edit a copy of the shim, so they need the exact text of constants and statements the spec does not name. This is a coupling of test to implementation text, not a black-box check. The plan keeps every such control in the build tasks' tests (T4, T5) and out of the edge tests (section 5). Fix: name the reads-per-mint constant and the statements to edit. Advisory.
- **G-6. R-2, the insecure API base.** "An insecure API base" does not name how the test sets it. The plan's go/no-go table names `SIGIL_GITHUB_API_URL`, derived from the source by the plan's author, not from the spec. TP-L6 uses it. Fix: name the override in the spec's Constraints. Advisory.
- **G-7. R-10, names and paths.** The override `SIGIL_GH_REAL` and the install-time file `$HOME/.config/sigil/gh-real-path` are fixed only in the Decision levels table; R-10 itself says "an override environment variable" and "an install-time setting". The tests use the table's names. Fix: cite the row from R-10. Advisory.
- **G-8. R-5's skip-gated test and the https scheme rule.** R-2 requires an https API base, and TP-L8 and TP-L9 need a mock GitHub the binary will accept. The spec does not say how (a TLS test server the binary trusts, or a loopback exception). The T3 author reads the existing CLI harness for this, declares it in the header, and reports the answer. Advisory, and only the two skip-gated tests are affected.
- **G-9. The edge-test form.** The spec's Constraints name `just check` and no language. The plan names Go test files. The default canon order for a binary and a script would be a Python `unittest` file, which cannot run in `just check`, and the spec's out-of-scope list forbids changing that recipe. This plan follows the spec and the plan: Go, through `os/exec`. If the lead wants the Python form, a Constraints line is needed and the gate question (how `just check` would run it) has to be answered first. Material if overturned; the lead decides.
- **G-10. R-28, the preconditions' wording.** The bullets under "Preconditions" read as the conditions that cause an inconclusive result (clone resolves to a different repository: listed as a failure; working directory inside the clone: listed as a failure), while the Acceptance bullet and the pin canary's meaning make them the conditions that must hold. The plan reads them as conditions that must hold, and TP-C5 follows the Acceptance bullet. Fix: reword the two bullets as "holds". Advisory.
- **G-11. R-25, no gate coverage.** The wiring line has no tracked form and no test in `just check`. Both states are observed live at `LT-6`. This is the spec's choice and is recorded, not a defect.

## 8. What T3, T6 and T8 owe back to this plan

- Each test's name or header comment carries its `TP-` id and its `R-n`.
- A scenario of section 3 that a task drops, or one it adds, is reported to the lead rather than quietly changed.
- T3, T6 and T8 each run the missing-artifact check once (the guard test pointed at a missing path fails and names it).
- T6's concurrency, `SHELLOPTS` and static-check items belong to T4 and are not duplicated.
- The three guard scenarios and every `plan-only` scenario are the lead's to accept or drop, since the build tasks' criteria are narrower than the areas T1 names (the plan's T6 criteria omit R-14, R-16, R-19, R-21's modes, R-22 and the below-line and volume-failure cases of R-23).

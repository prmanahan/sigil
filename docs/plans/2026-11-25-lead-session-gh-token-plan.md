---
plan: lead-session-gh-token
status: active
author: Wyrd
task: "#4741 in the maintainer's tracker"
epic: "#4734 in the maintainer's tracker"
form: full
base: 7fd2fa5cb70252ddf795f9b80b341574c9158176
specs:
  - path: docs/specs/lead-session-gh-token.md
    spec_version: "f9ea9ea639b8c9b3bfaa3b09e9318e4a0adecba6 blob 725bf343a3ff06fb2136321c6050725033b32c27"
---

# Plan: Narrowed GitHub token for lead sessions' `gh` work

## Purpose

This plan implements the locked spec `docs/specs/lead-session-gh-token.md`, pinned at lock commit `f9ea9ea639b8c9b3bfaa3b09e9318e4a0adecba6`, blob `725bf343a3ff06fb2136321c6050725033b32c27`. It covers the `sigil token --landing <owner>/<repo>` mint, the `gh` shim with its call record and volume warning, S7's one command, the README setup section, and the live runs that prove them, through to the throwaway repository's deletion and `LT-7`'s one-day half.

**Form.** Full form. Tasks are numbered `T<N>` and the plan's own decisions `D<N>`. `R-N` is the spec's requirement id. `FD-n`, `LT-n`, `KG-n`, `RR-n`, `DEF-1`, `ED-n` and `RISK-x` keep the Frame's meaning.

**Upstream documents.** The spec adopts the Frame's lists "as written at" Frame lock commit `dd9ca140cab2818189d767e751d07fe47a788caf` (tag `brief/lead-session-gh-token/frame-locked`). That commit is provenance only, and no task reads it. Read the Frame and the intake on `main`, at `docs/intent/lead-session-gh-token-frame.md` and `docs/intent/lead-session-gh-token.md`, on `main` since PR #16 as a wording-amended version of the locked Frame. The adopted lists are unchanged in that copy, so reading `main` is reading what the spec adopts (D8).

**Go/no-go on the trunk: go.** Checked against `main` at `7fd2fa5cb70252ddf795f9b80b341574c9158176` by reading files and git objects. No build or test was run in authoring.

| What the spec assumes | Found |
|---|---|
| `dispatchToken` enumerates two forms by exact string match | `main.go#dispatchToken` |
| `parseOwnerRepo` and `revokeNarrowedToken`, reused unchanged | `main.go#parseOwnerRepo`, `token_narrow.go#revokeNarrowedToken` |
| The three bodies R-6 freezes | `token_narrow.go#getRateLimitOnlyToken`, `#exchangeNarrowedToken`, `#verifyNarrowedGrant` |
| R-1 and R-2's pinned stderr markers | `open config` (`config.go`), `unknown owner` (`routing.go`), `https scheme required`, `OP_SERVICE_ACCOUNT_TOKEN is not set`, `stdout is not a terminal` (`main.go`) |
| R-5's test hooks | `captureStd` (`credential_channel_test.go`); `TestCLI_LeakScan_TokenNotInStderr` (`cli_test.go`); the `SIGIL_GITHUB_API_URL` override (`main.go#apiBase`) |
| R-1's change does not break a test R-6 freezes | The only usage assertion in an existing test is a substring check for `usage` (`cli_test.go`, `TestCLI_TokenRateLimitOnly_RejectedShapes_NoConfigLoad`). No test sends a single `-`-prefixed argument to `cmdToken` |
| `--landing` not already built | It does not appear in any `.go` file |
| R-37's `sigil doctor`; R-27's README setup section | `main.go#cmdDoctor`; README `## Setup` |
| `just check` green at base | Inherited from `CLAUDE.md` (measured 2026-11-09: 113 top-level tests pass, 2 skip), not re-run here. Release binding makes a fresh baseline the first gate |

## Not planned

None. The spec set has one member, `docs/specs/lead-session-gh-token.md`, enumerated with `git ls-files -- 'docs/specs/*lead-session-gh-token*'`. It passes both limbs of the lock test: the tag resolves to `f9ea9ea639b8c9b3bfaa3b09e9318e4a0adecba6`, and `git diff --quiet` against the tag exits 0.

## Not covered

None. Every requirement R-1 to R-39 is cited by a task's acceptance criteria or a `D<N>` row. The coverage map below lists the owner of each.

## Unwired tasks

None. Every code task is reachable through a shipped entry point from its own landing:
- T2 through the built `sigil` binary;
- T4 and T5 through the tracked shim, which the Go tests run in place (R-8);
- T7 through the tracked command.

T1, T9 and T10 to T15 change no code: T1, T9 and T10 are documents, and T11 to T15 are live runs and records.

## Coverage map

| Requirement | Owner | Also cited by |
|---|---|---|
| R-1, R-2 | T2 | T3 (built-binary criteria, D2) |
| R-3, R-4 | T2 | — |
| R-5 | T2 | T3 (skip-gated criterion, D2) |
| R-6, R-7 | D3 | T2 |
| R-8 to R-22, R-39 | T4 | T6 (black-box, D2). R-20's second half is R-4's letter-case case in T2 |
| R-23, R-24 | T5 | T6 (D2) |
| R-25, R-26 | T12 | — |
| R-27 | T9 | T12 (deploy and reversal order) |
| R-28 | T7 | T8 (D2), T12 (first live run) |
| R-29 | D3 | T12 (first search, once B is chosen) |
| R-30 | D4 | T11, T12, T14 |
| R-31, R-33, R-36 | T13 | — |
| R-32 | T15 | — |
| R-34 | D4 | T12, T15 |
| R-35 | T14 | — |
| R-37 | D4 | T11, T14 |
| R-38 | T10 | T11 (Bolt's walk) |

## Tasks

### T1 — Test plan from the spec

- **Files:** `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md` (new)
- **Type:** test
- **Depends on:** None
- **Delivery:** `test-plan`
- **What:** Glitch writes the feature's test plan from the spec and the Frame on `main` at `docs/intent/lead-session-gh-token-frame.md` (D8), without reading the code. It names the edge areas that T3, T6 and T8 drive. It lands as its own documentation pull request before any build task is dispatched.
- **Reaches:** nothing runnable. It is the input to T3, T6 and T8.
- **Acceptance Criteria:**
  - [ ] The test plan is on `main` at `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md` before T2 or T4 is dispatched (D9).
  - [ ] It names at least one edge case per way in, each anchored to the requirement it comes from:
    - for `sigil token --landing`: argument shapes, ordering, success, widened grant, revoke failure (R-1, R-2, R-4, R-5);
    - for each shim failure class (`home`, `real-gh`, `no-repo`, `refused`, `mint-failed`) and the success path (R-9 to R-19, R-39);
    - for the record line and marker at the boundary (R-21, R-23, R-24);
    - for each R-28 exit class and check name (R-28).
  - [ ] It names which spec criteria run in `just check`, which are skip-gated, which are *macOS runtime* and which are live, as the spec's "Where each criterion runs" assigns them (D2).
  - [ ] It names no repository B, no absolute path under a home directory and no tracker activity id (D3).
- **Test Expectations:** none; this task writes no tests.
- **Anchors:** D2, D8, D9

### T2 — Landing mint: `sigil token --landing <owner>/<repo>`

- **Files:** `main.go` (`dispatchToken`, its usage text, a landing command beside `cmdTokenNarrowed`); a new sibling file such as `token_landing.go`, holding the landing exchange and verifier and reusing `revokeNarrowedToken` unchanged; `token_landing_test.go`; `CLAUDE.md` (layout table, token-path invariants and order, "four token paths"). All advisory.
- **Type:** backend
- **Depends on:** T1
- **Delivery:** `landing-mint`
- **What:** Adds the third enumerated form of `sigil token`. It mints with the fixed five-permission body, verifies GitHub's grant by exact match with private repositories allowed, revokes on mismatch, and prints only on a verified grant. It never touches the cache and never checks for a terminal. The existing rate-limit and full-grant paths are untouched, apart from R-1's refusal of a single `-`-prefixed argument.
- **Reaches:** `sigil token --landing <owner>/<repo>` through the built binary.
- **Acceptance Criteria:**
  - [ ] R-1: `dispatchToken` accepts `--landing <owner>/<repo>` by exact string equality as a third form. Unit tests show each of these rejected without delegating: `--landing=o/r`, `--landing a/b/c`, `--landing` alone, and the single argument `-x`. A single argument not beginning with `-` still delegates to `cmdToken`. The usage text names all three forms.
  - [ ] R-2: the landing command runs config load, scheme check, routing, key read, exchange in that order, with no terminal check. At the pull request, the lead reads one bounded context created before the key read and passed to both the key read and the exchange.
  - [ ] R-3: against a recording mock server, the decoded request body equals the `## API Contract` body for the named repository, field for field, and has no other field.
  - [ ] R-4, scenarios "GitHub widens the grant" and "the revoke fails" (function level):
    - A matching response for a public repository, and one for a private repository, each return the token.
    - Each one-field difference returns no token and an error containing `grant mismatch`, and sends exactly one `DELETE /installation/token` carrying the minted token. The differences: an extra permission key, a missing key, a changed level, `repository_selection: all`, a repository count of 0 and of 2, and a name differing only in letter case.
    - A revoke returning a non-204 status, or failing in transport, gives an error containing `revoke failed`.
    - A 201 with an empty token gives an error and sends no revoke.
    - A 401 is retried exactly once, with a freshly minted JWT.
    - No returned error contains the token or a granted permission level. The same substring check finds the token in the mock's response body, which is the control.
  - [ ] R-5, scenario "unattended landing mint" (function level):
    - Under the `captureStd` swap, the mint function returns the token and writes to neither stream.
    - A cache directory seeded with an entry holds the same files, byte for byte, before and after a mint.
  - [ ] R-6 and R-7 hold on this pull request's diff, read by the lead (D3).
- **Test Expectations:** Each R-4 field difference, tested as a property: perturb any single field of a matching response, and the result is rejected with exactly one revoke. Add the permission-map perturbations in both directions, key and level. The 401 retry count and the fresh JWT. The cache byte-equality. The `captureStd` silence. The new arm of `dispatchToken`, including R-1's single-dash refusal. Mutation target: the landing verifier and the `--landing` arm of `dispatchToken`, as convention only (D10). Fixture tokens are built at runtime, are not `ghs_`-shaped, and add no `gitleaks:allow` or ignore entry (D3). Expect 1 to 2 agent runs. The size anchor is `f4bc12b` (task #3647 in the maintainer's tracker), which added `--rate-limit-only` in 1,216 inserted lines across 7 files.
- **Anchors:** D1, D3, D10

### T3 — Landing mint edge tests

- **Files:** `cli_test.go` (new tests, no existing line changed, R-6). Advisory.
- **Type:** test
- **Depends on:** T1, T2
- **Delivery:** `landing-mint`
- **What:** Glitch drives the built binary from the test plan at `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md`: each way in once, plus R-5's skip-gated sibling of the leak scan.
- **Reaches:** the same surface as T2, through the built binary.
- **Acceptance Criteria:**
  - [ ] R-1 (built binary): with an empty `HOME`, `token --landing myorg/myrepo` exits non-zero, and its stderr contains `open config` and not `usage`. This is the positive control. `--landing=o/r`, `--landing a/b/c` and the single argument `-x` each exit non-zero with empty stdout, and with stderr containing `usage` and not `open config`.
  - [ ] R-1, TP-L10 (built binary): with an empty `HOME`, `token someowner` gives stderr containing `open config` and not `usage`. A single argument not beginning with `-` still reaches the owner form.
  - [ ] R-2 (built binary, no service-account token):
    - `--landing unknown-owner/r` gives stderr containing `unknown owner`.
    - An insecure API base gives `https scheme required`.
    - With stdout a pipe and a known owner, stderr contains `OP_SERVICE_ACCOUNT_TOKEN is not set` and not `stdout is not a terminal`.
  - [ ] R-5, *skip-gated* on `OP_SERVICE_ACCOUNT_TOKEN` and `SIGIL_TEST_OP_KEY_REF`: a new sibling of `TestCLI_LeakScan_TokenNotInStderr` runs the built binary on `--landing` against a mock GitHub.
    - On a matching grant, the token is on stdout, absent from stderr, and the exit is 0.
    - On a widened grant, stdout is empty and the exit is non-zero.
    - The test skips without both variables, so the macOS skip count, counted by `CLAUDE.md`'s `grep -c '^--- SKIP'` method, reads 3.
- **Test Expectations:** Black-box only. They must fail when the binary is missing. Fixture tokens follow D3.
- **Anchors:** D2, D5, D15

### T4 — The `gh` shim: resolve, mint, hand off, record

- **Files:** a tracked POSIX shell script named `gh`, at a level-5 path of the builder's choosing (for example `shim/gh`); new Go test files that run it in place, with a fake harness process, a fake `sigil-helper`, a recording fake `gh`, and on Linux a fake `lsof`; the R-14 lexical static check, as Go test code; `CLAUDE.md` (layout table, control surfaces, the shim's invariants). All advisory.
- **Type:** backend
- **Depends on:** T1
- **Delivery:** `gh-shim`
- **What:** Writes the shim, apart from R-9's step 7, which T5 adds in the same delivery (D13). The shim resolves the session's repository from the harness process, mints through `sigil-helper token --landing`, sets `gh`'s environment, hands the token over only in the `exec` assignment, and appends the call record. It needs nothing from T2: the tests use a fake helper.
- **Reaches:** the tracked shim, run in place by `just check` (R-8). It reaches no session until T12 deploys it (D6).
- **Acceptance Criteria:**
  - [ ] R-8: the tracked file's first line is exactly `#!/bin/sh`. At the pull request, the lead reads that the Go tests run the tracked file by its path in the tree, and that only the controls of R-9, R-13, R-14 and R-23 use temporary copies.
  - [ ] R-9: steps 1 to 6 and 8 run in R-9's order, and each failure writes one stderr line beginning `gh shim: <class>:`.
    - A call that is both outside a clone and `gh auth …` records `no-repo`, and its stderr begins `gh shim: no-repo:`.
    - With an empty `HOME` and no real-`gh` path, stderr begins `gh shim: home:`. With `HOME` set and no real-`gh` path, it begins `gh shim: real-gh:`, including when the harness process is outside a clone.
    - The steps-2-and-3-swapped copy gives `gh shim: no-repo:` and fails that check.
    - A failure in steps 1 to 5 never starts the fake `gh`.
  - [ ] R-10: each of these exits non-zero with no helper call: no value from either source, a relative path, a missing file, a non-executable file, a directory. The override is `SIGIL_GH_REAL`. The install-time path is one line in `$HOME/.config/sigil/gh-real-path`.
  - [ ] R-39: the override set to the shim's own path, and to a symbolic link to it, each exit non-zero with `gh shim: real-gh:` and no helper call.
  - [ ] R-11, scenario "`gh auth` or `gh extension` through the shim" (gate half):
    - `gh auth setup-git`, `gh auth token`, `gh extension install o/r`, `gh extensions` and `gh ext` each exit non-zero, record `refused`, and invoke neither fake.
    - `gh api user` is not refused, which is the control.
  - [ ] R-12, scenario "the mint fails" (gate half):
    - The fake helper receives exactly `token --landing <fixture owner/repo>`.
    - These outputs are each rejected with `mint-failed` and no `gh` start: a token followed by a non-zero exit, two lines, and a token ending in a carriage return.
    - A token with no trailing newline is accepted, which is the control.
  - [ ] R-13: every `FD-4` environment Observable holds against the recording fake `gh`, with `GH_CONFIG_DIR=$HOME/.config/sigil/gh` and `XDG_DATA_HOME=$HOME/.local/share/sigil/gh-data`. Seeded values of the ten listed variables are removed or overwritten, `GH_PROMPT_DISABLED` among them, seeded as `0`. Seeded and exported `BASHOPTS=extglob BASH_XTRACEFD=7` reach neither fake, and the copy without the `unset` shows both in the fake helper's environment. The *macOS runtime* `SHELLOPTS=xtrace` helper-secret test passes, with its control.
  - [ ] R-14:
    - The token is in neither fake's argv, and not in the shim's own stdout or stderr.
    - The shared seeded control makes the shim-output scan fail.
    - No file under the test's temporary root holds the token, with `TMPDIR` set inside that root. A token the test plants under `TMPDIR` is found, which is the control.
    - The static check passes on the tracked script and fails on every seeded copy R-14 lists. There are 20 of them, among them `exec env GH_TOKEN=...` and the capture straight into `GH_TOKEN`.
    - The *macOS runtime* `SHELLOPTS=xtrace` and `SHELLOPTS=allexport` tests pass, each with its control.
    - The inherited-export test of the declared token variable passes on every platform, with its control.
  - [ ] R-13 and R-14 (*macOS runtime*): exactly three top-level Go test names are recorded on the task. Each carries exactly one of `R13_Xtrace`, `R14_Xtrace` and `R14_Allexport`, and each segment is carried by exactly one name. The spec's anchored `-v` run on macOS counts exactly three `--- PASS` lines. The lead also reads each body for its `SHELLOPTS` value, its scanned asset and its control (D3).
  - [ ] R-15: the fake `gh` records the shim's argv exactly, `-R`, `--repo=`, URL and `repos/...` arguments included, and the shim's exit status equals the fake `gh`'s.
  - [ ] R-16: two consecutive calls invoke the fake helper twice, and neither token is found under the test's temporary root afterwards, with R-14's `TMPDIR` scan and its control.
  - [ ] R-17: `HOME` unset, `HOME=`, `HOME=relative`, and a two-line `HOME` whose first line is absolute each exit non-zero, with no helper call and no record line.
  - [ ] R-18, scenario "a session outside a clone" (gate half): every item of `FD-7`'s gate Observable list holds against a fake harness process named by `CLAUDE_PID`. So does the two-line `CLAUDE_PID` case. Each refusal records `no-repo` with an empty repository field.
  - [ ] R-19, scenario "a call aimed at another repository" (gate half): in each case `FD-7`'s Observable names, the fake helper receives the session's repository and the fake `gh` receives the call's argv unchanged.
  - [ ] R-20 (shim half): a mixed-case `origin` reaches the fake helper unchanged, and a failing helper leaves the fake `gh` uninvoked. The other half is T2's R-4 letter-case case.
  - [ ] R-21:
    - Each outcome writes one line in `## Data Model`'s format: `minted`, `mint-failed`, `no-repo` or `refused`.
    - A pre-existing `0755` directory and `0644` file read back as `0700` and `0600` after a call.
    - A grep of the record for every token minted in the run finds none. The same grep finds a token appended to a copy, which is the control.
    - Twenty concurrent calls produce twenty well-formed lines, none interleaved.
  - [ ] R-22: with the state directory's path occupied by a regular file, the fake `gh` still runs, and stderr holds one line beginning `gh shim: record:` with no token in it. The shared seeded control on that line makes the scan fail.
- **Test Expectations:**
  - Each failure class at each step, positive and negative.
  - `FD-7`'s injected-variable cases on both CI platforms. R-14's static check bans `env`, so the shim enumerates and unsets `GIT_`-prefixed names in shell. `export -p` and `set` print differently under macOS `/bin/sh` (bash 3.2) and Linux `dash`, so both platforms must run those cases.
  - Tests that set environment variables are not parallel.
  - On macOS the tests find the real `lsof` and fail, not skip, when it is absent. On Linux they put the fake `lsof` on `PATH`.
  - Fixture clones are built at test time and fixture tokens follow D3. None of them is named `config.toml` or `*.pem`, which `.gitignore` matches at any depth.
  - Mutation target: the shim script and the static check's decision logic, as convention only (D10). The spec's seeded copies are its hand-mutation sample.
  - The size anchor is `c1db4ea` (task #4672 in the maintainer's tracker), which added a 58-line shell hook and 163 lines of Go tests with stub binaries. This task carries about fifteen requirements and a 20-control lexical check. Expect about 5 agent runs, each committing a green build:
    1. the harness, fakes and fixture clones;
    2. resolution, R-17 to R-20;
    3. the real-`gh` path, refusals, mint and hand-off, R-10 to R-13, R-15, R-16 and R-39;
    4. the record, R-21 and R-22;
    5. R-14's static check, its seeded copies and the three *macOS runtime* tests.
- **Anchors:** D1, D3, D6, D8, D10, D13, D17, D18

### T5 — Volume warning and marker

- **Files:** the shim script (step 7); its Go tests; `CLAUDE.md` if the record section needs it. All advisory.
- **Type:** backend
- **Depends on:** T4
- **Delivery:** `gh-shim`
- **What:** Adds R-9's step 7. After a `minted` line, the shim counts today's mints, compares (N + 52) × reads-per-mint, in tenths, with 70% of the daily limit, and prints one warning per UTC day across concurrent shims. Whichever shim atomically creates the day's marker is the one that prints.
- **Reaches:** the tracked shim, run in place by `just check`.
- **Acceptance Criteria:**
  - [ ] R-23, scenario "the volume line is reached":
    - At the shipped constants (reads-per-mint 3.0 held as 30 tenths, limit 1,000), the warning fires at N = 182 and not at N = 181.
    - On a copy with reads-per-mint 3.4, it fires at N = 154 and not at N = 153.
    - A later call that day prints nothing. Ten concurrent calls past the line print one warning.
    - With the marker's path occupied by a directory, a minting call past the line prints a line beginning `gh shim: volume:`.
  - [ ] R-24: the warning is one stderr line beginning `gh shim: volume warning:`. It holds the count, both constants and the word `maintainer`, and no fake token. The shared seeded control on that line makes the scan fail.
- **Test Expectations:**
  - The firing condition as a property over N and reads-per-mint: the warning fires exactly when (N + 52) × tenths ≥ 7,000.
  - The marker holds the creation time, a tab and N.
  - R-22's skip: with the record unwritable, no marker is created and no warning prints.
  - The R-23 Reader is D14 and is not a criterion here.
  - Mutation target: the threshold arithmetic and the marker creation, as convention only (D10). Expect 1 agent run.
- **Anchors:** D3, D10, D13, D14, D17

### T6 — `gh` shim edge tests

- **Files:** new Go test file(s). Advisory.
- **Type:** test
- **Depends on:** T1, T4, T5
- **Delivery:** `gh-shim`
- **What:** Glitch drives the tracked shim black-box from the test plan at `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md`: each way in once, and the record and warning formats at the boundary.
- **Reaches:** the same surface as T4 and T5.
- **Acceptance Criteria:**
  - [ ] R-9, R-10, R-11, R-12, R-17, R-18, R-39: each failure class produces its `gh shim: <class>:` prefix once (`home`, `real-gh`, `no-repo`, `refused`, `mint-failed`), with a non-zero exit and the fake `gh` not started.
  - [ ] R-13, R-15: on the success path, the fake `gh` receives `GH_TOKEN` equal to the helper's line, the two dedicated directories, and the argv unchanged. The shim's exit status equals the fake `gh`'s.
  - [ ] R-21: for each outcome, the record line has `## Data Model`'s five tab-separated fields, in order, with the closed values for class and outcome.
  - [ ] R-23, R-24: past the line, one warning line in R-24's form and one marker in `## Data Model`'s form; none on a second call that day.
  - [ ] R-14, TP-S12: after a successful call, the token is in neither fake's argv, not in the shim's own stdout or stderr (the fake `gh`'s output excluded), and in no file under the test's temporary root, with `TMPDIR` inside that root. The same file scan finds a token the test plants under `TMPDIR`, which is the control.
  - [ ] R-16, TP-S13: two consecutive calls run the fake helper twice, the two tokens differ, and neither token is found under the test's temporary root afterwards, by TP-S12's scan and its control.
  - [ ] R-19, TP-S14: with the session in clone A, a call with `GH_REPO=other/x`, a call from another clone's directory, and a call with `-R other/x` each give the fake helper A's `owner/repo`, and the fake `gh` the call's argv unchanged.
  - [ ] R-21, TP-S19: a pre-existing state directory at `0755` and record file at `0644` read back as `0700` and `0600` after a call.
  - [ ] R-22, TP-S20: with the state directory's path occupied by a regular file, the fake `gh` still runs, stderr holds exactly one line beginning `gh shim: record:`, and that line holds no token.
  - [ ] R-23, TP-S23: with today's record seeded with 180 `minted` lines, a minting call (N = 181) prints no `gh shim: volume warning:` line, and no marker file exists afterwards. This is the control for the warning case.
  - [ ] R-23, TP-S24: with the marker's path occupied by a directory and today's record seeded past the line, a minting call writes a stderr line beginning `gh shim: volume:`, and the fake `gh` still runs.
- **Test Expectations:** Black-box. They must fail when the tracked shim is missing. The edge author writes its own recording fake `gh` from the spec, and may reuse T4's process plumbing (temporary root, fake harness process). Fixture tokens follow D3.
- **Anchors:** D2, D15, D17, D18

### T7 — S7's one command

- **Files:** a tracked command beside the shim, a shell script with a level-5 name; its Go tests; `CLAUDE.md` layout table. All advisory.
- **Type:** backend
- **Depends on:** T4, T5, T6
- **Delivery:** `s7-command`
- **What:** Writes the command that compares the deployed shim with the tracked one, runs the pin canary through the shim, and runs the login check. It exits 0 to pass, 1 to fail and 2 when inconclusive, and names the failing check on stderr.
- **Reaches:** the tracked command, run in place by `just check`.
- **Acceptance Criteria:**
  - [ ] R-28:
    - With fakes that behave correctly, the command exits 0. This is the positive control.
    - A deployed copy differing by one byte exits 1, naming `byte-compare`.
    - A given clone resolving to the expected repository, or a working directory outside the given clone, exits 2, naming `preconditions`, before any shim call.
    - Each of these exits 1, naming `pin-canary`: a fake `gh` printing another repository; a failing fake helper with an expected-repository `minted` line seeded into the record; a record path occupied by a directory.
    - A fake `gh` printing `User` exits 1, naming `login-check`.
    - The command's stdout and stderr hold no token, and the shared seeded control, applied to a copy of the command, makes that scan fail.
  - [ ] R-28: when T7 lands, the command's usage text, which states the pinned invocation form, is recorded on task #4741 in the maintainer's tracker (D16).
- **Test Expectations:** Each exit class, and each check's failure at its own position, with the earlier checks passing. Mutation target: the command's decision logic, as convention only (D10). Expect 1 to 2 agent runs.
- **Anchors:** D1, D3, D10, D14, D16, D19

### T8 — S7 command edge tests

- **Files:** new Go test file. Advisory.
- **Type:** test
- **Depends on:** T1, T4, T7
- **Delivery:** `s7-command`
- **What:** Glitch drives the tracked command black-box from the test plan at `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md`: each exit class and check name once.
- **Reaches:** the same surface as T7.
- **Acceptance Criteria:**
  - [ ] R-28: one case each gives exit 0; exit 1 with `byte-compare`; exit 1 with `pin-canary`; exit 1 with `login-check`; and exit 2 with `preconditions`. No case's output holds a token.
  - [ ] R-28: every test calls the command in the invocation form T7 pinned, and each test file's header states that form, taken from the usage text recorded on task #4741 in the maintainer's tracker at T7's landing (D16).
- **Test Expectations:** Black-box. They must fail when the command is missing.
- **Anchors:** D2, D15, D16, D19

### T9 — README setup and reversal

- **Files:** `README.md` (`## Setup`, `## Commands`); `CLAUDE.md` if it describes the wiring. Advisory.
- **Type:** infra
- **Depends on:** T2, T3, T4, T5, T6
- **Delivery:** `readme-setup`
- **What:** Documents R-27's items, and records the deployed shim directory's path, which is a level-5 call. The deployed directory must sit outside every directory already on `PATH`.
- **Reaches:** nothing runnable. It is the deploy and reversal reference for T10 and T12.
- **Acceptance Criteria:**
  - [ ] R-27: the README holds each of these, with no token or secret value, read by the lead at the pull request:
    - placing the shim by temporary name, then a rename onto `gh`;
    - setting the install-time real-`gh` path, and the wiring line;
    - the deploy order: binary, with SHA-256 and source commit recorded; the `--landing` mint at the maintainer's terminal; the shim; the wiring line;
    - the reversal order: remove the wiring line; restart sessions; remove the shim; restore the prior binary; confirm `command -v gh` in a new session;
    - the `ED-7` blast-radius statement;
    - one command that reads the record and the marker files.
- **Test Expectations:** none; this task writes no tests. Its check is the lead's read.
- **Anchors:** D3

### T10 — Runbook on the task

- **Files:** none in this repository. The runbook is written on task #4741 in the maintainer's tracker.
- **Type:** infra
- **Depends on:** None
- **Delivery:** `runbook`
- **What:** The lead writes R-38's runbook from the spec, covering the attended mint, the attended shim run and the run with nobody present. It cites the README for the shim directory once T9 lands. It needs no code, so it is written while the code builds, and it must exist before T11.
- **Reaches:** nothing runnable. It is the walk-through for T11.
- **Acceptance Criteria:**
  - [ ] R-38: before T11's recorded start, the task holds the runbook with four sections: Pre-checks, Steps, Post-checks and Rollback.
    - Pre-checks hold `FD-11`'s B pre-check, the installation's workflow-write read, R-37's recorded SHA-256, and `LT-8`'s check that the throwaway repository's secret lists are empty.
    - Steps hold R-27's deploy order and `LT-3`, which runs attended outside the shim.
    - Rollback holds R-27's reversal order.
    - The runbook states that the maintainer types the attended mint and the B pre-check mint in his own terminal.
  - [ ] `CLAUDE.md`, "Adding an `[installations]` entry may silently widen what agents can mint" (not an R-38 item; carried into the runbook's Pre-checks): the Pre-checks record whether the sigil config's `[installations]` table already maps the throwaway repository's owner. If it does not, whether to add that owner is the maintainer's level-3 decision at T11's stop. If he adds it, the same change checks every agent permission rule that matches owners by name against the new owner.
- **Test Expectations:** none; this task writes no tests.
- **Anchors:** UNANCHORED

### T11 — First real run: the attended landing mint

- **Files:** none. Records go on the task.
- **Type:** infra
- **Depends on:** T2, T3, T10
- **Delivery:** `first-real-run`
- **What:** The earliest real run, a level-3 hard stop: the maintainer is present and Bolt walks the runbook first. The maintainer creates the throwaway repository to `FD-11`'s criteria, with `LT-8` read at creation. The prior binary is recorded. The binary built from `main` with the `landing-mint` delivery is placed. Then the maintainer types `sigil-helper token --landing <throwaway owner>/<repo>` at his terminal.
  - **Live state.** This replaces the hand-placed `sigil` binary, so every wired clone's `sigil get` now runs the new binary. R-6 is what keeps that path unchanged.
  - **Rollback.** Restore the prior binary recorded under R-37 (D4).
  - **Who.** The maintainer acts at his terminal; the lead records.
  - **Not deployed.** The shim and the wiring stay off until T12.
- **Reaches:** the deployed `sigil-helper token --landing` at the maintainer's terminal, against real 1Password and real GitHub.
- **Acceptance Criteria:**
  - [ ] R-38: Bolt's walk of the runbook and the maintainer's approval are recorded on the task before the mint's recorded start.
  - [ ] R-37: before the new binary is placed, the task records the prior binary's path, its SHA-256, and one successful `sigil doctor` run of it (D4).
  - [ ] R-30: the deployed binary's SHA-256 and its source commit on `main` are recorded at placement (D4).
  - [ ] R-30: `LT-8`'s secret-list reads are recorded at the repository's creation and at this run, each with its positive control as `LT-8` states (D4).
  - [ ] R-30, scenario "unattended landing mint" (live at `LT-1`): a dated `LT-1` result is recorded. Under the answered level-3 row, a verified exit 0 is the evidence, with key names only on a mismatch.
- **Test Expectations:** none; this is a live run.
- **Anchors:** D4

### T12 — Attended shim run

- **Files:** none in this repository. It deploys the shim to the directory the README records, adds the wiring line to `~/.zshrc`, and records on the task.
- **Type:** infra
- **Depends on:** T6, T8, T9, T11
- **Delivery:** `attended-shim-run`
- **What:** A level-3 stop, with deploying the wiring line part of it. The shim and the wiring are deployed in R-27's order. A lead session started in a clone of the throwaway repository then runs `LT-2` to `LT-7` (the attended half), `LT-9`, `LT-10` and R-28's command. `LT-3` runs attended outside the shim, from an untracked patch on a throwaway checkout (R-7). Before `LT-10`, the lead proposes repository B, the maintainer reads `FD-11`'s criteria at his terminal, and the pre-check is recorded. B is named only on the task.
  - **Live state.** Every agent session started after the wiring line runs `gh` through the shim.
  - **Rollback.** R-27's reversal order.
  - **Who.** The maintainer approves and deploys at his terminal; the lead runs the session-side checks.
- **Reaches:** the deployed shim, in every newly started agent session on the Mac.
- **Acceptance Criteria:**
  - [ ] R-25 (live, `LT-6`): `command -v gh` is recorded in a lead session, a plain dispatched agent, a worktree-isolated dispatched agent, the maintainer's terminal, a `--worktree` session, a background session, and each other spawn path in routine use from `KG-1`'s list. The shim path prints in each session and agent, and the real path at the terminal.
    - A miss in a lead session, a dispatched agent or at the terminal stops the build on R-25 at level 3, and the Frame is revised.
    - A miss on any other path takes the maintainer's signature on the re-rated `KG-1`.
  - [ ] R-26 (live): the harness permission rules containing `sigil`, in the user-level settings and in the throwaway clone's `.claude` settings, are the same before and after deployment. The same search finds a rule seeded into a copy, which is the control.
  - [ ] R-30: dated results for `LT-2`, `LT-3`, `LT-4`, `LT-5`, `LT-6`, `LT-7` (attended half), `LT-9` and `LT-10` are recorded on the task. Scenarios "a call aimed at another repository", "a session outside a clone", "`gh auth` or `gh extension` through the shim" and "the mint fails" run live here, as their headings name (D4).
  - [ ] R-30: `FD-11`'s B pre-check is recorded before `LT-10` starts (D4).
  - [ ] R-30: R-28's first pin-canary output on the throwaway repository is recorded, and its exit 0 has the cross-call record R-28 states (D4).
  - [ ] R-34: before `LT-7` first uses a 1Password usage reading, the maintainer's dated confirmation that the output carries no secret is on the task. Usage entries hold numbers and times only (D4).
  - [ ] R-29: once B is chosen, the three searches return no hit for B's name, case-insensitive, and the same searches for `sigil` hit (D3).
  - [ ] R-30: each failure takes its spec branch before T13. `LT-9` failing takes the maintainer's signature on `RISK-B`. `LT-10` failing, inconclusive after triage included, takes `FD-7`'s fallback and a Frame revision (D4).
- **Test Expectations:** none; this is a live run.
- **Anchors:** D3, D4, D6, D7, D8

### T13 — Pre-start record and the run with nobody present

- **Files:** none. The `DEF-1` load-count script lives outside this repository, as the answered level-3 row states.
- **Type:** infra
- **Depends on:** T12
- **Delivery:** `unattended-run`
- **What:** The lead amends the load-count script to R-33's window and evaluates `DEF-1`, then writes the pre-start record. Before the start, the lead raises whether the harness classifier still refuses `gh pr merge`, a level-3 row with no workaround. On the maintainer's go, also level 3, the run with nobody present lands a pull request on the throwaway repository through the shim. A workflow-changing pull request carries S5 if it was not carried at T12.
- **Reaches:** the deployed shim in an unattended lead session.
- **Acceptance Criteria:**
  - [ ] R-33: the pre-start record cites:
    - the per-day counts over the 15 whole UTC days before the evaluation day, with that day excluded;
    - the peak;
    - `LT-7`'s reads per mint;
    - the projected daily reads, and the result against the 70% line;
    - the content hash of the amended script;
    - the quoted match rule.
  - [ ] R-31: the pre-start record is on the task, timestamped to the minute before the run's recorded start. It cites each of `FD-11`'s five results, or the substitute `FD-11` names:
    1. `LT-7`'s attended half;
    2. R-33's evaluation, with the maintainer's choice where the result is at or above the line, or the median wall time is above 10 s;
    3. `LT-9` passed, or the signature on `RISK-B`;
    4. `LT-10` passed;
    5. `LT-6` recorded, with the `KG-1` signature where one is needed.
  - [ ] R-31: the run's record cites the pre-start record and the run's own start time to the minute. No inconclusive result is counted as a pass.
  - [ ] R-36: the run's record holds:
    - Launch: how the run was launched, and `command -v gh` from inside it.
    - S1: a non-terminal-stdout mint whose response equals the contract.
    - S4: every landing step done through the shim with no prompt, each exit status taken from the transcript, and a `narrowed` record line at each step's time. `gh api user --jq .type` through the shim does not print `User` at the start or the end, while the real binary does. GitHub attributes the pull request and the merge to the App's bot identity. `gh` at the maintainer's terminal acts as him.
    - S6: a read through the shim more than one hour after the first mint, exiting 0.
    - S5: recorded in whichever run carried it, with the live read of the installation's workflow write.
- **Test Expectations:** none; this is a live run.
- **Anchors:** D4

### T14 — Closing step: delete the throwaway repository

- **Files:** none.
- **Type:** infra
- **Depends on:** T13
- **Delivery:** `close-out`
- **What:** `FD-11`'s closing step, run by the maintainer at his terminal after the run with nobody present passes. If that run failed, the lead's failure report states which branch of the answered level-3 row applies. With a fix in hand, the repository is kept for one re-run and closed out by this step after the re-run passes. Otherwise it is deleted now. Once the last of S1 and S3 to S7 is recorded, R-37's prior binary may be removed.
- **Reaches:** nothing runnable.
- **Acceptance Criteria:**
  - [ ] R-35: S4's and S5's evidence is copied into the run record, the repository is deleted, and a read at his terminal returns HTTP 404, recorded. All of this happens before the task is marked done or cancelled.
  - [ ] R-35: if the run with nobody present failed, the lead's report names the branch taken: kept for one re-run, or deleted.
  - [ ] R-37: the prior binary's file exists until the last of S1 and S3 to S7 has a recorded pass (D4).
  - [ ] R-30: every row R-30 lists has a dated result on the task, apart from `LT-7`'s one-day half, which T15 records (D4).
- **Test Expectations:** none; this is a live record.
- **Anchors:** D4, D14

### T15 — `LT-7`'s one-day half

- **Files:** none.
- **Type:** infra
- **Depends on:** T13
- **Delivery:** `close-out`
- **What:** The lead records the first full UTC day of running. The task is not marked done while this entry is absent.
- **Reaches:** nothing runnable.
- **Acceptance Criteria:**
  - [ ] R-32: the entry is for the first UTC calendar day that begins after the run with nobody present passed and after the deployed shim passed R-28's command, and whose record file holds at least one `minted` line. It gives:
    - the date;
    - that day's `minted` count;
    - 1Password's reported usage;
    - the implied reads per mint, which is an upper bound;
    - whether the UTC day matches the account's reset, with the reset time.
  - [ ] R-32: an implied reads per mint above the shim's constant, or a reset mismatch, is reported to the maintainer at the routine update.
  - [ ] R-34: the day's usage reading is taken after 24:00 UTC, with its time recorded, and the entry holds numbers and times only (D4).
- **Test Expectations:** none; this is a live record.
- **Anchors:** D4

## Decisions

| ID | Decision | Rationale | Canon anchor |
|---|---|---|---|
| D1 | Code lands in five deliveries, each one pull request squashed to one commit carrying the task number: `test-plan` (T1), `landing-mint` (T2, T3), `gh-shim` (T4, T5, T6), `s7-command` (T7, T8) and `readme-setup` (T9). | The mint and the shim share no code: the shim's tests use a fake helper. Splitting them keeps each review to one surface, lets T11 run on the mint alone, and lets each pull request land building and green. | dev-loop `<slicing>`, "work lands as vertical slices"; `CLAUDE.md`, "Squash to one delivery commit carrying the task number"; V-Maintainability |
| D2 | A requirement is owned by its build task. The edge-test task in the same delivery re-cites the criteria that run through the built artifact or the tracked script: R-1, R-2, R-5 (skip-gated), R-9 to R-24, R-28 and R-39. The pair lands as one delivery. | The spec writes some criteria as black-box tests, and dev-loop gives black-box tests to a separate author. Sharing a delivery keeps each requirement's ownership to one landing. | dev-loop `<tests>`, "Edge tests are a separate author's"; spec `## Requirements`, "A sub-bullet under a requirement is an acceptance criterion" |
| D3 | Cross-task constraints read by the lead on every pull request of this feature, from T1 on: R-6 over the diff from the base commit (Release binding) to the head; R-7; the spec's `## Constraints` "Public repository" search, with its control; and, once B is chosen at T12, R-29's three searches. On every pull request touching the shim or its tests, the lead also makes the three-name `-v` read. In every test task, fixture tokens are built at runtime, are not `ghs_`-shaped, and add no `gitleaks:allow` or ignore entry. | R-6, R-7 and R-29 are tagged lead-run at every pull request. The fixture rule is a `## Constraints` clause that binds every test author. | spec R-6, R-7 and R-29 tags; spec `## Constraints`, "Fixture tokens" and "Public repository"; spec `## Requirements`, "Where each criterion runs" |
| D4 | R-30, R-34 and R-37 are run-record constraints spanning T11 to T15. Each live task writes its own rows, and T14 checks R-30 complete apart from `LT-7`'s one-day half. | Their criteria fall at several live runs, so no single task can hold them. | spec R-30, "A row with no recorded result is the observable failure"; P-InstrumentBefore |
| D5 | The landing mint's in-gate edge tests reach argument handling and order (R-1, R-2). The mint path through the built binary is reached only by the skip-gated R-5 test and live at `LT-1`. This is not a layer: `--landing` is reachable through the built binary from T2. | R-5 rules out a new key-source seam, and `CLAUDE.md` records the seam as deliberately not added. | spec R-5; spec `## Constraints`, "A green `just check` does not cover the built binary's mint path"; `CLAUDE.md`, "What is still open" |
| D6 | No code toggle. The shim reaches no session until T12's hand deploy and wiring line, a level-3 stop. The run with nobody present waits on R-31's lead-held gate. | No task before T12 exposes the shim to a user, so a toggle would guard nothing. | dev-loop `<slicing>`, "No toggle unless a task before the feature's last would expose unfinished behaviour"; spec `## First real run`, "a lead-held gate, not a code toggle" |
| D7 | Every planned code delivery lands before T12 deploys the wiring. A landing after T12, such as a fix, follows the spec's answered row (b): it lands through the shim after a recorded read, at the maintainer's terminal, of the personal installation's repository selection, with R-27's reversal as the escape when the shim itself is broken. | This keeps planned work off the shim, and gives later work the route the maintainer chose. | spec `## Decision levels`, "How sigil's own changes land once the wiring is deployed"; V-Reversibility |
| D8 | Implementers read the Frame and the intake on `main`, at `docs/intent/lead-session-gh-token-frame.md` and `docs/intent/lead-session-gh-token.md`, on `main` since PR #16 as a wording-amended version of the locked Frame. No task tells an agent to read the lock commit `dd9ca140cab2818189d767e751d07fe47a788caf`; the plan keeps that SHA as provenance only. Evidence: the amended Frame differs from the lock commit in wording only, at lines 93, 372, 390, 392, 473, 500, 556, 573 and 575, and the intake does not differ. None of those lines is an `FD-n` Observable line, an `LT` row, a signed line, the actor set or a known gap, and `FL-5`'s wording was approved by the maintainer. A reader who finds an adopted list differing on `main` stops and raises it to the lead. | The lock commit holds private deployment text, and implementers edit public files such as `CLAUDE.md` with what they read in context. The spec adopts the Frame's lists "as written at the commit above", and those lists are unchanged on `main`. | spec `## Constraints`, "Public repository"; spec `## Purpose`, "adopts that list as written at the commit above"; P-PreserveDecisionSpace, "A provenance fact is copied verbatim from its source" |
| D9 | One test plan for the feature, at `docs/plans/2026-11-25-lead-session-gh-token-test-plan.md`, landed as its own documentation delivery before any build task. | This repository has no test-plan convention. The path sits beside this plan, and landing it first lets T3, T6 and T8 read it from `main`. | dev-loop `<tests>`, "A test plan derived from the spec before the build" |
| D10 | Mutation testing is not priced and not wired here. Each task's mutation target is convention, never reported as an enforced gate. The spec's seeded copies are the hand-mutation sample for the shim, the static check and the command. | Adding a mutation tool is a new dependency, and the spec approves none. | spec `## Decision levels`, "Any new dependency … None approved"; dev-loop `<mutation>`, "never report a mutation gate as in force" |
| D11 | `NO-TASK`: no walking skeleton and no `verify-smoke` recipe. | sigil already runs. Wiring `verify-smoke` and `puck floor check` into the gate would change the `check` recipe, which the spec's out-of-scope list forbids. If floor adoption is wanted, it is separate work. | spec `## Out of scope`, "changes … to the justfile's `ci`, `check`, `test`, `vet` or `fmt-check` recipes" |
| D12 | `NO-TASK`: the spec's level-4 rows (reads-per-mint after `LT-7` or R-32, and the daily limit) and the counting-window row after a reset mismatch get no task. | Each fires on a live result that may never occur. When it does, it is a tracked change under its row, followed by R-28's command, as the rows state. | spec `## Decision levels`, rows "Updating reads-per-mint after `LT-7` or R-32" and "The counting window after R-32 finds a reset mismatch" |
| D13 | R-23 and R-24 are a second build task (T5) in the `gh-shim` delivery, not a separate delivery. T4 implements R-9's steps 1 to 6 and 8, and T5 fills step 7 before that delivery lands. | R-9 is a list of SHALL steps. A separate delivery would leave step 7 missing on `main` for one landing. In one delivery, R-9 holds in full at its landing. | Wyrd stance, "Every requirement maps to exactly one task"; dev-loop `<slicing>`, "Each commit of a multi-commit landing counts" |
| D14 | The monthly lead-session run is parked, human-noticed, monthly. It covers R-28's command, R-23's marker read and R-35's abandoned-task read, and also runs after a binary or wiring replacement or a Claude Code upgrade. It is a standing procedure, not build work. | The spec parks it with that cadence and builds no detector. | spec `## Constraints`, "Parked, human-noticed"; spec R-23 "Reader" and R-35 "An abandoned open task"; P-Defer DF1 |
| D15 | Level 5. The edge tests (T3, T6, T8) are Go test files that drive the built binary, the tracked shim and the tracked command through `os/exec`. This settles the test plan's gap G-9. | `just check` runs Go tests, and the spec puts that recipe out of scope, so a test in another language could not run in the gate. | spec `## Decision levels`, level 5, "gate mechanics, reversible choices, anything where building proceeds either way"; spec `## Out of scope`, "the justfile's `ci`, `check`, `test`, `vet` or `fmt-check` recipes"; spec `## Requirements`, "Where each criterion runs" |
| D16 | Level 5. R-28's invocation form is the builder's choice. T7 pins it and records the command's usage text on task #4741 in the maintainer's tracker when T7 lands, and T8 takes the form from that record. This settles the test plan's gap G-1. | R-28 names the two inputs and not how they are given. The Frame's bounded option-space does not list the invocation form among its Fixed items, so it is free. | Frame `## Locked decisions`, "Bounded option-space for the Spec"; spec `## Decision levels`, level 5, and row "In-repository names for the shim and R-28's command" |
| D17 | Level 5. The shape of the record's `mint_seconds` field is pinned by T4's own unit tests, since T4 writes R-21's record line. The marker's time format is pinned by T5's own unit tests. The edge tests assert only what the spec states: the marker holds two tab-separated fields, a non-empty creation time and N; `mint_seconds` is a non-empty decimal number where a mint was attempted, and empty otherwise. This settles the test plan's gaps G-2 and G-3. | The spec's `## Data Model` gives no time format for the marker and no bound on `mint_seconds`. Either shape is reversible and building proceeds either way. | spec `## Decision levels`, level 5; spec `## Data Model`, "warning marker" and field `mint_seconds` |
| D18 | Level 5. R-9's "one stderr line beginning `gh shim: <class>:`" is read as exactly one stderr line beginning `gh shim: <class>:`. Other stderr lines may appear: helper stderr that R-12 passes through, and R-22's `gh shim: record:` line when the record cannot be written. This settles the test plan's gap G-4. | R-12 passes the helper's stderr through, so a `mint-failed` call can write more than one stderr line in total. A failing call that is recorded can also meet R-22's record failure, so a reading of exactly one line beginning `gh shim: ` would reject a correct shim. | spec `## Decision levels`, level 5; spec R-9, "Each failure writes one stderr line"; spec R-12, "pass its stderr through"; spec R-22 |
| D19 | Level 5. R-28's Preconditions are read as its Acceptance text states them: a given clone resolving to the expected repository, or a working directory outside the given clone, exits 2 naming `preconditions`, before any shim call. This settles the test plan's gap G-10. | The two Preconditions bullets read as failure causes, while the Acceptance bullet and the pin canary's meaning make them conditions that must hold. The Acceptance text is the binary-observable one. | spec `## Decision levels`, level 5; spec R-28, "Acceptance" |

## Sequencing

**Parallel-safe groups.**
- After T1 lands, these run in parallel: {T2, then T3}, {T4, then T5, then T6} and {T10}. T10 needs nothing and can start at once.
- After the `gh-shim` delivery (T4, T5, T6) lands, {T7, then T8} runs beside T9.
- T9 needs the `landing-mint` and `gh-shim` deliveries.
- T11 needs the `landing-mint` delivery and T10. It runs while the shim builds.

**Strict sequences.**
- T11 → T12 → T13 → {T14, T15}.
- T12 waits on every code delivery (D7).
- T15's day cannot begin before T13 passes.

**Critical path:** T1 → T4 → T5 → T6 → T7 → T8 → T12 → T13 → T15. T15 adds at least one full UTC day after T13. T9 sits beside T7 and T8, and joins at T12.

**Review gates.**
- **Code review.** Each code delivery gets its review before it lands, plus D3's lead-run reads.
- **T11 (level 3).** Bolt walks the runbook and the maintainer approves before the first write-capable mint. It sits off the critical path, as early as the mint allows.
- **T12 (level 3).** The maintainer approves the wiring deploy and reads B's criteria.
- **T13 (level 3).** The classifier question is raised, and the maintainer gives the go on the pre-start record.

## Test surface

- **Unit and gate.**
  - Go's `testing` package only, with no new dependency. GitHub is mocked with `httptest`, and RSA keys are generated in process.
  - The shim and the command are run in place by Go tests, with a fake harness process named by `CLAUDE_PID`, a fake `sigil-helper` on `PATH`, a recording fake `gh` through `SIGIL_GH_REAL`, the real `lsof` on macOS and a fake `lsof` from `/proc` on Linux.
  - Fixture clones are made with `git` at test time, under a temporary root with `TMPDIR` inside it.
- **Property.**
  - T2: rejection of any single-field perturbation of the matching grant.
  - T5: the threshold condition over N and reads-per-mint.
- ***macOS runtime*.** Three tests (R-13, R-14). The `-v` read is in D3. The Linux CI job skips them.
- **Skip-gated.** R-5's sibling of the leak scan. The macOS skip count goes from 2 to 3.
- **Live.** `LT-1` to `LT-10`, and S1 and S3 to S7, at T11 to T13.
- **Smoke.** R-28's command is the standing smoke of the deployed shim (D14).
- **Mutation.** Convention only (D10).

## Release binding

- **Target release:** none. This repository cuts no releases. Each delivery lands on `main` by pull request, and the binary and shim are deployed by hand at T11 and T12.
- **Branch posture:** worktree branches, one pull request per delivery, rebase-merged with both required CI checks, as `CLAUDE.md` states. Landing is the lead's.
- **Base and baseline.**
  - R-6's base commit is `main` immediately before T1's pull request lands. The lead records its SHA on task #4741 in the maintainer's tracker. At authoring, `main` was `7fd2fa5cb70252ddf795f9b80b341574c9158176`.
  - Before the first code task is dispatched, the lead runs `just check` at that base and records the counts by `CLAUDE.md`'s method. The figure inherited from `CLAUDE.md` is 113 pass and 2 skip.
- **Gate per landing:**
  - `just ci` green locally;
  - `ci (ubuntu-latest)` and `ci (macos-latest)` green;
  - D3's lead-run reads;
  - for T2 and T3 at the build's close, the macOS skip count reading 3.

## Spec gaps surfaced

None.

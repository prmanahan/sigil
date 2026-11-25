---
# Stamped by Puck at spec-exit; leave `draft` until then. The lock test is `skills/brief/references/stage-3-spec.md`, "What locked means".
status: locked           # draft | locked
lock_tag: "brief/lead-session-gh-token/spec-locked" # brief/<slug>/spec-locked (base) or brief/<slug>/spec--<part>-locked (part), stamped at spec-exit
---

# Feature: Narrowed GitHub token for lead sessions' `gh` work

> **Spec for:** task #4740 in the maintainer's tracker (parent task #4655, epic task #4734)
> **Date:** 2026-11-20
> **Design ref:** Frame `docs/intent/lead-session-gh-token-frame.md`, locked at commit `dd9ca140cab2818189d767e751d07fe47a788caf` (tag `brief/lead-session-gh-token/frame-locked`). Intake `docs/intent/lead-session-gh-token.md`, tag `brief/lead-session-gh-token/intent-locked`.

## Purpose

A lead agent session's routine `gh` work runs on a GitHub App installation token minted per call, narrowed to the session's own repository and a fixed landing permission set, and verified against GitHub's actual grant. Nobody is present. The routine `gh` path, which is the shim, has no route to the maintainer's `gh` login. A session that reaches wider on purpose still can, by the residual routes `RR-1` to `RR-7`.

**What this adopts, by name.**

- The Frame at the commit above is the closed world for this spec. Its identifiers keep the Frame's meaning here and are not redefined: `FD-n`, `QA-n`, `OC-n`, `FL-n`, `LT-n`, `RR-n`, `KG-n`, `DEF-1`, `ED-n`, `ACTOR-n`, `RISK-A`, `RISK-B` and `RISK-C`. A requirement that cites an `FD-n` Observable list adopts that list as written at the commit above, and does not restate it.
- The intake's Terms keep their meaning: lead session, landing work, target repository, session's repository, wiring, expected grant. Its success criteria S1 to S9 are cited by number.
- The per-actor shape of task #4566 in the maintainer's tracker: one GitHub App, with tokens narrowed at mint for each actor. This spec builds the lead's landing profile only.
- The existing rate-limit path, `sigil token --rate-limit-only` (`token_narrow.go`), is the precedent for the mint, verify and revoke shape. The landing path departs from it only where `FD-1` to `FD-3` say so: a five-permission constant, private repositories allowed, and no terminal check.

**Risk ids.** `RISK-A`, `RISK-B` and `RISK-C` are the Frame's ids. Each resolves to an entry in `docs/src/adrs/RISK-REGISTER.md` through the Frame's `## Accepted and deferred risks`.

**Rulings this spec applies, and does not re-open.**

- No security-scope expansion. This is the maintainer's ruling of 2026-11-19, relayed by the lead. A concern that leaves the signed actor set, the bar and the accepted reach unchanged adds no requirement.
- The landing token is pinned to the session's repository, by the maintainer's ruling of 2026-11-11 (Frame 2a).
- The maintainer creates repositories by hand. A push or a settings change in this work never needs him.

## Requirements

Identifiers: `R-n` is a requirement in this spec. A sub-bullet under a requirement is an acceptance criterion, and the criteria are the scenarios of record. The **Anchor** line on each requirement holds the decision's rationale.

**Where each criterion runs.** A criterion of R-1 to R-5, R-8 to R-28, and R-39 runs in `just check` unless it is tagged with one of three other places:
- *skip-gated:* it needs real 1Password, as `CLAUDE.md` describes;
- *lead-run at the pull request:* the lead reads the diff of every pull request that touches the requirement's surface, and the check holds on each one;
- *live:* it needs real GitHub, at a run under `## First real run`.

*macOS runtime* is a platform condition on `just check`, not a place: the test runs in `just check` on macOS and skips only when `GOOS != darwin`. Three tests carry it: R-13's `SHELLOPTS=xtrace` test, R-14's `SHELLOPTS=xtrace` test and R-14's `SHELLOPTS=allexport` test. Each is its own top-level Go test, and the builder SHALL record exactly three Go test names on the task record. Each name SHALL carry, as a substring, the segment that names its requirement and variant: `R13_Xtrace` for R-13's `SHELLOPTS=xtrace` test, `R14_Xtrace` for R-14's `SHELLOPTS=xtrace` test and `R14_Allexport` for R-14's `SHELLOPTS=allexport` test. An illustrative name, not a required one: `TestShim_R13_Xtrace_HelperSecretNotOnStderr`. The pairing is one-to-one: each name carries exactly one of the three segments, and each segment is carried by exactly one name, so the `-v` output shows which test ran. CI runs `go test` without `-v`, so a skip is otherwise invisible: the lead runs `go test -count=1 -v -run '^(<name 1>|<name 2>|<name 3>)$' ./...` on macOS, with the three recorded names anchored as shown, and counts the top-level `--- PASS` lines by `CLAUDE.md`'s method (`grep -c '^--- PASS'`). The lead then opens each named test's body and confirms that it exports `SHELLOPTS` with the value its segment names (`xtrace` for `Xtrace`, `allexport` for `Allexport`) and scans for the asset its requirement names: the helper secret on the shim's stderr for `R13_Xtrace`, the token on the shim's stderr for `R14_Xtrace`, and the token under a name other than `GH_TOKEN` in the fake `gh`'s recorded environment for `R14_Allexport`. The lead also confirms that each named test fails when its control copy plants the leak: the control its requirement's criterion states, on R-13's xtrace criterion for `R13_Xtrace` and on R-14's xtrace and allexport criteria for `R14_Xtrace` and `R14_Allexport`. Wherever this paragraph says a test "fails when its control copy plants the leak", "fails" belongs to the leak scan run against the control copy, not to the test's result: run against the leaking control copy, the test's leak scan detects the leak, and the test as a whole passes only when that detection happens and fails if the control copy shows no leak, which is the in-test control each of those three criteria states ("and the test fails if it does not"). The read passes only when the task record holds exactly three names, the count is exactly three, each of the three segments appears in exactly one of the three top-level `--- PASS:` lines, and each named test's body exports the `SHELLOPTS` value, scans for the asset its segment pairs it with, and fails when its control copy plants the leak. Any other number of recorded names, a name carrying no segment or more than one, a segment carried by no name or by two, an unanchored pattern, any other count (zero included), or a body that does not export that value, does not scan for that asset or does not fail when its control copy plants the leak, is a failure. The lead makes this read at the build's close and at the lead-run pull-request read of every pull request that touches the shim or those tests.

R-29 to R-38 are lead-run or live, as each says; a criterion there with no tag is read by the lead from the task record.

### A. The landing mint: `sigil token --landing <owner>/<repo>`

- **R-1** `sigil token` SHALL accept `--landing <owner>/<repo>` as a third enumerated form, matched by exact string equality in `main.go#dispatchToken`. Every argument shape outside the three forms SHALL be refused before config load, with `usage` on stderr and nothing on stdout. That refusal includes one change to the existing single-argument form: a single argument that begins with `-` is refused, where today it reaches `cmdToken` as an owner. *Anchor: FD-1's Observable (P-LeastAuthority; enumerate the permitted).*
  - Run with an empty `HOME`, `sigil token --landing myorg/myrepo` exits non-zero. Its stderr contains `open config` and not `usage`. This is the positive control.
  - The refused shapes include `--landing=o/r`, `--landing a/b/c` and the single argument `-x`. Each exits non-zero, with stderr containing `usage` and not `open config`.
  - A single argument that does not begin with `-` still reaches `cmdToken`. The usage text names all three forms.

- **R-2** The landing form SHALL run in this order: config load, the API-base scheme check, owner routing, the key read, the exchange. It SHALL NOT run the terminal check. One bounded context SHALL cover the key read and the exchange, as on the rate-limit path. *Anchor: FD-1; the `CLAUDE.md` invariant that routing runs before any network call and before the key read.*
  - Through the built binary, with no service-account token:
    - `--landing unknown-owner/r` gives stderr containing `unknown owner`.
    - An insecure API base gives `https scheme required`.
    - With stdout a pipe and a known owner, stderr contains `OP_SERVICE_ACCOUNT_TOKEN is not set` and not `stdout is not a terminal`.
  - *Lead-run at the pull request:* the landing command creates one bounded context before the key read and passes it to both the key read and the exchange.

- **R-3** The landing exchange SHALL POST to the installation's `access_tokens` endpoint with the fixed body under `## API Contract`. The only caller input is the one repository name. *Anchor: FD-1 and FD-2 (P-LeastAuthority, "scope a credential to the operations its role performs and no more").*
  - Against a recording mock server, the decoded body equals the contract body for the repository named, field for field, and has no other field.

- **R-4** The landing path SHALL verify GitHub's response before it trusts or prints the token. The check is exact-match, with no public-repository condition. It passes only when all of these hold:
  - the granted permission map equals the contract constant in both directions, keys and levels;
  - `repository_selection` is `selected`;
  - exactly one repository is listed;
  - that repository's name equals the requested one.

  *Anchor: FD-3 (P-SecurityLayered, "a control that errors denies").* These criteria are checked at function level, with an in-process key and an `httptest` GitHub. The CLI's exit and stdout are R-5's.
  - A matching response for a public repository, and one for a private repository, each return the token.
  - A response differing in any one of those fields returns no token and an error containing `grant mismatch`, and sends exactly one `DELETE /installation/token` carrying the minted token. The differences tested include: an extra permission key, a missing key, a changed level, `repository_selection: all`, a repository count other than one, and a name differing only in letter case.
  - A revoke that returns a status other than 204, or fails in transport, gives an error containing `revoke failed`.
  - A 201 with an empty token gives an error and no revoke. A 401 is retried once with a fresh App JWT.
  - No returned error contains the token or a granted permission level. Control: the same substring check finds the token in the mock's response body.

- **R-5** A successful landing mint SHALL write the token and one newline to stdout, write nothing else to stdout or stderr, and exit 0. It SHALL NOT read or write the token cache in `cache.go`. No new key-source seam is added, so `just check` covers the mint function and the dispatch, not the built binary's mint path. *Anchor: FD-1; S2; S9; the `CLAUDE.md` invariant that the token leaves by exactly one channel.*
  - Under the `captureStd` swap, the mint function returns the token and writes to neither stream.
  - A cache directory seeded with an entry holds the same files, byte for byte, before and after a mint.
  - *Skip-gated* on `OP_SERVICE_ACCOUNT_TOKEN` and `SIGIL_TEST_OP_KEY_REF`: a new sibling of `TestCLI_LeakScan_TokenNotInStderr` runs the built binary on `--landing` against a mock GitHub.
    - On a matching grant: the token is on stdout, absent from stderr, and the exit is 0.
    - On a widened grant: stdout is empty and the exit is non-zero.

- **R-6** Apart from R-1's change to `dispatchToken` and its usage text, this change SHALL NOT alter the behaviour or the tests of the existing token paths. *Anchor: FD-1 (P-MinBlastRadius, extract on the third occurrence); S8.*
  - *Lead-run at the pull request,* over the diff from the feature's base commit (the commit before its first landing, its SHA recorded on the task) to each pull request's head:
    - the bodies of `getRateLimitOnlyToken`, `exchangeNarrowedToken` and `verifyNarrowedGrant` are unchanged;
    - `dispatchToken`'s changes are confined to the `--landing` arm, R-1's refusal and the usage text;
    - no `*_test.go` line that exists at the base commit is removed or changed.
  - Every test present at the base commit passes.

- **R-7** Tracked code SHALL NOT:
  - mint with any permission set other than the contract constant, or take a permissions input of any kind;
  - serve a landing token from `sigil get`;
  - cache a landing token.

  `LT-3`'s minus-`workflows` variant is an untracked patch on a throwaway checkout: never committed, never deployed, and deleted after the run. *Anchor: FD-1, FD-8 and FD-12; ED-1(d); intake Non-goal 4.*
  - R-3, R-5 and R-6 cover the request body, the cache and the existing paths.
  - *Lead-run at the pull request:* no diff adds no permissions parameter and no landing call inside `cmdGet`.

### B. The `gh` shim

- **R-8** The shim SHALL be a POSIX shell script named `gh`, tracked in this repository. It is deployed by hand to a directory that holds nothing but `gh`, apart from a temporary file during placement (R-27). *Anchor: FD-4 (P-TestableByDesign, a double only at a true system boundary); Frame routine decisions.*
  - The tracked file's first line is exactly `#!/bin/sh` (R-14's static check).
  - *Lead-run at the pull request:* the Go tests execute the tracked file in place, by its path in the tree, and never a substituted copy. The controls of R-9, R-13, R-14 (its shared control included, used by R-22 and R-24) and R-23 that edit a temporary copy are the only exceptions.

- **R-9** For each call, the shim SHALL perform these steps in order, and SHALL stop at the first that fails. Each failure writes one stderr line beginning `gh shim: <class>:`, with `<class>` as given.
  (1) the `HOME` check, R-17, class `home`; (2) the real-`gh` check, R-10, class `real-gh`; (3) the session's repository, R-18, class `no-repo`; (4) the subcommand refusal, R-11, class `refused`; (5) the mint, R-12, class `mint-failed`; (6) the record line, R-21; (7) the threshold check, R-23; (8) the `exec` of the real `gh`, R-15.

  *Anchor: FD-4's list; P-SecurityLayered, fail closed.*
  - A call that is both outside a clone and `gh auth …` records `no-repo`, and its stderr begins `gh shim: no-repo:`.
  - With an empty `HOME` and no real-`gh` path, stderr begins `gh shim: home:`; with `HOME` set and no real-`gh` path, `gh shim: real-gh:`.
  - With `HOME` set, no real-`gh` path, and the harness process outside a clone, stderr begins `gh shim: real-gh:`. Control: a temporary copy of the shim with steps 2 and 3 swapped gives `gh shim: no-repo:` on the same call, and fails this check.
  - A failure in steps 1 to 5 never starts the fake `gh`.

- **R-10** The shim SHALL take the real `gh` path from an override environment variable when it is set and non-empty, and otherwise from an install-time setting held outside the script's bytes. The path SHALL be absolute and name an executable file. Otherwise the shim exits non-zero with an error on stderr, does not mint, and does not start `gh`. *Anchor: FD-4's testing seam; KG-9 (accepted under `RISK-B`).*
  - Each of these exits non-zero with no helper call: no value from either source, a relative path, a missing file, a non-executable file, a directory.

- **R-39** *Active: the level-3 row on the real-`gh` path resolving to the shim is answered yes, by the maintainer, 2026-11-24.* The shim SHALL refuse, in step 2 and before any mint, a real-`gh` path that is the shim's own file, by path or by file identity, with class `real-gh`. *Anchor: that row; `RISK-C`; KG-9.*
  - The override set to the shim's own path, and to a symbolic link to it, each exit non-zero with `gh shim: real-gh:` and no helper call.

- **R-11** When the shim's first argument is `auth`, `extension`, `extensions` or `ext`, it SHALL exit non-zero with an error on stderr, without minting or starting `gh`, and SHALL record `refused`. *Anchor: FD-4; KG-8 states the limit (one alias defeats it).*
  - `gh auth setup-git`, `gh auth token`, `gh extension install o/r`, `gh extensions` and `gh ext` each exit non-zero, with the fake helper and the fake `gh` not invoked.
  - Control: `gh api user` is not refused.

- **R-12** The shim SHALL obtain the token by running `sigil-helper token --landing <owner>/<repo>`, with `sigil-helper` found on `PATH` and the session's repository as the one argument. It SHALL capture the helper's stdout and pass its stderr through. It accepts the output only when the helper exits 0 and its stdout is exactly one line that is non-empty and holds no space, tab or carriage return; a trailing newline is optional. Otherwise it SHALL exit non-zero without starting `gh`, even if a token was printed, and SHALL record `mint-failed`. *Anchor: FD-4.*
  - The fake helper receives exactly `token --landing <fixture owner/repo>`.
  - The rejected outputs include a helper that prints a token and then exits non-zero, two lines, and a token ending in a carriage return.
  - Control: a token with no trailing newline is accepted.

- **R-13** Before it starts `gh`, the shim SHALL set the environment as follows, unconditionally, overwriting any inherited value. *Anchor: FD-4 and FD-5 (P-SecurityLayered, "each layer is independently load-bearing").*
  - `GH_TOKEN` is the accepted helper line, with no line ending, assigned only in R-14's hand-off assignment, from the token variable R-14's static check declares.
  - `GITHUB_TOKEN`, `GH_HOST`, `GH_ENTERPRISE_TOKEN` and `GITHUB_ENTERPRISE_TOKEN` are removed.
  - `GH_CONFIG_DIR` is `$HOME/.config/sigil/gh`.
  - `XDG_DATA_HOME` is `$HOME/.local/share/sigil/gh-data`.
  - `GH_PROMPT_DISABLED` is `1`. `gh help environment` documents it as disabling interactive prompts [read from the installed `gh`'s help on the Mac, 2026-11-20].
  - `BASHOPTS` and `BASH_XTRACEFD` are unset before the helper runs (R-9 step 5). With both seeded (`BASHOPTS=extglob BASH_XTRACEFD=7`), `unset BASHOPTS BASH_XTRACEFD` returned 0 and left both unset [measured by the lead on macOS `/bin/sh`, GNU bash 3.2.57, 2026-11-20]. `SHELLOPTS` is not unset: on macOS `/bin/sh` it is read-only, and `unset` prints an error on every call. `FL-2`'s protection against an inherited `SHELLOPTS=xtrace` reaching `sigil-helper`, whose assignment holds the service-account token, is R-14's required first statement, `set +x +v +a`. With `SHELLOPTS=xtrace:allexport` inherited, after `set +x +v +a` the `SHELLOPTS` seen by a child was `braceexpand:hashall:interactive-comments:posix`, with no `xtrace` and no `allexport` [measured by Cerberus and Warden, review round 6, 2026-11-24, on macOS `/bin/sh`, GNU bash 3.2.57; Cerberus fed the script to `/bin/sh` on stdin, and Warden ran a `#!/bin/sh` script through its shebang].

  Every item in `FD-4`'s Observable list on the environment holds against the recording fake `gh`, with these two paths as the dedicated values. Pre-seeded values of all ten variables above other than `SHELLOPTS` are removed or overwritten; `GH_PROMPT_DISABLED` is seeded as `0`.
  - With `BASHOPTS=extglob` and `BASH_XTRACEFD=7` seeded and exported, neither variable is in the fake helper's or the fake `gh`'s recorded environment. Control: a temporary copy of the shim without that `unset`, run the same way, shows both in the fake helper's recorded environment.
  - *macOS runtime:* with `SHELLOPTS=xtrace` exported, a `#!/bin/sh` fake helper that assigns a helper secret, a string distinct from the minted token it prints, leaves that helper secret out of the shim's stderr. The scan looks for the helper secret. The first statement's own trace line is expected on stderr and is not a leak. Control: a temporary copy of the shim without its first statement, `set +x +v +a`, run the same way, shows the helper secret on stderr, and the test fails if it does not. Linux's `dash` ignores `SHELLOPTS`.

- **R-14** The token SHALL reach `gh` only through an environment assignment on `exec`. It SHALL NOT be an argument to any external command, be printed by the shim (shell tracing included), or be written to any file by the shim. The shim SHALL hold the token in a variable whose name is not `GH_TOKEN`, and `GH_TOKEN` SHALL take the token only in the `exec` hand-off assignment. The shim SHALL unset that variable before the capture, so that a variable of that name inherited already exported does not pass the token to the children the shim starts before the `exec`. The shim's first statement SHALL be `set +x +v +a`. *Anchor: FD-4; S9; OC-8.*
  - The token appears in neither the fake `gh`'s argv nor the fake helper's.
  - The shim's own stdout and stderr hold no token; output written by the fake `gh` itself is excluded.
  - **Seeded control, shared with R-22, R-24 and R-28.** For each of four paths, a temporary copy that also prints the token on that path makes that path's no-token scan fail: the shim's stdout and stderr on a minting call (this criterion); the `gh shim: record:` line (R-22); the volume-warning line (R-24); and the stdout and stderr of R-28's command, where the copy is of that command rather than the shim (R-28).
  - After a call, no file under the test's temporary root holds the token. The test sets `TMPDIR` inside that root. Control: the same scan finds a token the test plants under `TMPDIR`.
  - The static check in `FD-4`'s Observable runs over the tracked script's text. It declares the token variable it tracks, and allows it only in its capture, its validity `case` and its hand-off assignment, each defined by its shape below, and in each only where that shape puts it. The capture is an assignment to the tracked variable whose value is one command substitution running `sigil-helper` and nothing else, written as a simple command with no command word, together with an `unset` simple command before it that has the tracked variable's name, unexpanded, as an operand. The name occurs there only as that assignment's target and that `unset` operand; any other command in the same list or compound command, such as an `||` branch or the body of an `if` whose condition is the assignment, is outside the capture. The check allows that `unset` and does not require it; the runtime test of the inherited export, below, requires it. The validity `case` is a `case` command whose subject word is the tracked variable's expansion and nothing else; its patterns and arm bodies are outside it. The hand-off assignment is an assignment to `GH_TOKEN` whose value is the tracked variable's expansion and nothing else, written as an assignment prefix of the one simple command whose command word is `exec` and which starts the real `gh`. It fails in each of these cases:
    - an `env` invocation, bare or by path;
    - a reference to the tracked variable anywhere other than its capture, its validity `case` and the hand-off assignment, or any assignment to `GH_TOKEN` other than the hand-off assignment;
    - a first statement that is not `set +x +v +a`;
    - any later `set` with a `-` option cluster containing `x` or `v` (`set -x`, `set -eux`), or with `-o xtrace` or `-o verbose`;
    - a first line other than exactly `#!/bin/sh`, which includes a shebang with any flag (`-x`, `-v`, or a cluster such as `-ex`). `BASH_ENV` and `ENV` are not read by `/bin/sh -c`, and `BASH_ENV` is read by `/bin/bash -c` [measured by Warden on macOS, 2026-11-20]. Run through its shebang, a `#!/bin/sh` script sourced neither `BASH_ENV` nor `ENV`; under `/bin/bash`, `BASH_ENV` was sourced, the positive control [measured by the lead on macOS `/bin/sh`, GNU bash 3.2.57, 2026-11-20]. So closing an inherited `BASH_ENV` depends on this shebang;
    - a `PS4` assignment.

    Controls: each of these seeded copies fails it: `exec env GH_TOKEN=...`; `exec /usr/bin/env GH_TOKEN=...`; the first statement moved to second; the first statement replaced by `:`; the first statement `set +x +v`, without `+a`; a later `set -x`; a later `set -eux`; a later `set -v`; a later `set -o xtrace`; a later `set -o verbose`; a first line `#!/bin/sh -ex`; a first line `#!/bin/sh -v`; a first line `#!/bin/bash`; a `PS4` assignment; one copy holding both a standalone `GH_TOKEN=` assignment from the tracked variable and an `export GH_TOKEN`, before the `exec`; a reference to the tracked variable in another statement; a print of the tracked variable inside a validity `case` arm, with the capture, the `case` subject and the hand-off left as they are; a reference to the tracked variable in an `||` branch after the capture, with the capture, the `case` subject and the hand-off left as they are; a reference to the tracked variable in the body of an `if` whose condition is the capture, with the capture keeping its defined shape as that condition, its `unset` before the `if`, and the `case` subject and the hand-off left as they are; the helper's output captured straight into `GH_TOKEN`.

    The static check guards against accidental edits. It is not a complete list of the ways tracing or export can be turned on: `shopt -so xtrace`, an inner `sh -x` or `bash -x`, `eval`, a `trap ... DEBUG` and a later `set -a` are not listed, and no form is added to the list. The backstops are R-13's and R-14's *macOS runtime* tests and the lead-run pull-request read of the shim, which includes their `-v` output (`## Requirements`, "Where each criterion runs"). The token variable's allowed-places rule (capture, validity `case` and hand-off only) is part of the same lexical check: it catches a reference to the tracked variable by its name in any other place, but does not catch one made through a name constructed at run time with `eval`. The check's `GH_TOKEN` half is deliberately stricter than R-14's SHALL: it fails any assignment to `GH_TOKEN` other than the hand-off assignment, whatever value it assigns, and `unset GH_TOKEN`, which is not an assignment, passes.
  - *macOS runtime:* with `SHELLOPTS=xtrace` exported, the shim's stderr holds no token. Control: a temporary copy of the shim without its first statement, `set +x +v +a`, run the same way, shows the token on stderr, and the test fails if it does not. Linux's `dash` ignores `SHELLOPTS`. The static check holds on both.
  - *macOS runtime:* with `SHELLOPTS=allexport` exported, the fake `gh`'s recorded environment holds the token only as the value of `GH_TOKEN`, under no other name. The observable is the fake `gh` because it is the one recording child the change can alter: the fake helper, like every other fake the test supplies, runs before the token variable is assigned, while the `exec`'d `gh` inherits every variable allexport exported. Control: a temporary copy whose first statement is `set +x +v`, run the same way, shows the token under a name other than `GH_TOKEN` in the fake `gh`'s recorded environment, and the test fails if it does not. With `SHELLOPTS=allexport` inherited by a `#!/bin/sh` script run through its shebang, the shim-style token variable reached a child's environment after a first statement `set +x +v` (`/usr/bin/env | grep -c` returned 1), and did not after `set +x +v +a` (0), nor with no `SHELLOPTS` (0, the control) [measured by the lead on macOS `/bin/sh`, GNU bash 3.2.57, 2026-11-20]. Linux's `dash` ignores `SHELLOPTS`.
  - With a variable of the name the static check declares seeded and exported, holding a value other than the token, the fake `gh`'s recorded environment holds the token only as the value of `GH_TOKEN`, under no other name. This is a sibling test of the allexport test, not a seed inside it, and carries no tag, so it runs in `just check` on every platform. Control: a temporary copy of the shim without the `unset` of the token variable, run the same way, shows the token under the declared name in the fake `gh`'s recorded environment, and the test fails if it does not. Warden's measurement: "with `tok=pre` exported by the caller, a shim `set +x +v +a; tok=$(printf 'SECRETTOK123\n'); /usr/bin/env | grep -c SECRETTOK123` printed 1 after the capture. The control with `env -u tok` printed 0. With `unset tok` before the capture the count was 0." [measured by Warden, review round 8, 2026-11-24, on macOS `/bin/sh`, GNU bash 3.2.57, `#!/bin/sh` scripts run through their shebang]. That Linux's `dash` behaves the same is inferred from POSIX, under which a variable taken from the environment is marked for export, and is not measured.

- **R-15** The shim SHALL start the real `gh` with `exec`, by its absolute path, with the call's argv forwarded unchanged. *Anchor: FD-4; FD-7 (the call's argv is not read).*
  - The fake `gh` records the shim's argv exactly, `-R`, `--repo=`, URL and `repos/...` arguments included.
  - The shim's exit status equals the fake `gh`'s.

- **R-16** Each shim call SHALL mint its own token. No token SHALL be stored for reuse. A single `gh` call that runs past its token's hour fails at GitHub and is re-run by the session (QA-6). *Anchor: FD-8; the maintainer's 2a ruling, "per-call"; S9.*
  - Two consecutive calls invoke the fake helper twice. Neither token is found under the test's temporary root afterwards, with R-14's `TMPDIR` and its planted-token control.

- **R-17** When `HOME` is unset, empty, not absolute, or holds a newline, the shim SHALL exit non-zero with an error on stderr, without minting or starting `gh`. No record line is written, since the record's location derives from `HOME`. The value is matched as a whole string. *Anchor: P-SecurityLayered, "a control that errors denies rather than passes through"; R-13's paths derive from `HOME`.*
  - These cases exit non-zero with no helper call: `HOME` unset, `HOME=`, `HOME=relative`, and a two-line `HOME` whose first line is absolute.

### C. The session's repository

- **R-18** The shim SHALL resolve the session's repository exactly as `FD-7` locks it. Each refusal case `FD-7` lists under "When it refuses" SHALL exit non-zero, without minting or starting `gh`, and SHALL record `no-repo` with the repository field empty. *Anchor: FD-7 (D-ProjectIsARepo; P-LeastAuthority; P-SecurityLayered).*
  - **Harness process.** `CLAUDE_PID` must match `^[0-9]+$` as a whole string, with no newline.
  - **Reader.** `lsof -a -p <pid> -d cwd -Fn`, found on `PATH`. Its result is the output lines beginning `n`, with the prefix removed. There must be exactly one, a non-empty absolute path. A non-zero exit refuses.
  - **Path guard.** No empty or relative path ever reaches `git -C` or `cd`.
  - **Clone.** A separate check confirms the path is inside a git work tree.
  - **Origin.** `origin` comes from `git config --local --get remote.origin.url`.
  - **Git environment.** Both git reads run with every `GIT_`-prefixed variable unset in a subshell, never through `env` (R-14).
  - **Origin form.** `origin` must be `https://github.com/<owner>/<repo>`, with one trailing `.git` stripped.
  - **Name halves.** Each half must match `[A-Za-z0-9_.][A-Za-z0-9_.-]*` with ASCII ranges in the C locale, and neither may be `.` or `..`. The value holds no newline or carriage return, matched as one string.

  Acceptance:
  - Every item of `FD-7`'s list "Observable, by gate tests against a fake harness process" holds.
  - A two-line `CLAUDE_PID` whose first line is a valid process id gives `no-repo`.

- **R-19** The shim SHALL NOT read a repository from the call. That covers `-R`, `--repo`, an API path, a positional, a URL, `GH_REPO`, and the call's own working directory. *Anchor: FD-7, "Rejected: reading the call and refusing" (P-SecurityLayered's ceiling on count; V-Simplicity); FD-12.*
  - In each case `FD-7`'s Observable names, the fake helper receives the session's repository, and the fake `gh` receives the call's argv unchanged.

- **R-20** An `origin` whose letter case differs from GitHub's name SHALL be minted for as written. The landing path's name check then refuses it and revokes the token (R-4), and the shim exits non-zero without starting `gh`. *Anchor: FD-7, "Letter case"; S2.* `FD-7`'s last gate Observable needs a key-source seam, which does not exist (R-5), so it is checked in two halves, and the whole chain is not run:
  - the shim passes a mixed-case `origin` to the fake helper unchanged, and a failing helper leaves the fake `gh` uninvoked (R-12);
  - R-4's letter-case case gives `grant mismatch` and one revoke.

### D. The call record and the volume warning

- **R-21** Before it starts `gh`, and on every failure in steps 3 to 5 of R-9, the shim SHALL append exactly one line to the record at `$HOME/.local/state/sigil/gh-shim/<UTC date>.log`, in the format under `## Data Model`. The state directory SHALL be mode `0700` and each record file `0600`, re-applied after creation. *Anchor: FD-9 (P-InstrumentBefore; OC-8; V-Consistency with the cache's mode rule).*
  - Each outcome writes one line with its value: `minted`, `mint-failed`, `no-repo` or `refused`.
  - A pre-existing `0755` directory and `0644` file read back as `0700` and `0600` after a call.
  - A grep of the record for every token minted in the run finds none. Control: it finds a token appended to a copy.
  - Twenty concurrent calls produce twenty well-formed, non-interleaved lines.

- **R-22** When the record cannot be written, the shim SHALL print one stderr line beginning `gh shim: record:`, saying the volume count is unavailable for this call. It SHALL skip the threshold check for that call and continue. *Anchor: FD-9, "operations data is not a gate"; V-Observability, "an instrument that … stops reporting silently is worse than no instrument".*
  - With the state directory's path occupied by a regular file, which R-21's re-chmod cannot undo, the fake `gh` still runs, and stderr holds that line, with no token in it. Control: R-14's shared seeded control, on the `gh shim: record:` line.

- **R-23** After a `minted` line is appended, the shim SHALL count N, the `minted` lines in today's record file. It SHALL print the volume warning when (N + 52) × reads-per-mint is at or above 70% of the daily limit. *Anchor: FD-9 and DEF-1 (V-Consistency: one line for the warning and the rule; V-Observability, "an instrument that … stops reporting silently is worse than no instrument").*
  - **Constants.** Reads-per-mint is held as an integer number of tenths, and the comparison uses integer arithmetic with no truncation. The shipped constants are 3.0 and 1,000, until `LT-7` records a value (level-4 row).
  - **Once a day.** The warning prints at most once per UTC day across concurrent shims. The one that prints is whichever atomically creates that day's marker file (`## Data Model`).
  - **Failures.** When N cannot be counted, or the marker cannot be created for any reason other than a regular file already at its path, the shim prints one line beginning `gh shim: volume:` and continues.
  - **Reader.** Parked, human-noticed, monthly: the lead session that makes R-28's monthly run lists any marker files written since the last run, and reports them to the maintainer.
  - At the shipped constants, the warning fires at N = 182 and not at N = 181. A change to a constant recomputes these two figures in the same change.
  - On a temporary copy with reads-per-mint set to 3.4, the warning fires at N = 154 and not at N = 153; a truncating shim would wait for N = 182.
  - A later call that day prints nothing. Ten concurrent calls past the line print one warning.
  - With the marker's path occupied by a directory, a minting call past the line prints the `gh shim: volume:` line.

- **R-24** The volume warning SHALL be one stderr line beginning `gh shim: volume warning:`. It states today's `minted` count, the assumed limit and reads-per-mint, and tells the session to report it to the maintainer. It holds no token. *Anchor: FD-9.*
  - The fixture's line holds the count, both constants and the word `maintainer`, and no fake token. Control: R-14's shared seeded control, on the volume-warning line.

### E. Wiring and installation

- **R-25** The wiring SHALL be the last `PATH` change in `~/.zshrc`. It prepends the deployed shim directory when `CLAUDECODE` is non-empty and changes nothing otherwise. No other start-up file is edited. *Anchor: FD-6 and its Observable; FD-11 item 5 and the Frame's known-gaps re-sign rule.*
  - **Range (*live*, `LT-6`).** `command -v gh` runs in a lead session, a plain dispatched agent, a worktree-isolated dispatched agent, the maintainer's terminal, and each other spawn path in routine use, named in the run record from `KG-1`'s list. A `--worktree` session and a background session are among those other paths.
  - **What passes.** The shim path prints in each session and agent; the real path prints at the terminal.
  - **A miss in a lead session, in a dispatched agent, or at the terminal** fails `FD-6`'s Observable. The build stops on this requirement, and the Frame is revised before `FD-6`'s `SessionStart` alternative is built.
  - **A miss on any other spawn path** takes R-31 item 5: the maintainer's signature on the re-rated `KG-1`.

- **R-26** This change SHALL add no harness permission rule containing `sigil`. *Anchor: FD-10.*
  - *Live:* the rules containing `sigil` are the same before and after deployment in two places: the user-level harness settings, and the `.claude` settings of the throwaway repository's clone.
  - Control: the same search finds a rule seeded into a copy of the settings.

- **R-27** The README's setup section SHALL document the following, with no token or secret value. *Anchor: QA-7 (V-Reversibility, "name a mitigation path before committing to a change"); FD-4; FD-9.*
  - **Placing the shim:** write it to a temporary name in the deployed directory, then rename it onto `gh`.
  - **Setup:** setting the install-time real-`gh` path, and the wiring line.
  - **Deploy order:** the `sigil` binary, with its SHA-256 and source commit recorded (R-30); a `--landing` mint at the maintainer's terminal (`LT-1`); the shim; the wiring line.
  - **Reversal order:** remove the wiring line; restart running sessions, which keep the shim on `PATH` until then; remove the shim; restore the prior binary (R-37); confirm `command -v gh` gives the real binary in a new session.
  - **Blast radius (`ED-7`):** once the wiring is in place, every lead and dispatched session on the Mac runs `gh` through the shim, and fails visibly outside a covered https clone. This is the one place the spec states it.
  - **Reading:** how to read the record and the marker files, as one command.
  - *Lead-run at the pull request:* the README section holds each item above.

### F. S7's one command

- **R-28** A command tracked beside the shim SHALL take two inputs: the expected session `owner/repo`, and the path of a clone of a different repository. It exits 0 for pass, 1 for fail and 2 for inconclusive. A non-zero exit names its check on stderr (`preconditions`, `byte-compare`, `pin-canary` or `login-check`). Its output holds no token. *Anchor: FD-11, "S7's one command" (P-GuaranteeByMechanism; P-TrustworthySignal, "inconclusive is not failed").*
  - **Preconditions.** If either fails, the result is inconclusive before any shim call:
    - the given clone's `origin`, read as R-18 reads one, resolves to an `owner/repo` other than the expected one;
    - the command's working directory is inside the given clone.
  - **1. Byte comparison.** The file `command -v gh` resolves to is byte-identical to the tracked shim. Otherwise the command fails.
  - **2. Pin canary.** One shim call, `gh api installation/repositories --jq '.repositories[].full_name'`, is made from that working directory. To pass, the call exits 0 and its stdout is exactly one line, the expected `owner/repo`. As corroboration, today's record must also hold a `minted` line for the expected repository timestamped at or after the command's start. Anything else fails. The per-call output decides, because the record carries no session identity. That a narrowed token lists exactly its own repository on that endpoint is inferred; R-30 records its first output.
  - **3. Login check.** `gh api user --jq .type` through the shim does not print `User`; if it does, the command fails. This is a negative smoke check. Its positive control, the real binary printing `User`, is R-36's one-time S4 control, so the command still passes once the login is revoked.
  - **What exit 0 shows.** The pin held for a call made from inside another repository's clone. The cross-call half of `FD-11`'s canary (a `cd` made in an earlier Bash call, not carried into this one) holds only when the session ran the `cd` as its own earlier Bash call. The command cannot tell that from a same-call `cd`.
  - **The cross-call record.** The lead SHALL record the cross-call half from the session transcript as two separate tool calls: the `cd` into the given clone in one, the command in the next. A run whose `cd` was in the same call is recorded as same-call only, never as the cross-call pass.
  - **An inconclusive result** is never recorded as a pass. It is re-run, and one that stays inconclusive is raised to the maintainer.

  Acceptance:
  - With fakes that behave correctly, exit 0. This is the positive control.
  - A deployed copy differing by one byte: exit 1, `byte-compare`.
  - A given clone resolving to the expected repository, or a working directory outside it: exit 2, `preconditions`.
  - A fake `gh` printing another repository, or a failing fake helper with an expected-repository `minted` line seeded into the record: exit 1, `pin-canary`. A record path occupied by a directory also gives exit 1, `pin-canary`.
  - A fake `gh` printing `User`: exit 1, `login-check`.
  - The command's stdout and stderr hold no token. Control: R-14's shared seeded control, on the command's own output.

### G. The first live run, the run with nobody present, and the record

- **R-29** The run record SHALL be kept outside this repository, on this feature's task in the maintainer's tracker, because it names repository B. This repository SHALL NOT name B. *Anchor: FD-11 ("named in the run's record and not here"; this repository is public).*
  - *Lead-run once B is chosen, and again at every later landing:* each of these returns no hit for B's name, case-insensitive: `git grep -I -F -i` on `main` at that point and at each later pull request's head; `git log --format=%B` over the feature's landed commits; every pull request's title and body.
  - Control: the same three searches for `sigil` hit.

- **R-30** The run record SHALL hold a dated result for each of `LT-1` to `LT-10`, as `FD-11`'s table at the Frame lock commit defines them. All come from the first live run, except `LT-7`'s one-day half (R-32) and the closing step's 404 (R-35). The record also holds:
  - `FD-11`'s pre-check on B, recorded before `LT-10`;
  - the deployed binary's SHA-256 and source commit, taken at placement;
  - R-28's first pin-canary output on the throwaway repository.

  A row with no recorded result is the observable failure. *Anchor: FD-11's Observable (P-InstrumentBefore).*

- **R-31** Before the run with nobody present starts, a pre-start record SHALL be written on the task, timestamped to the minute before that run's recorded start. It SHALL cite each of `FD-11`'s five results, or the substitute `FD-11` names:
  1. `LT-7`'s attended half;
  2. `DEF-1`'s evaluation (R-33), with the maintainer's choice when the result is at or above the line or the median wall time exceeds 10 seconds;
  3. a pass of `LT-9`, or his signature on `RISK-B` with the extension route returned to it;
  4. a pass of `LT-10`; a fail stops the run under `FD-7`'s fallback, and has no substitute;
  5. a recorded `LT-6`, with his signature on the re-rated `KG-1` for a miss on any path other than those R-25 routes to `FD-6`.

  The record of the run with nobody present SHALL cite the pre-start record and its own start time to the minute. *Anchor: FD-11 items 1 to 5; P-TrustworthySignal, "inconclusive is not failed" and "where you cannot eliminate it, raise it"; carry-in: a pre-start record for `FD-11`'s gate.*
  - **Inconclusive results.** An inconclusive result never counts as a pass. It is triaged and re-run. One that stays inconclusive is raised to the maintainer as that result's fail or miss path.
  - **The gate is lead-held prose.** No mechanism refuses the start. Its failure is observable afterwards: a run record citing no pre-start record, or one timestamped after the start.

- **R-32** `LT-7`'s one-day half SHALL be taken over the first full UTC day of running and recorded on the task. *Anchor: FD-11's Observable, "fired by the build's close"; P-Defer; carry-in: what fires `LT-7`'s one-day half.*
  - **The day** is the first UTC calendar day that begins after the run with nobody present passed, after the deployed shim passed R-28's command, and whose record file holds at least one `minted` line.
  - **The entry** gives the date; that day's `minted` count; 1Password's reported usage for the account; the implied reads per mint (usage divided by `minted`, an upper bound, since usage includes `sigil get` and other use); and whether the UTC day matches the account's reset, with the reset time.
  - **What follows.** An implied reads per mint above the shim's constant, or a reset mismatch, is reported to the maintainer at the routine update. A change then takes the level-4 rows. Until then the warning knowingly counts the UTC day.
  - **What fires it.** The task SHALL NOT be marked done while this entry is absent. The Frame calls this re-check "the Spec's verify contract". The template carries no such section, so this requirement carries it.

- **R-33** `DEF-1`'s evaluation SHALL use a load count that meets the following. *Anchor: DEF-1 (its fixed window); FD-9's load paragraph; carry-in, the load-count script's home.*
  - **What it counts.** Bash tool calls invoking `gh`, by the 2026-11-09 count's command-text match, which the script defines (its home is the level-3 row). The pre-start record quotes that match rule.
  - **Dating.** Each call is attributed to the UTC date of its own entry timestamp. File modification time only skips files last modified before the window opens.
  - **The window.** The 15 whole UTC days before the evaluation day, which is excluded.
  - **The pre-start record cites:** the per-day counts, the peak, the `LT-7` reads-per-mint, the projected daily reads, the result against the 70% line, and a content hash of the script.
  - **The 2026-11-09 script fails the window rule:** a rolling cut-off by modification time, with the run day included. It is amended before use.
  - **Its home** is a level-3 row.

- **R-34** 1Password usage figures SHALL be read by the maintainer at his terminal, with `op service-account ratelimit` or 1Password's own usage view. Whether that reading gives the account's daily usage and its reset time is not measured (level-3 row). A day's reading is taken after 24:00 UTC that day, and also just before the account's reset where the reset is not at 00:00 UTC; each reading's time is recorded. Before that read's output is first used, he SHALL confirm it carries no secret. Only numeric figures and the reset time are copied into the run record. *Anchor: FD-9 (the "Spec confirms its output carries no secret" item, which no agent here can run: none holds `op`); OC-8.*
  - The run record carries the dated confirmation, and its usage entries hold numbers and times only.

- **R-35** Once the throwaway repository exists, it SHALL be deleted by `FD-11`'s closing step, which the maintainer runs at his terminal:
  1. copy S4's and S5's evidence into the run record;
  2. delete the repository;
  3. read it and record the HTTP 404.

  The step runs after the run with nobody present passes, and before the task is closed or cancelled for any other reason. *Anchor: FD-11's closing step; P-LeastAuthority, "an elevation names the event that ends it"; P-Defer; the level-3 row on the throwaway repository after a failed run with nobody present, which answers the Frame's carried finding DQ3.*
  - **When that run fails,** the lead's report of the failure SHALL state which branch of the answer in the level-3 row on the throwaway repository after a failed run applies: with a fix in hand, the repository is kept for one re-run; otherwise it is deleted. Either action is taken at the maintainer's terminal, by R-35's closing step. A kept repository is closed out by the same step after the re-run passes.
  - **The trigger.** While the repository exists, the task is neither marked done nor cancelled until the 404 is recorded.
  - **An abandoned open task: parked, human-noticed, monthly.** The lead session that makes R-28's monthly run reads the task's state. While the task is open, it asks the maintainer to check at his terminal whether the repository still exists, and to give its disposition. The shim's token cannot see another repository.
  - No automatic deletion or timer is built.

- **R-36** The run with nobody present SHALL record the following evidence. A record line supports a call's own result and never replaces it. *Anchor: FD-11 (S1, S4 and S6 are met only in that run); FD-9, "What S4 can and cannot show".*
  - **Launch:** how the run was launched, and `command -v gh` from inside it.
  - **S1:** a non-terminal-stdout mint for the throwaway repository whose response equals the contract.
  - **S4:** every landing step done through the shim with no prompt, each exit status taken from the session's transcript; a `narrowed` record line at each step's time; `gh api user --jq .type` through the shim, at the start and the end, not printing `User`, while the real binary run directly does; GitHub attributing the pull request and the merge to the App's bot identity; `gh` at the maintainer's terminal acting as him.
  - **S6:** a read call through the shim, made more than one hour after the run's first mint (by transcript time), that exits 0.
  - **S5:** recorded in whichever run carried it, together with the live read of the installation's workflow write.

- **R-37** The prior `sigil` binary SHALL be kept until S1 and S3 to S7 have recorded passes. *Anchor: intake hard constraint on rollback; QA-7 (V-Reversibility).*
  - The task records its path, its SHA-256, and one successful `sigil doctor` run of it at the time of recording.
  - That file exists until the last of those passes is recorded.

- **R-38** Before the earliest real run, a runbook SHALL exist on the task, covering the attended mint, the attended shim run and the run with nobody present. Bolt walks it at the level-3 stop. *Anchor: FD-11 and ED-5 (the first write-capable mint is a hard stop with an operations walk-through); V-Reversibility.* It has four sections:
  - **Pre-checks:** `FD-11`'s B pre-check, the installation's workflow-write read, R-37's recorded SHA-256, and `LT-8`'s check that the throwaway repository's secret lists are empty.
  - **Steps:** R-27's deploy order, and `LT-3`, which runs attended outside the shim because its patched build is never deployed. Every mint whose token prints at his terminal, the attended mint and `FD-11`'s B pre-check mint included, is typed by the maintainer in his terminal, never run by an agent's tool call, which would put the token in a transcript (`KG-2`).
  - **Post-checks.**
  - **Rollback:** R-27's reversal order.

  Acceptance:
  - The task holds the runbook, with all four sections, before the earliest real run's recorded start. Its pre-checks hold `LT-8`'s empty-secret-lists check, and it states that the maintainer types the attended mint and the B pre-check mint in his terminal.

## Out of scope

The build SHALL NOT build any of the following.

**The not-to-build lists of `FD-12` and the intake:** certificate pinning; caller identity or an agent-name input (Non-goal 6); a cache for narrowed tokens (only `DEF-1`(a), by the maintainer's choice, could re-open it); a caller-chosen permissions flag; a wrapper that locks the keyring; a human branch inside the shim; post-call revocation; a rate limiter inside sigil; an allow rule for the new path; a `gh alias` refusal; a shim check of a repository the call names; a session-repository check inside sigil.

**Other actors and paths:**
- tokens for any job but the lead's landing work, or the other actors of task #4566 in the maintainer's tracker (Non-goal 3);
- narrowing `sigil get`'s grant or pinning the branch push (Non-goal 4). This is parked, human-noticed, and read when the maintainer revokes his `gh` login from sessions (`FD-7`);
- revoking his login, or removing his SSH key (Non-goals 1 and 2);
- new GitHub Apps, or changes to the App's permissions (Non-goal 5);
- an approval step on landing (Non-goal 7);
- a key source other than 1Password, or GitHub Enterprise Server (Non-goal 9).

**Declined or rejected:**
- a read-only coverage check for repository B (a Frame review finding, declined as a design change under the 2026-11-19 ruling);
- a `SessionStart` hook, which is `FD-6`'s and `FD-7`'s fallback, built only after a Frame revision;
- a carrier of the session's repository for a future agent sandbox. This is self-announcing: a sandbox that blocks the read makes every shim call fail as `no-repo`.

**Dropped by `ED-5`:** a live private-repository run, a remote extension install in `LT-9`, and monthly secret-list reads.

**Mechanisms not added:** enforcement of R-31, R-35 or R-38 (lead-held prose); automatic deletion of the throwaway repository; a binary drift detector beyond R-28's shim comparison and R-30's SHA-256; a standing detector of a session whose `gh` is not the shim (`KG-1`, signed); a key-source seam for `just check` (`CLAUDE.md`); a deadline inside the shim on the helper.

**Other:**
- changes to `sigil-helper`;
- changes to `.github/**`, to the justfile's `ci`, `check`, `test`, `vet` or `fmt-check` recipes, to `.githooks/**` or `.gitleaksignore`, or to any `gitleaks:allow` comment;
- a CI secret-scanning job (`FL-4`, as amended);
- changes to the harness classifier's treatment of `gh pr merge`, which is tracked outside this repository (`FD-11`).

## Scenarios

Each scenario names where it runs. The R-n criteria are the scenarios of record.

### Scenario: unattended landing mint (function level and skip-gated, R-4 and R-5; live at `LT-1`)

**Given** a configured owner, and stdout a pipe
**When** `sigil token --landing owner/repo` runs, and GitHub's response equals the contract
**Then** stdout holds the token and one newline, stderr is empty, the exit is 0, and the cache directory is unchanged

### Scenario: GitHub widens the grant (function level, R-4; skip-gated, R-5)

**Given** a mocked response that adds `issues: write`
**When** the landing mint runs
**Then** one `DELETE /installation/token` is sent with that token, no token is returned or printed, and the error contains `grant mismatch`

### Scenario: the revoke fails (function level, R-4)

**Given** a mismatched grant, and a revoke that returns 500
**When** the landing mint runs
**Then** no token is returned, and the error contains `revoke failed`

### Scenario: a call aimed at another repository (R-19; GitHub's refusal live at `LT-10`)

**Given** a wired session whose harness process works in a clone of repository A
**When** the session runs `gh api -X POST repos/B/git/refs …`, or sets `GH_REPO=B`, or runs from B's clone
**Then** the helper is asked for A only, `gh` gets the call unchanged with A's token, GitHub refuses the write with 403 or 404, and the record line names A

### Scenario: a session outside a clone (R-18; live at `LT-10`(5))

**Given** a session whose harness working directory is not inside a git work tree, with the maintainer's login in place
**When** any `gh` command runs through the shim
**Then** nothing is minted, `gh` does not start, stderr begins `gh shim: no-repo:`, and the record shows `no-repo`

### Scenario: `gh auth` or `gh extension` through the shim (R-11; live at `LT-5`)

**Given** a wired session in a resolving clone
**When** `gh auth setup-git` or `gh extension install o/r` runs
**Then** the shim exits non-zero, nothing is minted, `gh` does not start, and the record shows `refused`

### Scenario: the mint fails (R-12; live at `LT-5`)

**Given** a helper that exits non-zero
**When** a `gh` call runs
**Then** `gh` does not start, the shim exits non-zero, the helper's error is on stderr, and the record shows `mint-failed`

### Scenario: the volume line is reached (R-23)

**Given** today's record holds 181 `minted` lines, at the shipped constants
**When** a shim call mints
**Then** one `gh shim: volume warning:` line prints with count 182, the marker records 182, and the next call that day prints none

## Data Model

**Entity: call record line.** One line per shim call that reaches step 3 of R-9. It is appended to `$HOME/.local/state/sigil/gh-shim/<YYYY-MM-DD>.log`, where the date is UTC. Fields are tab-separated, in this order, and the line ends with a newline. `durable`.

| Field | Type | Constraints | Notes |
|---|---|---|---|
| time | UTC timestamp, ISO 8601 to the second, `Z` suffix | always present | when the line is written |
| repo | `owner/repo` | empty on `no-repo`; otherwise the session's repository | never the repository the call aimed at (`FD-7`, `FD-9`) |
| class | `narrowed` or `none` | `narrowed` exactly when outcome is `minted` | |
| outcome | `minted`, `mint-failed`, `no-repo` or `refused` | closed set | |
| mint_seconds | decimal seconds, at least whole-second resolution | empty when no mint was attempted | `LT-7`'s median input |

The record never holds a token, a token prefix or hash, `gh` arguments, or a session identity (`FD-9`; KG-5).

**Entity: warning marker.** `<YYYY-MM-DD>.warned`, in the same directory. It is created atomically by the shim that prints that day's warning. It holds one line: the creation time, a tab, and N. It is the durable trace that the line was crossed; R-23 names who reads it. `durable`.

## API Contract

### `POST /app/installations/{installation_id}/access_tokens`: landing mint

- **Request:** authenticated with the App JWT. The body has exactly two fields: permissions `{contents: write, pull_requests: write, checks: read, workflows: write, metadata: read}`, and `repositories` set to `[<repo>]`. This is `FD-2`'s candidate set; only a level-3 decision changes it after `LT-1` to `LT-3`.
- **Response verified (R-4):** `token` non-empty; `permissions` equal to the request map; `repository_selection` `selected`; exactly one entry in `repositories`, named `<repo>`. Neither `private` nor the owner is checked (level-3 row).
- **Errors:** a 201 failing verification is revoked, then exits non-zero; a 201 with an empty token exits non-zero with no revoke; a 401 is retried once with a fresh JWT; any other status exits non-zero.

### `DELETE /installation/token`: revoke

- Authenticated with the minted token. Success is 204. Anything else, or a transport error, produces an error containing `revoke failed`.

### CLI: `sigil token --landing <owner>/<repo>`

- stdout holds the token and one newline on success, and nothing otherwise. stderr is empty on success, and otherwise holds one error with no token bytes. Exit 0 only on a verified grant. No terminal is required. The flag spelling is `durable`.

## Constraints

- **Security invariants.** The `CLAUDE.md` security invariants stay in force: the App key never on disk; the service-account token env-only; one channel for a token; cache modes `0600` and `0700`; routing fail-closed before the key read on all four token paths; no token material in errors.
- **Dependencies.** No new dependency without a level-3 approval. The shim uses POSIX `sh`, `git`, `lsof` and base system tools.
- **Control surfaces.** The shim and R-28's command fail closed, in shell as in Go (OC-2).
- **Where acceptance runs.**
  - On macOS, no `just check` test of R-1 to R-5, R-8 to R-28, and R-39 skips, other than R-5's 1Password-gated test. A missing `sh`, `git` or `lsof` fails the test.
  - On Linux, a test SHALL put on `PATH` a fake `lsof` that emits R-18's format from `/proc/<pid>/cwd`.
  - The macOS skip count, counted by `CLAUDE.md`'s `grep -c '^--- SKIP'` method at the build's close, rises from 2 to 3. The third skip is R-5's test.
  - A green `just check` does not cover the built binary's mint path or the R-20 chain.
- **Fixture tokens.** Fixture token strings SHALL NOT match the secret-scan rules: build them at runtime, in a shape unlike `ghs_…`. No `gitleaks:allow` or ignore entry is added.
- **Public repository.** This repository is public. No tracked file names repository B, a path in the maintainer's private research store, or a tracker activity id. Tracker task numbers are written as "task #N in the maintainer's tracker".
  - *Lead-run at the pull request:* no pull request's diff holds an absolute path under the maintainer's home directory and no activity id. This spec file is excluded, because it states the patterns.
  - Control: the same search finds a seeded line.
- **Degradation.**
  - The shim sets no deadline on the helper. The helper's 30-second context bounds the network waits, but not the 1Password SDK's local load (`CLAUDE.md`). The harness's tool timeout is the outer backstop.
  - Any outage of 1Password or GitHub records `mint-failed`, with the helper's error on stderr.
  - The daily 1Password limit is per account, and shared with `sigil get` (`FD-9`). Once it is reached, `gh` stops on every lane, and git stops within 55 minutes.
- **Parked, human-noticed** (intake S7, `FD-11`).
  - R-28's command is run by the lead session that replaces the binary or the wiring, monthly in a lead session started in this repository's clone, and after a Claude Code upgrade, each time recorded as R-28's cross-call record states.
  - The monthly run also carries R-23's marker read and R-35's abandoned-task read.
  - A run is recorded as a pass only on exit 0.

## First real run

- **Before anything runs:** R-38's runbook is on the task.
- **The earliest real run.** One attended landing mint, at the maintainer's terminal: `sigil-helper token --landing <throwaway owner>/<repo>`, against `FD-11`'s throwaway repository in the organization installation, with real 1Password and real GitHub.
  - It records `LT-1` and needs only R-1 to R-7.
  - It is the first write-capable mint, which makes it a level-3 hard stop: the maintainer is present, and Bolt walks the runbook (`ED-5`).
  - `LT-8`'s secret reads are recorded at the repository's creation and at this run.
- **The attended shim run.** A lead session started in a clone of the throwaway repository, using R-8 to R-28, and R-39.
  - Deploying the wiring is part of this level-3 stop (R-27 states the blast radius).
  - It runs `LT-2` to `LT-7` (the attended half), `LT-9`, `LT-10` (which also covers S3), and R-28's command.
  - `LT-3` runs attended, outside the shim.
- **What stays off until later.** The run with nobody present waits for R-31's pre-start record: a lead-held gate, not a code toggle. R-32 and R-35 follow that run.
- **Not covered by these runs:** no private repository is run live (`FD-11`).

## Decision levels

Every decision this build meets has a level. "Wait until told" and "ask what to do" are not levels.

- **3, recommend and wait for approval:** a scope-bearing premise, your own included, raised the moment you hold it and never parked for retro; a new dependency; the first run on real data, with Bolt walking the runbook before it; any change to the floor, actor set, bar or known-gaps list, signed or not; a decision to decline a floor control; the 3-round ask. A decision that touches an actor, a floor, the bar or a known gap counts as that stop. In doubt, it touches.
- **4, act and tell at once:** reversible, but the decomposer may want to overturn it.
- **5, act and report at the routine update** (the grant's reporting point or the landing's report): gate mechanics, reversible choices, anything where building proceeds either way.

A choice no row answers that is durable or rests on context the builder lacks is raised before acting. Levels never close a finding or replace a gate signature.

| Decision this build will meet | Level | Answer, or the default the builder takes |
|---|---|---|
| A change to the landing permission constant after `LT-1`, `LT-2` or `LT-3` | 3 | No change at this gate. A live result that calls for one stops the run and goes to the maintainer, with the response quoted. `ED-1`(d) holds |
| The first write-capable mint (the earliest real run) | 3 | The maintainer is present, and Bolt walks R-38's runbook first (`ED-5`) |
| Deploying the wiring line | 3 | Part of the attended shim run's stop (`ED-7`, accepted; R-27) |
| Starting the run with nobody present | 3 | Only with R-31's pre-start record complete, and on the maintainer's go |
| `LT-6` misses in a lead session, a dispatched agent or at the terminal | 3 | `FD-6`'s failure: the build stops on R-25, and the Frame is revised |
| `LT-6` misses on another spawn path; `LT-9` fails; `LT-10` fails (inconclusive after triage included) | 3 | `FD-11`'s named paths: his signature on the re-rated `KG-1`; his signature on `RISK-B`; or `FD-7`'s fallback and a Frame revision |
| `DEF-1`'s result at or above the line, or median mint wall time above 10 s | 3 | His choice of `DEF-1`(a) or (b), recorded in the pre-start record |
| Choice of repository B | 3 | The lead proposes a repository meeting `FD-11`'s criteria; he reads them at his terminal; the pre-check is recorded before `LT-10` |
| The throwaway repository after a failed run with nobody present (the Frame's carried finding DQ3: Cerberus, Frame review round 10, Domain Question 3) | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": keep it for one re-run when the fix is in hand; otherwise delete it now. Either way, by R-35's closing step at his terminal |
| Where the `DEF-1` load-count script lives (carry-in) | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": the maintainer's research store, amended to R-33's window, with the pre-start record citing its hash. Rejected: tracking it here, a second ecosystem (P-StackDiscipline) for a one-time input; a Go port, a second package in a flat single-package repository for the same one-time use |
| The real-`gh` path resolving to the shim itself (R-10 accepts it; each `exec` re-enters and mints again, spending the 1Password limit, `RISK-C`) | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": one refusal in R-10 when the path is the shim's own file (by path or file identity), before any mint, with one acceptance case. That refusal is R-39, which this answer makes active. Touches `KG-9` |
| The override variable's name, and where the install-time real-`gh` path is held | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": override `SIGIL_GH_REAL`; install-time path as one line in `$HOME/.config/sigil/gh-real-path`, outside the shim directory and R-28's byte comparison. Touches `KG-9` |
| R-4 checks the repository name, not its owner, as `FD-3` locks it (a Frame question) | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": no change at this gate. A same-named repository under another owner verifies only if the config maps two owners to one installation, which is a config error or `RR-6`. Comparing `full_name` would revise `FD-3` |
| The harness classifier still refuses `gh pr merge` when the run with nobody present is prepared | 3 | Raise it before the start. No workaround here (`FD-11`) |
| `LT-1` and S1 need GitHub's raw grant response, which R-4 and R-5 keep off every output (touches `FD-2`'s Observable) | 3 | Options: (a) a verified exit 0 is `LT-1`'s evidence, with key names only on a mismatch; (b) an attended, untracked diagnostic patch, as for `LT-3`; (c) a tracked output, rejected because it breaks the one-channel invariant. Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": (a), since verification is exact-match in both directions |
| How sigil's own changes land once the wiring is deployed | 3 | Options: (a) land all code before the wiring, and reverse the wiring (R-27) before landing any later fix; (b) land through the shim, after a recorded read at his terminal of the personal installation's repository selection. Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": (b), with R-27's reversal as the escape when the shim itself is broken |
| The 1Password reading lacks the account's daily figure or reset time (touches `DEF-1`, `RISK-C` and `FD-11` item 1) | 3 | Options: (a) a usage figure from 1Password's own view, read by him; (b) `LT-7`'s attended half recorded as not measurable, with `DEF-1` evaluated at 3 reads per mint. Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": (a) where it exists, else (b) |
| Any new dependency, including a shim linter | 3 | None approved |
| Removing or changing any line of an existing `*_test.go` | 3 | None (R-6) |
| Flag spelling `--landing` | durable | `--landing` (`FD-1`) |
| `GH_CONFIG_DIR` and `XDG_DATA_HOME` values | durable | `$HOME/.config/sigil/gh` and `$HOME/.local/share/sigil/gh-data` (R-13). Every lane shares them |
| Record location, line format and marker | durable | As under `## Data Model`. R-28, R-32, the `LT-7` counts and R-23's monthly read use them |
| Updating reads-per-mint after `LT-7` or R-32 | 4 | A tracked change in tenths, with R-23's two figures recomputed, redeployed, then R-28's command run |
| The daily limit if the plan is not Families | 4 | The limit becomes the plan's daily account limit (`FD-9`) |
| The counting window after R-32 finds a reset mismatch | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": keep the UTC day until a second R-32 entry shows the same mismatch; then count by the account's reset |
| `gh` environment variables beyond R-13 (for example `GH_NO_UPDATE_NOTIFIER`) | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": none beyond R-13 in this build. `KG-8` names this environment, so each addition goes to the maintainer |
| In-repository names for the shim and R-28's command | 5 | Builder's call; the deployed directory holds only `gh` |
| The deployed shim directory's path | 5 | Builder's call, outside every directory already on `PATH`; recorded in the README (R-27) |
| `SHELLOPTS` dropped from R-13's unset list; the shim's first statement, `set +x +v +a`, carries `FL-2`'s protection against an inherited `SHELLOPTS=xtrace` (touches a floor item) | 3 | Answered by the maintainer, 2026-11-24, "Accept set +x +v +a (Recommended)", as taken: R-14's first statement, `set +x +v +a`, carries `FL-2`'s protection in place of unsetting `SHELLOPTS`, with R-13's and R-14's macOS runtime tests and R-14's static check behind it. The measurements behind it, on macOS `/bin/sh`, GNU bash 3.2.57: `unset SHELLOPTS` fails there as read-only (measured by Cerberus, review round 7, 2026-11-24: his probe printed `unset: SHELLOPTS: cannot unset: readonly variable` and `rc=1`, and the claim holds "with and without an inherited value (stdin path, not the shebang path)"); with `SHELLOPTS=allexport` inherited, the token variable reached a child's environment after `set +x +v` and not after `set +x +v +a` (R-14, measured by the lead); after `set +x +v +a` the `SHELLOPTS` a child inherits holds no `xtrace` and no `allexport` (R-13, measured by Cerberus and Warden). `+a` is required because without it an inherited allexport hands the landing token to every later child by environment, against R-14. Rejected: unsetting `SHELLOPTS`, which errors on every call there |
| Whether `P-SecurityLayered`'s deferred rules SL1 and SL2 fire for this work. Their trigger: they "fire when a second standing autonomous capability lands beyond the unattended-credential decision (G-0016) — a new credential, a new agent class, or a new unattended loop". The landing token is a new credential used with nobody present | 3 | Answered by the maintainer, 2026-11-24, "Fires; your ruling declines SL1 (Recommended)": the trigger fires. SL1, separation of privilege ("require more than one condition or actor to authorize a consequential action"), is declined for landing by his ruling of 2026-11-09 on task #4566 in the maintainer's tracker, where he confirmed a shape with no approval rule and landings unattended. His revisit condition, verbatim: "if it comes back as a real problem we can revisit". SL2, least common mechanism, is covered by the Frame's `KG-8` (the dedicated config and data directories every lane shares) and `RISK-C` (the one 1Password account limit every lane shares). Canon does not define "a new credential" for this trigger. Parked, human-noticed: the revisit condition is read at the retro after the run with nobody present (task #4743 in the maintainer's tracker); the definition of "a new credential" is task #4950 in the maintainer's tracker |
| The shim's language: a POSIX shell script (R-8) in a Go repository. Canon leaves unruled which of `P-StackDiscipline` and `P-RustDefault` owns the choice (`P-StackDiscipline`: "Which principle owns an ecosystem-adoption decision is unruled"). Raised by Charon's canon review | 3 | Answered by the maintainer, 2026-11-24, "Keep shell, record why (Recommended)": shell. The shim only sets the environment and `exec`s `gh`, and the shell hazards (tracing, allexport, an inherited export, prints in the allowed places) are specified and seeded in R-13 and R-14. A later port stays open |
| The locked Frame's `FD-9` anchors the runtime warning on `P-TrustworthySignal`, which canon scopes to verification signals; R-22 and R-23 anchor on `V-Observability` instead. Raised by Charon's canon review | 3 | Answered by the maintainer, 2026-11-24, "Accept all as written (Recommended)": the Frame is left as it is, and behaviour is unaffected. The citation is fixed at the next Frame touch, because a re-lock now would re-open the Frame-scope closures. Parked, human-noticed: task #4954 in the maintainer's tracker fires when a Frame revision for this feature opens; no detector exists |
| Reusing `parseOwnerRepo` and `revokeNarrowedToken` unchanged | 5 | Reuse (Frame routine decisions) |
| Error wording after the pinned markers | 5 | Builder's call, with no token bytes |
| Record retention | 5 | Keep every file |
| `CLAUDE.md` updates (layout table, token-path invariants, control surfaces) | 5 | In each pull request that changes what it describes, scoped as the Purpose scopes "no route to the login" |

## Intent self-report

*Non-normative: the Requirements govern.*

**JTBD reading.** The routine `gh` path in a lead session defaults to a credential bounded by the session's repository, the landing job and one hour, with no human present. Nothing here bounds a session that reaches wider on purpose (`RR-1` to `RR-7`).

**Strain.**

1. The Frame relies on "the Spec's verify contract" for `LT-7`'s one-day half and the recorded 404. The template forbids that section, so R-32 and R-35 carry them, fired by the task's close, which the lead holds.
2. R-31, R-35 and R-38 are lead-held prose. A miss shows only afterwards, in the run record.
3. S4's record cannot show that no call went out on the login (Frame strain 7). R-36 takes each call's result from the transcript.
4. R-28's canary rests on an inferred property of `installation/repositories`, confirmed at its first live run. Its cross-call half is lead procedure: exit 0 cannot show the `cd` came from an earlier Bash call, so R-28 makes the lead record it. The Frame's within-call repeat is dropped, because once the precondition puts the working directory inside the given clone it repeats the same call.
5. R-25 sends a miss in a lead session, a dispatched agent or at the terminal to `FD-6`'s failure, following `FD-6`'s Observable. It sends every other path, the `--worktree` and background sessions included, to `FD-11` item 5's signature. The Frame's `KG-1` text also covers the first group, so `FD-6`'s stop is read as taking precedence there.
6. The load-count script lives outside this repository, in the maintainer's research store (the answered level-3 row on where it lives), so a cold reader cannot re-run `DEF-1`'s evaluation from the tree. The content hash in the pre-start record ties it.
7. `just check` does not reach the built binary's mint path, so S2's CLI half is skip-gated and live only.

No strain on Non-goals 1 to 9, or on the 2026-11-11 pin ruling.

## Dismissed review flags

- **Frame-scope closure** — Bolt round 1, F-12, "the attended mint prints the token to a terminal". Actors as the reviewer stated them: ACTOR-2, ACTOR-1. Cell as the reviewer stated it: low/low, below the Frame's bar. Closed by two Frame items at frame-locked commit dd9ca140cab2818189d767e751d07fe47a788caf: `FD-1`, "Emits the token on stdout only", so stdout is the mint's product; and `FL-2`'s scope sentence, "By his ruling the item governs secrets from a store: the App key and the service-account token." Warden, round 4, re-checked the closure against the Frame's `## Floor check` and found it engages no floor item: `FL-2` covers store secrets, and `FL-3` covers error text, the record and failure behaviour, while stdout is the product (`FD-1`). The closure's premises are kept by R-38: `LT-8`'s pre-check, and the maintainer typing the attended mint (`KG-2`). Signed by the maintainer, 2026-11-24, "Sign the list as written (Recommended)".
- **Frame-scope closure** — Glitch round 8, G-2, "'the helper's output captured straight into `GH_TOKEN`' is silent on whether the tracked capture remains": the R-14 seeded copy that captures the helper's output straight into `GH_TOKEN` does not say the normal capture stays in the copy. Actors as the reviewer stated them: ACTOR-1, and ACTOR-3 through ACTOR-1. Cell as the reviewer stated it: low/medium, below the Frame's bar. No floor item engaged. Frame lock commit dd9ca140cab2818189d767e751d07fe47a788caf. Signed by the maintainer, 2026-11-24, "Sign the list as written (Recommended)".
- **Frame-scope closure** — Warden, pre-lock read, F-2, "R-35's first bullet has the lead decide whether a fix is in hand, and a failed re-run has no stated outcome". Actors as the reviewer stated them: ACTOR-1, and ACTOR-3 through ACTOR-1. Cell as the reviewer stated it: low/medium, below the Frame's bar. No floor item engaged. Frame lock commit dd9ca140cab2818189d767e751d07fe47a788caf. Signed by the maintainer, 2026-11-24, "Confirm with the F-1 clause (Recommended)", which added it to the signed list.
- Cerberus round 8, F-B, "Line 145, 'the one simple command whose command word is `exec` and which starts the real `gh`' presupposes a single such command and a text test for 'starts the real `gh`'": the hand-off definition assumes one `exec`. Not security-tagged. Dismissed: R-9's step order has exactly one `exec`, and the builder's matcher is held to the seeded copies. Signed by the maintainer, 2026-11-24, "Sign the list as written (Recommended)".
- Cerberus round 8, F-C, "Line 134 states the read-only fact with no tag; only line 520 carries one" (line numbers of the round-8 draft; the row is the `SHELLOPTS` row under `## Decision levels`): R-13's read-only `SHELLOPTS` sentence has no tag of its own. Not security-tagged. Dismissed: the tag is in the decision row. Signed by the maintainer, 2026-11-24, "Sign the list as written (Recommended)".
- Glitch round 1, F-3, "the CLI-level mint and verify paths have no route through `just check`". Not security-tagged. Closing it needs a key-source seam that this repository records as deliberately not added (`CLAUDE.md`, "What is still open"); the spec instead states where each part is checked (R-4 at function level, the CLI path skip-gated, R-20 in two halves). Recorded by Puck, round 1.

## Carried review findings (decomposer override)

None.

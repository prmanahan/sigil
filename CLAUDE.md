# sigil — project instructions

A git credential helper that mints short-lived GitHub App installation tokens, so unattended
agent runs reach GitHub without a human unlocking the 1Password desktop app. Built 2026-06-28;
reviewed, shipped, and in daily use since. Home: `github.com/prmanahan/sigil`. Licensed Apache-2.0
(`LICENSE`).

Single flat Go package, 24 Go files, ~5.5k lines.

## Read this before treating anything here as settled

Where a comment and the code disagree, the code is right and the comment is a fossil. Most stale
comments were reworded before publication, but none of this is machine-checked.

## Layout

Flat `package main` at the repo root. No `cmd/`, no `internal/`, no subpackages.

| File | Responsible for |
|---|---|
| `main.go` | CLI dispatch (`run()`), the `get` / `token` / `doctor` handlers, `dispatchToken`'s argument-polarity parser, config + key loading, PEM parsing, credential-protocol I/O |
| `config.go` | The three-field TOML config and its loader |
| `routing.go` | `resolveInstallation` — repo owner → installation ID. Fail-closed, no default |
| `jwt.go` | `mintAppJWT` — the RS256 App JWT. `now` is injected, for testability |
| `exchange.go` | The App-JWT → installation-token HTTP exchange (nil body, full permissions), and the `errUnauthorized` sentinel |
| `token.go` | The seven-step orchestrator; the only file that composes routing + cache + jwt + exchange |
| `token_narrow.go` | `sigil token --rate-limit-only`: the fixed-body (`{"permissions":{"metadata":"read"},"repositories":[repo]}`) access_tokens POST, `verifyNarrowedGrant`'s exact-match check against GitHub's actual response, and revoke-on-mismatch. Sibling to `token.go`/`exchange.go` — never imports `cache.go` |
| `cache.go` | On-disk token cache, `0600`-enforced. Only `token.go`'s full-permission path reads/writes it |
| `*_test.go` | Tests. `credential_channel_test.go` pins the stdout/stderr channel separation of `writeCredentialResponse`; `parse_key_test.go` covers `parseRSAPrivateKey`; `lazy_key_test.go` and `cli_cache_test.go` pin the routing → cache → key-read order |
| `justfile` | `build` · `test` · `vet` · `fmt` · `fmt-check` · `check` · `ci` (`check`, then `govulncheck` at a pinned version: exactly what GitHub CI runs, so run it before opening a PR) · `hooks` (installs a COPY of the pre-commit hook into the clone's git hooks dir) · `secret-scan` (full history and tree scan). Neither `hooks` nor `secret-scan` is part of `check` or `ci`. ⚠ **No `install` recipe** |
| `.github/workflows/ci.yml` | The CI workflow: `pull_request`, `push` to `main` and a weekly `schedule`, `permissions: contents: read`, no secrets, a job timeout and a concurrency group. One job on `ubuntu-latest` and `macos-latest` that checks out, sets up Go from `go.mod`, installs `just` and runs `just ci`. `just` 1.59.0 is downloaded from its casey/just release by a plain `run:` step and checked against a SHA-256 committed in the workflow matrix (from the release's `SHA256SUMS`, cross-checked by hashing the downloaded asset); a mismatch fails the step. It is not `extractions/setup-just`, which sibling repos use, because that action does not verify its download; a `just` bump means re-reading both hashes. The only actions used are GitHub's own, `actions/checkout` and `actions/setup-go`, pinned to a full commit SHA with the tag in a comment; a bump re-reads the SHA from the action's repository. `.github/dependabot.yml` proposes weekly `github-actions` updates (not `gomod`). `govulncheck` is pinned in the `ci` recipe, so a bump is a justfile change |
| `config.example.toml` | The template. Non-secret fields only; its comments carry the secret-boundary rationale |
| `testdata/*.toml` | Three config fixtures: valid, malformed, and no-installations |
| `LICENSE` | Apache-2.0, unmodified text, no NOTICE file |
| `.gitleaksignore` | One entry: a known false positive on a test fixture string. Together with the inline `// gitleaks:allow` on the same fixture in `token_narrow_test.go`, it configures the secret scan, which two things run: the `.githooks/pre-commit` hook (`gitleaks git --staged`) and `just secret-scan` (`gitleaks git` for history, then `gitleaks dir` for the tree, **run by hand**). The CI workflow does not run it. Keep both; do not widen either. ⚠ **The `.gitleaksignore` entry is commit-keyed**, so it suppresses only the history scan; the inline `gitleaks:allow` is what covers the staged and tree scans. Measured: the tree scan with `--ignore-gitleaks-allow` reports 1 finding despite the entry (the staged case is inferred from the entry's commit-keyed form, not run) |
| `.githooks/pre-commit` | Secret scan of the staged change, failing closed: an unreadable staged set (a plain `git diff --cached` preflight), a missing `gitleaks`, a scan error, a finding, **or an `ERR`-level line on gitleaks' stderr even at exit 0** all refuse the commit; only exit 0 and no `ERR` line passes. ⛔ **The stderr check is load-bearing — do not remove it as redundant.** gitleaks 8.30.1 logs a failure of its own git step (measured: `GIT_INDEX_FILE=/dev/null`) at `ERR` and still exits 0 with `no leaks found`, so its exit status alone reads an unscanned change as clean. ⚠ **Per-clone and locally bypassable, and `git commit` only** — `just hooks` installs a *copy* into the clone's git hooks dir (never committed, so it does not travel with a clone), a later edit to `.githooks/pre-commit` needs a re-run, and `git commit --no-verify` skips it. A velocity aid, not a boundary; CI does not run the scan, so no CI layer backs it. Needs `gitleaks` on `PATH` (8.30.1 measured). `precommit_hook_test.go` pins the decision logic with stub `git`/`gitleaks` in the default gate; it does not run a real gitleaks |
| `.githooks/gitleaks-tree.toml` | Config for `just secret-scan`'s tree half only (explicit `-c`, never auto-loaded): default rules plus an allowlist of `^\.claude/worktrees/` so sibling worktrees do not produce false hits. Deliberately not a root `.gitleaks.toml`, which gitleaks would load for every scan, the hook included |
| **Control surfaces** | `.githooks/`, `.githooks/gitleaks-tree.toml` and `.gitleaksignore` decide what the secret scan sees or suppresses; `.github/**`, the justfile's `ci`/`check`/`test`/`vet`/`fmt-check` recipes and the `go` line in `go.mod` decide what CI checks, because under `pull_request` a PR's own workflow and justfile define its check. A change to any of them is a change to a control: review it as one, and widen narrowly (never disable) |

⚠ **`unexported` buys nothing here.** One package, no import boundary — every identifier is
reachable from every file. Invariants that read as "enclosed" are enclosed by *convention*.

**One exception to the flat package, and it is load-bearing:** `cli_test.go` is
`package main_test`. It holds the only `TestMain`, which is what makes `just check` gate
compilation — see below.

## Verify command

```
just check
```

Chains `vet` → `fmt-check` → `test` (`go vet ./...`, `test -z "$(gofmt -l .)"`,
`go test ./...`). **Measured 2026-11-09 after `go clean -testcache`: all green, exit 0, ~11 s
cold. 113 top-level tests pass, 2 skip, 0 fail.**
⚠ **State the counting method, because it has already drifted once**: this number is
top-level only — `go test -count=1 -v ./...` then `grep -c '^--- PASS'` and `'^--- SKIP'`,
which counts neither subtests nor `TestMain`. The tree holds 116 `func Test`; one is `TestMain`,
leaving 115 top-level tests = 113 pass + 2 skip.
**`just ci` is `check` plus `govulncheck`, and it is what GitHub runs** (`.github/workflows/ci.yml`, on
`ubuntu-latest` and `macos-latest`); run it locally before opening a PR. ⚠ **Measured locally 2026-11-11, not yet on GitHub:**
`just ci` on go directive 1.26.10, 14:52:55 to 14:53:04, exited 1 (govulncheck found GO-2026-6859, exit 3); at e99273b on
go1.26.11 it exited 0 (14:54:44 to 14:54:59, "Your code is affected by 0 vulnerabilities"). It has not
run on GitHub, and no run has yet been seen red there: a first green run does not show the gate can fail.
The Linux job skips the pty CLI tests (they run only on darwin), so the macOS job is *expected* to be
the one with pty coverage; that is not yet confirmed on a hosted runner.
⛔ **The skip count is 2 on macOS, gated on `OP_SERVICE_ACCOUNT_TOKEN` + `SIGIL_TEST_OP_KEY_REF`**,
and the two skipped tests below remain the only coverage of the happy path and the end-to-end
stderr leak scan. ⚠ **Those two are the only skips measured, and they are not the only ones
possible.** Three more are platform-conditional and did not skip on macOS: the two CLI pty tests
(`cli_test.go`, `cli_cache_test.go`) skip unless `GOOS` is darwin (they use the BSD `script`
argument form) or when `script` is absent, and `tty_test.go` skips when `/dev/ptmx` cannot be
opened. On another OS the skip count will be higher than 2 and the pty coverage will be absent;
that has not been measured here. ⚠ `just check` prints `(cached)` on a warm test cache; `go clean
-testcache` first when the run itself is the evidence.

**`check` has no `build` recipe and does not need one.** `cli_test.go`'s `TestMain` runs
`go build` into a temp dir and panics on failure *before* any test executes, and Go compiles
the internal and external test packages into one binary with one `TestMain`. So the build
gates every test in the repo. **Positive control, measured 2026-08-21:** with a fully warm
cache, `go test -count=1 -run '^NoSuchTestZZZ$'` — matching *zero* tests — still took 1.456 s.
That residual is the build running. ⇒ A compile break fails `just check`.

⛔ **`just check` DOES NOT cover the happy path.** Two tests skip unless **both**
`OP_SERVICE_ACCOUNT_TOKEN` and `SIGIL_TEST_OP_KEY_REF` are set, and they are the two that
matter most:

| Skipped test | What goes unchecked |
|---|---|
| `TestCLI_HelperGet_ValidGitHubPath` | The **entire end-to-end path**. A default run never proves the helper emits `username=` / `password=` at all |
| `TestCLI_LeakScan_TokenNotInStderr` | The **only** end-to-end check that a token does not reach stderr, across the whole binary (`get` and `token --rate-limit-only`). `writeCredentialResponse` alone is covered in the default gate — see below |

Both are gated on real 1Password, not on network or GitHub — everything else mocks GitHub with
`httptest`. Green here means *"the units are fine and nothing leaked a PEM"*, never *"it works."*

⛔ **The false green, narrowed but not closed.** `fmt.Fprintln(os.Stderr, token)` added inside
`writeCredentialResponse` once left `just check` at exit 0, all tests `ok`: the in-comment claim
that the function "has no code path to `os.Stderr`" was backed only by a test that declined to
check. **Now:** `TestWriteCredentialResponse_TokenReachesOnlyTheGivenWriter`
(`credential_channel_test.go`) swaps the `os.Stdout` and `os.Stderr` package variables for pipes
(`captureStd`), calls the function, and fails if the token reaches either. The same seed was
re-applied and the test failed on it (stderr contained the token); reverted, it passes.
⚠ **The swap sees only writes made through those two variables.** A write to file descriptor 2
directly (`syscall.Write`, a child process) never touches the variable and would pass. The
black-box CLI test below runs the built binary and reads its real descriptors, so it does catch
that, but only on the paths it drives.
⚠ **What it covers, exactly:** that one function, `writeCredentialResponse`, in the default gate
with no 1Password. A second default-gate check goes through the built binary:
`TestCLI_Get_CacheHit_NoKeyRead_TokenOnStdoutOnly` (`cli_cache_test.go`) seeds a cache entry,
runs `sigil get` with no service-account token, and asserts the token is on stdout and stderr is
empty — so a token printed to stderr anywhere on **`sigil get`'s cache-hit path** (config load,
routing, cache read, output) now fails the gate. **What it does NOT cover:** the cache-**miss**
path — the mint, the 401 retry, `exchange.go`, error formatting after a key read — and
`sigil token <owner>`'s and `--rate-limit-only`'s stderr (the pty test only checks the cached
token appears; it does not assert stderr), and `doctor`. A `fmt.Fprintln(os.Stderr, token)` added
on any of those still leaves `just check` green. The wider invariant ("the token leaves by exactly
one channel") for those paths remains carried by the skip-gated `TestCLI_LeakScan_TokenNotInStderr`
plus convention; it is **not** closed. Closing it needs an in-gate run of the built binary that
reaches the mint path against a fake key and a mock API — which needs a non-test seam for the key
source, deliberately not added.

⚠ **`parseRSAPrivateKey` is covered — `parse_key_test.go`, keys generated in-process.** It is
security-critical and the function behind the one production bug this repo has had (PEM newlines
stripped by a concealed 1Password field). Covered: valid PKCS#1; valid PKCS#8 RSA; PKCS#8 holding
ECDSA and Ed25519 (error naming the type); no PEM block (empty, garbage); newline-stripped PEM for
both encodings, with an intact-newlines control proving the stripping alone causes the failure; an
unsupported block type (`EC PRIVATE KEY`); truncated DER in both arms. The key-material check
(`assertNoKeyMaterial`) takes the DER bytes that actually reached the function (for the truncated
case, the truncated bytes, not the full key) and tests three windows spread across them, each in
raw, hex (either case), `%q`-escaped and base64 (standard and URL) form. **Seeded and measured:**
removing the PKCS#8 type assertion (`rsaKey, _ := k.(*rsa.PrivateKey)`) made the non-RSA test
fail (`expected an error for a non-RSA PKCS#8 key, got key <nil>`); quoting the input into the
corrupt-DER error, once with `%q` and once with `%x`, made the truncated-DER test fail in both
arms; each reverted, the tests pass. **Not covered:** the real 1Password round trip (the
field-type landmine itself — a concealed field — only manifests there, and `readKey` needs real
1Password, so only the skip-gated tests reach it); the key-material assertion is a windowed substring check, not a proof that no
error path can ever quote bytes; the stdlib's own error text for corrupt DER is asserted not to
leak, not pinned.

⛔ **`semgrep` is NOT in `just check` or `just ci`, and nothing re-runs it.** It was run once, at the
original review. `govulncheck` is not in `just check` but is in `just ci`, so CI runs it on every PR
and every push to `main`; it last ran clean (0 reachable vulnerabilities) on 2026-11-09, at Go
directive 1.26.10. `just ci` reported GO-2026-6859 against 1.26.10 (`just` exit 1);
the directive is now 1.26.11 and `just ci` at e99273b exited 0 with 0 vulnerabilities (both runs
local, 2026-11-11). **No GitHub run has confirmed it yet.** ⚠ **It also runs weekly** (Monday 06:17 UTC, a `schedule` trigger), so an
advisory against an unchanged tree is seen within a week. Per GitHub's docs ("Notifications for scheduled workflows are
sent to the user who last modified the cron syntax in the workflow file",
`events-that-trigger-workflows.md` in `github/docs`), a red scheduled run notifies the user who last
modified the cron line, subject to their notification settings. ⚠ If that commit lands through the App,
no human may be notified (inferred, not measured). **Parked, human-noticed:** a red weekly run is
seen only if someone looks at the Actions tab, cadence weekly. GitHub also disables scheduled workflows after 60 days without repository activity (GitHub's
documented behaviour, not measured here). One direct
dependency drags in a WASM runtime, a plugin framework and an OTLP/protobuf stack — that surface
accrues advisories between runs.

## Remote and merge posture (current, not aspirational)

- **`github.com/prmanahan/sigil`** is the home; default branch `main`. ⚠ **Whether a clone's
  `origin` is wired to reach it through Sigil is a per-clone question** — see *Remotes*, below.
- ⚠ **A ruleset, `protect-main` (id 27038849), is active on `main` — measured 2026-11-11.**
  Rules: `deletion`, `non_fast_forward`, `required_linear_history`, and `pull_request` with
  rebase-only merges, **0 required approvals** and `require_extra_approval_for_unattributed_changes:
  true` (GitHub supplied that one; the seed does not carry it). No bypass actors
  (`current_user_can_bypass: never`). ⇒ A direct push to `main`, a force-push, a deletion and a
  merge commit are refused server-side; a change reaches `main` only through a PR, rebase-merged.
  ⛔ **Zero approvals means whoever can push a branch and open a PR can merge it.** The ruleset
  forces the PR path; it does not put a second person, or any check, on it. It is not review and
  not verification — do not read "protected" as "reviewed".
  ⛔ **Applied once, by hand, with the frozen `~/claude_workspace/scripts/repo-protect`. Nothing
  detects drift** — a loosened or deleted ruleset reads as nothing until someone looks. The repo
  standard that will own it is #4222, not built; until its check exists, re-read
  `gh api repos/prmanahan/sigil/rulesets/27038849` rather than trusting this entry.
- ⛔ **A PR runs the `CI` workflow (`.github/workflows/ci.yml`), and the ruleset does NOT require it.**
  CI runs `just ci` (`vet`, `fmt-check`, `test`, `govulncheck`) on `pull_request`, on `push` to
  `main` and weekly. It does **not** run the 1Password-gated happy-path tests (no secrets there; both skip) and
  does **not** run the secret scan. ⇒ **It is not yet a required check: `protect-main` has no
  `required_status_checks` rule until the ruleset is changed, which waits for the workflow to have run
  green.** A red run does not block a merge, and auto-merge does not wait for it. ⇒ **Until then the
  local `just ci` and the per-clone pre-commit secret-scan hook remain layers the actor being
  constrained can skip** (the hook with `--no-verify` or by never installing it; CI by merging
  anyway). A PR is the forced route, not yet a gate. Do not let a green PR imply the happy path was
  verified, or that a PR with no run was.
- **Land via PR**: squash to one delivery commit, push to `origin`, open the PR,
  rebase-and-merge, then fast-forward local `main` **from the remote** — never advance it
  locally. The sequence above is executed by hand, so any guards a landing skill would carry —
  chiefly the pinned rebase base and the branch-delete predicate — are yours to apply rather
  than automatic.
- **Squash to one delivery commit carrying the task number** before landing.
- **Repo settings, measured 2026-11-11 (`gh api repos/prmanahan/sigil`):** `delete_branch_on_merge`
  `true` (a merged PR's remote branch is deleted; local and worktree branches are still yours to
  delete), `allow_auto_merge` `true`, `allow_update_branch` `true`; **only rebase merge is allowed**
  (`allow_merge_commit` and `allow_squash_merge` `false`). Auto-merge with 0 approvals and no
  required checks merges as soon as the PR is mergeable. `security_and_analysis`: secret scanning
  and push protection `enabled`; non-provider patterns, validity checks and Dependabot security
  updates `disabled`. These are settings, not committed files — they can change with no diff here.
- **Work in a worktree**, never the main checkout.

## Install and runtime wiring

Three pieces outside the repo, and the repo is useless without them.

1. **`~/.local/bin/sigil`** — the built binary. ⚠ **No `install` recipe exists.** `just build`
   writes `./sigil` into the repo (gitignored); the deployed copy is placed by hand. **Nothing
   detects divergence between source and deployed binary** — a green `just check` says nothing
   about what is actually on disk at that path.
2. **`~/.local/bin/sigil-helper`** — a three-line wrapper that reads the 1Password
   service-account token from the macOS Keychain, exports it, and `exec`s the binary. Its whole
   job is the env-var injection sigil deliberately refuses to do itself, keeping the Keychain
   dependency out of the Go binary.
3. **The repo's own `.git/config`** — registers the helper for `https://github.com` and sets
   `useHttpPath = true`, alongside the `origin` remote. ⛔ **Per-clone and never committed**, so it
   does not travel with the repository — see *Remotes*. ⚠ A global `includeIf` arrangement scoped
   to a workspace path is the retired way of doing this; if you find one, it is dead weight, not
   configuration to restore.

Config lives at `~/.config/sigil/config.toml` — `op_key_ref` (an `op://` **pointer** to the App
private key, never key material), `app_id`, and an `[installations]` table mapping repo owner to
installation ID. Copy `config.example.toml` and fill it. ⚠ It holds no secrets by design, but it
does publish the App ID and installation IDs to any local user who can read it.

⛔ **Adding an `[installations]` entry may silently widen what agents can mint — and nothing
here will tell you.** Agent permission rules that keep the full-permission `sigil token <owner>`
command away from agents may deny it by matching the owner name literally. If so, adding an
owner to this table means `sigil-helper token <newowner>` matches no deny rule: an agent can mint
a **full-permission, write-capable** token for that owner, and the first sign of it is the token,
not an error. The narrow `--rate-limit-only` form is unaffected — it verifies its own grant and
fails closed. Check any agent permission rules that match owners by name, in the same change that
adds the owner.

⚠ **This is PROSE, not mechanism, and that is the durable point.** Nothing in this repo checks
the coupling — an allowlist of tool names cannot express a constraint that lives in a command's
*argument*. If a mechanism is ever built, it belongs on the rule side (deny by default,
allow-list the narrow form), not here.

⚠ **No new owner is needed to evade a literal owner match.** `sigil token <owner>/<anything>`
routes exactly like `sigil token <owner>`, because routing reads only the first path segment, so
a rule that matches the argument as a whole string can be stepped around with a suffix.

## Remotes — one, `origin`, over https

⛔ **This section states the CONVENTION, not the state of your clone.** Remotes and credential
config live in `.git/config`, which is never committed — so none of it arrives with a `git
clone` and none of it is true on a new machine until someone wires it. **Check, do not assume.**

**The convention.** An agent's clone of a GitHub-backed repository carries one remote, `origin`,
over https, so every git operation goes through the credential helper and therefore through Sigil:

```
origin      https://github.com/<owner>/sigil.git   → via Sigil → the APP
```

**Confirm before pushing:**

```bash
git remote -v          # origin must be https:// — if it is git@, this clone is UNWIRED
```

⛔ **A clone whose remote is on `git@` is unwired. Do not push agent work from it.** Wire it
first — the README's setup section (step 11, "Register the helper") carries the credential-helper
block, and its optional "second remote" section shows adding an `ssh_origin` SSH remote for a
human's own pushes. That is an addition beside `origin`, never a replacement for it: an agent's
clone keeps `origin` on https, which `git remote set-url origin
https://github.com/<owner>/sigil.git` restores. A `git@` remote bypasses the credential helper
entirely: SSH never consults git's credential system at all, so Sigil is not merely bypassed — it is never asked, and the push authenticates as
whoever owns the SSH key (the maintainer), not as the App.

⚠ **A successful push proves authentication, not *which* helper supplied it** — `gh` holds a
valid token too. To confirm Sigil served a request rather than something else, check that
`~/.cache/sigil/<installation-id>.json` was written at that moment. A bare success does not
establish it.

⛔ **No `url.insteadOf` rewrite as a substitute for wiring the remote.** `insteadOf` applies when
git resolves *any* URL, so the transport actually used can differ from the one configured.
**Measured** in a throwaway repo: with a `git@`→https rewrite set, `git ls-remote --get-url
<remote>` returns the rewritten **HTTPS** URL. Fix the remote's URL itself instead; a rewrite is
the shape that looks like the obvious fix, which is why the ban is recorded rather than assumed.

⛔ **Landing here is done by hand**, as described under *Remote and merge posture*: squash to one
delivery commit, push to `origin`, open the PR, rebase-and-merge, then fast-forward local `main`
from the remote.

## Security invariants — do not regress these

Stated in `main.go`'s header comment. Each names what breaking it costs.

- **The App private key never touches disk.** Resolved in-process from 1Password over the API,
  parsed from memory. ⇒ Breaking it puts the root credential that mints every token on disk;
  recovery is key rotation plus re-install.
- **`OP_SERVICE_ACCOUNT_TOKEN` is env-only.** Never written, never logged, no field for it on
  `Config`. The wrapper injects it; sigil never reads the Keychain. This is secret zero.
- **The token leaves by exactly one channel.** ⚠ Signature and convention, plus two default-gate
  checks: a test for `writeCredentialResponse` alone, and the built-binary check on `sigil get`'s
  cache-hit path — see the false green above. **This is the invariant most
  likely to break silently.**
- **Cache files and the cache dir are `0600`/`0700`, re-chmod'd explicitly** after creation,
  because POSIX ignores the mode argument when the file already exists. ⇒ Deleting the "redundant"
  chmod leaves a pre-existing `0644` file at `0644`, holding live GitHub tokens. **This is the
  best-mechanised invariant in the repo** — defensive code, correctly tested, reasoning inline.
- **The cache holds a token and an expiry, nothing else.**
- **Routing is fail-closed and runs before any network call and before the key read, on all three
  token paths** (`sigil get`, `sigil token <owner>`, `sigil token --rate-limit-only`). Unknown
  owner errors; no default installation is ever guessed. ⇒ Breaking it mints a token for the wrong
  installation.
- **Order on `sigil get` and `sigil token <owner>`: config → API-base scheme check → (token path
  only) terminal check → owner routing → cache lookup → key read, on a cache miss only.** A cache
  hit or an unknown owner never reads the key, so never touches 1Password; the key is loaded once
  and reused for the 401 retry (`getInstallationTokenLazy` in `token.go`). ⇒ Breaking it makes
  every `git` operation read 1Password even on a cache hit, and makes an unknown owner fail with a
  key error instead of the routing error. `--rate-limit-only` is config → scheme check → routing
  → key read, and never reads the cache.
  Pinned in the default gate: the key-load order and call counts by `lazy_key_test.go` (key
  loader), and the same order through the built binary with no service-account token by
  `cli_cache_test.go` — cache hit for `get` and `token`, unknown owner for `get` and
  `--rate-limit-only` (none for `token <owner>`), and the terminal gate refusing under a pipe
  **with a live cache entry present** (`TestCLI_Token_NonTerminal_LiveCache_StillRefuses`; moving
  the gate into the key-loader closure made it fail while the empty-cache refusal test stayed
  green). The scheme-before-key-read part is asserted for `sigil token`
  (`TestCLI_InsecureAPIBaseURL_ExitsNonZero`) and unasserted for `sigil get` and
  `--rate-limit-only`.
  One 30-second context, created before the key read, covers the key read and the API exchange
  together on all three paths: `readKey` hands it to the 1Password client and the secret
  resolution, so a slow key read spends part of the same budget. ⚠ **That deadline bounds the
  1Password network waits, not everything the SDK does.** The WASM core load and guest compute
  are not interrupted: onepassword-sdk-go v0.4.0 sets no manifest timeout (so
  `WithCloseOnContextDone` stays off), and `GetExtismCore` loads the core under
  `context.Background()`. Do not read the 30 s as a ceiling on a run.
  Pinned: `getInstallationTokenLazy` passes the caller's context, deadline included, to the key
  loader (`TestGetInstallationTokenLazy_LoadKeyReceivesCallerContext`). Not pinned: that `readKey`
  forwards it to the SDK, at its call sites in `cmdGet`, `cmdToken` and `cmdTokenNarrowed`, or
  that the SDK aborts on it. The test proves only that the lazy function forwards its context;
  the rest needs real 1Password.
- **Error messages carry no key or token material.** They quote types, status codes and lengths,
  and also file paths (the cache directory), URLs (the API base) and GitHub response strings (the
  granted permission names, repository name and `repository_selection` in `verifyNarrowedGrant`'s
  mismatch errors). Wrapped library errors pass the library's own text through unchanged: the
  PEM/DER parse (asserted not to leak key bytes for `parseRSAPrivateKey`), and the 1Password SDK
  errors in `readKey` (client init, secret resolve), the JSON decodes and the HTTP transport
  errors, none of which is asserted.
- **Cached expiry is GitHub's `expires_at` minus five minutes**, so a cache hit can never hand
  back a token within five minutes of death.
- **A `--rate-limit-only` token is verified against GitHub's actual grant before it is trusted
  or printed, and is never cached.** `token_narrow.go`'s `verifyNarrowedGrant` checks
  `permissions`, `repository_selection` and `repositories` from the response — five conditions,
  all ANDed, exact match (the permission set; `repository_selection`; the repository count; the
  repository name; and a refusal of a private repository) — not from what was requested. Any
  mismatch revokes the token (`DELETE /installation/token`) and returns an error instead. ⇒ Breaking the verification lets
  a silently-widened grant reach stdout. GitHub's grant does not always match the request: an
  access_tokens body of `{"permissions":{}}` has been observed to grant every installation
  permission including write, and `{"permissions":{"metadata":"read"},"repositories":[]}` to
  select all repositories. Do not weaken `verifyNarrowedGrant` to a single
  `permissions["metadata"]=="read"` check, which passes the first of these silently.
- **The full-permission `sigil token <owner>` refuses a non-terminal stdout**, from the
  credential-free preflight band: after the scheme check, before the App key is read, before any
  network call — so a refused run reads no secret and makes no `access_tokens` POST. ⇒ Breaking
  it turns the ordinary unattended invocation (a pipe, a redirect to a file, an agent shell)
  from a loud immediate failure back into a silent mint.
  ⛔ **This one is a SPEED BUMP, NOT A BOUNDARY — and this section is exactly where that gets
  misread.** Four gaps are enumerated at `requireTerminalStdout` in `main.go` and all four are
  open: `sigil get` is itself an unattended full-permission token printer by design (see
  gotchas — it dominates the other three); the cache already holds a live token on disk, so
  `sigil token <owner> > /dev/null` mints into the cache rather than wasting the mint; a pty
  walks straight through and **need not even be allocated** — redirecting into a terminal device
  the user already owns is enough; and `/dev/null` is a character device, so that redirect is
  permitted. The gate narrows the *accidental* case to a loud failure. An agent permission rule is
  what keeps this command out of unattended runs, and nothing here replaces it.

## Per-repo gotchas

- ⛔ **`sigil get` IS an on-demand, non-interactive, FULL-PERMISSION token printer — by design,
  and it dominates every bypass the `sigil token` TTY gate documents.** Three lines on stdin —
  `protocol=https`, `host=github.com`, `path=<owner>/<anything>` — and it writes
  `username=x-access-token` / `password=<full-permission installation token>` to stdout under a
  **pipe**. No pty, no prior git operation, a warm cache not required (a cold one mints), no subcommand, nothing the gate is
  near. ⚠ **`resolveInstallation` reads only the FIRST path segment**, so the repo half is
  arbitrary: `path=myorg/x` routes identically to `path=myorg/real-repo`. This is **required,
  not a defect** — git drives the credential helper over pipes on every invocation, so gating it
  would break every git operation on every wired repository. A stdout gate structurally cannot
  reach it. ⇒ **Do not read the `sigil token` TTY gate as controlling access to full-permission
  tokens**: the cheaper, fully-supported, unattended path to the same credential is the helper
  itself. Narrowing the grant the helper requests is a known follow-up (`exchange.go` POSTs
  `access_tokens` with a **nil body**, so git receives every installation permission including
  write when it needs repo read/write on one repo); the narrowing machinery already exists in
  `token_narrow.go`.
- ⛔ **The 1Password field TYPE is a landmine that has already detonated.** A *concealed* field
  strips PEM newlines. The key must be stored as multiline text or a file attachment.
  `config.example.toml` is authoritative on the shape. The failure surfaces as "no PEM block
  found" with nothing pointing at field type; `parseRSAPrivateKey`'s tests reproduce the
  symptom, but only a real 1Password round trip reproduces the cause.
- ⛔ **The empty `helper =` line in the gitconfig is deliberate** — `credential.helper` is a list
  accumulated across system → global → local, and an empty value resets it. Delete it as untidy
  and whatever was configured earlier answers first, typically the `gh` credential helper.
  `gh` holds a valid token, so it answers *successfully* every time and Sigil is never
  consulted. The failure is silent by construction: git operations keep working, just not
  through Sigil.
- ⛔ **`useHttpPath = true` is load-bearing.** Without it git sends no `path`, routing gets an
  empty owner, and every fetch fails with an error that never mentions gitconfig.
- ⚠ **`.gitignore` has two UNANCHORED entries: `config.toml` and `*.pem`.** They match at any
  depth. A fixture named `testdata/config.toml` would be **silently untracked** — the existing
  fixtures dodge this only by their names. A PEM fixture cannot be committed at all; generate keys
  in-process instead, as `jwt_test.go` and `parse_key_test.go` do.
- ⚠ **`run()`'s `default:` arm routes any unrecognised subcommand to `cmdGet`.** So `sigil --help`
  blocks reading stdin and then behaves as a credential helper. There is no help output.
- ⚠ **`$HOME` is read directly, rejecting `os.UserConfigDir()` / `os.UserCacheDir()` on purpose** —
  XDG variance makes them unpredictable under a test that sets only `HOME`. "Modernising" these
  silently relocates config and cache.
- ⚠ **Two swallowed errors are deliberate**, both with inline rationale: corrupt cache JSON is
  treated as a miss rather than a failure, and a cache-write failure is discarded because the token
  is already in hand. A linter-driven "fix" to either turns a cache problem into a credential
  failure.
- **28 MB binary, multi-second cold build.** The 1Password Go SDK is a WASM core, so the tree
  embeds a WASM runtime, the Extism plugin framework, and an OTLP/protobuf stack. Three direct
  dependencies, nine indirect.
- **Go 1.26.11** in `go.mod` — a `go` directive, so a **minimum, not a pin**, raised from 1.26.10
  to take GO-2026-6859 (net/http, fixed in 1.26.11); before that, from 1.26.4 to take the
  standard-library advisories fixed in 1.26.10. There is deliberately no `toolchain`
  line. **Measured 2026-11-09 with go1.26.10 darwin/arm64, in a throwaway module:** with
  `go 1.26.10` and `toolchain go1.26.10`, `go build` fails (`go: updates to go.mod needed; to
  update it: go mod tidy`) and `go mod tidy` removes the line; a `toolchain` line newer than the
  `go` line (`go 1.26.4` plus `toolchain go1.26.10`) is kept. So the Go tool drops a toolchain
  line equal to the `go` line, and a `toolchain` line therefore cannot be the way to pin
  1.26.10 here. Tests use only the standard `testing` package: no
  assertion library, no mocks, no golden files.

## What is still open

- ⚠ **Installation permissions: read them live, and say which installation you read.** The
  installation carries more permissions than the original design documented, so do not quote any
  written list of them without re-measuring.
- **Rolling the https-`origin` wiring out to other repositories.** There is deliberately no global
  config doing this: every repo needing unattended access is wired individually, per clone. ⚠ **No
  code change is involved**: `resolveInstallation` reads git's credential `path` (`owner/repo`),
  never a filesystem path, and no `.go` file carries a hardcoded path. Keying the map by
  `owner/repo` rather than owner *would* be a code change, and this shape does not need it.
- ⚠ **A new repo may also need adding to the GitHub App installation.** An org installation may
  be `repository_selection: all`; a personal-account installation may be select-repos — check
  each live.
- **Narrow the credential-helper grant** so `git` gets repo-scoped access instead of the
  installation's full permissions. **Deliberately deferred, not deprioritised**, and it is the
  git-critical path, so it needs a spec and a real acceptance run rather than a quick patch. The
  property it closes is the first per-repo gotcha above.
- **The two test gaps were addressed; one remains partly open.** `parseRSAPrivateKey` is covered
  (see above). The channel-separation invariant is covered for `writeCredentialResponse` and for
  `sigil get`'s cache-hit path through the built binary; a token printed to stderr from the
  cache-miss path or the other subcommands still passes `just check`, and the only end-to-end check
  of those (`TestCLI_LeakScan_TokenNotInStderr`) is still gated on 1Password. Closing the wider
  invariant needs either a key-source seam so the built binary can reach the mint path in-gate, or a
  mechanism that bounds what may write to `os.Stderr` — neither exists.
- **One GitHub App per agent** was decided against, and the ruling names its own reversal
  condition: a second execution host. Self-announcing; nothing to watch for.
- **Whether `.claude/` stays gitignored** — undecided. It may be un-ignored later, since agent
  memories are written there. Currently ignored, and that is deliberate rather than an oversight.
  **Do not "fix" the `.gitignore` entry**, and do not read the absence of tracked agent memory as
  a defect.

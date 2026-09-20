# sigil — project instructions

A git credential helper that mints short-lived GitHub App installation tokens, so unattended
agent runs reach GitHub without a human unlocking the 1Password desktop app. Built 2026-06-28
under spec `#1880` and ADR `G-0016`; reviewed, shipped, and in daily use since.

Brain project 35, repo 17. Single flat Go package, 16 files, ~3k lines. Two commits.

## Read this before treating anything here as settled

This repo was onboarded into the new per-project workflow on **2026-08-21** — brain
registration, this file, and the README all landed then. The code has not changed since
June. ⚠ **Several in-code comments describe a state that no longer exists** (see gotchas) —
where a comment and the code disagree, the code is right and the comment is a fossil.

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
| `token_narrow.go` | `sigil token --rate-limit-only`: the fixed-body (`{"permissions":{"metadata":"read"},"repositories":[repo]}`) access_tokens POST, `verifyNarrowedGrant`'s exact-match check against GitHub's actual response, and revoke-on-mismatch. Sibling to `token.go`/`exchange.go` — never imports `cache.go` (`#3647`) |
| `cache.go` | On-disk token cache, `0600`-enforced. Only `token.go`'s full-permission path reads/writes it |
| `justfile` | `build` · `test` · `vet` · `fmt` · `fmt-check` · `check`. ⚠ **No `install` recipe** |
| `config.example.toml` | The template. Non-secret fields only; its comments carry the secret-boundary rationale |
| `testdata/*.toml` | Three config fixtures: valid, malformed, and no-installations |

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
`go test ./...`). **Measured 2026-08-21: all green, exit 0.** ~8 s cold, ~0.3 s warm. 63 tests
pass, 2 skip. **Re-measured 2026-09-17 after `#3647` (`--rate-limit-only`) landed: still
green, exit 0, ~3.3 s cold. 77 top-level tests pass, 2 skip** — the 14 new top-level tests are
`token_narrow.go`'s coverage (including the Warden-review follow-up: the 401-retry-still-verifies,
empty-token, and revoke-survives-a-cancelled/expired-context tests) plus the CLI
argument-polarity and empty-body tests; skip count is unchanged (still gated on
`OP_SERVICE_ACCOUNT_TOKEN` + `SIGIL_TEST_OP_KEY_REF`, per the table
below).

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
| `TestCLI_LeakScan_TokenNotInStderr` | The **only** test that checks a token does not reach stderr |

Both are gated on real 1Password, not on network or GitHub — everything else mocks GitHub with
`httptest`. Green here means *"the units are fine and nothing leaked a PEM"*, never *"it works."*

⛔ **The false green — the one to know. Seeded and measured 2026-08-21.** `writeCredentialResponse`
claims in-comment that it "has no code path to `os.Stderr`", and the test that supposedly proves it
explicitly declines to check (*"No os.Stderr interception needed"*), asserting buffer contents only.
The one test that would catch a token on stderr is the skip-gated one above. ⇒ **`fmt.Fprintln(os.Stderr,
token)` was added inside `writeCredentialResponse` and `just check` returned exit 0, all tests `ok`.**
The gate does not see a token written to stderr from the one function whose entire stated purpose is
to make that impossible. The channel-separation invariant is carried by a function signature and a
comment — convention, not mechanism.

⚠ **`parseRSAPrivateKey` has zero test coverage** — measured by identifier count across all
`*_test.go`. It is security-critical, it is the function behind the one production bug this repo
has had (`#1892`, PEM newlines), and its PKCS#8 type assertion is exactly the branch that rots
unnoticed. It is directly unit-testable: takes `[]byte`, returns `(*rsa.PrivateKey, error)`, no
1Password needed, and `jwt_test.go` already generates keypairs in-process.

⛔ **`semgrep` and `govulncheck` are NOT in `just check` and nothing re-runs them.** Both were
run once, at the original Warden review (`#1880`), and CI was explicitly scoped out. One direct
dependency drags in a WASM runtime, a plugin framework and an OTLP/protobuf stack — that surface
accrues advisories with nothing watching.

## Remote and merge posture (current, not aspirational)

- **`prmanahan/sigil` on github.com** — private, default branch `main`. ⚠ **Which remote name
  reaches it is a per-clone question** — see *Remotes*, below; a clone is expected to carry two.
- ⛔ **Branch protection is not merely absent, it is UNAVAILABLE.** Measured 2026-08-21: the
  protection and ruleset APIs both return **403 — "Upgrade to GitHub Pro or make this repository
  public"**. `#2560` recorded sigil as "unprotected by decision"; more precisely, there is no
  protection to configure at this plan and visibility. Same disposition as `mentat`.
- ⛔ **There is no `.github/` directory. A PR here runs NOTHING** — no workflow, no check, no
  server-side gate. A PR is a review surface, not a gate. ⇒ **A local `just check` is the entire
  verification layer, and it is the layer the actor being constrained can skip.** Do not let a
  green PR imply anything was verified.
- **Land via PR** (Peter, 2026-08-21): squash to one delivery commit, push to `origin`, open the
  PR, rebase-and-merge, then fast-forward local `main` **from the remote** — never advance it
  locally. ⛔ **The `merge` skill cannot run here**: it treats more than one remote as a hard
  stop, and this project requires two (`#3410`, filed against `puck-plugin`). Until that lands,
  the sequence above is executed by hand — which means the guards the skill carries, chiefly the
  pinned rebase base and the branch-delete predicate, are yours to apply rather than automatic.
- **Squash to one delivery commit carrying the brain task number** before landing.
- ⚠ **`deleteBranchOnMerge` is `false`** — measured 2026-08-21. Branches do not clean themselves
  up; delete them explicitly. All three merge methods are enabled at the repo level, so nothing
  stops a merge commit but the skill.
- **Work in a worktree**, never the main checkout.

## Install and runtime wiring

Three pieces outside the repo, and the repo is useless without them.

1. **`~/.local/bin/sigil`** — the built binary. ⚠ **No `install` recipe exists.** `just build`
   writes `./sigil` into the repo (gitignored); the deployed copy was placed by hand. **Nothing
   detects divergence between source and deployed binary** — a green `just check` says nothing
   about what is actually on disk at that path.
2. **`~/.local/bin/sigil-helper`** — a three-line wrapper that reads the 1Password
   service-account token from the macOS Keychain, exports it, and `exec`s the binary. Its whole
   job is the env-var injection sigil deliberately refuses to do itself, keeping the Keychain
   dependency out of the Go binary.
3. **The repo's own `.git/config`** — registers the helper for `https://github.com` and sets
   `useHttpPath = true`, alongside the two remotes. ⛔ **Per-clone and never committed**, so it
   does not travel with the repository — see *Remotes*. ⚠ This replaced an earlier
   `~/.config/git/sigil-workspace.gitconfig` + `includeIf` arrangement scoped to the old
   workspace path, retired 2026-08-21. If you find that file or an `includeIf` referencing it,
   it is dead weight, not configuration to restore.

Config lives at `~/.config/sigil/config.toml` — `op_key_ref` (an `op://` **pointer** to the App
private key, never key material), `app_id`, and an `[installations]` table mapping repo owner to
installation ID. Copy `config.example.toml` and fill it. ⚠ The live file is `0644`. It holds no
secrets by design, but it does publish the App ID and installation IDs to any local user.

⛔ **Adding an `[installations]` entry may silently widen what agents can mint — and nothing
here will tell you.** The agent permission rules that keep the full-token command away from
agents are understood to deny it by matching the owner name literally, which couples them to this
table holding **exactly** `prmanahan` and `mnemra` (the owners are measured, 2026-09-20; the
coupling is **INHERITED AND UNVERIFIED** — see below). If that is right, adding a third owner
means `sigil-helper token <newowner>` matches no deny rule: an agent can mint a
**full-permission, write-capable** token for that owner, and the first sign of it is the token,
not an error. The narrow `--rate-limit-only` form is unaffected either way — it verifies its own
grant and fails closed.

⚠ **The coupling above has NOT been confirmed from this root, and the rules were not found where
they were looked for.** Searched 2026-09-20: `~/.claude/settings.json`,
`~/.claude/settings.local.json`, and both `.claude/settings*.json` under this repo — **no rule
mentioning sigil or the narrow command exists on any of them.** The rules are believed to live in
mnemra-core's own settings, which is a different repository and deliberately not read from here.
⇒ Treat this as a hazard to check before adding an owner, not as a described mechanism, and
re-derive it from mnemra-core rather than from this paragraph.

⚠ **This is PROSE, not mechanism, and that is the durable point.** Nothing in this repo, in
sigil, or in mnemra-core checks the coupling — an allowlist of tool names cannot express a
constraint that lives in a command's *argument*. Whoever adds an owner here updates those rules
in the same change, or the widening ships unremarked. ⇒ If a mechanism is ever built, it belongs
on the rule side (deny by default, allow-list the narrow form), not here.

## Remotes — two of them, and which one you use matters

⛔ **This section states a REQUIREMENT, not the state of your clone.** Remotes and credential
config live in `.git/config`, which is never committed — so none of it arrives with a `git
clone` and none of it is true on a new machine until someone wires it. **Check, do not assume.**

**The convention.** This project uses two remotes, and the remote name *is* the identity choice,
because git picks its transport from the remote's URL:

```
origin      https://github.com/<owner>/sigil.git   → agent + unattended work, via Sigil → the APP
ssh_origin  git@github.com:<owner>/sigil.git       → Peter's own admin/fixup, over SSH → HIM
```

**Confirm before pushing:**

```bash
git remote -v          # origin must be https:// — if it is git@, this clone is UNWIRED
```

⛔ **A clone with a single `origin` on `git@` is unwired. Do not push agent work from it.** Wire
it first — the README's setup section carries the credential-helper block and the remote
commands. Pushing from an unwired clone silently authenticates as **Peter**, because SSH never
consults git's credential system at all, so Sigil is not merely bypassed — it is never asked.

⛔ **Agent work pushes to `origin`. Never to `ssh_origin`.** Not hypothetical: on 2026-08-21,
before the split existed, a docs branch of agent work went out over SSH and GitHub recorded it
as Peter. The constraint in his words: *"i need to be able via ssh and attribute the change to
me when i do things, normally its an admin only or fixup i do, so more rare."*

⚠ **A remote rename drags branch upstreams with it**, so a bare `git push` can change identity
without changing any command you typed. Observed here the same day. **Prefer naming the remote
explicitly over relying on an upstream**, and check `git config --get branch.<name>.remote`
after any remote surgery.

⚠ **A successful push proves authentication, not *which* helper supplied it** — `gh` holds a
valid token too. To confirm Sigil served a request rather than something else, check that
`~/.cache/sigil/<installation-id>.json` was written at that moment. A bare success does not
establish it.

⛔ **No `url.insteadOf` rewrite — it defeats the whole arrangement.** `insteadOf` applies when
git resolves *any* URL, remote names included, so a `git@`→https rewrite converts `ssh_origin`
too. **Measured 2026-08-21** in a throwaway repo: with the rewrite set,
`git ls-remote --get-url ssh_origin` returns the **HTTPS** URL. The rewrite existed only to force
a single SSH remote onto HTTPS; choosing transport by remote name replaces it. ⚠ This is the
shape that looks like the obvious fix, which is why the ban is recorded rather than assumed.

⛔ **`/merge` cannot land this repo — it treats more than one remote as a hard stop** (`#3410`,
filed against `puck-plugin`). Until that is fixed, landing here is done by hand: squash to one
delivery commit, push to `origin`, open the PR, rebase-and-merge, then fast-forward local `main`
from the remote. Peter: *"merge skill is fixable we own it, so not critical."*

## Security invariants — do not regress these

Stated at `main.go:18-31`. Each names what breaking it costs.

- **The App private key never touches disk.** Resolved in-process from 1Password over the API,
  parsed from memory. ⇒ Breaking it puts the root credential that mints every token on disk;
  recovery is key rotation plus re-install.
- **`OP_SERVICE_ACCOUNT_TOKEN` is env-only.** Never written, never logged, no field for it on
  `Config`. The wrapper injects it; sigil never reads the Keychain. This is secret zero.
- **The token leaves by exactly one channel.** ⚠ Signature and convention only — see the false
  green above. **This is the invariant most likely to break silently.**
- **Cache files and the cache dir are `0600`/`0700`, re-chmod'd explicitly** after creation,
  because POSIX ignores the mode argument when the file already exists. ⇒ Deleting the "redundant"
  chmod leaves a pre-existing `0644` file at `0644`, holding live GitHub tokens. **This is the
  best-mechanised invariant in the repo** — defensive code, correctly tested, reasoning inline.
- **The cache holds a token and an expiry, nothing else.**
- **Routing is fail-closed and runs before any network call.** Unknown owner errors; no default
  installation is ever guessed. ⇒ Breaking it mints a token for the wrong installation.
- **Scheme validation runs before the key read**, so a misconfigured API base is caught with no
  secret in memory. The *ordering* is the property, and no test asserts the ordering directly.
- **Error messages quote types, status codes and lengths — never bytes.**
- **Cached expiry is GitHub's `expires_at` minus five minutes**, so a cache hit can never hand
  back a token within five minutes of death.
- **A `--rate-limit-only` token is verified against GitHub's actual grant before it is trusted
  or printed, and is never cached.** `token_narrow.go`'s `verifyNarrowedGrant` checks
  `permissions`, `repository_selection` and `repositories` from the response — four conditions,
  all ANDed, exact match — not from what was requested. Any mismatch revokes the token
  (`DELETE /installation/token`) and returns an error instead. ⇒ Breaking the verification lets
  a silently-widened grant reach stdout. **Measured by Peter, live run of
  `live_narrow_probe_test.go`, 2026-09-17, mnemra installation `143101545`** (not independently
  re-verified by Forge — Keychain access was denied to the implementing session, see `#3647`):
  an access_tokens request body of `{"permissions":{}}` was granted **all seven** installation
  permissions including write, not none; a request body of
  `{"permissions":{"metadata":"read"},"repositories":[]}` was granted `repository_selection:
  "all"`, not zero repositories. Both are exactly the class this verification exists to catch —
  do not weaken `verifyNarrowedGrant` to a single `permissions["metadata"]=="read"` check, which
  passes the first of these two silently.

## Per-repo gotchas

- ⛔ **`hardening_compilefail_test.go` is a TDD fossil, and its header is a lie.** It says the
  package "will not compile until Forge implements them" — all three functions exist, and its four
  tests are ordinary tests that pass under a plain `go test ./...`. There is no build tag anywhere
  in the repo. **The risk is inverted from the obvious one:** nothing is silently skipped; instead
  a reader believes the file needs special invocation and either avoids it or "fixes" the stale
  comment by deleting live tests.
- ⚠ **Other in-code comments are stale the same way.** `hardening_test.go` describes a
  `context.Background()` gap that is closed, citing line numbers that no longer resolve. Several
  tests carry `RED:` / `GREEN` prefixes and bare Warden finding IDs (`M1`, `L1`–`L4`, `N2`, `N3`)
  that are meaningless without the review record.
- ⛔ **The 1Password field TYPE is a landmine that has already detonated.** A *concealed* field
  strips PEM newlines — that was `#1892`. The key must be stored as multiline text or a file
  attachment. Three documents describe three different shapes; `config.example.toml` is
  authoritative. The failure surfaces as "no PEM block found" with nothing pointing at field type,
  **and the function that reports it has no test.**
- ⛔ **The empty `helper =` line in the gitconfig is deliberate** — `credential.helper` is a list
  accumulated across system → global → local, and an empty value resets it. Delete it as untidy
  and whatever was configured earlier answers first. **Measured 2026-08-21: that is
  `!/opt/homebrew/bin/gh auth git-credential`, from `~/.gitconfig`** — not `osxkeychain`, which
  is what the original design note assumed. `gh` holds a valid token, so it answers
  *successfully* every time and Sigil is never consulted. The failure is silent by construction:
  git operations keep working, just not through Sigil.
- ⛔ **`useHttpPath = true` is load-bearing.** Without it git sends no `path`, routing gets an
  empty owner, and every fetch fails with an error that never mentions gitconfig.
- ⚠ **`.gitignore` has two UNANCHORED entries: `config.toml` and `*.pem`.** They match at any
  depth. A fixture named `testdata/config.toml` would be **silently untracked** — the existing
  fixtures dodge this only by their names. A PEM fixture cannot be committed at all; generate keys
  in-process instead, as `jwt_test.go` already does.
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
- **Go 1.26.4** in `go.mod` — a bare `go` directive, so a **minimum, not a pin**. No `toolchain`
  line. Tests use only the standard `testing` package: no assertion library, no mocks, no golden
  files.

## What is still open

- ⛔ **`G-0016`'s permission list is STALE — do not quote it.** The ADR documents three
  permissions (`contents:write`, `pull_requests:write`, `metadata:read`). **Measured live
  2026-08-21**, the org installation carries **seven**: `actions:write`, `checks:write`,
  `contents:write`, `issues:write`, `metadata:read`, `pull_requests:write`, `workflows:write`,
  at `repository_selection: all`. ⚠ **That reading covers the ORG installation only** — the
  personal-account installation returns 403 without App-authorized auth and is unverified.
  `#2820` — append the drafted amendment plus a `checks:write` row — has been open since
  2026-07-25 with **no tripwire**; it is parked, not deferred. ⇒ **When writing about
  permissions, read them live and say which installation you read.** Do not repeat the ADR's
  list, and do not repeat the numbers above without re-measuring.
- **Rolling the two-remote shape out to other repositories** — `#3409`. There is deliberately no
  global config doing this any more: the `includeIf` that once served `~/claude_workspace` was
  removed 2026-08-21 and nothing replaced it. Every repo needing unattended access is wired
  individually, per clone. ⚠ **No code change is involved**: `resolveInstallation` reads git's
  credential `path` (`owner/repo`), never a filesystem path, and no `.go` file carries a
  hardcoded path — measured 2026-08-21. Keying the map by `owner/repo` rather than owner *would*
  be a code change, and this shape does not need it.
- ⚠ **A new repo may also need adding to the GitHub App installation.** The org installation
  measured `repository_selection: all` on 2026-08-21; `#1880` recorded personal-account installs
  as select-repos, and that one returns 403 without App-authorized auth — unverified.
- **`#2651`, 2026-10-21** — review whether the `actions` permission can narrow to `read`. A date
  rather than a usage trigger, deliberately: non-use emits no signal, so a usage-based trigger
  would rot silently.
- **The two test gaps** — the channel-separation invariant and `parseRSAPrivateKey` — are the two
  things worth closing before this repo is ever made public. Both are cheap and neither needs
  1Password.
- **N-Apps** (one GitHub App per agent) was decided against, and the ruling names its own reversal
  condition: a second execution host. Self-announcing; nothing to watch for.
- **Whether `.claude/` stays gitignored** — undecided. Peter, 2026-08-21: *"long term we may un
  gitignore .claude since that is where the agent memories except pucks are written. its ok for
  now though."* ⇒ Currently ignored, and that is deliberate rather than an oversight. **Do not
  "fix" the `.gitignore` entry**, and do not read the absence of tracked agent memory as a defect.

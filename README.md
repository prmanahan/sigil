# Sigil

A [git credential helper](https://git-scm.com/docs/gitcredentials) that mints short-lived GitHub
App installation tokens on demand, so automated and unattended git operations can authenticate to
GitHub without a human present.

Sigil holds no credential of its own. It resolves a GitHub App private key from 1Password in
process, signs a JWT, exchanges it for a one-hour installation token, caches that token, and hands
it to git in the format git expects. Nothing durable is written except the cached token, and that
expires.

## The problem

Unattended runs — overnight jobs, long automated sequences, a push→PR→merge loop — authenticated
to GitHub over SSH, using a key served by the **1Password SSH agent**.

That agent suspends when the machine sleeps, and expires on session timeout. Which is precisely
when an unattended run needs it. The symptom is `Permission denied (publickey)`, and it was the
single most common cause of a halted run: **27 of the last 30 days** at the time the problem was
written up. Keeping the machine awake and pausing-to-prompt are both workarounds, not fixes.

The requirement was a credential that is **non-interactive, survives sleep, can open and merge pull
requests, works across more than one repository, and does not sit on disk as a long-lived secret.**

## How it works

```
git ──► sigil-helper ──► sigil
             │              │
             │              ├─1─ parse git's request, ignore anything not https://github.com
             │              ├─2─ map the repo owner ──► installation ID   (fail closed)
             │              ├─3─ cache hit? ──► return the token, no network
             │              ├─4─ read the App private key from 1Password  (in process)
             │              ├─5─ sign a 10-minute RS256 App JWT
             │              ├─6─ exchange it for a 1-hour installation token
             │              └─7─ cache it, minus a 5-minute safety margin
             │
             └─ injects the 1Password service-account token from the macOS Keychain
```

**1. Git asks.** Git invokes the helper with the credential request on stdin — protocol, host, and
(because `useHttpPath` is set) the repository path. Sigil parses until the first blank line.

**2. Scope gate.** Anything that is not `https` to exactly `github.com` is a silent no-op, exit 0,
no output — git simply moves on to the next helper. Sigil never answers for a host it was not
built for.

**3. Routing.** The repo owner is the substring before the first `/` of the path, looked up in a
map of owner → installation ID from the config file. **An unknown owner is an error, not a
fallback** — there is no default installation, and this check runs before any network call or any
secret read.

**4. Cache.** A token cached under that installation ID, still inside its validity window, is
returned immediately with no mint and no network. A missing or corrupt cache file is treated as a
miss rather than a failure.

**5. Key read.** The App private key is resolved from 1Password by reference, **in process, over
the API** — no `op` CLI on `PATH`, no desktop app, no biometric prompt, and the key never touches
disk. The service-account token that authorizes this read comes from the environment only; the
wrapper puts it there.

**6. Mint and exchange.** An RS256 JWT is signed with the App key — issued 60 seconds in the past
for clock skew, expiring in 10 minutes, GitHub's documented ceiling — and exchanged at
`POST /app/installations/{id}/access_tokens` for an installation token valid for one hour. A `401`
triggers exactly one retry with a freshly minted JWT, which absorbs the case where the JWT aged out
in flight.

**7. Return.** The token is cached with its expiry reduced by five minutes, so a cache hit can
never hand back a token about to die mid-operation, and written to stdout as the two lines git
expects:

```
username=x-access-token
password=<token>
```

The whole path is bounded by a 30-second deadline, at both the context and the HTTP client layer.

## Why this design

The problem splits into two questions that were decided independently: **what GitHub trusts**, and
**how the secret reaches a headless machine**.

### What GitHub trusts — a GitHub App

One App, installed on each target account, minting short-lived installation tokens. It was the only
option satisfying every requirement at once: non-interactive and sleep-safe, capable of pull
requests and merges, multi-repository, free on the existing plan, and — the part that matters most
— **two-tier**. A protected long-lived root credential that never leaves 1Password, and an
ephemeral operational token that is useless within the hour.

The App is **public** rather than private. This is not an exposure decision: a *private* personal
App can only be installed on the account that owns it, and installing across a personal account and
a separate organization requires a public one. Public governs who may discover and install it, not
who can read the key.

Sigil exists as code rather than configuration because GitHub's own token-minting helper is
Actions-only and does not apply to a local runner.

**What was rejected, and why:**

| Option | Why it lost |
|---|---|
| **Fine-grained personal access token** | A standing operational secret, usable for its full remaining life if leaked. Also acts as the account owner, which muddies attribution. |
| **Machine user + fine-grained PAT** | Buys clean attribution at the cost of a second GitHub account to secure — another login, another 2FA, another phishable surface — and the token is still standing. GitHub's own guidance prefers Apps. |
| **Per-repo deploy keys** | The tightest scope of any option, and disqualified anyway: deploy keys are git-only. No API, no pull requests, no merges. Plus plaintext-on-disk by default and N keys to manage. |
| **Status quo — personal SSH key in the 1Password agent** | Account-wide scope, and the agent suspends on sleep. This *is* the failure being fixed. |

Classic PATs and plain `gh auth` are **not** recorded as having been evaluated. They are absent
from the decision record rather than rejected by it.

### How the secret arrives — a 1Password service account

A read-only service account, scoped to a single dedicated vault holding only the App key. Its own
bootstrap token — "secret zero" — lives in the macOS Keychain, ACL-bound so only the intended
binary can read it silently, on an encrypted disk.

**What was rejected, and why:**

| Option | Why it lost |
|---|---|
| **Keychain / `chmod 600` file / age / sops, as the primary store** | No central audit or revocation. `age` and `sops` shift the bootstrap problem rather than solving it. Kept as a fallback — and note the Keychain was not rejected outright, it was demoted to holding secret zero, which is exactly the job it does today. |
| **A dedicated non-admin OS user to run the helper** | On a single-user machine the Keychain item's binary-bound ACL already provides access control at the same granularity. Redundant. |
| **`launchctl setenv` for the bootstrap token** | Process-wide readable. Never. |
| **A second 1Password account per organization** | An organization is a GitHub-side install target, not a secrets-side boundary. Only warranted if a separate account later holds unrelated secrets. |

### One App, not one App per agent

Where several automated agents share this credential, the obvious instinct is an App per agent, for
least authority and clean attribution. That was investigated and **rejected**, for a reason that
inverts the premise:

The installation-token endpoint accepts `repositories` and `permissions` parameters. **A token can
be scoped down at mint time, from the same App and the same private key.** Least authority is
achievable at the token layer, so the security property that appeared to require N Apps does not
require them.

The unforgeability argument fails too, for this deployment: one local machine holds every key, so
an attacker who reaches the machine reaches all N. N Apps would mean N keys, N installations, N
approval round-trips and N rotations — all defending against an adversary the architecture does not
stop anyway.

What N Apps genuinely buys is kept on the record: a distinct GitHub-recorded actor per agent, and
independent revocation — deleting one App is cleaner than declining to mint for one role, though
one-hour tokens plus a local mint step narrow the gap. A middle option of two or three Apps split
by privilege class was considered and rejected as paying the multi-key tax for a coarsened version
of the one benefit.

**The decision names its own reversal condition:** if the runner stops being a single trust domain
— agents on separate hosts, or an agent somewhere outside the operator's control — the shared-key
objection disappears and per-agent Apps become correct. That condition cannot arrive unnoticed;
someone has to provision the second host.

### Why Go

The surrounding workspace defaults to Rust. Sigil is a deliberate, scoped exception.

**There is no official 1Password Rust SDK.** 1Password ships official SDKs for Go, JS/TS, Python
and .NET. The only Rust crate is unofficial, unmaintained and unsuitable for a credential helper —
so the Rust path would mean shelling out to the `op` CLI as a subprocess and parsing its output.
The official Go SDK reads the key **in process**, which removes the `op`-on-`PATH` dependency
entirely. JWT signing, the token exchange, and the credential-helper protocol are all
straightforward in Go.

One honest footnote: the driver was a cleaner dependency story, and on *operational* dependencies
it delivered — no external binary, no CLI output parsing. On *supply chain* it went the other way.
The 1Password Go SDK is a WASM core, so the tree pulls in a WebAssembly runtime, a plugin
framework, and an OpenTelemetry/protobuf stack. Three direct dependencies, nine indirect, and a
28 MB binary.

## Setup

1. **Register a GitHub App** on a personal account. Webhook off. Set visibility to *Any account* if
   it must install anywhere other than the owning account. Note the App ID — it is not a secret.
2. **Set its repository permissions** to what your automation actually needs, and no more.
3. **Generate a private key** and download the `.pem`.
4. **Install the App** on each target account or organization. Note each installation ID from the
   URL you land on after installing.
5. **Store the `.pem` in 1Password**, in a dedicated vault. ⛔ **Store it as multiline text or as a
   file attachment — never as a concealed field.** Concealed fields strip the PEM's newlines and
   the key will not parse. Delete the local `.pem` afterwards.
6. **Create a read-only service account** scoped to that vault. Vault access is immutable after
   creation — get it right or recreate it.
7. **Store the service-account token in the macOS Keychain**, ACL-bound to the binary that reads
   it, with the partition list set for silent reads. Never `launchctl setenv` it.
8. **Build and install:** `just build`, then copy `sigil` onto your `PATH`.
9. **Install the wrapper** alongside it — a short shell script that reads the service-account token
   from the Keychain, exports it as `OP_SERVICE_ACCOUNT_TOKEN`, and `exec`s `sigil`.
10. **Write the config** — copy `config.example.toml` to `~/.config/sigil/config.toml` and fill in
    the key reference, App ID, and owner → installation ID map.
11. **Register the credential helper** in your git config:

    ```gitconfig
    [credential "https://github.com"]
        # credential.helper is a LIST, not a single value. Git accumulates helpers
        # across system -> global -> local config and asks each in turn until one
        # returns a credential — first usable answer wins.
        #
        # An empty value RESETS that accumulated list. It is not a typo and not
        # decoration: without it, any helper configured earlier (a system keychain,
        # or `gh auth git-credential` from a global config) stays ahead of Sigil in
        # the queue. Those helpers hold valid tokens, so they answer successfully,
        # every time — and Sigil ends up installed but never consulted.
        helper =

        # The leading "!" marks this as a shell command, so PATH resolution
        # applies. Without it git would look for an executable named
        # `git-credential-sigil-helper` — its `git-<name>` convention — and
        # silently find nothing. An absolute path works too, and needs no "!".
        helper = !sigil-helper

        # Makes git send the repository path with the credential request. Sigil
        # routes on the repo OWNER, which it takes from that path — without this,
        # the path arrives empty and every request fails with "repoPath is empty",
        # an error that says nothing about git config.
        useHttpPath = true
    ```

    This only ever fires for HTTPS requests to `github.com`. SSH remotes never touch git's
    credential system at all, so registering this globally does not disturb them.

12. **Give each repository two remotes** — this is what lets automated and human access coexist:

    ```bash
    git remote rename origin ssh_origin                              # your own work, over SSH
    git remote add origin https://github.com/<owner>/<repo>.git      # unattended, via Sigil
    ```

    Git picks its transport per remote, from that remote's URL. `origin` on HTTPS reaches
    Sigil; `ssh_origin` on `git@` uses your own SSH key. Automation gets the right path from
    the default remote and cannot silently fall back; you reach for `ssh_origin` explicitly on
    the rare occasions you want a push recorded as **you** rather than as the App.

    Both remotes live in the repository's own `.git/config`, which is never committed and never
    pushed — the correct home for machine-specific credential wiring.

    ⛔ **Do not add a `url.<base>.insteadOf` rewrite alongside this.** It looks like the obvious
    way to force SSH remotes onto HTTPS, and it silently destroys the arrangement above:
    `insteadOf` is applied whenever git resolves *any* URL, remote names included. A rewrite of
    `git@github.com:` → `https://github.com/` turns `ssh_origin` into an HTTPS remote too, so
    `git push ssh_origin` goes through Sigil and is attributed to the App. Verified directly —
    with such a rewrite in place, `git ls-remote --get-url ssh_origin` reports the HTTPS URL.
    The rewrite only made sense when there was a single SSH remote that had to be forced onto
    HTTPS. Choosing transport by remote name replaces it.

13. **Verify:** `sigil doctor`.

## Configuration

`~/.config/sigil/config.toml`. **Non-secret values only** — see `config.example.toml`.

| Field | Meaning |
|---|---|
| `op_key_ref` | An `op://` **reference** to the App private key. A pointer, never key material. Prefer a stable item and file name so it survives key rotation. |
| `app_id` | The GitHub App ID, used as the JWT issuer. Not secret — it appears in the App's own URL. |
| `[installations]` | Repository owner → installation ID. One entry per account or organization the App is installed on. |

The 1Password service-account token is **not** in this file and has no field. It comes from the
environment, placed there by the wrapper.

## Commands

| Command | Does |
|---|---|
| `sigil` / `sigil get` | The credential-helper path. Reads a request on stdin, writes credentials to stdout. |
| `sigil token <owner>` | Prints an installation token for that owner. Useful for scripting and debugging. |
| `sigil doctor` | Checks config, the environment token, the key read, and a JWT mint — **without contacting GitHub**. Prints only counts and sizes, never key or token material. The cheapest smoke test. |
| `sigil store` / `sigil erase` | No-ops. Git expects them to exist; Sigil has nothing to store or erase. |

⚠ Any unrecognized subcommand — including `--help` — falls through to the credential-helper path
and will block reading stdin. There is no help output.

## Development

```
just check     # vet + gofmt check + tests — the gate
just test      # tests only
just build     # writes ./sigil
```

Tests use only the standard library — no assertion library, no mocking framework. GitHub is mocked
with `httptest`; RSA keypairs are generated in process. **Two tests skip unless both
`OP_SERVICE_ACCOUNT_TOKEN` and `SIGIL_TEST_OP_KEY_REF` are set**, and they are the end-to-end path
and the stderr leak scan — so a green run on a machine without 1Password credentials has not
exercised the happy path.

## Security properties

- The App private key is **never written to disk**. It is resolved in process and held in memory
  for the duration of one mint.
- The 1Password service-account token is **environment-only** — never written, never logged, and
  absent from the config schema entirely.
- **Tokens are short-lived**: 10 minutes for the App JWT, one hour for the installation token,
  and the cache retires a token five minutes early.
- **Cache files are `0600` and the cache directory `0700`**, re-applied explicitly after creation
  because the mode argument is ignored when the file already exists.
- **Routing fails closed.** An unknown owner is an error; no default installation is ever guessed.
- **The API base URL is validated before the key is read**, so a misconfigured endpoint is caught
  with no secret in memory. Plain HTTP is accepted only for loopback.
- **Error messages carry types, status codes and lengths — never bytes.** PEM parse failures name
  the block type; exchange failures name the status code and installation ID, never the response
  body.

---

Design record: ADR `G-0016`, implementation spec `#1880`.

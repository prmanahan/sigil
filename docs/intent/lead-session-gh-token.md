---
spec_type: code
frame_relevant: true
---

# Intake: Narrowed GitHub token for lead sessions' `gh` work

**Date:** 2026-11-09
**Status:** locked
**Consumer:** agents. Lead agent sessions on the maintainer's Mac.
**Kind:** other: a CLI subcommand on one Mac, plus the wiring that makes a session's `gh` use its output.

## Terms

- **Intake, Frame, spec:** the three design stages of this work, requirements, then architecture, then testable behaviour. The maintainer confirms each at its exit gate. At the Frame he signs the actor set and each accepted risk into a risk register.
- **Lead session:** the top-level agent session on the Mac. In the Read-back, the JTBD and the success criteria it means one the wiring was applied to; a session started without the wiring is the known gap in `## Security implications`. An agent it dispatches is expected to run in the same environment and get the same credential (inherited, not measured; the Frame confirms it). Giving a dispatched agent a different one is out of scope (Non-goals 3 and 6).
- **Landing work:** the closed set of GitHub API steps that take a pushed branch onto the default branch of the session's repository, each through `gh` or the GitHub API: open the pull request, read its checks, merge it, delete its branch. The branch push before them is git over https through `sigil get`, and is outside this narrowing (Non-goal 4). Any other `gh` use in a wired session runs on the same token and succeeds only as far as that token allows; where it does not, the failure is visible and a need that recurs is a later addition (Non-goal 3).
- **Target repository:** a repository, public or private, covered by the GitHub App's installation, whose clone has an https `origin`. A repository's own branch protection or ruleset may refuse a landing; that refusal is reported, not worked around.
- **Session's repository:** the target repository whose clone the lead session was started in, read from that clone's https `origin`. A worktree of that clone has the same `origin`, and an agent the session dispatches inherits it (inferred; the Frame's live check `LT-10` checks it). It is read again on each call from where the session was started, not from where the call runs. So changing directory, or naming another repository in a command, does not change it; it changes only if the `origin` of the session's own clone is changed. A lead session's routine `gh` work runs on a token for its session's repository only, by the maintainer's ruling of 2026-11-11 (Read-back).
- **Wiring:** the one arrangement, applied when a lead session is launched on the Mac, that gives the session's `gh` the narrowed token. It does not change `gh` at the maintainer's own terminal.
- **Expected grant:** the exact permission set the design fixes for landing work, selected-repository scope, and the single repository requested. It may include a read-only permission GitHub is shown by trial to add to the request, quoted from GitHub's response at the Frame. No write permission enters it that way.

## Read-back

The lead proposed each answer below. The maintainer's own words are quoted where he gave them. He confirmed the intake at its exit gate, verbatim: "its fine, we have a layered approach, repository setup, minting, etc my login here, my github login will be off once this works in gh so together its should be pretty secure." And on what closes the rest: "until i sandbox agents then that will remove the final step"

1. Job (proposed): a lead session's `gh` work uses a token limited to that repository and job.
2. Earliest real run (proposed): one lead session opens and lands a real pull request on one repository with the narrowed token, the maintainer's `gh` login still in place on the Mac but unused by that session.
3. Attended or not (proposed): unattended, no prompts.
4. Must stay (proposed): `sigil get`, `sigil token --rate-limit-only`, the terminal check on `sigil token <owner>`, the App key never on disk, cache file modes.
5. Who could attack it (proposed, unsigned): see `## Security implications (draft scope)`.
6. Outside systems (proposed): the GitHub API and the 1Password SDK, both already used, and the `gh` CLI. No new dependency expected. 1Password ships an official Go SDK, already in use; GitHub is called over its REST API, as today.
7. Promise words (proposed, as corrected by review): "narrowed" means GitHub's own response shows only the requested permissions and one repository, otherwise the token is revoked and nothing is printed. That is the token's direct reach; a workflow it changes can reach further (`## Risk profile`). "Short-lived" means GitHub's stated one-hour expiry, which sigil does not control. "Off your personal token" means `gh` in a lead session acts by default as the App's bot identity, not the maintainer's account. It does not mean the maintainer's login is out of reach.

Scope questions as asked, with the maintainer's answers verbatim:

- A: which tokens ship first, the lead's landing tokens only, or every job type now?
- B: is revoking the maintainer's `gh` login on the Mac part of done?
- C: `gh`-driven git uses the maintainer's SSH key; switch sessions to https?
- Answer: "A. yes, B. No C. We can set the remotes to be only https anyway so how would ssh work?"
- Follow-up on A and C, after the lead answered his question: "lead's tokens only, and set gh to https, i don't think it matters tbh but ok"
- Whether landings should need an approval, asked with the shape existing App, narrowed at mint, no approval rule: "yes, unless you can tell me how to handle approvals that don't require me?" and "this is part of the shifting left, i've already done my approvals on the input, it would be a rare case needing an approval later would it not?" The lead's reading, not his words: no approval step on landing (Non-goal 7).
- Whether the landing token is pinned to the session's own repository, asked 2026-11-11. He first restated the goal: "the problem was allowing unattended runs to allow merges and PR creation to gitub from the repo they are running in without me. Doing that in a secure way." Then: "ie with least privledge where possible." The lead's question, in the lead's words: "Should the landing token be pinned to the session's own repo (its clone's origin)? Pinning means: amend the design doc and its intake's not-to-build list, then one more review round before the gate." He chose: "Pin to own repo (Recommended)". This defines the session's repository (Terms) and amends Non-goal 8 and the not-to-build list.

## JTBD

When a lead session does landing work, I need that work by default to run on a credential good only for the session's repository and that job, with nobody present, so that what routine `gh` use carries, and what a leaked or logged copy exposes, is direct access to that one repository for one hour rather than a broad personal token.

This does not bound a session that deliberately reaches wider. It can still use the maintainer's login, his SSH key, `sigil get`, or, by calling sigil directly, a narrowed token for another repository (Non-goals 1, 2, 4 and 8).

## Non-goals

1. Revoking the maintainer's `gh` login on the Mac. He intends to turn it off once this works. That is a separate later step, and it must re-decide the risk in `## Risk profile`.
2. Removing the maintainer's SSH key from reach. Anything running as the user can use it.
3. Tokens for any job other than the lead's landing work, such as a read-only agent or an operations agent's read-and-rerun. Those are later additions.
4. Narrowing the grant `sigil get` returns to git. The branch push before landing work therefore still uses that full grant until separate work narrows it.
5. New GitHub Apps, or changes to the existing App's configured permissions.
6. Identifying which agent is calling, or an agent-name input to any command.
7. A review or approval requirement on landing. Landings stay unattended.
8. Stopping a process running as the same user from asking for a wider credential, or, by calling sigil directly, for a narrowed token for any repository the installation covers. The routine `gh` path asks only for the session's repository (Terms). Narrowing limits what one token reaches, and the pin limits what the routine path asks for; neither limits who may ask. Until 2026-11-11 the routine path itself could ask for any covered repository; the maintainer's pinning ruling (Read-back) replaced that.
9. A key source other than 1Password, or GitHub Enterprise Server support.

## Success criteria

Each is marked **gate** (runs in `just check`, against a mocked GitHub) or **live** (runs against real GitHub and 1Password, and is part of done).

S1 (live). A lead session with no human present and a non-terminal stdout obtains a token for one named target repository, shown on a public repository. GitHub's own response for that token matches the expected grant exactly. By the maintainer's ruling of 2026-11-11 no private repository is run live: the token's behaviour on one is left unproven, and S2 covers a private response against a mocked GitHub only.

S2 (gate). A response matching the expected grant, for a public or a private repository, emits the token on stdout only and exits zero. If GitHub's response differs from the expected grant in any respect, no token is emitted, the token is revoked, and the command exits non-zero with an error containing no token bytes. If the revoke itself fails, no token is emitted, the command exits non-zero, and the error says the revoke failed.

S3 (live). A token issued for target repository A is refused by GitHub for write access to repository B, where B is also covered by the same installation, shown by a pre-check recorded before the run. B is an existing repository, not one created for the run. Each attempted write uses a ref name unique to the run; one GitHub does not refuse is deleted at once, and S3 fails.

S4 (live). One lead session performs all landing work on its session's repository, a target repository whose protection does not refuse the landing, using only the narrowed token, with no prompt, while the maintainer's login stays in place on the Mac. The run keeps its own record of the credential each `gh` call used, because a read leaves no trace at GitHub, and that record shows no call went out on his login. GitHub's record of the pull request and the merge shows the App's bot identity. `gh` at the maintainer's own terminal still acts as him.

S5 (live). A lead session lands a pull request that changes a file under `.github/workflows/`, on its session's repository, the one throwaway target repository the Frame names for the first live run, which holds workflow files and whose protection does not refuse the landing. Precondition: the installation holds workflow write, read live before the run. If it does not, S5 and the workflow clause of the landing constraint are re-opened with the maintainer rather than worked around.

S6 (live, by seeded expiry). When a lead session's token has expired, the wiring obtains a fresh one with no human and landing work continues.

S7 (live, by seeded states). In a lead session the wiring was applied to, where there is no currently valid narrowed token for the session's repository (token absent, empty or invalid; expired with no refresh obtained; the mint failed; the session's repository cannot be read), `gh` work stops with a visible error and does not continue on the maintainer's login. A call naming another repository, or made from another repository's clone, runs on the session's repository's token: no call goes out on his login, and its write steps on the other repository are refused. The states are run with his real login in place on the Mac, and a fallback to it is the failing case. The check is one command. Parked, human-noticed: it is re-run whenever the sigil binary or the wiring is replaced, and at least monthly, the monthly run in a lead session started in this repository's own clone. This guards the routine path only; see the JTBD.

S8 (gate). After the change: `sigil get` behaves as before and returns no narrowed token; a narrowed token is never served in place of another grant; `sigil token --rate-limit-only` behaviour and its tests are unchanged, including its refusal of a private repository; `sigil token <owner>` still returns the full grant and still refuses a non-terminal stdout; the App private key never touches disk; cache files stay mode 0600 and hold only a token and an expiry.

S9 (gate for sigil, live for the wiring). The narrowed token is not written to any file or to `gh`'s credential store, by sigil or by the wiring, and is not printed by the wiring. It is not cached.

## Hard constraints

- The existing GitHub App only, narrowed at mint.
- Unattended: no prompt at mint or at use.
- The permission set is a fixed constant, not chosen by the caller. Each permission is included only for a landing step shown by trial to need it, apart from a read-only addition GitHub itself makes (see Expected grant).
- Landings go through pull requests, workflow changes included. This is a convention: where a target repository has no branch protection, nothing server-side enforces it, and the same token can push to the default branch.
- The security invariants in `CLAUDE.md` stay in force.
- No new dependency without the maintainer's approval.
- Lead sessions' git goes over https: `origin` is https on target repositories, and `gh`'s protocol for github.com is https (set and read back on 2026-11-09). A clone still on an SSH remote is switched before a lead session lands there.
- `just check` does not cover the end-to-end path without real 1Password, so the live criteria are part of done, and the prior binary is kept for rollback until they pass.

## Evidence

- Measured 2026-11-09: `gh` in a session authenticates by default as the maintainer through a keyring login holding a broad personal token, and landing work runs through it.
- Measured 2026-11-09, read from `main.go` and `token_narrow.go`: the only unattended narrowed mint today is `sigil token --rate-limit-only`, which grants `metadata: read`. `sigil get` mints the full grant unattended, for git. `sigil token <owner>` mints the full grant and refuses a non-terminal stdout.
- Recorded in `CLAUDE.md`: GitHub's grant does not always match the request. An access-token body of `{"permissions":{}}` has been observed to grant every installation permission, and `"repositories":[]` to select all repositories. Requested scope cannot be trusted without checking the response.
- Measured 2026-11-09 across the five project clones with a GitHub remote: some repositories have branch protection and some have none.
- Inherited from a live read on 2026-11-09, not re-verified here: the App's installation held workflow write.
- Inherited, not re-verified here: the merge endpoint sits under `contents: write`, so a token that can push can merge. The exact permission set is settled by trial at Frame. A Dependabot-alert permission is excluded unless a landing step is shown to need it.
- The lead's own observation, not measured: lead sessions commonly run past one hour, so a token expiring mid-session is the ordinary case.

## Risk profile

Trust boundary: yes. This creates a new unattended path to a write-capable credential and changes what `gh` authenticates with by default. Required: threat modelling runs at the Frame.

Risk to be signed at Frame. An earlier acceptance was reported from another work stream, without his exact words: for now, an App token may land a pull request that changes a deploy workflow, and that workflow can read its secret. The security read says the true risk is wider, and that wider statement is what he is asked to sign. A token that can land can change any file on the default branch where nothing protects it, workflows included, and a workflow it changes can read any secret that workflow can reach, which may include a credential for another repository. So "one repository" is the token's direct reach, not its indirect reach.

Parked, human-noticed: this acceptance is re-decided when the maintainer's `gh` login is revoked from sessions, because the App token then becomes the highest authority there and the reason for accepting no longer holds. The Frame's risk-register entry will carry this condition. When that entry is written at Frame exit, the lead adds its citation to the tracked work to revoke the login, so it is read whenever that work is picked up.

## Security implications (draft scope)

From the security read. Deployment: one maintainer's Mac, single user, agent sessions running as that user; GitHub API over https; the App key in 1Password; the service-account token in the Keychain. The actor set is unsigned and the maintainer signs it at Frame.

- ACTOR-1, anything running as the user, including a misdriven or injected agent — in — NOT bounded by this work (Non-goals 1, 2, 4 and 8); the work changes only its default path.
- ACTOR-2, a reader of a leaked token — in — this is the actor the narrowing bounds.
- ACTOR-3, an outsider acting through public issues or pull requests, reaching sigil only through an agent — in — security read recommends modelling it as a source of ACTOR-1's likelihood, at the path where a session reads attacker-written text through `gh`.
- ACTOR-4, a write collaborator, or a compromise of that account — in — security read recommends moving it out unless a surface is named that this change gives it. Maintainer's call at Frame.
- ACTOR-5, a network attacker with a mis-issued certificate — in — security read recommends out: nothing here changes TLS and pinning is not worth building; retires no layer. Maintainer's call at Frame.
- Another local user, a thief of the Mac, GitHub, 1Password or Apple themselves, the upstream Go modules as a modelled actor, a stranger installing the public App — out — single-user machine and scope of this work; retires no layer.
- Downstream deployments — out — note that a bad commit landed on a public repository is the route by which they would be hit; retires no layer.

Assets: the maintainer's personal token and SSH key, both still reachable (high); the App key and service-account token (high); the narrowed landing token (high where the repository's workflows hold secrets, otherwise medium; one hour); the default branch and workflow files of target repositories (high).

Not to build: certificate pinning, caller identity, a cache for narrowed tokens, a caller-chosen permissions flag, a wrapper that locks the keyring. Per-session repository pinning was on this list until 2026-11-11, when the maintainer's ruling to pin the landing token to the session's own repository (Read-back) removed it: his goal is landing "from the repo they are running in", "with least privledge where possible".

Known gap for the Frame to rate and the maintainer to sign: a lead session started without the wiring runs on his login, as sessions do today, and nothing reports it.

Items for the Frame: whether the wiring reaches every lead launch automatically or only one launch path, which sets the size of that gap, and whether it reaches dispatched agents; which repositories run S4 and S5, including one whose protection permits the landing; the residual routes to a wider credential for ACTOR-1, so the narrowing's marginal gain is stated from that list; `gh`'s credential precedence and how S7 fails closed; how the token reaches `gh`, how it is refreshed and the resulting mint rate, under S9's no-cache and no-file limits; how a session's token is held to the session's repository (asked until 2026-11-11 as how a session working on several repositories gets the right token; the pinning ruling replaced that question); the permission set and GitHub's exact expected response by trial, the check staying exact-match; which secrets target repositories' workflows can reach; whether the App or an admin-equivalent actor sits on a bypass list on protected repositories; an agent permission rule for the new mint written narrowly enough not to admit the full-grant form.

## Consultations

Warden's discovery-stage security read supplied `## Security implications (draft scope)`.

## Dismissed review flags

- S4 and S5 name their repository by outcome rather than by property (Cerberus, low to medium): the repository is chosen at the Frame, once the permission trial shows which ones qualify.
- Read-back items 5 and 6 repeat other sections (Cerberus, Warden, as cut candidates): the read-back is the required record of what was put to the maintainer.

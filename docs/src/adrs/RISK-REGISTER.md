# Risk register

Above-the-bar risks the maintainer accepted or deferred, one entry each, numbered `R-NNNN` from `R-0001`. Each entry is taken from the Frame named on its `Architecture session` line, which carries the full text; ratings are `likelihood/impact`. A field the Frame does not state reads "not stated in the Frame".

## R-0001: Indirect reach of a landing token

- **Status:** accepted
- **Date signed off:** 2026-11-11
- **Signed off by:** the maintainer. His choice, verbatim: "Accept, one signature (Recommended)". It covers the world after his `gh` login is revoked (the Frame's `ED-6`, answered yes).
- **Rating:** medium/high — above the bar
- **Actor:** ACTOR-1, or ACTOR-3 through it by attacker-written issue or pull-request text that a session reads through `gh`
- **DFD element:** DF-11, the path by which a session reads that text
- **STRIDE category:** not stated in the Frame
- **Architecture session:** `docs/intent/lead-session-gh-token-frame.md`, `## Accepted and deferred risks`, `RISK-A`
- **Rationale:** A token that can land can change any file on the default branch where nothing protects it, workflows included, and a workflow it changes can read any secret that workflow can reach, which may include a credential for another repository. "One repository" is the token's direct reach, not its indirect reach. No lens covers that steering, and narrowing does not change the route, only bounds its direct reach to the session's repository. Written to hold once his login is off: at that point the highest authority reachable in a session is `sigil get`'s full grant and the service account's reach (RR-4 to RR-6), and this token remains narrower than those.
- **Trigger (deferred only):** not applicable. Ending event: agent sandboxing, which is self-announcing, because someone has to provision it.
- **Compensating controls (if any):** the narrowing, which bounds the token's direct reach to the session's repository (the Frame's `FD-1` to `FD-3`, `FD-7`)
- **Review date:** not stated in the Frame

## R-0002: Wider routes left open to ACTOR-1

- **Status:** accepted
- **Date signed off:** 2026-11-11
- **Signed off by:** the maintainer. His choice, verbatim: "Accept (Recommended)".
- **Rating:** medium/high — above the bar
- **Actor:** ACTOR-1
- **DFD element:** not stated in the Frame as one element; the routes are the Frame's residual routes RR-1 to RR-7
- **STRIDE category:** not stated in the Frame
- **Architecture session:** `docs/intent/lead-session-gh-token-frame.md`, `## Accepted and deferred risks`, `RISK-B`
- **Rationale:** RR-1 and RR-2 end when his login is off; the rest remain, as the intake's Non-goals state. RR-4 is the one an ordinary command reaches: `git push https://github.com/<other-owner>/<other-repo>.git`, from the session's own clone, pushes with a full-grant token to any covered repository, workflow-bearing branches included. On a default branch nothing protects, that is a merge without a pull request, which the repository pin does not stop. If the Frame's `LT-9` fails, the extension route returns as one above the bar, this signature does not cover it, and it is put to him for signature before the run with nobody present.
- **Trigger (deferred only):** not applicable. RR-4's closing work, narrowing the credential helper's grant, is parked, human-noticed: read whenever he revokes his `gh` login from sessions. Ending event: agent sandboxing, as for `R-0001`.
- **Compensating controls (if any):** not stated in the Frame
- **Review date:** not stated in the Frame

## R-0003: 1Password daily limit exhausted by ordinary use

- **Status:** deferred
- **Date signed off:** 2026-11-11
- **Signed off by:** the maintainer. His choice, verbatim: "Defer to the rule (Recommended)".
- **Rating:** high/medium — above the bar; below it if `DEF-1`'s decision rule comes out below its line, or once his choice under it is in place
- **Actor:** ACTOR-1, through ordinary heavy use
- **DFD element:** not stated in the Frame
- **STRIDE category:** not stated in the Frame
- **Architecture session:** `docs/intent/lead-session-gh-token-frame.md`, `## Accepted and deferred risks`, `RISK-C`
- **Rationale:** Per-call minting raises what was a recoverable quota worry to something ordinary use can reach, at least on the lowest tiers. On the reported plan, at the measured peak day, the limit is reached above about 1.8 reads per mint, and that count is a floor [estimate]. Reaching it stops `gh` and, within 55 minutes, git on every lane until the limit resets. Mitigation: the Frame's `DEF-1`.
- **Trigger (deferred only):** `DEF-1`'s decision rule, evaluated before the run with nobody present, which does not start until the rule's result is recorded (the Frame's `FD-11`); in running, the `FD-9` warning, which fires on the mint count at the same line.
- **Compensating controls (if any):** not stated in the Frame
- **Review date:** not stated in the Frame

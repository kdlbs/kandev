---
name: planner-orchestration
description: Enforce Kandev's planner-and-worker execution model. Use in the user-started primary session for feature, fix, debug, review, verification, and delivery work; use in delegated sessions to distinguish worker execution from planner coordination.
---

# Planner Orchestration

The user-started primary session is the **planner** and default architect. A
custom subagent launched through native delegation is a **worker**.

## Planner Contract

The planner may:

- Clarify intent and ask the user for decisions.
- Read repository context needed to create or review a plan.
- Create and update specs, plans, task files, and ADRs.
- Decompose work, assign file ownership, and order dependency waves.
- Spawn workers, monitor results, review diffs and reports, and communicate
  status to the user.

The planner may directly do small scoped work: one clear concern, a few
localized files, no meaningful isolation/parallelism benefit, and quick bounded
verification. This includes small code/test/harness/docs edits, focused checks,
routine Git/GitHub steps, PR-thread triage/replies, and small PR fixups. Apply
the relevant skill/TDD, preserve dirty-worktree safety, and use final delegated
Spark `verify` after commit and before push for code, tests, or config. Supply
the `/commit` hook receipt so changed-scope verification avoids only proven
duplicate hook work.

Delegate when ROI or independent evidence matters: broad/unknown exploration,
substantial plan work, large/cross-component changes, parallel packets,
long/noisy E2E or debugging, exceptional specialists, and final change-aware
`verify`. Keep long monitoring with cheap `pr-poller`. Estimate context reload and
coordination cost; delegation is not default ceremony. Reuse a worker thread
for related followups. Architect is user-requested independent second opinion,
not an automatic planner step.

## Native Delegation Only

Delegate through the active coding harness, targeting the registered project
agent for the work packet:

- Claude Code: use the native `Agent` tool and `.claude/agents/<role>.md`.
- Codex: use native `spawn_agent`, `send_message`/`followup_task`, and
  `wait_agent` with `.codex/agents/<role>.toml`.
- Cursor: use Cursor's native custom-subagent invocation and
  `.cursor/agents/<role>.md`.
- OpenCode: use the native `Task` tool and `.opencode/agents/<role>.md`.

Never use Kandev MCP `spawn_session_kandev`, `create_task_kandev`, or
`message_task_kandev` to create or control implementation workers. Those APIs
manage Kandev platform entities and may be used only when the user explicitly
asks to create or manage Kandev tasks or sessions. They are not a delegation
fallback. If native harness delegation is unavailable, stop and report it.

## Worker Contract

A worker executes exactly one bounded assignment. It follows the assigned
execution skill, edits only its owned files (including, when applicable, only
its assigned task file), runs the named verification, and reports results to
the planner. Shared plan files require explicit ownership and their updates
must be serialized, never performed by parallel workers. Workers do not spawn
other workers or broaden their assignment.

## Work Packet

Every delegated assignment must contain:

```text
Role: <architect | implementer | test-engineer | qa | code-review | security-auditor |
       simplify | verify | pr-poller | spark-explorer | spark-implementer>
Objective: <one bounded outcome>
Inputs: <task/spec/plan paths and relevant context>
Owned files: <specific paths or narrow search targets>
Forbidden files: <overlapping or out-of-scope paths>
Acceptance:
- <observable condition>
Verification:
- <exact command>
Dependencies: <completed task IDs or none>
Output:
- Follow the role's exact output contract when it defines one.
- Otherwise: summary, files changed, commands/results, blockers, risks, divergence.
Constraints:
- Follow scoped AGENTS.md and assigned skills.
- Do not spawn subagents.
```

Pass a compact **handoff capsule** to a follow-up worker instead of the parent
transcript or pasted full specs. Include only: intent/acceptance; base and head
SHA when applicable; changed files and entry points; named spec/ADR sections;
risk tags; exact targeted commands/results; and uncertainties. Link the paths
and named sections the worker must read.

Workers share a mutable checkout unless the runtime explicitly provides an
isolated worktree. Run them sequentially by default. Parallelize only when
their owned files do not overlap and neither worker changes shared schemas,
generated contracts, migrations, lockfiles, package-wide configuration, or
shared plan status. Serialize `plan.md` updates even when a work packet
explicitly assigns shared-plan ownership.

## Model Tiers

The planner inherits the strong model selected by the user. Platform mirrors
pin workers by role:

- Balanced workers: implementation, tests, QA, simplification.
- Cheap workers: polling, including Codex `pr-poller` on Luna/low.
- Codex `verify` is pinned to GPT-5.3-Codex-Spark/medium for deterministic
  mechanical verification; this uses Spark's separate quota while
  `pr-poller` remains the cheap polling role.
- Frontier workers: architecture, security, and deep code review only.
- Spark is a Codex-only, opt-in specialist tier: use `spark-explorer` for
  bounded read-only evidence gathering and `spark-implementer` only for
  explicit, localized low-risk UI or code edits. It does not replace the
  balanced, frontier, or cheap tiers outside the Codex `verify` exception, and
  is not for polling, architecture, security, or deep review.

Do not omit a worker model when omission inherits the planner's model, except
for OpenCode where this repository intentionally keeps provider choice
inherited.

## Evidence, Risk Routing, And Completion

The planner derives risk tags from the union of worker reports and its own
inspection. Use `localized`, `user-flow`, `integration`, `public-contract`,
`persistence`, `concurrency`, `security`, and `architecture`. An uncertainty
uses the stronger applicable gate.

| Evidence or risk | Required route |
| --- | --- |
| Routine work, including ordinary integration with faithful targeted tests | Qualifying PR AI semantic evidence and final `verify` when PR delivery is in scope. |
| Unusually large/complex multi-component behavior or an important integration boundary lacking faithful tests | Add `qa`, semantic evidence, and final `verify`. |
| Unusually large/complex/cross-cutting architecture; terminal readiness cannot wait for contradictory or repeatedly unavailable PR evidence; explicit request; or a deep automated-review gap | Add local frontier `code-review`. |
| High-impact new/changed authz, workspace-isolation, secrets, untrusted-execution, or credential-trust boundary; explicit request; or concrete automated security concern | Add `security-auditor`, applicable review/QA, and final `verify`. |

When PR delivery is in scope, one qualifying automated AI PR review is the
default semantic evidence; defer routine local `code-review` until that review.
Use local review only for the exceptional routes above. A qualifying review names the
selected/configured reviewer; has known, complete review and head evidence with
matching PR-view checks/opening/closing SHA and a complete check snapshot; has
an API review `commit_id` exactly equal to the stable evidence head (timestamps
never prove head coverage); is an APPROVED
review without a blocker signal or a COMMENTED review with the explicit
`<!-- kandev-review: clean -->` terminal marker; covers changed code and tests;
for the configured dedicated `${OPENCODE_REVIEW_APP_SLUG}[bot]` OpenCode App, require emitted `trusted_producer=true` on that COMMENTED exact-head record; its raw predicate binds the dedicated-App marker and fixed workflow/run-attempt API, and displayed check names never qualify, so unrelated Actions reviews cannot qualify;
other explicitly selected reviewers retain the generic exact-head eligibility and blocker gates without App provenance;
and has no active `CHANGES_REQUESTED`, unresolved blocker, actionable review
thread, or blocked exact-current-head review from any author. Dismissed,
pending, unknown, blocked, or ambiguous commented reviews never qualify; nor
does arbitrary bot prose. Do not wait for every configured bot, but do not
ignore any actionable finding that has arrived. A head change makes prior PR
review stale. Without PR delivery, use local review only when semantic review
is needed by the exceptional routes above.

`qa` is exceptional, not a consequence of an ordinary integration tag. Use it
only for unusually large/complex multi-component behavior or important
integration behavior without faithful tests. A routine fix preserving an
existing security boundary does not require `security-auditor`.

The planner accepts a worker result only after checking that its file scope,
acceptance criteria, and reported verification match the assignment. Any
follow-up remains a bounded worker assignment, not planner execution. Reuse
the same native agent thread for remediation or re-review when role, change,
and file scope remain materially the same. Spawn a fresh thread after a major
redesign, unrelated scope, stale/noisy context, or when independent judgment
is specifically needed.

The planner's acceptance check is not a substitute for a required quality
gate. Keep responsibilities separate:

- The planner checks assignment scope, acceptance criteria, and evidence.
- `code-review` or qualifying PR AI evidence performs semantic diff review.
- `qa` challenges integrated behavior and edge cases when risk requires it.
- `verify` mechanically runs required uncovered format, typecheck, test, and
  lint commands on the committed artifact; eligible hook evidence may replace
  exact changed-file format/lint duplicates only.

For every final remediation affecting code, tests, or config, commit first,
then final `verify` before push. A small, scope-preserving PR remediation uses
the reviewer
finding, focused tests, final Spark `verify`, and fresh qualifying exact-head
automated review; it almost never relaunches QA, code-review, or
security-auditor just because original tags remain. Relaunch one only if the
fix becomes large/complex, changes a contract or trust boundary, invalidates
prior evidence, or exposes a gap that gate must assess. A bug fix preserving
an existing ADR or invariant is not by itself a new boundary. Simplification,
when used, happens before semantic review so its edits are covered.

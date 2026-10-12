---
id: coordinator-proposal-recovery-design
title: Proposal approval recovery design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-PROPOSALS-002
---

# Proposal approval recovery System Design

## Purpose and boundaries

Recovery of an `approving` proposal whose claim went stale: the startup pass, the once-a-minute sweep and the approve request's stale re-claim, which all share one claim `UPDATE`. The approve flow, the stale re-claim itself and the eligible-step check stay in [proposals](proposals.md); the card's stale state is in [proposals](proposals.md#cards).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PROPOSALS-002` | [Recovery](#recovery) (`AC-COORDINATOR-PROPOSALS-002.7`, `002.10`, `002.14`) |

## Recovery

Three callers run recovery on an `approving` row: the startup pass, a sweep,
and an approve request. A proposal read, single or list, never writes; it
always returns rows as stored.

- **Approve request.** A claim is stale after two minutes. An approve of an
  `approving` row whose claim is stale, with no edits, takes the
  [stale re-claim](proposals.md#stale-re-claim) with `cutoff = now - 2 minutes`.
- **Startup pass.** task-07's decisions registration function returns the
  hook that `startCoordinatorBackgroundPass`
  (`internal/backendapp/coordinator.go`) runs once per startup, after the
  conversation and subscriber hooks, with `T0`, the time recorded before the
  coordinator routes register. Every claim this process makes has
  `claimed_at >= T0`, so an `approving` row with `claimed_at < T0` was
  claimed by a process that has since stopped. The pass therefore uses
  `cutoff = T0`, not the two-minute rule, and a restart within two minutes
  of a claim still recovers it. Were a still-running process to hold that
  claim, the claim token keeps the outcome to one task: that process's
  completion matches no row and returns the current row (step 5).
  - **Discovery.** task-07 adds the store method
    `ListApprovingClaimedBefore(ctx, cutoff)`: `SELECT ... FROM
    coordinator_proposals WHERE status='approving' AND claimed_at < ? ORDER
    BY claimed_at ASC, id ASC`, across every workspace and coordinator, with
    no limit (open proposals are capped at 25 per coordinator) and no new
    index. It runs with the hook's context, which carries no principal, so
    the task-service calls it leads to are unscoped, like other internal
    passes.
  - **Per row, in that order, one at a time.** Take the stale re-claim with
    `cutoff = T0`. A re-claim that matches no row (an approve won it, or the
    row is gone) skips the row with no action. A committed re-claim runs the
    lookup and, when no task is found, steps 4 to 6 exactly as the stale
    re-claim describes.
  - **Errors.** A discovery query that errors is logged at warn and ends the
    pass; nothing retries it until the next startup. An error on one row
    (the re-claim `UPDATE`, a read after it, or a create or completion) is
    logged at warn with the proposal id, and the pass continues with the
    next row. The row is left as the stale re-claim's error rule says.
  - The pass stops early when its context is cancelled (shutdown).
- **Sweep.** The startup pass covers a claim left by a stopped process; a
  claim that goes stale while the process keeps running (its approve request
  died after the claim) has no other caller until a manager acts, and its card
  showed no action. The proposal service therefore runs the same per-row
  recovery once a minute while `features.coordinator` is on, with
  `cutoff = now - 2 minutes`, so such a claim is recovered within a minute
  with no manager action (`AC-COORDINATOR-PROPOSALS-002.14`). The sweep is a
  `time.Ticker` goroutine owned by the proposal service, started by the
  decisions registration after the startup pass returns and stopped when the
  app context is cancelled; its first tick is one minute after start (the
  startup pass covers everything older than `T0`), and each tick runs one
  pass over `ListApprovingClaimedBefore(now - 2 minutes)` with the startup
  pass's per-row order and error rules, `now` read once at the tick. A pass
  still running when the next tick is due makes the ticker drop that tick,
  so two passes of one process never overlap. The one-minute bound applies
  to the first recovery attempt after a claim goes stale. An attempt whose
  re-claim commits but whose lookup, create or completion then errors leaves
  the row `approving` under the refreshed `claimed_at` (the stale re-claim's
  error rule); that claim goes stale two minutes later, and the next sweep
  after that retries it, so a failing row is retried roughly every two to
  three minutes until it completes, fails with a descriptive error, or is
  deleted. Tests drive the sweep through an injected clock and tick channel,
  never real sleeps. The card of a
  row whose claim is stale shows **Retry**, which sends an approve without
  edits and so takes the approve path (`AC-COORDINATOR-PROPOSALS-005.10`).

All three callers use the same stale re-claim `UPDATE`, which refreshes
`claimed_at` and sets a new `claim_token`. When two of them race on one row, exactly one wins, and a slow original claimer's completion
no longer matches the token. The winner then follows the stale re-claim's
order. First it looks the task up by the reserved external id; a found task
completes the approval with it, whatever the step's eligibility is now. Only
when no task is found does it run steps 4 to 6, whose pre-create check
against the frozen spec's workflow and step
([No agent starts](proposals.md#no-agent-starts)) sets the proposal `failed` with a
descriptive error and skips the create when the step is ineligible, and
whose idempotent create otherwise returns the task an earlier attempt made,
if any. Recovery keeps
`final_spec_json` and `decided_by` from the first claim, and never touches a
session.


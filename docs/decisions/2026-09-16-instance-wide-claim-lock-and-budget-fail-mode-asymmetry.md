# ADR-2026-09-16-instance-wide-claim-lock-and-budget-fail-mode-asymmetry: Instance-Wide Launch-Claim Lock Scope and Budget Fail-Mode Asymmetry

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend

## Context

Office's unattended launch safety work
([Office Unattended Launch Safety](../specs/office/requirements/unattended-launch-safety.md),
[Office Launch Budgets](../specs/office/requirements/launch-budgets.md)) bounds unattended
agent launches at three concurrency scopes — per agent profile, per workspace, and
instance-wide — and separately bounds launch rate with rolling workspace and routine
budgets. Two design questions came up that the acceptance criteria constrain but do not
settle by themselves.

**Claim lock scope.** `ClaimNextEligibleRun`'s claim statement
(`unattended-launch-safety-02.md#claim`) enforces all three concurrency ceilings — agent,
workspace, instance — as subqueries inside one `UPDATE ... WHERE id = (SELECT ...)`. Those
subqueries count *other* rows the row-level update does not itself lock. SQLite's single
writer makes this safe by construction; PostgreSQL under `READ COMMITTED` does not, so two
concurrent claim attempts can both read the same pre-commit ceiling counts and both pass.
Postgres therefore needs an explicit advisory lock, and the lock's key scope was not fixed
by any AC: workspace-scoped and agent-scoped keys were both plausible, matching the
enqueue-time self-trigger lock's per-agent-profile key
(AC-OFFICE-LAUNCH-SAFETY-003.8).

**Budget fail mode.** `AC-OFFICE-LAUNCH-SAFETY-005.5` requires that an unreadable
launch-count budget counter defer the run and record the failure
(AC-OFFICE-BACKPRESSURE-003.3) — fail closed. The pre-existing pre-execution cost budget
check (`internal/office/costs/budgets.go`) does the opposite: it proceeds when its own
counter read errors — fail open. Both checks gate the same launch, so leaving the asymmetry
unexplained invites "fixing" one to match the other.

## Decision

**The Postgres claim-time advisory lock uses a single fixed instance-wide key**, not a
workspace-scoped or agent-scoped one. The claim statement enforces the agent, workspace,
and instance ceilings together in one statement; the instance ceiling is the broadest of
the three and is what protects the shared machine. A lock keyed narrower than that would
serialize only the ceiling it matches while leaving the instance ceiling racing across
every workspace or agent it does not cover — the exact defect AC-OFFICE-LAUNCH-SAFETY-001.6
exists to prevent. The enqueue-time self-trigger lock stays per-agent-profile: it counts
over a `(agent_profile_id, reason)` scope and a `(agent_profile_id)` scope, both no broader
than the agent profile itself, so per-agent is sufficient there and keeps unrelated
enqueues parallel. The two locks are deliberately scoped differently because they guard
different statements.

**The launch-count budgets (workspace, routine) fail closed; the pre-existing cost budget
stays fail open.** These are two different risks, not one mechanism applied
inconsistently. An unreadable launch-count check risks the unbounded concurrent-launch
fan-out this whole capability exists to prevent, and deferring is safe — the queued run
survives untouched (AC-OFFICE-LAUNCH-SAFETY-005.3/005.4) and is retried once the counter is
readable again. An unreadable cost check risks an overspend that a human operator can see
on a provider invoice and stop; that is a materially different failure a fail-open posture
already tolerates in production. Retrofitting the cost budget to fail closed, or relaxing
the launch-count budgets to fail open for consistency, is explicitly out of scope for this
decision and for `launch-budgets.md`.

This same fail-closed-for-refusal-gates direction, for a different mechanism, is what
`AC-OFFICE-LAUNCH-SAFETY-004.9` states for the self-trigger allowance gates: an unreadable
self-trigger count refuses the enqueue and records the failure, exactly the launch-count
budgets' posture, and for the same reason — the risk being guarded against is unbounded
self-caused fan-out, not a reversible overspend. The one carve-out shared by both is a
shutdown-triggered context cancellation (AC-OFFICE-LAUNCH-SAFETY-001.8): that defers or
refuses without recording a gate failure, so a restart is never mistaken for a gate that is
genuinely failing closed.

## Consequences

- A new budget or gate dimension must pick fail-open or fail-closed explicitly against this
  precedent — the launch-count family (unbounded-fan-out risk) fails closed and records the
  failure; the cost family (reversible-overspend risk) fails open — rather than defaulting
  to whichever is easiest to implement.
- The Postgres claim lock serializes every claim attempt instance-wide even when two
  attempts would only ever contend on a workspace or agent ceiling. This is accepted: at
  `maxRunsPerTick = 10` contention is negligible, and a lock avoids the retry contract
  `SERIALIZABLE` isolation would require.
- A future per-workspace configured ceiling override (explicitly out of scope in
  `launch-budgets.md`) must not narrow the claim lock's key without re-deriving this
  argument — narrowing the key while the claim statement still enforces the instance
  ceiling reopens the exact race this decision closes.
- `AC-OFFICE-LAUNCH-SAFETY-004.9`'s self-trigger refusal-on-unreadable-count behavior and
  this decision's launch-count-budget behavior are now documented as one fail-closed family;
  a reviewer changing one should check the other.

## Alternatives Considered

1. **Workspace-scoped claim lock key.** Rejected: the instance ceiling, the broadest of the
   three enforced in the same statement, would still race across workspaces.
2. **Agent-profile-scoped claim lock key**, matching the enqueue-time self-trigger lock for
   consistency. Rejected: narrower than the workspace-scoped option and the same instance
   ceiling still races; consistency with a lock that guards a different statement is not a
   reason to under-serialize this one.
3. **Postgres `SERIALIZABLE` isolation instead of an explicit advisory lock.** Rejected: the
   claim loop has no retry contract for a serialization failure, and an explicit lock is
   already precedented in `internal/secrets` and `internal/workflow/repository`.
4. **Fail-open launch-count budgets, matching the cost budget.** Rejected: the risk an
   unreadable launch-count counter masks (unbounded concurrent launch fan-out) is not
   reversible the way an overspend is; AC-OFFICE-LAUNCH-SAFETY-005.5 requires fail-closed
   explicitly.
5. **Fail-closed cost budget, matching the launch-count budgets.** Rejected: explicitly out
   of scope per `launch-budgets.md`'s own text — the cost check is a live, separately-owned
   path, and changing its fail mode is a different piece of work with its own review.

## Related decisions

- [ADR 0027 — replayable schema migrations](0027-replayable-schema-migrations.md), which the
  claim-lock and budget migrations in this work follow.
- [Consolidate Office run enqueue onto one authoritative API](2026-09-16-consolidate-office-enqueue-paths.md),
  the sibling ADR from the same design that this document's enqueue-time self-trigger lock
  discussion assumes.

# ADR-2026-10-30-turn-checkpoint-branch-binding: Preserve proven branch identity across title renames

**Status:** proposed
**Date:** 2026-10-30
**Area:** backend

## Context

[Issue #4961](https://github.com/kdlbs/kandev/issues/4961) exposes a gap between title resolution and turn history.
The title owner can rename a verified generated branch after start capture.
The comparison then rejects different symbolic refs, even with identical HEAD commits and trees.
A genuine switch to another branch at the same commit produces the same endpoint values.

The existing history decision requires immutable evidence and explicit unavailable results when evidence is insufficient.
Commit equality alone cannot distinguish these operations.

## Decision

Add an optional branch-binding UUID to immutable v2 checkpoint provenance.
A cached checkout operator owns one live binding and its observed symbolic ref.
Only the generated-branch rename path can transfer that binding to a different ref.
The orchestrator supplies its expected generated branch; agentctl checks the actual ref before the rename.
Capture and the guarded rename share the checkout capture lock.

Different ref names can compare only when the receipts prove the same binding.
Receipts with different known binding IDs remain unavailable even when the ref names match,
because a switch can recreate the previous branch name at the same commit.
Receipts without binding evidence keep the legacy equal-ref fallback.
All independent target, history, operation, and object checks still apply.
Comparison uses captured evidence after both endpoints exist.
It never consults mutable branch names or tool output to recover missing proof.

This extends the [history integrity decision](2026-10-27-turn-change-history-integrity.md).
It preserves [immutable turn intervals](2026-10-07-immutable-turn-change-intervals.md).
The [system design](../specs/tasks/system-design/turn-change-history-integrity.md#generated-branch-continuity)
owns the request, receipt, synchronization, and compatibility details.

## Consequences

- Title renames preserve valid file comparisons without changes to transcript visibility rules.
- A genuine same-commit branch switch remains unavailable.
- The receipt extension fits existing JSON storage and checkpoint commit bodies.
- Older endpoints without binding evidence retain conservative comparison behavior.
- Replacement of agentctl before end capture loses unrecorded continuity and can produce unavailable history.
  Finalized receipts remain sufficient after process replacement.
- The bounded live binding needs no additional persistence, cleanup service, or SQL migration.
- The guarded operation must verify the expected branch at the Git mutation boundary.
  A database branch snapshot alone cannot establish the current checkout state.

## Alternatives considered

- Ignore ref changes when HEAD is equal: accepts a genuine branch switch at the same commit.
- Infer ownership from branch names or Git reflogs: lacks the required immutable proof of the Kandev operation.
- Suppress empty unavailable cards: conceals the failed comparison and still loses turns with file edits.
- Rewrite the start receipt after title resolution: changes an accepted boundary and weakens historical review.
- Persist a separate branch-identity journal: adds recovery and retention contracts for evidence already stored in endpoint receipts.
  Conservative failure after an interrupted process is sufficient for this repair.

## Related artifacts

- [Requirements](../specs/tasks/requirements/turn-changed-files.md), AC-TASKS-TURN-CHANGES-002.14 and .15.
- [Fix plan](../plans/turn-change-title-branch-continuity/plan.md).

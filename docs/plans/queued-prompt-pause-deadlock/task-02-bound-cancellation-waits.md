---
id: "02-bound-cancellation-waits"
title: "Bound cancellation guard waits"
status: done
wave: 2
depends_on:
  - "01-separate-prompt-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-UI-CANCEL-TURN-PROGRESS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
  - AC-UI-CANCEL-TURN-PROGRESS-001.5
  - AC-UI-CANCEL-TURN-PROGRESS-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/ui/system-design/cancel-turn-progress.md
---

# Task 02: Bound cancellation guard waits

## Summary

Make cancellation-owned mutex waits observe the existing operation deadline.
Settle all joined callers and release pending progress when the guard cannot be acquired.
An expired operation must have no later side effects.

## In scope

- Add context-aware acquisition against the existing physical mutex and guard reference registry.
- Cover initial preparation, lifecycle-return reacquisition, joined explicit reconciliation, and source-action acquisition.
- Preserve single-owner cancellation and service-owned lifetime after the initiating caller disconnects.
- Propagate deadline failures through existing errors and revisioned pending=false publication.
- Add deterministic timeout, mixed-source join, retry, and successor-safety tests.

## Out of scope

Do not change provider escalation, the 30-second production operation TTL, or successful workflow completion policy.
Do not replace every repository mutex, synthesize success, or add another cancellation registry.

## Acceptance

1. `TestCancelAgent_GuardDeadlineSettlesOperation` holds the guard through an injected short deadline.
   Cover initial acquisition, post-runtime reacquisition, and joined actions. Every waiter settles; pending clears once with an increasing revision.
2. `TestCancelAgent_GuardDeadlineCannotMutateSuccessor` releases the guard after expiry and admits replacement work.
   Assert no late cancellation, workflow action, queue dispatch, turn closure, or deletion of successor ownership. A fresh retry can succeed.
3. Table-driven mixed-source cases cover explicit, silent, peer, and Send Now participants, including a disconnected initiating caller.
   Preserve one runtime cancel and source-specific completion policy. When lifecycle cancellation never started, its call count remains zero.

## Verification

Run from the repository root with an owned temporary directory on a volume with capacity.
Remove only the directory created by this block after verification.

```bash
task_tmp="$(mktemp -d /root/kp.XXXXXX)"
export TMPDIR="$task_tmp" GOTMPDIR="$task_tmp"
(cd apps/backend && go test ./internal/orchestrator -run '^TestCancelAgent_GuardDeadline' -count=1 -timeout=90s)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(Cancel|PromptTask_QueuedStreamBoundary|PromptTask_PreparedDispatch)' -count=10 -timeout=300s)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/handlers -count=1 -timeout=300s)
git diff --check
```

Record expected RED behavior before the helper exists, then record every final result.
Use short injected contexts or a fake clock without reducing production timeouts.
Assert reference cleanup and final operation result, not only caller timeout.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/service.go` if test injection or guard types require it
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations_cancellation_deadline_test.go` (new)
- Existing cancellation coordinator, projection, Send Now, and peer-interrupt tests in the orchestrator package

## Dependencies

Task 01. Both tasks change admission and cancellation ownership in `task_operations.go`.

## Risks

A detached goroutine waiting on `Lock` can mutate state after timeout; do not use that pattern.
Projection cleanup must occur even when reconciliation cannot acquire the guard.
An expired operation must not clear a newer operation's reference or advertise successful cancellation.

## Parallelism

`sequential`

## Inputs

- [Bounded cancellation acquisition](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md#bounded-cancellation-guard-acquisition)
- [Existing progress contract](../../specs/ui/requirements/cancel-turn-progress.md)
- `finishCancellationWithActions`, `reconcileJoinedExplicitCancellation`, and `cancelAgentWhileUnlocked`
- [Backend-owned progress ADR](../../decisions/2026-08-03-backend-owned-cancellation-progress.md)

## Results

Implemented context-aware acquisition against the existing reference-counted session guard for
initial cancellation ownership, lifecycle-return reacquisition, joined explicit reconciliation,
and source-specific actions. Deadline errors settle joined waiters, release only the matching
projection, and cannot run late mutations after a successor is admitted.

Verification passed:

- TestCancelAgent_GuardDeadlineSettlesOperation and TestCancelAgent_GuardDeadlineCannotMutateSuccessor.
- Targeted cancellation/dispatch race run with count=10: passed in 77.146s.
- Final orchestrator and handler race suite: orchestrator 430.509s, handlers 5.642s.

PR fixup findings (2026-09-28):

- Use a FIFO cancellation guard with context-aware acquisition and direct waiter handoff.
  A stream of TryLock callers can no longer starve an already-queued cancellation waiter,
  and timed-out waits do not allocate repeated polling timers.
- Keep the dispatch guard reference through prompt-claim rollback, reacquire it before
  rollback writes, and preserve successor turns when the failed claim becomes stale.
- Stop clarification recovery when cancellation guard reacquisition fails; no fallback
  state write or turn recovery may occur without the guard.
- `TestCancelInFlightMutexContextWaitCanBeCancelled`,
  `TestCancelInFlightMutexHandsOffInQueueOrder`,
  `TestRollbackPromptClaimWaitsForSuccessorGuardAndKeepsItsTurn`, and
  `TestRetryClarificationAfterCancel_DoesNotRecoverAfterGuardReacquireFails` cover these
  boundaries.
- The final four-package race suite passed after these changes: orchestrator 411.827s, executor
  3.667s, lifecycle 87.349s, backendapp 92.980s.

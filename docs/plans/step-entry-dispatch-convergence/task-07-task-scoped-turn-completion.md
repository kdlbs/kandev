---
id: "07-task-scoped-turn-completion"
title: "Task-scoped turn-completion serialization"
status: pending
wave: 7
depends_on: ["03-ledger-ownership"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-003
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.5
  - AC-OFFICE-STEP-ENTRY-DISPATCH-003.6
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 07: Task-scoped turn-completion serialization

## Summary

Re-key `turnCompletionLocks` and the staleness decision from session id to task
id, so two sessions of one task cannot both drive a transition. Keep
`turnCompletionConsumedGeneration` session-keyed as the narrower redelivery
check inside the task-keyed critical section.

## In scope

- `acquireTurnCompletionCriticalSection` taking the task id already in scope.
- The staleness guard rejecting a caller whose observed step differs from the
  task's current step, carried by the step comparison already in the critical
  section.
- `processOnChildrenCompleted` taking the task lock before reaching
  `applyEngineTransition`, with lock order task lock then operation lock
  everywhere.
- A test driving two distinct sessions of one task.

## Out of scope

- Deleting `turnCompletionConsumedGeneration`, which would lose same-session
  redelivery de-duplication.
- Any advisory or row lock. Both maps stay in-process `sync.Map`.
- Rewriting `allocateStepEntryIfPending` to `MAX(entry_seq)+1`. The unique
  constraint remains a correct backstop.

## Acceptance

- Two turn-completion signals for one task serialize, and the loser is rejected
  on the step comparison rather than on a session snapshot.
- A redelivered signal for the same session is still de-duplicated.
- The task lock and the operation lock cannot deadlock, because their order is
  fixed.

## Verification

```bash
cd apps/backend && go test -race -count=3 -run 'TestProcessOnEnter_|TestProcessOnTurnComplete_' ./internal/orchestrator/
```

## Files likely touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/step_entry_dispatch_concurrent_test.go`

## Dependencies

Task 03, which touches the same dispatch entry points.

## Risks

- The guard's meaning changes with the key: a snapshot from session B is not
  comparable to one recorded for session A, so restating it as the step
  comparison is required rather than incidental.
- The task lock widens a critical section that currently admits concurrent
  sessions of one task, making contention visible where it was previously
  silent corruption.
- The guarantee is scoped to one backend process.

## Parallelism

`sequential`

## Inputs

- System design, "Task-scoped turn completion".
- The existing single-session concurrency test, which does not build two
  sessions on one task.

## Results

Pending.

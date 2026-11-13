---
id: "04-completion-effects"
title: "Completion effects from committed state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.4
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 04: Completion effects from committed state

## Summary

Close linked routine runs from the committed task-state transition, and remove
the Office `task.moved` path that classifies completion by step name and
assignee. Resolves #5253. This work order does not depend on PR #4642.

## In scope

- Characterization tests first. Move an Office task into a completing final
  step named "Shipped", once with an assignee and once without. Assert that the
  parent wake (`orchestrator.processParentChildrenCompletedForTaskState`) and
  the blocker wake (scheduler `handleCommittedTaskCompletion`) each fire once.
  Assert that the routine run closes, which fails today.
- New `task.state_changed` subscriber in `internal/office/service` that maps
  `COMPLETED` to `done`, `CANCELLED` to `cancelled`, and `FAILED` to `failed`,
  then calls `routineRunSyncer.SyncRunStatus`.
- The state-keyed blocker cascade handles `COMPLETED`, `FAILED`, and
  `CANCELLED`, with duplicate delivery waking dependents at most once.
- Characterize a real child-completion event followed by a reconciler tick;
  prove an orchestrator run or receipt prevents a duplicate children-completed
  run.
- Delete `finalizeDone`. Remove its call from `handleTaskMoved`, and delete
  `categorizeStep` and `stepCategory` once they are unused. `handleTaskMoved`
  keeps its activity-log write and assignee condition.
- Answer the issue's question in the design: the assignee condition is not
  intended for completion effects.

## Out of scope

- Changing the parent-wake contract or its stable reconciliation identity.
- The activity-log fallback in `handleTaskMoved`.

## Acceptance

- The characterization test passes with the routine run closed as `done`. A
  `FAILED` task closes it as `failed`, and a `CANCELLED` task closes it as
  `cancelled`.
- A duplicate `task.state_changed` delivery leaves the routine run's
  `completed_at` unchanged and queues no second parent or blocker run.
- `FAILED` and `CANCELLED` blockers wake eligible Office dependents once, even
  when the state event is delivered twice.
- A completed Office child followed by a reconciler tick creates one
  children-completed run for its parent.
- Existing `routine_run_sync_test.go`, `children_completed_wave_identity_test.go`,
  and `scheduler_wake_reconciler_test.go` pass. Tests that asserted
  name-based `finalizeDone` behavior are rewritten to assert the state-based
  outcome.

## Verification

```bash
(cd apps/backend && go test ./internal/office/service/... ./internal/office/scheduler/... ./internal/office/routines/... ./internal/orchestrator/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/office/service/event_subscribers.go`
- New `apps/backend/internal/office/service/routine_run_completion.go` and `routine_run_completion_test.go`
- `apps/backend/internal/office/service/routine_run_sync_test.go`

## Dependencies

None.

## Risks

- If the characterization test shows that the state-keyed paths miss a case
  that `finalizeDone` covered (for example, an Office task completed by an
  engine move that publishes `task.moved` before `task.state_changed`), keep
  that wake and move it to the state subscriber. Do not delete it.
- Office's `queueChildrenCompletedRun` and the orchestrator's
  `processOnChildrenCompleted` may produce different payloads. Compare them
  before deleting the Office call.

## Parallelism

`parallel-safe` with Tasks 01 to 03. The files are disjoint.

## Inputs

- System design: "Completion effects".
- `AC-TASKS-COMPLETION-001.9` and `001.10`.

## Results

Moved routine-run close-out to committed `task.state_changed` events and removed
the step-name finalization path. Routine-run, Office scheduler, and orchestrator
completion tests passed.
Follow-up review widened blocker resolution to COMPLETED, FAILED, and
CANCELLED, retaining duplicate-delivery deduplication. The step-name-only test
now confirms that a task.moved event does not trigger completion effects. The
orchestrator records the evaluated child-wave identity in the Office wake
receipt only after destination `on_enter` reports successful delivery. A
failed entry leaves the parent eligible for reconciliation, covered by
`TestProcessOnChildrenCompleted_DoesNotRecordWakeWhenOnEnterDeliveryFails`;
the successful state-path test still confirms the reconciler does not dispatch
the same completion again. Wave identity is checked before persisting the
broader child snapshot used by the reconciler, so a concurrent wave change is
not hidden. A failed operation-ledger read stops processing without writing a
wake receipt; already-applied and non-transition deliveries also leave the
state-path wake receipt untouched. The Task 04 verification command passed for
Office service, scheduler, routines, and the full orchestrator package. Changed
Go lint and `make -C apps/backend build` passed. PostgreSQL verification was
unavailable because `KANDEV_TEST_POSTGRES_DSN` is unset.

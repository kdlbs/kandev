---
id: "01-drop-superseded-deferred-moves"
title: "Drop a deferred move superseded by a later move"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.1
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.2
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.3
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.4
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.5
  - AC-TASKS-WORKFLOW-STAGE-MOVES-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-stage-moves.md
---

# Work Order 01: Drop a deferred move superseded by a later move

## Outcome

A pending move records the step and the latest ledger id that it was
requested from. Turn-end replay and the atomic admission write discard the
move when a later move changed either value. A move with no later step change
applies exactly as before.

## In scope

- `apps/backend/internal/orchestrator/messagequeue/types.go`: add
  `PendingMove.FromStepID` and `PendingMove.FromTransitionID`.
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`:
  DDL columns, idempotent `ADD COLUMN`, and every pending-move select, upsert,
  restore, and take.
- `apps/backend/internal/orchestrator/messagequeue/pending_move_sweep.go`:
  `ListPendingMoves` carries both values.
- `apps/backend/internal/mcp/handlers/config_task_handlers.go` and
  `handlers.go`: `deferMoveTask` records both values. A small optional reader
  interface supplies `GetLatestTaskStepTransitionID`.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`:
  `applyPendingMove` uses the recorded source, extracts the current-target
  consume into a helper, adds the ledger-id check, and drops superseded moves
  through the existing compare-and-delete and prompt correlation.
- `apps/backend/internal/orchestrator/workflow_store.go`: return the sentinel
  `errDeferredMoveSourceChanged` for a rejected atomic write.
- `apps/backend/internal/task/repository/sqlite/task.go` and
  `step_transitions.go`: compare `FromTransitionID` with the latest ledger id
  inside the admission transaction.

## Out of scope

- Caller-initiated cancellation of a pending move.
- The age-based pending-move reaper.
- Any change to deferred-move `on_enter` dispatch identity.

## Acceptance conditions

1. Scenarios A to B (later move) and A to B to A (leave and return) leave the
   task on its current step. The pending row and its hand-off prompt are
   removed.
2. A race in which the later move commits between the replay check and the
   atomic write rejects the write without a task change and drops the move.
3. A deferred move with no later step change, and a legacy row without the new
   values, apply as before.

## Verification

Add focused tests beside the existing pending-move tests
(`event_handlers_pending_move_test.go`, `task_deferred_move_test.go`, and
`messagequeue` repository tests). Run them before the fix (expected to fail)
and after it:

```bash
cd apps/backend
go test ./internal/orchestrator/ -run 'PendingMove|DeferredMove' -count=1
go test ./internal/orchestrator/messagequeue/ -run 'PendingMove' -count=1
go test ./internal/task/repository/sqlite/ -run 'DeferredMove|StepTransition' -count=1
go test ./internal/mcp/handlers/ -run 'MoveTask' -count=1
```

## Likely files

See In scope. Dependencies: none.

## Results

- `applyPendingMove` drops a superseded move through a shared
  `discardPendingMove`, which the existing invalid-target drop now also uses.
- Tests: `TestPendingMove_DropsMoveSupersededByLaterMove`,
  `TestPendingMove_DropsMoveSupersededByLeaveAndReturn`,
  `TestPendingMove_AppliesRecordedMoveWithoutLaterMove`,
  `TestUpdateTaskWithWorkflowStepAdmissionForDeferredMoveRejectsLeaveAndReturn`,
  and `TestDeferMoveTask_RecordsSourceStepAndTransition`. The first two, the
  admission test, and the handler test failed on assertions before the fix and
  pass after it. The moves are built from their persisted JSON shape, so the
  same test source compiles before and after the new fields exist.

- Review remediation uses `BIGINT` for the new transition reference, matching
  the PostgreSQL ledger's 64-bit identity. SQLite and PostgreSQL regression
  tests cover large IDs through upsert, get, list, snapshot, take, restore,
  deletion, legacy upgrade, and schema replay. The isolated static range proof
  rejected the former 32-bit declaration; actual database behavior is verified
  only by hosted Backend CI.
- The deferred-move leave-and-return admission regression now runs against an
  isolated real PostgreSQL schema as well as SQLite.

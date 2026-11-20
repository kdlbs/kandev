---
id: "02-tree-state-events"
title: "Publish tree state transitions"
status: done
wave: 2
depends_on:
  - "01-dashboard-state-events"
plan: "plan.md"
requirements:
  - REQ-TASKS-STATE-CHANGE-EVENTS-001
acceptance_criteria:
  - AC-TASKS-STATE-CHANGE-EVENTS-001.1
  - AC-TASKS-STATE-CHANGE-EVENTS-001.2
  - AC-TASKS-STATE-CHANGE-EVENTS-001.3
  - AC-TASKS-STATE-CHANGE-EVENTS-001.5
  - AC-TASKS-STATE-CHANGE-EVENTS-001.6
  - AC-TASKS-STATE-CHANGE-EVENTS-001.7
  - AC-TASKS-STATE-CHANGE-EVENTS-001.10
system_design:
  - ../../specs/tasks/system-design/state-change-events.md
---

# Task 02: Publish Tree State Transitions

## Summary

Replace the silent tree-cancel state write with a transactional task-writer
batch that returns change receipts. Publish cancellation and restoration
changes through the canonical seam from Task 01.

## In scope

- Add a receipt-returning batch cancellation method to the task writer and wire
  it into the Office tree service. Preserve hold membership and skip markers.
- Commit all selected member states and checkout releases before publishing
  any member event. Keep checkout cleanup for already-cancelled members while
  producing no state receipt for unchanged rows.
- Replace the raw `BulkUpdateTaskState` call and retire that Office repository
  method if no consumer remains. Keep run cancellation and terminal-shape
  recording under their current owners.
- Retain successful receipts through post-commit run-cleanup errors. Publish
  before the tree-hold notification on success.
- Restore each eligible member through the guarded receipt seam. Publish each
  committed restore even if a later member fails. Preserve and log members that
  drifted from `CANCELLED`, then release the hold after all remaining restores
  succeed. A write failure keeps the hold active for retry.
- Let the canonical activity hook create one row per changed member.

## Out of scope

- Hold-graph enumeration redesign or a whole-operation hold transaction.
- New tree controls, blocker semantics, runtime flags, or UI changes.

## Acceptance

1. `TestOfficeTreeStateEvents` first fails for missing events on cancel and
   restore. After correction, a root and child emit truthful changes, while an
   already-cancelled sibling remains excluded. Every cancel observer sees the
   complete committed batch.
2. A failed batch produces no events. A post-commit cleanup error preserves all
   committed notifications. A partial restore preserves notifications for
   successful members and keeps the hold active. A drifted restore member is not
   overwritten or published; later members restore and the hold is released.
3. Pause/resume and repeated cancellation produce no state transition events.
   Checkout release, run cancellation, and terminal-shape tests still pass.

## Verification

Run the complete block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/office/service ./internal/office/repository/sqlite -run 'Test(TaskStateCommit|TaskStateBatchCancel|OfficeTreeStateEvents|CancelAndRestoreTaskTree|CancelTaskTree|PauseTaskTree)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Run new dialect-sensitive batch tests against real PostgreSQL through
`KANDEV_TEST_POSTGRES_DSN`, including competing writes. Record a skipped
infrastructure-dependent case explicitly. Run `bash scripts/lint-go-changed`
with this work order's actual changed Go paths.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task_state_commit.go` and
  `task_state_batch_cancel_test.go` (new test file).
- `apps/backend/internal/office/shared/task_state_writer.go` or a narrow sibling
  batch-writer interface.
- `apps/backend/internal/office/service/tree_controls.go`, `service.go`,
  `tree_controls_test.go`, and `tree_controls_state_events_test.go` (new).
- `apps/backend/internal/office/repository/sqlite/tree_holds.go`.
- `apps/backend/internal/backendapp/main.go`.
- Writer and publisher doubles affected by the required seams.

## Dependencies

Task 01. Reuse its receipt and canonical publisher instead of another serializer.

## Risks

- Publishing before the batch commits can escalate toward a parent that this
  same cancellation will stop.
- Cleanup can change timestamps and create false duplicate failure occurrences
  if overlapping #5277 triggers remain. Task 03 removes that overlap.
- Parent assignment in test setup must use `UpdateTaskParentID` or explicit
  parent-admission intent. A full task snapshot preserves the existing parent.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/state-change-events.md).
- [Design](../../specs/tasks/system-design/state-change-events.md), tree flow
  and failure sections.
- [Existing subtree contract](../../specs/tasks/requirements/subtree-controls.md).
- `office/service/tree_controls_test.go` for hold, run, and skip-marker fixtures.
- `task/repository/sqlite/task_state_commit.go` from Task 01.

## Results

Done. The focused race suite, sqlguard, store conformance, build, and
changed-Go lint pass. PostgreSQL-specific assertions were skipped because the
test DSN is unavailable, as required by the work order.

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/office/service ./internal/office/repository/sqlite -run 'Test(TaskStateCommit|TaskStateBatchCancel|OfficeTreeStateEvents|CancelAndRestoreTaskTree|CancelTaskTree|PauseTaskTree)' -count=1)` passed.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)` passed.
- `(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)` passed.
- Changed-Go lint passed with `0 issues` using the affected package set and a
  15-minute timeout after the repository wrapper's 5-minute timeout expired.
- The PostgreSQL state commit and batch race test was invoked; its assertions
  were skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.

Review-fixup verification:

- The focused Office cancellation and restore regressions passed. The
  `improvekandev` bootstrap test passed after its fixture wired the workflow
  step service required by the production path.
- `make test-backend` initially failed only at that missing test dependency;
  the complete backend check on the fixup head remains the authoritative
  rerun.
- `make fmt-backend`, changed-Go lint across 35 PR files, `make build-backend`,
  spec validation, and spec lint passed.

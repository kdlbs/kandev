---
id: "01-dashboard-state-events"
title: "Publish dashboard state changes once"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-STATE-CHANGE-EVENTS-001
acceptance_criteria:
  - AC-TASKS-STATE-CHANGE-EVENTS-001.1
  - AC-TASKS-STATE-CHANGE-EVENTS-001.2
  - AC-TASKS-STATE-CHANGE-EVENTS-001.3
  - AC-TASKS-STATE-CHANGE-EVENTS-001.4
  - AC-TASKS-STATE-CHANGE-EVENTS-001.7
  - AC-TASKS-STATE-CHANGE-EVENTS-001.9
  - AC-TASKS-STATE-CHANGE-EVENTS-001.10
system_design:
  - ../../specs/tasks/system-design/state-change-events.md
---

# Task 01: Publish Dashboard State Changes Once

## Summary

Publish canonical state events from the shared Office dashboard status path.
Keep authoritative transition evidence and remove its orchestrator's
compensating publication and activity suppression.

## In scope

- Add the receipt-returning task writer and task-owned receipt/write models
  described in the design. Preserve existing raw repository APIs for unrelated
  consumers and reuse transactional completion guards.
- Return the previous state from the writer transaction. Distinguish rejected
  guards from unchanged state. Capture the committed task snapshot before
  releasing the transaction.
- Add the canonical receipt publication seam to the task service and dashboard.
  Its activity-handled option is internal and does not alter event payloads.
- Wire the required publisher in `backendapp`; refuse missing wiring before
  mutation. Update narrow test constructions and writer doubles.
- Cover user, runtime-agent, automatic, and approval-redirect paths. Record
  activity once before canonical and Office refresh notifications.
- Remove `SuppressStatusActivity` and compensating publication/direct parent
  handling from the Office automatic completion caller. Replace the Office
  completion mutex with its existing state-and-step CAS admission.
- Preserve comments and explicit resume behavior on a same-state request.

## Out of scope

- Tree batch mutation, new blocker policy, and public event schema changes.
- Unrelated task writers and generic task-service state refactors.

## Acceptance

1. `TestOfficeStatusStateEvents` first fails for missing canonical events on
   the current implementation. After correction, each changed state produces
   one truthful event and one correctly attributed activity row.
2. Guard rejection, no-op, invalid status, missing task, and completion-gate
   errors produce no canonical transition. A committed redirect reports
   `REVIEW`, retains its typed error, and never reports completion.
3. Concurrent and reentrant automatic completion produces one event without
   deadlock. Post-commit caller cancellation cannot prevent publication.

## Verification

Run the complete block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/office/dashboard ./internal/orchestrator -run 'Test(TaskStateCommit|OfficeStatusStateEvents|UpdateTaskStatus|LogTaskStateChange|MarkOfficeTaskCompleted)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Run dialect-sensitive receipt tests against real PostgreSQL with
`KANDEV_TEST_POSTGRES_DSN` supplied through the normal environment. Include
concurrent writers there. Record unavailable infrastructure explicitly.
Run `bash scripts/lint-go-changed` with the actual changed Go paths before
marking this work order done.

## Files likely touched

- `apps/backend/internal/task/models/task_state_change.go` (new).
- `apps/backend/internal/task/repository/sqlite/task_state_commit.go` and
  `task_state_commit_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/task.go` for shared transaction
  helpers only where necessary.
- `apps/backend/internal/task/service/service_events.go` and a focused test file.
- `apps/backend/internal/office/shared/task_state_writer.go`.
- `apps/backend/internal/office/dashboard/service.go`, `service_tasks.go`,
  `task_status_persistence.go`, and `task_status_state_events_test.go` (new).
- Dashboard writer doubles, activity tests, and lifecycle-publisher doubles.
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`,
  `service.go`, and `office_task_status_test.go`.
- `apps/backend/internal/backendapp/main.go`.

## Dependencies

None. Refresh base/PR evidence before implementation. If #5277 is already
present, retain its policy and record any overlap for Task 03.

## Risks

- Reentrant publication under the Office completion mutex can deadlock.
- Matched/no-op and rejected guard are distinct results.
- A reload after commit cannot supply authoritative transition states.
- Moving activity ownership must preserve actor/run fields and row-before-event
  ordering for every notification.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/state-change-events.md), criteria
  listed in frontmatter.
- [Design](../../specs/tasks/system-design/state-change-events.md), commit
  evidence, canonical publication, dashboard flow, and failure sections.
- `office/service/assignment_blocker_resolution_test.go` supplies the real
  repository/service fixture used in the temporary reproduction.
- `office/dashboard/status_change_activity_test.go` pins synchronous activity.
- `task/service/service_events.go` supplies the immutable publication FIFO.

## Results

Done. The focused race suite, sqlguard, store conformance, build, and
changed-Go lint pass. PostgreSQL-specific assertions were skipped because the
test DSN is unavailable, as required by the work order.

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/office/dashboard ./internal/orchestrator -run 'Test(TaskStateCommit|OfficeStatusStateEvents|UpdateTaskStatus|LogTaskStateChange|MarkOfficeTaskCompleted)' -count=1)` passed.
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)` passed.
- `(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)` passed.
- `make -C apps/backend build` passed.
- Changed-Go lint passed with `0 issues` using the affected package set and a
  15-minute timeout after the repository wrapper's 5-minute timeout expired.
- `TestPostgresTaskStateCommitAndBatchCancel` was invoked; PostgreSQL-specific
  assertions were skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.

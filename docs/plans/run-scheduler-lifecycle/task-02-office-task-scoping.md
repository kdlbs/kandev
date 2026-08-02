---
id: "02-office-task-scoping"
title: "Scope autonomous assignment to Office tasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/run-scheduling.md"
---

# Task 02: Scope autonomous assignment to Office tasks

## Acceptance

- Task and Office repositories share one authoritative Office-task SQL
  predicate matching `Task.IsFromOffice`.
- Office task-created/task-updated subscribers never queue `task_assigned` for
  a Kanban task solely because it has a runner.
- Recovery selects canonical Office-workflow and project-linked tasks across
  multiple workspaces, while excluding ordinary Kanban tasks.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/office/service
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/is_from_office_test.go`
- `apps/backend/internal/office/repository/sqlite/tasks.go`
- `apps/backend/internal/office/repository/sqlite/tasks_test.go`
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/service/event_subscribers_test.go`
- `apps/backend/internal/office/service/scheduler_recovery_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` relative to Task 01 only: the file sets are disjoint and no
schema, package configuration, or generated contract changes.

## Inputs

- Spec: autonomous assignment and Office identity scenarios.
- Plan: Authoritative Office-task scoping.
- Existing projection: `task/repository/sqlite.isFromOfficeProjection`.

## Risks

- Project-linked Office tasks may use a non-canonical workflow and must remain
  included.
- Explicit engine `queue_run` actions for custom workflows must remain
  unaffected.

## Output contract

Report the scoping behavior, files changed, test command/result, blockers,
remaining risks, and update this task plus `plan.md` status in the same
conversation.

## Result

- Exported the task repository's Office identity predicate and reused it in
  Office execution-field and recovery queries.
- Task-created/task-updated assignment subscribers now ignore ordinary Kanban
  tasks even when they have a runner; project-linked and canonical
  Office-workflow tasks remain eligible.
- Added SQLite and event-subscriber coverage for both Office identity forms,
  Kanban exclusion, and multi-workspace recovery selection.
- Verification: `cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/office/service -count=1` passed.

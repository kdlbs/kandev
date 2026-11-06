---
id: "01-relation-storage-and-creation"
title: "Add task_relations storage and wire it into task creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-004
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.2
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.3
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.4
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.2
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.3
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.4
system_design:
  - ../../specs/tasks/system-design/task-create-relationships.md
---

# Task 01: Add task_relations storage and wire it into task creation

## Summary

Add the `task_relations` table and `RelationRepository`, and extend
`Service.CreateTask`, `create_task_references.go`, `BuildDependencyViews`, and
`create_task_kandev` to accept, validate, store, and roll back a related-task
list, per [ADR-2026-11-05](../../decisions/2026-11-05-task-related-to-storage.md).

## In scope

- `task_relations` table (`CREATE TABLE IF NOT EXISTS`, normalized
  `task_id_low < task_id_high` key) in
  `internal/office/repository/sqlite/base.go`.
- `RelationModel` in `internal/office/models/models.go`; `RelationRepository`
  interface in `internal/task/service/service_office.go`; its sqlite
  implementation in `internal/office/repository/sqlite/relations.go`.
- `Service.CreateTask`'s `RelatedTaskIDs []string` field;
  `validateRelationReferences` in `create_task_references.go`; the
  `finalizeCreatedTask` relation-write loop; the `rollbackPartialTask`
  relation cleanup.
- `BuildDependencyViews`'s new `Related` field, populated from
  `ListRelationsForTasks`.
- `create_task_kandev`'s `related_task_ids` parameter and pass-through.
- Task deletion's existing cleanup pass gains `RemoveRelationsForTask`.

## Out of scope

- The task-graph read's `kind` discriminator (task 02).
- Any frontend change.

## Acceptance

- Inserting a relation in either task order produces exactly one
  `task_relations` row, and both tasks report each other via
  `ListRelationsForTask`.
- `Service.CreateTask` with `RelatedTaskIDs` containing an archived or unknown
  task ID fails with the same validation-error shape `BlockedBy` already uses,
  and creates no task row.
- `create_task_kandev` with `related_task_ids` creates the relation; omitting
  the parameter changes no existing behavior.

## TDD sequence

1. Failing repository tests: insert-order symmetry, duplicate no-op,
   `ListRelationsForTasks` batch read, `RemoveRelationsForTask`.
2. Failing service tests: `validateRelationReferences` rejecting an archived
   or unknown ID; `finalizeCreatedTask` writing relations;
   `rollbackPartialTask` removing them on a simulated failure.
3. Failing `BuildDependencyViews` test asserting `Related` population.
4. Failing MCP handler test for `related_task_ids` validation and
   pass-through.
5. Implement, then refactor with tests green.

## Verification

```bash
cd apps/backend && go test ./internal/office/repository/sqlite -run 'Relation' -count=1
cd apps/backend && go test ./internal/task/service -run 'Relation|CreateTask' -count=1
cd apps/backend && go test ./internal/mcp/server -run 'CreateTask' -count=1
make -C apps/backend test
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/base.go`
- `apps/backend/internal/office/repository/sqlite/relations.go`
- `apps/backend/internal/office/repository/sqlite/relations_test.go`
- `apps/backend/internal/office/models/models.go`
- `apps/backend/internal/task/service/service_office.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/create_task_references.go`
- `apps/backend/internal/task/service/service_dependencies.go`
- `apps/backend/internal/mcp/server/server.go`

## Dependencies

None.

## Risks

- `finalizeCreatedTask` already does one-edge-at-a-time writes for blockers;
  adding a second loop must keep `rollbackPartialTask` covering both, or a
  mid-failure task can be created with a dangling relation or blocker.
- `related_task_ids` validation must reuse the exact lookup
  `validateBlockerReferences` uses (same workspace, non-archived), not a
  slightly different rule that could accept a cross-workspace ID.

## Parallelism

`sequential`

## Results

Added the normalized `task_relations` table and repository, validation and rollback wiring for task creation, the batched `related` projection, and `related_task_ids` in `create_task_kandev`. Task deletion and create compensation clear relation rows.

Validation results:

- `go test ./internal/office/repository/sqlite -run Relation -count=1`: passed.
- `go test ./internal/task/service -run 'Relation|CreateTask|TaskGraph' -count=1`: passed.
- `go test ./internal/mcp/server -run 'CreateTask' -count=1`: passed.
- `make -C apps/backend test`: failed in the full run. Existing failures included fixed-port collisions and launcher version assertions; `internal/task/repository/sqlite` also reached its 600-second timeout. Focused affected-package tests passed.
- `bash scripts/lint-go-changed` on the relation and graph service files: initially found an unchecked relation setup error in a service test; checked the error and reran successfully with 0 issues.
- `git diff --check`: passed.

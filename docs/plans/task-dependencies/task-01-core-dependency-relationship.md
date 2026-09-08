---
id: "01-core-dependency-relationship"
title: "Promote dependency edges to a core relationship"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 01: Promote Dependency Edges to a Core Relationship

## Acceptance

- The dependency edge store is reachable in a Kanban-only install:
  `SetBlockerRepository` is wired before the `Features.Office` early return in
  `internal/backendapp/main.go`, so `create_task` with `blocked_by` succeeds and
  `list_related_tasks_kandev` populates `blockers` / `blocked_by` with
  `Features.Office` false.
- Exactly one edge validator exists. Self-edge, cross-workspace, and BFS cycle
  detection (with the existing walk limit and `*BlockerCycleError{Path}`) live in
  the task domain, and both the new task-scoped routes and the existing Office
  routes call it. `checkCircularBlocker` in `service_office.go` is deleted, not
  left as a second path.
- A `(blocker_task_id)` index on `task_blockers` is added through an idempotent
  migration in `internal/task/repository/sqlite/base_migrations.go`.
- **Deleting** a task removes its edges in both directions, on SQLite and on
  PostgreSQL, via explicit repository work rather than an `ON DELETE CASCADE`
  the table does not have. **Archiving** does not: an archived predecessor still
  blocks its dependents and reads as `pending` (Task 04), so its edges survive
  and reappear if the task is unarchived.
- One batch service helper resolves, for a list of task IDs, each task's direct
  predecessors **and direct dependents**, and a per-predecessor verdict of
  `resolved` / `failed` / `pending`, using `state = COMPLETED` or
  `IsTerminalStepName` for resolved and `FAILED` / `CANCELLED` for failed.
  Archived predecessors are `pending`. The reverse direction uses the
  `(blocker_task_id)` index added above; it is one batched query, not one per
  task.
- Task DTOs over HTTP, WebSocket boot, WebSocket events, and MCP carry
  `blocked`, `blocked_reason`, `depends_on`, `blocks`, and
  `start_when_unblocked`. No `is_blocked` column is persisted.
- `depends_on` and `blocks` entries carry id, title, and state so the dependency
  chip renders without a per-entry fetch. Neither list is transitive.
- New task-scoped routes exist and behave as specified:
  `POST /api/v1/tasks/:id/dependencies`,
  `DELETE /api/v1/tasks/:id/dependencies/:depId`. `POST` returns `409` with a
  `cycle` array on a cycle and `400` on a self-edge or cross-workspace edge;
  `DELETE` of an absent edge succeeds. Both return the mutated task's dependency
  projection so a caller does not re-fetch. There is no graph-wide listing
  route: every reader gets its edges from the dependency fields already on the
  task payload.
- `task.updated` with `fields: ["dependencies"]` is published on edge add and
  remove, through the task event publisher (not a bare repository write).
- `POST /api/v1/tasks` and `create_task_kandev` accept `start_when_unblocked`;
  when true **and the request declares at least one dependency**, the resolved
  launch inputs are persisted as `metadata.deferred_launch` in the same atomic
  boundary as the task row and no session, workspace, or executor is prepared.
  `start_when_unblocked: true` with no dependencies records no intent: the task
  is already unblocked, so an agent-start request on it launches immediately
  exactly as it would without the flag, and a create with neither launches
  nothing. The flag defers a start; it never invents one.
- Edge routes authorize both tasks with the existing `authorize*` helpers, and
  the payload field names stay `task_id` / `session_id` compatible with the
  gateway dispatch backstop.

## TDD sequence

1. Failing test: with `Features.Office` false, `CreateTask` with `blocked_by`
   succeeds and `list_related_tasks_kandev` returns the edge.
2. Failing tests for the shared validator: cycles of length 2, 3, and N return
   `*BlockerCycleError` with the full path; self-edge and cross-workspace return
   the typed errors; a duplicate edge is idempotent; both the task-scoped and
   Office entry points hit the same validator.
3. Failing tests for the batch derived-state helper across pending,
   resolved-by-state, resolved-by-terminal-step, failed, cancelled, and archived
   predecessors, asserting the batch result equals the per-task result, and that
   the dependents (`blocks`) direction is returned for the same call.
4. Failing DTO/event tests for the five new fields on HTTP, boot, WS, and MCP,
   including the id/title/state shape of `depends_on` and `blocks` entries.
5. Failing handler tests for the three routes including the `409` `cycle` body
   shape and the authorization denials.
6. Failing tests for edge cleanup on task delete, for edge *survival* across
   archive, and for zero-dependency `start_when_unblocked`, plus the
   environment-gated PostgreSQL equivalents.
7. Implement: wiring move, migration/index, validator consolidation, batch
   helper, DTO fields, routes, event publication, `start_when_unblocked`
   persistence.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/... ./internal/office/dashboard ./internal/mcp/... ./internal/backendapp -run 'Test.*(Dependenc|Blocker|BlockedBy|StartWhenUnblocked)' -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/service/service_office.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/dto/dto.go`, `requests.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/pkg/api/v1/task.go`
- focused service, repository, handler, DTO, and MCP tests

## Dependencies

None.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record where the single validator now lives, the exact
migration/index statements, the batch helper's query shape, and test results in
this file and `plan.md`.

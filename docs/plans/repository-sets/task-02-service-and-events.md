---
id: "02-service-and-events"
title: "Repository set service, validation, and events"
status: done
wave: 2
depends_on: ["01-persistence-contracts"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 02: Repository Set Service, Validation, And Events

## Acceptance

- `CreateRepositorySet`, `GetRepositorySet`, `ListRepositorySets`, `UpdateRepositorySet`, and
  `DeleteRepositorySet` enforce workspace access on every call, resolving the set's workspace first
  for item operations, and validate the whole request before any write: trimmed 1-to-100-character
  name, case-insensitive per-workspace name uniqueness, non-empty `repository_ids`, no duplicate ids,
  and every id resolving to a live repository in the same workspace. Each failure returns a distinct
  typed error so the handler can map `400` / `404` / `409` / `422`, and the conflict error names the
  existing set.
- Name, description, and membership from one update commit together; a supplied `repository_ids`
  replaces the whole list, which is also how reordering is expressed.
- Create, update, and delete publish `repository_set.created|updated|deleted` with a payload carrying
  `id` and `workspace_id`, and the gateway bridges each to its WebSocket action so the existing
  workspace routing delivers it without a new routing branch.

## Verification

```bash
cd apps/backend
go test ./internal/task/service/... ./internal/events/... ./internal/gateway/websocket/...
```

## Files likely touched

- `apps/backend/internal/task/service/service_repository_sets.go` (new)
- `apps/backend/internal/task/service/service_repository_sets_test.go` (new)
- `apps/backend/internal/task/service/service_events.go` (new publisher beside
  `publishRepositoryEvent` `:977`)
- `apps/backend/internal/task/service/service.go` (store wiring)
- `apps/backend/internal/events/types.go` (beside the repository events `:125-127`)
- `apps/backend/pkg/websocket/actions.go` (beside `ActionRepository*` `:233`)
- `apps/backend/internal/gateway/websocket/task_notifications.go` (`b.subscribe` block `:60-62`)

## Dependencies

Task 01 for the store interface and models.

## Inputs

- Spec: API surface, Permissions, Failure modes, Persistence guarantees.
- Patterns: `service_resources.go` repository CRUD (`CreateRepository:735`, `UpdateRepository:1051`)
  for authorization and logging shape; `service_test.go:248` for asserting published event types.
- Placement: a new service file, not `service_resources.go`, which is already large.
- Do not reveal whether a cross-workspace repository or set id exists: unknown-workspace and
  no-access return the same result.

## Risks

- A per-workspace case-insensitive uniqueness check done outside the write transaction can race two
  concurrent creates; the database `UNIQUE(workspace_id, name)` must be handled as a conflict rather
  than a 500.
- Publishing before the write commits would broadcast a set that does not exist; publish after.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.

---
id: "01-backend-workflow-memory"
title: "Persist per-workspace workflow memory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-workflow-memory.md"
---

# Task 01: Persist Per-Workspace Workflow Memory

## Intent

Extend the backend-owned task-create preference so every successful HTTP or
WebSocket task creation records its effective workflow under the task's
workspace without clobbering another workspace's history.

## Acceptance

- `task_create_last_used.workflow_ids_by_workspace` round-trips through the
  model, user-settings response/event, and boot payload; missing maps remain
  backward compatible.
- SQLite and PostgreSQL patch builders update individual workspace entries and
  preserve other workspace entries plus repository, branch, agent, and executor
  preferences.
- Successful HTTP and WebSocket task creation record the request's workspace
  and effective workflow, while preference-write failure remains non-fatal to
  task creation.

## TDD sequence

1. Extend handler recorder tests for HTTP and WebSocket creation and confirm RED
   because workflow history is absent.
2. Add SQLite repository and PostgreSQL query-builder regressions that preserve
   two workspace entries across partial updates; confirm RED before store
   changes.
3. Add the model, service emptiness rule, targeted JSON patches, transport
   wiring, event mapping, and boot mapping.
4. Run the focused backend packages and keep every existing last-used field
   regression green.

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_routes_test.go`

## Dependencies

None.

## Parallelism

`sequential` — this task owns the shared persisted JSON contract consumed by
all later tasks.

## Inputs

- Spec sections `Data model`, `Persistence guarantees`, and the HTTP/WebSocket
  recording scenarios.
- ADR 0028, ADR 0041, and
  ADR-2026-08-08-workspace-scoped-task-create-workflow-memory.
- Existing `makeTaskCreateLastUsedJSONSetArgs`,
  `applyPostgresTaskCreateLastUsedPatch`, `buildTaskCreateLastUsedPatch`, and
  `mapTaskCreateLastUsed` patterns.

## Verification

- `cd apps/backend && go test ./internal/user/store ./internal/user/service ./internal/task/handlers ./internal/backendapp`

## Risks

- Workspace IDs must be represented through safe JSON paths rather than raw SQL
  interpolation.
- A whole-map replacement would pass a one-workspace test while losing history
  during alternating workspace writes; the regression must assert two keys.

## Output contract

Report RED evidence, changed files, focused Go test results/counts, SQLite and
PostgreSQL path coverage, blockers, risks, and synchronize this task plus
`plan.md` status/results in the primary conversation.

## Results

Implemented the workspace-scoped workflow map in the existing user-settings
JSON contract. HTTP and WebSocket task creation now record the effective
workspace/workflow pair after successful creation; SQLite and PostgreSQL
updates patch individual map entries while preserving existing preferences and
other workspace history. Boot-state and service/event mappings normalize
missing maps for backwards compatibility.

TDD RED evidence included handler recorder assertions failing with a missing
workflow map before transport wiring, followed by the store/service compile
adjustments required by the new map field. Final verification:

`cd apps/backend && go test ./internal/user/store ./internal/user/service ./internal/task/handlers ./internal/backendapp`

All four packages passed, including real SQLite preservation coverage and
parameterized PostgreSQL query-builder coverage.

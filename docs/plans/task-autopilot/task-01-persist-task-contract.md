---
id: "01-persist-task-contract"
title: "Persist the autopilot task contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/autopilot-mode.md"
---

# Task 01: Persist the Autopilot Task Contract

## Acceptance

- Existing and newly created tasks round-trip a non-null `autopilot` boolean, with
  omission and migrated rows resolving to false.
- HTTP and `create_task_kandev` creation accept the same optional field, do not
  inherit it from a parent, and reject incompatible agent runtime profiles before
  persistence.
- The MCP schema uses the exact short description: "Start this task in autopilot
  mode. Default: false. The value is fixed at creation and is not inherited by
  subtasks. The agent does not ask the user directly; it asks its direct parent only
  for critical decisions." At runtime, a root autopilot task has no question
  capability, while an autopilot child has only the direct-parent question tool.
- Update/edit APIs cannot mutate the property, and task read/list/boot payloads
  expose it consistently to clients.

## Verification

```bash
cd apps/backend && go test ./internal/task/models ./internal/task/dto ./internal/task/repository/sqlite ./internal/task/handlers ./internal/mcp/server ./internal/mcp/handlers
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/requests.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- Related model, repository, handler, and tool-schema tests

## Dependencies

None.

## Parallelism

This establishes the shared persisted/API contract. Runtime and UI work start only
after its field name, default, and compatibility behavior are fixed.

## Inputs

- Spec sections `Creation API`, `Persistence and restart`, and `Permissions and boundaries`.
- Existing task schema migrations, `TaskDTO` conversion, HTTP create handler, and
  MCP `registerCreateTaskTool`/`handleCreateTask` paths.

## Output contract

Report the migration number and SQL, Go/JSON field names, compatibility default,
profile-validation boundary, exact creation/read paths changed, tests run, and any
runtime profile type that remains unsupported.

## Results

Done. Added the immutable SQLite-backed field with migration/default, HTTP and MCP
create propagation, DTO/WS/boot read paths, exact MCP parameter text, and tests for
round trips, omitted values, nested creation, and update immutability.

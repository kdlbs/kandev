---
id: "01-durable-source-contracts"
title: "Durable source contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 01: Durable Source Contracts

## Acceptance

- `task_workspace_folders` persists canonical task-owned folder attachments with replay-safe schema,
  uniqueness, cascade deletion, and indexed ordering in SQLite and Postgres-compatible schema paths.
- Task/API/event models expose `workspace_folders` while existing repository payloads remain backward
  compatible.
- Repository methods can create and compensate one attachment batch transactionally, and tests pin
  path/name uniqueness plus mixed source position allocation.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/repository/... ./internal/task/dto/... ./pkg/api/v1/...
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/workspace_folder.go` (new)
- `apps/backend/internal/task/repository/**/*workspace_folder*_test.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/pkg/api/v1/task.go`
- `apps/backend/internal/task/service/service_events.go`

## Dependencies

None.

## Inputs

- Spec: Data model, Persistence guarantees.
- ADR: preserve `task_repositories`; folders use a separate relation.
- Patterns: `sqlite/task_repository.go`, replayable schema tests, task event repository projection.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.

---
id: "01-persistence-contracts"
title: "Repository set persistence contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 01: Repository Set Persistence Contracts

## Acceptance

- `repository_sets` and `repository_set_items` exist in both the base schema and the replayable
  migrations, with workspace and set cascade deletion, `UNIQUE(workspace_id, name)`,
  `UNIQUE(repository_set_id, repository_id)`, and workspace/set lookup indexes. Initializing the
  schema twice is a no-op.
- Store methods create, read, list, update, and delete sets, and `ReplaceRepositorySetItems` rewrites
  a whole membership list transactionally with contiguous zero-based positions in the requested
  order. Reads exclude members whose repository is soft-deleted or belongs to another workspace, and
  list resolves items for all returned sets without a per-set query.
- Soft-deleting a repository removes its `repository_set_items` rows inside the same transaction that
  already removes `repository_secret_bindings`, for both delete paths.

## Verification

```bash
cd apps/backend
go test ./internal/task/repository/... ./internal/task/dto/...
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go` (new `initRepositorySetsSchema` step
  appended to the `steps` slice at `:16-48`)
- `apps/backend/internal/task/repository/sqlite/base_migrations.go` (`runMigrations` at `:124`)
- `apps/backend/internal/task/repository/sqlite/repository_set.go` (new)
- `apps/backend/internal/task/repository/sqlite/repository_entity.go` (`DeleteRepository` `:143`,
  `DeleteRepositoryIfNoActiveTaskSessions` `:199`)
- `apps/backend/internal/task/repository/sqlite/repository_set_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/schema_replay_test.go`,
  `postgres_schema_test.go`
- `apps/backend/internal/task/repository/interface.go` (beside `RepositoryEntityRepository` `:370`)
- `apps/backend/internal/task/models/models.go` (beside `Repository` `:1426`)
- `apps/backend/internal/task/dto/dto.go` (beside `RepositoryDTO` `:48`, `FromRepository` `:605`)

## Dependencies

None.

## Inputs

- Spec: Data model, Persistence guarantees.
- Patterns: `sqlite/task_repository.go` for the membership table shape,
  `office/repository/sqlite/projects_test.go:16-70` for the full-column fixture and assertion style,
  `db/migratelog.go:33` for idempotent migration naming (`"repository_sets.table"`).
- Constraint: repositories are soft-deleted (`repositories.deleted_at`), so the foreign-key cascade
  does not clean memberships. Both the delete-path cleanup and the read filter are required.
- Portability: use `r.db.Rebind(...)`, keep the SQL Postgres-safe, and add the tables to the
  Postgres schema test.

## Risks

- A table-rebuild migration added later would drop these tables' columns if it precedes them; keep
  the new `Apply` calls after the existing rebuild migrations in `runMigrations()`.
- Positions must stay contiguous after a replace, or the picker's apply order silently changes.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.

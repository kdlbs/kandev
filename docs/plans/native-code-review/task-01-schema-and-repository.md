---
id: "01-schema-and-repository"
title: "Review run/finding schema, models, and repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 01: Review run/finding schema, models, and repository

Persist review runs and findings. No service, no API yet.

## Inputs

- Spec **Data model** and **Persistence guarantees**.
- `apps/backend/AGENTS.md` → "Schema & migrations": table DDL in `init*Schema`, every index and anything referencing a new column in `runMigrations()`.
- Pattern: `internal/task/repository/sqlite/walkthrough.go` (JSON column, upsert, nil-on-missing) and its `walkthrough_test.go`.

## Work

1. `internal/task/models/models.go` — `TaskReviewRun`, `TaskReviewFinding`, enums `ReviewRunStatus` (`pending|running|completed|failed|cancelled`), `ReviewRunTrigger` (`manual|workflow_step|agent`), `ReviewSeverity` (`blocker|major|minor|nit`), `ReviewFindingStatus` (`open|resolved|dismissed`), sentinels `ErrTaskReviewRunNotFound`, `ErrTaskReviewFindingNotFound`.
2. `internal/task/repository/sqlite/base_schema.go` — `initTaskReviewSchema()` with both `CREATE TABLE IF NOT EXISTS` statements, registered in `initSchema()`.
3. `internal/task/repository/sqlite/base_migrations.go` — the four `CREATE INDEX IF NOT EXISTS` statements.
4. `internal/task/repository/sqlite/review.go` — the repository methods listed in the plan's "Schema & models" section. `CreateTaskReviewFindings` inserts in one transaction. `DeleteSupersededTaskReviewFindings` deletes `open` rows from *other* runs matching `(repository_name, file_path, start_line, end_line, title)`. `CancelInFlightTaskReviewRuns` sets `pending`/`running` → `cancelled` with `error_message = "interrupted by restart"`, called from the same boot path that runs other recovery sweeps.

## Acceptance

- Fresh DB and replayed existing DB both reach the new schema without error.
- Deleting a task deletes its runs and findings.
- Supersede deletes only matching `open` rows from other runs.

## Verification

```
cd apps/backend && go test ./internal/task/repository/sqlite/... -run 'Review'
cd apps/backend && go build ./...
```

## Files likely touched

`internal/task/models/models.go`, `internal/task/repository/sqlite/{base_schema.go,base_migrations.go,review.go,review_test.go}`, the repository interface in `internal/task/repository/`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, and `status: done` here plus the Wave 1 checkbox in `plan.md`.

---
id: "02-state-contracts"
title: "Task language state contract"
status: pending
wave: 2
depends_on: ["01-acceptance-harness"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 02: Task Language State Contract

## Acceptance

- `task_lsp_languages` exactly represents the spec's task/language policy, detection, lifecycle,
  generation/revision, timestamps, reason/initiator, restart-required overlay, and error contract
  with a composite primary key and task cascade.
- Store transitions atomically increment revision; generation allocation is monotonic; absent rows
  synthesize Inherit/off defaults without creating session/browser ownership.
- Fresh SQLite, replayed SQLite, and env-gated Postgres schemas are idempotent and preserve existing
  task rows.

## TDD sequence

1. Add failing store/schema tests for fresh creation, composite uniqueness, policy/evidence
   round-trip, revision compare/update, generation allocation, archive retention, delete cascade,
   and replay.
2. Add the task LSP enums/snapshot types and narrow `Store` interface under `internal/lsp`.
3. Add fresh schema, replayable migration, and task-repository implementation using rebind-safe SQL.
4. Run the same migration path twice on SQLite and Postgres fixtures and refactor only after every
   focused contract is green.

## Verification

```bash
cd apps/backend && go test ./internal/lsp/... ./internal/task/repository/sqlite -run 'Test(TaskLSP|TaskLsp|PostgresTaskLSP|PostgresTaskLsp|SchemaReplay)'
cd apps/backend && go test ./internal/lsp/... ./internal/task/repository/sqlite
```

When `KANDEV_TEST_POSTGRES_DSN` is available, record the Postgres replay test separately rather than
treating an env-gated skip as passing evidence.

## Files likely touched

- `apps/backend/internal/lsp/models.go`
- `apps/backend/internal/lsp/store.go`
- `apps/backend/internal/lsp/models_test.go`
- `apps/backend/internal/task/repository/sqlite/task_lsp.go`
- `apps/backend/internal/task/repository/sqlite/task_lsp_test.go`
- `apps/backend/internal/task/repository/sqlite/task_lsp_postgres_test.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/schema_replay_test.go`

## Dependencies

Task 01 fixes the externally observable contract that these types support.

## Parallelism

Sequential. Later backend tasks consume these shared models, schema, and store methods.

## Inputs

- Spec: Data model; Persistence guarantees; task cleanup scenarios.
- ADR 0027 replayable schema rules and existing task-status-summary schema/replay tests.
- Existing task repository rebind conventions for SQLite/Postgres.

## Output contract

Report schema and API types, exact migration/replay results for each database, tests/counts, and any
compatibility limitation. Reconcile actual files and update task/plan status.

## Results

Pending.

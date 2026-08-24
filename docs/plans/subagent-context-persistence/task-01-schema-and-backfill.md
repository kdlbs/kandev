---
id: "01-schema-and-backfill"
title: "Schema, backfill migration, activation keys"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/subagent-context-persistence.md"
---

> **Amendment 1 update:** the shipped schema and migration differ from this
> task as originally drafted — see the "Amendment 1 update" note near the top
> of `plan.md` for the current uniqueness key, the execution-identity
> migration, and the atomic backfill transaction.

# Task 01: Schema, backfill migration, activation keys

Create the `task_session_subagents` table, the one-time backfill from existing
message metadata, and the two write-once `kandev_meta` activation keys.

## Acceptance

1. A fresh database and an existing database both end up with the table, its
   three indexes, the `UNIQUE (task_session_id, tool_call_id)` constraint, and
   the `task_sessions` cascade FK — and no FK on `turn_id`. `runMigrations()`
   replays cleanly twice with no schema or row-set change. (AC-17, AC-18)
2. The backfill inserts one `source = 'backfill'` row per existing
   `subagent_task` message, deriving NULL (never `0`, never `''`) for every
   unreported or empty field, skipping rows with no `task_id` or no
   `tool_call_id`, and leaving any pre-existing row untouched. (AC-21, AC-22,
   AC-23, AC-23a, AC-2)
3. `subagent_context_capture_since` and `subagent_context_backfill_through` are
   written once and never overwritten on replay. (AC-24)

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go` — new
  `initSubagentContextSchema()`, called from `initSessionSchema()` right after
  `initMessageTurnSchema()`.
- `apps/backend/internal/task/repository/sqlite/base_migrations.go` — new
  `migrateSubagentContextBackfill()` called from `runMigrations()` after the
  existing statements; new `jsonKey`, `jsonInt`, `jsonBoolToInt` helpers beside
  `jsonColumn` / `jsonText` (currently lines 319-335).
- `apps/backend/internal/task/repository/sqlite/subagent_context_migration_test.go`
  — new.

## Dependencies

None.

## Parallelism

`sequential` — owns shared schema DDL and the migration runner.

## Inputs

- Spec § *Data model*, § *Column-level normalization rules*, § *Migration*,
  § *Backfill*, § *Activation point and provenance*.
- Plan § *Schema (task 01)* and § *Migration: backfill and activation keys*,
  which carry the full DDL, the derivation table, and the resolution of the
  spec's "DDL *and* migration" wording. **Do not duplicate the `CREATE TABLE`
  into `base_migrations.go`** — read that section before starting.
- Pattern to copy: `task_external_id_migration_test.go` (seed a pre-migration
  row, call `runMigrations` twice, assert idempotence).
- `internal/db/migratelog.go:33` — `MigrateLogger.Apply` swallows errors and logs
  `WARN` with the migration name. It takes **no bind args**, so every statement
  in this task must be argument-free SQL; inline the `capture_since` timestamp
  with `fmt.Sprintf` over `time.Now().UTC().Format(time.RFC3339)`.
- `internal/persistence/provider.go:72` — `ensureMetaTable` runs before the task
  repo is built, so `kandev_meta` is guaranteed present.
- ADR `docs/decisions/0027-replayable-schema-migrations.md` — use `internal/db`
  error classification, never local error-string matching.

## Verification

```
cd apps/backend && gofmt -l internal/task/repository/sqlite && make lint && go test -run 'TestSubagentContext' ./internal/task/repository/sqlite/... && go test ./internal/task/repository/sqlite/...
```

The final unscoped run is required: it re-runs `TestSQLiteSchemaReinitializes`
and the existing migration suite against the new DDL.

## Results

Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries
when applicable, or explicitly state `None`.

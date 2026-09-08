---
id: "03-ledger-schema-migration"
title: "Ledger schema and migration"
status: done
wave: 3
depends_on: ["01-telemetry-activation-registry", "02-turn-start-step-stamp"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 03: Ledger schema and migration

Create `task_step_transitions` idempotently on SQLite and Postgres, register its
contract, and prove the replay behaviour. No writer yet — this task ends with an
empty table that boots cleanly three times in a row.

**Gated.** Do not start until Slice 1 has shipped and been measured per the
spec's *Gate between slices*. Record the measurement (share of post-activation
turns carrying the stamp, and the resulting change in the 47.0 / 30.5 / 22.5
attribution split) in this task's `## Results` before proceeding. If the
measurement is inconclusive, the recorded default is that Slice 2 proceeds as
specified — note that and continue.

## Acceptance

1. `task_step_transitions` exists with the spec's exact columns, both indexes,
   a cascade FK to `tasks`, a set-null FK to `task_sessions`, and **no** FK to
   `workflows` or `workflow_steps`.
2. A database created before this feature gains the table on boot; pre-existing
   tasks have no rows and nothing is backfilled.
3. Running the migration runner twice more succeeds both times and changes
   nothing, on SQLite and on Postgres.

## Verification

```
cd apps/backend && go test ./internal/task/repository/sqlite/... ./internal/telemetrycontract/... && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN}" go test -run 'Postgres|StepTransition' ./internal/task/repository/sqlite/... && make lint
```

Record in `## Results` whether the Postgres leg ran or skipped.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go` — new
  `initStepTransitionsSchema`, appended to the `initSchema` step list after
  `initSessionSchema` and before `runMigrations`
- `apps/backend/internal/task/repository/sqlite/step_transitions_migration_test.go`
  (new)
- `apps/backend/internal/telemetrycontract/contract.go` — register
  `task_step_transitions` at version 1 with its existence and stats queries

## Dependencies

Tasks 01 and 02, plus the Slice 1 measurement gate.

## Parallelism

`sequential`.

## Inputs

- Spec, *`task_step_transitions` (new)*, *Persistence guarantees*, *Migration
  and activation*, and *Scenarios → Migration and activation*
- Plan, *Area 3*
- `internal/workflow/repository/sqlite.go:88-96` is the working precedent for
  the dialect-split monotonic `id` (`INTEGER PRIMARY KEY AUTOINCREMENT` on
  SQLite, `BIGSERIAL PRIMARY KEY` on Postgres). Copy that shape.
- This is a new **table**, so `CREATE TABLE IF NOT EXISTS` in the init block is
  correct and complete. The backend guide's "columns only via `runMigrations`"
  rule applies to adding a column to an existing table and does not apply here.
- **Deliberately no FK to `workflow_steps` or `workflows`.** Steps get deleted,
  and the historical fact that a card was in a now-deleted step must survive
  that deletion. A reviewer may read the missing FK as an oversight; the DDL
  should carry a comment saying it is not.
- Nothing is backfilled with a substituted value. Pre-activation state is
  absent, and absent is the correct reading.
- Model the test on `task_external_id_migration_test.go`: seed a pre-migration
  row, run migrations, assert the pre-existing row is untouched and the new
  objects exist, then call `NewWithDB` **twice more** and assert idempotence.
- Use `internal/db`'s `IsDuplicateColumnError` / `IsAlreadyExistsError` for any
  replay classification, never local string matching (ADR 0027).

## Output contract

Summary, files changed, tests run with counts, the Slice 1 gate measurement,
blockers, risks, and a status update to this file and `plan.md` in the same
conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).

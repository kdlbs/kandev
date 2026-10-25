---
id: "01-telemetry-activation-registry"
title: "Telemetry contract activation registry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-task-step-transition-ledger.md"
---

# Task 01: Telemetry contract activation registry

Create `telemetry_activations` and the boot-time activation write plus health
report. Both slices depend on this: the downstream extract is a point-in-time
snapshot with no schema versioning, so a column whose meaning changes mid-series
is silently discontinuous unless its activation point is readable from the
snapshot itself.

Register **one** contract in this task: `turn.workflow_step_id_at_start` at
version 1, which Task 02 then makes true. `task_step_transitions` is registered
by Task 03, after the gate. Registering a contract whose backing objects do not
exist yet is exactly what the health line's existence column is for, but do not
register a contract this repository does not yet write.

## Acceptance

1. `telemetry_activations` exists on SQLite and Postgres with primary key
   `(contract_key, contract_version)`, so a future version bump appends a row
   rather than overwriting the first activation.
2. The first boot after this lands writes one row for
   `turn.workflow_step_id_at_start` at version 1 with that boot's UTC time; every
   later boot leaves it byte-identical.
3. Boot emits one health line per registered contract carrying object existence,
   activation timestamp, row count, and most recent occurred_at.

## Verification

```
cd apps/backend && go test ./internal/telemetrycontract/... ./internal/backendapp/... && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN}" go test -run 'Postgres|Activation' ./internal/telemetrycontract/... && make lint
```

The Postgres leg is env-gated and skips when `KANDEV_TEST_POSTGRES_DSN` is
unset; record in `## Results` whether it ran or skipped rather than reporting a
pass it did not perform.

## Files likely touched

- `apps/backend/internal/telemetrycontract/contract.go` (new) — `Contract`
  struct and `Registry()`
- `apps/backend/internal/telemetrycontract/store.go` (new) — DDL, `NewWithDB`,
  `Activate`, `LogHealth`
- `apps/backend/internal/telemetrycontract/store_test.go` (new)
- `apps/backend/internal/telemetrycontract/health_test.go` (new)
- `apps/backend/internal/backendapp/storage.go` — call `Activate` then
  `LogHealth` immediately before the existing `recordSchemaVersion(...)` at
  line 101

## Dependencies

None.

## Parallelism

`sequential` — Task 02 and Task 03 both register contracts into `Registry()`.

## Inputs

- Spec, *`telemetry_activations` (new)* and *Migration and activation*
- Plan, *Area 1*
- The composite primary key is the point of the table: read the spec paragraph
  beginning "The primary key is `(contract_key, contract_version)`" before
  choosing the DDL.
- `internal/backendapp/storage.go:99-101` — the comment there marks the exact
  moment every repository has finished `initSchema`, which is when a contract's
  backing objects are known to exist. Hook in there, not earlier.
- `internal/db.MigrateLogger` swallows migration errors by design; that is why
  the health line reports object *existence* rather than assuming it.
- Neither `Activate` nor `LogHealth` may be fatal. A failure logs and boot
  continues, matching `recordSchemaVersion`'s contract directly above.
- Use `INSERT ... ON CONFLICT DO NOTHING` (both SQLite and pgx support it) so a
  repeated or concurrent boot is a no-op rather than an error path.

## Output contract

Summary, files changed, tests run with counts, blockers, risks, and a status
update to this file and `plan.md` in the same conversation.

## Results

See `plan.md` § Verification Results for the consolidated commands, outcomes, and decision record covering all six tasks (implemented and committed together).

---
id: "02-wave-identity-persistence"
title: "Persist wave identity and classify its unique violation"
status: pending
wave: 2
depends_on: ["01-wave-identity-primitives"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 02: Persist wave identity and classify its unique violation

## Summary

Add the two `runs` columns and their partial unique index, thread them
through `runs/service`'s request/insert path with dedupe-on-conflict
handling and a coalescing exclusion, and add the observability counter. No
producer sets these fields yet — verified directly against `runs/service`.

## In scope

- `runs.wake_wave_key` / `runs.wake_wave_string` columns (both `TEXT NOT
  NULL DEFAULT ''`), `idx_run_wake_wave(wake_wave_key, agent_profile_id)
  WHERE wake_wave_key <> ''`, added to `createRunTables` (fresh schema) and a
  new replayable `migrateWakeWaveColumns()` migration (existing databases).
- `models.Run.WakeWaveKey` / `WakeWaveString` fields; `CreateRunTx`'s column
  list and bound args.
- `internal/runs/repository/sqlite/idempotency_violation.go`:
  `IsWakeWaveUniqueViolation(err error) bool`.
- `internal/runs/service/service.go`: `QueueRunRequest` gains the two
  fields; `insertRun` sets them on the inserted row; a second sentinel
  (parallel to `errIdempotencyKeyConflict`) maps
  `IsWakeWaveUniqueViolation` to `QueueOutcomeDeduped`; `shouldCoalesceRun`
  excludes wave-carrying requests.
- `internal/runs/repository/sqlite/runs.go`, `CoalesceRun`: exclude
  wave-carrying rows from the merge-target candidate query.
- `parent_wake_deduped_total` expvar, incremented at this classification
  site (the second site, in `office/scheduler`, is Task 03's
  responsibility — this task only covers the one runs/service exercises).

## Out of scope

- Any producer actually populating `WakeWaveKey`/`WakeWaveString` on a
  request (Tasks 03-05).
- `office/scheduler.SchedulerService.QueueRun`'s own classification site and
  coalescing guard (Task 03) — `runs/service` and the office scheduler's
  direct-insert path are two separate call sites into `CreateRun`, per the
  system design's "Failure and recovery" section, and this task only closes
  the `runs/service` one.
- `ListStuckParents` (Task 06).

## Acceptance

- A `QueueRun` call with a `WakeWaveKey` set inserts a row with both columns
  populated; a second call with the identical `(WakeWaveKey,
  AgentProfileID)` returns `QueueOutcomeDeduped`, not an error, and is not
  coalesced into instead.
- A `QueueRun` call with `WakeWaveKey` set to a *different, already-queued*
  wave's key is never selected as a `CoalesceRun` merge target by an
  unrelated wave-carrying request for the same agent+reason inside the
  coalescing window.
- `IsWakeWaveUniqueViolation` returns true for a real SQLite composite
  unique-index violation on this index (confirmed empirically, not assumed
  from the single-column message) and for the matching PostgreSQL
  `pgconn.PgError` shape (gated test, `KANDEV_TEST_POSTGRES_DSN`).

## Verification

```bash
cd apps/backend
go test ./internal/office/repository/sqlite/... -run TestMigrate -v
go test ./internal/runs/service/... -run TestQueueRun -v
go test ./internal/runs/repository/sqlite/... -run TestIsWakeWaveUniqueViolation -v
KANDEV_TEST_POSTGRES_DSN=... go test ./internal/runs/service/... -run TestPostgresQueueRun -v
```

## Files likely touched

- `internal/office/repository/sqlite/base.go`
- `internal/office/repository/sqlite/base_migrations.go`
- `internal/office/repository/sqlite/base_migrations_test.go` (replay
  coverage, per the backend's table-rebuild/migration testing convention)
- `internal/office/models/models.go`
- `internal/runs/repository/sqlite/runs.go`
- `internal/runs/repository/sqlite/idempotency_violation.go`
- `internal/runs/repository/sqlite/idempotency_violation_test.go` (new)
- `internal/runs/service/service.go`
- `internal/runs/service/service_test.go`
- `internal/runs/service/service_postgres_test.go`
- `internal/office/service/wake_metrics.go`

## Dependencies

Task 01 (uses `waveidentity` types only for test fixtures, not production
code in this task).

## Risks

- **SQLite composite-index message shape** — confirm empirically before
  hardcoding; a wrong substring silently fails classification and P1/engine
  producers would see a hard error instead of a dedupe on their second
  arrival.
- **`parent_wake_deduped_total` cross-package increment** — check
  `wake_metrics.go`'s existing wiring before adding a new seam; prefer
  reusing whatever pattern already gets a runs/service-side outcome back to
  office's expvars, if one exists.

## Parallelism

`sequential`

## Inputs

- System design: "Persistence", "Coalescing: the third write path into
  runs", "Failure and recovery" sections.
- `internal/runs/repository/sqlite/idempotency_violation.go` (pattern to
  mirror).
- `internal/office/repository/sqlite/base_migrations.go`
  (`migrateContinuationScope` as the replayable-migration precedent).
- `internal/runs/service/service_test.go:578`
  `TestQueueRun_DedupesOnIdempotencyIndexRace` (pattern for the new race
  test written here at the request-construction level; the full
  cross-producer race test is Task 07's).

## Results

Pending.

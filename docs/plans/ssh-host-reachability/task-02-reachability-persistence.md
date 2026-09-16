---
id: "02-reachability-persistence"
title: "Reachability record persistence"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.16
  - AC-EXECUTORS-SSH-REACHABILITY-001.21
  - AC-EXECUTORS-SSH-REACHABILITY-001.22
system_design:
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 02: Reachability Record Persistence

## Summary

Add the `executor_reachability` table, its model, and its repository methods.
Exactly one row per SSH executor, surviving restart, converging on the later
observation when two writes race, and deleted when its executor is
soft-deleted. No probe writes to it yet.

## In scope

- `models.ExecutorReachability` with the state and reason constants, plus a
  `ListSSHExecutorsForReachability` projection returning eligible executors
  (`type = 'ssh'`, `deleted_at IS NULL`, `status = 'active'`) ordered ascending
  by `executors.id`.
- A single additive `r.migrate.Apply("executor_reachability.table", …)` in
  `base_migrations.go`, following the `repository_secret_bindings` precedent so
  it is idempotent on SQLite, on Postgres, and on an already-migrated database.
- `GetExecutorReachability`, `ListExecutorReachability`,
  `UpsertExecutorReachability`, `DeleteExecutorReachability` on
  `ExecutorRepository` and its SQLite implementation.
- Columns per the system design, including `last_success_at` alongside
  `checked_at`: the pre-launch warning needs the age of the last *successful*
  probe, and `checked_at` is overwritten by every failure. `updated_at
  TIMESTAMP NOT NULL` is a distinct column from `checked_at`: it advances on
  every write including a reset (whose `checked_at` is `NULL`), and is the
  field task 04's client reconciliation depends on.
- Last-write-wins on `checked_at`: the upsert's conflict clause writes only
  when the incoming `checked_at` is strictly later than the stored one, or the
  stored one is `NULL`. A write whose `checked_at` equals the stored value is
  discarded, and timestamps are persisted at millisecond resolution or finer so
  two probes in the same second still order. The consecutive-failure counter is
  read and written inside that same statement's transaction so a streak cannot
  be lost to an interleaving.
- **A second, distinct method, `ResetExecutorReachability(ctx, executorID,
  host string) error`**, is its own SQL statement, not a branch of the upsert:
  the upsert's `state` CASE has no `unknown` branch and its `WHERE` admits only
  a strictly later `checked_at`, which a reset does not carry. It writes
  `unknown`, a zero counter, cleared reason/message, the newly saved `host`,
  and both `checked_at`/`last_success_at` set `NULL`, guarded only by `type =
  'ssh' AND deleted_at IS NULL AND status = 'active'` — no `checked_at` or
  `updated_at` pin, because a reset is ordered by the save that caused it, not
  by an observation clock, and must win over whatever is stored. An executor
  that is `ssh` but not `active` is *not* reset (eligibility wins per
  `AC-…-001.17`), and the statement lands zero rows for it rather than
  erroring. This task owns only the statement; task 04 owns who calls it.
- `DeleteExecutor` removes the reachability row alongside the soft delete.

## Out of scope

- The poller, the ticker, the hysteresis rule, and the decision of *what*
  counter value or state to write. This task stores what it is given.
- Any HTTP route, event, or DTO.
- Columns on `executors`. Observed state must not share the row that holds
  user-authored `config` and the user-controlled `status` switch.
- Retention beyond one row per executor: no history, trend, or incident log.

## Acceptance

- Two upserts for one executor carrying an older and a newer `checked_at`
  leave the newer values stored regardless of which transaction commits second,
  including when the older one lands on a whole second and the newer one carries
  a fraction of that same second.
- Soft-deleting an executor removes its reachability row, and re-reading it
  returns a not-found rather than a stale record.
- A record written, then read back through a reopened repository against the
  same database file, is byte-identical including both `checked_at` and
  `last_success_at`.
- An upsert whose `checked_at` equals the stored value leaves the row
  unchanged.
- `ResetExecutorReachability` on a record with a populated `checked_at`,
  `last_success_at`, and non-zero counter leaves `state = unknown`, the
  counter zeroed, reason/message cleared, `host` set to the new value, and
  both timestamps `NULL`; `updated_at` still advances even though `checked_at`
  does not.
- `ResetExecutorReachability` against an executor whose `status` is not
  `active` (but is still `type = 'ssh'` and not soft-deleted) affects zero
  rows and leaves the existing record untouched.

## Verification

Start with the concurrency test as a failing test — issue the newer write
first and the older write second, assert the newer survives, and confirm a
plain `ON CONFLICT DO UPDATE` without the timestamp predicate fails it. Then
run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/task/repository/sqlite/ -run 'ExecutorReachability|ResetExecutorReachability'
go test -tags fts5 -race ./internal/task/repository/... -run 'Migration|Schema'
make lint
```

Postgres parity runs through the existing `*_postgres_test.go` harness in the
same package.

## Files likely touched

- `apps/backend/internal/task/models/executor_reachability.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/executor_reachability.go`
- `apps/backend/internal/task/repository/sqlite/executor_reachability_test.go`
- `apps/backend/internal/task/repository/sqlite/executor_reachability_postgres_test.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/executor.go`

## Dependencies

None.

## Risks

- `MigrateLogger.Apply` swallows non-"already exists" errors, so a malformed
  `CREATE TABLE` fails silently at migration time and only surfaces as a
  "no such table" at first write. Assert the table's presence in a migration
  test rather than trusting the apply to be loud.
- `TIMESTAMP` comparison semantics differ between SQLite and Postgres, and the
  difference is not symmetric. Postgres compares its `timestamp` type
  temporally; SQLite stores the driver's text encoding in a NUMERIC-affinity
  column and compares it lexically. Write UTC and bind `time.Time` rather than a
  pre-formatted string, per the design's *Persistence* section — an RFC3339 `Z`
  string inverts the comparison for any whole-second value. A Postgres-only
  assertion passes while SQLite keeps the wrong row, so the ordering test must
  run on both, with a sub-second pair and a whole-second/sub-second pair.
- Adding methods to `ExecutorRepository` widens an interface with existing
  fakes across the codebase; expect compile breaks in unrelated test packages
  and extend those fakes rather than narrowing the interface.

## Parallelism

`parallel-safe` with task 01 — disjoint packages, and task 01 carries no
schema change.

## Inputs

- System design, section *Persistence*.
- `base_migrations.go` `repository_secret_bindings` block as the
  create-table-through-migration precedent.
- `executor.go` for the existing executor CRUD and soft-delete shape.
- `models.ExecutorTypeSSH` and `models.ExecutorStatusActive`.

## Results

Pending.

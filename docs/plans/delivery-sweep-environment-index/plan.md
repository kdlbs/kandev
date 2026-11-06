---
created: 2026-11-05
status: complete
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-008
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
legacy_specs: []
---

# Implementation Plan: Delivery environment lookup index

## Overview

Resolve [issue #5088](https://github.com/kdlbs/kandev/issues/5088) with one additive
index and regression coverage. One sequential work order owns the migration,
upgrade safety, query-plan proof, and dual-engine result checks.

Platform owns shared read availability. Tasks retains ownership of environment
bindings and delivery semantics. The existing availability requirement now
includes REQ-PLATFORM-INTERACTIVE-READS-008 because earlier criteria cover other
workloads. No new system boundary or ADR is necessary.

## Confirmed cause and evidence

`mostRecentGitSnapshotObservationQuery` contains a correlated session-binding
lookup on `(task_environment_id, task_id)`. No existing index leads with the
environment column. A low-selectivity task predicate therefore permits a full
session scan for each outer snapshot.

A temporary test used the real repository and task bootstrap on commit
`5e751beb1f4d1ce3747895dfffbfb84ecf809238`.
With 3,500 snapshots and 3,500 inherited bindings, SQLite reported `SCAN binding`.
The proposed index changed that operation to a covering search on both columns.
One observation call changed from 275.543 ms to 3.658 ms with the same maximum.
See [the diagnostic evidence](evidence.md) for the fixture, plans, and limits.

`SnapshotsForPair` contains the same binding predicate and benefits from this
index. Its provenance lookup already uses the session primary key.
The environment-repository lookup already has an environment-leading index.
No further index is supported by this investigation.

## Scope

### In scope

- Install the portable environment/task index through the task migration path.
- Cover fresh, existing, pre-column, rebuilt, and replayed session schemas.
- Prove the production sweep query uses the new access path.
- Preserve delivery results on SQLite and PostgreSQL.

### Out of scope

- Query rewrites, delivery classification changes, health-probe policy, or new pools.
- Changes to sweep cadence, timeouts, runtime flags, frontend, or translations.
- Production database access, manual operator SQL, commits, and PR creation.
- Changes to existing delivery-ledger or availability package completion records.

## Technical approach

In `apps/backend/internal/task/repository/sqlite/base_migrations.go`, add a checked
`ApplyContext` statement near the end of `runMigrations`:

```sql
CREATE INDEX IF NOT EXISTS idx_task_sessions_environment_task
ON task_sessions(task_environment_id, task_id);
```

Place it after `migrateTurnChangeHistorySchema` and before `r.migrate.Err()`.
All known session rebuilds then precede index creation.
Leave the pre-migration index block in `base_schema.go` unchanged.
That block runs before old databases receive `task_environment_id`.

The index is non-unique and includes empty legacy values.
The same statement serves both database engines.
Existing operator installations of this exact index remain valid.
Index failures must fail initialization through the required migration path.

The query continues to accept an environment owner or at least one matching
session binding. Mixed unrelated siblings must not change that existential rule.
Preserve nullable capture provenance and all existing repository filters.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| AC-PLATFORM-INTERACTIVE-READS-008.1 | `delivery/sweep_query_plan_test.go:TestDeliveryGitSnapshotObservationUsesEnvironmentTaskIndex` explains the production query after real bootstrap and rejects `SCAN binding`. |
| AC-PLATFORM-INTERACTIVE-READS-008.2 | The query-plan fixture asserts the exact observation. Existing delivery tests retain capture-session deletion, session-free snapshots, and branch deduplication coverage. A new mixed-binding case protects shared-environment eligibility on both engines. |
| AC-PLATFORM-INTERACTIVE-READS-008.3 | `task/repository/sqlite/task_session_environment_index_test.go:TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay` covers lifecycle and row preservation. Its PostgreSQL counterpart uses `OpenIsolatedPostgres`. |

The new index tests must fail before the migration exists.
The delivery plan test must reject the original binding scan.
The retained diagnostic already demonstrated that plan.

Use fixed fixture times, one transaction for bulk seeds, and immediate cleanup.
Keep latency measurements informational. Avoid a fragile millisecond CI gate.

## End-to-end boundary

This is a database access-path repair with no rendered behavior change.
Integration tests use the production constructor, query, and delivery selection.
A browser test does not inspect or prove the failed access path.
The reporter's complete HTTP 503 incident was not reproduced locally.

## Work orders

- [x] [Task 01: Index delivery environment bindings](task-01-index-environment-bindings.md)

Executed sequentially on 2026-11-05 after the implementation request.
The work order contains the exact TDD, Go, SQL guard, lint, and conformance commands.

## Verification results

Design-package checks passed on 2026-11-05:

- The temporary reproduction passed its before/after comparison with an unchanged result.
- GitHub confirmed assignment to `carlosflorencio`.
- `python3 scripts/list-docs.py validate`: 432 decisions and 1,921 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The exported `validateCoverage` preflight returned `exempt` for the actual documentation-only diff.
- The same preflight returned `covered` with no errors after inclusion of the four planned Go paths.
- `git diff --check`: passed. The temporary reproduction is absent.

Implementation checks passed on 2026-11-05:

- RED before the migration: the observation plan reported `SCAN binding` and the
  SQLite lifecycle test reported no environment index.
- GREEN: the analyzed SQLite plan reports
  `SEARCH binding USING COVERING INDEX idx_task_sessions_environment_task (task_environment_id=? AND task_id=?)`.
  The exact maximum observation is unchanged.
- SQLite lifecycle coverage passes for fresh creation, replay, a missing index,
  an identical operator-created index, both legacy session rebuilds, and a
  pre-column database. Sentinel task, environment, session, timestamp, and
  snapshot rows are preserved; the pre-column session binding is healed.
- PostgreSQL lifecycle coverage passes against a disposable server for fresh
  creation, replay, missing-index upgrade, an identical operator-created index,
  and a pre-column database. The mixed-binding eligibility fixture also passes
  on PostgreSQL.
- `go test -trimpath -tags fts5 -race ./internal/delivery -count=1` passed with
  `KANDEV_TEST_POSTGRES_DSN` configured, including the PostgreSQL eligibility
  subtest.
- `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run
  '^(TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay|TestPostgresTaskSessionEnvironmentTaskIndexFreshUpgradeReplay|TestSQLiteSchemaReinitializes|TestGitSnapshotEnvironmentMigration.*)$'
  -count=1 -v` passed with the same DSN.
- `go test -trimpath -tags fts5 ./internal/task/repository/sqlite -count=1` passed.
- `go build -tags fts5 ./...`, `go run ./cmd/sqlguard ./internal`, and
  `go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1`
  passed.
- `bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/base_migrations.go apps/backend/internal/task/repository/sqlite/task_session_environment_index_test.go apps/backend/internal/task/repository/sqlite/task_session_environment_index_postgres_test.go apps/backend/internal/delivery/sweep_query_plan_test.go apps/backend/internal/worktree/store.go apps/backend/internal/worktree/manager_cleanup_shared_reference_test.go`
  reported `0 issues`.
- The fixup also corrected task-scoped worktree inventory: the query now joins
  only sessions belonging to the environment owner task. Without that, a shared
  environment borrower session could make owner cleanup skip its worktree.
  `go test -trimpath -race ./internal/worktree -count=1` and
  `go test -trimpath -race ./internal/task/inventoryrepair -count=1` passed.
- Catalog validation, all specification lint, and `git diff --check` passed.

No millisecond CI timer was added. The disposable PostgreSQL server was removed
after verification.

## Risks

- An early index can fail before the column exists or disappear during a rebuild.
- PostgreSQL index creation takes a startup lock. This repair does not promise zero-downtime migration.
- PostgreSQL behavior checks require `KANDEV_TEST_POSTGRES_DSN`. A skipped test is not passing PostgreSQL evidence.
- Larger databases incur index storage and startup build work. The synthetic timing does not predict every installation.
- This index removes one confirmed bottleneck. Other readers or writers can still overload persistence.

## Documentation impact

Internal requirements and design change. Public APIs, configuration, recovery
instructions, and UI copy retain their current contracts.
No public documentation update is necessary for this package.

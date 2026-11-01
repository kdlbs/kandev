---
id: "01-native-backfill-order"
title: "Retain pre-upgrade native usage during migration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-007.11
system_design:
  - ../../specs/costs/system-design/usage-history.md
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 01: Retain pre-upgrade native usage during migration

## Summary

Native backfill runs before source adoption, so it finds no eligible binding and records a permanent gap. A temporary test on `15d0ddc` showed `native buckets=0, native gaps=1, eligible bindings=1` after migration completed. The usage reached retained history only when the task was deleted, and the gap row stayed afterwards. Run adoption first, and clear the gap when an event is retained.

## In scope

1. Bring this package onto the PR branch: `git switch feature/investigate-token-co-9dc` with the uncommitted files in place. If the branch is checked out elsewhere, STOP and report.
2. In `BackfillUsageHistory` (`usage_history_migration.go`), call `r.BackfillUsageSourceAdoption(ctx, limit)` right after the limit checks. Return `false, err` on error and `false, nil` when not done. Remove the later adoption call and return the `repairNativeUsageAccountingBatch` result directly.
3. In `persistScopedNativeUsageTx` (`usage_history.go`), after the native usage persists, delete its gap row: `DELETE FROM usage_history_migration_gaps WHERE source_key='native' AND (record_key = ? OR record_key = (SELECT CAST(id AS TEXT) FROM task_usage_events WHERE usage_event_id = ? AND task_id = ?))`. Use `r.db.Rebind`.
4. Add the test below.

## Out of scope

- Changing adoption's exclusion rules or `ReopenUsageHistoryAfterDisabledPeriod`.

## Acceptance

- After `BackfillUsageHistory` reports done, a pre-upgrade eligible session's event has one native bucket and no gap row, without deleting the task.
- An event on a forked session still has a gap row after migration.
- Deleting the eligible session's task afterwards succeeds, and its bucket remains.

## Verification

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite/ -run 'TestPreUpgradeNativeUsageAppearsAfterMigration' -count=1)
(cd apps/backend && go test ./internal/analytics/... ./internal/task/repository/sqlite/ -count=1)
(cd apps/backend && go test -race ./internal/analytics/repository/sqlite/ -run 'TestSourceAdoption|Migration|Backfill' -count=1)
bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/usage_history_migration.go apps/backend/internal/analytics/repository/sqlite/usage_history.go
```

Also run the first two commands with `KANDEV_TEST_POSTGRES_DSN` set if a PostgreSQL 18 server is available.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_migration.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history.go`
- `apps/backend/internal/task/repository/sqlite/usage_history_native_backfill_test.go` (new)

## Test recipe

In package `sqlite` under `internal/task/repository/sqlite`, use `newUsageEventsTestRepo(t)`.
1. Create the workspace, task, and a session with `Metadata: {"acp": {"session_id": "old-transcript"}}`, then call `repo.CreateTaskUsageEvent(ctx, newTestUsageEvent("old-event", "task-old", "session-old"))`. Do all of this before setting the retainer, so the rows look pre-upgrade.
2. Create `analyticssqlite.NewWithDB(repo.db, repo.db)` and call `repo.SetUsageHistoryRetainer`.
3. Loop `BackfillUsageHistory(ctx, 200)` until it reports done.
4. Assert `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'` is 1, and the same count on `usage_history_migration_gaps WHERE source_key='native'` is 0.

## Dependencies

None.

## Risks

- Existing tests may assume the old order or a `Preparing` result. Update fixtures, not assertions. If an assertion genuinely encodes the old order, list it in Results.

## Parallelism

`sequential`

## Inputs

- [Import design](../../specs/costs/system-design/usage-history-import.md), Native usage on existing sessions.

## Results

`BackfillUsageHistory` now completes source adoption before native source pages. Successful native retention clears migration gaps by event ID and legacy ledger-row ID.

`TestPreUpgradeNativeUsageAppearsAfterMigration` confirms that an eligible pre-upgrade event is retained once without a gap, a forked event keeps a gap, and deleting either task preserves the expected retained copy or gap.

The focused test, analytics and task repository suites, analytics race check, and changed-file Go lint passed. PostgreSQL checks were skipped because no PostgreSQL 18.6 image or configured test DSN was available.

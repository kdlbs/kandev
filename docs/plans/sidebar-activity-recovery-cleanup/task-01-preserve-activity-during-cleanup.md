---
id: "01-preserve-activity-during-cleanup"
title: "Preserve activity during completed move cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-001
acceptance_criteria:
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.3
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.7
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-001.9
system_design:
  - ../../specs/ui/system-design/sidebar-last-activity-sort.md
---

# Task 01: Preserve Activity During Completed Move Cleanup

## Summary

Prove that startup recovery changes an idle task's activity time, then keep its timestamp stable when clearing completed manual-move markers. Preserve marker convergence and the conditional guard against a newer move.

## In scope

- Add a failing startup integration regression that observes `tasks.updated_at` and projected `last_activity_at` before and after completed-marker cleanup.
- Extend the repository contract for SQLite and PostgreSQL to cover successful, absent, and stale-generation clears plus reconstructed activity.
- Change only the completed-marker cleanup write; verify a genuine subsequent task mutation still advances activity.

## Out of scope

- Historical data repair, general metadata timestamp policy, and frontend changes.

## Acceptance

1. A completed-marker sweep clears only the intended markers and leaves task `updated_at` and summary `last_activity_at` unchanged; rebuilding the summary does not reintroduce the cleanup time.
2. Failed feeder reconciliation retains the marker; a stale clear cannot erase a newer pending or completed move; the existing `task.updated` notification still occurs after a successful clear.
3. A real task edit or workflow move after cleanup advances activity and changes the sidebar order through the existing contract.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test(SetTaskMetadataKeyIfPresentSQLite|PostgresSetTaskMetadataKeyIfPresent|TaskLastActivityBatch|PostgresTaskLastActivityBatch|ClearManualMoveLifecycleMarkersPreservesActivity)' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test(RecoverTaskLifecycleToken|RecoveryDoesNotClearPendingMarkerFromNewManualMove|RecoveryRetriesWhenNewManualMoveAlreadyCompleted|FailedManualMoveContinuationRetainsCompletionMarker)' -count=1)
(cd apps/backend && go test ./internal/task/statussummary -run 'LastActivity' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep 'sorts by last activity')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep 'mobile last activity sort')
```

The PostgreSQL repository case runs when `KANDEV_TEST_POSTGRES_DSN` is configured. Record it as skipped when that database is unavailable; do not treat a SQLite pass as PostgreSQL evidence.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_metadata_cas_test.go`
- `apps/backend/internal/orchestrator/lifecycle_sweep_startup_test.go`

## Dependencies

None.

## Risks

- The `updated_at` predicate is a move-generation guard even after the successful clear stops writing a new timestamp. Concurrent newer-move regressions must remain green.
- The startup integration test must use the persisted task timestamp and real summary projection, rather than mocking the timestamp it needs to protect.

## Parallelism

`sequential`

## Inputs

- [Sidebar last activity sort requirements](../../specs/ui/requirements/sidebar-last-activity-sort.md)
- [Sidebar last activity sort system design](../../specs/ui/system-design/sidebar-last-activity-sort.md)
- [Activity timestamp decision](../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md)

## Results

Implemented the timestamp-preserving cleanup in both SQL dialect branches.
The repository contract verifies marker removal, unchanged `updated_at`, stable
activity reconstruction, a genuine subsequent task edit, and stale-generation
refusal. The orchestrator regression verifies marker convergence, unchanged
task state and live activity through the status projector when startup
recovery publishes `task.updated`.

Validation passed:

- Repository, orchestrator, and status-summary targeted Go tests.
- Orchestrator newer-move recovery cases under `-race`.
- `make build` from `apps/backend`.
- `pnpm install --frozen-lockfile` from `apps`.
- Desktop `sorts by last activity` E2E and mobile `mobile last activity sort` E2E.

The PostgreSQL marker-cleanup contract was skipped because
`KANDEV_TEST_POSTGRES_DSN` was unset in this environment.

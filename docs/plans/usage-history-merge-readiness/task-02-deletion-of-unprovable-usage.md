---
id: "02-deletion-of-unprovable-usage"
title: "Allow deletion of unprovable native usage with a gap"
status: done
wave: 2
depends_on:
  - "01-native-backfill-order"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-007.2
  - AC-COSTS-USAGE-HISTORY-007.12
system_design:
  - ../../specs/costs/system-design/usage-history.md
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 02: Allow deletion of unprovable native usage with a gap

## Summary

Tasks with native usage on a forked or transferred session can never be deleted today, because the event has no eligible binding and so no retained copy. Once adoption has completed, allow deletion when the event has a recorded gap. Before that, keep refusing so the event can still be retained.

## In scope

Add a helper in `usage_history_native_preservation.go`:

`nativeUsageGapAcceptedTx(ctx, tx, usageEventID, taskID) (bool, error)` returns true only when both hold:
- `usageSourceAdoptionCompleteTx` is true.
- A row exists in `usage_history_migration_gaps` with `source_key='native'`, and `record_key` equals `usageEventID` or the ledger row's `CAST(id AS TEXT)`.

Use it in two places:
1. `preserveNativeUsageRowTx`: when `retained` is false, return nil if the helper returns true. Otherwise return `unpreservedNativeUsageError`.
2. `requireNativeUsagePreservedTx`: when the query finds an event, return nil only if every unretained event for the target has an accepted gap. Change the query to exclude events with a gap when adoption is complete. Do not fetch all events into memory.

Rename and rewrite the test as described below. Update the test's comment.

## Out of scope

- Retaining forked events with a new attribution rule.
- Any change to erasure, or to events with a usage binding.

## Acceptance

- A forked session's task deletes successfully after adoption completes, its ledger row is gone, and `usageReportGap` still reports a gap for the workspace.
- With the `source-adoption` cursor incomplete, the same deletion fails with "no accepted retained copy", and succeeds after retry once adoption finishes.
- An eligible session's event is still retained rather than dropped.

## Verification

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite/ -run 'TestUsageHistoryForkedNativeUsageDeletesWithGap|TestUsageHistoryUnprovenNativeBlocksDeletionBeforeAdoption|TestPreUpgradeNativeUsageAppearsAfterMigration' -count=1)
(cd apps/backend && go test ./internal/analytics/... ./internal/task/repository/sqlite/ -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite/ -run 'TestUsageHistory' -count=1)
bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/usage_history_native_preservation.go
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_native_preservation.go`
- `apps/backend/internal/task/repository/sqlite/usage_history_forked_native_test.go`
- `docs/public/usage-history.md`: state that usage which can't be attributed safely is removed with its task, and is reported as a history gap.

## Dependencies

Task 01.

## Risks

- Some preservation tests may expect a block for a gap-only event. In those fixtures adoption is usually incomplete, so they should still pass. If one fails, STOP and report it rather than editing its assertion.
- This is the one deliberate exception to AC-COSTS-USAGE-HISTORY-007.2. Do not broaden it.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/analytics/repository/sqlite/usage_history_source_adoption_fixture_test.go`: `completeUsageSourceAdoptionForTest` is in another package, so call `analyticsRepo.BackfillUsageSourceAdoption` in a loop instead.
- `usageReportGap` in `usage_history_report.go`.

## Results

Unretained native usage can pass task and session cleanup only after source adoption completes and a native gap exists for the event. Cleanup still fails before adoption. The check accepts event-ID and legacy ledger-row-ID gap keys and does not load all events into memory.

The forked-usage tests confirm that deletion fails before adoption, succeeds after adoption, removes the task ledger event, and leaves the workspace gap report visible. Public usage-history documentation now explains this behavior.

The focused tests, analytics and task repository suites, both race checks, changed-file Go lint, and public documentation validators passed. PostgreSQL checks were skipped because no PostgreSQL 18.6 image or configured test DSN was available.

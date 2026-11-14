---
id: "03-import-engine"
title: "Rebuild the two-step import engine"
status: done
wave: 2
depends_on:
  - "01-source-adoption"
  - "02-session-isolation"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-002
  - REQ-COSTS-USAGE-IMPORT-005
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-002.1
  - AC-COSTS-USAGE-IMPORT-002.2
  - AC-COSTS-USAGE-IMPORT-002.3
  - AC-COSTS-USAGE-IMPORT-002.4
  - AC-COSTS-USAGE-IMPORT-002.5
  - AC-COSTS-USAGE-IMPORT-002.6
  - AC-COSTS-USAGE-IMPORT-002.7
  - AC-COSTS-USAGE-IMPORT-002.8
  - AC-COSTS-USAGE-IMPORT-005.3
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 03: Rebuild the two-step import engine

## Summary

Replace the page-by-page loop in `server/import.go` with a scan and all-time step, then a daily step that runs one report per active day. Add a core test proving that this sequence counts usage once.

## In scope

- Schema version 3 checkpoint, interval merging, and the in-memory target index.
- Report pacing through `waitImportBudget`.
- Skipping ineligible targets, and reclassifying targets that become ineligible during a run.
- Removal of `LegacyDiagnostics` and page checkpoint fields, and their tests.
- Core test `TestLifetimeThenDatedImportCountsOnce`.

## Out of scope

- Queueing and restart behavior, which Task 04 owns.
- UI changes, which Task 05 owns.

## Acceptance

- Ten sessions sharing 3 active days across several snapshot pages produce exactly 1 lifetime report and 3 dated reports.
- A rerun over unchanged rows leaves observation revisions unchanged, and rows outside the index are not written.
- Through real core admission, lifetime then dated usage for one session contributes once, and partial dated usage does not reduce the lifetime total.

## Verification

```bash
(cd "$PLUGIN_ROOT" && go test ./server/ -run 'TestImportEngine' -count=1)
(cd "$PLUGIN_ROOT" && go test -race ./server/... -count=1)
(cd apps/backend && go test ./internal/analytics/repository/sqlite/ -run 'TestLifetimeThenDatedImportCountsOnce' -count=1)
```

## Files likely touched

- `server/import.go`
- `server/usage_target_dates.go`
- `server/import_engine_test.go` (new)
- `server/usage_test.go`
- `server/review_regression_test.go`
- `server/review_source_contract_test.go`
- `server/review_scope_test.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_import_sequence_test.go` (new)

## Dependencies

Tasks 01 and 02.

## Risks

- `TestReviewSourceContractUnsafeImportCannotUseLiveDatesOrTranscript` expects `unsupported`. Update it to expect a skip while keeping its assertion that no live date or transcript is used.
- A long-lived session expands to every day in its range, including days with no rows. This remains bounded by merged intervals.
- Memory for the target index grows with session count. It holds identifiers and date bounds only.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/costs/system-design/usage-history-import.md), Import worker.
- `docs/specs/costs/system-design/usage-history.md`, Canonical facts and daily summaries.
- `server/testdata/tokscale-4.15.1-session-model.json`.

## Results

Replaced the page loop with persisted scanning, daily, and completed phases and a schema version 3 checkpoint.
The scan builds an in-memory target index, runs one lifetime report, merges trusted date ranges into intervals, and then runs one dated report per active day with 5-60 second pacing.
Ineligible targets are skipped or reclassified without stopping the run, and unchanged reruns leave observation revisions unchanged. The index is keyed by subject binding so sessions sharing one transcript keep separate ranges, and an open session with a known start extends its import range to today.
Core `TestLifetimeThenDatedImportCountsOnce` proves lifetime then dated admission contributes once and a partial day does not reduce the lifetime total.

Validation: plugin `go test ./server/ -run TestImportEngine -count=1` and `go test -trimpath -race ./...` passed. Core `go test ./internal/analytics/repository/sqlite/ -run TestLifetimeThenDatedImportCountsOnce -count=1` passed.

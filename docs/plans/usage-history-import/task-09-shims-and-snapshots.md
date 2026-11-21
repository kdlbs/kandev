---
id: "09-shims-and-snapshots"
title: "Remove compatibility shims and bound snapshots"
status: done
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-004.6
  - AC-COSTS-USAGE-HISTORY-005.2
  - AC-COSTS-USAGE-HISTORY-006.5
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 09: Remove compatibility shims and bound snapshots

## Summary

Remove compatibility code that one binary cannot need, including a fallback that reports authoritative zeros. Stop discovery snapshots accumulating forever.

## In scope

- `httpGetTokenUsage`: replace the `UsageRepository` type-assertion fallback with a required dependency.
- `listSessionsForPluginFallback` and its `nolint`.
- `ColumnExistsContext` and `TableExistsContext` probes in `captureTargetSessionTx` and `usageLiveSourceCrossesScopeTx`.
- When a source creates a snapshot for a scope, delete that source's older snapshots for the scope in the same transaction.

## Out of scope

- Other report query performance, which Task 12 owns.

## Acceptance

- The token usage route cannot return an empty report because of a missing repository capability.
- Capturing a target page runs no schema probe query.
- Repeated initial `ListTargets` calls leave one snapshot row per source and scope, while a resumed cursor on the current snapshot still works.

## Verification

```bash
(cd apps/backend && go test ./internal/analytics/... ./internal/task/service/... -count=1)
(cd apps/backend && go test ./internal/analytics/repository/sqlite/ -run 'TestUsageHistoryBounds|TestUsageSnapshotRetention' -count=1)
bash scripts/lint-go-changed apps/backend/internal/analytics/handlers/stats_handlers.go apps/backend/internal/task/service/service_sessions.go apps/backend/internal/analytics/repository/sqlite/usage_history_targets.go
```

## Files likely touched

- `apps/backend/internal/analytics/handlers/stats_handlers.go`
- `apps/backend/internal/analytics/handlers/usage_history_test.go`
- `apps/backend/internal/task/service/service_sessions.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_targets.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_live_source.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_bounds_test.go`

## Dependencies

None for implementation. Sequence it before Task 10 so the toggle gates the final code.

## Risks

- Test repositories that lack these tables or columns may rely on the probes. Fix those fixtures rather than keeping the probes.
- An interrupted import holds an old snapshot ID. Deletion applies only when the same source creates a newer one, and the import then restarts with its own snapshot. Task 04's lifecycle tests must still pass.

## Parallelism

`sequential`

## Inputs

- [Usage History design](../../specs/costs/system-design/usage-history.md), collection-target discovery.

## Results

The token usage route now requires the usage repository instead of returning an empty report.
Removed the plugin session fallback and the `ColumnExistsContext`/`TableExistsContext` probes from target capture and live-source checks; fixtures carry the real tables.
Target discovery keeps one snapshot row per source and scope, and a resumed cursor on the current snapshot still works.

Validation: `go test ./internal/analytics/... ./internal/task/service/... -count=1`, `go test ./internal/analytics/repository/sqlite/ -run 'TestUsageHistoryBounds|TestUsageSnapshotRetention' -count=1`, and changed-file Go lint passed.

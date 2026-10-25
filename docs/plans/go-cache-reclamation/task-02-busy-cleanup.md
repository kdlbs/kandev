---
id: "02-busy-cleanup"
title: "Optional cleanup during active tasks"
status: done
wave: 2
depends_on:
  - "01-direct-deletion"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-004
  - REQ-SYSTEM-PAGE-GO-CACHE-005
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-004.2
  - AC-SYSTEM-PAGE-GO-CACHE-004.3
  - AC-SYSTEM-PAGE-GO-CACHE-004.4
  - AC-SYSTEM-PAGE-GO-CACHE-004.5
  - AC-SYSTEM-PAGE-GO-CACHE-004.6
  - AC-SYSTEM-PAGE-GO-CACHE-004.7
  - AC-SYSTEM-PAGE-GO-CACHE-004.8
  - AC-SYSTEM-PAGE-GO-CACHE-004.10
  - AC-SYSTEM-PAGE-GO-CACHE-005.4
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 02: Optional cleanup during active tasks

## Summary

Persist the busy-cleanup setting and make opted-in Go cleanup progress during active task work.

## In scope

Existing settings JSON, preflight, runner provider partitioning, shared mutation serialization, scheduler behavior, mixed-run results, and cancellation.

## Out of scope

Busy cleanup of other providers, new timers, new database tables, and automatic build retries.

## Acceptance

- The setting defaults false and persists; scheduling and threshold prerequisites remain effective.
- When enabled, existing and newly admitted tasks do not block or cancel Go cleanup; other providers retain normal admission.
- Manual/scheduled cache mutations serialize, and completed Go results survive other-provider busy or failure states.

## Verification

Use TDD for changed logic. Commands start from the repository root.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/system/storage/... -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/backendapp -run 'Test.*(Storage|GoCache|Quarantine)' -count=1)
```

Cover the complete trigger matrix in the design, including disabled scheduling, disabled Go management, explicit selection, below-threshold no-op, force behavior, partial runs, shutdown, and adoption races. Use barriers to admit a new task during deletion and prove cleanup proceeds.

## Files likely touched

- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/system/storage/settings_test.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/runner.go`
- `apps/backend/internal/system/storage/runner_go_cache_test.go (new)`
- `apps/backend/internal/system/storage/scheduler.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`

## Dependencies

Task 01 must pass first.

## Risks

Preserve path safety and the exact activity-admission scope. Busy deletion intentionally can fail active builds.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
- [Design](../../specs/system-page/system-design/go-cache-reclamation.md)
- [Plan](plan.md), including compatibility matrix and existing regression suites.

## Results

Added persisted `go_cache.allow_cleanup_while_busy` (default false), explicit
selection handling, Go-only admission before the normal provider gate, mixed-run
skip reporting, captured-settings execution, and a cancellable mutation gate
shared by cache cleanup, adoption, and historical Go-cache restore/delete.
Other providers continue through ordinary idle/force admission.

Validation passed:

- `(cd apps/backend && go test -race -tags fts5 ./internal/system/storage/... -count=1)`
- `(cd apps/backend && go test -race -tags fts5 ./internal/backendapp -run 'Test.*(Storage|GoCache|Quarantine)' -count=1)`

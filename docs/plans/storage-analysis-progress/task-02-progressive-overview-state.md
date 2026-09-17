---
id: "02-progressive-overview-state"
title: "Add progressive overview state"
status: done
wave: 2
depends_on:
  - 01-bounded-filesystem-scanner
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.1
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.2
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.4
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.5
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.9
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.10
system_design:
  - ../../specs/system-page/system-design/storage-analysis-progress.md
---

# Task 02: Add Progressive Overview State

## Summary

Make storage overview reads nonblocking. Add single-flight progress state, stale-while-revalidate
snapshots, safe notifications, timing metadata, logs, and metrics.

## In scope

- Add progressive source callbacks to `storageOverview`.
- Extend `OverviewCache` with nonblocking state reads and generation fencing.
- Extend the storage response with snapshot, progress, and timing fields.
- Publish `system.storage.analysis.updated` without storage details.
- Add gateway wiring and focused observability.
- Preserve manual Analyze run history and job behavior.

## Out of scope

- Frontend rendering and polling.
- Persistent cache storage.
- Cleanup provider changes.

## Acceptance

- A blocked cold provider cannot block the initial HTTP response.
- Expired and failed refreshes keep the last successful snapshot unchanged.
- Concurrent reads, Analyze, invalidation, and late progress obey one generation-fenced flight.

## Verification

```bash
(cd apps/backend && go test -race ./internal/system/storage ./internal/backendapp ./internal/gateway/websocket)
```

## Files likely touched

- `apps/backend/internal/system/storage/overview_cache.go`
- `apps/backend/internal/system/storage/overview_cache_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/gateway/websocket/system_notifications.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

Task 01 supplies bounded progress callbacks.

## Risks

- Manual Analyze can accidentally create a second scan or job.
- Publishing while the cache mutex is locked can deadlock event consumers.
- A late callback can replace current state without generation checks.

## Parallelism

`sequential`

## Inputs

- `storage-analysis-progress.md` sections "Analysis state", "Cache flow", and "Progress delivery".
- ADR `2026-09-05-bounded-progressive-storage-analysis`.

## Results

Implemented the progressive cache/API contract, generation-fenced single-flight refreshes,
WebSocket notifications, structured completion logging, and storage scan metrics. The exact race
suite passed: 1,423 tests across storage, backendapp, and gateway/websocket.

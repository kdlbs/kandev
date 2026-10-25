---
id: "01-bounded-filesystem-scanner"
title: "Build bounded filesystem scanner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.2
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.10
system_design:
  - ../../specs/system-page/system-design/storage-analysis-progress.md
---

# Task 01: Build Bounded Filesystem Scanner

## Summary

Add a shared read-only scanner with four active directory partitions. Migrate the three filesystem
analysis providers without changing cleanup behavior or byte semantics.

## In scope

- Add `internal/system/storage/filescan` with indexed multi-root results.
- Preserve symlink, marker, warning, cancellation, and missing-root behavior.
- Migrate workspace, Go-cache, and temporary-artifact analysis.
- Publish partition and root progress through a callback.
- Add deterministic concurrency tests and a comparison benchmark.

## Out of scope

- Cache, HTTP, WebSocket, or React changes.
- Destructive cleanup paths.
- Operator configuration for concurrency.

## Acceptance

- No more than four directory partitions execute at once across one overview scan.
- Two independent partitions can enter before either finishes, without timing assertions.
- Serial and bounded implementations return the same bytes, warnings, and symlink outcomes.

## Verification

```bash
(cd apps/backend && go test -race ./internal/system/storage/filescan ./internal/system/storage/workspaces ./internal/system/storage/gocache ./internal/system/storage/tempartifacts ./internal/backendapp)
(cd apps/backend && go test ./internal/system/storage/filescan -run '^$' -bench 'BenchmarkMeasureTrees' -benchmem)
```

## Files likely touched

- `apps/backend/internal/system/storage/filescan/*.go`
- `apps/backend/internal/system/storage/filescan/*_test.go`
- `apps/backend/internal/system/storage/workspaces/provider.go`
- `apps/backend/internal/system/storage/workspaces/provider_test.go`
- `apps/backend/internal/system/storage/gocache/provider.go`
- `apps/backend/internal/system/storage/gocache/provider_test.go`
- `apps/backend/internal/system/storage/tempartifacts/provider.go`
- `apps/backend/internal/system/storage/tempartifacts/provider_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`

## Dependencies

None.

## Risks

- Root partitioning can double-count direct files or overlapping paths.
- A shared limiter can deadlock if a worker acquires another slot recursively.
- Provider warning order can become nondeterministic.

## Parallelism

`sequential`

## Inputs

- `storage-analysis-progress.md` sections "Bounded filesystem scanner" and "Filesystem scan".
- Existing `directorySizeNoFollow` implementations and provider tests.

## Results

- Added the shared four-partition limiter and context-aware indexed scanner.
- Migrated workspace, Go-cache, and temporary-artifact analysis to the scanner while leaving cleanup traversal serial.
- Verified with `go test -race ./internal/system/storage/filescan ./internal/system/storage/workspaces ./internal/system/storage/gocache ./internal/system/storage/tempartifacts ./internal/backendapp` (853 tests passed).
- Verified `BenchmarkMeasureTrees` with `go test ./internal/system/storage/filescan -run '^$' -bench 'BenchmarkMeasureTrees' -benchmem` (81,604 ns/op, 11,367 B/op).

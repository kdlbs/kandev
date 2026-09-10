---
id: "01-database-measurements"
title: "Database footprint measurements"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.8
system_design:
  - ../../specs/system-page/system-design/storage-database-footprint.md
---

# Task 01: Database footprint measurements

## Summary

Implement the read-only database and backup reader, then wire both sources through cached and progressive overview responses.

## In scope

- Both sources measure configured local locations and preserve independent failures, cancellation, and unsupported-driver behavior.
- Overview responses and progress include both sources, with cache reuse and explicit overlap attribution.
- Tests exercise main-file/sidecar sizes, manual backups, relative/custom paths, symlinks, unavailable reads, and no mutation.

## Out of scope

New cleanup behavior, retention changes, remote database measurement, and unrelated storage categories.

## Acceptance

- Both sources measure configured local locations and preserve independent failures, cancellation, and unsupported-driver behavior.
- Overview responses and progress include both sources, with cache reuse and explicit overlap attribution.
- Tests exercise main-file/sidecar sizes, manual backups, relative/custom paths, symlinks, unavailable reads, and no mutation.

## Verification

Run from `apps/backend`:

```bash
rtk go test ./internal/system/storage/...
rtk go test ./internal/backendapp -run 'TestStorage|TestSummary|TestOverview' -count=1
```

## Files likely touched

- `apps/backend/internal/system/storage/databasestore/provider.go`
- `apps/backend/internal/system/storage/databasestore/provider_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/analysis_state.go`
- `apps/backend/internal/system/storage/overview_cache.go`
- `apps/backend/internal/system/storage/overview_cache_progress_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go`

## Dependencies

None.

## Risks

Backup rotation can race reads. Use deterministic injected failures for permission tests, since root can read mode-restricted fixtures.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), draft extension `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002`.
- [System design](../../specs/system-page/system-design/storage-database-footprint.md).
- Existing storage provider, overview cache, resource rows, and mobile storage tests.

## Results

Implemented the SQLite database and sibling backup measurements, source progress,
cache projection, overlap attribution, unsupported-driver states, and regression
coverage. The exact backend verification commands passed: 210 storage-package
tests and 9 targeted backend-app tests.

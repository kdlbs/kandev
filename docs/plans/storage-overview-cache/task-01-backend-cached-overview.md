---
id: "01-backend-cached-overview"
title: "Backend cached overview"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 01: Backend cached overview

## Acceptance

- Successful overview scans are reused for 15 minutes and concurrent cache misses invoke the typed
  providers once.
- Manual Analyze bypasses the freshness window and replaces the shared successful snapshot.
- `GET /api/v1/system/storage` returns `analyzed_at` for the summary it serves.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/system/storage/overview_cache.go`
- `apps/backend/internal/system/storage/overview_cache_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/operations_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- focused overview fakes in backend System route tests

## Dependencies

None.

## Inputs

- Spec: What, API surface, Persistence guarantees, and cache scenarios
- `apps/backend/internal/backendapp/storage_maintenance.go:storageOverview`
- Existing `storage.OverviewProvider`, handler GET, and manual Analyze call paths

## Output contract

Return a compact handoff capsule with acceptance status, changed entry points, focused test results,
risk tags, uncertainties, and set this task to `done`.

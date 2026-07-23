---
spec: docs/specs/system-page/storage-maintenance.md
created: 2026-07-23
status: implemented
---

# Implementation Plan: Storage Overview Cache and Settings Stability

## Overview

Put a concurrency-safe 15-minute cache in front of the existing typed overview providers, then
expose the snapshot timestamp through the existing Storage API. Manual Analyze uses the same cache
owner but forces a refresh. The frontend displays snapshot age, hydrates the external Go-cache path
from persisted settings, and separates policy-save blocking from unrelated analysis and cleanup
activity.

## Backend

### Cached overview contract

- Add `apps/backend/internal/system/storage/overview_cache.go` with an `OverviewSnapshot` containing
  `Summary` and `AnalyzedAt`, plus cached and forced-refresh methods around the existing provider.
- Use a 15-minute default TTL, cache only successful scans, and serialize concurrent cache misses so
  page refreshes do not duplicate provider work.
- Keep the snapshot process-local. A backend restart starts with an empty cache.

### API and composition

- Update `apps/backend/internal/system/storage/handler.go` so `GET /storage` returns cached
  `summary` and `analyzed_at`.
- Update `apps/backend/internal/system/storage/operations.go` so manual Analyze calls the forced
  refresh path and records that summary in the existing run.
- Wrap `storageOverview` once in
  `apps/backend/internal/backendapp/storage_maintenance.go`, then pass the same cache owner to the
  handler and operations so forced analysis immediately updates subsequent GET responses.
- Update handler/composition fakes that implement the overview interface.

## Frontend

### Contracts and analysis card

- Add `analyzed_at` to `StorageOverviewResponse` in `apps/web/lib/types/system.ts`.
- Update `apps/web/components/settings/system/storage/storage-overview-card.tsx` to render
  “Last analyzed …” using the shared `formatRelativeTime` helper and an absolute timestamp tooltip.
- Preserve the existing loading spinner while the first post-startup snapshot is measured.

### Policy and external Go cache

- In `storage-policy-card.tsx`, initialize the external-cache input from
  `savedSettings.go_cache.adopted_path` and synchronize later persisted changes without overwriting
  an in-progress user edit.
- In `storage-maintenance-settings.tsx`, distinguish policy-mutating pending actions from analysis,
  cleanup, restore, and delete jobs. Those jobs must not disable policy editing.
- Do not mark the contributor invalid because its own save is in progress. If adoption is the
  conflicting mutation, show a specific adoption message rather than the generic current-action
  text.

## Tests

- **Cache freshness and forcing:** `apps/backend/internal/system/storage/overview_cache_test.go`
  verifies one source call within 15 minutes, refresh after expiry, forced refresh inside the TTL,
  successful-snapshot timestamps, failed-refresh retention, and concurrent miss coalescing.
- **Handler/operations wiring:** focused tests in `handler_test.go` and `operations_test.go` verify
  `analyzed_at` output and that manual Analyze uses the forced path.
- **External path hydration and save blocking:** existing Storage component tests verify the adopted
  path survives rerender/reopen and analysis/cleanup do not disable policy controls or the shared
  Save action.
- **Frontend contract:** update Storage fixtures and API tests for `analyzed_at`.

## E2E Tests

- Extend `apps/web/e2e/tests/system/storage-maintenance.spec.ts` to count overview measurements
  across page refresh/save and verify manual Analyze produces a newer displayed timestamp.
- Extend `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts` to verify the relative
  analysis timestamp and persisted external-cache field remain usable without horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [Task 01: Backend cached overview](task-01-backend-cached-overview.md)

Wave 2:

- [x] [Task 02: Frontend settings stability](task-02-frontend-settings-stability.md)

Wave 3:

- [x] [Task 03: Integrated desktop and mobile coverage](task-03-integrated-e2e.md)

## Risks

- Summary values can intentionally lag newly saved thresholds or an adopted path until manual
  Analyze or TTL expiry; `analyzed_at` makes that staleness visible.
- The cache must be shared by GET and manual Analyze. Separate wrappers would leave GET stale after
  a forced scan.
- Pending-state changes must retain mutual exclusion for adoption and saving without coupling
  policy controls to cleanup jobs that only consume a settings snapshot.

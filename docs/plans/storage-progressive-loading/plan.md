---
spec: docs/specs/system-page/requirements/storage-maintenance.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Progressive Storage Loading and Totals

## Overview

Decouple the lightweight Storage policy read from the scan-backed overview, then let the frontend
publish policy, history, quarantine, and analysis results independently. Add derived totals for the
analysis snapshot and current quarantine list, with explicit partial semantics when a top-level
measurement is unavailable. Preserve the existing `GET /storage` response for compatibility.

## Backend

### Lightweight policy read

- In `apps/backend/internal/system/storage/handler.go`, register
  `GET /api/v1/system/storage/settings` beside the existing `PATCH` route.
- Implement `getSettings` by calling `SettingsManager.GetSettings` and
  `OverviewReader.SettingsCapabilities`; return `{ settings, capabilities }` without calling
  `OverviewReader.Get`, `RunLister.ListRuns`, or any analysis provider.
- Keep `GET /api/v1/system/storage` unchanged so existing clients still receive
  `{ settings, capabilities, summary, analyzed_at, last_run }`.
- Cover the route directly in `apps/backend/internal/system/storage/handler_test.go` with a
  recording overview reader that fails the test if `Get` is invoked, and through the composed
  System router in `apps/backend/internal/system/storage_routes_test.go`.

## Frontend

### API and state contracts

- Add `StoragePolicyResponse` to `apps/web/lib/types/system.ts` and
  `fetchStoragePolicy()` to `apps/web/lib/api/domains/system-api.ts` for the new lightweight GET.
- Add `policy: StoragePolicyResponse | null` and `setSystemStoragePolicy` to the System Zustand
  slice in `apps/web/lib/state/slices/system/types.ts` and `system-slice.ts`.
- Extend the existing API and slice tests for the route, response shape, and policy update.

### Independent loading and refresh

- Refactor `apps/web/hooks/domains/system/use-storage-maintenance.ts` so policy, overview, runs,
  and quarantine requests start together but commit independently. Track per-section loading and
  error state plus per-section request generations so a slow older response cannot overwrite newer
  data from the same section.
- Remove first-load completion from the global `pendingAction`; policy-save coordination depends
  on `policyLoading`, while analysis, history, and quarantine can remain independently pending.
- Make save refresh only the lightweight policy contract, and apply the existing adoption response
  directly to policy state. Restore refreshes quarantine without waiting for an overview scan.
  Terminal analysis/cleanup/delete jobs refresh the affected sections concurrently so history and
  quarantine can settle even when the invalidated overview needs a new scan.
- Update `storage-maintenance-settings.tsx` to derive drafts and scheduling copy from `policy`, and
  pass section loading/error state to the analysis, history, and quarantine cards.
- Give `storage-overview-card.tsx`, `storage-run-history.tsx`, and
  `storage-quarantine-card.tsx` distinct progress, empty, and error states; a failure in one card
  must not suppress sibling cards.

### Totals

- Add `apps/web/components/settings/system/storage/storage-totals.ts` with pure helpers that:
  - sum workspace total, quarantine, managed and distinct user Go caches, managed-container
    writable layers, Docker image layers, and Docker build cache;
  - exclude active/candidate workspace and unused-image subset bytes to avoid double-counting;
  - return a `partial` flag when a top-level measurement is unavailable or missing; and
  - sum `size_bytes` across every listed quarantine entry.
- Render a localized **Total counted** value and partial indicator in
  `storage-overview-card.tsx`, and a localized total in `storage-quarantine-card.tsx`.
- Add the new user-facing strings to `apps/web/src/locales/en/settings.json`; do not localize
  identifiers, test IDs, confirmation tokens, or values used in logic.

### Mobile design contract

- Desktop outcome: the existing full-width card stack renders lightweight cards before analysis,
  and totals appear in the relevant card headers without changing actions or navigation.
- Mobile entry point and exemplar: keep the Settings sheet → System → Storage path and existing
  one-column card composition proven by
  `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`.
- Mobile hierarchy: policy, history, and quarantine remain ordinary inline cards in the page's
  single document scroll; Storage analysis owns only its card-level progress state. No drawer,
  overlay, fixed control, or second scroll owner is introduced.
- Touch behavior and state are shared with desktop. Existing action targets stay at least 44px;
  the totals are read-only and add no touch affordance. The mobile E2E scenario delays analysis,
  proves the other cards and totals render, and asserts no horizontal document overflow.

## Tests

- **Lightweight policy route:** `apps/backend/internal/system/storage/handler_test.go` verifies the
  response and proves no overview scan or run-history read occurs.
- **Composed route:** `apps/backend/internal/system/storage_routes_test.go` verifies
  `GET /api/v1/system/storage/settings` is registered and returns `200`.
- **API and store:** `apps/web/lib/api/domains/system-api.test.ts` and
  `apps/web/lib/state/slices/system/system-slice.test.ts` verify the new policy contract.
- **Progressive controller:** `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
  uses deferred promises to prove policy, runs, and quarantine publish before overview; covers
  isolated failures, same-section stale-response protection, and mutation-specific refreshes.
- **Totals:** `apps/web/components/settings/system/storage/storage-totals.test.ts` verifies exact
  sums, subset exclusion, partial analysis, zero/empty data, and quarantine aggregation.
- **Rendered states:** `storage-maintenance-settings.test.tsx`,
  `storage-overview-card.test.tsx`, `storage-run-history.test.tsx`, and
  `storage-quarantine-card.test.tsx` verify policy composition, loading/error/empty distinctions,
  and localized totals.

## E2E Tests

- **Desktop progressive load:** extend
  `apps/web/e2e/tests/system/storage-maintenance.spec.ts` to delay `GET /storage`, then verify the
  policy controls, maintenance history, and quarantine list are visible before analysis finishes;
  after release, verify the analysis total and quarantine total.
- **Mobile parity:** extend
  `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts` with the same delayed-analysis
  outcome on `mobile-chrome`, including totals and absence of horizontal overflow.

## Verification Results

Completed. Each task records its exact commands and outcomes in its `## Results` section.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Lightweight policy endpoint](task-01-policy-endpoint.md) — done

Wave 2:

- [x] [Task 02: Progressive section loading](task-02-progressive-loading.md) — done

Wave 3:

- [x] [Task 03: Storage totals](task-03-storage-totals.md) — done

Wave 4:

- [x] [Task 04: Desktop and mobile E2E](task-04-storage-e2e.md) — done

The tasks run sequentially in the primary conversation. The waves do not authorize subagents.

## Risks

- Docker unused-image bytes are a reclaimable subset of image-layer usage; adding both would
  overstate the total. The pure total helper and tests make that exclusion explicit.
- Cleanup and restore invalidate the overview cache, so an automatic overview refresh may scan
  again. Section-specific refreshes must let history and quarantine update without awaiting it.
- Empty arrays currently mean both “not loaded” and “loaded with no items”; explicit section state
  is required to prevent misleading empty-state copy during requests.
- Capability reads ping Docker. They are expected to be much cheaper than `DiskUsage` and
  filesystem traversal, but remain isolated from the overview scan and failure-scoped to policy.

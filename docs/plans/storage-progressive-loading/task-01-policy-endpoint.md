---
id: "01-policy-endpoint"
title: "Lightweight policy endpoint"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 01: Lightweight policy endpoint

## Acceptance

- `GET /api/v1/system/storage/settings` returns persisted settings and current capabilities.
- The lightweight GET does not request an overview snapshot, scan providers, or read run history.
- The existing scan-backed `GET /api/v1/system/storage` contract remains unchanged.

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/system/storage ./internal/system
```

## Files likely touched

- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/storage_routes_test.go`

## Dependencies

None.

## Parallelism

Sequential. This contract is required by Task 02.

## Inputs

- Spec: What, API surface, failure modes, and first-load scenarios
- Plan: Backend → Lightweight policy read
- Existing `SettingsManager`, `OverviewReader.Capabilities`, and Storage route tests

## Risks

- Do not call `OverviewReader.Get` from the new handler.
- Keep internal settings read errors from exposing sensitive details beyond the existing contract.

## Output contract

Report the route and response shape, files changed, exact test result, blockers/risks, and update
this task plus `plan.md` status in the same conversation.

## Results

- RED: `rtk go test -run TestGetStorageSettingsReturnsPolicyWithoutOverviewScan ./internal/system/storage`
  failed as expected with HTTP 404 before the route existed.
- GREEN/final: `rtk go test ./internal/system/storage ./internal/system` — 107 tests passed.
- Added `GET /api/v1/system/storage/settings`; it returns settings and settings-derived capabilities
  without calling the scan-backed overview reader or probing Docker. Existing `GET /storage` behavior
  remains unchanged.
- Settings-load failures now use a client-safe response while preserving the original error in the
  server-side logging callback.
- No external side effects or cleanup required.

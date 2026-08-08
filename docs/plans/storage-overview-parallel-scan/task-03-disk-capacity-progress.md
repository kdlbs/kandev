---
id: "03-disk-capacity-progress"
title: "Add disk capacity and progress card"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-overview-parallel-scan.md"
---

# Task 03: Add disk capacity and progress card

## Acceptance

- `GET /api/v1/system/storage/disk` measures the filesystem containing Kandev's storage root and
  returns total, used, available, percentage, path, and availability/error fields.
- The disk request is independent from the cached overview and remains responsive while the
  overview scan is held or cold.
- The Storage page shows localized used percentage, used/available/total sizes, and an accessible
  progress bar with warning styling at 80% and critical styling at 90%.
- A capacity failure renders an unavailable card and does not block policy, history, quarantine, or
  overview sections.
- Desktop and mobile layouts remain usable without horizontal overflow.

## Verification

```bash
cd apps/backend && go test -race ./internal/system/metrics ./internal/system/storage ./internal/backendapp
cd apps/web && pnpm run typecheck
cd apps/web && pnpm test -- --run components/settings/system/storage/storage-disk-capacity-card.test.tsx
```

The implementation must reuse or factor the existing cross-platform disk-free-space semantics in
`apps/backend/internal/system/metrics`; do not add a Unix-only storage API.

## Files likely touched

- `apps/backend/internal/system/metrics/disk_unix.go`
- `apps/backend/internal/system/metrics/disk_windows.go`
- `apps/backend/internal/system/metrics/*_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/types/system.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/components/settings/system/storage/storage-disk-capacity-card.tsx`
- Storage API/component fixtures and tests
- localized system translation resources

## Dependencies

None. The card is intentionally independent of Task 01's overview fan-out.

## Inputs

- Spec: filesystem capacity indicator, API surface, and failure scenarios
- Existing `apps/backend/internal/system/metrics/disk_unix.go` and `disk_windows.go`
- Existing progressive-loading request orchestration in `use-storage-maintenance.ts`
- Existing `formatGigabytes` and localized Storage card patterns

## Output contract

Return a compact handoff capsule with API/test results, desktop/mobile evidence, exact percentage
semantics, and any platform-specific capacity limitations.

## Result

Added the cross-platform `GET /api/v1/system/storage/disk` endpoint and an independently loaded
capacity card. Unix statfs and Windows `GetDiskFreeSpaceEx` expose total, service-available, used,
and clamped percentage values. The UI renders the exact percentage and byte-derived sizes, with
accessible progress semantics plus 80%/90% warning styling; unavailable reads remain non-blocking.
Backend metrics/storage tests, web typecheck/lint, focused component tests, and the desktop/mobile
managed Storage E2E flows pass.

---
id: "03-live-analysis-ui"
title: "Render live analysis details"
status: done
wave: 3
depends_on:
  - 02-progressive-overview-state
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.2
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.3
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.4
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.5
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.6
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.7
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.8
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.9
system_design:
  - ../../specs/system-page/system-design/storage-analysis-progress.md
---

# Task 03: Render Live Analysis Details

## Summary

Consume the progressive backend contract. Show cold-scan results, keep stale snapshots visible, and
add the scan and cache information disclosure on desktop and phone.

## In scope

- Update API, Zustand, and WebSocket types.
- Reload on analysis revisions and poll while a scan is active.
- Request a refresh at `refresh_due_at` while the page is mounted.
- Render completed, pending, failed, stale, and ready states.
- Add localized timing and progress copy in all required catalogs.
- Update `docs/public/operations.md` with the refresh behavior.

## Out of scope

- Backend scan logic.
- A new mobile route, drawer, or scroll owner.
- Time-remaining estimates.

## Acceptance

- The first scan reveals completed sources and labels every aggregate as counted so far.
- A stale snapshot remains readable while progress updates continue.
- The timing disclosure works by hover, focus, click, and 44-pixel phone tap.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/system/system-slice.test.ts lib/ws/handlers/system-events.test.ts hooks/domains/system/use-storage-maintenance.test.tsx hooks/domains/system/use-storage-maintenance-terminal-refresh.test.tsx components/settings/system/storage/storage-overview-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/lib/state/slices/system/types.ts`
- `apps/web/lib/state/slices/system/system-slice.ts`
- `apps/web/lib/state/slices/system/system-slice.test.ts`
- `apps/web/lib/ws/handlers/system-events.ts`
- `apps/web/lib/ws/handlers/system-events.test.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-setting-help.tsx`
- `apps/web/src/locales/*/system.json`
- `docs/public/operations.md`

## Dependencies

Task 02 defines the API and event contract.

## Risks

- A refresh timer can operate twice after a rerender if cleanup is incomplete.
- Partial and stale summaries can be combined accidentally.
- Tooltip content can become hover-only or too small for phone input.

## Parallelism

`sequential`

## Inputs

- `storage-analysis-progress.md` sections "Progress delivery" and "Storage page".
- Current `StorageSettingHelp` and Storage mobile E2E patterns.

## Results

Implemented progressive API/event state, polling and deadline refresh hooks, localized source and
timing disclosure UI, mobile-sized help interaction, and operator documentation. The focused suite
passed with 48 tests across five files, typecheck and lint passed, i18n catalogs and ratchet passed,
and public-doc validation passed.

---
id: "17-bundle-customizer-refinement"
title: "Bundle customizer refinement"
status: done
wave: 13
depends_on:
  - "12-custom-bundle-contracts"
  - "14-bundle-customizer-ui"
  - "15-custom-bundle-e2e"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 17: Bundle customizer refinement

## Acceptance

- System Logs exposes one **Customize bundle** entry point. Its default is the
  backend/frontend bundle; runtime and debug-only ACP evidence are optional
  sources in the same customizer.
- The wider desktop Dialog and phone Drawer share selection state. Selecting
  ACP lazy-loads candidates, shows the high-sensitivity warning, and provides
  select-all/clear controls that never exceed the backend session limit.
- Each authorized ACP candidate shows its task title as a safe new-tab link.
  The candidate API carries that title intentionally, while the runtime index,
  manifest, and archive continue to exclude it.

## Verification

```bash
cd apps/backend
go test ./internal/system/logbundle ./internal/backendapp
```

```bash
cd apps
pnpm --filter @kandev/web test -- --run components/settings/system/log-viewer.test.tsx
```

```bash
cd apps/web
pnpm e2e:run e2e/tests/system/logs-page.spec.ts
pnpm e2e:run --no-build --project mobile-chrome e2e/tests/system/mobile-logs-bundle.spec.ts
```

## Files likely touched

- `apps/backend/internal/system/logbundle/contracts.go`
- `apps/backend/internal/system/logbundle/handler.go`
- `apps/backend/internal/system/logbundle/handler_test.go`
- `apps/backend/internal/backendapp/diagnostic_session_provider.go`
- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/log-viewer.tsx`
- `apps/web/components/settings/system/bundle-customizer.tsx`
- `apps/web/components/settings/system/log-viewer.test.tsx`
- `apps/web/e2e/tests/system/logs-page.spec.ts`
- `apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `docs/specs/platform/diagnostic-logging.md`
- `docs/specs/system-page/spec.md`
- `docs/public/operations.md`
- `docs/decisions/2026-07-30-file-backed-diagnostic-bundles.md`

## Mobile design contract

- The existing System Logs route remains the mobile entry point. Its one
  Customize action opens the existing inset Drawer rather than a compressed
  desktop dialog.
- The source/session body remains the only internal scroll owner; the
  safe-area-aware footer keeps Cancel/Create reachable. Select-all and clear
  controls are visible touch targets rather than hover-only affordances.
- The same source/session state drives desktop and mobile. Mobile coverage
  proves the Drawer, ACP source choice, selection controls, and no horizontal
  overflow.

## Risks

- Task titles are user-visible task metadata, but must be returned only after
  existing task authorization and must never leak into bundle files.
- The picker can have more eligible sessions than its backend maximum. Bulk
  selection must stay deterministic and bounded.
- The wider desktop Dialog must remain viewport-contained; the phone Drawer
  must retain its current safe-area and scroll ownership behavior.

## Results

- Consolidated the Logs page to one **Customize bundle** action with
  backend/frontend selected by default and runtime/ACP left optional.
- Added lazy ACP candidate loading, bounded select-all/clear controls, and an
  authorized task-title link that opens each candidate's Kandev task in a new
  tab. Task titles are API-picker metadata only and remain excluded from the
  runtime index, manifest, and archive.
- Widened the desktop customizer without imposing empty fixed-height space; the
  phone flow remains an inset Drawer with one scroll owner and touch-sized
  controls.
- Refined the Logs-page hierarchy with matched card padding, vertically ordered
  privacy/source notes, and a desktop action aligned under the bundle
  description. The phone page retains the same single-column flow and Drawer
  entry point.
- Updated the public operations guide and diagnostic-bundle decision to match
  the consolidated flow.
- Verified with backend package tests (252), focused web tests (28), production
  web build, desktop E2E (3), mobile E2E (2), web typecheck, lint, and i18n
  checks.

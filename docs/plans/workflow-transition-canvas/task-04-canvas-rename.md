---
id: "04-canvas-rename"
title: "Rename a canvas from host chrome"
status: done
wave: 4
depends_on:
  - "03-live-canvas"
plan: "plan.md"
requirements:
  - REQ-CANVASES-NAME-SHARE-001
acceptance_criteria:
  - AC-CANVASES-NAME-SHARE-001.1
  - AC-CANVASES-NAME-SHARE-001.2
  - AC-CANVASES-NAME-SHARE-001.3
  - AC-CANVASES-NAME-SHARE-001.4
system_design:
  - ../../specs/canvases/system-design/canvas-name-and-share-defaults.md
---

# Task 04: Rename a canvas from host chrome

## Summary

Let a workspace-authorized user change the canvas instance title directly
from its host toolbar without publishing a new release.

## In scope

- Add conditional title update in the canvas repository/service, an
  owner-authorized `PATCH /api/v1/canvases/:canvasID`, and a safe
  `canvas.updated` lifecycle event for task and workspace projections.
- Add typed web API and a host-owned name editor beside the desktop toolbar
  title. Expose Rename through the phone action drawer and focused sheet.
- Refresh task picker and workspace navigation titles after rename without
  remounting the running canvas iframe. Localize host copy in all required
  catalogs and update user documentation for canvas management.

## Out of scope

Changing release display names, package IDs, scope, grants, state, or runtime
capabilities; editing inside the canvas application.

## Acceptance

1. Save persists a trimmed valid name across reload/restart; task picker,
   workspace navigation, and host toolbar agree.
2. Invalid, unauthorized, stale, or failed updates retain the old name and
   keep the draft available for correction. Cancel makes no request.
3. Desktop keyboard and phone touch flows work with localized labels,
   predictable focus, 44px phone actions, and no horizontal overflow.

## Verification

```bash
(cd apps/backend && go test ./internal/canvas ./internal/backendapp)
(cd apps/web && pnpm exec vitest run components/settings/canvas-host-components.test.tsx components/settings/canvas-host-route.test.tsx)
(cd apps/web && pnpm e2e:run --host --project=chromium tests/canvas/canvas-host-rename.spec.ts)
(cd apps/web && pnpm e2e:run --host --project=mobile-chrome tests/canvas/canvas-host-rename.spec.ts)
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/backend/internal/canvas/repository.go`, `service.go`, and tests
- `apps/backend/internal/backendapp/canvas_routes.go` and tests
- `apps/web/lib/api/domains/canvas-api.ts`
- `apps/web/components/settings/canvas-host-components.tsx`
- `apps/web/components/settings/canvas-host-route.tsx` and host action files
- `apps/web/src/locales/*/canvases.json`
- `apps/web/e2e/tests/canvas/canvas-host-rename.spec.ts`
- Relevant `docs/public/**` canvas management page

## Dependencies

Task 03 is sequenced first so the example's release is current before host
management checks; the rename contract itself does not depend on its data API.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/canvases/requirements/canvas-name-and-share-defaults.md)
- [System design](../../specs/canvases/system-design/canvas-name-and-share-defaults.md)
- [Complete UI preview](plan.md#ascii-ui-preview)

## Results

Added conditional title update, owner-authorized HTTP PATCH, safe
`canvas.updated` event, and desktop/phone host editors. The host refreshes
metadata without replacing the iframe. Canvas/backend tests, typecheck,
localization checks, focused unit tests, and desktop/phone rename E2E passed.

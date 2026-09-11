---
id: "05-marketplace-ui"
title: "Build canvas marketplace browsing"
status: done
wave: 5
depends_on:
  - "04-registry-catalog"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-004
  - REQ-CANVASES-MARKETPLACE-005
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-002.1
  - AC-PLUGINS-MARKETPLACE-002.2
  - AC-PLUGINS-MARKETPLACE-002.3
  - AC-PLUGINS-MARKETPLACE-002.4
  - AC-CANVASES-MARKETPLACE-002.3
  - AC-CANVASES-MARKETPLACE-004.1
  - AC-CANVASES-MARKETPLACE-004.2
  - AC-CANVASES-MARKETPLACE-004.3
  - AC-CANVASES-MARKETPLACE-004.4
  - AC-CANVASES-MARKETPLACE-004.5
  - AC-CANVASES-MARKETPLACE-004.6
  - AC-CANVASES-MARKETPLACE-005.2
  - AC-CANVASES-MARKETPLACE-005.3
  - AC-CANVASES-MARKETPLACE-005.4
  - AC-CANVASES-MARKETPLACE-006.1
  - AC-CANVASES-MARKETPLACE-006.2
  - AC-CANVASES-MARKETPLACE-006.3
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 05: Build canvas marketplace browsing

## Summary

Add the Canvases tab, visual details, and upload/link/catalog installation to
Plugins settings. Use one review model across desktop and focused phone views.

## In scope

- Workspace selection, catalog cards with required registry cover, search/sort, instance
  actions, and Browse shared canvases from workspace settings.
- Shared plugin/canvas gallery with explicit controls, descriptions, one-image mode, image errors,
  enlargement within details, and keyboard/focus handling.
- Upload and direct-link forms, authoritative package/permission review,
  confirmation, progress/errors, retry, and installed Open canvas action.
- Typed API client and domain hooks with stale-response invalidation.
- Workspace-owner actions independent of native-plugin administrator gating.
- Localized labels/statuses across all catalogs; desktop/mobile E2E.

## Out of scope

Share/export author forms, canvas updates, and provider UI. Native plugin
listings only gain an optional gallery/details action; their direct install remains.

## Acceptance

- UI-01/UI-02 support all three input paths through real inspection and explicit
  workspace permission approval, then show the installed canvas after reload.
- Gallery, failure states, per-workspace copies, and late-response handling work
  without executing the canvas in details or leaking another workspace's data.
- Phone composition, keyboard flow, actual touch targets, source outage,
  localization, and feature-off behavior pass the assigned rendered checks.

## ASCII UI preview

Excerpt of [UI-01/UI-02 in the plan](plan.md#ui-01-canvas-catalog).
AC-CANVASES-MARKETPLACE-002.3, 004.1, 004.2, 005.2, 006.1, and 006.2.

```text
Desktop
Plugins: Installed | Browse | CANVASES
Workspace [Project A v] Search [____] [Install canvas]
[cover + card] [cover + card] -> View details
+------------------------------------------------------+
| [Back] Name / version / author                       |
| [selected image]           Description / license     |
| [Prev] 1/3 [Next] [thumbs]  Required permissions       |
| Workspace [Project A v]           [Review & install]  |
+------------------------------------------------------+

Phone details
+------------------------------+
| [Back] Name                  | fixed
| [selected image]             |
| [Previous] 1/3 [Next]        |
| Description / author         | single scroll
| License / required version   |
| Read / Write / Events        |
| State / exact network hosts  |
| Workspace [Project A v]      |
| Package inspected            |
|------------------------------|
| [Install in Project A]       | fixed, safe-area
+------------------------------+
```

Upload/link opens final review without a gallery or screenshot requirement.
Registry canvas listings require preview URLs; broken remote preview shows Retry
without blocking installation. Inspecting is
announced; stale/expired review requires review again; incompatibility disables
confirm with its reason; success shows Open. Empty/degraded catalogs retain
Install canvas. Phone catalog uses one-column cards. Copy/layout details are
illustrative, action hierarchy and containment are required.

## Verification

Optional plugin gallery excerpt of UI-01, covering
AC-PLUGINS-MARKETPLACE-002.2-002.3:

```text
Browse:  [icon] Plugin name [View details] [Install]
Details: [Back] [Image] [Previous] 1/3 [Next] [Install]
```

On phones, reuse the focused UI-02 detail surface; plugins without previews keep
their existing row. Native installation does not acquire a new review step.

Install dependencies once from `apps/` if not already installed. E2E fixtures
must not skip on missing canvas readiness. Record screenshots/geometry results
from the managed test artifacts and compare with UI-01/UI-02.

```bash
(cd apps/web && rtk pnpm exec vitest run lib/api/domains/canvas-distribution-api.test.ts lib/api/domains/marketplace-api.test.ts hooks/domains/canvas/use-canvas-install.test.ts hooks/domains/plugins/use-marketplace.test.ts components/settings/plugins/canvas-marketplace.test.tsx components/settings/plugins/canvas-marketplace-detail.test.tsx components/settings/plugins/marketplace-preview-gallery.test.tsx components/settings/plugins/plugin-row.test.tsx components/settings/plugins/install-plugin-dialog.test.tsx)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm exec eslint components/settings/plugins lib/api/domains/canvas-distribution-api.ts lib/api/domains/marketplace-api.ts hooks/domains/canvas hooks/domains/plugins/use-marketplace.ts components/settings/workspace-canvases-page.tsx)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/canvas/canvas-marketplace.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/canvas/mobile-canvas-marketplace.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/plugins/plugins.spec.ts -- --retries=0)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-settings-row.spec.ts -- --retries=0)
rtk git diff --check
```

Create `install-plugin-dialog.test.tsx` only for the native compatibility cases
affected by shared entry controls; do not manufacture unrelated tests.

## Files likely touched

- `apps/web/components/settings/plugins/plugins-settings.tsx`
- `apps/web/components/settings/plugins/marketplace-browser.tsx`
- `apps/web/components/settings/plugins/canvas-marketplace.tsx`, `canvas-marketplace-detail.tsx` (new)
- `apps/web/components/settings/plugins/canvas-install-dialog.tsx` (new)
- Shared `marketplace-preview-gallery.tsx` (new), its focused unit tests, and
  existing plugin row/details components and tests that expose optional previews
- `apps/web/components/settings/workspace-canvases-page.tsx`
- `apps/web/lib/api/domains/canvas-distribution-api.ts` and tests (new)
- `apps/web/lib/api/domains/marketplace-api.ts` and tests
- `apps/web/hooks/domains/canvas/use-canvas-install.ts` and tests (new)
- `apps/web/hooks/domains/plugins/use-marketplace.ts` and tests
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/`
- `apps/web/e2e/tests/canvas/canvas-marketplace.spec.ts`,
  `mobile-canvas-marketplace.spec.ts`, shared distribution fixtures (new)

## Dependencies

Tasks 01-04 provide real package/catalog/installation routes. Reuse existing
canvas permission projections and the current native-plugin install dialog.

## Risks

Current plugin management controls are admin-gated; static workspace installs
must not inherit that restriction or weaken native controls. Late package and
workspace responses must not overwrite the review the user is confirming.

## Parallelism

`sequential`

## Inputs

- Design: UI composition, Mobile contract, Catalog projection, Authorization.
- Existing `PluginsSettings`, `InstallPluginDialog`, `MarketplaceBrowser`.
- Existing canvas lifecycle review, `canvas-host-route.tsx`, `useResponsiveBreakpoint`.
- Plugin registry preview contract and current native-plugin install controls.
- `apps/web/AGENTS.md`, /mobile-parity, /e2e, and existing canvas fixture tests.

## Results

Implemented the Canvases marketplace tab, workspace selection, search/category/
sort controls, registry cover cards, shared preview gallery, detail/review
surfaces, upload/direct-link/catalog installation entry points, and localized
desktop/phone layouts with touch-sized controls.

Verification: 8 focused Vitest files passed 21 tests; web typecheck, full lint,
i18n checks, and the new-code ratchet passed; desktop and mobile marketplace E2E
passed; and the existing plugin marketplace regression passed 16 tests.

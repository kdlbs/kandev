---
id: "01-workspace-canvas-discovery"
title: "Align workspace canvas discovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-005
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.7
  - AC-CANVASES-AGENT-WEB-APPS-005.2
  - AC-CANVASES-AGENT-WEB-APPS-005.3
  - AC-CANVASES-AGENT-WEB-APPS-005.7
  - AC-CANVASES-AGENT-WEB-APPS-005.8
  - AC-CANVASES-AGENT-WEB-APPS-005.9
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 01: Align workspace canvas discovery

## Summary

Make the sidebar, workspace cards, settings tree, and tab strip use one
feature-aware canvas discovery contract. Keep the sidebar folded unless the
user explicitly changes it.

## In scope

- Remove route-forced Canvases expansion.
- Preserve explicit sidebar expansion state.
- Add Canvases to the shared workspace tab catalog.
- Remove the separate settings-tree append path.
- Add feature-gated active-canvas counts and responsive summary tiles.
- Render the canvas settings page inside `WorkspaceSettingsShell`.
- Add required translations and feature-off coverage.

## Out of scope

- Canvas creation, panel activation, host controls, and iframe behavior.

## Acceptance

- A fresh or newly enabled client keeps Canvases folded on every route.
- Every workspace settings navigation surface derives Canvases from one
  catalog.
- Narrow cards omit the canvas tile, but the Canvases tab remains reachable.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/app-sidebar/sections/canvases-section.test.tsx components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/settings/workspaces/workspace-settings-shell.test.tsx components/settings/workspaces/workspace-section-links.test.tsx components/settings/workspace-canvases-page.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/app-sidebar/app-sidebar.tsx`
- `apps/web/components/app-sidebar/sections/canvases-section.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
- `apps/web/components/app-sidebar/sections/settings/use-settings-menu-branches.ts`
- `apps/web/lib/settings/workspace-settings-tabs.ts`
- `apps/web/components/settings/workspaces/workspace-section-links.tsx`
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx`
- `apps/web/components/settings/workspace-canvases-page.tsx`
- `apps/web/src/spa-routes.tsx`
- `apps/web/src/locales/**/canvases.json`

## Dependencies

None.

## Risks

- Static catalog imports can bypass runtime feature state.
- Count requests can multiply across workspace cards.

## Parallelism

`sequential`

## Inputs

- Desktop information architecture and feature-gate sections.
- `CANVAS-UX-01` and `CANVAS-UX-02` in the investigation.

## Results

Implemented the feature-aware workspace canvas discovery surfaces:

- kept the Canvases sidebar section folded by default and removed route-forced
  expansion;
- made the shared workspace settings catalog drive the settings tree, tab
  strip, route shell, and responsive workspace summary cards;
- added active valid canvas counts without requesting canvas data when the
  feature is disabled;
- kept the canvas settings route inside `WorkspaceSettingsShell`.

Verification passed:

- focused web suite: 7 files, 69 tests;
- `pnpm run typecheck`;
- `pnpm run lint`;
- `pnpm run i18n:check`.

---
id: "01-center-files-copy-path-icons"
title: "Center Files copy-path icons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PANEL-TOOLBARS-001
acceptance_criteria:
  - AC-UI-PANEL-TOOLBARS-001.1
  - AC-UI-PANEL-TOOLBARS-001.2
  - AC-UI-PANEL-TOOLBARS-001.6
  - AC-UI-PANEL-TOOLBARS-001.7
system_design:
  - ../../specs/ui/system-design/panel-toolbars.md
---

# Task 01: Center Files copy-path icons

## Summary

Add a browser regression for the Files header copy-path button, then center
its folder, copy, and success glyphs within the existing button target. Keep
the clipboard action and responsive target sizing intact.

## In scope

- Rendered icon-center assertions in normal, hovered, and copied desktop
  states, plus normal and copied phone states. Touch has no hover state.
- The smallest local icon-positioning correction in `CopyWorkspacePathButton`.

## Out of scope

- Changing target dimensions, other toolbar controls, tooltip copy, or the
  clipboard implementation.

## Acceptance

1. The visible icon center is within 1px of the button center on both axes in
   normal, hovered, and copied desktop states, and in normal and copied phone
   states. Touch has no hover state.
2. The target remains 24px in fine-pointer desktop and at least 44px square on
   phone; the path row remains contained without overlap or horizontal overflow.
3. Activation still copies the full workspace path and exposes the copied state.

## ASCII UI preview

UI-01 and UI-02 excerpts from [the plan](plan.md#ascii-ui-preview), covering
`AC-UI-PANEL-TOOLBARS-001.7` and the retained toolbar geometry:

```text
Desktop: [   icon   ] /root/...    (24px icon target)
Phone:   [   icon   ] /root/...    (44px icon target)
```

The glyph remains centered in the same target through hover and copied states.
The drawings show hierarchy, not exact text or pixel spacing.

## Verification

First add the geometry assertions and confirm the desktop and phone tests fail
on the current copy/check glyph positioning. Desktop covers the hover glyph;
phone covers the copied check glyph because touch has no hover state. Rebuild
after the component change so Playwright serves the new frontend bundle.

```bash
cd apps/web && pnpm exec vitest run components/task/file-browser-toolbar.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/task/file-browser-toolbar.tsx e2e/helpers/panel-toolbar-geometry.ts e2e/tests/panel-toolbars.spec.ts e2e/tests/mobile-panel-toolbars.spec.ts
cd apps/web && pnpm exec prettier --check components/task/file-browser-toolbar.tsx e2e/helpers/panel-toolbar-geometry.ts e2e/tests/panel-toolbars.spec.ts e2e/tests/mobile-panel-toolbars.spec.ts
make build-web
cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/panel-toolbars.spec.ts -g "centers Files copy path icons in every state"
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/mobile-panel-toolbars.spec.ts -g "centers Files copy path icons before and after copying"
```

## Files likely touched

- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/e2e/helpers/panel-toolbar-geometry.ts`
- `apps/web/e2e/tests/panel-toolbars.spec.ts`
- `apps/web/e2e/tests/mobile-panel-toolbars.spec.ts`

## Dependencies

None.

## Risks

- A CSS test in jsdom cannot validate the SVG's rendered center; use browser
  bounding boxes after the relevant hover and copied transitions.

## Parallelism

`sequential`

## Inputs

- [Panel toolbar requirements](../../specs/ui/requirements/panel-toolbars.md).
- [Panel toolbar design](../../specs/ui/system-design/panel-toolbars.md).
- Existing `CopyWorkspacePathButton` and panel-toolbar Playwright fixtures.

## Results

The regression failed before the fix: the desktop hover glyph was 5px left of
the target center, and the phone copied glyph was 15px left. Adding `m-auto` to
the two absolute overlay glyphs centered both without changing target size.
The phone hover probe confirmed touch emulation has no hover state, so phone
coverage verifies the normal and copied states after a real tap.

Passing checks:

- `pnpm exec vitest run components/task/file-browser-toolbar.test.tsx`
- `pnpm run typecheck`
- Focused ESLint and Prettier checks for the changed component and E2E files.
- `make build-web`
- `pnpm e2e:run --host --no-build --project chromium tests/panel-toolbars.spec.ts -g "centers Files copy path icons in every state"`
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/mobile-panel-toolbars.spec.ts -g "centers Files copy path icons before and after copying"`

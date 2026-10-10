---
created: 2026-09-29
status: complete
requirements:
  - REQ-UI-PANEL-TOOLBARS-001
system_design:
  - ../../specs/ui/system-design/panel-toolbars.md
legacy_specs: []
---

# Implementation Plan: Files copy-path alignment

## Overview

Center the Files toolbar's copy-path icon in the action target, including its
hover and copied states. One work order covers the visual regression and the
small component correction.

## Root cause and reproduction

The user-provided screenshot shows the copy-path glyph toward the upper-left
of the Files toolbar action. `CopyWorkspacePathButton` places its folder glyph
in a centered flex button, but gives the 14px copy and check glyphs absolute
positioning with `inset-0`. That combination places those smaller SVGs at the
button's top-left rather than its center. Hover the Files header copy-path
button, or click it and observe the success glyph, to reproduce the affected
states. The button's target itself remains in the correct toolbar row.

## Scope

### In scope

- Center the folder, copy, and check glyphs inside the existing copy-path target.
- Preserve desktop and touch target dimensions, tooltip, accessible name, and
  clipboard behavior.
- Add rendered browser geometry coverage for the icon states on desktop and phone.

### Out of scope

- Other Files actions, toolbar sizing, path text, navigation, and clipboard semantics.

## Technical approach

Update `CopyWorkspacePathButton` in
`apps/web/components/task/file-browser-toolbar.tsx` so its overlaid glyphs
share the button's horizontal and vertical center. Extend the existing
`apps/web/e2e/tests/panel-toolbars.spec.ts` and
`apps/web/e2e/tests/mobile-panel-toolbars.spec.ts` to compare the visible SVG's
bounding-box center with its button center in normal, hovered, and copied
desktop states, and normal and copied phone states. Keep the 24px fine-pointer
and 44px touch target rules intact.

## ASCII UI preview

UI-01: Files path row, desktop (normal, hover, copied). The icon stays centered
inside the existing 24px target; the path truncates within the same fixed row.

```text
Current hover/copied:       Proposed normal/hover/copied:
+----------------------+    +----------------------+
| [icon               ] |    | [        icon        ] |
| /root/...            |    | /root/...            |
+----------------------+    +----------------------+
```

UI-02: Files path row, phone. The same icon alignment applies within its 44px
touch target. The row stays outside the file-tree scroll area.

```text
+--------------------------------------+
| [        icon        ] /root/... [...] |
+--------------------------------------+
| file tree (scrolls)                  |
```

Spacing is illustrative. The structural requirement is that the visible glyph
and its target share horizontal and vertical centers across states and viewport
sizes. UI-01 and UI-02 map to `AC-UI-PANEL-TOOLBARS-001.7`; retained target
geometry maps to `.1`, `.2`, and `.6`.

## Tests

The existing `file-browser-toolbar.test.tsx` exercises toolbar actions. This
repair changes rendered geometry, so the regression belongs in Playwright.

## E2E tests

- `apps/web/e2e/tests/panel-toolbars.spec.ts`: add a focused desktop test named
  `centers Files copy path icons in every state`, covering `.7` and the retained
  compact target. It must fail on the current offset copy/check glyphs.
- `apps/web/e2e/tests/mobile-panel-toolbars.spec.ts`: add a focused phone
  scenario covering the normal and copied glyphs, `.2`, `.6`, and `.7`, including
  the 44px target. Touch has no hover state; the desktop scenario covers hover.

## Work orders

- [x] [Task 01: Center Files copy-path icons](task-01-center-files-copy-path-icons.md)

## Verification results

- `pnpm exec vitest run components/task/file-browser-toolbar.test.tsx` passed
  (1 file, 8 tests).
- `pnpm run typecheck` passed.
- Focused ESLint and Prettier checks passed for the changed component and E2E
  files.
- `make build-web` passed.
- The focused desktop and phone Playwright tests passed against the rebuilt
  frontend. Desktop covers normal, hover, and copied states; phone covers
  normal and copied states.

## Risks

- Both overlay icons must remain centered during opacity transitions; checking
  only the initial folder glyph would miss the reported defect.
- Phone sizing uses the same component with a larger target, so a pixel offset
  tuned only to desktop would reintroduce the issue on touch layouts.

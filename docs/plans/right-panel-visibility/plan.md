---
created: 2026-09-13
status: implemented
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/right-panel-visibility.md
legacy_specs: []
---

# Implementation plan: Right panel visibility

## Overview

Add a persistent right-panel toggle to the task header on desktop and tablet.
Deliver one sequential vertical work order, including both layout adapters and browser coverage.
The package is implemented and its verification is recorded below.

## Evidence and root cause

Source revision: `bc38ea1ec7`; package commit: `f05aaee8f0`. Investigation date: 2026-09-13.
Source: [issue 3657](https://github.com/kdlbs/kandev/issues/3657), its body image, and the maintainer comment image.
Both images were inspected. GitHub authenticated user: `carlosflorencio`; issue assigned to that user.

The read-only trace establishes four relevant facts:

1. `RightTopGroupActions` lives in the top-right Dockview group and is the only caller of `toggleRightPanels` outside the store.
2. Hiding removes that group, so its control disappears. `TopbarToolsGroup` has no corresponding restore button.
3. `SessionTabletLayout` reads column state but always renders the right Panel.
4. `buildVisibilityActions` blocks reopening when the responsive default preset is compact.

Minimal reproduction: open a Default task workbench, activate Hide right panels, then look for the reverse header action.
The hidden group takes its control with it. Restoring a preset is an indirect workaround, not a paired toggle.
For the tablet fallback, a stored `right: false` still renders Files and Terminal.

Existing evidence: `pnpm exec vitest run lib/state/dockview-preset-persistence.test.ts` passed all 23 tests.
Those tests establish the existing store behavior; they do not prove the proposed UI.
No live user instance was mutated. Browser reproduction and new regression tests belong to implementation.

## Scope

### In scope

- Persistent localized header control and active-layout state adapter.
- Desktop, compact desktop, large touch tablet, and narrow tablet fallback.
- Independent left navigation; readiness and restoration handling.
- Focused mixed-layout protection required by the newly exposed control.
- Phone parity checks and public usage instructions with implementation.

### Out of scope

- Backend changes, new persistence, release flags, new breakpoint rules, and arbitrary custom-panel snapshots.
- Left-sidebar redesign, phone sidebars, Office-specific workbench changes, and terminal lifecycle changes.

## Technical approach

Follow the [system design](../../specs/ui/system-design/right-panel-visibility.md).
Reuse both existing layout stores through one active-layout adapter.
Keep the existing Dockview reconstruction behavior and tablet split storage.
The UI system owns this interaction independently of task business state and reusable layout-profile management.
Existing task-layout-profile packages are related context; their accepted profile behavior is unchanged.

## ASCII UI preview

### UI-01: Desktop and tablet task header

Entry: open a task. The task header stays fixed; panel bodies own scrolling.

```text
BEFORE: current Dockview
+----------------------------------------------------------+
| Task title                         [Layouts] [Editor]     |
+---------------------------------+------------------------+
| Chat                            | Files       [Hide >]   |
|                                 | Terminal               |
+---------------------------------+------------------------+
After Hide: the right column AND its Hide button disappear.

AFTER: right panels visible
+----------------------------------------------------------+
| Task title                    [R >] [Layouts] [Editor]    |
+---------------------------------+------------------------+
| Chat                            | Files / Changes        |
|                                 +------------------------+
|                                 | Terminal               |
+---------------------------------+------------------------+
[R >] = Hide right panels

AFTER: right panels hidden
+----------------------------------------------------------+
| Task title                    [< R] [Layouts] [Editor]    |
+----------------------------------------------------------+
| Chat now uses the released width                         |
|                                                          |
+----------------------------------------------------------+
[< R] = Show right panels; same position, one tap to restore.
```

The existing left-sidebar toggle remains independent, outside this cropped region.
The coarse-pointer tablet fallback uses the same header control and hides Files plus Terminal.
Its Chat/Plan/Changes tabs remain in the left content surface.
Compact desktop starts with its existing single-group default; explicit Show adds a right column.
During initialization or restoration, the same control is disabled and keeps its position.

### UI-02: Phone task navigation

Entry: task below 768 CSS pixels. Retain the shipped full-screen panel composition.

```text
+------------------------------------+
| Task context                       |
+------------------------------------+
| Chat OR Files OR Terminal          |
| One active content surface         |
|                                    |
+------------------------------------+
| Chat | Plan | Changes | Files | >_  |
+------------------------------------+
```

The bottom row is an excerpt of existing navigation; other existing destinations remain available.
Select Files or Terminal, then Chat to return. There is no right-sidebar toggle on phones.
Keep existing safe areas and content scrolling; do not overwrite wider-layout preferences.

Required structure: persistent control order, independent sidebars, reclaimed width, and separate phone navigation.
Spacing and ASCII glyphs are illustrative. Use existing tokens and localized labels.
UI-01 maps to AC-UI-RIGHT-PANEL-VISIBILITY-001.1 through .5 and .7; UI-02 maps to .6.

## Tests

| Criteria | Test file and coverage |
| --- | --- |
| .1, .4, .7 | `components/task/task-right-panels-toggle.test.tsx`: localized next-action labels, focus retention after activation, and an explanatory disabled maximized state |
| .1, .3, .5, .7 | `hooks/use-task-right-panels-toggle.test.ts` and `lib/state/dockview-env-switch-action.test.ts`: desktop/tablet routing, empty-session and phone guards, A/B visibility round trips, and saved-maximize restoration |
| .2, .3, .5 | `lib/state/layout-manager/serializer.test.ts` and `lib/state/dockview-right-panel-visibility.test.ts`: production-shaped compact and mixed capture, right-column ownership, compact reopening, center preservation, and globally unique panel IDs on Show |
| .2, .3, .5 | `components/task/mobile/session-tablet-layout.test.tsx`: stored hidden right column is not rendered and the center surface remains mounted; browser coverage owns persistence assertions |
| .1, .2, .5, .7 | `lib/state/dockview-right-panel-visibility.test.ts`: maximize blocks mutation, exit restores the authoritative visibility, and regular layout persistence resumes |

Criteria refer to `AC-UI-RIGHT-PANEL-VISIBILITY-001`.
First behavioral RED: `stored hidden right column is not rendered` against the current tablet component.
A second behavioral RED covers compact reopening. Missing selectors alone do not qualify as behavioral RED.

## E2E tests

The executed `apps/web/e2e/tests/layout/right-panel-visibility.spec.ts` coverage uses the `chromium` project for desktop, compact desktop, and the 900-pixel coarse-pointer tablet fallback. It asserts desktop hide/show, center-width recovery, hidden and visible desktop reloads, compact hidden-state reload and reopening, maximized disabled-state and exit/reload recovery, tablet hide/show persistence, and keyboard activation with focus retention.

The executed `apps/web/e2e/tests/layout/mobile-right-panel-visibility.spec.ts` coverage uses the `mobile-chrome` project with Pixel 5 device settings to assert the existing full-screen Chat, Files, and Terminal navigation, coarse-pointer input, and the absence of the wider-layout toggle on phones.

The following browser scenarios remain the planned regression matrix. They are retained here because the current implementation run does not assert every case:

- Desktop Default: all four left/right visibility combinations, no duplicate panels, and header reachability with a long title and linked review.
- Touch tablet at both 1280x900 and 900x900: pointer mode, 44-pixel targets, and both desktop and tablet adapters.
- Fine pointer at 900x800: compact default, explicit Show, Hide, and Show again.
- Cross-task and cross-viewport handoffs: resize across 1024 and 768 boundaries without copying state between stores.
- Archived tasks and repeated activation during restoration.
- Mixed center Agent/Files/PR Details plus a separate right column through a browser fixture; the production-shaped capture and store cases cover the logic below the browser layer.
- Phone navigation after crossing into a wider view and back while preserving wider-layout visibility.

Reuse `pane-persistence-tablet.spec.ts` and `compact-desktop-responsive.spec.ts` for the retained tablet split and compact geometry cases. All executed browser checks use causal waits and owned fixtures.

## Work orders

  - [x] [Task 01: Persistent toggle](task-01-persistent-toggle.md) (done)

## Verification results

Implementation is complete. The new control is shared by desktop and tablet adapters, the tablet right column is conditional, compact desktop can reopen it, and phone navigation keeps its existing full-screen composition.

Checks passed:

- `pnpm install --frozen-lockfile` from `apps`.
- Review-focused unit tests after fixup: 9 files, 102 tests; the store-focused follow-up passed 45 tests in 2 files.
- Full web unit suite: 2,058 files, 17,788 passed and 4 skipped tests.
- `pnpm run typecheck` and `pnpm run lint` from `apps/web`.
- Targeted ESLint for changed source and browser files.
- Prettier check for changed TypeScript, TSX, and JSON files.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`.
- `pnpm --filter @kandev/web build:vite` from `apps`.
- Managed Chromium E2E fixup run: 5 right-panel tests passed, including compact reload, maximized disabled/exit/reload, tablet persistence, and keyboard activation.
- Managed mobile-chrome E2E fixup run: 1 test passed with Pixel 5 device and coarse-pointer assertions.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published documents validated.
- `python3 scripts/list-docs.py validate`: 267 decisions and 896 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

The implementation adds localized labels in all five supported catalogs and updates the public task-workspace instructions.

The assertions above cover the review regressions. The retained browser matrix is broader than this run. The tablet component test uses mocked panel primitives and persistence callbacks, so it proves conditional composition and center identity only; the browser test proves the stored visibility round trip, while saved split geometry remains in the retained matrix. The component test proves focus retention after a click and the disabled maximized accessibility wrapper; the browser test proves native Enter and Space activation plus maximized exit/reload recovery. Resize handoffs, archived-task restoration, the 1280-pixel coarse-pointer case, all four sidebar combinations, mixed center/right browser fixtures, and the wider-to-phone handoff remain planned coverage.

New files were inspected explicitly; work-order references resolve to the new requirement and design.
Implementation commands and browser results are recorded above and in the completed work order.

## Risks

- Existing hide logic can remove mixed columns; protect center content before exposing the control globally.
- Existing Show rebuilds the standard right column; custom right tabs are not an exact snapshot restore.
- Conditional tablet panels can overwrite saved split geometry unless single-panel saves are excluded.
- Header descendant sizing can override a touch button's hit area.
- Device-local desktop and tablet restoration have different scopes; avoid cross-store writes during resize.

## Public documentation

Added a short task-workspace how-to to `docs/public/tasks-and-workflows.md`.
It explains Hide/Show, independent sidebars, tablet support, and existing phone navigation.

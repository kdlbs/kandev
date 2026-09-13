---
created: 2026-09-13
status: draft
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
This package is ready for review. Production implementation remains pending.

## Evidence and root cause

Source revision: `bc38ea1ec7`. Investigation date: 2026-09-13.
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

| Criteria | Test file and proposed case |
| --- | --- |
| .1, .4, .7 | `components/task/task-right-panels-toggle.test.tsx`: label follows visibility; keyboard retains focus; unavailable layout disables action |
| .1, .3, .5, .7 | `hooks/use-task-right-panels-toggle.test.ts`: routes desktop and tablet actions; rejects empty session; phone does not mutate layout; restored state updates label |
| .2, .3 | `lib/state/dockview-preset-persistence.test.ts`: compact hidden layout can show; mixed center keeps Agent and PR Details when hiding |
| .2, .3, .5 | `components/task/mobile/session-tablet-layout.test.tsx`: stored hidden right column releases width; toggling preserves center identity and saved two-panel geometry |

Criteria refer to `AC-UI-RIGHT-PANEL-VISIBILITY-001`.
First behavioral RED: `stored hidden right column is not rendered` against the current tablet component.
A second behavioral RED covers compact reopening. Missing selectors alone do not qualify as behavioral RED.

## E2E tests

New `apps/web/e2e/tests/layout/right-panel-visibility.spec.ts`, project `chromium`:

- Desktop Default: hide/show, released width, all four left/right visibility combinations, focus, and no duplicate panels (.1, .2, .4).
- Touch tablet at 1280x900 and 900x900: use `tabletTestPage`; assert pointer mode, 44px targets, and both adapters (.1-.4).
- Fine pointer at 900x800: compact default, explicit Show, Hide, and Show again (.3).
- Hidden reload and visible reload; tablet split restoration; switch tasks and return (.5).
- Resize across 1024 and 768 boundaries; retain each layout's state without copying it to another store (.5, .6).
- Header with a long title and linked review; no overflow, control remains reachable (.4, .6).
- Archived task with mounted workbench; initialization and repeated activation during restoration (.1, .7).
- Mixed center Agent/Files/PR Details and separate right column: preserve unrelated center panels (.2).

New `apps/web/e2e/tests/layout/mobile-right-panel-visibility.spec.ts`, project `mobile-chrome`:

- Open Files and Terminal from existing navigation, return to Chat, inspect phone composition and overflow (.6).
- Cross into a wider view and back; phone navigation leaves wider-layout visibility intact (.5, .6).

Reuse `pane-persistence-tablet.spec.ts`, `compact-desktop-responsive.spec.ts`, and their fixtures.
Capture rendered desktop and phone views during these checks and compare them with UI-01/UI-02.
Use causal waits and owned fixtures, not arbitrary sleeps or the user's running instance.

## Work orders

- [ ] [Task 01: Persistent toggle](task-01-persistent-toggle.md)

## Verification results

Investigation: existing store suite passed, 23 tests.

Package checks passed:

- `python3 scripts/list-docs.py validate`: 267 decisions and 896 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/right-panel-visibility`: passed.
- Catalog discovery includes both new specification files.
- GitHub issue assignee readback: `carlosflorencio`.

New files were inspected explicitly; work-order references resolve to the new requirement and design.
Implementation commands and browser results: pending.

## Risks

- Existing hide logic can remove mixed columns; protect center content before exposing the control globally.
- Existing Show rebuilds the standard right column; custom right tabs are not an exact snapshot restore.
- Conditional tablet panels can overwrite saved split geometry unless single-panel saves are excluded.
- Header descendant sizing can override a touch button's hit area.
- Device-local desktop and tablet restoration have different scopes; avoid cross-store writes during resize.

## Public documentation

With implementation, add a short task-workspace how-to to `docs/public/tasks-and-workflows.md`.
Explain Hide/Show, independent sidebars, tablet support, and existing phone navigation.
Do not publish future behavior during this design-only change.

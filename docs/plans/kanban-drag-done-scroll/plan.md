---
created: 2026-11-06
status: implemented
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-003
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
legacy_specs: []
---

# Fix plan: Kanban Done drag scrolling

## Overview

Keep the final workflow column under the pointer when a user drags a task over it.
One sequential work order owns the regression, bounded horizontal scrolling, and rendered verification.
The existing [Adaptive Kanban requirements](../../specs/ui/requirements/adaptive-kanban.md) remain authoritative.
No new requirement or acceptance ID is needed.

## Evidence and root cause

The official v0.98.0 Linux release reproduces the defect on a fresh database.
Its commit is `2d93502c256780e1f3d9df4348dafbd97b584188`. The archive checksum passed.
A default four-column Development workflow contains one inert task in Backlog.
The test used headless Chromium 156.0.8078.4, a visible sidebar, and no code patches.

1. At 1440 by 900, drag the task toward the original center of Done and hold there.
2. Drag activation increases the lane's physical scroll width from 1120px to 1928px.
3. The lane scrolls 840px. Done moves to the left edge and the other three columns leave the viewport.
4. Release at the original target. The task remains in Backlog. The columns return when the drag ends.

At 1920 by 900 the same gesture scrolls 1176px and fails.
All four columns remain mounted. The baseline drag produced no page error.
The failure comes from dnd-kit auto-scroll entering the trailing drag reserve.
Mandatory horizontal CSS snap makes the movement abrupt.

Browser-only comparisons confirmed the cause:

| Comparison | Result |
| --- | --- |
| Remove snap only | Movement becomes gradual. Drop still fails after holding. |
| Remove trailing reserve | The 1440px drop succeeds with at most 32px of actual-content travel. |
| Disable board scrollBy | The 1920px drop succeeds. This is diagnostic evidence, not the production repair. |
| Card menu: Move to, Done | The saved task step changes to Done. |

Session-local evidence is under the ignored `.playwright-cli/kanban-drag-0980/` directory.
It includes `reproduction.json`, `comparisons.json`, baseline screenshots, scripts, and a diagnostic ZIP.
The diagnostic manifest reports both requested sources ready with no warnings.
Its empty frontend log is not evidence that the geometry failure did not occur.
The measurements above make this package usable without the ignored files.

## Requirement conformance and ownership

UI owns this repair because the failed contract is reusable board geometry and drag-time scrolling.
Tasks retains move validation, ordering, optimistic reconciliation, and persistence.
AC-UI-ADAPTIVE-KANBAN-001.4 already requires visible-column drag moves in fitting and windowed boards.
AC-UI-ADAPTIVE-KANBAN-001.9 and 001.10 protect column width and contained anchor space.
AC-UI-ADAPTIVE-KANBAN-003.9 protects auto-scroll and anchor geometry.
The repair corrects their implementation. It does not redefine product behavior.

No material user choice remains. The user requested the fix after reproduction.
The user subsequently requested implementation with "go for impl". That request satisfied the repository planning checkpoint.
No delegation is authorized.

## Scope

### In scope

- Separate automatic horizontal travel from programmatic anchor space.
- Keep final-column hit targets stable during a stationary hover.
- Preserve offscreen and temporary destinations, cancellation, reverse travel, and vertical auto-scroll.
- Preserve widths, document containment, and desktop/phone composition.

### Out of scope

- Task move APIs, workflow permissions, ordering rules, or backend changes.
- Removing the reserve, changing its sizing, or changing the source-anchor formula.
- New settings, dependencies, translated copy, or visual styling beyond drag-time snap control.
- Tablet scrolling redesign, native webview certification, commit, push, or PR publication.

## Technical approach

Follow [Automatic scrolling and anchor space](../../specs/ui/system-design/adaptive-kanban.md#automatic-scrolling-and-anchor-space).
Retain `KANBAN_DRAG_END_RESERVE` and the separate lane grid.
In `AdaptiveDesktopKanban`, disable snap while `isDragging` or background panning is active.
A small local `use-kanban-horizontal-drag-scroll.ts` hook receives active drag state and the viewport/grid refs.
It owns a bounded horizontal frame loop. Existing dnd-kit code still owns sensors, hit testing, overlay, drop, and vertical auto-scroll.
`KanbanDragSurface` uses the supported `canScroll` option to exclude only desktop board windows from built-in scrolling.
Do not monkey-patch native scroll methods or import dnd-kit private helpers.

The hook reads live geometry and edge intent. It clamps each rightward movement to the lane grid's end, not the physical reserve.
Leftward movement can return from a programmatic anchor beyond that end without jumping immediately to the normal bound.
Keep the current proportional edge-zone behavior where practical. Use elapsed time and a capped frame delta.
Attach document input listeners only while the accepted task drag is active.
Use `useDndMonitor` to begin capture synchronously at accepted drag start.
Seed coordinates from the activation event so a stationary edge pickup also scrolls.
Stop on drop, cancellation, unmount, or a layout branch change. No listener or frame may survive its owner.

## UI preview

These structural previews use prose because the task requires native rich output instead of ASCII diagrams.
The same labelled views are rendered in the task conversation.

**UI-01: Desktop Home, four-column board, task held over Done.**
Before: Done moves to the left. Backlog, In Progress, and Review leave the viewport. The pointer lies over blank space.
After: the four columns keep their widths. A fitting board does not move. An overflowing board moves only enough to reveal actual columns.
Done remains under the stationary pointer. Releasing moves the task to Done.
The sidebar and workflow header keep their existing positions. The lane owns horizontal scrolling and columns own vertical scrolling.
Maps to AC-UI-ADAPTIVE-KANBAN-001.4, 001.9, 001.10, and 003.9.

**UI-02: Desktop Home, long workflow and auto-hidden destinations.**
The board reveals eligible temporary columns during dragging.
Edge travel reaches real columns in either direction. Programmatic source anchoring can still use reserve space.
Cancellation removes the temporary columns and reserve. Normal snapping returns.
Manual hiding remains authoritative. Maps to AC-UI-ADAPTIVE-KANBAN-001.3, 001.4, and 003.9.

**UI-03: Phone Home, focused workflow and column.**
The existing `MobileColumnTabs` navigator remains above the task list.
The task list remains the primary vertical scroll region. The visible card menu provides Move to and destination selection.
The full-height list retains dynamic viewport sizing, safe areas, and existing touch targets.
No desktop board controller mounts. Card swipes scroll and do not initiate dragging.
This reuses the phone composition under AC-UI-ADAPTIVE-KANBAN-003.7 and the linked phone scrolling contract.

No control order, label, density, surface, or primary action changes.
The short destination choice keeps its shipped menu surface. Primary task content remains inline in the focused column.
Shared move actions and filtering stay shared. Desktop geometry remains local and never changes saved view preferences.

## Tests

| Criteria | Evidence |
| --- | --- |
| 001.4, 003.9 | Proposed hook tests cover fitting and overflowing grids, reverse travel, live resize, and current anchor beyond content end. |
| 001.4, 001.9, 001.10 | Existing adaptive desktop tests retain grid/reserve assertions and add drag-time snap restoration after cancellation. |
| 003.9 | Drag surface tests prove only the desktop window is excluded from dnd-kit auto-scroll. Column, outer-container, and tablet paths remain eligible. |
| 003.9 | Hook lifecycle tests prove input/listener/frame cleanup after drop, cancellation, unmount, and branch changes. |

## E2E tests

Extend `apps/web/e2e/tests/kanban/auto-hide-empty-columns.spec.ts` instead of creating a separate suite for the same feature.
The primary RED test is named `keeps Done under a held drag at the final column` and runs at 1440px and 1920px.
Use real pointer movement, unchanged target coordinates, and `dwell` for a 1200ms negative-observation window.
Sample target hit testing and scroll bounds throughout the hold, not only after it.
Release and prove the saved step, visible card, and reload persistence.
A missing selector or setup timeout is not behavioral RED.

A second scenario uses at least six columns. Real edge input reveals the final destination.
Reverse toward the first column within the same drag and verify reverse travel, then complete a move.
Keep existing width, reserve, temporary destination, manual-hide, and cancellation assertions.
Cover source anchoring with temporary columns inserted before the source.
Record the source position immediately before activation and compare it after the temporary columns render.
Also prove an overflowing vertical task column still auto-scrolls while dragging.

Run existing `swimlane-scroll-affordances.spec.ts` with the changed desktop suite.
Run `mobile-auto-hide-empty-columns.spec.ts` and `mobile-kanban-reorder.spec.ts` in `mobile-chrome`.
Those existing scenarios prove menu moves, native card swipes, no drag pickup, and the 768px boundary.
No new phone composition or phone E2E file is required.
Inspect desktop drag and phone focused-column captures against UI-01 through UI-03.
Native webviews and browsers outside the named projects remain unverified.

## Work orders

- [x] [Task 01: Bound desktop drag scrolling](task-01-bound-desktop-drag-scroll.md)

One work order runs sequentially. No parallel agent work is planned.

## Companion packages

The completed adaptive-kanban, kanban-drag-column-width-stability, kanban-large-column-virtualization,
kanban-swimlane-height, and kanban-swimlane-scrolling packages are historical delivery records.
Their width, reserve, virtualization, and scrolling acceptance conditions remain in force.
This repair adds the held-final-column scenario. Historical test counts are not current verification evidence.
Do not rewrite completed delivery results or remove their existing assertions.

## Verification results

Implemented on 2026-11-06. The work order records RED evidence and exact command scope.

- All 46 focused unit/component tests passed, including real swimlane render and keyboard consumers.
- All 12 desktop/tablet browser tests passed on the final production build without retries.
- The three screenshot-enabled desktop cases passed after the final test-only capture edit.
- All six phone browser tests passed without retries.
- Final type checking, scoped lint, the i18n ratchet, catalog validation, specification lint, and diff whitespace checks passed.
- Rendered desktop and phone checks match UI-01 through UI-03. No unresolved composition difference remains.

Source capture starts synchronously through `useDndMonitor`. Drag snap suppression also uses conditional classes because overlay mouse leave clears the pan handler's inline style.
The existing anchor reserve and shared pan hook remain intact.
No temporary tracing remains. Test fixtures stopped their owned processes. Changes remain uncommitted.

Package checks passed on 2026-11-06:

- `python3 scripts/list-docs.py validate`: 436 decisions and 1928 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local link checks across the three changed/new documents: no missing targets.
- `git diff --check`: passed.

Application verification is complete. Native desktop webviews and browsers outside the named Chromium projects were not tested.

## Documentation

Internal docs updated: the existing system design and this delivery package.
No public guide, command, setting, label, screenshot catalog, or supported workflow changes.
No ADR is required for this local correction within the existing board ownership boundary.

## Risks

- Removing anchor space breaks source positioning when temporary destinations appear.
- A shared auto-scroll exclusion can accidentally disable vertical or tablet scrolling.
- Cached geometry can admit blank travel after resizing or changing temporary steps.
- Snapshot-only tests miss movement during a held pointer. Real geometry and hit testing are required.
- Native desktop webviews can differ from Chromium. Do not claim coverage without a separate run.

## PR review follow-up

Replaced test-ID-based scroll ownership with a dedicated production data attribute. Added a rename regression and vertical-owner geometry checks. Post-review validation passed: 47 focused tests, seven affected desktop browser cases on a fresh production build, typecheck, scoped lint, and specification checks. Remote CI and automated reviews remain pending for the follow-up head.

CodeRabbit follow-up retains capture-phase coordinates across sensor activation and completes the registry-ordering monitor stub. Local verification passed: 50 focused tests, all 12 desktop/tablet browser tests, typecheck, and scoped lint. Exact-head remote CI and review evidence remain pending after the follow-up push.

Summary-review follow-up: the snap-restoration test rerenders the same board and checks its inline style and snap classes. Secondary touch end/cancel events no longer stop horizontal scrolling while another touch remains. Both touch regressions failed before the correction. Local verification passed: 52 focused tests, all 12 desktop/tablet browser cases on a fresh build, typecheck, and scoped lint. Remote results remain pending for this follow-up head.

Claude follow-up: the bounded-scroll attribute name is exported by the controller and shared by the desktop viewport and drag surface. The desktop test verifies that the rendered viewport carries this attribute. This preserves the existing DOM contract.

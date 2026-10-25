---
created: 2026-09-11
status: complete
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-002
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
legacy_specs: []
---

# Fix plan: Compact workflow swimlanes

## Overview

Give sparse workflows compact heights while preserving bounded virtualized columns. One sequential work order owns the regression, correction, and focused verification.
The UI system owns this presentation contract. Task and workspace state remain under their existing owners.

## Evidence and root cause

The user screenshot shows a sparse first workflow occupying most of the board, with another workflow header at the bottom.
The source trace identifies the height allocation responsible for this geometry:

1. `SwimlaneContainer` passes `fillHeight={view.id === "kanban"}` for all Kanban workflows.
2. `WorkflowItems` enables that value for each expanded workflow.
3. `SortableWorkflowItem` applies `h-full min-h-0 flex-1` inside a desktop block container.
4. `SwimlaneSection` propagates the definite parent height through its content.

Thus every expanded lane receives a board-sized area regardless of its task count.
The separate `sm:min-h-[200px]` in `KanbanColumn` is not the cause of viewport-sized blank space.
Commit `6a4ba4cbc8` introduced the desktop full-height path for large-column virtualization.
Removing the entire height chain risks unbounded virtualized content or an empty viewport.

Evidence is a read-only source/history trace plus the supplied screenshot. No live browser reproduction ran during package preparation.
The minimal browser reproduction uses All Workflows, two expanded workflows, and one short card in each workflow.
At 1440 by 900, the second header and its first card must fit without outer scrolling after the correction.

## Requirement conformance

The existing Adaptive Kanban contract covers width and virtualization but lacks a multi-workflow height rule.
`REQ-UI-ADAPTIVE-KANBAN-002` adds that missing rule in the existing requirement document.
The implementation uses the approved 400px maximum. The 200px minimum preserves the existing column floor.
The paired design is current after implementation and rendered verification.

## Scope

### In scope

- Content-sensitive compact heights for multiple desktop and tablet lanes.
- Full-height single-workflow and phone views.
- Collapsed and retained-empty lane recovery.
- Bounded virtualization and unchanged card interactions.

### Out of scope

- Manual lane resizing or a saved height preference.
- Pipeline, List, or task lifecycle redesign.
- Backend, dependency, localization, and public documentation changes.
- Commits and publication.

## Technical approach

Use the mode and measurement boundaries in the [system design](../../specs/ui/system-design/adaptive-kanban.md#workflow-height-allocation).
Keep all sizing state local. Reuse virtualizer totals and actual column chrome measurements.
Keep the lane height fixed during active task drag. Recompute after drag completion.
Preserve callback identity and update the existing column memo comparator for new props.

## ASCII UI preview

UI-01: Desktop Home, All Workflows, two sparse expanded lanes.

```text
Before                         After
[Workflow A / Columns]         [Workflow A / Columns]
[Todo] [Plan] [Done]            [Todo] [Plan] [Done]
[card]                        [card]
|                             | compact column area
| full board height           [Workflow B / Columns]
|                             [Todo] [Review] [Done]
|                             [card]
[Workflow B / Columns]
```

UI-02: Desktop mixed state and phone Home.

```text
Desktop                        Phone
[Workflow A > / Columns]       [Workflow / Step v]
[Workflow B v / Columns]       [card]
[Todo] [Plan] [Done]            [card]
[card]                        | focused column scroll
| dense column scroll         | fills available height
| bounded column area         [Create task]
```

Header order, compact multiple lanes, and focused phone composition are structural requirements.
Spacing and labels are illustrative. UI-01 covers 002.1-002.2. UI-02 covers 002.3 and 002.6.
An empty retained lane keeps its header and existing empty guidance. A collapsed lane contains only its header.
Tablet keeps its existing two-column horizontal snap view inside each compact lane.

## Tests

The primary RED test is browser geometry, not class-name presence.
A small measurement reducer or height helper needs unit coverage only if implementation extracts one.
Existing render-isolation tests protect stable lane and card updates.

## E2E tests

| Criteria | File and scenario |
| --- | --- |
| 002.1-002.2 | New `swimlane-height.spec.ts`: two sparse lanes fit, mixed dense lane caps, last task reachable. |
| 002.3-002.5 | Same file: collapse/expand, filter to one lane, clear filter, retained-empty recovery, preview resize. |
| 002.6 | Existing `mobile-kanban.spec.ts`: add full-height focused-lane geometry with multiple workflows and breakpoint transitions. |
| 002.1, 002.7 | New `swimlane-height.spec.ts`: coarse-pointer tablet case through `tabletTestPage`. |
| 001.5-001.7, 002.7 | Existing desktop/tablet/mobile large-column suites plus a 440-task lane beside a sparse lane. |
| 001.4, 001.9-001.10, 002.7 | Existing board, auto-hide, WIP, and workflow-sorting suites. |

All named files live under `apps/web/e2e/tests/kanban/`, except `workflow/workflow-sorting.spec.ts`.
Desktop and tablet use the `chromium` project. Phone uses `mobile-chrome`.
The new phone assertions use the canonical device, then 767px and 768px with the same pointer mode.
They prove branch selection and preserve the saved desktop view.

## Companion packages

The completed adaptive-kanban, kanban-large-column-virtualization, and kanban-drag-column-width-stability packages remain historical delivery records.
This package adds height coverage and retains their width, drag, and virtualization acceptance conditions.
Their recorded test counts are historical, not verification results for this repair.

## Work orders

- [x] [Task 01: Compact workflow heights](task-01-compact-workflow-heights.md)

## Verification results

Implemented and verified on 2026-09-11.

- The sparse-workflow geometry test failed before production changes and passed after the correction.
- The final managed build passed for the backend, Vite assets, and fixture plugin.
- All 22 targeted Chromium tests and 29 mobile tests passed without retries on the final build.
- All 14 focused hook and render-stability tests passed.
- Typecheck, targeted ESLint, Prettier, and the i18n ratchet passed.
- All 36 specification-linter tests, full specification lint, and `git diff --check` passed.
- Desktop, tablet, and phone captures match the structural previews. No unresolved visual differences were identified.

Task 01 records the commands, intermediate failures, and verification scope.
No dependency, backend source, public guide, or saved preference changed. No commit or publication was performed.

## Risks

- Measuring constrained boxes can create a height feedback loop. Natural content totals avoid this dependency.
- Dynamic card metadata changes height. Tests must allow measurement settlement without fixed sleeps.
- Compact lanes still require both outer workflow scrolling and internal task scrolling.
- A CSS-only removal of `h-full` can break virtualization. The definite column area is required.

---
created: 2026-09-16
status: implemented
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
system_design:
  - ../../specs/tasks/system-design/subtask-reparenting-drag-drop.md
legacy_specs: []
---

# Implementation Plan: Fix Nest under candidates

## Overview

The sidebar can incorrectly disable `Nest under` for two independent reasons. Its context menu
prefers any present multi-workflow snapshot even when that snapshot is a partial placeholder, and
the shared eligibility helper applies the one-level Kanban rule to Office tasks despite the
backend's explicit Office depth exemption. The correction preserves Office identity through the
frontend projection, uses the rendered task group for menu and drag candidates, and makes their
shared pure filter match the backend depth contract.

No backend, API, persistence, localization, or layout change is required.

The smallest reproductions are focused frontend tests. An Office subject with a child and an
eligible Office root produces an empty candidate array, while a context menu rendered beside an
eligible task cannot find that task when the raw multi-workflow snapshot contains only the subject.
Before the correction, the focused command reports two failures and 28 passes:

```bash
(cd apps/web && pnpm test -- lib/sidebar/nest-candidates.test.ts components/task/task-switcher-context-menu.test.tsx)
```

The permanent regressions are `allows deep Office targets and excludes descendants` in
`lib/sidebar/nest-candidates.test.ts` and `uses rendered candidates when the workflow snapshot is
partial` in `components/task/task-switcher-nest-context-menu.test.tsx`.

## Scope

### In scope

- Preserve the existing backend Office discriminator through the sidebar task projection.
- Make menu and drag candidates share the rendered task group and eligibility helper.
- Match the backend's existing Office depth exemption and retain Kanban's one-level rule.
- Prove the correction with focused tests and an Office sidebar browser flow.

### Out of scope

- Backend validation, API, persistence, workspace materialization, and WebSocket contracts.
- Kanban card drag, sibling reorder, and the separate Office list parent picker.
- New controls, menu copy, layout, touch gestures, or navigation.

## Technical approach

1. Preserve the DTO's optional `is_from_office` value through `toKanbanTask`, the Kanban snapshot
   type, desktop and phone sidebar projections, and `TaskSwitcherItem` as `isFromOffice`. Preserve
   the cached value when a lightweight `task.updated` event omits the field and when a partial
   active-board task is reconciled with a full multi-workflow projection.
2. Extend `computeNestCandidates` with explicit descendant detection. Preserve the current Kanban
   root-only and childless-subject rules, while allowing arbitrary-depth Office candidates when
   either endpoint is Office.
3. Pass the flattened rendered `groupTasks` through the task-tree and row composition to
   `TaskNestContextMenuItems`. Remove its direct raw-snapshot selection. Keep drag target discovery
   on the same collection and helper.
4. Add focused projection, helper, menu, and drag tests. Add an Office E2E that reproduces the
   disabled submenu with a parent task and proves successful deeper nesting.

## ASCII UI preview

### UI-01: `Nest under` submenu with a visible eligible Office target

Current behavior established by the focused component reproduction:

```text
Nest under  >  +--------------------+
                 | No other tasks     |  disabled
                 +--------------------+
```

Required behavior:

```text
Nest under  >  +--------------------+
                 | Office target      |  selectable
                 +--------------------+
```

For a Kanban subject that already has a child, the disabled `No other tasks` state remains correct.
Desktop retains the anchored submenu shown above. On phone, the existing inset menu sheet renders
the same candidate row and retains its existing touch target, scroll, dismissal, and safe-area
behavior. Spacing is illustrative; candidate presence and eligibility are required by
AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1 through .5.

## Tests

- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1` and
  `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.2`:
  `task-switcher-nest-context-menu.test.tsx`, `uses rendered candidates when the workflow snapshot
  is partial`; `task-switcher-subtask-dnd.test.ts`, `matches menu candidates for Office and Kanban
  tasks`.
- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3`: `nest-candidates.test.ts`, `keeps the
  one-level Kanban limit`.
- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4`: `map-task.test.ts`, `maps is_from_office to
  isFromOffice`;
  `tasks-office-identity.test.ts`, `preserves Office identity when task.updated omits it`;
  `task-session-sidebar-aggregate.test.ts`, `preserves projected Office identity through a newer
  partial active task`;
  `task-session-sidebar-item.test.ts`, `preserves isFromOffice`;
  `session-task-switcher-sheet-item.test.ts`, `carries Office identity into the phone task drawer
  row`;
  `nest-candidates.test.ts`, `allows deep Office targets and excludes descendants`.
- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5`: the shared component tests render the
  existing menu and drag paths without separate desktop and phone candidate logic.

## E2E tests

- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1` through
  `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4`: add `office-sidebar-nest-task.spec.ts`, Chromium
  scenario `nests an Office parent under another Office task`, asserting the target row, persisted
  parent, and deeper rendered hierarchy.
- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3`: rerun `sidebar-nest-task.spec.ts` in
  Chromium as the shallow Kanban regression.
- AC `AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.5`: rerun
  `mobile-subtask-reparent-drag-drop.spec.ts` and add `mobile-office-sidebar-nest-task.spec.ts` in
  `mobile-chrome` to preserve the shared touch path and prove the Office candidate reaches the
  phone task sheet.

## Work orders

- [x] [Task 01: Correct Nest under candidates](task-01-correct-nest-under-candidates.md)

One sequential work order owns the projection, shared algorithm, both interaction paths, and browser
proof because each layer is required for one observable result. A wave does not authorize
delegation.

## Verification results

Implemented in one work order. The focused Vitest suite passes 115 tests across nine files;
typecheck, full frontend lint, and the i18n ratchet pass. Chromium passes both the existing Kanban
nesting flow and the new Office depth-two nesting and reload flow. The `mobile-chrome` project
passes both existing touch re-parenting cases and the new Office phone-sheet flow. Specification
validation and the scoped diff check also pass.

## Risks

- The backend remains the final validator for races and hierarchy data that is absent from the
  rendered group; rejected mutations must retain the existing rollback and error path.
- Reading a raw snapshot in either consumer would recreate the menu/drag divergence.
- Missing Office metadata must default to non-Office so the correction cannot relax Kanban rules.

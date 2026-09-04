---
created: 2026-09-04
status: draft
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-001
  - REQ-TASKS-PRIORITY-VISIBILITY-003
  - REQ-TASKS-PRIORITY-VISIBILITY-004
  - REQ-TASKS-PRIORITY-VISIBILITY-005
system_design:
  - ../../specs/tasks/system-design/task-priority-visibility.md
legacy_specs: []
---

# Implementation Plan: Task Priority in the Task Switcher

## Overview

Show task priority in the shared desktop and mobile task-switcher row, then add
the same single-task priority action to its responsive menu. The work first
makes the shared task projection and indicator complete. The menu can then use
that value for its current marker and prove live updates end to end.

## Scope

### In scope

- Carry task priority through the desktop sidebar and phone or tablet task-switcher projections.
- Show the existing non-medium priority indicator after the title in its inline badge area.
- Move shared priority display metadata out of the kanban-only component boundary.
- Add a flag-labelled Priority submenu to the single-task task-switcher menu.
- Reuse the existing field-scoped update request, failure toast and task event convergence.
- Prove desktop right-click and visible phone or tablet task-actions flows.

### Out of scope

- A new task field, endpoint, WebSocket action, database migration or priority vocabulary.
- Bulk priority changes, archived-task priority editing, sorting or grouping by priority.
- Changes to Office priority controls, the command palette or task-detail properties.
- A new mobile overlay; the existing responsive context menu remains the temporary choice surface.

## Technical approach

### Shared task-priority presentation

- Replace the kanban-named priority metadata boundary in
  `apps/web/lib/kanban/task-priority.ts` with a task-owned module under
  `apps/web/lib/tasks/`. Keep one token order, label-key map, value guard, icon
  map and color map for board cards and task-switcher rows.
- Move `KanbanCardPriorityIndicator` to a generic `TaskPriorityIndicator` under
  `apps/web/components/task/`. Keep the current contract: `critical`, `high`
  and `low` render distinct icons; `medium`, absent and invalid values render
  nothing. Update the kanban card to consume the generic component.
- Add `priority` to `TaskSwitcherItem`. Project it in
  `buildSidebarItem` and `toSheetItem`, then pass it through `TaskRow` to
  `TaskItem`. Render the indicator through the existing inline badge area after
  `TaskItemTitle`. Keep `TaskStateIcon` as the only fixed leading icon so every
  title starts at the same horizontal position.

### Task-switcher priority action

- Add a focused task priority context-menu submenu beside the existing task
  menu helpers. It uses `IconFlag`, the shared four-token order and localized
  labels, and an enabled `Current` marker that permits idempotent reselection.
- Render the submenu only in `SingleSelectionMenuItems` for live tasks. The
  existing bulk and archived menu branches remain unchanged.
- Call `useUpdateTaskPriority` with the clicked task ID. Do not write an
  optimistic row value. The existing `task.updated` and workflow-snapshot merge
  paths update both desktop and mobile projections; failure keeps the prior
  indicator and uses the existing localized toast.
- Reuse the current `ContextMenu` primitive. Fine-pointer desktop users use
  right-click or the row ellipsis. Phone and tablet users use the visible
  ellipsis inside `SessionTaskSwitcherSheet`; the current responsive menu
  styling supplies the inset bottom surface, internal overflow and touch-sized
  rows.

### Responsive contract

- Desktop outcome: priority is visible immediately after the title of each live
  row, and the right-click menu changes it without navigation. Rows without an
  indicator retain the same title alignment.
- Mobile entry point: the existing **Task actions** ellipsis in the task-switcher
  drawer or sheet. No required action depends on hover, right-click or long press.
- Nearest shipped exemplar: `SessionTaskSwitcherSheet` and its existing Edit and
  Move actions. Reuse its drawer or sheet hierarchy, fixed header, single
  `overflow-y-auto` task-list owner and responsive Radix menu treatment.
- Presentation choice: keep the short priority choice in the existing inset
  bottom menu on phones. A route or additional drawer would add unnecessary
  navigation for four options.
- Shared task data, priority validation and mutation logic remain common across
  viewports. Only the existing responsive menu presentation differs.
- Coarse-pointer action controls and menu rows stay at least 44 CSS pixels. The
  menu and nested priority choices must remain inside the viewport, clear the
  bottom safe area and create no document horizontal overflow.

## Tests

- `apps/web/components/task/task-session-sidebar-item.test.ts` and
  `apps/web/components/task/mobile/session-task-switcher-sheet-item.test.ts`
  cover `AC-TASKS-PRIORITY-VISIBILITY-005.1` and `.7` by retaining priority in
  both task-switcher projections.
- `apps/web/components/task/task-priority-indicator.test.tsx` and
  `apps/web/components/task/task-item.test.tsx` cover
  `AC-TASKS-PRIORITY-VISIBILITY-005.1` through `.3`, including medium and invalid
  fallback plus localized accessible names.
- `apps/web/components/task/task-switcher.test.tsx` covers
  `AC-TASKS-PRIORITY-VISIBILITY-005.4`, `.6` and `.8`: icon presence, option
  order, current and unknown state, idempotent selection, and archived or bulk
  exclusions.
- Existing `apps/web/hooks/use-update-task-priority.test.ts` continues to prove
  the field-only request and failure toast used by both board and task switcher.

## E2E tests

- `apps/web/e2e/tests/task/sidebar-layout.spec.ts` covers
  `AC-TASKS-PRIORITY-VISIBILITY-005.1`, `.4`, `.6` and `.7` through first render,
  desktop right-click, persistence, no navigation and live row convergence.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` covers
  `AC-TASKS-PRIORITY-VISIBILITY-005.1`, `.4`, `.5`, `.6` and `.7` through the
  visible Task actions trigger, nested priority selection, 44-pixel targets,
  viewport containment, safe scrolling and no document horizontal overflow in
  the `mobile-chrome` project.

## Work orders

- [ ] [Task 01: Project and render task-switcher priority](task-01-project-and-render-priority.md)
- [ ] [Task 02: Add responsive task-switcher priority actions](task-02-add-priority-actions.md)

## Verification results

Pending.

## Risks

- The desktop and mobile projection adapters are separate. Omitting either one
  would create viewport-specific priority loss.
- The existing menu is long and its nested submenu is portaled. Adding another
  submenu can expose phone-height overflow or horizontal containment regressions.
- Renaming the kanban-owned helper can leave a stale import in create, card or
  tests. An `rg` audit and focused typecheck must cover all consumers.
- Task rows already contain state, plugin, repository and change-request
  metadata. The priority indicator must remain in the inline badge cluster,
  preserve title alignment and not displace the visible mobile action trigger.

## Open questions

- None. Medium priority remains visually silent, matching the board contract.

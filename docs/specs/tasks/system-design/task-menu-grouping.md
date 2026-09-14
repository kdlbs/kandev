---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MENU-GROUPING-001
---

# Task menu grouping system design

## Purpose and boundaries

This change reorders existing task operations and centralizes divider placement within each menu composition.
The [task actions menu design](task-actions-menu.md) continues to own subject resolution, requests, confirmations, and navigation.
This design owns its new ordering. No API, storage, permission, or state subscription changes are necessary.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-TASKS-MENU-GROUPING-001.1 through .3 | Menu composition |
| AC-TASKS-MENU-GROUPING-001.4 through .6 | Conditional entries and plugins |
| AC-TASKS-MENU-GROUPING-001.7 and .8 | Responsive interaction |

## Menu composition

`buildKanbanCardMenuEntries` in `apps/web/components/kanban-card-menu-items.tsx`
already supplies both card renderers. It will assemble arrays for mark, edit,
relationships, move, primary plugins, and removal groups.
The composition filters empty arrays before it inserts separators.
Existing entry keys, submenu children, and callbacks remain stable.

`buildTaskActionsMenuEntries` in `apps/web/lib/kanban/task-actions-menu-entries.ts`
inherits the card order for normal rows. Its archived and unresolved branches
will use the same divider rule. An unresolved row has a plugin group, if visible,
then Archive and Delete together. An archived row has plugins, if visible, then Delete.

`SingleSelectionMenuItems` and `BulkSelectionMenuItems` in
`apps/web/components/task/task-switcher-context-menu.tsx` compose task-row actions.
Their group boundaries must use actual entry eligibility, including children that currently return null.
Counting JSX child elements cannot establish whether a group is empty.
Keep eligibility beside its existing action owner and expose visible entries or explicit visibility to the composition.
Use small focused helpers as necessary. Do not build a general menu registry.

Inspect separators owned by `task-switcher-action-items.tsx` and
`task-move-context-menu.tsx` before moving their items.
Remove or make those separators optional for grouped callers, while preserving other callers.
The parent composition owns every top-level divider.

## Conditional entries and plugins

The change preserves existing distinctions between cards, rows, bulk selection,
archived tasks, and unresolved board rows. In particular, it does not add card capabilities.
Subtask detachment moves into relationships without changing its eligibility.

`buildPrimaryPluginEntries` and `TaskPluginPrimaryMenuItems` retain registry order,
visibility evaluation, immutable task context, and current failure handling.
The primary group moves after movement and before removal.
Edit and Link plugin actions retain their current nesting and surface restrictions.
No label-based classification or plugin SDK schema change is necessary.

Existing location descriptions in `docs/plans/plugins/PLUGIN-API.md`, SDK comments,
host types, and `components/plugins/task-menu-actions.ts` need synchronization during implementation.
Only placement descriptions change. Existing plugin registration and callback types remain intact.

## Responsive interaction

The nearest shipped examples are the card dropdown and the task-row menu inside
`components/task/mobile/session-task-switcher-sheet.tsx`.
The shared Radix menu treatment in `app/globals.css` supplies the inset bottom sheet below 640px.
This short-lived action choice retains that shell and its internal scroll owner.
At wider widths, existing anchored menus and pointer behavior remain intact.

Both presentations share action eligibility and handlers.
Phone users open the visible ellipsis, tap a submenu, and select its action.
The existing sheet uses dynamic viewport limits and safe-area spacing.
The implementation must preserve that geometry when additional dividers increase menu height.
No new fixed footer, drawer stack, or separate mobile action list is necessary.

Task-row event propagation guards and touch-drag cancellation remain in place.
Escape, focus restoration, confirmation anchors, and close-before-dialog sequencing retain their existing owners.
Long-menu tests cover internal scrolling, submenu containment, touch targets, and last-row reachability.

## Implementation Plans

- [Task menu grouping](../../../plans/task-menu-grouping/plan.md)

---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-VIEW-REORDER-001
---

# Sidebar view editor reordering design

## Purpose and boundaries

Use the existing `@dnd-kit/core` and `@dnd-kit/sortable` packages for sidebar editor ordering.
This design changes presentation and input handling. Existing view drafts and personal settings retain ownership of data and writes.

This replaces the explicit row arrow layout in the
[sort-chain editor design](sidebar-running-first-activity-sort.md#editor).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-VIEW-REORDER-001` | List integration, Reorder controls, Responsive composition, State and recovery |

## Section inventory

`SidebarViewEditor` composes these sections:

| Section | Current behavior |
| --- | --- |
| View header | Rename, delete, save/discard; unchanged |
| Filters | Clause editing and removal; no reorder action |
| Sort | Ordered cards use a leading grip and More menu, with removal retained |
| Group by | One `GroupPicker` selector and indentation toggle; no reorder action |
| Task row | `SortableDetailRow` uses a grip and More menu beside the visibility switch |
| Automatic colors | Rule rows use a grip and More menu, with removal retained |

`ThreadSortSection` uses a single `TypedSortPicker`. Its direction arrow toggles ascending/descending and must remain.
Saved-view chips already support dragging. Navigation customization and task-list group headings have separate contracts.

## List integration

`SortChainEditor` already holds `IdentifiedSortRule` entries with editor-local stable IDs.
Use these IDs for `SortableContext` and `useSortable`, keeping the existing input acknowledgement logic.
Never use field names, colors, or current indexes as drag identity. Several color rules can coexist and change color during editing.

Add a guarded ID-to-ID reorder helper beside `moveIdentifiedSortRule` in `sort-chain-editor-model.ts`.
Resolve both IDs against the current entries and use `arrayMove` for arbitrary insertion.
The current helper swaps positions and is suitable only for adjacent menu moves.
Return the original entries for missing or equal IDs. Call `commitEntries` only when order changes.
Keep `withRules` conversion and its Custom restriction.

Wrap the sort cards with `DndContext`, `SortableContext`, and `verticalListSortingStrategy`.
`SortChainRuleCard` attaches the sortable node ref, transform, transition, and active drag styling to its card.
Only the grip receives activator listeners and `setActivatorNodeRef`.
Retain a position label in the phone header and include position in accessible names on every viewport.
The desktop grip replaces the old ordinal-only slot.

Reuse `AutomaticColorRuleList`, `AutomaticColorRuleCard`, and `useAutomaticColorSettingsController.reorderRules` for color rules.
That controller already uses `arrayMove` and rejects missing or equal IDs.
`TaskRowDetailsSection` retains `reorderSidebarTaskRowDetails` and routes menu moves through the same update/announcement path as dragging.

## Reorder controls

Add a small shared `SidebarReorderMenu` in `components/task/sidebar-filter/`.
It wraps existing `@kandev/ui/dropdown-menu` primitives and accepts current position, list size, and an adjacent-move callback.
Keep mutation and list identity with callers. The component owns no persistence or drag context.
Use a visible ellipsis trigger and localized accessible item names.
Reuse existing domain Move up/down labels. New labels and announcements enter all shipped task locale catalogs and generated pseudo copy.

Keep Remove visible for sort and color rules. Keep the existing final-sort-rule removal guard.
Task-row details retain their visibility switches and have no removal action.
Menus expose disabled boundary actions and return focus to the same stable item after a move.

Use pointer/touch activation thresholds and `KeyboardSensor` with `sortableKeyboardCoordinates`, matching the existing detail/color lists.
Bind touch suppression only to grips. Dragging must not trigger drawer dismissal, selectors, or row switches.
Other card regions remain normal scroll and input surfaces.
Use localized dnd accessibility instructions and pickup/move/drop/cancel announcements.
Keyboard pickup/drop uses Space or Enter, movement uses arrows, and Escape cancels.

## Responsive composition

Desktop keeps the existing anchored popover and compact horizontal sort rows.
Grips and icon actions use 28px fine-pointer dimensions. Remove the two dedicated reorder button slots.
Selectors use the recovered width, including color rows with three selectors.

Phone entry remains Tasks picker, Sidebar filters, then the expanded section.
The nearest shipped exemplar is `MobileSidebarFilterSurface` in `sidebar-filter-popover.tsx`.
It provides the inset drawer, fixed header, dynamic viewport bounds, safe-area padding, and one scroll body.
This temporary settings flow retains that surface because users edit a short list and return to task navigation.

Phone sort cards show a header with grip, position, More, and Remove, then stacked field controls.
Color headings retain grip, rule label, More, and Remove. Detail rows retain grip, label, More, and visibility switch.
Use phone/coarse-pointer targets of at least 44px, including narrow fine-pointer phones and wider coarse-pointer tablets.
Use the shared responsive Radix menu treatment for the short Move up/down menu.
The menu is a temporary action surface, while the editor remains the sole content scroller.

## State and recovery

Sort and detail changes continue through `SidebarFilterPopover`'s guarded `updateSidebarDraft` path.
Automatic colors continue through existing immediate updates, revision checks, and rollback.
No persisted drag IDs, new API, database migration, dependency, or runtime flag is needed.

Guard drops against current IDs and list generation. Discard active gestures when an external replacement rebuilds sort entries.
View changes must invalidate an active sort/detail gesture even when both views have identical field values.
Use the resolved workspace and active-view identity at the editor boundary for that cancellation, retaining workspace guards.
Do not remount the complete editor on every draft change or erase disclosure state.
Dismissal or workspace changes abandon active gestures. A drop with no valid destination produces no mutation.

## Related contracts

- [Sort priority and persistence](sidebar-running-first-activity-sort.md)
- [Personal automatic colors](sidebar-automatic-task-colors.md)
- [Workspace view isolation](workspace-sidebar-task-views.md)
- [Control sizing](control-sizing.md)
- [Composable sort decision](../../../decisions/2026-10-06-composable-sidebar-sort-rules.md)

## Delivery

- [Plan and work order](../../../plans/sidebar-view-editor-reordering/plan.md)

---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-002
---

# Sidebar Task Tree Activity Sort System Design

## Purpose and boundaries

The task system publishes each task's `last_activity_at`. The UI owns the saved-view ordering of a parent and its visible descendants as one sidebar tree. This design changes only the derived order used when the view's sort key is `lastActivityAt`. It does not change task activity publication, the saved-view wire shape, row timestamps, or other sort modes.

## Requirement mapping

| Requirement                             | Design sections                                                                                                                                            |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-002` | [Tree activity derivation](#tree-activity-derivation), [Shared sort path](#shared-sort-path), [Responsive behavior](#responsive-behavior), [Tests](#tests) |

## Components and responsibilities

- `TaskSwitcherItem` retains each task's own `lastActivityAt`, `updatedAt`, and `createdAt` fields.
- The phone task-switcher projection derives `lastActivityAt` from the task's status summary, falling back to that task's `updatedAt` and `createdAt`. It must not use summary freshness as activity.
- `applyView` in `apps/web/lib/sidebar/apply-view.ts` filters tasks, builds the included parent-child map with `separateSubtasks`, and passes that map to `applySort`.
- A pure tree-activity resolver under `apps/web/lib/sidebar/` derives one maximum activity timestamp per included task subtree. Keep it separate from `apply-view.ts`, which is already close to the frontend file-size limit.
- `applySort` uses the derived timestamp for `lastActivityAt` comparisons when the included map is available. The existing per-task fallback remains available for direct callers without a map.
- Desktop `useGroupedSidebarView` and phone `MobileTaskList` continue to call the same `applyView` path.

## Tree activity derivation

The resolver starts with the current per-task sort value: `lastActivityAt`, falling back to `updatedAt` and then `createdAt`. For each included task, it takes the maximum of its own value and the resolved values of all included children. This makes a root's key the newest activity anywhere in its visible tree and gives a child with descendants the same subtree semantics among its siblings.

Resolve the map in one bounded traversal with memoized subtree values and an iterative walk. A visited or active-path guard prevents malformed parent cycles from hanging the sidebar. Do not mutate task records or replace the row's own timestamp with the aggregate.

`applyView` calls `applyFilters` before `separateSubtasks`; therefore filtered-out descendants cannot contribute. A child whose parent is filtered out becomes a root through the existing `separateSubtasks` behavior. Collapse preferences only affect rendering and do not remove members from the derived map.

## Shared sort path

Compare activity values by chronological instant, including timezone offsets and all fractional-second digits. RFC3339Nano values can have different fractional precision, so string order does not always match time order. Semantically equal instants retain stable input order, and missing values keep their existing fallback behavior.

The existing stable `applySort` index tiebreak handles equal aggregate values. Its direction sign gives descending and ascending order from the same maximum value. `applyGroup` then keeps roots and children attached and uses the sorted input; group headings keep their existing ordering rules. `applySubtaskOrder` and pinned-task floating still run afterward with their current precedence. Only the `lastActivityAt` comparator branch consumes the tree activity map; `updatedAt`, state, title, created, and custom order keep their existing semantics.

The parent row still receives its original `TaskSwitcherItem`. Its relative time describes the parent's own activity, while tree placement reflects the newest included member. No new label, icon, setting, or persisted aggregate is introduced.

## Responsive behavior

The nearest phone exemplar is `SessionTaskSwitcherSheet`: its drawer contains `MobileTaskList` and the shared view editor. Desktop and phone share the derived ordering and view state. The change does not alter the phone drawer, task-row layout, navigation, touch targets, safe-area handling, or the existing single task-list scroll body.

## Tests

- A focused unit test for the resolver and `applyView` covers descending and ascending root order, deep descendants, siblings, equal timestamps, missing activity fallbacks, filtered-out children, orphan promotion, collapsed trees, and unchanged pin/manual/other-sort behavior.
- A desktop Playwright scenario selects Last activity descending and shows a parent tree moving ahead of a newer standalone root after child activity, while the parent's displayed time remains its own.
- A `mobile-chrome` Playwright scenario opens the existing task-switcher drawer, selects the same sort, and verifies the parent tree's order and child reachability by touch. No new mobile control or geometry is introduced.

## Failure and recovery

Missing task activity uses the existing update and creation fallbacks. A missing descendant projection cannot influence the current visible tree; the next task snapshot or event recomputes the shared `applyView` result. No separate cache or retry path is needed.

## Persistence and security

The aggregate exists only during sidebar view derivation. It adds no API field, database value, permission, or cross-workspace data access. It uses only tasks already included in the current sidebar view.

## Related design

- [Sidebar effective task tree state](sidebar-effective-task-tree-state.md) defines the corresponding state grouping and sorting pattern.
- [Activity timestamp decision](../../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md) defines the source activity value.

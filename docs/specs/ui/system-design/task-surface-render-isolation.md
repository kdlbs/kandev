---
status: draft
system: ui
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
---

# Task Surface Render Isolation System Design

## Purpose and boundaries

The UI system owns render isolation for task surfaces with many repeated rows. This design covers
the file tree, sidebar task rows, pull-request indicators, and task-top-bar plugin contributions.

The task, integration, and plugin systems still own their data and actions. This design does not
change their contracts, persistence, or user-visible behavior.

The browser trace from 2026-08-31 showed three full render waves in 5.74 seconds. Each wave rendered
approximately 602 file rows and 48 sidebar rows. Layout, paint, and garbage collection were minor
costs, so this design targets React work and mounted component count.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-FILE-TREE-CHAT-CONTEXT-001` | [File-tree rows](#file-tree-rows), [File-tree virtualization](#file-tree-virtualization), [Responsive behavior](#responsive-behavior) |
| `REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001` | [Sidebar rows](#sidebar-rows), [Derived contribution props](#derived-contribution-props), [Responsive behavior](#responsive-behavior) |
| `REQ-UI-MOBILE-TASK-NAVIGATION-001` | [Responsive behavior](#responsive-behavior), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `useFileBrowserTree` owns stable file-tree state and action identities.
- `FileTreeView` owns row-window calculation and the single file-tree scroll owner.
- `TreeNodeItem` owns one file or directory row and skips semantically unchanged renders.
- `FileContextMenu` preserves the established pointer, keyboard, and touch actions for mounted rows.
- `useSidebarSelection` owns stable selection actions for the task switcher.
- `TaskSwitcher` and `TaskRow` skip work when their rendered inputs do not change.
- `TaskTopBarPluginActions` derives stable session identifiers for plugin slot props.
- `useTaskPRTooltipHydration` returns a stable value while its status and actions do not change.
- `PRTaskIcon` reuses stable empty and unchanged pull-request inputs.

## Render identity contract

A parent render must not change a row callback when the callback behavior is unchanged. A hook must
not return a new aggregate object when all fields retain the same identity and value.

List rows use memoized component boundaries. Each row receives the smallest required state. A row
rerenders when its node, selection, expansion, active-file, drag, or action state changes.

Render-count tests prove identity behavior. The tests use controlled rerenders and do not use
elapsed-time thresholds.

## File-tree rows

File-tree actions read changing tree data from stable refs or functional state transitions. The
callbacks do not depend on the complete `treeState` aggregate object.

`TreeNodeItem` uses a memoized boundary with explicit, stable inputs. Selection or expansion of one
node does not rerender unrelated visible rows. File opening, drag and drop, rename, download,
deletion, multi-selection, and chat-context actions keep their current behavior.

Each mounted row keeps its own context-menu trigger. A shared menu is not required for this change.
Virtualization limits the number of mounted menu providers.

## File-tree virtualization

`FileTreeView` uses the existing `@tanstack/react-virtual` dependency. The flattened `visibleRows`
array remains the source for order, nesting depth, expansion, and filtering.

The existing file-tree viewport remains the only vertical scroll owner. The virtualizer positions
only the current row window and a small overscan window inside that viewport.

Rows use measured sizes because editing controls and touch layouts can change row height. The
virtualizer uses an estimated size before measurement. It updates positions after a measured size
changes.

Active-file reveal and restored scroll operations resolve a row index from `visibleRows`. They use
the virtualizer scroll API before they query a row element. This sequence makes an offscreen row
available before focus or selection work uses its element.

## Sidebar rows

`useSidebarSelection` derives `onBulkMove` from stable action functions and current selection data.
It does not depend on a newly allocated selection aggregate.

`TaskSwitcher` receives stable callbacks during unrelated parent renders. A memoized task row can
then isolate row work when task data and row state do not change.

The task row remains the primary mobile tap target. The visible mobile action and fine-pointer
hover behavior remain unchanged.

## Derived contribution props

Plugin slot props use a stable session-identifier array while the ordered identifiers do not
change. Session status updates cannot rerender the plugin contribution when its slot input stays
equal.

Pull-request tooltip hydration returns one stable aggregate while its status, hydration action,
and automation options stay equal. Pull-request indicators also reuse one empty array constant.

These changes do not alter plugin host APIs or integration data. They only stabilize values at UI
component boundaries.

## Control flow

1. A store or local state update renders a task-surface owner.
2. Stable selectors and callbacks preserve identities for unchanged inputs.
3. Memoized boundaries skip unchanged rows and contributions.
4. The file-tree virtualizer calculates the required row window from the scroll position.
5. React renders only changed components inside the active row window.

## Responsive behavior

Desktop and mobile use the same flattened file-tree model and virtual row window. The layout does
not create a second vertical or horizontal scroll owner.

Each mounted mobile file row keeps its visible action trigger and 44 CSS pixel touch target. The
responsive menu stays inside the viewport and keeps its safe-area behavior.

Each mobile sidebar row remains the primary navigation action. Its explicit task menu stays visible
and touch-reachable. Virtualization does not change the sidebar composition.

## Failure and recovery

If row measurement is unavailable, the virtualizer uses the estimated row size. If a target row is
offscreen, reveal logic scrolls to its index before it requests the element.

An empty tree renders the current empty state. A changed filter or expansion state recalculates the
flat row list and virtual range from the current scroll owner.

Identity optimizations must fail toward an extra render, not stale UI. Equality logic must include
every input that changes visible output or interaction behavior.

## Persistence

This design adds no persisted state. Existing expansion, selection, file, sidebar, plugin, and
pull-request state keeps its current owner and lifetime.

## Security

This design adds no trust boundary or data access. Virtual rows use the same authorized file and
task data as the current complete list.

## Observability

Unit tests record render counts for controlled updates. Browser E2E tests record the number of
mounted rows for a large generated file tree.

Final performance evaluation uses a warmed production build without React DevTools or CPU profiler
startup in the measured window. The report includes main-thread work, long tasks, and mounted-row
counts. Development traces remain diagnostic evidence only.

## Related decisions

No ADR is required. This design applies the repository's existing virtual-list dependency and UI
ownership model.

## Related designs

- [Sidebar Task Row Presentation](sidebar-task-row-presentation.md)
- [PR Task Status Summary](pr-task-status-summary.md)
- [Mobile Task Chrome](mobile-task-chrome.md)

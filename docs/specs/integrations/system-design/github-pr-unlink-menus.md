---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
created: 2026-09-26
owners:
  - Kandev
---

# GitHub pull request unlink menus System Design

## Purpose and boundaries

This design adds two entry points to the existing GitHub task-PR unlink
operation. The integration system owns association identity, mutation, and
refresh. The UI system supplies existing context-menu and dropdown primitives.
No backend endpoint, database migration, or provider mutation is needed. The
status-summary projector consumes the existing detach event to refresh the
compact task-row projection.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001` | [Association data and mutation](#association-data-and-mutation), [Top bar](#top-bar), [Task cards](#task-cards), [Sidebar task rows](#sidebar-task-rows), [Responsive behavior](#responsive-behavior), [Failure and recovery](#failure-and-recovery) |

## Association data and mutation

`apps/web/hooks/domains/github/use-task-pr.ts` already exposes `unlink(id)`.
It calls `deleteTaskPR`, removes only the matching task association after HTTP
success, and invalidates the scoped synchronization resource so an older sync
response cannot restore the link. The existing `github.task_pr.deleted` event
updates other connected clients. The task status-summary projector also handles
that event by reloading the task's active PR associations and persisting the
complete summary. This clears the compact PR indicator after the final unlink,
including in task-list responses and boot state after reload. The backend API
and event payload stay unchanged. Both new menus use this path, including its
workspace and workspace-context guards. Do not construct a PR URL or identify
an association by PR number alone.

Extract the mutation into a focused reusable hook or function only if card
menus need it without subscribing every mounted card to `useTaskPR`'s sync
lifecycle. Keep the API, store, event, and toast behavior consistent with the
existing multi-PR unlink control. A per-association pending guard prevents
duplicate requests without disabling unrelated PR choices.

## Top bar

`apps/web/components/task/task-top-bar.tsx` mounts `PRTopbarButton` for GitHub.
In `apps/web/components/github/pr-topbar-button.tsx`, add a desktop
`ContextMenu` to the existing PR trigger. Its root entry is `Edit`, with one
localized `Unlink pull request #N` child for one PR or one child per PR for a
multi-PR task. Include repository identity in the multi-PR label; the selected
child carries the persisted `TaskPR.id` to `unlink`.

Keep the current single-PR click destination, multi-PR click selector, and
hover CI popover. Opening the context menu closes or suppresses the hover
popover so two overlays do not compete. The menu closes when its action is
selected; it must tolerate the trigger unmounting after the final unlink.
Keyboard context-menu activation follows the existing Radix primitive.

## Task cards

`apps/web/components/kanban-card-menu.tsx` builds both the desktop context
menu and the visible dots dropdown from one task and workspace. The board
already calls `useWorkspacePRs`, and card PR indicators can hydrate a task's
exact PR records on demand. Read association rows only from the current
workspace context using `getTaskPRsForCurrentWorkspace`; do not render a
destructive choice from `statusSummary.pull_request.number`.

When a menu opens before its task's records are present, reuse the task-scoped
PR hydration path to resolve exact rows. While it loads, show a disabled
localized loading entry if the card's summary indicates a PR. Once rows
arrive, `buildEditMenuEntry` in
`apps/web/components/kanban-card-edit-submenu.tsx` includes the native
unlink children beside `Edit task` and any registered plugin `edit` actions.
The same built entries feed `KanbanCardContextMenuItems` and
`KanbanCardDropdownMenuItems`. Preserve the flat `Edit` item on tasks with no
PR and no plugin edit contributions. If several cards are selected, unlink
still targets only the card whose menu opened; it is not a bulk action.

## Sidebar task rows

The pictured sidebar menu is separate from the Kanban card menu.
`TaskItemWithContextMenu` in
`apps/web/components/task/task-switcher-context-menu.tsx` owns its open state,
and `SingleEditGroup` in `task-switcher-context-menu-items.tsx` currently renders
a flat `Edit` item. On an unarchived row with linked GitHub PRs, make `Edit` a
submenu containing `Edit task` and the same association-specific unlink
choices. Keep `Rename` and `Duplicate` at their current root level, and keep
the flat `Edit` item when no PR is linked. The menu uses the row's task ID,
never the active task's ID. Existing multi-selection reduced menus must not
offer a single-task unlink action.

The sidebar can begin with only `TaskSwitcherItem.prInfo`. Reuse the scoped
exact-record hydration path on menu open and render a disabled loading row
while it resolves. The row's visible dots trigger opens the same menu on a
phone. Use the shared unlink action and pending guard, and preserve the row's
selection, drag, and navigation event boundaries.

## Responsive behavior

Desktop uses a small context menu anchored to the top-bar PR trigger and the
existing right-click card and sidebar menus. The card and sidebar visible dots
buttons expose their `Edit` submenus on a phone. The phone does not mount the
desktop top bar; its existing PR status chip and drawer remain the task-view
path to unlink.

The closest phone exemplars are `KanbanCardActions` and the sidebar
`TaskMenuButton`: their visible dots controls already open secondary actions.
Keep card and row bodies as primary tap destinations and make unlink a labeled
secondary action. Phone controls and rows need 44-pixel hit targets. Submenus
must fit the viewport, scroll within the menu when necessary, retain safe-area
clearance, and return focus to their trigger on dismissal. Use localized labels
and semantic menu items.

## Failure and recovery

Pending state is scoped to the selected association. A failed HTTP unlink
leaves the row in the store and shows a localized error toast; closing and
reopening either menu permits retry. An association removed by another client
disappears on the existing WebSocket event. If it disappears while a menu is
open, do not submit a stale selection or remove another row. A missing active
workspace disables mutation rather than guessing a scope.

## Persistence and security

The existing backend persists detachment and authorizes the association's
workspace. This change adds no storage. The browser supplies the exact
association ID from the scoped task data, and the backend retains authority
over the mutation. Unlink does not call GitHub to alter the remote PR.

## Tests

- Component tests for the top-bar context menu cover one PR, duplicate
  numbers across repositories, normal click/hover behavior, pending state,
  failure retention, and final-control unmount.
- Card menu tests cover exact scoped rows, delayed hydration, flat `Edit` on
  unlinked tasks, coexistence with plugin `edit` actions, and identical
  context/dots entries. Sidebar menu tests cover its separate Edit group,
  selection boundary, and row-scoped target.
- Desktop Playwright covers unlink from top bar, sidebar row, and card context
  menus, including persistence after reload and sibling preservation.
- Mobile Playwright covers sidebar and card dots paths, 44-pixel action
  targets, and no horizontal overflow.

## Related designs

- [Task PR synchronization](github-task-pr-sync-coordination.md)
- [PR task status summary](../../ui/system-design/pr-task-status-summary.md)

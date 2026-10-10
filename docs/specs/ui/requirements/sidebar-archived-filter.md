---
status: active
system: ui
created: 2026-08-04
owners:
  - Kandev
---
# Sidebar Archived Task Views Requirements

## Overview

Users who organize the task sidebar with saved views cannot browse archived tasks there: the **Archived** filter is offered, but the sidebar only receives active workflow snapshots. Archived views must show the tasks they describe without putting archived cards back on active Kanban boards.

## Requirements

### REQ-UI-SIDEBAR-ARCHIVED-FILTER-001: Sidebar Archived Task Views

**Intent:** Users who organize the task sidebar with saved views cannot browse archived tasks there: the **Archived** filter is offered, but the sidebar only receives active workflow snapshots. Archived views must show the tasks they describe without putting archived cards back on active Kanban boards.

#### Acceptance criteria

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.1:** Desktop and mobile sidebar view editors offer **Archived** as a boolean filter dimension.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.2:** **Archived: Show** displays only archived, non-ephemeral tasks from the current workspace. Other clauses, sorting, grouping, and saved-view behavior apply to those tasks normally.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.3:** **Archived: Hide**, and views with no Archived clause, continue to display only active tasks.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.4:** Persisted clauses equivalent to **Archived: Show** (for example, `archived is_not false`) also load archived candidates.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.5:** Archived tasks never enter active workflow snapshots or Kanban columns.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.6:** Archived rows use the existing archived badge and open the archived task detail when selected. The detail's existing **Unarchive** action remains the recovery path.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.7:** Archive, unarchive, and delete events update an already-loaded archived view without a page reload. Active and archived caches never contain the same task after the event is applied.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.8:** Switching workspaces never displays archived tasks cached for another workspace.

- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.9:** Initial loading and failed reads shall remain distinct from a successfully empty archive. Failed reads shall offer Retry.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.10:** Selecting an archived task shall show its existing conversation without a browser reload, on desktop and phone.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.11:** Opening an archived task shall not create, prepare, resume, or launch an agent session. A task without sessions shall show an archived empty state.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.12:** A delayed selection or read shall not replace a later task selection. Content from another task shall never appear as the selected conversation.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.13:** A directly opened task shall remain accessible outside the current archive page. A matching page row shall not duplicate its current-task entry.
- **AC-UI-SIDEBAR-ARCHIVED-FILTER-001.14:** Saved views and drafts shall retain valid archive clauses through boot, hydration, and live settings updates.

## Related contracts

[Pagination for every sidebar view](sidebar-task-pagination.md) owns size-based paging.

[Last activity](sidebar-last-activity-sort.md) owns timestamp meaning and tree order.
[Effective tree state](sidebar-effective-task-tree-state.md) owns state placement.
[Workspace views](workspace-sidebar-task-views.md) owns saved preferences.

The existing workspace task-list API retains its archive modes, totals, authorization,
search, and pagination behavior. Archived browsing does not change active board contents.
The archive listing remains a runtime cache, separate from persisted saved views.

## Out of scope

Task retention, archive cascades, session finalization, active-board pagination,
new archive row actions, and changes to the Tasks page or command panel.

## Implementation plans

- [Original archive views](../../../plans/sidebar-archived-filter/plan.md)
- [Bounded archive browsing and navigation repair](../../../plans/archived-sidebar-loading/plan.md)

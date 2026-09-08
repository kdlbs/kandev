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

## Migrated source detail

## Why

Users who organize the task sidebar with saved views cannot browse archived
tasks there: the **Archived** filter is offered, but the sidebar only receives
active workflow snapshots. Archived views must show the tasks they describe
without putting archived cards back on active Kanban boards.

## What

- Desktop and mobile sidebar view editors offer **Archived** as a boolean
  filter dimension.
- **Archived: Show** displays only archived, non-ephemeral tasks from the
  current workspace. Other clauses, sorting, grouping, and saved-view behavior
  apply to those tasks normally.
- **Archived: Hide**, and views with no Archived clause, continue to display
  only active tasks.
- Persisted clauses equivalent to **Archived: Show** (for example,
  `archived is_not false`) also load archived candidates.
- Archived tasks never enter active workflow snapshots or Kanban columns.
- Archived rows use the existing archived badge and open the archived task
  detail when selected. The detail's existing **Unarchive** action remains the
  recovery path.
- Archive, unarchive, and delete events update an already-loaded archived view
  without a page reload. Active and archived caches never contain the same task
  after the event is applied.
- Switching workspaces never displays archived tasks cached for another
  workspace.
- A directly opened archived task may still render the existing synthetic
  current-task row when the archived listing is not loaded; a fetched row with
  the same task ID replaces, rather than duplicates, that placeholder.
- Persisted saved views and in-flight drafts retain valid `archived` clauses
  through boot, hydration, and live user-settings updates.

## API surface

The existing workspace task-list route gains one additive query parameter:

- `GET /api/v1/workspaces/:id/tasks?only_archived=true` returns only archived,
  non-ephemeral tasks that satisfy the existing workspace, workflow,
  repository, query, sort, and pagination parameters.
- The response remains `ListTasksResponse { tasks: Task[], total: number }`,
  where `total` counts the archived-only result set before pagination.
- With neither archive parameter, the route continues to return active tasks.
- Existing `include_archived=true` continues to return active and archived
  tasks. If both flags are supplied, `only_archived=true` takes precedence.
- The route retains the existing workspace authorization and not-found
  behavior.

## Failure modes

- While the first archived-only request is pending, the sidebar shows its
  loading state instead of a successful-empty state.
- If an archived-only request fails, a previously successful archived cache is
  retained. With no cache, the view exits loading and surfaces the existing
  task-load error treatment; reopening the view or a foreground refresh
  retries the request.
- A stale request from a previous workspace or request generation cannot write
  into the current workspace's sidebar state.

## Persistence guarantees

Saved views and drafts keep using the existing user-settings persistence. The
archived-task listing is a runtime cache only: it is reloaded after a client
restart and does not change task retention or archive persistence.

## Scenarios

- **GIVEN** a workspace with active and archived tasks, **WHEN** the user opens
  the default sidebar view, **THEN** only active tasks are listed.
- **GIVEN** a workspace with active and archived tasks, **WHEN** the user sets
  **Archived: Show**, **THEN** only archived tasks from that workspace are
  listed after the loading state completes.
- **GIVEN** an archived view with an additional title, workflow, repository, or
  state clause, **WHEN** the view is applied, **THEN** the archived result set
  is narrowed by every additional clause and uses the selected sort/group.
- **GIVEN** an archived view is loaded, **WHEN** an active task is archived,
  **THEN** that task leaves active caches and appears once in the archived view
  without a reload.
- **GIVEN** an archived view is loaded, **WHEN** an archived task is unarchived
  or deleted, **THEN** it disappears from the archived view without a reload;
  an unarchived task returns to the appropriate active workflow snapshot.
- **GIVEN** an archived row, **WHEN** the user selects it, **THEN** Kandev opens
  the archived task detail and exposes the existing Unarchive action.
- **GIVEN** a saved view or draft containing an `archived` clause, **WHEN**
  Kandev boots, hydrates, or receives updated settings, **THEN** the clause is
  preserved and remains editable.
- **GIVEN** an archived request for workspace A is in flight, **WHEN** the user
  switches to workspace B, **THEN** no task from workspace A appears in B.
- **GIVEN** a phone viewport, **WHEN** the user opens the task-switcher drawer
  and applies **Archived: Show**, **THEN** the same archived tasks are reachable
  and the drawer/filter surfaces remain viewport-contained and internally
  scrollable.
- **GIVEN** the archived-only request fails, **WHEN** the failure is reported,
  **THEN** the sidebar does not present the failed request as a successful empty
  archive and the user can trigger a retry.

## Out of scope

- Showing archived tasks on Kanban boards or changing workflow snapshot
  semantics.
- Changing archive, unarchive, cascade, retention, or session-finalization
  behavior.
- Adding an Unarchive context-menu action directly to sidebar rows.
- Changing the full Tasks page or command-panel meaning of
  `include_archived=true`.
- Persisting the archived-task client cache.

## Implementation plan

- [Sidebar Archived Task Views](../../../plans/sidebar-archived-filter/plan.md)

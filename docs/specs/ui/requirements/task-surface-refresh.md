---
status: active
system: ui
created: 2026-07-27
owners:
  - Kandev
---
# Task Surface Foreground Refresh and Mobile Create Action Requirements

## Overview

People returning to Kandev after the browser, window, or device has been idle can see a task List, Kanban board, or task-detail chat that no longer matches backend state. This affects desktop window switching as well as suspended mobile browsers. Phone users also need a familiar recovery gesture and the same thumb-reachable create action in List that Kanban already provides.

## Requirements

### REQ-UI-TASK-SURFACE-REFRESH-001: Task Surface Foreground Refresh and Mobile Create Action

**Intent:** People returning to Kandev after the browser, window, or device has been idle can see a task List, Kanban board, or task-detail chat that no longer matches backend state. This affects desktop window switching as well as suspended mobile browsers. Phone users also need a familiar recovery gesture and the same thumb-reachable create action in List that Kanban already provides.

#### Acceptance criteria

- **AC-UI-TASK-SURFACE-REFRESH-001.1:** Mobile List shows the same safe-area-aware floating **Add task** button as mobile Kanban.
- **AC-UI-TASK-SURFACE-REFRESH-001.2:** The List floating action opens the existing task-create flow in the active workspace and workflow; a successful creation becomes visible under the current List filters and pagination rules.
- **AC-UI-TASK-SURFACE-REFRESH-001.3:** Mobile List and mobile Kanban support pull-to-refresh from the top of their current vertical scroll surface.
- **AC-UI-TASK-SURFACE-REFRESH-001.4:** The pull gesture activates only for a primarily downward single-touch drag that starts while the active vertical scroll owner is at its top. Horizontal Kanban navigation, task drag-and-drop, and ordinary scrolling do not trigger refresh.
- **AC-UI-TASK-SURFACE-REFRESH-001.5:** The UI shows pull progress, a release threshold, and an in-progress state without replacing already-rendered tasks with an empty or loading screen.
- **AC-UI-TASK-SURFACE-REFRESH-001.6:** Releasing beyond the threshold performs one refresh at a time. Releasing before the threshold restores the surface without fetching.
- **AC-UI-TASK-SURFACE-REFRESH-001.7:** A List refresh reloads the current page using the current workspace, workflow, repository, query, archive, sort, and pagination settings.
- **AC-UI-TASK-SURFACE-REFRESH-001.8:** A Kanban refresh reloads the current workspace's visible workflow snapshots, including the active workflow state used by task preview and creation.

## Migrated source detail

## Why

People returning to Kandev after the browser, window, or device has been idle
can see a task List, Kanban board, or task-detail chat that no longer matches
backend state. This affects desktop window switching as well as suspended
mobile browsers. Phone users also need a familiar recovery gesture and the same
thumb-reachable create action in List that Kanban already provides.

## What

- Mobile List shows the same safe-area-aware floating **Add task** button as
  mobile Kanban.
- The List floating action opens the existing task-create flow in the active
  workspace and workflow; a successful creation becomes visible under the
  current List filters and pagination rules.
- Mobile List and mobile Kanban support pull-to-refresh from the top of their
  current vertical scroll surface.
- The pull gesture activates only for a primarily downward single-touch drag
  that starts while the active vertical scroll owner is at its top. Horizontal
  Kanban navigation, task drag-and-drop, and ordinary scrolling do not trigger
  refresh.
- The UI shows pull progress, a release threshold, and an in-progress state
  without replacing already-rendered tasks with an empty or loading screen.
- Releasing beyond the threshold performs one refresh at a time. Releasing
  before the threshold restores the surface without fetching.
- A List refresh reloads the current page using the current workspace,
  workflow, repository, query, archive, sort, and pagination settings.
- A Kanban refresh reloads the current workspace's visible workflow snapshots,
  including the active workflow state used by task preview and creation.
- When Kandev becomes the focused window, becomes visible after being hidden,
  resumes from the browser page cache, or returns online, the active task
  surface performs an authoritative foreground refresh even when the WebSocket
  connection still reports `connected`.
- Foreground events emitted together for one return to Kandev are coalesced so
  each active data source refreshes once rather than once per browser event.
- Kanban/Home foreground refresh reloads the current workspace's workflow
  snapshots.
- List foreground refresh reloads the current filtered and paginated List
  request.
- Task-detail foreground refresh reloads the open task, its session list, the
  active session's chat messages, and its queued-message state. Existing
  subscription and focus state remains attached to the same task and session.
- Automatic foreground refresh applies on desktop and mobile. Pull-to-refresh
  and the List floating create action are phone-only.
- Refreshed responses must not overwrite a newer workspace selection or a task
  mutation that lands while a refresh is in flight.

## Failure modes

- A failed manual refresh keeps the last rendered tasks and presents the
  existing user-facing error feedback pattern; the pull indicator settles back
  to idle so the user can retry.
- A failed automatic foreground refresh keeps the last rendered tasks and does
  not interrupt the user with a modal.
- A refresh requested while another refresh for the same data source is in
  flight is coalesced rather than issuing overlapping requests.
- If the active workspace or workflow changes during a refresh, the stale
  response is discarded.

## Scenarios

- **GIVEN** List is open on a phone, **WHEN** the user taps the floating **Add
  task** button and completes task creation, **THEN** the existing create flow
  uses the active workspace and workflow and the new task appears when it
  matches the current List query.
- **GIVEN** mobile List is scrolled to the top, **WHEN** the user pulls downward
  beyond the release threshold, **THEN** the current filtered page is reloaded
  and a refresh indicator remains visible until the request settles.
- **GIVEN** mobile Kanban's current column is scrolled to the top, **WHEN** the
  user pulls downward beyond the release threshold, **THEN** current workflow
  snapshots are reloaded without changing the focused workflow or column.
- **GIVEN** either task listing is not at its vertical scroll top, **WHEN** the
  user drags downward, **THEN** the surface scrolls normally and no refresh is
  started.
- **GIVEN** mobile Kanban, **WHEN** the user swipes primarily horizontally
  between columns, **THEN** pull-to-refresh does not activate.
- **GIVEN** a task changes while Kandev is hidden or suspended, **WHEN** the
  user returns to List, **THEN** the changed task is reflected without a full
  browser reload.
- **GIVEN** a task changes while Kandev is hidden or suspended, **WHEN** the
  user returns to Kanban, **THEN** the changed task is reflected without a full
  browser reload even if connection status never left `connected`.
- **GIVEN** Kanban/Home remains visible in an unfocused desktop browser window
  while tasks change, **WHEN** the user focuses Kandev again, **THEN** the board
  reflects those changes without a full browser reload.
- **GIVEN** an open task receives messages or session-state changes while the
  Kandev window is unfocused, hidden, suspended, or offline, **WHEN** the user
  returns to Kandev, **THEN** the task details, session list, active chat
  messages, and queued-message state converge to their authoritative backend
  values without changing the selected task or session.
- **GIVEN** one return to Kandev emits `focus`, `visibilitychange`, `pageshow`,
  and/or `online` close together, **WHEN** foreground recovery runs, **THEN**
  each active data source issues at most one concurrent refresh.
- **GIVEN** a manual or automatic refresh fails, **WHEN** the request settles,
  **THEN** previously rendered tasks remain usable and a later pull or
  foreground event can retry.

## Out of scope

- Adding pull-to-refresh to task detail, Office, settings, or other Kandev
  pages.
- Changing the existing WebSocket event schema or backend task APIs.
- Periodically polling all Kandev data while the app remains continuously
  focused.
- Adding a floating create action on desktop List.
- Persisting pull distance, refresh timestamps, or refresh state.
- Changing List filters, sorting, grouping, pagination, or Kanban navigation.

## Implementation plan

- [Task surface refresh implementation plan](../../../plans/task-surface-refresh/plan.md)

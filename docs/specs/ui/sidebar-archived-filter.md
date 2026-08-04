---
status: building
created: 2026-08-04
owner: Kandev
---

# Sidebar Archived Filter Retirement

## Why

The task sidebar lists active workflow tasks, but its saved-view editor offers
an **Archived** filter that can only produce an empty list. Users need every
offered filter to operate on data the sidebar actually owns, without an
unreachable control suggesting that the sidebar is an archived-task browser.

## What

- Desktop and mobile sidebar view editors do not offer **Archived** as a filter
  dimension.
- Existing saved sidebar views that contain an `archived` clause discard that
  clause during frontend view migration while preserving the view, its other
  clauses, sort, grouping, collapsed groups, name, and active selection.
- In-flight sidebar drafts that contain an `archived` clause are migrated at
  boot, hydration, and live user-settings updates while preserving valid
  clauses and sort/group state.
- A migrated view whose only clause was `archived` behaves as an unfiltered
  sidebar view instead of remaining permanently empty.
- The sidebar continues to list active workflow tasks and may still show the
  existing synthetic archived row when the user is directly viewing an
  archived task.
- Archived-task browsing, archive badges, unarchive actions, and
  `include_archived` behavior on the full Tasks page remain unchanged.

## Scenarios

- **GIVEN** the desktop task sidebar, **WHEN** the user adds or changes a saved
  view filter, **THEN** **Archived** is absent from the dimension choices.
- **GIVEN** the mobile task-switcher sheet, **WHEN** the user adds or changes a
  saved view filter, **THEN** **Archived** is absent from the dimension choices.
- **GIVEN** a persisted saved view with an `archived` clause and another valid
  clause, **WHEN** Kandev hydrates the view, **THEN** only the `archived` clause
  is removed and the valid clause and all other view settings are preserved.
- **GIVEN** a persisted saved view whose only clause is `archived`, **WHEN**
  Kandev hydrates the view, **THEN** the view has no filters and active tasks are
  visible.
- **GIVEN** a persisted or live sidebar draft with an `archived` clause,
  **WHEN** Kandev restores or receives the draft, **THEN** the clause is
  removed before the editor applies it and the remaining draft state is
  preserved.
- **GIVEN** an archived task opened from the full Tasks page, **WHEN** the task
  sidebar renders, **THEN** the existing synthetic archived row can still
  identify the current task.

## Out of scope

- Loading archived task history into workflow snapshots or the task sidebar.
- Changing archive, unarchive, redirect, WebSocket eviction, or task-retention
  behavior.
- Changing the full Tasks page, command panel, or workspace task-list API.
- Renaming or deleting user-created saved views whose names mention archived
  tasks.
- Redesigning the desktop sidebar or mobile task-switcher sheet.

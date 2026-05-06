---
status: draft
created: 2026-05-03
owner: cfl
---

# Office Editable Task Properties

## Why

Today the task detail page's right-hand "Properties" panel is read-only:
Status, Priority, Assignee, Project, Parent, Blocked-by, etc. all render
as static text. Only Labels has an editor. Users have to drop to the
backend, the kanban view, or an agent prompt to change anything,
which breaks the obvious mental model that "I can click on a property
to change it". Result: friction every
time someone wants to triage a task by hand.

## What

- Every property row that holds an enumerable, lookup-able, or
  free-text value MUST be editable inline by clicking the value:
  Status, Priority, Assignee, Project, Parent task, Blocked by,
  Reviewers, Approvers.
- Read-only rows that derive from system state STAY read-only:
  Created by, Started, Completed, Created, Updated, Tree cost
  metrics. (They reflect events, not user intent.)
- Edits MUST be optimistic: the new value renders immediately; the
  panel reverts to the prior value with a toast on API failure.
- Pickers MUST be searchable when the candidate list could exceed
  ~10 items (Assignee, Project, Parent, Blocked-by). Status and
  Priority pickers stay as a fixed list.
- The editor MUST work without leaving the task detail page — no
  separate dialog, no navigation, just an inline popover.
- Changes MUST broadcast over the existing WS event stream so other
  open sessions update reactively. No polling.
- The Labels editor that already exists stays as-is functionally;
  visual styling MAY be aligned with the new pickers.

## Scenarios

- **GIVEN** a task is `in_review`, **WHEN** the user clicks the Status
  value and selects "Done", **THEN** the row updates to Done within
  100ms (optimistic) and the backend persists. Other open clients on
  the same task observe the change via the existing
  `office.task.status_changed` WS event.
- **GIVEN** the user clicks Assignee, **WHEN** the picker opens,
  **THEN** it shows a searchable list of agents in the workspace.
  Selecting an agent updates the task and triggers an
  `office.task.updated` event.
- **GIVEN** the user clicks Priority and chooses "High", **WHEN** the
  request fails, **THEN** the priority reverts to its previous value
  and a toast says "Failed to update priority".
- **GIVEN** the user clicks Project, **WHEN** they pick a project,
  **THEN** the row updates and the backend stores the new
  `project_id`.
- **GIVEN** the user clicks Parent, **WHEN** they search "KAN" and
  pick a candidate task, **THEN** the row shows the new parent and
  the task's `parent_id` updates server-side.
- **GIVEN** the user clicks Blocked by, **WHEN** they add a task,
  **THEN** the new blocker appears in the list and the dependency
  edge persists.

## Out of scope

- Editing fields for tasks the user does not own (no permission model
  changes — assume the current authenticated user / agent can edit any
  task in workspaces they have access to).
- Bulk-editing across multiple tasks.
- Editing reviewers / approvers when those roles aren't yet wired
  end-to-end (the backend may not have endpoints; if so, add them in a
  follow-up).
- A "no permission" UI state (gray-out) for fields the user cannot
  edit. Defer until a permission model exists.
- Drag-and-drop reordering of sub-tasks (this spec is about scalar
  properties).

## Open questions

- Should each property edit fire its own focused WS event
  (`office.task.priority_changed`, etc.) or piggyback on a generic
  `office.task.updated`? The plan picks one — leaning toward the
  generic event since the existing `office.task.updated` already
  triggers a refetch.
- Where do we draw the line between picker-with-search and
  picker-with-fixed-list? Suggest: ≤ 8 candidates → fixed list with
  no search input; > 8 → text input + filtered list. Plan finalizes.

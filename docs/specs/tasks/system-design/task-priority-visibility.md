---
status: current
system: tasks
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-001
  - REQ-TASKS-PRIORITY-VISIBILITY-002
  - REQ-TASKS-PRIORITY-VISIBILITY-003
  - REQ-TASKS-PRIORITY-VISIBILITY-004
created: 2026-09-03
owners:
  - kandev
---

# Task priority visibility System Design

## Purpose and boundaries

The task system owns the priority value and its update contract. The web board
owns only the presentation and the user actions that call this contract. The
Office task surface remains a separate consumer with its own vocabulary.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-PRIORITY-VISIBILITY-001 | Components and responsibilities, Control flow |
| REQ-TASKS-PRIORITY-VISIBILITY-002 | Data and contracts, Control flow |
| REQ-TASKS-PRIORITY-VISIBILITY-003 | Data and contracts, Control flow, Failure and recovery |
| REQ-TASKS-PRIORITY-VISIBILITY-004 | Data and contracts, Persistence |

## Components and responsibilities

- The task service authorizes task writes, validates task fields, and publishes
  `task.updated` after a successful mutation.
- The task repository stores the task row. Its
  `TaskPriorityRepository.UpdateTaskPriority` capability updates only
  `priority` and `updated_at` for a priority-only write. Full task updates keep
  their existing snapshot behavior for requests that change several fields.
- The task HTTP and WebSocket handlers keep using the existing task update
  request and response contracts. No priority-specific endpoint is added.
- The boot-state mapper includes priority in the task snapshot used by the
  initial kanban render.
- The web task API client uses the shared `TaskPriority` type. The kanban card
  renders a non-medium indicator, and the shared card menu sends priority
  changes through the existing update hook.
- The task creation dialog sends the selected priority token and uses the same
  four-token picker inside the Advanced settings section, which is collapsed by
  default, at every supported breakpoint. On wide layouts, the dependency picker
  is first and the priority picker is second. The priority label exposes a
  localized help description through a fine-pointer tooltip or a coarse-pointer
  touch disclosure.
- The mobile E2E scenario checks the touch target sizes and the complete create
  and update flow.

## Data and contracts

The canonical task priority type is the four-token set
`critical`, `high`, `medium`, and `low`. Tokens are stored and sent unchanged.
Labels are resolved from the active locale when the UI renders them.

Task creation and task update requests carry an optional `priority` token. A
successful update publishes the complete task in `task.updated`, so every
connected board can replace its task projection. A board refresh that does not
carry priority preserves the existing value in the client projection.

## Control flow

Creation follows this path:

`Create dialog -> Advanced settings -> priority picker -> task create request -> task service -> task row -> task.created -> board store -> card indicator`

The backend applies `medium` when a create request omits priority. The selected
token is present in the created task and in the event consumed by the board.

Priority changes follow this path:

`Card menu -> updateTask hook -> PATCH task -> task service -> field-scoped repository write -> reload -> task.updated -> WebSocket clients -> board store`

The service first authorizes and reads the task. For a request that contains
only priority, it uses `TaskPriorityRepository.UpdateTaskPriority`. The service
then reloads the row and publishes the resulting complete task. Requests that
also change another field continue through the normal full task update path.

## Failure and recovery

If the priority write fails, the service returns the error and does not publish
`task.updated`. The web hook reports the error with the existing localized toast,
and the card keeps its last known value. Repeating the current token is safe and
uses the same write and event path.

The database constraint remains the final guard for values outside the four
tokens. User-facing board controls can emit only the canonical tokens.

## Persistence

The `tasks.priority` column, default, and constraint already exist. This design
adds no migration. The field-scoped update changes only `priority` and
`updated_at`, so a concurrent priority action cannot replace a stale title,
description, metadata, workflow placement, parent, or position from a task
snapshot.

## Security and accessibility

The task service applies the existing task authorization before either update
path. Priority tokens are not translated at the API boundary. The card menu
and the creation picker expose localized accessible names. On coarse pointer
layouts, the card menu trigger, picker trigger, picker items, and priority help
 trigger use at least a 44 CSS-pixel hit target while the desktop layout keeps its
 compact controls. The priority help description is available on hover and focus
 for fine pointers, and from a touch disclosure for coarse pointers.

## Verification

- Backend tests verify the field-scoped repository write preserves other task
  fields and that the service uses it for a priority-only request.
- The mobile kanban test verifies the 44-pixel hit targets, localized controls,
  persistence, and the live `task.updated` result.
- Existing desktop priority tests continue to cover the card indicator, create
  dialog, both menu triggers, idempotent writes, and remote convergence.

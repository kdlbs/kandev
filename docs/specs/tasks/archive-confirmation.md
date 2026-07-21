---
status: shipped
created: 2026-07-15
owner: kandev
---

# Task Archive Confirmation

## Why

Frequent task archiving makes a mandatory confirmation dialog costly for users who understand the cleanup consequences. Users need to choose whether archive actions require explicit confirmation while retaining the safer confirmed behavior by default.

## What

- A user-level setting named **Confirm before archiving tasks** controls whether user-initiated archive actions require the archive confirmation dialog.
- The setting is enabled by default for new users and for existing users whose saved settings predate the preference.
- When enabled, archive actions continue to show the existing cleanup summary and optional subtask cascade control before archiving.
- When disabled, archive actions from every UI surface archive immediately without rendering the confirmation dialog.
- Confirmation-free archive actions do not cascade to subtasks. Users who need to archive subtasks together can temporarily enable confirmation and use the existing cascade control.
- Delete confirmations and programmatic archive operations are unaffected.

## Data model

`users.settings` stores `confirm_task_archive` as a boolean in the existing per-user JSON settings blob. A missing field is interpreted as `true` for backward compatibility.

## API surface

The existing user settings endpoints carry the preference:

- `GET /api/v1/user/settings`: `settings.confirm_task_archive: boolean`
- `PATCH /api/v1/user/settings`: optional `confirm_task_archive: boolean`
- `user.settings.updated` WebSocket payload: `confirm_task_archive: boolean`

No archive endpoint contract changes.

## Failure modes

- If saving the preference fails, the settings control returns to its previous value and archive behavior remains unchanged.
- If user settings have not loaded or omit the field, the client requires confirmation.
- Archive API failures continue to use each archive surface's existing error handling.

## Persistence guarantees

The preference survives backend and client restarts as part of the existing user settings record. It applies across workspaces for the current user.

## Scenarios

- **GIVEN** a new or upgraded user has not changed the preference, **WHEN** they request an archive from any UI surface, **THEN** the archive confirmation dialog is shown.
- **GIVEN** confirmation is enabled, **WHEN** the user cancels the archive dialog, **THEN** the task remains active.
- **GIVEN** confirmation is disabled, **WHEN** the user requests an archive from the sidebar, task banner, task card, list, pipeline, mobile task switcher, or bulk action, **THEN** the archive starts immediately and no archive confirmation dialog appears.
- **GIVEN** confirmation is disabled and a task has active subtasks, **WHEN** the user archives the task, **THEN** the parent is archived with cascade disabled and the subtasks remain active.
- **GIVEN** saving the preference fails, **WHEN** the request completes, **THEN** the control and archive behavior revert to the previously persisted value.

## Out of scope

- Disabling confirmation for task deletion or other destructive actions.
- Adding a per-archive cascade default when confirmation is disabled.
- Changing API, CLI, MCP, automation, or agent-driven archive behavior.

## Implementation plan

- [Archive Confirmation Preference](../../plans/archive-confirmation-preference/plan.md)

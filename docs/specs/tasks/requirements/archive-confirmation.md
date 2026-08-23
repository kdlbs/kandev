---
status: active
system: tasks
created: 2026-07-15
updated: 2026-08-12
owners:
  - kandev
---
# Task Archive Confirmation Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-ARCHIVE-CONFIRMATION-001: Task Archive Confirmation

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-ARCHIVE-CONFIRMATION-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Frequent task archiving makes a mandatory confirmation dialog costly for users who understand the cleanup consequences. Users need to choose whether archive actions require explicit confirmation while retaining the safer confirmed behavior by default.

## What

- A user-level setting named **Confirm before archiving tasks** controls whether user-initiated archive actions require the archive confirmation dialog.
- The setting is enabled by default for new users and for existing users whose saved settings predate the preference.
- When enabled, archive actions continue to show the existing cleanup summary and optional subtask cascade control before archiving.
- When disabled, archive actions from every UI surface archive immediately without rendering the confirmation dialog.
- Confirmation-free archive actions do not cascade to subtasks. Users who need to archive subtasks together can temporarily enable confirmation and use the existing cascade control.
- When an active task is archived, the rendered task and the task ID in the URL change together.
- If another live task remains, Kandev opens a task that is not part of the archive operation.
- If no task remains outside the archive operation, Kandev opens the task overview after the archive succeeds.
- A cascade archive never uses the parent or one of its descendants as the temporary or final navigation target.
- Desktop and mobile task switchers use the same post-archive navigation behavior.
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
- If an active task was temporarily replaced before an archive request fails, the client restores that task and its URL.
- If no safe replacement task exists, the client stays on the active task until the archive request succeeds.

## Persistence guarantees

The preference survives backend and client restarts as part of the existing user settings record. It applies across workspaces for the current user.

## Scenarios

- **GIVEN** a new or upgraded user has not changed the preference, **WHEN** they request an archive from any UI surface, **THEN** the archive confirmation dialog is shown.
- **GIVEN** confirmation is enabled, **WHEN** the user cancels the archive dialog, **THEN** the task remains active.
- **GIVEN** confirmation is disabled, **WHEN** the user requests an archive from the sidebar, task banner, task card, list, pipeline, mobile task switcher, or bulk action, **THEN** the archive starts immediately and no archive confirmation dialog appears.
- **GIVEN** confirmation is disabled and a task has active subtasks, **WHEN** the user archives the task, **THEN** the parent is archived with cascade disabled and the subtasks remain active.
- **GIVEN** an active parent has subtasks plus an unrelated live task, **WHEN** the user enables cascade archive, **THEN** Kandev opens the unrelated task, not a doomed descendant.
- **GIVEN** the active parent tree contains all live tasks, **WHEN** the user enables cascade archive, **THEN** Kandev waits for success and then opens the task overview.
- **GIVEN** an archive completes on desktop or mobile, **WHEN** the user opens another task, **THEN** the URL and rendered task update without a hard refresh.
- **GIVEN** an archive request fails after Kandev opened a replacement task, **WHEN** failure handling completes, **THEN** Kandev restores the original active task and URL.
- **GIVEN** saving the preference fails, **WHEN** the request completes, **THEN** the control and archive behavior revert to the previously persisted value.

## Out of scope

- Disabling confirmation for task deletion or other destructive actions.
- Adding a per-archive cascade default when confirmation is disabled.
- Changing API, CLI, MCP, automation, or agent-driven archive behavior.
- Changing the task switcher layout, archive dialog, or mobile composition.

## Implementation plan

- [Archive Confirmation Preference](../../../plans/archive-confirmation-preference/plan.md)
- [Cascade Archive Navigation](../../../plans/cascade-archive-navigation/plan.md)
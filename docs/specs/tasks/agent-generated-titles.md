---
status: shipped
created: 2026-07-31
owner: kandev
---

# Agent-Generated Task Titles

## Why

Writing a separate title adds friction when the task prompt already describes the work. Users who
prefer prompt-first creation need tasks and subtasks to appear immediately with a readable label while
letting the assigned agent replace that label with a concise title.

Decision: [ADR-2026-07-31-agent-generated-task-titles](../../decisions/2026-07-31-agent-generated-task-titles.md).

## What

- **Let agents name new tasks** is a per-user setting under **Settings → General → Task Actions**. It is
  disabled by default, including when the saved settings record predates the field.
- With the setting disabled, the existing task and subtask creation flows remain unchanged: their title
  inputs are visible and the user-supplied title is required. Sessions for those tasks receive neither
  the agent-title prompt instruction nor the `set_task_title_kandev` tool schema.
- With the setting enabled, the **New Task** and **New Subtask** dialogs hide their title inputs. Editing
  or manually renaming an existing task still shows the normal title control.
- Auto-titled creation requires a non-empty prompt. An empty prompt cannot create a task in this mode,
  because Kandev has no source for either a provisional title or the agent's first turn.
- Kandev creates the task with a provisional title before starting or preparing a session. It trims the
  user-visible prompt, splits it on whitespace, and joins its first six words with one ASCII space. A
  shorter prompt uses all available words. No ellipsis is appended and there is no normal
  character-count truncation; the existing absolute 500-character task-title limit remains a safety
  boundary for pathological words.
- The provisional title remains usable if session preparation, agent launch, MCP discovery, or the
  agent's title call fails.
- The first session launched while the title is pending instructs the agent to call
  `set_task_title_kandev` before any other work or tool call, even though the task already has a
  provisional title. The replacement summarizes the user's request with a target of three words and
  no more than six words when practical.
- A successful agent title call updates every live task surface through the normal `task.updated`
  event and ends the pending state. Later sessions are not instructed to rename the task again.
- A user or other ordinary task-title update made while the agent title is pending wins: it ends the
  pending state, and a later `set_task_title_kandev` call does not overwrite it.
- Task and subtask dialogs provide the same capability on desktop and phone. Their existing responsive
  surfaces, scroll ownership, repository/profile controls, and primary actions remain unchanged; the
  prompt becomes the first editable field when the title control is absent.

## Data model

`users.settings` stores `agent_generated_task_titles` as a boolean in the existing per-user JSON
settings blob. A missing field is interpreted as `false`.

An opted-in task stores `agent_title_pending: true` in its existing `tasks.metadata` JSON object. The
key is removed, rather than set to `false`, when an agent or ordinary title update resolves the title.
Tasks without the key are never treated as pending.

No database column or schema migration is required.

## API surface

### User settings

- `GET /api/v1/user/settings`: `settings.agent_generated_task_titles: boolean`
- `PATCH /api/v1/user/settings`: optional `agent_generated_task_titles: boolean`
- `user.settings.updated`: `agent_generated_task_titles: boolean`

### Task creation

`POST /api/v1/tasks` accepts `auto_title?: boolean`.

- When absent or `false`, `title` remains required and existing behavior is unchanged.
- When `true`, `description` must contain non-whitespace text, `title` may be omitted, and Kandev
  derives the persisted provisional title and pending marker from `description`.
- `auto_title` applies only to this creation request. It is not stored as a permanent task preference.

### Task MCP

A title-pending task-mode session exposes:

```json
{
  "name": "set_task_title_kandev",
  "arguments": {
    "title": "string"
  }
}
```

The tool targets the current task bound to the MCP server. A successful response contains
`{"accepted": true, "task_id": "...", "title": "..."}`. A task whose pending marker is already gone
returns `{"accepted": false, "reason": "title_not_pending", "task_id": "..."}` without mutation.
Blank titles and titles over the existing 500-character limit are validation errors.

The tool description tells the agent to set a short title for the current task before beginning work,
to target three words (and use no more than six when practical), and to make the call even when the
existing provisional title appears usable. The `title` argument repeats that guidance. The word counts
are generation guidance, not server-side rejection thresholds; the existing 500-character limit
remains the hard validation boundary.

`set_task_title_kandev` is registered only for a task-mode MCP server launched while the current task
has `agent_title_pending: true`. Ordinary task-mode sessions, including tasks created while the setting
is disabled, do not expose its schema. It is also absent from `ModeOffice`, `ModeConfig`, and
`ModeExternal`. Registration is stable for the lifetime of that MCP server; after a successful title
call, the current session may retain the now-idempotent tool, but later sessions omit it because the
pending marker is gone.

## Permissions

- Only the task agent connected through its task-bound MCP server can call `set_task_title_kandev`.
- The server injects the current task ID; the agent cannot supply or override it.
- Existing task-service authorization and task-owner scoping apply before the title is read or changed.
- Human title edits keep their existing permissions and take precedence by clearing the pending marker.

## Failure modes

- Saving the setting fails through the existing settings save coordinator; the persisted value and
  creation behavior remain unchanged.
- Auto-titled creation with an empty prompt returns a validation error and creates no task or session.
- If the agent never starts, cannot discover MCP, ignores the instruction, or receives an MCP error,
  the provisional title remains and the task stays pending for the next eligible first session.
- If two sessions try to set the title, only the first accepted pending update mutates it; later calls
  return `title_not_pending`.
- If a human changes the title before the agent call arrives, the human title remains and the tool call
  returns `title_not_pending`.
- If the title update cannot be persisted, the tool returns an error, the pending marker remains, and no
  success event is published.

## Persistence guarantees

The setting survives browser and backend restarts as part of backend-owned portable user settings and
applies across the current user's workspaces. The provisional title and pending marker survive backend,
session, and executor restarts as part of the task row. MCP catalog state is reconstructed from task
mode and the pending marker; the marker is re-read when Kandev launches a session and composes its
eligible first-turn instruction.

## Scenarios

- **GIVEN** the setting is missing or disabled, **WHEN** a user opens either creation dialog, **THEN**
  the title input is visible and required exactly as before.
- **GIVEN** a task was created while the setting was disabled, **WHEN** its task-mode session starts,
  **THEN** its system context contains no agent-title instruction and its MCP catalog does not contain
  `set_task_title_kandev`.
- **GIVEN** the setting is enabled, **WHEN** a user opens **New Task** or **New Subtask** on desktop or
  phone, **THEN** no title input is rendered and the prompt is the first editable field.
- **GIVEN** the setting is enabled and the prompt contains leading whitespace, line breaks, repeated
  spaces, and more than six words, **WHEN** the task is created, **THEN** its immediate title is the
  first six words joined by single spaces and it is marked title-pending.
- **GIVEN** the setting is enabled and the prompt contains fewer than six words, **WHEN** the task is
  created, **THEN** its immediate title contains every normalized prompt word without an ellipsis.
- **GIVEN** the setting is enabled and the prompt is empty, **WHEN** the user tries to create the task,
  **THEN** creation is blocked with prompt-required guidance and no task is created.
- **GIVEN** a title-pending task starts a structured task-mode session, **WHEN** Kandev composes its
  first-turn context, **THEN** the agent is told to call `set_task_title_kandev` before any other work,
  even though a provisional title exists, with a title targeting three words and no more than six when
  practical, and the session's MCP catalog exposes that tool.
- **GIVEN** a title-pending task starts a passthrough session, **WHEN** Kandev sends its initial launch
  prompt, **THEN** the equivalent short instruction precedes the user prompt in the native TUI.
- **GIVEN** the agent follows the tool guidance and calls `set_task_title_kandev` with a valid few-word
  title, **WHEN** persistence succeeds, **THEN** the task title changes, pending state ends, and
  connected task surfaces update.
- **GIVEN** a user manually renames a title-pending task, **WHEN** the agent later calls
  `set_task_title_kandev`, **THEN** the user title is preserved and the tool returns
  `title_not_pending`.
- **GIVEN** an agent title call fails or never occurs, **WHEN** the user returns after a restart,
  **THEN** the provisional title remains visible and the task still has a usable name.
- **GIVEN** an agent already resolved the pending title, **WHEN** a later session starts, **THEN** its
  first-turn context contains no instruction to rename the task and its MCP catalog omits
  `set_task_title_kandev`.

## Out of scope

- Changing titles supplied to `create_task_kandev`, Office tasks, automation runs, integration imports,
  plugin-created tasks, or external task-creation clients that do not opt into `auto_title`.
- Generating a title through a separate utility-agent request before task creation.
- Retrying or enforcing the agent tool call after the first-turn instruction.
- Removing manual task rename/edit controls.

## Implementation plan

- [Agent-Generated Task Titles](../../plans/agent-generated-task-titles/plan.md)

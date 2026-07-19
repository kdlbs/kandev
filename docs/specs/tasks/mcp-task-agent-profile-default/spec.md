---
status: shipped
created: 2026-07-19
owner: kandev
---

# MCP-Created Task Agent Profile Default

## Why

Agents can create tasks and subtasks through `create_task_kandev`. When the tool call omits `agent_profile_id`, inheriting the calling task's profile can unintentionally select an expensive model instead of the workspace's configured default. Users need a durable choice between preserving that inheritance behavior and using the target workspace's default profile.

## What

- Task Actions settings exposes a per-user **MCP-created task agent profile** choice with two values:
  - **Current task profile** (`current_task`): preserve the existing profile-resolution behavior.
  - **Workspace default profile** (`workspace_default`): skip the current task profile, honor any workflow profile, then use the default agent profile of the workspace that owns the new task.
- `current_task` is the default for new users and for existing settings that do not contain the preference. Selecting `workspace_default` is opt-in.
- The preference applies only to tasks and subtasks created by `create_task_kandev` when `agent_profile_id` is omitted.
- An explicit `agent_profile_id` always wins and does not require the preference to be read.
- In `current_task` mode, the existing fallback order remains unchanged: parent task or calling source task, workflow step or workflow default, then target workspace default.
- In `workspace_default` mode, parent/source task inheritance is skipped. The profile resolves from the workflow step or workflow default first, then from the target workspace default. This preserves workflow policy while avoiding inheritance from the calling task.
- Executor and executor-profile inheritance are unchanged in both modes.
- The selected profile is persisted in the created task's launch metadata even when `start_agent=false`, matching the existing deferred-start contract.
- The setting is portable per-user state. It applies across workspaces, while `workspace_default` dynamically resolves against each new task's target workspace.
- The setting is usable at narrow mobile widths without horizontal page scrolling, clipped labels, or hover-only interaction.

## Data model

The existing JSON settings object in `users.settings` gains:

```text
mcp_task_agent_profile_default  string  enum: current_task | workspace_default
```

The value is non-null in the normalized user-settings model. Missing or unrecognized stored values normalize to `current_task` for backward and forward compatibility. PATCH requests accept only the two documented values.

No relational schema migration is required because user settings are stored as JSON.

## API surface

The existing user-settings contracts gain `mcp_task_agent_profile_default`:

- `GET /api/v1/user/settings` returns `settings.mcp_task_agent_profile_default` as `current_task` or `workspace_default`.
- `PATCH /api/v1/user/settings` accepts an optional `mcp_task_agent_profile_default`. Omission leaves the value unchanged; an unsupported value returns the existing validation response and does not update settings.
- `user.settings.updated` includes `mcp_task_agent_profile_default` so open clients converge on the saved value.
- The Go-served SPA boot settings include `userSettings.mcpTaskAgentProfileDefault` with the same normalized enum value.

The `create_task_kandev` input and response schemas do not change. The preference changes server-side resolution only when its existing optional `agent_profile_id` input is empty. Its MCP tool description explains the two server-side default modes and states that workflow step/default profiles retain precedence over the workspace default.

## Permissions

- A user who can read or update their existing user settings can read or update this preference.
- The preference does not grant task-creation or agent-profile permissions. Existing `create_task_kandev`, workspace, workflow, and profile authorization rules remain in force.

## Failure modes

- If `workspace_default` is selected and neither the task's workflow nor the target workspace supplies an agent profile, `create_task_kandev` returns a validation error and creates no task, regardless of `start_agent`.
- If a workspace lookup required by the selected resolution chain fails, task creation fails and creates no partial task.
- If user settings cannot be read for an omitted-profile request, task creation fails and creates no partial task; the server does not silently choose another policy.
- An explicit `agent_profile_id` continues through existing validation and resolution even if reading the preference would fail.
- A failed settings save restores the last confirmed selection in the UI. A live update for another saved value remains authoritative over stale optimistic state.

## Persistence guarantees

- The preference survives backend restarts and frontend reloads as part of backend-owned portable user settings.
- Existing installations with no stored field behave exactly as `current_task`.
- Saving the preference publishes the existing user-settings update event; no browser storage is a durable fallback.
- Created tasks retain the resolved agent profile in task metadata, so later session creation does not re-evaluate a changed preference.

## Scenarios

- **GIVEN** an existing user with no stored preference, **WHEN** Task Actions settings loads, **THEN** **Current task profile** is selected.
- **GIVEN** `current_task` is selected and the calling task uses profile A while its workspace default is profile B, **WHEN** the agent creates a task without `agent_profile_id`, **THEN** the created task records profile A under the existing resolution rules.
- **GIVEN** `workspace_default` is selected, the calling task uses profile A, no workflow profile applies, and the target workspace default is profile B, **WHEN** the agent creates a top-level task without `agent_profile_id`, **THEN** the created task records profile B.
- **GIVEN** `workspace_default` is selected, a parent task uses profile A, no workflow profile applies, and its workspace default is profile B, **WHEN** the agent creates a subtask without `agent_profile_id`, **THEN** the subtask records profile B and retains the existing executor inheritance.
- **GIVEN** `workspace_default` is selected and the target workflow step or workflow default supplies profile W, **WHEN** the agent creates a task without `agent_profile_id`, **THEN** the created task records profile W instead of the caller or workspace-default profile.
- **GIVEN** `workspace_default` is selected and no workflow profile applies, **WHEN** the agent creates a task in a different workspace without `agent_profile_id`, **THEN** the new task uses that target workspace's default rather than the caller's workspace default.
- **GIVEN** either preference is selected, **WHEN** `create_task_kandev` includes profile C explicitly, **THEN** the created task records profile C.
- **GIVEN** `workspace_default` is selected and neither the workflow nor the target workspace has a default agent profile, **WHEN** an agent creates a task without `agent_profile_id`, **THEN** the tool returns a validation error and no task exists.
- **GIVEN** `workspace_default` is selected, no workflow profile applies, and a workspace default exists, **WHEN** an agent creates a task with `start_agent=false` and no `agent_profile_id`, **THEN** the task is created without a session and records the target workspace default for later launch.
- **GIVEN** the setting is open on a mobile viewport, **WHEN** the user selects **Workspace default profile**, **THEN** the choice saves and remains selected after reload without horizontal overflow.
- **GIVEN** a settings update fails, **WHEN** the user changes the choice, **THEN** the control returns to the last confirmed value.

## Out of scope

- Changing profile resolution for UI-created tasks, API-created tasks, automations, Office routing, or `spawn_session_kandev`.
- Changing workspace default profile configuration or agent-profile lifecycle rules.
- Changing executor or executor-profile inheritance.
- Selecting a specific profile directly from Task Actions settings; the tool's explicit `agent_profile_id` remains the per-call override.

## Implementation plan

See [`../../../plans/mcp-task-agent-profile-default/plan.md`](../../../plans/mcp-task-agent-profile-default/plan.md).

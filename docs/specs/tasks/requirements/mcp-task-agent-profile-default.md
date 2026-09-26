---
status: active
system: tasks
created: 2026-07-19
owners:
  - kandev
---
# MCP-Created Task Agent Profile Default Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-MCP-TASK-AGENT-PROFILE-DEFAULT-001: MCP-Created Task Agent Profile Default

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-MCP-TASK-AGENT-PROFILE-DEFAULT-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Agents create tasks and subtasks through `create_task_kandev`. Most coding
harnesses start delegated work with the configuration of the agent session that
created it. Kandev instead uses the profile that originally started the source
task, which is opaque when a task has multiple sessions or a session changes its
model.

Users need a predictable default that follows the creating session. They also
need an explicit workspace-default policy for cost control.

## What

- Task Actions settings exposes a per-user **MCP-created task agent profile**
  choice with two stored values:
  - **Creating session profile** (`current_task`) uses the verified session that
    called `create_task_kandev`. It copies that session's agent profile and its
    effective model, mode, and dynamic options.
  - **Workspace default profile** (`workspace_default`) skips the creating
    session and task profiles. It uses the workflow profile, then the target
    workspace default.
- `current_task` remains the stored default for new users and existing settings.
  The label changes because the creating session, not the task's original
  profile, owns the default.
- The preference applies to top-level tasks and subtasks created by
  `create_task_kandev` when `agent_profile_id` is omitted.
- The session-bound MCP server supplies `source_task_id` and `source_session_id`.
  These fields are server-owned context and are not tool arguments.
- The backend uses `source_session_id` only when the session exists and belongs
  to `source_task_id`. A live agent cannot select another session as the source.
- The creating session's effective configuration is its profile snapshot plus
  the latest provider runtime state and explicit runtime overrides. Later values
  override earlier values.
- Kandev stores the copied effective configuration as the initial session's
  runtime overrides. The copied values therefore beat later edits to the shared
  agent profile.
- The copied runtime configuration applies only when the creating session's
  profile wins profile resolution. Kandev does not mix the values into an
  explicit profile, a workflow-selected profile, or the workspace-default
  profile.
- An explicit `agent_profile_id` wins when the task does not land on a workflow
  step. It also prevents a user-settings read and prevents creator runtime
  inheritance.
- A task that lands on a workflow step uses that step's launch profile. This is
  the step's pinned profile, or the workflow default when the step has no pinned
  profile. This workflow profile wins over an explicit or inherited profile.
- In `current_task` mode, a call without verified session context keeps the
  compatibility fallback: parent or source task, workflow default, then target
  workspace default. This supports external MCP calls and deferred tasks that
  have no session.
- In `workspace_default` mode, parent and source task profiles remain skipped.
  The workflow profile wins before the target workspace default.
- Executor and executor-profile inheritance do not change. Subtasks continue to
  inherit executor configuration from the parent. Top-level tasks continue to
  inherit it from the source task.
- The resolved profile and initial runtime configuration persist when
  `start_agent=false`. The first session uses both values when a user starts the
  task later.
- The setting explains that `create_task_kandev` is the only affected tool. It
  states that `spawn_session_kandev` and UI-created tasks are not affected.
- The setting remains usable at narrow mobile widths without clipped text,
  hover-only information, or horizontal page scrolling.

## Data model

The existing JSON settings object in `users.settings` retains:

```text
mcp_task_agent_profile_default  string  enum: current_task | workspace_default
```

No user-settings migration is required. The `current_task` value now means the
verified creating session when one exists.

Task launch metadata gains a typed initial-session runtime configuration. It
contains the effective `model`, `mode`, and `config_options`. It contains no
credentials, prompts, or provider capability catalog.

The task session copies this launch configuration into its existing
`runtime_config_overrides` metadata when Kandev creates the initial session.
Later sessions do not copy the initial-session value.

## API surface

- The `create_task_kandev` public input and output schemas do not change.
- The session-bound MCP transport adds internal `source_session_id` context to
  the backend request. External MCP mode sends no session ID.
- `GET /api/v1/user/settings`, `PATCH /api/v1/user/settings`, the
  `user.settings.updated` event, and the SPA boot payload retain the existing
  `current_task | workspace_default` values.
- The MCP tool description explains creating-session inheritance, effective
  runtime configuration, override precedence, and the external fallback.

## Permissions

- Existing user-settings and task-creation permissions remain in force.
- The backend derives creator identity from the MCP server bound to the live
  session. The tool caller cannot supply or override that identity.
- Session lookup uses the existing task-session access rules.

## Failure modes

- If a request contains server-supplied creator context that does not resolve to
  the declared source task, task creation fails and creates no partial task.
- If Kandev cannot read the verified creator session, task creation fails and
  creates no partial task.
- If `workspace_default` is selected and no workflow or target-workspace profile
  exists, task creation returns a validation error and creates no task.
- If user settings cannot be read for an omitted-profile request, task creation
  fails. An explicit profile does not read this setting.
- If a provider no longer supports a copied model or option, the existing
  session-start error and model-fallback policy apply. Kandev does not silently
  substitute the source task's original profile configuration.

## Persistence guarantees

- The user preference survives restart as portable user settings.
- The resolved profile and initial runtime configuration survive restart in task
  metadata, including for `start_agent=false`.
- The initial task session receives an immutable copy of the launch seed in its
  own runtime overrides. Later changes to the source session do not change it.
- Existing installations with no stored preference continue to normalize to
  `current_task`.

## Scenarios

- **GIVEN** a task started with profile A and a second live session that uses
  profile B, **WHEN** session B creates a task without `agent_profile_id`,
  **THEN** the created task records profile B.
- **GIVEN** a live session changed from its profile model to another model and
  selected a new reasoning value, **WHEN** it creates a task without
  `agent_profile_id`, **THEN** the initial child session starts with the changed
  model and reasoning value.
- **GIVEN** a live session creates a top-level task in the same workspace,
  **WHEN** it omits `agent_profile_id`, **THEN** the new task inherits that
  session's profile and effective runtime configuration.
- **GIVEN** a live session creates a subtask under another task, **WHEN** it
  omits `agent_profile_id`, **THEN** the subtask inherits the creating session's
  agent configuration and the parent task's executor configuration.
- **GIVEN** `current_task` is selected and an external MCP caller has no session,
  **WHEN** it creates a subtask without `agent_profile_id`, **THEN** profile
  resolution uses the parent task compatibility fallback.
- **GIVEN** `workspace_default` is selected, **WHEN** a live session creates a
  task without `agent_profile_id`, **THEN** Kandev does not copy the source
  session profile or runtime configuration.
- **GIVEN** a call supplies `agent_profile_id`, **WHEN** the target task does not
  land on a workflow step, **THEN** Kandev uses that profile and does not copy
  the creating session runtime configuration.
- **GIVEN** a workflow step selects profile W, **WHEN** a live session creates a
  task on that step, **THEN** the created task uses profile W and does not copy
  the creating session runtime configuration.
- **GIVEN** a live session creates a task with `start_agent=false`, **WHEN** the
  user later starts its first session, **THEN** that session uses the persisted
  creator profile and runtime configuration.
- **GIVEN** creator context names a session from another task, **WHEN** Kandev
  handles the request, **THEN** creation fails before any task persists.
- **GIVEN** the Task Actions setting is open on a phone, **WHEN** the user reads
  and changes the choice, **THEN** all precedence information is touch-visible
  and the page has no horizontal overflow.

## Out of scope

- Changing profile resolution for UI-created tasks, API-created tasks,
  automations, Office routing, utility agents, or `spawn_session_kandev`.
- Changing executor or executor-profile inheritance.
- Adding a third preference value or changing the stored enum names.
- Copying credentials, environment variables, CLI flags, or MCP configuration
  from the creating session.
- Changing provider model fallback policy.

## Implementation plan

See [`../../../plans/creator-session-task-inheritance/plan.md`](../../../plans/creator-session-task-inheritance/plan.md).
---
status: draft
system: tasks
created: 2026-09-22
owners:
  - kandev
---
# MCP Create-Task Agent Profile Validation Requirements

## Overview

`create_task_kandev` accepts `agent_profile_id` as a free string. No code checks
that the value names an existing agent profile. A value that does not resolve
still creates the task, still counts as an explicit profile (so every
inheritance and workspace-default fallback is skipped), and only fails later
inside the fire-and-forget auto-start goroutine, whose error reaches the backend
log and nothing else. The caller has already received a success result.

The result is a task in `CREATED` with no session and no visible error. That is
worse than either a working task or a rejected call, because it looks like
success.

The `agent_profile_id` description also names `current_task` and
`workspace_default`. Those are the two values of the per-user setting
`mcp_task_agent_profile_default` defined in
[`mcp-task-agent-profile-default.md`](mcp-task-agent-profile-default.md); they
are not accepted argument values. The description does not say so, and callers
pass them as if they were.

## Requirements

### REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001: Create-task agent profile validation

**Intent:** `create_task_kandev` must resolve `agent_profile_id` before it
creates anything, and must never report success for a call that cannot produce
the task the caller asked for.

#### Acceptance criteria

- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.1:** An `agent_profile_id` that does not name an existing agent profile is rejected with a validation error before the task is created.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.2:** A rejected call creates no task, no session, and no deferred-launch record.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.3:** Passing the setting values `current_task` or `workspace_default` as `agent_profile_id` is rejected with a message that names the setting they belong to and states that omitting the argument selects it.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.4:** The `agent_profile_id` description distinguishes the accepted argument, an agent profile ID, from the setting values that govern the omitted-argument default.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.5:** Omitting `agent_profile_id` keeps the existing resolution precedence unchanged, including creating-session inheritance, workflow-step profiles, and the workspace default.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.6:** When `start_agent` is true and the asynchronous launch fails after the tool has returned, the failure is recorded on the task and delivered on the task event stream. It is not recorded only in the backend log.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.7:** **GIVEN** a caller passes `agent_profile_id: "current_task"`, **WHEN** the tool runs, **THEN** it returns a validation error naming the setting, and no task exists afterward.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.8:** **GIVEN** a caller passes an agent profile ID from another workspace or a deleted profile, **WHEN** the tool runs, **THEN** it returns the same validation error shape and creates no task.
- **AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.9:** **GIVEN** a valid profile and `start_agent` true, **WHEN** the asynchronous launch fails for an unrelated reason, **THEN** the task carries the failure reason and emits a task event carrying it.

## Out of scope

- Changing the omitted-argument resolution precedence or the
  `mcp_task_agent_profile_default` setting.
- Adding new sentinel argument values to `agent_profile_id`.
- Changing `spawn_session_kandev`, covered by
  [`../../agents/requirements/spawn-session-effective-profile.md`](../../agents/requirements/spawn-session-effective-profile.md).
- Validating `executor_profile_id`, `workflow_id`, or other create-task
  arguments beyond the existing admission checks.

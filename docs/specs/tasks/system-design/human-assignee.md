---
status: draft
system: tasks
requirements:
  - REQ-TASKS-HUMAN-ASSIGNEE-001
---

# Human Assignee System Design

## Purpose and boundaries

The task system owns the human-assignee value and its assignment flow. The
human assignee is independent of the agent profile that runs the task.

Authentication supplies the current mode and user identity. Organization units
define whether a selected user can reach the task workspace.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-HUMAN-ASSIGNEE-001` | [Data and contracts](#data-and-contracts), [Assignment flow](#assignment-flow), [Interface exposure](#interface-exposure) |

## Data and contracts

`models.Task.AssigneeUserID` stores the human assignee separately from
`AssigneeAgentProfileID`. An empty value means that the task has no human
assignee.

The task HTTP and WebSocket update requests accept `assignee_user_id`. The
Office task update route delegates this field to the task service.

Task DTOs, the boot payload, and `task.updated` events carry the field to
connected clients. Events include an explicit empty value when an assignment is
cleared.

## Assignment flow

The task service requires `task.write` for an assignment. The service accepts
self-assignment and reassignment without a confirmation step.

The service refuses a target user who cannot reach the task workspace. An
assignment does not change the agent, session, worktree, executor, or current
agent turn.

The frontend gets display names from the user directory and workspace member
list. The backend remains the authority for workspace reach.

## Interface exposure

The frontend auth state mirrors the backend auth payload. Authentication has
three modes: `disabled`, `setup`, and `enabled`.

Disabled mode intentionally provides an authenticated synthetic
`default-user`. Therefore, user presence and `authenticated` are not valid
tests for multi-user feature exposure.

Human-assignee controls and indicators are available only when
`auth.mode === "enabled"` and `auth.user` exists. All other states hide
these surfaces.

This gate applies to the task-topbar picker, the Office task property row, and
kanban card indicators. Hidden surfaces do not request directory data.

The gate changes presentation only. It does not clear `assignee_user_id` or
change the disabled-mode synthetic identity used by backend services.

The task topbar is a desktop surface. The Office property row uses the same
auth gate at each viewport, so this correction adds no mobile interaction or
layout.

## Failure and recovery

An absent or unknown auth state hides the feature. A later valid enabled state
lets React render the controls from the same stored task data.

Directory or member-list errors retain the existing fallback behavior. The
server returns assignment validation and authorization errors to the active
picker.

## Security

Interface hiding is not an authorization boundary. The task service always
enforces `task.write` and target-user reach.

The synthetic disabled-mode identity remains available to backend operations.
This behavior preserves single-user compatibility.

## Test strategy

Component tests use the real disabled-mode auth shape: `mode: "disabled"`,
`authenticated: true`, and a synthetic `default-user`.

The tests cover hidden topbar, Office, and card surfaces. Existing enabled-mode
tests continue to cover assignment and display behavior.

A Playwright test uses the default authentication-disabled fixture. It proves
that the task page does not show the picker and the board does not show a
persisted synthetic assignment.

## Related decisions

- [Opt-in Authentication and Per-User Workspace Scoping](../../../decisions/2026-07-24-opt-in-authentication.md)

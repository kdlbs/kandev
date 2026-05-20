---
status: draft
created: 2026-04-28
owner: cfl
---

# Granular Agent Permissions

## Why

Kandev's office module carries six permission keys (`can_create_tasks`, `can_assign_tasks`, `can_create_agents`, `can_approve`, `can_manage_own_skills`, `max_subtask_depth`) defined in `shared/permissions.go` and surfaced in the `/meta` endpoint. However, only one of those keys — `can_create_agents` — is actually enforced at the handler level. The rest are stored on the agent record and reported to the UI but never checked when an agent attempts the corresponding operation.

This creates concrete workflow failures:

- **Reviewer agents cannot reassign tasks.** A reviewer working through an execution-policy review stage needs to reassign (hand back) a task to a builder after writing feedback. The `can_assign_tasks` permission exists and defaults to `false` for workers/specialists, but no code checks it, so the question of whether the operation is permitted is never answered. Agents fall back on ad-hoc workarounds (comments, manual UI actions) or escalate to the CEO, adding latency.

- **Workers cannot create subtasks for other workers.** A worker decomposing a large task into subtasks via `CreateOfficeTask` needs to set an assignee. The `can_create_tasks` permission is meant to gate this, but it is never enforced. Whether an agent succeeds or fails depends entirely on which code path is hit — not on its declared permissions.

The mismatch between declared permissions and actual enforcement erodes trust in the permission model. Operators configuring roles expect the UI's permission defaults to reflect what agents can actually do.

## What

### Audit: current enforcement state

| Operation | Where it happens | Permission checked today |
|---|---|---|
| Create agent instance | `agents/handler.go createAgent` | `can_create_agents` — **enforced** |
| Delete / status change agent | `agents/handler.go` | CEO role check (not permission key) |
| Create task (kanban) | `task/service CreateOfficeTask` via `onboarding/service.go` | none |
| Assign/reassign task (`UpdateTaskAssignee`) | `service/execution_policy.go SetTaskAssignee` | none |
| Update task status (`PATCH /tasks/:id`) | `dashboard/handler.go updateTask` | none |
| Approve requests | `approvals/` | none |

`can_create_tasks` and `can_assign_tasks` are defined and shown in the UI but never checked at runtime.

### Permissions to enforce

Three permissions must work independently of the CEO role:

**`can_create_tasks`**
- Controls whether an agent may call `CreateOfficeTask` (create a task/subtask and set its assignee).
- Also controls the office task API endpoint once a direct-create endpoint exists.

**`can_assign_tasks`**
- Controls whether an agent may call `SetTaskAssignee` (change the `assignee_agent_instance_id` on an existing task).
- This is the operation reviewers need to hand tasks back to builders.

**`can_manage_own_skills`**
- Controls whether an agent may create or update skills it owns (`created_by_agent_instance_id = self`).
- Enforcement point: `skills/handler.go` when the caller is an authenticated agent.

### Default permission sets by role

The current defaults for worker/specialist leave `can_assign_tasks: false` and `can_manage_own_skills: false`. These defaults should be adjusted so common workflows don't require custom overrides:

| Role | `can_create_tasks` | `can_assign_tasks` | `can_manage_own_skills` | `can_create_agents` | `can_approve` | `max_subtask_depth` |
|---|---|---|---|---|---|---|
| CEO | true | true | true | true | true | 3 |
| assistant | true | true | true | false | false | 1 |
| worker | true | true | false | false | false | 1 |
| specialist | true | false | false | false | false | 1 |
| reviewer _(no dedicated role; use worker + override)_ | — | true | — | — | — | — |

Key changes from current defaults:
- **worker**: `can_assign_tasks` changes from `false` to `true`. Workers running execution-policy stages need to hand off tasks.
- **specialist**: retains `can_assign_tasks: false` — specialists typically complete bounded tasks and do not coordinate work.

Operators can override any key per agent instance via the `permissions` JSON field.

### Permission enforcement in the service layer

Enforcement belongs in the service layer, not the HTTP handler, so it applies to both API and internal callers (scheduler, wakeup, config-import). The service receives the caller agent ID and resolves its permissions before executing:

- `Service.CreateOfficeTaskAsAgent(ctx, callerAgentID, ...)` — resolves caller's permissions, checks `can_create_tasks`, then delegates to `taskCreator.CreateOfficeTask`.
- `Service.SetTaskAssigneeAsAgent(ctx, callerAgentID, taskID, assigneeID)` — checks `can_assign_tasks`, then calls `repo.UpdateTaskAssignee`.

For HTTP endpoints that already have an authenticated agent in context (`dashboard/handler.go updateTask`), the handler extracts the caller agent ID and passes it to the service method. UI/admin requests (no JWT) bypass the check.

### Permission enforcement in the CLI

`kandev task create --assignee <agent>` calls `CreateOfficeTask` through the agent's JWT. The service-layer check applies transparently.

`kandev task assign <task> <agent>` (a new subcommand to add in a later CLI spec) will call `SetTaskAssigneeAsAgent`.

### Permission enforcement in MCP tools

Office MCP tools that create or assign tasks call the same service methods and therefore inherit the same permission checks. No MCP-specific changes are needed beyond ensuring all tool implementations route through the service layer rather than calling the repository directly.

## Scenarios

- **GIVEN** a reviewer agent with `can_assign_tasks: true`, **WHEN** it calls `SetTaskAssigneeAsAgent(reviewerID, taskID, builderID)`, **THEN** the operation succeeds and the task's assignee is updated to the builder.

- **GIVEN** a specialist agent with `can_assign_tasks: false`, **WHEN** it calls `SetTaskAssigneeAsAgent(specialistID, taskID, otherAgentID)`, **THEN** the call returns `ErrForbidden` and the assignee is unchanged.

- **GIVEN** a worker agent with `can_create_tasks: true` (new default), **WHEN** it calls `CreateOfficeTaskAsAgent(workerID, wsID, projectID, anotherWorkerID, title, desc)`, **THEN** the subtask is created with the specified assignee.

- **GIVEN** any agent without a valid JWT, **WHEN** a request reaches the API, **THEN** the permission check is skipped (treated as a UI/admin request) — no behavior change for existing UI flows.

## Out of scope

- Adding a dedicated `reviewer` role (out of scope; use role `worker` with `can_assign_tasks: true` override).
- Enforcing `can_approve` on the approvals endpoints (separate feature).
- Adding `can_manage_own_skills` enforcement to the skills handler (can follow in the same PR or a follow-up).
- Auditing or changing the `updateTask` status-change handler's permission model (currently no restriction; leave as-is for now).

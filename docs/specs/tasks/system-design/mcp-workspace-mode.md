---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-002
  - REQ-TASKS-MCP-WORKSPACE-MODE-003
---

# MCP task workspace mode design

## Boundary and evidence

This design changes task-creation admission, not Office task creation.
`server.ModeOffice` explicitly excludes Kanban tools because Office agents
use CLI commands. `server.profileToolGroups` exposes generic creation to
Kanban and external callers, but not Office sessions.

Office guidance under `internal/office/configloader/skills/` directs agents
to CLI task creation. `office/runtime/handler.go` registers
`POST /runtime/tasks`; `office/runtime.Actions.CreateTask` enforces the
run's capabilities and workspace/parent/project checks.

Before this change, `internal/mcp/handlers/handlers.go:handleCreateTask`
accepted a destination workspace without enforcing the session caller's mode.
Its assignee-field rejection did not prevent Office targeting. The implemented
handler now rejects Office sessions and wrong-mode destinations before
repository resolution, and revalidates deduplicated results before exposing
them.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-MCP-WORKSPACE-MODE-001 | Trusted admission; Destination resolution |
| REQ-TASKS-MCP-WORKSPACE-MODE-002 | Surface contract |
| REQ-TASKS-MCP-WORKSPACE-MODE-003 | External compatibility; Errors |

## Surface contract

| Caller | Creation path | Destination |
| --- | --- | --- |
| Kanban session | `create_task_kandev` | Authorized Kanban workspaces |
| Office session | Existing skills and CLI | Its Office workspace and run scope |
| External MCP | `create_task_kandev` | Authorized workspaces of either mode |

Do not register `create_office_task_kandev`, add a new backend creation action,
or reconstruct an Office run for MCP creation. Preserve the Office catalog,
CLI schema, runtime permissions, assignment, and scheduler.

Keep generic MCP arguments and result envelopes intact. `workspace_id`
selects the logical workspace; `workspace_mode` selects a subtask's
materialized-workspace policy. Neither grants authority. In particular,
`agent_profile_id` remains a launch profile, not an Office assignee.

This follows the existing [typed MCP profile decision](../../../decisions/2026-08-08-mcp-tool-profiles.md);
no amendment adding Office creation is needed.

## Trusted admission

Reuse `scope.Principal` and `Resolver.ScopePrincipal`, which derive identity
from the bound execution's task and session. Reuse
`origin.IsTrustedExternalTransport`, set only by
`server.NewExternalDispatcherBackendClient`, to recognize external calls.

Add a focused guard in MCP creation before repository resolution and other
side effects. Keep this caller policy outside generic
`task/service.Service.CreateTask`, which also serves native and internal callers.

- Verified Office session: reject MCP task creation, regardless of destination.
- Verified Kanban session: resolve the stored source workspace and enforce
  Kanban destination/task classification.
- Trusted external transport: permit either destination mode, subject to
  existing access controls.
- Missing identity or conflicting principal/external markers: reject.
- Automation/configuration: retain their existing explicit policies. Do not
  accidentally widen or remove the automation creation catalog.

Use `Workspace.OfficeWorkflowID` for workspace classification and the
existing `Task.IsFromOffice` projection for stored task references.
A workspace lookup failure is not a Kanban classification. If a session's
surface/task classification disagrees with its workspace, deny creation until
the backend context is reconciled.

Preserve trusted source attribution. Reject source IDs conflicting with the
principal; external requests cannot fabricate creator-session context.
User authentication and workspace authorization remain independent checks.
Disabling optional user authentication does not disable this admission policy.

## Destination resolution

Resolve explicit input without fallback. Otherwise use the authorized parent,
then the principal workspace. External roots may auto-select only a sole
authorized workspace across both modes; multiple choices require an explicit ID.

Validate parent, workflow, step, project, and repository associations without
registering repositories or mutating state. Parents cannot cross workspaces.
Kanban root creation in another permitted Kanban workspace must not inherit
source repositories owned by a different workspace. Preserve applicable
profile precedence after admission.

Kanban sessions cannot target Office workspaces, Office parents, or
project-linked Office tasks, including inconsistent legacy combinations.
External callers retain both modes; selecting an Office workspace must not
make the external caller an Office session.

Run admission before remote repository registration, task insertion,
idempotency lookups that return task data, and launch. Revalidate the mode and
visibility of the task returned on an external-ID retry. A rejected request
must not mutate or leak an existing wrong-mode task.

## External compatibility

Keep `registerCreateTaskTool` and `registerConfigTaskTools` on the external
surface. Do not require existing external Office callers to rename their tool.
Keep existing creation fields, assignee limitations, launch behavior, and
idempotency outcomes. This package does not add external assignment support.

Verify existing external listing, reads, moves, state updates, archive, and
delete against both task modes. These operations retain their own authorization
and lifecycle checks; the creation gate is not a general management guard.

Use isolated composition tests to prove the transport marker survives external
dispatch, foreign targets are denied, and both destination modes remain
reachable. Do not create tasks in a developer's live workspace.

## Errors, persistence, and documentation

Use existing MCP tool-error and WS validation/not-found envelopes.
For an authorized Kanban caller targeting Office, explain that session MCP
creation is restricted to Kanban. For an Office session, direct it to its
skills/CLI creation path. Do not suggest connecting through external MCP as a
way for a session to bypass its scope.

Unknown and inaccessible resources return the same error. Log trusted IDs
and a rejection reason using existing logging; do not log submitted prompts
or credentials.

No migration, mode column, historical rewrite, new endpoint, or UI is planned.
Update public reference documentation when the gate is implemented.
The earlier proposal for a separate Office MCP creation tool is withdrawn.

## Related contracts

- [Workspace-owned mode](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md)
- [Office tasks](../../office/requirements/tasks.md)
- [External MCP](../../integrations/requirements/external-mcp.md)
- [Kanban profile defaults](../requirements/mcp-task-agent-profile-default.md)
- [External-ID boundaries](../requirements/external-id-idempotency-boundaries.md)
- [Implementation plan](../../../plans/mcp-workspace-mode/plan.md)

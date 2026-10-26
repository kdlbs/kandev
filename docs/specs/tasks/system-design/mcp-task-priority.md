---
status: current
system: tasks
requirements:
  - REQ-TASKS-MCP-PRIORITY-001
---

# MCP task priority design

## Ownership and mapping

The task system owns priority creation and mutation. MCP registration and
transport extend the existing task service rather than introduce another writer.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-MCP-PRIORITY-001 | Tool contracts; Request mapping; Validation; Compatibility and authorization; Verification |

## Tool contracts

`create_task_kandev` gains optional string `priority`. Its enum uses the four
`models.TaskPriority*` constants. Advertise `medium` with `mcp.DefaultString`.
The backend service remains responsible for creation defaulting.

`update_task_kandev` gains the same optional enum without a default. Its existing
`task_id` requirement and other fields remain intact. A caller can submit
`{"task_id":"task-id","priority":"high"}` without sending state.

Extract the existing update registration from `registerKanbanTools` into
`registerUpdateTaskTool` in `internal/mcp/server/server.go`. Reuse it from the
Kanban registration and a new external-only `profileToolGroups` entry.
Automation continues to obtain the same tool through its existing Kanban group.
Do not register the generic update tool in configuration mode.

The external catalog adds one existing canonical tool. This deliberately
extends the external config-plus-create surface. Task and automation catalog
names remain stable. The state-only configuration tool stays unchanged.

## Request mapping

Creation crosses three existing boundaries:

1. `server.createTaskHandler` includes supplied priority in `mcp.create_task`.
2. `handlers.mcpCreateTaskRequest` in `create_task_mode.go` decodes priority.
3. `handlers.handleCreateTask` forwards it into `service.CreateTaskRequest`.

Omission leaves the service input empty so its existing `defaultPriority`
applies. Priority is not inherited from the parent or creating session.

Update uses the existing `mcp.update_task` action:

1. `server.updateTaskHandler` forwards priority only when supplied.
2. `handlers.handleUpdateTask` decodes `Priority *string` and retains nil for
   omission.
3. Its `service.UpdateTaskRequest` carries the same pointer without synthesizing
   title, description, state, or other fields.

`Service.UpdateTask` already recognizes `isPriorityOnlyTaskUpdate` and uses
`tryUpdateTaskPriorityOnly`. This path authorizes the write, updates priority
and its timestamp, reloads the task, and publishes `task.updated`.
Combined updates retain the existing service field-update path.
Keep the existing compact write response, including description omission.

`dto.FromTask`, task listing, and the task event payload already include
priority. No frontend projection or database change is necessary.

## Validation and failure behavior

Both registrations use `mcp.Enum` and the standard `wrapHandler` path.
`compileToolArgumentSchema` rejects invalid values before dispatch.
Do not trim, translate, coerce, or normalize priority tokens.

The backend creation handler validates nonempty priority with
`service.ValidateTaskPriority` immediately after payload validation, before
repository resolution and creation. Empty internal creation input retains the
service's omission convention. Explicit empty MCP arguments fail schema validation.

Before `applyDeferredLaunchPromptUpdate`, the update handler validates every
provided priority, title length, and normalized state. A rejected field cannot
replace a deferred prompt first. Return `ws.ErrorCodeValidation` through the
existing error envelope. The task service retains its own priority and title
validation.

An omitted update priority never receives a schema or server default.
The task service supplies error and event behavior for failed writes.
Do not bypass it with a repository update or add an alternate event publisher.

## Compatibility and authorization

| Surface | Create priority | Update priority | Boundary |
| --- | --- | --- | --- |
| Task and task-title-pending | Existing create tool | Existing generic update tool | Bound task/session scope |
| Automation | Existing create tool | Existing generic update tool | Existing workspace and target guard |
| External | Existing create tool | Newly registered generic update tool | Authenticated transport and task service |
| Configuration | No create tool | No generic update tool | Existing state-only tool remains |
| Office and coordinator | No generic creation | No generic update | Existing catalog and runtime policies |

`NewExternalDispatcherBackendClient` preserves external identity and auth context.
`Service.UpdateTask` enforces task-write and executor authority.
`Service.UpdateDeferredLaunchPrompt` enforces task-write and executor authority
before mutation.
The guarded dispatcher retains automation target admission and self-denial.
Do not alter permission maps, trust markers, task scope, or Office CLI routes.
Test supported profiles explicitly rather than infer support from shared code.

External clients already create and manage authorized Kanban and Office tasks.
The added generic update tool uses that same task-service authority.
It does not grant Office-session capabilities or Office-specific assignment.

`external_id` remains create-if-absent. The existing Found paths return the
stored DTO without applying submitted priority, including when priority is omitted.

## Verification

Server tests cover enum schemas, optionality, default metadata, forwarding,
mode visibility, and rejected values without dispatch. Backend tests use the
real task service and temporary SQLite databases for persistence and events.
They cover defaults, parent non-inheritance, omission, retry, field preservation,
validation before deferred-prompt writes, and denied targets.

An external Streamable HTTP journey uses `NewExternal`, registered backend
handlers, and SQLite. It exercises create, list, update, and list again.
It also proves auth context propagation, viewer denial before a deferred-prompt
write, authorized owner updates, and denied-target preservation.
This is the end-to-end boundary for this API change. Existing task events and
UI priority consumers are reused without rendered UI changes.

Update the public MCP reference and coordination guide with creation defaults,
priority-only updates, enum validation, and deduplicated-create behavior.
Update catalog assertions to reflect the single external addition.

## Related contracts and decisions

- [Task priority visibility design](task-priority-visibility.md)
- [MCP workspace admission](mcp-workspace-mode.md)
- [External-ID boundaries](../requirements/external-id-idempotency-boundaries.md)
- [Typed MCP profiles](../../../decisions/2026-08-08-mcp-tool-profiles.md)
- [Shared argument validation](../../../decisions/2026-08-01-validate-mcp-tool-arguments.md)
- [Implementation plan](../../../plans/mcp-task-priority/plan.md)

This additive task-field extension follows existing boundaries. Its local tool
reuse does not require a new architecture decision. A separate priority tool
would duplicate the update path. Extending the state-only tool would couple
priority with an unrelated transition contract.

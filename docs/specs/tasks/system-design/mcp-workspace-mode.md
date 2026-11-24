---
status: current
system: tasks
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-002
  - REQ-TASKS-MCP-WORKSPACE-MODE-003
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
---

# MCP task workspace mode design

## Boundary and evidence

This design covers MCP task-creation admission and subtask materialization.
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
| REQ-TASKS-MCP-WORKSPACE-MODE-004 | Subtask materialization policy, Inherited repository compatibility, Materialization schema discoverability |

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

### Subtask materialization policy

Resolve the admitted parent, including `parent_id="self"` redirection, before
`resolveMCPWorkspacePolicy` chooses the mode. Resolve the mode before mapping
the parent's task repositories. An omitted mode with a resolved parent becomes
an explicit service policy of `new_workspace`. This keeps the existing MCP
precedence over a saved `default_child_workspace`. The policy continues to
carry the parent's child ordering through the normal task service path.

When the caller omits repositories, treat a fresh child as a new branch
selection. Keep the parent's repository IDs, base selections, and order.
Apply a supplied top-level base override before combining equivalent
selections. Group by repository ID and exact effective Git base ref, using the
saved repository default for an empty base when known. Retain the first
selection in task-repository order. Send empty checkout branches and no
inherited placement so the existing Worktree allocator creates a fresh branch
per selection under the child's environment.

This normalization is limited to repository inputs derived from the resolved
parent for `new_workspace`. Explicit repository inputs use the normal create
validation, including duplicate rejection and contribution binding. Existing
top-level selection, profile inheritance, logical workspace, and hierarchy
behavior use their original paths. [Issue #4606](https://github.com/kdlbs/kandev/issues/4606)
owns default parity across other task-creation surfaces.

### Inherited repository compatibility

An explicit repository selection does not change what `inherit_parent` means:
the child must attach to the parent's existing materialized checkout. Before
task insertion, the MCP creation handler asks the task service to validate
explicit repository inputs against the parent's active task environment when
one exists. Each requested repository plus effective branch identity must match
exactly one active `task_environment_repos` row. Deleted, failed, missing, or
duplicate matches are unsafe.

The validation runs after repository defaults and workspace policy are resolved
but before remote-contribution registration, task insertion, session creation,
or launch. It is read-only. A mismatch returns the existing validation envelope
with bounded guidance to select `workspace_mode=new_workspace`; it does not
rewrite the repository input, silently change workspace mode, or alter the
parent environment. A parent without a materialized environment retains the
existing launch-time validation path.

When the caller explicitly asks to share and omits repositories and branch
overrides, copy the parent's ordered task-repository rows with their existing
`BaseBranch`, `CheckoutBranch`, and `WorkspaceRelativePath`. Mark a copied
checkout name as reuse-only in the existing repository metadata.

Extend `ValidateInheritedWorkspaceRepositorySelection` to check copied slots
after external-ID lookup and before insertion. For a ready Worktree parent,
use the existing branch identity planner and include relative placement in
the match, following the same canonical slot rules as launch. Keep distinct
row selection. When no canonical slot matches, the exact Git branch name may
identify its unique active holder at that placement; reject multiple holders.
Retain launch compatibility for a legacy unscoped Worktree row when no scoped
rows for that repository are live. Count only live Worktree rows with a
worktree ID. On failure, return the repository and requested checkout or base
through the existing validation response. If the parent is still preparing,
the existing launch check runs once inventory is ready.

The existing launch matcher and workspace group binding reuse the worktree
IDs. Use the copied name to derive the slot. Omit that name from fresh Git
requests so an eligible executor transition creates a new branch from the
saved base for the same slot.

This creation-time repository check does not select the runtime workspace. A
later executor mismatch follows the [additional-session workspace reuse
transition](additional-session-workspace-reuse.md#executor-transition), which
can detach a new non-Kubernetes launch without changing the requested
`workspace_mode` or the parent's environment.

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

## Materialization schema discoverability

The task system owns this addition because the field is a task-creation
contract. Workspace lifecycle, transport discovery guidance, and Office
admission remain with their existing owners.

In `internal/mcp/server/server.go`, `registerCreateTaskTool` adds
`mcp.Enum("inherit_parent", "new_workspace")` to the existing
`mcp.WithString("workspace_mode", ...)` in `registerCreateTaskTool`.
The pinned `mcp-go` v1.0.0-beta.1 supports this property option; existing
`delivery_mode` and profile-session-policy fields use it. Do not add
`mcp.Required()` or a default. Both `ModeTask` (through `registerKanbanTools`)
and `ModeExternal` use this registration.

The field description explains the separate workspace default for subtasks,
how to name `inherit_parent` to share a parent's files, and the existing
executor and Kubernetes limits. The handler trims internal input, defaults
blank mode with a parent to `new_workspace`, and leaves blank root policy
unspecified. It still rejects `inherit_parent` without a parent. The schema
continues to advertise exactly two explicit modes.

### Compatibility and schema propagation

`compileToolArgumentSchema` marshals the registered schema and only adds root
`additionalProperties: false`; it preserves enum metadata. Validation runs
before dispatch. `normalizeToolArguments` only handles the legacy prompt alias,
so the enum rejects empty, whitespace-only, and padded strings that
previously reached the trimming handler. This is a real MCP input narrowing,
not merely a description improvement. Callers should omit the key to request
defaulting, or send an exact enum value. Do not introduce normalization or alter
the backend to conceal this effect. This follows the existing
[schema validation decision](../../../decisions/2026-08-01-validate-mcp-tool-arguments.md).

`mcpToolInputSchema` preserves raw schemas or marshals structured schemas for
attachment evidence. `cloneValidMCPInputSchema` in
`internal/agentctl/types/streams/mcp_attachment.go` copies valid JSON unchanged;
size limits can drop a whole schema with a truncation marker, not individual
enum keywords. No enum-stripping transform was found in the inspected MCP
registration, validator, or attachment paths. Regression tests verify serialized
catalog and attachment schemas; this does not prove behavior of arbitrary
external clients.

Schema and wrapped-call tests in `server/create_task_workspace_mode_test.go`
cover task/external registration, exact enum order, optionality, absent default,
description semantics, both accepted values, omission, and rejected
blank/unsupported inputs without dispatch. The direct policy matrix in
`handlers/workspace_policy_test.go` and existing handler tests preserve root,
parent, trimming, and rejection behavior. The public MCP reference and
coordination guide document omission and blank-input compatibility. The
Worktree end-to-end fixture drives the MCP tool to verify workspace isolation
and sharing.

- [Enum implementation plan](../../../plans/mcp-workspace-mode-enum/plan.md)

## Related contracts

- [Workspace-owned mode](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md)
- [Office tasks](../../office/requirements/tasks.md)
- [External MCP](../../integrations/requirements/external-mcp.md)
- [Kanban profile defaults](../requirements/mcp-task-agent-profile-default.md)
- [External-ID boundaries](../requirements/external-id-idempotency-boundaries.md)
- [Implementation plan](../../../plans/mcp-workspace-mode/plan.md)
- [Subtask workspace repair](../../../plans/mcp-subtask-workspaces/plan.md)

---
status: current
system: office
requirements:
  - REQ-OFFICE-AGENT-TASK-RELATIONS-001
  - REQ-OFFICE-AGENT-TASK-RELATIONS-003
  - REQ-OFFICE-AGENT-TASK-RELATIONS-004
---

# Office Agent Task Relations System Design

## Purpose and boundaries

Office owns the signed runtime task-create action
(`internal/office/runtime`), the Office service that checks
`can_create_tasks` (`internal/office/service`), and the
`agentctl kandev task create` command. This design
extends them so an agent-created task can carry blockers and a workspace
policy.

Contracts used but not owned:

- The task system's dependency validator and edge store
  (`taskservice.Service.AddDependency`, `task_blockers`).
- The task system's workspace policy (`taskservice.WorkspacePolicy`,
  `prepareWorkspacePolicyForCreation`, `AttachWorkspacePolicy`), already used by
  the MCP surface ([MCP workspace mode](../../tasks/system-design/mcp-workspace-mode.md)).
- The shared read relation (`canReadDocuments` in
  `internal/task/service/handoff_access.go`), already used by document and
  comment reads ([agent comment reads](agent-comment-reads-01.md)).

This design replaces the scenario in [agents part 3](agents-03.md) that says
`task create` rejects blocker and workspace-policy flags.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENT-TASK-RELATIONS-001` | [Runtime input and validation](#runtime-input-and-validation), [Blocker scope](#blocker-scope), [Creation path](#creation-path), [Cycles and ancestor blockers](#cycles-and-ancestor-blockers) |
| `REQ-OFFICE-AGENT-TASK-RELATIONS-003` | [Runtime input and validation](#runtime-input-and-validation), [Creation path](#creation-path) |
| `REQ-OFFICE-AGENT-TASK-RELATIONS-004` | [CLI surface](#cli-surface), [Agent-facing documentation](#agent-facing-documentation) |

## Runtime input and validation

`runtime.CreateTaskInput` keeps its JSON shape. `unsupportedField` shrinks to
`priority`, `workspace_group_id`, `default_child_workspace`, and
`default_child_ordering`; the handler
keeps returning `400 "<field> is not supported by Office runtime task create"`
for them (`AC-OFFICE-AGENT-TASK-RELATIONS-004.3`).

`Actions.CreateTask` normalizes the two accepted fields before any lookup,
after the existing capability and parent-scope checks:

- `blocked_by`: each entry is trimmed; an empty entry, an entry over 128
  characters, or more than 512 entries returns a new caller-correctable
  `ErrInvalidTaskRelations` (mapped to `400` with a `runtime.denied` event in
  `respondRuntimeError`). Duplicates are removed, keeping first-seen order
  (`AC-…-001.4`, `AC-…-001.5`). The limits match the task service's
  `maxTaskDependencyCount` and `maxTaskDependencyIDLength`.
- `workspace_mode`: trimmed; empty, `inherit_parent`, or `new_workspace`.
  `inherit_parent` without `parent_id` and any other value, including
  `shared_group`, return `ErrInvalidTaskRelations` naming the field and the
  accepted values (`AC-…-003.6`, `AC-…-003.7`).

Validation runs before `resolveRootTaskProject` and `validateTaskRelations`, so
malformed input never causes a read.

## Blocker scope

`Actions.validateBlockerScope` (`internal/office/runtime/task_relations.go`)
runs after the existing project, parent, and assignee checks in
`validateTaskRelations`. The wildcard scope sentinel is never a task ID and is
refused outright. For each other normalized blocker ID:

1. If `runCtx.CanMutateTask(id)` is true, the blocker must also be in the
   run's workspace (`TaskCreator.GetTaskWorkspaceID(id) == runCtx.WorkspaceID`).
   A missing row returns `""` and fails this check.
2. Otherwise the run must have a task, and the new runtime dependency
   `TaskReadRelation.CanReadTask(ctx, runCtx.TaskID, id)` must return true. The
   relation already requires one workspace and resolves a missing task to a
   plain denial.

Any failure returns `ErrBlockerOutOfScope`, which wraps `shared.ErrForbidden`
so the handler maps it to `403` and records `runtime.denied`. The message is
`blocked_by[<index>] is not a task this run may depend on` for every cause, so
a missing, foreign-workspace, and unrelated task are indistinguishable
(`AC-…-001.2`, `AC-…-001.3`). A read error is returned unwrapped and surfaces
as `500`.

The scope rule is the read relation rather than workspace membership because a
blocker edge widens what the new task can read: `canReadDocuments` admits a
task's blockers. Admitting any workspace task as a blocker would let an agent
read any task's documents and comments by creating a dependent task. Edges are
only declared on the task being created, so an agent can delay only work it
creates.

`TaskReadRelation` is a one-method interface in `internal/office/runtime`.
`taskservice.HandoffService` implements it with an exported `CanReadTask` that
calls `canReadDocuments`, so runtime blockers, document reads, and comment
reads share one rule. `office.RegisterAllRoutes` (`internal/office/routes.go`)
wires the `HandoffService` it already receives into
`runtime.ActionDependencies.TaskReads`, guarding against a typed-nil pointer.
An unwired dependency returns `ErrRuntimeDependencyMissing` only when a blocker
falls outside the run's mutation scope and needs the relation.

## Creation path

The accepted fields travel as one value type, `shared.AgentTaskRelations
{BlockedBy []string; WorkspaceMode string}` in
`internal/office/shared`, which both `runtime` and `service` already import.
The runtime `TaskCreator` methods, `service.Service.CreateOfficeTaskAsAgent` /
`CreateOfficeSubtaskAsAgent`, and the Office `TaskCreator` / `SubtaskCreator`
adapter interfaces each gain this parameter next to the existing
`causingRunID`, which is unchanged. Onboarding's `CreateOfficeTask`
is unchanged.

In `internal/backendapp/adapters_office.go`:

- Root create sets `CreateTaskRequest.BlockedBy` and, when `workspace_mode` is
  set, `CreateTaskRequest.WorkspacePolicy = &WorkspacePolicy{Mode}`. With no
  policy, `prepareWorkspacePolicyForCreation` behaves as today.
- Subtask create passes the relations into `taskservice.ChildTaskSpec`.

`taskservice.ChildTaskSpec` gains `BlockedBy` and `WorkspaceMode`, next to its
existing `Metadata` and `OfficeCarrierMetadata`. `CreateChildTask` uses
`spec.WorkspaceMode` when it is set. When it is empty, `CreateChildTask` keeps
setting `inherit_parent` exactly as today (`AC-…-003.5`). An explicit mode wins
over a parent default in `applyParentWorkspacePolicyDefaults`, so the parent's
`default_child_workspace` is never consulted on this path, before or after this
change. Workflow-engine children (`create_child_task`) leave both new fields
empty and are unchanged.

`CreateTask` already does the rest:

- `validateBlockerReferences` re-checks existence and workspace before insert.
- `finalizeCreatedTask` adds each edge through `AddBlocker` →
  `AddDependency`, the single validator with self-edge, cross-workspace, and
  cycle checks under the shared mutation lock. A failure rolls the task back
  through `rollbackPartialTask` (`AC-…-001.6`, `AC-…-001.7`). A new task has no
  dependents, so it cannot close a cycle.
- `new_workspace` records `metadata.workspace.mode` and attaches no workspace
  group, so the child gets its own task environment and worktree at launch.
  `inherit_parent` attaches the parent group as today (`AC-…-003.1`,
  `AC-…-003.2`).

No Kanban deferred-launch intent is recorded: Office creates pass no
`DeferredLaunch`. When the created task first wakes is owned by Office
assignment wake admission (`REQ-OFFICE-SCHEDULER-002`,
`shared.IsAssignmentWakeEligible`), which holds the `task_assigned` wake while a
blocker is pending. This design only creates the edges.

The `backendapp` adapter maps `taskservice.ErrTaskReferenceNotFound` from that
pre-insert re-check (a blocker deleted after the runtime check) to
`ErrBlockerOutOfScope`, so the race returns `403` rather than `500`.

## Cycles and ancestor blockers

Blocker-only cycles cannot be created here, and the existing check stays in
place. Every edge goes through `AddDependency`, whose `checkDependencyCycle`
walks forward from the blocker and rejects any edge that would reach the
dependent (`AC-…-001.7`). On this path the dependent is the task being
created, and nothing depends on it yet, so the walk cannot find it. No extra
check is needed.

A parent or other ancestor named as a blocker is accepted (`AC-…-001.8`). It is
the sequential-pivot pattern: a step creates its successor, often as its own
child, blocked by itself, and the successor runs once the step completes.
Parent completion does not require its children to be terminal. The
`task_children_completed` wake is a convention the parent's agent may wait on,
not a precondition of reaching `done`. Rejecting ancestor blockers would break
that pattern.

The wait can stall only when the parent's agent also waits for
`task_children_completed` before it completes. That wait spans a blocker edge
and the parent/child hierarchy, which `checkDependencyCycle` does not model.
The skills document it as a pattern to avoid. Detecting it is out of scope.

## CLI surface

`apps/backend/cmd/agentctl/kandev_task.go` `taskCreate`:

- Removes the `--priority`, `--workspace-group-id`,
  `--default-child-workspace`, and `--default-child-ordering` definitions.
  The `flag` package then rejects them
  as unknown flags before any client is created (`AC-…-004.2`).
- Keeps `--blocked-by` and `--workspace-mode` (help: `inherit_parent or
  new_workspace`), and deletes the reject-before-send loop.
- Splits `--blocked-by` on commas and trims each entry. An empty entry fails
  with `--blocked-by contains an empty task ID` (`AC-…-004.4`). Non-empty lists
  are sent as `blocked_by` (a JSON array) and a non-empty mode as
  `workspace_mode` (`AC-…-004.1`). Server-side
  validation stays authoritative.

## Agent-facing documentation

Update the Office material agents read so the advertised surface matches the
endpoint (`AC-…-004.5`):

- `internal/office/configloader/skills/kandev-protocol/SKILL.md`, its
  command table, and `kandev-task-ops/references/tasks.md`: document the two
  flags and the blocker scope rule. Include the fan-in and `new_workspace`
  batch-child examples, and the parent-waits-for-children stall to avoid.
- `config/prompts/office-context.md` if it lists `task create` flags.
- [agents part 2](agents-02.md) CLI synopsis and the [agents part 3](agents-03.md)
  scenario that says the flags are rejected.
- `docs/public/automation-and-mcp.md` Office `task create` example, through
  `/docs-maintainer`.

`internal/office/skills/system_sync_test.go` and the sysprompt sync tests keep
the bundled skills in sync with the system skill store.

## Failure and recovery

- Validation and scope failures create nothing. A post-insert edge failure rolls
  back the task through the existing lifecycle-aware rollback.

## Security

- The workspace, the caller, and the run task come only from the signed token.
- A blocker must be within the run's task scope or its read relation, in one
  workspace, as described in [Blocker scope](#blocker-scope).
- Edges are only declared on the created task. The runtime offers no way to
  add an edge to an existing task, so an agent cannot hold back tasks it did
  not create.
- Errors carry a field index, never another task's ID or existence.

## Observability

- `runtime.action` / `runtime.denied` events on `create_task` cover success,
  malformed relations, and blocker scope denials.
- No new metric. Edge changes publish the existing `task.updated` dependency
  projection.

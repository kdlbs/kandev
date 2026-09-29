---
id: coordinator-copilot-tools-design
title: Coordinator tool surface design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-COPILOT-003
  - REQ-COORDINATOR-COPILOT-005
---

# Coordinator tool surface System Design

## Purpose and boundaries

The Kandev MCP tools a coordinator session gets, the backend guard that confines them, and the item read behind an Ask about this reference. Principal resolution, fail-closed starts and the permission policy stay in [copilot](copilot.md); the chip and the stored reference are in [copilot panel](copilot-panel.md#ask-about-this).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-003` | [Tool surface](#tool-surface) |
| `REQ-COORDINATOR-COPILOT-005` | [Item read](#item-read) (`AC-COORDINATOR-COPILOT-005.6`) |

## Tool surface

`registerCoordinatorTools` in `internal/mcp/server` registers exactly these
seven tools, the phase-1 tool profile, reusing the existing handlers
unchanged except for the new two:

| Tool | Why |
| --- | --- |
| `list_tasks_kandev` | positions of the workspace's tasks |
| `get_task_conversation_kandev` | what a task's agent last said or asked |
| `list_workflows_kandev` | target workflow of a proposal |
| `list_workflow_steps_kandev` | target step of a proposal |
| `list_repositories_kandev` | repository of a proposal |
| `get_coordinator_item_kandev` | new; the record behind an Ask about this reference: `{kind: "proposal", id}` returns the coordinator's own proposal row (spec, status, error, timestamps), `{kind: "stall", id}` the stall record of that task ([needs-you](needs-you.md#stall-records)) |
| `propose_task_kandev` | new; sends the `coordinator.propose_task` action |

It registers no other tool: in particular no `list_related_tasks_kandev`, no
`get_task_plan_kandev`, no plan,
document or session reads, and no user-question, title, plugin, create, move,
message, archive or delete tool. The backend guard in
`internal/mcp/handlers/coordinator_authorization.go` runs for every action from
a coordinator principal: an allowlist of action names (anything else is
refused with an error naming it), and a workspace check over four categories
of id: workspace, task, workflow and step, and repository. A `workspace_id`
argument must equal `principal.WorkspaceID`; `list_workflows_kandev` and
`list_repositories_kandev` take a client-supplied `workspace_id` as their only
scope (`internal/mcp/server/config_handlers.go`), so without this check they
would enumerate any workspace. Every task, workflow, step and repository id
must resolve inside the coordinator's workspace, and a proposal id must belong
to the calling coordinator. A call failing either check is
refused with an error naming the argument, before the handler runs, and
returns no data.
`coordinator.propose_task` and `coordinator.get_item` from a principal that
is not a coordinator are refused with the unknown-action error. The guard is
tested by a table over every registered MCP action, so a newly added action is
refused unless listed. Approve and reject are never on this surface; the guard
refuses their reserved names from a coordinator and from an unresolved
principal ([proposals](proposals.md#security)).

### Item read

`get_coordinator_item_kandev` sends the MCP action `coordinator.get_item`
(constant `ActionGetItem` beside `ActionProposeTask` in
`internal/coordinator/mcpcontract`), handled in `internal/mcp/handlers` by the
coordinator service. It is read-only: it never writes, never runs proposal
recovery and never starts or messages a session.

- **Arguments.** `{"kind": string, "id": string}`, both required. `kind` is
  `proposal` or `stall`; `id` is the `<ref>` of the bracketed reference (a
  proposal id for `proposal`, a task id for `stall`). The tool description
  says `task` references are read with `list_tasks_kandev` and
  `get_task_conversation_kandev`, not with this tool.
- **Validation, in this order, before any read.** A missing or non-string
  `kind`, or any value other than `proposal` or `stall` (including `task`
  and a different case such as `Proposal`), is refused with `BAD_REQUEST`
  naming `kind`. A missing, non-string or empty-after-trimming `id` is
  refused with `BAD_REQUEST` naming `id`. Unknown extra fields are ignored.
- **Guard.** The generic reference check (`workspace_id`, `workflow_id`,
  `task_id` fields) finds none of its fields in this payload and passes; the
  scope check is the handler's, keyed by `kind`, always from the principal and
  never from the arguments:
  - `proposal`: `Store.GetProposal(principal.WorkspaceID,
    principal.CoordinatorID, id)`. A proposal of another coordinator, another
    workspace, or none at all (never existed, or deleted with its
    coordinator) answers `NOT_FOUND` "target not found", the guard's foreign-id
    error, so a caller cannot tell a foreign id from a missing one
    (`AC-COORDINATOR-COPILOT-003.3`). A proposal in any status, settled
    included, is returned.
  - `stall`: the task is read through the task service; a task that is
    missing or whose `workspace_id` differs from `principal.WorkspaceID`
    answers `NOT_FOUND` "target not found". Otherwise the new store method
    `Store.GetStall(ctx, workspaceID, taskID)` reads the one
    `coordinator_stalls` row `WHERE task_id = ? AND workspace_id = ?`
    ([needs-you](needs-you.md#stall-records); the table holds at most one row
    per task, the latest episode). No row (the task never stalled, or its row
    was removed by the startup pass) answers `NOT_FOUND` "no stall record for
    this task", which is distinct from the foreign-id error because the task
    is already known to be in the caller's workspace.
- **Result.** For `proposal`: `{"kind":"proposal","proposal": <the same
  proposal object the proposal GET route returns (`ProposalDTO`: `id`,
  `coordinator_id`, `workspace_id`, `status`, `spec`, `final_spec`,
  `claimed_at`, `task_id`, `error`, `reject_reason`, `decided_by`,
  `created_at`, `updated_at`)>}`. For `stall`: `{"kind":"stall","stall": {"task_id",
  "workspace_id", "stalled_for_ms", "last_event_at", "detected_at"}}`, times
  as RFC 3339 UTC.
- **Errors.** A store or task-service read error answers `INTERNAL_ERROR`
  and is logged at warn with the kind, id and coordinator id.
- **Tests.** A table over kinds and ids: own proposal (any status), foreign
  coordinator's proposal, another workspace's proposal, missing proposal,
  own-workspace stalled task, own-workspace task with no stall row, foreign
  task, missing task, `task` kind, unknown kind, empty id; plus a
  non-coordinator principal refused with the unknown-action error.


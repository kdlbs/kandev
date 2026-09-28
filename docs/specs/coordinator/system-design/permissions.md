---
id: coordinator-permissions-design
title: Coordinator permissions and Watches design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-002
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
---

# Coordinator permissions and Watches System Design

## Purpose and boundaries

This design stores D17's per-action settings and the Watches scope on each
coordinator, derives the coordinator's tool profile from them, binds that
profile to the conversation, and makes the MCP guard and agentctl
auto-approval read the policy instead of the phase-1 constants
(ADR decisions D18, D19, D13 and D25). It owns the May do and Watches
sections of the coordinator page.

Proposal execution per kind is in [proposal kinds](proposal-kinds.md), the
refused rows in [activity log](activity-log.md), and the flag and Configure
page shell in [coordinators](coordinators.md#phase-2). Phase-1 behaviour of
the guard, registration and fail-closed checks is in
[copilot](copilot.md#tool-surface) and is extended, not replaced.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PERMISSIONS-001` | [Store](#store), [Policy value](#policy-value), [Settings routes](#settings-routes), [May do UI](#may-do-ui) |
| `REQ-COORDINATOR-PERMISSIONS-002` | [Tool profile](#tool-profile), [Binding](#binding), [Registration](#registration), [Guard](#guard), [Auto-approval](#auto-approval), [Approve re-check](#approve-re-check) |
| `REQ-COORDINATOR-PERMISSIONS-003` | [Store](#store), [Watch filter](#watch-filter), [Workflow deletion](#workflow-deletion), [Watches UI](#watches-ui) |
| `REQ-COORDINATOR-PERMISSIONS-004` | [Settings routes](#settings-routes), [Conversation reset](#conversation-reset) |

## Store

Additive changes to the coordinator store (`internal/coordinator/store.go`),
both dialects, with an upgrade test from the phase-1 schema:

`coordinators` gains:

| Column | Type | Notes |
| --- | --- | --- |
| `policy_json` | text null | NULL means the phase-1 policy |
| `policy_revision` | integer not null default 0 | increased once per save that changes policy or Watches |
| `watch_scope` | text not null default 'all' | `all` or `selected` |

New table `coordinator_watches`:

| Column | Type | Notes |
| --- | --- | --- |
| `coordinator_id` | text not null | part of the primary key |
| `workflow_id` | text not null | part of the primary key; indexed alone for workflow deletion |
| `workspace_id` | text not null | |
| `created_at` | timestamp not null | UTC |

Existing rows read as `policy_json` NULL and `watch_scope` `all`, so a
phase-1 coordinator keeps the phase-1 policy and watches everything
(`AC-COORDINATOR-PERMISSIONS-001.1`, `003.1`). Coordinator delete and
workspace deletion delete the coordinator's watch rows in their existing
transactions.

## Policy value

`internal/coordinator/policy.go`:

```go
type Setting string // "denied" | "requires_approval" | "automatic"
type Action string  // create_task, start_agent, message, move, resume, stop

type Policy struct {
    Version int                 `json:"version"` // 1
    Actions map[Action]Setting  `json:"actions"`
}

func PhaseOnePolicy() Policy        // create_task requires_approval, rest denied
func (p Policy) Allows(a Action) bool // setting != denied
func ParsePolicy(raw *string) (Policy, error) // NULL -> PhaseOnePolicy
```

`ParsePolicy` fills any action absent from a stored map with `denied`, so a
map written by an older build never grants more. A stored value that fails
to parse is treated as all `denied` and logged at error once per
coordinator; the settings GET returns it as all `denied`.

`Validate(p Policy) error` returns a field error naming the action when an
action is outside the six, a value is outside the three,
`stop != denied`, or any value is `automatic` (code
`automatic_not_available`, D13). The first failing action in the fixed
action order is the one named. Validation has no case for merging or for a
Done step: those are not actions (`AC-COORDINATOR-PERMISSIONS-001.4`), and
the move and create validators refuse a Done step
([proposal kinds](proposal-kinds.md#propose)).

## Settings routes

| Route | Scope | Result |
| --- | --- | --- |
| `GET .../coordinators/:cid/settings` | `workspace.read` | `{policy: {actions}, policy_revision, watches: {scope, workflow_ids}}` |
| `PUT .../coordinators/:cid/settings` | `workspace.manage` | the same shape after the save |

Both are under `/api/v1/workspaces/:id/` and registered only when the
phase-2 flag is on. The coordinator GET and list responses also carry
`policy`, `policy_revision` and `watches` while the flag is on.

PUT body: `{policy?: {actions: {...}}, watches?: {scope, workflow_ids}}`.
An absent member is unchanged. `policy.actions` must name all six actions.
`watches.workflow_ids` is ignored for `all` and required for `selected`.

The save runs in one transaction: take the per-coordinator lock of
[proposals](proposals.md#propose), read the row and watch rows, validate
(policy as above; Watches: `selected` with 1 to 50 unique workflow ids, each
a workflow of the coordinator's workspace read through the workflow
service), compare with the stored values, and when anything differs write
`policy_json`, `watch_scope`, the watch rows (delete and re-insert),
`policy_revision = policy_revision + 1` and `updated_at`, and calls
`resetConversation`. When nothing differs it writes nothing
(`AC-COORDINATOR-PERMISSIONS-001.2`). Comparison is on the normalised
values: the policy map, and for `selected` the workflow id set.

The lock serialises concurrent saves, so each change increases the revision
exactly once and the last committed save sets every member it sent
(`004.2`). After commit, the conversation reset runs and
`coordinator.updated` is published.

## Conversation reset

`resetConversation(tx, coordinatorID)` in `internal/coordinator/service.go`
clears `conversation_task_id` and increments the phase-1 `config_revision`
in the caller's transaction, and after commit archives the old conversation
task through the same path a context change uses
([coordinators](coordinators.md#routes)), including its
warn-and-startup-pass handling of an archive failure. Because the
conversation route's step-4 conditional update already requires the
`config_revision` it read ([copilot](copilot.md#conversation-task)), an open
racing a settings save deletes its task and returns 409 exactly as for a
context change (`004.1`). A settings save that changed something calls it;
standing-order and goal changes call it too
([standing orders](standing-orders.md#conversation-reset),
[goals](goals.md#routes)). `policy_revision` counts policy and Watches
changes only and is what the binding records.

## Tool profile

`internal/coordinator/toolprofile.go`:

```go
var readTools = []string{ // always
    "list_tasks_kandev", "get_task_conversation_kandev",
    "list_workflows_kandev", "list_workflow_steps_kandev",
    "list_repositories_kandev", "get_coordinator_item_kandev",
}
var proposeTool = map[Action]string{
    ActionCreateTask: "propose_task_kandev",
    ActionMessage:    "propose_message_kandev",
    ActionMove:       "propose_move_kandev",
    ActionResume:     "propose_resume_kandev",
}
func ToolNames(p Policy, phase2 bool) []string
```

With `phase2` false it returns the phase-1 seven tools. With `phase2` true it
returns the read tools, `list_coordinator_activity_kandev`, and each propose
tool whose action `Allows`, in that fixed order (`002.1`). `start_agent` and
`stop` map to no tool.

## Binding

The conversation route's create step (step 3 of
[copilot](copilot.md#conversation-task)) also stamps task metadata
`kandev.coordinator_tool_policy` with:

```go
type CoordinatorToolPolicy struct {
    Version            int      `json:"version"` // 1
    CoordinatorID      string   `json:"coordinator_id"`
    WorkspaceID        string   `json:"workspace_id"`
    ConversationTaskID string   `json:"conversation_task_id"`
    PolicyRevision     int      `json:"policy_revision"`
    ToolNames          []string `json:"tool_names"`
}
```

`ConversationTaskID` is filled after the task id exists, through the task
service's internal metadata update in the same route step before step 4. The
policy revision is the one read in step 1, so a save in between fails step
4's revision check and the task is deleted. `Validate` requires version 1,
non-empty ids, ids equal to the task's own coordinator, workspace and task,
and every tool name in the phase-2 tool universe, following
`ManagedToolPolicy.Validate` in `internal/mcp/profile/profile.go`.

**Transport, as the managed tool policy travels.** `mcpprofile.Context`
gains `CoordinatorToolPolicy *CoordinatorToolPolicy` (the type lives in
`internal/mcp/profile` with `MarshalCoordinatorToolPolicy` and
`ParseCoordinatorToolPolicyMetadata`, key `kandev.coordinator_tool_policy`).
The executor's `resolveTaskSessionMCPProfile` coordinator branch sets it from
the task metadata. `buildLaunchMetadata`
(`internal/agent/runtime/lifecycle/manager_launch.go`) deletes any
caller-supplied value of the key and re-sets it from the profile, exactly as
it does for `kandev.managed_tool_policy`, so launch metadata can never carry
a value the resolver did not produce. `StreamManager.mcpHandlerFor`
(`lifecycle/mcp_identity.go`) parses it into the execution's MCP handler
identity, marking it required when present, and the guard reads it from
there.

Reading the binding for a session, `BoundToolNames(task)`:

| Metadata | Result |
| --- | --- |
| absent | phase-1 seven tools (`002.3`, a conversation opened before phase 2) |
| present and valid | its `ToolNames` |
| present and invalid | error: refuse every action (`002.3`) |

With the phase-2 flag off the binding is ignored and the phase-1 tools are
used (`AC-COORDINATOR-COORDINATORS-007.4`).

## Registration

`registerCoordinatorTools` (`internal/mcp/server/coordinator_tools.go`)
takes the bound names from the session's MCP profile context, which the
executor's `resolveTaskSessionMCPProfile` coordinator branch fills from
`BoundToolNames`. It registers each read tool and each named propose tool.
An invalid binding fails the start in that branch, as the other fail-closed
checks do ([copilot](copilot.md#fail-closed)).

## Guard

`authorizeCoordinatorRequest` (`internal/mcp/handlers/coordinator_authorization.go`)
keeps the phase-1 allowlist and reference checks and adds, before the
handler runs:

1. Resolve the bound names from the execution's MCP handler identity with
   the table above. A parse error refuses.
2. The action's tool name must be in the bound names.
3. For a propose action, read the coordinator's stored policy (one indexed
   row read per call) and require `Allows(action)`.
4. For every id argument, the [watch filter](#watch-filter).

A refusal returns the phase-1 unknown-action error text, "tool is not
available on the coordinator MCP surface", whatever the reason, and records
a refused activity row through `activity.RecordRefusal(ctx, coordinatorID,
actionClass, reasonCode)` ([activity log](activity-log.md#refusals)). Reason
codes: `not_in_profile`, `policy_denied`, `binding_invalid`, `not_watched`,
`foreign_reference`. A refused-row write failure is logged at warn and does
not change the refusal. The coordinator's surface has no settings, Watches,
standing-order or goal action, so the allowlist refuses any such attempt
(`002.5`).

## Watch filter

`WatchSet` (`internal/coordinator/watches.go`) is loaded once per guard call:
`All bool` or a set of workflow ids.

| Call | Rule when not `All` |
| --- | --- |
| `list_workflows_kandev` | the handler's result is filtered to the set |
| `list_tasks_kandev`, `list_workflow_steps_kandev` | `workflow_id` outside the set: phase-1 not-found error |
| `get_task_conversation_kandev`, `get_coordinator_item_kandev` (task or stall) | task whose workflow is outside the set: not found |
| propose tools | target task, workflow or step outside the set: refused naming the field |

A task with no workflow (a conversation task, a Quick Chat) is never
watched. An empty `selected` set (after workflow deletion) watches nothing:
list reads return empty results (`003.5`).

Needs you, the Queue and the count strip apply the same set in the
classification input query of [needs-you](needs-you.md#inputs): tasks and
stall rows are joined to the watched workflow ids when not `All`; the
coordinator's own proposals are not filtered (`003.4`). The count the
sidebar badge shows is recomputed through the same query.

## Workflow deletion

A subscriber to the workflow-deleted event deletes `coordinator_watches`
rows with that `workflow_id` in one statement, then publishes
`coordinator.updated` for each affected coordinator. It never changes
`watch_scope`, so a `selected` coordinator with no rows watches nothing.
It does not bump `policy_revision` or archive the conversation: the guard
reads Watches live, so the narrowed scope applies to the next call. The
Configure and Needs you empty-scope notices read `watch_scope = selected`
with zero rows. A repeated event deletes nothing and succeeds.

## Approve re-check

The approve route of [proposals](proposals.md#approve) gains, in step 1
after the status decision and before the claim, a read of the stored
policy: when the proposal's action (its kind's action, and `start_agent`
too when `starts_agent` is true) is `denied`, it returns 409
`{"error":"policy_denied","action":...}` and writes nothing (`002.6`). Reject
has no such check. The card shows "Its May do settings no longer allow
this" with Reject only.
Approve does not re-check Watches: Watches bounds what the coordinator reads
and proposes, so a proposal made while its workflow was watched stays
approvable after the workflow leaves scope, and the kind's executor still
checks the target at approval.

## Auto-approval

The permission-policy rule of [copilot](copilot.md#permission-policy)
changes one input: the allowed names are the session's bound names instead
of the constant six. agentctl receives them with the coordinator mode as a
list in the instance configuration (`CoordinatorToolNames`), set by the
executor from the same resolver output. Comparison stays by full qualified
name, never by prefix (`002.4`). An instance built by the lifecycle alone
(workspace-only restore) receives an empty list, so it auto-approves
nothing; the agent start that follows goes through the resolvers.

## May do UI

`apps/web/app/settings/workspace/[id]/coordinators/sections/may-do.tsx`:

```text
May do
  Create a task     (o) Denied  (*) Requires approval  ( ) Automatic
                    Last 30 days: 12 approved, 2 rejected  Review the last 30 days
  Start an agent    (*) Denied  ( ) Requires approval  ( ) Automatic
  ...
  Stop a task       (*) Denied   Stopping is not available yet.
  Merge a pull request      Always human
  Move a task to Done       Always human
```

- Automatic is a disabled radio with the note of
  `AC-COORDINATOR-PERMISSIONS-001.6`.
- The per-row line reads the activity summary for 30 days
  ([activity log](activity-log.md#summary)); zero approved and zero
  rejected shows "Nothing yet". The link routes to the Queue What it did
  section with `?class=<action>`.
- When Start an agent is not Denied, the note of `001.9` shows under its
  row.
- Readers see the radios disabled.
- The section registers with `useSettingsSaveContributor`; the page note
  says saving starts the next conversation fresh (`004.3`).

## Watches UI

`sections/watches.tsx` shows the switch "Watch every board, including new
ones". Off lists the workspace's workflows from the workflows store in its
existing order, each with its state and **Put this board in scope** /
**Take this board out of scope**. Taking the last board out shows an inline
error and leaves it in scope (`003.6`). A coordinator watching nothing
shows the "watches no board" notice with a link to this section for
managers.

## Security

- Settings routes authorise with `workspace.manage` at the backend; a
  reader's PUT is 403 (`001.5`).
- The binding is written server-side at task creation. The task service's
  HTTP and MCP task create and update paths refuse metadata keys with the
  `kandev.coordinator_` prefix (400), and launch metadata strips the key and
  re-derives it from the resolved profile, so no agent or client can supply
  a binding.
- The guard reads the live policy on every propose call, so loosening never
  applies to a running conversation and tightening always does.

## Observability

Settings saves log at info with coordinator id, old and new revision and the
changed members. Guard refusals log at info with the reason code (never the
arguments).

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)

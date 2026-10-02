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
(ADR decisions D18, D19, D13 and D25). The May do and Watches sections of the
coordinator page are in [permissions UI](permissions-ui.md).

Proposal execution per kind is in [proposal kinds](proposal-kinds.md), the
refused rows in [activity log](activity-log.md), and the flag and Configure
page shell in [coordinators](coordinators.md#phase-2). Phase-1 behaviour of
the guard, registration and fail-closed checks is in
[copilot](copilot.md#tool-surface) and is extended, not replaced.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PERMISSIONS-001` | [Store](#store), [Policy value](#policy-value), [Settings routes](#settings-routes), [May do UI](permissions-ui.md#may-do-ui) |
| `REQ-COORDINATOR-PERMISSIONS-002` | [Tool profile](#tool-profile), [Binding](#binding), [Registration](#registration), [Guard](#guard), [Interleavings](#interleavings), [Auto-approval](#auto-approval), [Approve re-check](#approve-re-check) |
| `REQ-COORDINATOR-PERMISSIONS-003` | [Store](#store), [Watch filter](#watch-filter), [Workflow deletion](#workflow-deletion), [Client watch filter](permissions-ui.md#client-watch-filter), [Watches UI](permissions-ui.md#watches-ui) |
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
coordinator and revision; the settings GET returns it as all `denied`. The
full result table, the `Allows` behaviour on an unknown action, where the
log state lives and the `Service.Policy` return shape are in
[shared interface](shared-interface.md#shared-interface).

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
| `PUT .../coordinators/:cid/settings` | `workspace.manage` | 200 with the coordinator DTO after the save (a save that changes nothing returns the stored DTO); its `policy`, `policy_revision` and `watches` members have exactly the shape of the GET body (`001.2`) |

Both are under `/api/v1/workspaces/:id/` and registered only when the
phase-2 flag is on. The coordinator GET and list responses also carry
`policy`, `policy_revision` and `watches` while the flag is on.

PUT body: `{policy?: {actions: {...}}, watches?: {scope, workflow_ids}}`.
An absent member is unchanged. `policy.actions` must name all six actions.
`watches.workflow_ids` is ignored for `all` and required for `selected`.
The client sends a member only when its section changed, so a May do save
by a coordinator that watches nothing (an empty `selected` set left by a
workflow deletion) carries no `watches`. The server also validates
`watches` only when it differs from the stored value: a body whose
`watches` equals the stored Watches, the stored empty set included, is
treated as absent.

**Validation and errors.** A coordinator that is absent, or belongs to
another workspace, is 404 before anything else; a reader's PUT is 403
(`001.5`). A body that is not a JSON object is 400 with code `invalid_body`.
A JSON `null` member (`policy`, `watches`, `actions`, `workflow_ids`) is the
same as an absent one; `{}` changes nothing and returns 200. Every 400 has the phase-1 error body
`{"error": <message>, "field": <field>}` plus a `code` member from the closed
set below, and stores nothing. `field` is `policy.actions.<action>` for an
action error (so the response names the action, `001.3`) and `watches` for a
Watches error (`003.2`). Policy is validated before Watches; when both are
invalid the policy error is the one returned. Within the policy the first
failing action in the fixed action order is named, and a key outside the six
actions is named before any known action is checked.

| Code | Cause |
| --- | --- |
| `invalid_body` | body is not a JSON object, or `policy.actions` / `watches` has the wrong type |
| `unknown_action` | `actions` has a key outside the six |
| `action_missing` | one of the six is absent from `policy.actions` (`policy.actions` must name all six) |
| `invalid_setting` | a value outside the three settings, or `null` |
| `stop_denied_only` | `stop` is anything but `denied` |
| `automatic_not_available` | any action is `automatic` (`001.3`); for `stop` this code wins over `stop_denied_only` |
| `invalid_scope` | `watches.scope` is neither `all` nor `selected` |
| `watches_empty` | `selected` with no workflow, including `workflow_ids` absent or `null` |
| `watches_too_many` | more than 50 workflow ids |
| `watches_duplicate` | a repeated workflow id |
| `watches_foreign_workflow` | an id that is not an existing workflow of the coordinator's workspace and is not in the stored set (a stored id that no longer exists is dropped) |

Workflow existence is read through the task service's `GetWorkflow`
(a hidden workflow exists; one of another workspace is foreign) **before** the
lock, a reader-pool read as [shared interface](shared-interface.md#transactions)
requires; the locked transaction compares and writes only. A failed read is
500 with nothing stored. The body is first compared with the stored Watches as
sent (equal means absent). Otherwise a body id that is in the stored set and
no longer exists is dropped; any other id that is not an existing workflow of
the workspace is `watches_foreign_workflow`; a set left empty by dropping is
`watches_empty`.

**Body shapes.** Top-level members other than `policy` and `watches` are
ignored. A present `policy` without `actions`, or with `actions` null, is
`action_missing` naming the first action in the fixed order. A present
`watches` without `scope`, or with a null one, is `invalid_scope`.
`{scope:"selected"}` with `workflow_ids` absent or null equals a stored empty
`selected` set (no-op) and is `watches_empty` otherwise. The route checks
unknown and missing action keys itself, unknown keys first in sorted key order,
before it calls `Validate`.

The save runs in one transaction: take the per-coordinator lock of
[proposals](proposals.md#propose), read the row and watch rows, validate
(policy as above; Watches: `selected` with 1 to 50 unique workflow ids, each
a workflow of the coordinator's workspace read through the workflow
service, already read), compare with the stored values, and when anything differs write
`policy_json`, `watch_scope`, the watch rows (delete and re-insert, only when the Watches member differs),
`policy_revision = policy_revision + 1` and `updated_at`, and calls
`resetConversation`. When nothing differs it writes nothing
(`AC-COORDINATOR-PERMISSIONS-001.2`). Comparison is on the normalised
values: the policy map, and for `selected` the workflow id set.

The lock serialises concurrent saves, so each change increases the revision
exactly once and the last committed save sets every member it sent
(`004.2`). After commit, the conversation reset runs, then
`coordinator.updated` is published.

## Conversation reset

`resetConversation(ctx, exec, coordinatorID)` in `internal/coordinator/conversation.go`
(exact signature and after-commit archive in
[shared interface](shared-interface.md#shared-interface)) clears `conversation_task_id` and increments the phase-1 `config_revision`
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

`ToolNames` is data, not a registration list: the bound names are the
policy's output whether or not every tool has a handler in the running build.
`ActionForTool(name)` maps a propose tool to its action; the inverse map from
a guard-visible action to its tool name is `ToolForAction(action string)
(string, bool)` in the same file, a fixed table that starts with the phase-1
seven (`mcp.list_tasks` to `list_tasks_kandev`, `mcp.get_task_conversation`
to `get_task_conversation_kandev`, `mcp.list_workflows`,
`mcp.list_workflow_steps`, `mcp.list_repositories`,
`coordinator.propose_task` to `propose_task_kandev`, `coordinator.get_item`
to `get_coordinator_item_kandev`, using the `ws.Action*` and
`coordinator.Action*` constants). Task 03 adds the row for
`list_coordinator_activity_kandev` and task 04 the three other propose rows,
each with its handler.

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

The create step stamps the key with an empty `ConversationTaskID`, which
`Validate` refuses, and no session exists until the update below fills it, so
nothing resolves the empty value. `ConversationTaskID` is filled after the task id exists, through the task
service's internal metadata update in the same route step before step 4. An
error from that update fails the route step exactly as a failed task create
does: the just-created task is deleted, nothing is bound, and the route
returns the phase-1 create-failure result. The write of the key, at create
and in that update, is the only one the task service allows
([Security](#security)). The
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

An invalid binding on an existing conversation task is not repaired by the
conversation route: step 2 reuses the task, its session start fails closed in
the executor branch, and the manager sees the phase-1 start-failure result.
Any settings save, standing-order or goal change, or context change replaces
the conversation (`resetConversation`) and so the binding; archiving the
conversation does the same. `mcpprofile` `sameProfile`
(`internal/mcp/server/server.go`) compares `CoordinatorToolPolicy` by its
marshalled JSON, so a re-resolved profile whose binding changed is not
treated as the same profile.
With the phase-2 flag off the binding is ignored and the phase-1 tools are
used (`AC-COORDINATOR-COORDINATORS-007.4`).

## Registration

`registerCoordinatorTools` (`internal/mcp/server/coordinator_tools.go`)
takes the bound names from the session's MCP profile context, which the
executor's `resolveTaskSessionMCPProfile` coordinator branch fills from
`BoundToolNames`. It registers each bound tool that has a handler in the running build,
looked up in the coordinator tool catalog (`ToolForAction`'s table plus its
handler registration); a bound name with no handler yet is skipped, not
stubbed, and is still in the bound list and the auto-approved names. In the
wave of task 02 the catalog holds the phase-1 seven; task 03 and task 04
each add their tools' rows and handlers, and task 04 owns the end-state test
that registered equals bound. The invariant this task tests is therefore:
registered tools = bound names that have a catalog handler; auto-approved
names = bound names; bound names = `BoundToolNames` at open (equal to
`ToolNames(policy, phase2)` of the policy as read at open, or the phase-1
seven for a conversation without a binding). A tool that is not
registered is unknown to the session's MCP server: the agent's call fails
there, before any backend action, so it has no refused row (`002.2`). The
guard's `not_in_profile` covers the calls that do reach the backend: a
direct request to the coordinator MCP endpoint with an action outside the
bound names. The refusal a running session can meet is
`policy_denied`: a propose tool registered at open whose action a manager
has since set to `denied`, called before the save's conversation reset has
archived that conversation.

## Guard

`authorizeCoordinatorRequest` (`internal/mcp/handlers/coordinator_authorization.go`)
is the one boundary for every request a coordinator principal makes to the
backend: the WebSocket or MCP action dispatch, including a request sent
directly to it without going through the agent's MCP client (this is what
"the coordinator MCP endpoint" means in this design). The guard runs, in this
order, for a coordinator principal with the phase-2 flag on:

0. Phase-1 preliminaries, unchanged and writing no row: a reserved decision
   action is refused with the phase-1 unknown-action error, and a
   principal-only action from a non-coordinator principal is refused.
1. Resolve the bound names from the execution's MCP handler identity with
   the table above. A parse or validation error refuses.
2. Map the action to its tool name with `ToolForAction`; the tool name must
   be in the bound names. An action with no tool name, including one outside
   the phase-1 allowlist and every settings-shaped action, fails here.
3. For a propose action, read the coordinator's stored policy through
   `Service.Policy` and require
   `Allows(action)`.
4. Parse the payload (an invalid payload keeps the phase-1 400, no row), run
   the phase-1 reference checks, then for every id argument the
   [watch filter](#watch-filter).

With the flag off, steps 1 to 3 are skipped and the guard is the phase-1
allowlist and reference checks (`AC-COORDINATOR-COORDINATORS-007.4`).

The coordinator and workspace ids for every refused row and for the policy
read come from the principal (`principal.CoordinatorID`,
`principal.WorkspaceID`), never from the binding, so a `binding_invalid` row
is written even when the binding cannot be parsed.

Each check has one result shape:

| Check | Response | Activity row |
| --- | --- | --- |
| 0 | phase-1 unknown-action error | none |
| 1, binding unparsable or invalid | phase-1 unknown-action error | `refused`, reason `binding_invalid`, class of the called action (`unknown` when the action maps to no propose tool) |
| 2, not in the bound names | phase-1 unknown-action error | `refused`, reason `not_in_profile`, class of the called action (`unknown` for a name that is no action, and for every read tool) |
| 3, stored policy `denied` | phase-1 unknown-action error | `refused`, reason `policy_denied`, class of the propose action |
| 3, policy or watch read fails | the phase-1 not-found error, logged at error | none |
| 4, invalid payload | the phase-1 400 | none |
| 4, read of an unwatched id | the phase-1 not-found error, identical to an absent id | none |
| 4, watch set read fails | the phase-1 not-found error, logged at error | none |
| 4, propose target unwatched | the propose tool's validation error naming the field | none |
| phase-1 reference checks | unchanged phase-1 result | none |

The phase-1 unknown-action error is "tool is not available on the
coordinator MCP surface". Rows go through `Service.RecordRefusal(ctx, coordinatorID, workspaceID,
actionClass, reasonCode)` ([activity log](activity-log.md#refusals)).
Only checks 1 to 3 write a row (`002.2`); the Watches
filter narrows what exists for the coordinator, as a missing id does, so
it writes no row, which also keeps a chip's unwatched id
([copilot everywhere](copilot-everywhere.md#server-side)) out of the log. A
read tool is never a class: reads are refused only by checks 1 and 2, whose
rows carry `unknown` because no action class names a read. A refused-row
write failure is logged at warn and does not change the refusal. The coordinator's surface has no settings, Watches,
standing-order or goal action, so step 2 refuses any such attempt
(`002.5`).

## Interleavings

The guard's policy read, the approve re-check's policy read and the
conversation route's revision read are point-in-time reads on the reader
pool, outside the per-coordinator lock; the propose insert and the approve
claim do not re-check policy. The settings save commits under the lock. The
result of each interleaving of a save S with an operation is fixed here and
is the table the work order's tests assert:

| # | Order | Result |
| --- | --- | --- |
| 1 | S tightens an action and commits before the guard's policy read | check 3 refuses with reason `policy_denied`; one `refused` row; no proposal |
| 2 | Guard's policy read (allowed) commits before S; S tightens; the propose insert follows | the call completes: a `pending` proposal exists and the `proposed` row is written. S has already reset the conversation, so the session ends. The proposal cannot execute: approve is refused 409 `policy_denied` while the setting stays `denied`, and Reject succeeds. No refused row (the call passed the guard). Archiving the session does not cancel a call the guard already authorized |
| 3 | S loosens an action (for example `message` from `denied`) while a conversation is running | the running conversation's bound names are unchanged, so the tool is not registered and a direct call fails check 2 (`not_in_profile`); loosening applies only to the next conversation, which S's reset makes the next open |
| 4 | Guard reads `denied`, S then loosens, in that order | the call is refused (`policy_denied` row) and stays refused; no retry is implied |
| 5 | Approve's re-check reads the setting as allowed, then S tightens it before the claim | the approval proceeds and executes: it was decided against the settings in force when the manager pressed approve. The next propose or approve sees the tightened setting |
| 6 | Approve's re-check reads after S committed the tightening | 409 `policy_denied`, no claim, no write; Reject succeeds |
| 7 | The conversation route reads `config_revision` r, S commits (`resetConversation` bumps it), the route then runs its conditional update | the update matches no row; the route deletes the new task and returns 409, as for a context change (`004.1`) |
| 8 | The route's conditional update commits, then S commits | the route returns 200; S's reset then clears `conversation_task_id`, increments `config_revision` and archives the just-opened conversation; the binding it carries is stale and never used again |
| 9 | Two managers' saves S1 and S2 | serialised by the lock; each save that changes something raises `policy_revision` by one; the last commit sets every member it sent (`004.2`); a save equal to the stored value at its commit is a no-op |
| 10a | W is deleted before S's workflow read | S's body with W: `watches_foreign_workflow` 400, nothing stored; if W was in the stored set it is dropped and the save is 200 |
| 10b | W is deleted after S's workflow read, S commits | 200; W's row is stored, `policy_revision` +1, `watch_scope` `selected`; the effective set omits W from then on and the subscriber, if it runs after S, removes the row |

## Watch filter

`WatchSet` (`internal/coordinator/reads_phase2.go`) is loaded once per guard call
(shape, order and error behaviour in
[shared interface](shared-interface.md#shared-interface)): `All bool` or a sorted set
of workflow ids.

| Call | Rule when not `All` |
| --- | --- |
| `list_workflows_kandev` | the handler's result is filtered to the set |
| `list_tasks_kandev`, `list_workflow_steps_kandev` | `workflow_id` outside the set: phase-1 not-found error |
| `get_task_conversation_kandev`, `get_coordinator_item_kandev` (task or stall) | task whose workflow is outside the set: not found |
| propose tools | target task, workflow or step outside the set: the tool's validation error naming the field, no activity row ([Guard](#guard)) |

A task with no workflow (a conversation task, a Quick Chat) is never
watched. The **effective watch set** is the stored ids that name an existing
workflow (hidden included). Every read of Watches applies it: the settings
GET and PUT DTO list only effective ids (`GetWorkflow` per id, at most 50), and
a stale id matches no task, so the guard needs no check. A `selected` scope
with an empty effective set watches nothing: list reads return empty results
and the "watches no board" state shows (`003.5`).

Needs you, Queue and the count strip apply the same set in the client, as
[permissions UI](permissions-ui.md#client-watch-filter) specifies (`003.4`,
`003.7`).

## Workflow deletion

The task service publishes `workflow.deleted` after the deletion has
committed. A subscriber, best-effort tidying only, runs in one transaction on
the writer pool `SELECT DISTINCT coordinator_id FROM coordinator_watches WHERE
workflow_id = ? ORDER BY coordinator_id`, `DELETE FROM coordinator_watches
WHERE workflow_id = ?`, and after commit publishes `coordinator.updated` once
per selected coordinator. It never changes `watch_scope`, `policy_revision` or
the conversation. A repeat or a workflow nobody watches deletes and publishes
nothing. A subscriber error is logged at error and not retried: the effective
set already omits the deleted workflow, so nothing visible depends on it. It
is registered with the phase-2 flag on.

## Approve re-check

Task 02 implements this check in the phase-1 approve route of
[proposals](proposals.md#approve); task 04 adds the kind-specific tests
(`starts_agent`, move no-op precedence). It runs only while the phase-2 flag
is on; with the flag off the route is phase 1 unchanged
(`AC-COORDINATOR-COORDINATORS-007.2`).

It sits in step 1 after the status decision and before the claim, and only
on the paths that take a **new** claim: a `pending` row, and a `failed` row
with no task found by its external id. It does not run on the stale
re-claim (`approving` with a stale claim and no edits) or on `failed` with
the task found, because those complete an approval a manager already gave and
do not begin one; the policy in force when that approval was first claimed
governs. The check reads the stored policy: when the proposal's action (its
kind's action, and `start_agent` too when `starts_agent` is true) is
`denied`, it returns 409
`{"error":"policy_denied","action":...}` and writes nothing (`002.6`).
`action` names one action: the kind's action (`create_task`, `resume`,
`message` or `move`) when it is `denied`, otherwise `start_agent`; so when
both are `denied` it is the kind's action. If the policy read fails the route
returns 500 and writes nothing. Reject
has no such check. The card shows "Its May do settings no longer allow
this" with Reject only. The re-check reads only the stored proposal and the
policy, never the target, so it runs before any kind's `Execute` checks:
a move proposal with `starts_agent` true is refused while `start_agent` is
`denied` even when its task already sits on the destination and the move
would have been a no-op ([proposal kinds](proposal-kinds.md#approve));
`AC-COORDINATOR-PROPOSAL-KINDS-002.3` takes precedence over the no-op of
`003.3`.
Approve does not re-check Watches: Watches bounds what the coordinator reads
and proposes, so a proposal made while its workflow was watched stays
approvable after the workflow leaves scope, and the kind's executor still
checks the target at approval. A save racing an approve is row 5 and row 6
of [Interleavings](#interleavings).

## Auto-approval

The permission-policy rule of [copilot](copilot.md#permission-policy)
changes one input: the allowed names are the session's bound names instead
of the constant seven. agentctl receives them with the coordinator mode as a
list in the instance configuration (`CoordinatorToolNames`), set by the
executor from the same resolver output. Comparison stays by full qualified
name, never by prefix (`002.4`). An instance built by the lifecycle alone
(workspace-only restore) receives an empty list, so it auto-approves
nothing; the agent start that follows goes through the resolvers.

## Security

- Settings routes authorise with `workspace.manage` at the backend; a
  reader's PUT is 403 (`001.5`).
- The binding is written server-side at task creation. The prefix is
  enforced in the task service, so every entry point inherits it, as for the
  [reserved external id prefix](proposals.md#reserved-prefix): `CreateTask`
  and `UpdateTask` refuse any request whose metadata has a key starting
  `kandev.coordinator_` with a reserved-metadata error (400 at the HTTP and
  MCP handlers and an error response on the WebSocket `task.create` and
  `task.update` actions), unless the request carries
  `AllowReservedMetadata bool` tagged `json:"-"`, which only the coordinator
  service sets. An update whose metadata lacks a `kandev.coordinator_` key keeps its
  stored value (restored after `protectedTaskMetadataUpdate` replaces the map,
  as for `MetaKeyHandoffs`). Launch metadata additionally strips the key and re-derives it
  from the resolved profile, so no agent or client can supply a binding
  through any path. Task-service tests cover HTTP, MCP and WebSocket create
  and update with the prefix (refused), the same requests with the flag
  (accepted), and an update without the key (binding unchanged).
- The guard reads the live policy on every propose call, so loosening never
  applies to a running conversation and tightening always does.

## Observability

Settings saves log at info with coordinator id, old and new revision and the
changed members. Guard refusals log at info with the reason code (never the
arguments).

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)

---
id: coordinator-copilot-design
title: Coordinator copilot and tool surface design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
requirements:
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-003
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
---

# Coordinator copilot and tool surface System Design

## Purpose and boundaries

The copilot is an ordinary Kandev session on an ephemeral task whose origin is
`coordinator`. This design adds one task origin, one MCP surface, one mcpmode,
one authorization guard and one popover shell. It reuses the session
lifecycle, the agentctl MCP server, the permission UI and the Quick Chat
session view unchanged in behaviour for every other origin.

Kandev controls which Kandev MCP tools a session has and which permission
requests Kandev auto-approves. It does not control the agent CLI's own tools;
see [Residual](#residual-external-surface).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-001` | [Conversation task](#conversation-task), [Standing instructions](#standing-instructions) |
| `REQ-COORDINATOR-COPILOT-002` | [Attended only](#attended-only) |
| `REQ-COORDINATOR-COPILOT-003` | [Principal and mode](#principal-and-mode), [Tool surface](#tool-surface), [Fail closed](#fail-closed), [Permission policy](#permission-policy) |
| `REQ-COORDINATOR-COPILOT-004` | [Popover](#popover) |
| `REQ-COORDINATOR-COPILOT-005` | [Ask about this](#ask-about-this) |

## Conversation task

- `internal/task/models` adds `TaskOriginCoordinator = "coordinator"` and
  `SurfaceCoordinator`. The HTTP create handler and the MCP `create_task`
  handler reject a request that names this origin (400), so only the
  coordinator service creates such tasks, through the task service's internal
  create call.
- `POST /api/v1/workspaces/:id/coordinators/:cid/conversation`
  (`workspace.manage`) returns `{task_id, session_id, archive_state}`:
  1. Load the coordinator (404) and check both profiles with `profileStatus`
     (409 with the `coordinator_profile_unavailable` body of
     [coordinators](coordinators.md#validation) when the agent profile is
     `missing` or `passthrough`, or the executor profile is `missing`).
  2. When `conversation_task_id` names a live, unarchived task, ensure its
     session (step 6) and return it.
  3. Otherwise create an ephemeral task through the task service's internal
     create call: origin `coordinator`, title `Coordinator: <name>`, the
     coordinator's agent and executor profiles, metadata
     `coordinator_id = <cid>` (`models.MetaKeyCoordinatorID`), no
     `start_agent`, no `prepare_session`, no external id and no
     `auto_start_on_create` marker, so `handleTaskCreated` starts nothing.
  4. Run `UPDATE coordinators SET conversation_task_id = ? WHERE id = ? AND
     (conversation_task_id IS NULL OR conversation_task_id = ?)` with the stale
     value read in step 2. One row updated: go to step 6 with the new task.
     Zero rows: re-read the coordinator row, delete the task just created
     through the task service and go to step 6 with the row's current task,
     so racing opens converge on one task with one session.
  5. When the re-read in step 4 finds no coordinator row (the coordinator was
     deleted between steps 1 and 4), the route deletes the task it created and
     returns 404. When a delete in step 4 or 5 fails, the route still answers
     and logs the task id at warn; the task carries `coordinator_id`, never
     had a session, and the [startup pass](#conversation-cleanup) removes it.
  6. Ensure the session with the orchestrator's
     `EnsureSession(ctx, taskID, EnsureSessionOptions{AutoStart: &false,
     ActivationSource: LaunchActivationSourceSessionOpen})`. It is serialised
     per task, returns the existing primary session when there is one (a
     passive open never resumes it), and otherwise creates a `CREATED`
     session through `IntentPrepare` with `NoAgentLaunch`, which never starts
     an agent. Its error returns 502 with the task kept as current, so the
     next open retries only this step. The route returns the session id with
     the task's archive state (always `false` from this route).
- `IsRestorableQuickChatTask` excludes origin `coordinator`, so the task never
  becomes a Quick Chat tab; board, list and snapshot queries already exclude
  ephemeral tasks.
- Quick Chat idle expiry excludes origin `coordinator`:
  `ListExpiredQuickChatTasks` and the re-check in
  `DeleteExpiredQuickChatTask` (`internal/task/repository/sqlite/task.go`)
  gain `AND t.origin <> 'coordinator'` beside the existing `automation_run`
  exclusion, so a conversation idle for more than seven days is kept.

### Conversation lifecycle

| Event | Current conversation task | Earlier conversation tasks |
| --- | --- | --- |
| Context change ([coordinators](coordinators.md#routes)) | archived through the task service's `ArchiveTask`, which stops a running turn; the reference is cleared | unchanged (archived) |
| Coordinator deletion | deleted through the task service, which stops a running turn | deleted |
| Workspace deletion | deleted with the workspace's tasks | deleted with the workspace's tasks |

An archived conversation task is never returned by the route, never resolves
to a coordinator (see [Principal and mode](#principal-and-mode)) and is never
listed. The popover of an archived conversation shows the missing-conversation
state and a new open creates the next task.

### Conversation cleanup

The task repository gains `ListCoordinatorOriginTasks(ctx, workspaceID)`
returning `{id, workspace_id, archived, created_at, coordinator_id}` for every task with
origin `coordinator` (all workspaces when `workspaceID` is empty);
`coordinator_id` is read from the metadata in Go, so the query has no
dialect-specific JSON. Coordinator deletion uses it for one workspace and
deletes every row whose `coordinator_id` matches. The startup pass uses it for
all workspaces, in one ordered walk by `id`.

The pass records its start time `T0` before the coordinator routes are
registered (see [coordinators](coordinators.md#flag-and-wiring)), so every task
the conversation route creates in this process has `created_at >= T0`. The
walk considers only tasks with `created_at < T0`; a task created at or after
`T0` is left alone, whatever its state, so the pass never touches a task an
open is creating (between steps 3 and 4 above), even though the routes may
serve while the pass runs. For each considered task:

- a task whose `coordinator_id` names no coordinator row is deleted;
- an unarchived task that is not its coordinator's `conversation_task_id`
  (a failed archive after a context change, or a losing race task whose
  delete failed) is archived;
- every other task is left alone.

Each step goes through the task service. A task already deleted
(`taskrepo.ErrTaskNotFound`) or already archived (`ErrTaskAlreadyArchived`), for
example by a concurrent coordinator delete or context change, counts as done.
Any other failure is logged at warn and the walk continues, so the next startup
retries it. Running the pass twice changes nothing the second time.

## Standing instructions

The first prompt of each session carries a system block built by
`internal/coordinator/prompt.go`: the coordinator's job (explain what needs the
manager and why; propose tasks), the workspace name and id, the context text
between explicit delimiters marked as operator-provided, and the rule that its
only write is `propose_task_kandev`, decided by a person. The block is attached
through the existing system-prompt path used for task sessions, not by
editing the stored user message.

## Attended only

- `autoResumeEligibility` returns `coordinator_message_only` for a coordinator
  task, so startup recovery, session restore and reconnect never resume it;
  the session lands idle with its transcript.
- The conversation route creates the task and prepares its session without an
  agent (step 6 above); it never starts an agent or sends a message. The
  created task carries no auto-start marker, so the workflow's create-time
  on_enter evaluation does not run for it. The stall subscriber,
  workspace-deletion subscriber, proposal service and recovery pass never call
  the session or message services; the startup pass only archives and deletes
  tasks.
- `message.add` for a coordinator task requires `workspace.manage`; a reader's
  message is refused before any session action.
- The popover passes `automaticRecovery={false}` to the Quick Chat session
  view, which forwards it to `useSessionResumption` as the new option
  `skipAutomaticRecovery`. With it set, the hook sends no check, resume or
  restore request on mount, on reload or on reconnect; it still reads the
  session from the store, so a turn that kept running while the page reloaded
  shows as running and the launcher shows busy (`AC-COORDINATOR-COPILOT-002.4`).
  The Retry action of the recovery feedback stays a manual action; it restores
  the execution and sends no message, so it starts no turn.

## Principal and mode

- `scope.Resolver` gains `principalSurface`, returning `SurfaceCoordinator` when
  the session's task has origin `coordinator`, and a `CoordinatorLookup`
  interface (implemented by the coordinator service) mapping the task to its
  coordinator: the task must equal a coordinator's current
  `conversation_task_id`. An orphaned or archived conversation task resolves to
  no coordinator and is refused.
- `mcpmode` adds `Coordinator`. The task session's mode is resolved in
  `Executor.resolveTaskSessionMCPMode`
  (`internal/orchestrator/executor/executor_execute.go`), which gains a branch
  returning the coordinator mode when `task.Origin ==
  models.TaskOriginCoordinator`, placed before the automation and office
  branches; the sibling `resolveTaskSessionMCPProfile` gains the matching
  branch returning a coordinator `mcpprofile.Context` with no user-question,
  title or canvas capability. Quick Chat code does not set the mode. agentctl
  `normalizeMode`, `surfaceForMode`, `modeForProfile`, `SetMode` and the
  `Legacy` mapping gain the coordinator case; plugin tool registration is
  skipped in this mode.

## Tool surface

`registerCoordinatorTools` in `internal/mcp/server` registers exactly these
seven tools, reusing the existing handlers of the first six:

| Tool | Why |
| --- | --- |
| `list_tasks_kandev` | positions of the workspace's tasks |
| `list_related_tasks_kandev` | parent, children and blockers of a stalled or waiting task |
| `get_task_conversation_kandev` | what a task's agent last said or asked |
| `list_workflows_kandev` | target workflow of a proposal |
| `list_workflow_steps_kandev` | target step of a proposal |
| `list_repositories_kandev` | repository of a proposal |
| `propose_task_kandev` | new; sends the `coordinator.propose_task` action |

It registers no other tool: in particular no `get_task_plan_kandev`, no plan,
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
must resolve inside the coordinator's workspace. A call failing either check is
refused with an error naming the argument, before the handler runs, and
returns no data.
`coordinator.propose_task` from a principal that is not a coordinator is
refused. The guard is tested by a table over every registered MCP action, so a
newly added action is refused unless listed.

## Fail closed

Before a coordinator session starts or resumes, the lifecycle checks: flag on,
coordinator resolvable, task readable, agent profile present and not
passthrough, executor profile present,
mode set to `Coordinator`. Any failure stops the start with an error surfaced
through the session recovery feedback. No branch falls back to the default
task mode. The popover follows the profile statuses of
[coordinators](coordinators.md#validation): when the coordinator GET reports
`agent_profile_status` or `executor_profile_status` other than `ok`, it does
not call the conversation route and shows the matching messages in place of
the composer, stacked, the agent profile message first. A 409 from the route
(a profile deleted or switched to passthrough since the GET) carries both
statuses in its `coordinator_profile_unavailable` body, and the popover shows
the messages built from them.

## Permission policy

- In the coordinator mode `AutoApprovePermissionsOverride=false` is applied on
  first launch, on a re-created or resumed execution and when a workspace-only
  execution is promoted, so the profile flag and the agentctl auto-approve
  environment variable are ignored.
- Kandev auto-approves a permission request only when the request's tool
  name parses with `ParseQualifiedMCPToolName`
  (`internal/agentctl/types/permission_identity.go`, the
  `mcp__<server>__<tool>` form ACP clients send, for example
  `mcp__kandev__list_tasks_kandev`) to server `kandev` and a tool that is one
  of the seven names above, each compared as the full string, never by prefix.
  A name that does not parse is not auto-approved.
- Every other request reaches the popover through the existing permission
  message flow with Approve and Deny.

## Residual external surface

The agent CLI keeps its own tools and settings: a shell on its executor, its
own MCP servers and its own permission configuration. Kandev does not register
or proxy them, so the guard above does not see them. Phase 1 is attended, so a
person is present for each turn and sees permission requests the CLI raises.
Enforced containment of these tools is a gate G4 condition before any
unattended turn. The [ADR](../../../decisions/2026-09-26-workspace-coordinator.md)
records this risk.

## Popover

- `ChatPopoverShell` is extracted from `ConfigChatPanel` (position, size,
  header, close, Escape handling, focus return). `ConfigChatPanel` keeps its
  Expand and behaviour; a snapshot test pins it.
- `CoordinatorCopilot` renders the shell at 420 by 550 pixels, bottom right,
  title `Coordinator: <name>`, no Expand, full width below 640px. The launcher
  renders only when the user holds `workspace.manage`; its busy state reads the
  conversation session's state from the session store.
- The body is `QuickChatSessionView` with new optional props, all defaulting
  to today's behaviour: `automaticRecovery` (default `true`; see
  [Attended only](#attended-only)), `hideSessionSelectors` (default `false`;
  hides the mode and model selectors), `taskArchiveState` (when given, it is
  used instead of `resolveTaskArchiveState`, whose fallback cannot see an
  ephemeral task outside the Quick Chat store), `initialDraft` and
  `transformOutgoing`. The popover builds the `QuickChatSession` value it
  passes from the route's response with `kind: "chat"`;
  `QuickChatSessionKind` (`"chat" | "config"`) is not widened, so the Quick
  Chat tab list, selection and `serverIdsByKind` types are untouched. The
  popover passes the route's `archive_state` as `taskArchiveState`; without
  the prop the view behaves as today.
- The empty state shows the intro text and one suggestion that fills the
  composer.
- On wide screens the popover is placed so it leaves the item column's action
  area uncovered at 1200px; a Playwright check asserts no overlap.

## Ask about this

- A small copilot store holds `{open, chip: {id, label} | null, draft}`.
  **Ask about this** sets the chip and the draft `Why is <id> here?`, opens the
  popover and focuses the composer; a second call replaces both.
- `transformOutgoing` prefixes `About <id>: ` while the chip is set; the hint
  under the composer shows the stored form.
- `user-message-body.tsx` gains a coordinator branch: when the task origin is
  `coordinator` and the text starts with `About `, up to the first `: `, it
  renders the remainder plus an `about <id>` tag. The stored text is unchanged.

## Security

- Authority comes from the task origin plus the coordinator lookup, both set
  server-side; the agent cannot claim it.
- Context text is operator-provided and delimited; it cannot change the
  registered tools or the guard.
- Proposal inputs are validated by the proposal service
  ([proposals](proposals.md)); the guard adds the workspace check.

## Observability

Refused coordinator actions log at warn with the action name, coordinator id
and workspace id. Fail-closed starts log at error with the failed condition.

## Related decisions

- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
- [Generic plugin host boundary](../../../decisions/2026-08-31-generic-plugin-host-boundary.md)

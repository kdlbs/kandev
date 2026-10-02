---
id: coordinator-copilot-design
title: Coordinator copilot and tool surface design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-003
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
  - REQ-COORDINATOR-COPILOT-006
---

# Coordinator copilot and tool surface System Design

## Purpose and boundaries

Panel: [copilot panel](copilot-panel.md).

The copilot is an ordinary Kandev session on an ephemeral task whose origin is
`coordinator`. This design adds one task origin, one MCP surface, one mcpmode,
one authorization guard and one right-side panel shared with the board
preview. Every other origin keeps its session lifecycle, MCP server,
permission UI and Quick Chat view.

Kandev controls the session's Kandev tools and auto-approvals, not the agent
CLI's own tools ([Residual](#residual-external-surface)).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-001` | [Conversation task](#conversation-task), [Standing instructions](#standing-instructions) |
| `REQ-COORDINATOR-COPILOT-002` | [Attended only](#attended-only) |
| `REQ-COORDINATOR-COPILOT-003` | [Principal and mode](#principal-and-mode), [Tool surface](#tool-surface), [Fail closed](#fail-closed), [Permission policy](#permission-policy) |
| `REQ-COORDINATOR-COPILOT-004` | [Copilot panel](copilot-panel.md#panel) |
| `REQ-COORDINATOR-COPILOT-005` | [Copilot panel](copilot-panel.md#ask-about-this) |
| `REQ-COORDINATOR-COPILOT-006` | [Copilot panel](copilot-panel.md#activity-display) |

## Conversation task

- `internal/task/models` adds `TaskOriginCoordinator = "coordinator"`.
  `internal/mcp/profile` adds `SurfaceCoordinator` to its `Surface` consts.
  The HTTP create handler and the MCP `create_task` handler reject a request
  that names this origin (400), so only the coordinator service creates such
  tasks, through the task service's internal create call.
- `POST /api/v1/workspaces/:id/coordinators/:cid/conversation`
  (`workspace.manage`) returns `{task_id, session_id, archive_state}`:
  1. Load the coordinator (404), keeping its `config_revision`
     ([coordinators](coordinators.md#store)), and check both profiles with `profileStatus`
     (409 with the `coordinator_profile_unavailable` body of
     [coordinators](coordinators.md#validation) when the agent profile is
     `missing` or `passthrough`, or the executor profile is `missing`).
  2. When `conversation_task_id` names a live, unarchived task, ensure its
     session (step 6). When the ensured session's state is not terminal,
     continue at step 7 with that task and return it. When it is `FAILED`,
     `CANCELLED` or `COMPLETED` (`AC-COORDINATOR-COPILOT-001.10`), a
     terminal session rejects every message, so the task is not reusable:
     archive it through the task service's `ArchiveTask` and continue at
     step 3, keeping this task's id as the stale value for step 4. A failed
     archive (including `ErrTaskAlreadyArchived` from a racing open that
     archived it first) is logged at warn with the coordinator id, task id
     and session state, and the route still continues at step 3; step 4's
     conditional update replaces the reference either way, and a task left
     unarchived is archived by the [startup pass](#conversation-cleanup)
     because it is no longer its coordinator's `conversation_task_id`. An
     `EnsureSession` error on this path is handled exactly as step 6's error.
     Two opens racing over the same ended task both reach step 4 with the
     same stale value, so exactly one new task wins and the other open
     deletes its own and converges on it. Terminal is the closed set of
     those three `TaskSessionState` values; `CREATED`, `STARTING`,
     `RUNNING`, `IDLE` and `WAITING_FOR_INPUT` are reusable.
  3. Otherwise create an ephemeral task through the task service's internal
     create call: origin `coordinator`, `IsEphemeral`, title
     `Coordinator: <name>`, no workflow and no workflow step, metadata
     `coordinator_id = <cid>` (`models.MetaKeyCoordinatorID`),
     `agent_profile_id = <coordinator.agent_profile_id>`
     (`models.MetaKeyAgentProfileID`) and
     `executor_profile_id = <coordinator.executor_profile_id>`
     (`models.MetaKeyExecutorProfileID`), no `start_agent`, no
     `prepare_session`, no external id and no `auto_start_on_create` marker,
     so `handleTaskCreated` starts nothing. The two profile keys are the only
     carriers of the coordinator's profiles: with no workflow step,
     `EnsureSession`'s `resolveTaskAgentProfile`
     (`internal/orchestrator/session_ensure.go`) takes the agent profile from
     `agent_profile_id` metadata before its assignee and workspace-default
     fallbacks, and the launch path reads `executor_profile_id` metadata
     (`internal/orchestrator/task_operations.go`), the same keys a plugin
     conversation task stamps. The values are copied once at create; a
     profile change archives this task ([coordinators](coordinators.md#routes)),
     so a current conversation task never carries stale profile ids. A test
     asserts the prepared session's agent profile and executor profile equal
     the coordinator's, with a different workspace default agent profile set.
  4. Run `UPDATE coordinators SET conversation_task_id = ? WHERE id = ? AND
     config_revision = ? AND (conversation_task_id IS NULL OR
     conversation_task_id = ?)` with the revision read in step 1 and the stale
     value read in step 2. One row updated: go to step 6 with the new task.
     Zero rows: re-read the coordinator row. When its `config_revision`
     differs from step 1's, a context or profile change was saved while this
     open created its task under the earlier configuration, even if the
     reference is NULL on both sides: delete the task just created and return
     409 (`AC-COORDINATOR-COPILOT-001.11`). Otherwise delete the task just created
     through the task service and go to step 6 with the row's current task,
     so racing opens converge on one task with one session. When the re-read
     instead finds `conversation_task_id` NULL (a concurrent context or
     profile change cleared it after the stale value was read in step 2, so
     no other task exists to converge on), delete the task just created and
     return 409; the panel's next open retries with the fresh row.
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
     an agent. On its error the route re-reads the coordinator row first: a
     row that is gone is answered as in step 7's deleted-coordinator case
     (404); otherwise the route returns 502 with the task kept as current,
     so the next open retries only this step.
  7. Re-read the coordinator's `conversation_task_id` and compare it to
     `taskID`. A concurrent context or profile change (steps 2 or 3 of
     [coordinators](coordinators.md#routes)) can archive exactly this task
     and clear the reference between step 2's read (the reuse path) or step
     4's commit (the create path) and this point, in which case they no
     longer match: return 409, with no task, so the panel's next open
     retries with the fresh value, the same shape as step 4's own race.
     When the re-read finds no coordinator row (checked before that
     comparison, so a deleted coordinator is never answered 409; the
     coordinator was deleted
     after step 4's commit, or after step 2's read on the reuse path), the
     route deletes `taskID` through the task service, counting
     `taskrepo.ErrTaskNotFound` as done because the coordinator delete's own
     `ListCoordinatorOriginTasks` pass may already have removed it, and
     returns 404. A failed delete is logged at warn with the task id and the
     route still returns 404; the task carries `coordinator_id`, which now
     names no row, so the [startup pass](#conversation-cleanup) deletes it.
     Otherwise return the session id with the task's archive state (always
     `false` from this route, since a task this route would return as
     archived is returned as 409 instead).
- `IsRestorableQuickChatTask` excludes origin `coordinator`, so the task never
  becomes a Quick Chat tab; board, list and snapshot queries already exclude
  ephemeral tasks.
- Quick Chat idle expiry excludes origin `coordinator`:
  `ListExpiredQuickChatTasks` and the re-check in
  `DeleteExpiredQuickChatTask` (`internal/task/repository/sqlite/task.go`)
  exclude origin `coordinator` beside their existing bound `automation_run`
  exclusion (`COALESCE(t.origin, '') NOT IN (?, ?)`), so a conversation idle for
  more than seven days is kept.

### Conversation lifecycle

| Event | Current conversation task | Earlier conversation tasks |
| --- | --- | --- |
| Context change ([coordinators](coordinators.md#routes)) | archived through the task service's `ArchiveTask`, which stops a running turn; the reference is cleared | unchanged (archived) |
| Open over an ended session (`FAILED`, `CANCELLED`, `COMPLETED`; step 2) | archived through the task service's `ArchiveTask`; the reference moves to the new task at step 4 | unchanged (archived) |
| Coordinator deletion | deleted through the task service, which stops a running turn | deleted |
| Workspace deletion | deleted with the workspace's tasks | deleted with the workspace's tasks |

An archived conversation task is never returned by the route, never resolves
to a coordinator (see [Principal and mode](#principal-and-mode)) and is never
listed. The panel of an archived conversation shows the missing-conversation
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
manager and why; propose changes), the workspace name and id, the context text
between explicit delimiters marked as operator-provided, and the rule that its
write actions are only those proposal tools bound to the conversation. These
tools can propose new tasks or actions on existing tasks, when each tool is
available. A person decides every proposal; none is applied automatically.
It also explains
the reference a manager's message may start with: the prefix
`About <id> [<kind>:<ref>]: ` names the item the question is about; for `proposal` and `stall`,
`get_coordinator_item_kandev` with that `kind` and `ref` as `id` reads its
record, and for `task` the task tools read it with `ref` as the task id. The block is attached
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
  tasks. One table test proves it: `TestCoordinatorConversationNoTurnStart`
  in `internal/coordinator/no_turn_start_test.go`, whose rows live in the
  package-level slice `noTurnStartPaths` (one row per backend path that could
  start a turn, each asserting no prompt is sent and no agent starts). This
  work package creates the file with the conversation-route, startup-cleanup
  and session-recovery rows; the stall and `workspace.deleted` subscribers
  (task 04) and the proposal decisions (task 07) append their rows to the
  same slice, and whichever of the three merges last adds any row still
  missing.
- `message.add` for a coordinator task requires `workspace.manage`, enforced
  in `Service.authorizeMessageCreate` → `AuthorizeTaskSessionPromptAccess`
  (`internal/task/service/service_access.go`), the single scope check
  `Service.CreateMessage` runs before building the message; both the WS
  `wsAddMessage` handler and any HTTP create-message route call
  `CreateMessage`, so neither transport can bypass it. A reader's message is
  refused before any session action.
- The panel passes `automaticRecovery={false}` to the Quick Chat session
  view, which forwards it to `useSessionResumption` as the new option
  `skipAutomaticRecovery`. With it set, the hook sends no check, resume or
  restore request on mount, on reload or on reconnect; it still reads the
  session from the store, so a turn that kept running while the page reloaded
  shows as running and the launcher shows busy (`AC-COORDINATOR-COPILOT-002.4`).
  The Retry action of the recovery feedback stays a manual action; it restores
  the execution and sends no message, so it starts no turn.
- Because the hook sets its error and notice only on its automatic path, the
  copilot does not rely on it for `AC-COORDINATOR-COPILOT-004.6`. The panel
  reads the session state from the store and keeps a coordinator-local
  recovery state: when the session is terminal or its start failed, it shows
  the recovery feedback with an action that re-runs the conversation open,
  which returns a fresh task and session (step 2). `useSessionResumption` is
  unchanged, so the task page, mobile and Settings chat keep their behaviour.
- While the session is running, the composer's send is disabled and Stop is
  offered (`AC-COORDINATOR-COPILOT-004.10`), so a message never queues behind a
  running turn.

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
  models.TaskOriginCoordinator`; the sibling `resolveTaskSessionMCPProfile`
  gains the matching branch returning a coordinator `mcpprofile.Context` with
  no user-question, title or canvas capability. In both resolvers the
  coordinator branch is the FIRST decision: each loads the task before the
  existing session `config_mode` check (today that check runs first, "config
  mode wins"), and a coordinator-origin task returns the coordinator mode and
  profile whatever the session's `config_mode` metadata says, then the
  automation and office branches follow as today. The conversation route never
  sets `config_mode`; a test sets it on a conversation session and asserts the
  coordinator mode and the seven-tool profile of [Tool surface](#tool-surface) still resolve. Because the task is now
  read before the `config_mode` check, the order in both resolvers is exactly
  as follows. "No row" is either form a missing task takes: an error with
  `errors.Is(err, repoerrors.ErrTaskNotFound)` (what the SQL `GetTask`
  returns; `internal/task/repository/repoerrors`) or a nil task with a nil
  error (executor test fakes). Any other `GetTask` error is a "read error".
  (1) `GetTask`; a read error fails the start for every session, config-mode
  ones included (a deliberate change: today a config-mode session never
  reads the task). (2) A task with origin `coordinator` takes the
  coordinator branch. (3) No row: the `CoordinatorLookup` check of
  [Fail closed](#fail-closed) runs; a match or a lookup error fails the
  start. (4) Otherwise a config-mode session returns config mode and config
  profile as today. (5) Otherwise, with no row, each resolver keeps today's
  result for the form it got: `ErrTaskNotFound` fails the start, a nil task
  gets no restricted mode and the `Legacy` profile; so no start that fails
  today launches after this. (6) A readable non-coordinator task follows the
  automation, office and Kanban branches as today. The tests, including each
  "no row" case in both forms, are listed in task 03's Verification. Quick
  Chat code does not
  set the mode. In `internal/mcp/server/server.go`, `normalizeMode`,
  `surfaceForMode`, `modeForProfile` and `Server.SetMode`, and `Legacy` in
  `internal/mcp/profile/profile.go`, gain the coordinator case; plugin tool
  registration is skipped in this mode.
- `internal/common/mcpmode/mode.go` adds `Coordinator` to `instanceModes`
  (so `InstanceModes` and `IsInstanceMode` accept it) and drops the comment
  saying a later package owns that; the agentctl `handleSetMcpMode` handler
  (`internal/agentctl/server/api/server.go`) builds its 400 message from
  `mcpmode.InstanceModes()` instead of its hard-coded mode list, so
  `SetMcpMode` with `coordinator` is accepted and an unknown mode's error lists
  it. Without this, every prepare-then-promote and mode-switch path would 400
  on the coordinator mode. A test posts `coordinator` to the handler (200) and
  an unknown mode (400 naming `coordinator` among the accepted modes). `mcpprofile.normalizeSurface`
  (`internal/mcp/profile/profile.go`) also gains the `SurfaceCoordinator`
  case: every `mcpprofile.New(...)` call funnels through it unconditionally,
  and its default case silently downgrades any Surface value it does not
  recognise to `SurfaceKanbanTask`, the full task tool set. Adding
  `SurfaceCoordinator` to the `Surface` consts and to `Legacy` without also
  adding it here would leave the coordinator session on the full Kanban tool
  set instead of the phase-1 tool profile below, so this switch is a required
  touch point, not an incidental one.
- **Wiring.** The shared hook call site in `registerCoordinatorRoutes`
  (`internal/backendapp/coordinator.go`) and the signature
  `registerCoordinatorConversation(router, eventBus, *coordinator.Service,
  logger)` stay unchanged. The dependencies reach the service first: a new
  file `internal/backendapp/coordinator_conversation.go`, owned by this work
  package, defines `wireCoordinatorConversation(p routeParams)`, called from
  the existing `if p.features.Coordinator` block in `registerSecondaryRoutes`
  (`internal/backendapp/helpers.go`, called from `registerRoutes`) on the
  line before `registerCoordinatorRoutes(p)`. It calls
  `svc.SetConversationDeps` with the task service (create, delete, archive)
  and the orchestrator service (`EnsureSession`) and `svc.SetConversationHooks`
  with the archive and delete hooks. `registerCoordinatorConversation` then
  reads the dependencies from `svc`, registers the conversation route and
  returns the startup cleanup hook; when the dependencies were not set it
  logs an error and registers nothing, so the route is 404 rather than
  half-built.
- **Lookup wiring.** The `CoordinatorLookup` is NOT handed over by
  `wireCoordinatorConversation`: the resolvers and the executor are built,
  and startup recovery runs, before routes register. It is set at
  construction from `services.Coordinator`, which `provideServices` builds
  first (`initCoordinatorWiring`, nil with the flag off). `mcpscope.Resolver`
  gains a `coordinators CoordinatorLookup` field and a
  `SetCoordinatorLookup(CoordinatorLookup)` setter; `NewResolver`'s signature
  is unchanged. The executor gains the same setter, reached through
  `orchestrator.Service.SetCoordinatorLookup`, which forwards to `Executor`.
  Each call is guarded by `services.Coordinator != nil`, so a nil
  `*coordinator.Service` is never stored as a non-nil interface. The call
  sites, and so the edits outside the new file, are exactly:
  (1) `startAgentInfrastructure` in `internal/backendapp/main.go`: on the
  resolver built there (`mcpScopeResolver`, whose `Scope` and
  `ScopePrincipal` go to `provideLifecycleManager`), right after
  `mcpscope.NewResolver`, and on `orchestratorSvc` right after
  `provideOrchestrator`, before `orchestratorSvc.Start` runs startup
  recovery; (2) `registerMCPAndDebugRoutes` in `internal/backendapp/helpers.go`:
  on the resolver built there, right after `mcpscope.NewResolver` and before
  `SetMCPPrincipalScoper`, since that scoper is the one installed last and so
  the one that scopes in-session dispatch; (3) the one call line in
  `registerSecondaryRoutes` above. The resolver in `buildHandoffDependencies`
  (`internal/backendapp/handoff_wiring.go`) gets no lookup: it serves only
  `ScopeOverridingIdentity` for the Office handoff's workspace check and never
  resolves a principal surface. A resolver or executor that never received
  the lookup resolves a coordinator-origin task to no coordinator and refuses
  it ([Fail closed](#fail-closed)), so a missed wiring site denies instead of
  granting. With the flag off no lookup is set, and a coordinator-origin task
  is refused everywhere. A test builds each of the two resolvers with a
  lookup and asserts a coordinator conversation task resolves to
  `SurfaceCoordinator` and its coordinator; without the lookup it is refused.

## Tool surface

The phase-1 tool profile (`AC-COORDINATOR-COPILOT-003.1`: seven tools), the
MCP guard and the `get_coordinator_item_kandev` item read are specified in
[coordinator tool surface](copilot-tools.md#tool-surface).

## Fail closed

Before a coordinator session starts or resumes, these checks run: flag on,
coordinator resolvable, task readable, agent profile present and not
passthrough, executor profile present,
mode set to `Coordinator`. Any failure stops the start with an error surfaced
through the session recovery feedback. No branch falls back to the default
task mode.

**Where.** The check site is the coordinator branch of the executor's two
resolvers (`resolveTaskSessionMCPMode`, `resolveTaskSessionMCPProfile` in
`internal/orchestrator/executor/executor_execute.go`). Every path that builds
an agentctl instance for a task session calls them before the instance
starts: first launch and prepare (`executor_execute.go`), resume and
re-created executions (`executor_resume.go`) and interaction launches
(`executor_interaction.go`). Promotion of a workspace-only execution is one
of those paths, not a separate one: the agent is started by an executor
launch or resume call that resolves the mode again through these resolvers
and hands it to `configureExistingWorkspace` before `LaunchAgent` promotes
the execution (`executor_execute.go`), so every fail-closed check runs again
at promotion. Nothing carries prepare's decision forward; a coordinator
whose flag, row or profile went away between prepare and promotion gets no
agent.
`WorkspaceInfo.McpMode` ([Permission policy](#permission-policy)) only sets
the mode of the agentctl instance that the lifecycle builds on its own
without an agent (workspace-only restore); it starts nothing. For a
coordinator-origin task the
branch returns an error, and the caller starts nothing, when the executor has
no `CoordinatorLookup` (the flag is off, so none was wired), when the lookup
finds no coordinator whose current `conversation_task_id` is the task, when
the lookup errors, or when `profileStatus` for that coordinator reports either
profile not `ok` or itself errors.

**Unreadable task.** A `GetTask` read error already fails both
resolvers; that stays. "No row" and "read error" are as defined in
[Principal and mode](#principal-and-mode): `ErrTaskNotFound` and a nil task
are both "no row". With no row, the mode resolver today fails on
`ErrTaskNotFound` and the profile resolver falls back to the full `Legacy`
Kanban profile on a nil task. Before either outcome, both
resolvers ask the `CoordinatorLookup` whether any coordinator's
`conversation_task_id` equals the task id, a query on the coordinator store
that needs no task row. A match, or a lookup error, fails the start with an
error naming the coordinator task; only no match with no error keeps
today's result (steps 4 and 5 of that order). The lookup is wired only when the flag is on
([Flag and wiring](coordinators.md#flag-and-wiring)); with the flag off a
start whose task row is absent keeps today's per-form result, which is an accepted
phase-1 residual: a deleted task's sessions go with it, so the only such
start is the existing transient case the fallback's own comment describes,
and a readable coordinator-origin task still fails closed on the missing
lookup. A test covers a coordinator start whose task row is absent, in both
"no row" forms: an error and no instance, with the flag on. The panel follows the profile statuses of
[coordinators](coordinators.md#validation): when the coordinator GET reports
`agent_profile_status` or `executor_profile_status` other than `ok`, it does
not call the conversation route and shows the matching messages in place of
the composer. A 409 from the route
(a profile deleted or switched to passthrough since the GET) carries both
statuses in its `coordinator_profile_unavailable` body, and the panel shows
the messages built from them.

## Permission policy

- A new `McpMode` field on `lifecycle.WorkspaceInfo`
  (`internal/agent/runtime/lifecycle/types.go`, built by the task service in
  `internal/task/service/service_turns.go`) carries the coordinator mode to
  the agentctl instances the lifecycle builds from `WorkspaceInfo` alone
  (workspace-only restore and admission through
  `GetWorkspaceInfoForSession`), which never pass through the executor
  resolvers. Its only source is the task row: `GetWorkspaceInfoForSession`
  reads the task by `session.TaskID` through the task repository and sets
  `McpMode = mcpmode.Coordinator` when `task.Origin ==
  models.TaskOriginCoordinator`, and leaves it empty (today's default) for
  any other origin. "No row" and "read error" are as defined in
  [Principal and mode](#principal-and-mode): a read error returns the error,
  as a session-read error does today; no row (`ErrTaskNotFound` or a nil
  task) leaves it empty and is not an error, which is safe because
  such an instance starts no agent and the agent-starting call goes through
  the executor resolvers ([Fail closed](#fail-closed)), which refuse it. The
  lifecycle passes `WorkspaceInfo.McpMode` into the instance it creates, and
  the executor paths (first launch, prepare, resume, interaction and the
  launch that promotes a workspace-only execution) pass the mode their
  resolvers returned (tests: task 03's Verification). agentctl itself does not consult its own
  `cfg.AutoApprovePermissions` when its mode is `Coordinator`; the mode, not
  the CLI's local config, decides.
- In the coordinator mode `AutoApprovePermissionsOverride=false` is applied on
  first launch, on a re-created or resumed execution and when a workspace-only
  execution is promoted, so the profile flag and the agentctl auto-approve
  environment variable are ignored.
- Kandev auto-approves a permission request only when the request's tool
  name parses with `ParseQualifiedMCPToolName`
  (`internal/agentctl/types/permission_identity.go`, the
  `mcp__<server>__<tool>` form ACP clients send, for example
  `mcp__kandev__list_tasks_kandev`) to server `kandev` and a tool that is one
  of the seven tool names of [Tool surface](#tool-surface)
  (`AC-COORDINATOR-COPILOT-003.1`), each compared as the full string, never
  by prefix. A test enumerates the seven and asserts each is auto-approved and
  that `get_task_plan_kandev` is not.
  A name that does not parse is not auto-approved.
- Every other request reaches the panel through the existing permission
  message flow with Approve and Deny.

## Residual external surface

Kandev does not register or proxy the agent CLI's own tools (its shell, MCP
servers and permission rules), so the guard does not see them; and board
content returned by the allowed tools can steer the coordinator. The
[ADR](../../../decisions/2026-09-26-workspace-coordinator.md#residual-risk-the-agents-own-tools)
records both: containment is a gate G3 condition, and content steering is
accepted for phase 1.

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

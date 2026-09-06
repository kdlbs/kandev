---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-001
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-003
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-005
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-007
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
---

# Workflow Step Script Actions System Design

## Purpose and boundaries

The task system owns the portable action, trigger ordering, session binding,
failure policy, durable run record, and chat audit. The agent runtime owns
managed process execution inside an executor. Agentctl owns shell invocation,
process-group termination, bounded output, and process status.

This design adds a session-bound `run_script` action to `on_enter`,
`on_turn_complete`, and `on_exit`. It reuses the `script_execution` message
presentation but does not reuse the host-local worktree script runner.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-001` | [Action contract](#action-contract), [Portable workflows](#portable-workflows) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-002` | [Session binding](#session-binding), [Runtime execution seam](#runtime-execution-seam) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-003` | [Trigger flows](#trigger-flows), [Failure application](#failure-application) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-004` | [Durable run model](#durable-run-model), [Recovery](#recovery) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-005` | [Chat projection](#chat-projection), [Output bounds](#output-bounds) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-006` | [Lifecycle action recipes](#lifecycle-action-recipes), [Focused action editing](#focused-action-editing) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-007` | [Trust boundary](#trust-boundary), [Observability](#observability) |
| `REQ-TASKS-WORKFLOW-STEP-SCRIPT-008` | [Editor architecture](#editor-architecture), [Desktop workflow workspace](#desktop-workflow-workspace), [Mobile workflow navigation](#mobile-workflow-navigation), [Draft and validation state](#draft-and-validation-state) |

## Action contract

The existing trigger-specific action arrays gain `run_script` enum values. The
engine compiles every supported persisted value into one typed action:

```yaml
events:
  on_enter:
    - type: run_script
      config:
        command: pnpm install --frozen-lockfile
        timeout_seconds: 600
        failure_policy: block
```

The typed config is:

```text
RunScriptAction {
  command: string
  timeout: duration = 10m
  failure_policy: block | continue = block
}
```

Model validation trims only to decide whether `command` is empty. It preserves
the exact command text for execution and display. Omitted values receive
defaults at compile time. Explicit invalid values fail workflow validation at
HTTP, import, sync, and embedded-template boundaries.

The command is the only executable field. A workflow can invoke a checked-in
file with a command such as `./scripts/verify.sh`; no separate path resolution
or executable-bit policy is introduced.

## Portable workflows

The action remains inside the existing portable `events` structure, so the
portable format version does not change. Export writes the normalized config.
Import and repository sync validate the config before mutating the workflow.
Duplication copies it with the rest of the step events.

The backend remains the validation authority. Frontend validation uses the same
limits for immediate feedback but cannot broaden the server contract. Unknown
action types retain their existing compatibility behavior; a recognized
`run_script` with malformed config is never silently skipped.

## Session binding

The orchestrator resolves one bound session before it asks the script runner to
admit a process:

| Trigger | Bound session | Point in lifecycle |
| --- | --- | --- |
| `on_enter` | Destination session | After `prepareWorkflowStepSession`, profile routing, context reset, and session config; before any automatic prompt |
| `on_turn_complete` | Source session | After the source turn settles; before transition commit |
| `on_exit` | Source session | After a destination is selected but before transition commit and source retirement |

For `on_enter`, `profile_session_start_policy` chooses whether the destination
profile reuses a parked session or creates a new one. The source step's end
policy can then complete or park the old profile session. The script never
temporarily writes to the old tab while the new profile is being prepared.

When consecutive steps have the same effective profile, the existing routing
policy retains the active session. Entry output appears in that same transcript.

Turn-complete and exit keep the source session even when the destination uses a
different profile. This preserves causal history: the source agent's answer,
its completion checks, and its exit checks remain together.

## Runtime execution seam

Add a narrow runtime service for managed workspace commands rather than making
the workflow package depend on lifecycle internals:

```text
WorkspaceProcessRunner.Start(ctx, request) -> process
WorkspaceProcessRunner.Get(ctx, executionID, processID, includeOutput) -> state
WorkspaceProcessRunner.GetByRequestID(ctx, executionID, requestID, includeOutput) -> state
WorkspaceProcessRunner.Stop(ctx, executionID, processID) -> error
```

`request` contains the stable run ID, session ID, execution ID, command,
working directory, timeout, process kind, and output limit. The runtime resolves
the live `AgentExecution`, its agentctl client, managed environment, and
`Execution.WorkspacePath`. It rejects a mismatched or stale execution identity.

Agentctl starts the command with its current platform shell behavior and emits
stdout, stderr, and status on the workspace process stream. The runtime does
not start or prompt an LLM process. The action can run against a prepared
workspace before the destination agent subprocess starts.

`GetByRequestID` is a recovery-only lookup. It attaches a `starting` run to an
already admitted process when the process ID was not persisted before a crash;
it never starts a replacement command. Implementations must validate the exact
execution and returned session before exposing the process.

The start request carries the workflow script run ID as an idempotency key.
Agentctl returns the existing process for a duplicate request with the same key
and rejects reuse with different process inputs. This closes retry races while
agentctl remains alive.

## Durable run model

A new `workflow_script_runs` repository is shared by SQLite and Postgres. Each
row stores:

- Run ID and unique occurrence key.
- Task, workflow, step, trigger, and zero-based action position.
- Bound session and agent execution IDs.
- Command, timeout seconds, and failure policy snapshots.
- Message and agentctl process IDs.
- Status: `pending`, `starting`, `running`, `succeeded`, `failed`, `timed_out`,
  or `interrupted`.
- Exit code, failure reason, output-truncated flag, and timestamps.

Occurrence keys are stable for the event that caused execution:

- Entry uses the durable workflow step-entry row plus action position.
- Turn completion uses the completed turn identity, source step, and action
  position.
- Exit caused by the same transition uses the transition occurrence identity,
  source step, and action position.

Every script-capable entry path must therefore supply a real step-entry ID.
Every turn-completion path must propagate the owning turn or prompt-generation
identity into workflow evaluation. A manual or deferred move uses its durable
move/transition identity. The repository has a unique constraint on the
occurrence key, and duplicate claims return the existing row.

The row is created with the immutable action snapshot before a message or
process is created. The runner persists `starting` before it calls agentctl.
This deliberately prefers a possible interrupted result over repeating a
non-idempotent command after an ambiguous crash.

## Trigger flows

### Entry

1. The transition commits and allocates its step-entry record.
2. Existing profile routing chooses and prepares the destination session.
3. Existing context reset and conditional session config finish.
4. The session-bound entry dispatcher walks entry actions. Each script claims
   its durable run, creates its message, executes, and applies its policy.
5. If entry dispatch is not blocked, the existing automatic or implicit
   profile-switch prompt runs.

Repository-owned, session-independent entry actions keep the existing
step-entry ledger and marker claims. They can commit before the destination
session dispatcher. Their effects are not rolled back by a later script
failure. The session dispatcher skips them, and the repository dispatcher
skips `run_script`, preserving one owner per action.

### Turn completion

The completion coordinator passes the stable completed-turn occurrence into
`Engine.HandleTrigger`. The normal action loop executes scripts and resolves
the first eligible transition in declared order. `EvaluateOnly` continues to
defer the transition commit, but it does not suppress script callbacks. A
blocking callback error returns no transition result. A continuing callback
records its failure and returns success to the engine so later actions run.

### Exit

After target validation and credential preflight, the transition coordinator
runs the source step's exit action list with the same transition occurrence.
It changes `processOnExit` from a best-effort void hook into a result-bearing
gate. A blocking result prevents the commit. Only after exit succeeds or
continues may the transition, source-session retirement, and destination entry
proceed.

Workflow-switch, deferred move, manual move, automatic completion, and explicit
workflow-step launch paths use these same trigger helpers. None can bypass the
script boundary.

## Failure application

The script coordinator always persists the terminal result before it returns a
policy outcome.

If the initial chat message cannot be created, the coordinator does not admit
the process. If a message update fails after admission, the coordinator stops
the process, records the persistence failure in the run row when storage is
available, and applies the action's failure policy. A command is not allowed to
continue invisibly after its required audit projection has failed.

`continue` converts a terminal script failure into a successful workflow
callback after the message and run row are final. The next action runs.

`block` returns a typed `WorkflowScriptBlockedError`. Turn-complete and exit
callers keep the task at the source step, set the source session to a promptable
waiting state, and publish the state. Entry callers cannot undo the committed
arrival; they stop the remaining session-bound entry actions and automatic
prompt, then leave the destination session waiting.

The error retains run and message IDs for logging and future recovery UI. This
version does not add an automatic or one-click retry. The user can inspect the
message, correct the workflow or repository, and deliberately cause a new
trigger occurrence.

## Chat projection

The coordinator creates one task-service message before process admission:

```text
type: script_execution
author_type: agent
task_session_id: bound session
metadata:
  script_type: workflow_step
  workflow_script_run_id
  workflow_step_id
  workflow_step_name
  trigger
  action_position
  command
  timeout_seconds
  failure_policy
  status
  process_id
  exit_code
  started_at
  completed_at
  output_truncated
  error
```

The message content holds the bounded combined output in process event order.
The task service remains the single persistence and WebSocket publication path.
Script messages have no user-input request, carry no `turn_id`, and do not
complete or create a turn.

The frontend extends `ScriptExecutionMessage` with a workflow-step header and
trigger badge. It displays command, output, duration, status, exit code,
failure reason, and truncation notice. The processing filter recognizes only
setup/cleanup preparation messages; `workflow_step` remains in the normal
chronological transcript.

## Output bounds

Agentctl's managed process buffer remains the authoritative byte budget. The
workflow request uses the current default managed-process limit. The task
message accumulator mirrors the ring buffer's newest retained UTF-8-safe
content and sets `output_truncated` after eviction.

The coordinator batches message persistence on a short fixed cadence and
flushes immediately at terminal status. It does not create one database write
per process chunk. Live output can therefore lag process output by one batch
interval while remaining visibly streaming.

## Recovery

Startup reconciliation scans nonterminal workflow script runs after task and
runtime repositories are ready.

- `pending` without an admission attempt can proceed normally.
- `starting` has an ambiguous admission boundary. Reconciliation looks up the
  stable request identity in the recorded execution but never issues a fresh
  start. If it cannot find the process, it marks the run interrupted.
- `running` with a process ID fetches buffered output and status from agentctl.
  A live process is observed until terminal or until explicit shutdown stops it.
- A missing execution, replaced agentctl generation, unknown process, or
  irreconcilable status produces `interrupted`.
- Terminal rows are immutable except for safe completion of a missing message
  projection. Their stored policy outcome is reused on duplicate dispatch.

The reconciling path updates the original message. It never creates a second
message or process. A duplicate trigger caller waits for an in-progress local
run or reads the durable result.

## Editor architecture

Workspace settings keeps a lightweight, reorderable workflow list. Opening one
workflow navigates to a dedicated editor route owned by the workspace settings
area. This avoids expanding every workflow and step in the list and gives the
editor a stable route-level draft contributor.

Existing workflows use
`/settings/workspace/:workspaceId/workflows/:workflowId`. New workflow creation
navigates to `/settings/workspace/:workspaceId/workflows/new` with the selected
template identity and creates the workflow plus initial steps as client-only
route drafts. The first successful Save replaces the URL with the persisted
workflow identity. Reload before that Save discards the draft, matching the
settings manual-save contract.

The editor uses a constrained pipeline, not a freeform canvas. It derives node
order from persisted step order and derives connectors from existing transition
actions. Authors can reorder steps but cannot store coordinates, create visual
edges outside the current transition contract, or pan and zoom an unbounded
surface.

A shared view-model layer derives presentation data without changing the wire
model:

```text
WorkflowEditorViewModel {
  step_summaries[]
  transition_edges[]
  lifecycle_action_groups_by_step
  configuration_issues[]
  selected_step
  selected_tab
  selected_action
}
```

Each step summary contains name, color, effective profile label, action count,
primary destination, dirty state, read-only state, and issue severity. The
effective profile is the current step override followed by the current workflow
profile and task default rules. The view model explains that result but does
not add a new inheritance tier.

An action catalog adapts existing trigger-specific action objects into editor
descriptors. Each descriptor supplies its localized label, summary, compatible
triggers, configuration editor, validation adapter, and transition projection.
It does not replace or flatten the persisted `events` shape. Unknown actions in
a synchronized workflow remain inspectable and are never discarded by a
frontend normalizer.

## Desktop workflow workspace

Desktop uses one editor shell with a compact header, the constrained pipeline,
and a persistent right inspector. The pipeline takes the flexible width; the
inspector uses a bounded width suitable for form controls. A long pipeline may
scroll within its own horizontal region, but the page does not overflow.

Selecting a step opens three inspector tabs:

- **Agent** contains the prompt, effective profile, profile override, session
  start/end behavior, and other agent-facing configuration.
- **Automation** contains lifecycle action recipes and transition behavior.
- **Policies** contains less frequent task and session policy toggles such as
  manual movement, WIP, archive, plan mode, command panel, and context reset.

The exact ownership of an existing control follows its user intent; the tabs do
not change persistence or runtime meaning. Only the selected step and selected
action render detailed controls. Pipeline nodes and action rows remain compact
summaries with explicit dirty and error markers. Destructive removal remains a
named, confirmed operation for persisted steps and workflows.

Workflow-level configuration checks aggregate field and transition diagnostics.
Each issue carries a selection target consisting of step, tab, optional trigger
and action position, and optional field. Activating the issue updates the editor
selection and moves focus to the resolvable control.

Reference desktop composition:

```text
+ Workflow: Feature delivery ------------------------- [Checks: 2] ---+
|                                                                     |
|  [Plan] ----> [Build *] ----> [Review !] ----> [Done]               |
|                           |  Step inspector                          |
|  * dirty   ! issue        |  Agent Automation Policies              |
|                           |  --------------------------------------  |
|                           |  When task enters        2 actions       |
|                           |  When agent finishes     3 actions       |
|                           |  When task leaves        1 action        |
|                           |                          [+ Add action]   |
+---------------------------------------------------------------------+
                         [Unsaved changes] [Reset] [Save changes]
```

## Lifecycle action recipes

The Automation tab presents three ordered groups matching runtime boundaries:

- **When task enters** maps to `on_enter`.
- **When agent finishes** maps to `on_turn_complete`.
- **When task leaves** maps to `on_exit`.

Each group renders compact action rows with an action label, human-readable
summary, order, dirty state, and validation state. Selecting a row opens its
focused editor. The add-action palette receives the trigger and displays only
catalog actions supported by that trigger. Reorder and delete mutate only that
trigger's array. Transition actions update the derived pipeline edge as soon as
the local draft changes.

The catalog exposes `run_script` for all three groups. Its summary shows the
first meaningful command line and policy. The focused editor contains the
multiline command, timeout seconds with the 600-second default, and failure
behavior of **Block workflow** or **Continue workflow**. It also explains the
selected lifecycle boundary and bound-session rule. A command editor may own
horizontal code scrolling, but the enclosing inspector never causes page
overflow.

## Focused action editing

Selecting an action replaces the inspector's action list with one editor and a
clear back affordance. This state is a presentation route over the workflow
draft, not a dialog with a separate save contract. Edits remain visible when
the author moves between actions or steps.

Selection is encoded in shallow route state for step, tab, trigger, and action
position so deep links and browser Back are predictable. The route-level draft
shell stays mounted across these selection changes. Action positions are
transient UI identities; add, delete, and reorder mutations repair selection
and replace invalid route state without adding IDs to the workflow format.

Existing action types migrate into the same descriptor pattern so the redesign
does not create a second legacy form beside the recipe. The first release does
not execute a **Test action** operation from the editor. Testing a script would
require an independently specified executor, permission, audit, and side-effect
lifecycle.

## Mobile workflow navigation

The nearest shipped mobile pattern is the settings and kanban drawer family,
including `MobilePickerSheet`, but a drawer is not the primary workflow editor.
On phone viewports the dedicated workflow route shows a vertical journey of
step cards. Selecting a step navigates to a full-height step editor; selecting
an action navigates again to a full-height focused action editor. Browser Back
returns through action, step, and journey states while the route shell retains
the unsaved draft.

The step screen uses **Agent**, **Automation**, and **Policies** tabs. It may
shorten the visible Policies label to **Rules** when the localized label needs
space, while preserving the same content and accessible name. Adding an action
opens an inset bottom drawer only for the temporary choice, then navigates to
the focused editor.

Mobile uses explicit move up/down controls for step and action ordering instead
of requiring precise drag gestures. Every primary operation has at least a
44-by-44 CSS-pixel target, safe-area padding protects fixed actions, and each
screen has one vertical scroll owner. There is no document-level horizontal
overflow and no operation available only through hover. Tablet widths may use
the desktop shell or a stacked pipeline and inspector according to available
space, but retain the same selection model.

Reference phone composition:

```text
+ Feature delivery -----------------------------+
| Checks: 2                         [More]       |
|                                               |
|  1  Plan                                      |
|     Agent: default     2 actions       >       |
|     |                                         |
|  2  Build *                                   |
|     Agent: Codex       5 actions       >       |
|     |                                         |
|  3  Review !                                  |
|     Agent: Reviewer    4 actions       >       |
|                                               |
|                              [+ Add step]     |
+-----------------------------------------------+
| Unsaved changes       Reset     Save changes |
+-----------------------------------------------+

Build > Automation > When agent finishes
+ Run command ----------------------------------+
| Command                                       |
| pnpm test                                     |
| Timeout: 600s       Failure: Block workflow   |
|                         [Move] [Delete]        |
+-----------------------------------------------+
```

## Draft and validation state

The editor preserves the settings manual-save contract. All workflow metadata,
step, action, transition, and ordering changes update one route-local draft.
The shared fixed **Save changes** surface is the only persistence action for
those edits. Navigation among the pipeline, tabs, steps, and actions does not
persist or discard data. Leaving the editor while dirty invokes the existing
**Save and leave**, **Discard and leave**, or **Continue editing** flow; reload
uses the existing native warning.

Client-only workflow and step identities retain their current remapping rules.
Dirty markers project onto the changed control, focused inspector, compact step
node/card, workflow header, and shared save surface. A server or WebSocket
refresh may update a clean draft but never overwrites a dirty one.

Local validation feeds both inline field errors and workflow configuration
checks. Save remains disabled while any dirty workflow action is invalid. The
backend remains authoritative, and save errors return focus to the issue target
when the response identifies one. Synchronized and managed workflows use the
same viewer with mutation controls disabled and a visible reason.

## Trust boundary

Workflow scripts are trusted workflow configuration and have the same command
execution power as the selected task executor. They are not a sandbox. The
existing workflow mutation authorization controls who can author them.

The runtime passes its managed environment but does not interpolate secrets
into the command text. Environment values are omitted from action metadata and
structured logs. Command and output are visible to anyone who can read the task
chat, so imports and sync reviews must treat them as executable code.

## Observability

Structured lifecycle logs use bounded identifiers and terminal fields. They do
not log environment maps. Counters record starts and terminal outcomes with
only `trigger` and `outcome` labels. The run ID links logs, the database row,
agentctl process, and chat message.

## Public documentation

`docs/public/tasks-and-workflows.md` documents authoring, trigger timing,
failure behavior, session/profile binding, timeout, chat output, and recovery.
`docs/public/workflow-import-export.md` documents the portable action config and
the executable-code trust warning. `docs/public/workflow-sync.md` carries the
same review warning for repository-owned workflows.

## Related decisions

- [Bind workflow scripts to the trigger-owning agent session](../../../decisions/2026-09-05-workflow-script-session-binding.md)
- [Use a constrained pipeline and focused workflow inspector](../../../decisions/2026-09-05-workflow-editor-pipeline-inspector.md)
- [Separate workflow step session start and end behavior](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Host utility agentctl for sessionless ACP flows](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md)

---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
---

# Workflow Step Agent Start Ownership System Design

## Purpose and boundaries

The task system owns workflow entry and agent turn admission. The agent runtime owns provider sessions and prompt completion signals.

This design defines the reset boundary between these systems. It covers never-started, idle, and active task sessions.

The design preserves runtime configuration through the existing reset contract. It does not change provider reset support or explicit user cancellation.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001` | [Session states](#session-states) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002` | [Active-turn reset flow](#active-turn-reset-flow), [Bounded predecessor wait](#bounded-predecessor-wait), [Reset failure containment](#reset-failure-containment) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003` | [Prompt fallback ownership](#prompt-fallback-ownership), [Prompt-history contract](#prompt-history-contract), [Workflow-entry prompt flow](#workflow-entry-prompt-flow) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004` | [Creation destination routing](#creation-destination-routing) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005` | [Asynchronous launch prompt preservation](#asynchronous-launch-prompt-preservation) |

## Components and responsibilities

`orchestrator.Service.resetAgentContext` owns workflow reset semantics. It also owns the session reset marker, the session lifecycle lock, and the shared per-session cancellation guard that closes prompt-admission races.

The cancellation coordinator owns active-turn quiescence. It uses the internal cancellation path, not the explicit user cancellation path.

`lifecycle.Manager.ResetAgentContext` replaces the provider session after quiescence. It restores runtime configuration through the existing reset contract.

`lifecycle.SessionManager` owns prompt serialization and the dispatch-only completion barrier. Prompt generations continue to identify completion ownership.

The orchestrator owns the task-description fallback decision. The task
repository owns the durable prompt counter and its atomic initial-fallback
claim. Direct user-message persistence and the fallback claim use the same
per-session write boundary.

The task service owns the initial workflow-step destination. It derives the
destination from explicit step selection and agent-start intent. Agent mode is
an execution setting and cannot override an immediate start.

## Creation destination routing

`task.Service.resolveWorkflowStep` applies this precedence:

1. Use an explicit `workflow_step_id` without further resolution.
2. If `StartAgent` is true, call `ResolveAutoStartStep` regardless of
   `PlanMode`.
3. If only `PlanMode` is true, call `ResolveFirstStep` for the existing
   plan-only prepared-session path.
4. Otherwise, call `ResolveStartStep`.

`ResolveAutoStartStep` selects the first positional step whose `on_enter`
actions include `auto_start_agent`. If no such step exists, it calls
`ResolveStartStep`. That resolver uses the configured start step and then the
first positional step as its fallback.

The HTTP and WebSocket create transports preserve both `start_agent` and
`plan_mode` in `CreateTaskRequest`. The service uses `start_agent` as the launch
intent before session preparation or launch begins.

Desktop and mobile use the same task-create payload builder and submission
handler. The implementation does not change their layout, labels, touch
targets, or navigation. Separate Playwright scenarios exercise the desktop
split-menu action and the mobile plan-mode action.

## Session states

A `CREATED` session has no conversation to reset. The workflow reset skips provider work and leaves the first start to `auto_start_agent`.

An idle session has no active turn. The workflow reset replaces its provider context directly.

An active session has a current turn. The workflow reset uses the active-turn flow before it replaces the provider context.

## Active-turn reset flow

The workflow reset takes the session lifecycle lock and the shared per-session cancellation guard. It sets the session reset marker while holding that guard before it starts quiescence.

The reset marker rejects new prompt admission. Normal and lifecycle prompt claims recheck the marker immediately before their final guarded claim. This check and marker publication use the same guard.

If the session owns an active turn, the orchestrator starts an exclusive internal cancellation operation, then releases the guard while it waits for the bounded lifecycle cancellation and escalation path. It reacquires the guard before provider replacement. If another cancellation already owns the session, reset fails closed instead of inheriting that operation's source-specific reconciliation. A reset with no active turn also fails closed when a cancellation operation is already in flight.

The internal operation reconciles the active turn and session state. It does not create the visible user-cancellation message or evaluate `cancel_triggers_turn_complete`.

The orchestrator calls `lifecycle.Manager.ResetAgentContext` only after the internal operation finishes. An error stops the workflow entry before automatic prompt dispatch.

After a successful reset, the existing entry flow marks the session idle. Then `auto_start_agent` dispatches the step prompt into the new provider session.

The reset marker stays active through quiescence, provider replacement, configuration restoration, and reset-state persistence. The guard and marker together prevent successor admission races.

## Bounded predecessor wait

A dispatch-only prompt leaves `dispatchedPromptPending` set until its completion signal arrives. A later prompt waits at this barrier before it resets shared buffers.

`waitForPendingDispatchedPrompt` uses a 10-second internal timeout. The caller context can end the wait sooner.

If the timeout expires, the function returns a typed transient error. It does not clear the pending flag or dispatch the successor prompt.

The error releases `promptMu` and the orchestrator dispatch guard through existing deferred cleanup. Queued workflow prompts return to the queue through existing transient-error handling.

After guard release, cancellation can use its existing escalation path. That path clears the pending flag and emits a generation-bound synthetic completion signal.

## Completion ownership

Provider completion events keep their `(agent_execution_id, prompt_generation)` identity. The lifecycle manager rejects a completion that does not own the current generation.

An unnumbered completion cannot release a pending numbered dispatch-only prompt. This prevents a delayed synthetic completion from being consumed as the predecessor's completion.

Internal cancellation reconciles the old generation before provider replacement. Escalation releases local completion state but does not prove that the provider stopped. The reset does not drain a completion signal from an active generation.

## Reset failure containment

This draft extension covers `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002`.
Its delivery package is [Workflow reset failure containment](../../../plans/workflow-reset-failure-containment/plan.md).
The task system owns this contract because workflow entry controls successor prompt admission.

### Preserve the provider cancellation outcome

The cancellation coordinator retains the raw provider outcome separately from its reconciliation result.
`cancelOperation` owns this outcome under the existing coordinator synchronization.
`cancelAgentWhileUnlocked` must not erase `lifecycle.ErrCancelEscalated` before that outcome is captured.

Explicit cancellation, clarification, peer interruption, and queue cancellation retain their current success semantics after escalation.
They still reconcile the captured turn and session. Their shared operation error must not become an escalation error.
Joined explicit callers must still perform their own reconciliation.

The exclusive reset helper reads the provider outcome after the owned cancellation operation completes.
`quiesceActiveResetTurn` rejects escalation even when local reconciliation succeeds.
An unset outcome cannot count as provider confirmation for an active execution.
Missing executions retain the existing persisted-resume cleanup behavior after lookup confirms their absence.

This path returns reset failure. It does not attempt automatic process replacement after escalation.
Process replacement needs stronger evidence than local cancellation reconciliation.
The existing restart helper tolerates a stop error, so calling it alone does not prove provider termination.
The reset marker and guard retain their existing ownership and release order.

### Bound the provider reset request

`Manager.ResetAgentContext` supplies a child context with a 10-second deadline to `client.ResetSession`.
The earlier caller deadline wins. The child context covers both the WebSocket write and response wait.
The HTTP client timeout does not apply to this WebSocket request.

On reset timeout or caller cancellation, return a wrapped error before the restart fallback.
Do not classify a timeout as unsupported reset. Do not launch a goroutine that returns while provider work still owns the guard.
The WebSocket pending-request cleanup removes the abandoned correlation ID.
Late replies cannot publish reset success, clear a successor generation, or dispatch a step prompt.

Ordinary unsupported-reset errors retain the existing restart fallback and captured runtime configuration.
Timeout changes neither that fallback's semantics nor its separate initialization budget.
Contention on `remoteInstanceLifecycleMu` or `streamWriteMu` is a separate wait from the unanswered request addressed here.
Tests must establish that the peer received the request before measuring its response deadline.

### Persist a visible failure

The workflow reset returns an error through an error-bearing helper.
The existing boolean wrapper can remain for callers that need only success or failure.
`processOnEnter` records the error and preserves its early return before automatic prompt dispatch.
It must not call the general agent-failure handler, which can run `on_agent_error` workflow actions.

Use the existing `LastAgentError` metadata and `persistLastAgentError` publication path.
Store a bounded, sanitized reset-specific message with the session and current execution identity.
The message states that context reset failed and that the workflow step prompt did not start.
Publish the waiting-state projection from fresh metadata so it cannot overwrite the error with the pre-reset session snapshot.
Use a short bounded cleanup context if the request context has expired.
Log persistence failure with task, session, and step identity; never report a successful reset after that failure.

Desktop and phone reuse `LastAgentErrorNotice` in the existing conversation flow.
The notice remains visible after reload and uses the existing dismissal behavior.
No new control, navigation surface, or workflow recovery action is introduced.
The phone conversation remains its own scroll surface, with the existing dismissal control.
Give that control a coarse-pointer target of at least 44 pixels without changing its desktop dimensions.
Any new static frontend copy must use the existing five-language localization contract.

Automatic prompting remains blocked on every reset failure.
An operator can remove the affected session after reset releases its guard.
This change does not make re-entering an idle step an automatic restart gesture.

## Prompt fallback ownership

A non-empty `WorkflowStep.Prompt` is work for each applicable step entry. The
orchestrator evaluates and dispatches this prompt with the current placeholder
rules.

An empty `WorkflowStep.Prompt` does not define new step work. For an unprompted
session, the task description supplies the first prompt. After that first user
prompt, the task description is no longer a workflow-entry prompt.

This rule changes only the empty-step fallback. It preserves workflow-level
instructions, prompt reference expansion, plan-mode context, and queued handoff
behavior.

## Prompt-history contract

`task_session_prompt_seq.last_seq` is the durable session prompt counter. A
positive value is an accepted user-prompt ordinal. The zero value is reserved
for an admitted empty-step fallback whose visible message has not been written
yet. The repository also exposes an atomic insert-if-absent claim for the
empty-step task-description fallback. A direct user-message write and this
claim take the same per-session write boundary, so the first committed
admission wins.

The counter does not decrease after message deletion. This property prevents a
deleted transcript row from making the task description eligible again.

The task repository exposes a bounded existence query for this state and the
atomic fallback claim. The existence of the counter row, including a
zero-valued reservation marker, is the history signal. The orchestrator does not
load the complete session transcript to make the fallback decision. The
replay-safe counter table has no foreign key, so session deletion explicitly
removes the counter before commit. This change needs no schema migration.

## Workflow-entry prompt flow

`launchAfterOnEnterDispatch` and `StartSessionForWorkflowStep` use one prompt
composition helper. The helper applies these rules:

1. If `WorkflowStep.Prompt` is non-empty, retain the task description for
   placeholder evaluation. Non-empty prompts, including `{{task_prompt}}`, keep
   their existing semantics.
2. If the step prompt is empty, atomically claim the initial fallback slot.
3. If the claim succeeds, use the task description as the base prompt.
4. If the claim is already taken, use an empty base prompt.
5. Build the workflow prompt with the existing workflow instructions and
   reference expansion.

The `on_enter` caller applies this result before it selects ACP or passthrough
delivery. Thus, both transports use the same fallback rule.

Before emptiness is decided, the explicit and automatic paths apply the same
plan-mode and session-configuration transforms that prompt dispatch applies.
The ACP path still lets `autoStartStepPrompt` merge a queued handoff. If the
merged result has no content or attachments, it returns without a message or
agent dispatch. An attachment-only handoff is admitted, persisted with its
attachment metadata, and dispatched even when its text is empty. A started
passthrough session drains the queued handoff before returning from a suppressed
empty-step decision.

For a `CREATED` session, `autoStartStepPrompt` records the merged prompt before
it calls `startCreatedSessionWithComposedPrompt`. The private launch path marks
the prompt as composed. As a result, `startCreatedSession` does not apply the
step prompt a second time. If a non-empty step prompt does not contain
`{{task_prompt}}`, this rule preserves the textual handoff.

The explicit workflow-step launch keeps its existing resume and session-setting
behavior. It does not call `PromptTask` when the composed prompt is empty.

## Failure and recovery

If internal cancellation fails, the provider session remains unchanged. The workflow entry records the reset error and does not send the automatic prompt.

If provider reset or configuration restoration fails, the existing reset reconciliation applies. The automatic prompt remains blocked.

If a stale predecessor barrier times out, the successor prompt fails before dispatch. The session guard becomes available for cancellation and recovery.

If the prompt-history read or atomic claim fails, prompt composition returns an
error. The automatic entry uses the existing waiting-state recovery. An
explicit workflow-step launch returns the error to its caller.

This repair does not reconcile sessions that became stuck before the new boundary existed. Users can replace such a session with a new session.

The prompt counter and fallback claim are durable across backend restarts. A
restart cannot make an earlier task description eligible for another fallback
dispatch, and deleting/recreating a session ID starts a new prompt boundary.

## Asynchronous launch prompt preservation

This extension belongs to the task system because workflow entry owns the pending input.
The [implementation package](../../../plans/workflow-async-start-prompt-preservation/plan.md)
delivers the design and its regression matrix.

### Capture before asynchronous admission

`autoStartStepPrompt` already separates recorded content, agent content, references, attachments, and completion handoff text.
Capture the queue-form input before `startCreatedSessionWithComposedPrompt` admits the launch.
Use a private, immutable attempt envelope in the orchestrator, carried through the launch context.
Existing executor context propagation preserves values through `context.WithoutCancel` and dynamic launch paths.
The executor need not interpret the prompt envelope or import workflow composition types.

The envelope includes task, session, workflow step, unique launch token, initial turn identity, and the queue-form input.
Bind the turn identity in `startCreatedSession` before `launchPreparedSessionWithDynamicFallback` starts asynchronous work.
Copy mutable slices and metadata. Preserve the actual `userMsgRecorded` result rather than assuming that transcript persistence succeeded.
Do not recover from the mutable `lastTurnPrompt` cache or select the latest transcript row.
Neither source binds all workflow metadata to the failed attempt.

An attempt-local completion claim permits one preservation operation.
Synchronous rejection retires the envelope and retains `handleCreatedAutoStartLaunchFailure` as its sole queue owner.
Success retires the envelope without queue insertion. Context lifetime bounds retention after the launch callback returns.
Dynamic fallback attempts must not independently preserve the same logical prompt while another candidate can still start.
Only the final accepted startup failure can consume the preservation claim.

### Preserve through the accepted failure path

Extend `handleAgentStartFailed` after its cancellation, terminal-session, and execution checks.
Under the existing cancellation guard, validate the captured turn and workflow entry before queue persistence.
An execution ID alone is insufficient when successive attempts reuse a prepared execution.
Reject superseded attempts, completed or cancelled sessions, archived tasks, and missing sessions.
Queue admission must retain the existing session-incarnation and purge-generation protections.
Do not treat the unconditional `onAgentProcessStartFailed` notification as evidence that a failure passed these guards.

Persist the envelope using the existing message-queue service and workflow metadata format.
Extract persistence from `queueAutoStartPrompt` so this call does not run `scheduleAutoResumeForWorkflowQueue`.
Preservation must not start a replacement before failure cleanup finishes.
Leave the user's `auto_run` policy unchanged.
After explicit recovery, existing boot-ready admission can drain the entry when all eligibility checks pass.

The queue contains the composed visible workflow input, with entity references and completion handoff in their existing metadata fields.
It must not contain a second injected Kandev system block or independently restored copy of an already-merged handoff.
`executeQueuedMessage` continues to honor `user_message_recorded` and compose dispatch context through the existing path.
The queue uses its existing storage, attachment ownership, capacity limits, transfer rules, and restart recovery.
No new table, public payload, or provider API is required.

### Failure and recovery boundaries

The original error continues through auth, managed-runtime, or generic bootstrap handling.
The [launch recovery design](task-launch-failure-recovery.md) remains authoritative for safe errors, stamps, history, and recovery authorization.
`RecoverTaskLaunch` already routes session-owned retry through `RecoverSession` on the same session.
Recovery boots the session and lets the existing queue drain send its preserved input.
Do not send the same prompt as both a launch description and a queue entry.

A queue write failure does not mask the original startup failure.
Log a separate bounded preservation diagnostic with task, session, execution, and attempt identity, without input text or attachment contents.
The existing persistent launch error remains the visible recovery signal.
No frontend layout or copy changes are required by this package.

The guarantee starts when a live failure callback accepts ownership and persists the queue entry.
A process crash before that point is excluded. Successful process startup is not proof of exactly-once provider execution after an ambiguous prompt error.
Post-start prompt failures keep their separate correlated terminal path and do not use this replay mechanism.

### Verification

Use controlled startup barriers to prove preservation after the launch call already returned success.
Cover duplicate callbacks, replacement execution, same-execution successor turn, terminal races, and dynamic fallback ownership.
Exercise real queue persistence and the service recovery-to-boot-ready-to-dispatch path.
Verify one transcript row, one delivered prompt, preserved metadata, paused queues, and restart after queue persistence.

## Observability

The workflow reset logs the task, session, workflow step, and execution identifiers. Internal cancellation uses the existing cancellation and escalation logs.

The bounded wait error names the session execution and the timeout. Existing prompt failure logs show that the successor did not reach agentctl.

## Related decisions

- [Quiesce active turns before context reset](../../../decisions/2026-08-30-context-reset-quiesces-active-turn.md)
- [Preserve ACP runtime configuration across context reset](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md)
- [Version AgentReady events by prompt generation](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

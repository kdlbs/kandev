---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
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
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002` | [Active-turn reset flow](#active-turn-reset-flow), [Bounded predecessor wait](#bounded-predecessor-wait) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003` | [Prompt fallback ownership](#prompt-fallback-ownership), [Prompt-history contract](#prompt-history-contract), [Workflow-entry prompt flow](#workflow-entry-prompt-flow) |
| `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004` | [Creation destination routing](#creation-destination-routing) |

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

Internal cancellation finishes or escalates the old generation before provider replacement. The reset does not drain a completion signal from an active generation.

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

## Observability

The workflow reset logs the task, session, workflow step, and execution identifiers. Internal cancellation uses the existing cancellation and escalation logs.

The bounded wait error names the session execution and the timeout. Existing prompt failure logs show that the successor did not reach agentctl.

## Related decisions

- [Quiesce active turns before context reset](../../../decisions/2026-08-30-context-reset-quiesces-active-turn.md)
- [Preserve ACP runtime configuration across context reset](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md)
- [Version AgentReady events by prompt generation](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

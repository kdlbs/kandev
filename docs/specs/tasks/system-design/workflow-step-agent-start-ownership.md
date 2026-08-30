---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
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

## Components and responsibilities

`orchestrator.Service.resetAgentContext` owns workflow reset semantics. It also owns the session reset marker, the session lifecycle lock, and the shared per-session cancellation guard that closes prompt-admission races.

The cancellation coordinator owns active-turn quiescence. It uses the internal cancellation path, not the explicit user cancellation path.

`lifecycle.Manager.ResetAgentContext` replaces the provider session after quiescence. It restores runtime configuration through the existing reset contract.

`lifecycle.SessionManager` owns prompt serialization and the dispatch-only completion barrier. Prompt generations continue to identify completion ownership.

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

## Failure and recovery

If internal cancellation fails, the provider session remains unchanged. The workflow entry records the reset error and does not send the automatic prompt.

If provider reset or configuration restoration fails, the existing reset reconciliation applies. The automatic prompt remains blocked.

If a stale predecessor barrier times out, the successor prompt fails before dispatch. The session guard becomes available for cancellation and recovery.

This repair does not reconcile sessions that became stuck before the new boundary existed. Users can replace such a session with a new session.

## Observability

The workflow reset logs the task, session, workflow step, and execution identifiers. Internal cancellation uses the existing cancellation and escalation logs.

The bounded wait error names the session execution and the timeout. Existing prompt failure logs show that the successor did not reach agentctl.

## Related decisions

- [Quiesce active turns before context reset](../../../decisions/2026-08-30-context-reset-quiesces-active-turn.md)
- [Preserve ACP runtime configuration across context reset](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md)
- [Version AgentReady events by prompt generation](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

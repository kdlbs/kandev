# ADR-2026-08-30-context-reset-quiesces-active-turn: Quiesce Active Turns Before Context Reset

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, workflow

## Context

A workflow transition can enter a step with `reset_agent_context` while the current session still owns an active turn.

The reset currently replaces the ACP session and drains a shared completion channel. The old turn can then finish against retired session state.

A later prompt waits for that lost completion while it holds the session dispatch guard. The same guard prevents explicit cancellation from reaching the lifecycle manager.

Context reset needs one owner for turn quiescence. The owner must preserve workflow cancellation semantics and prompt-generation ownership.

## Decision

A workflow context reset is a system-owned turn-replacement operation when the target session has an active turn.

The orchestrator takes the shared per-session cancellation guard before it sets the reset marker. It then claims the session's cancellation coordinator exclusively for an internal cancellation operation and uses the existing bounded path to stop and reconcile the active turn. The guard is released while that operation waits and reacquired before provider replacement. If another cancellation already owns the session, reset fails closed rather than inheriting that source's reconciliation semantics. A reset with no active turn also fails closed when a cancellation is already in flight.

This cancellation does not create a user-cancellation message. It does not evaluate `cancel_triggers_turn_complete` or `on_turn_complete`.

The lifecycle manager replaces the provider session only after internal cancellation finishes. Normal and lifecycle prompt claims perform their final marker check under the same guard. The reset marker prevents new prompt admission until provider state and persisted reset state are settled.

A successor that waits for an unresolved dispatch-only completion has a 10-second bound. Timeout returns a typed transient error without clearing the predecessor gate.

Existing deferred cleanup releases prompt serialization and the session dispatch guard. The normal cancellation escalation path remains the only owner that clears the stale gate.

Prompt generations remain the authority for terminal events. A completion from the replaced turn cannot finish a later generation. An unnumbered completion also cannot release a pending numbered dispatch-only prompt. Synthetic or delayed completion events without ownership identity are ignored at that boundary.

## Consequences

- A workflow reset can stop an old turn before it starts work in the destination step.
- Internal reset cancellation cannot trigger user-configured workflow completion.
- A cancellation escalation can add up to the existing bounded cancellation interval before provider reset.
- A stale predecessor wait fails before dispatch and leaves cancellation available.
- Existing sessions that already contain the stuck state do not receive automatic reconciliation.

## Alternatives Considered

- **Replace the provider session immediately.** Rejected because it can orphan the active completion signal and block all later session operations.
- **Wait for `agent.ready` before reset.** Rejected because workflow entry remains incomplete while the old turn can continue under the destination step.
- **Use explicit user cancellation.** Rejected because configured user cancellation can evaluate `on_turn_complete` and move the task again.
- **Clear the pending gate when its wait expires.** Rejected because the predecessor can still run and corrupt successor buffers or completion ownership.
- **Restart the agent process for every active reset.** Rejected because process replacement is slower and does not replace cancellation reconciliation.

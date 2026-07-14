# 0035: Version AgentReady Events by Prompt Generation

**Status:** accepted
**Date:** 2026-07-14
**Area:** backend

## Context

Cancel escalation must publish `agent.ready` outside the cancelling goroutine because the in-memory event bus invokes orchestrator subscribers synchronously and those subscribers acquire the same per-session cancel guard. A detached publish avoided that reentrant deadlock, but an unversioned event could be delayed until a replacement prompt was active and then complete the replacement turn. Passing `*AgentExecution` to the detached goroutine also let the eventual payload observe mutable replacement state.

## Decision

Lifecycle prompt ownership is identified by `(agent_execution_id, prompt_generation)`. The generation advances explicitly before every prompt dispatch attempt, including the initial prompt and replacements dispatched while the execution is already `RUNNING`; status transitions alone do not create prompt identities. Turn-ending ready events include both values, and lifecycle captures the complete ready payload atomically with the transition to `Ready` before any detached publication begins.

The generation is also part of the lifecycle-to-agentctl prompt request. The adapter echoes it on the terminal `complete` or prompt-error event instead of lifecycle reading the execution's current generation when that event eventually arrives. Lifecycle atomically compares the echoed generation with the active prompt before flushing buffers, signaling prompt completion, changing execution status, or publishing `agent.ready`. A terminal event for a superseded generation is discarded in full. Synthetic adapter turns that do not originate from a lifecycle prompt carry generation zero and retain their existing wakeup reconciliation path.

The orchestrator validates this identity against the lifecycle execution store while holding the session's `cancelInFlightGuard`, before turn completion, pending moves, or `on_turn_complete` evaluation. Backend adapters between the orchestrator and lifecycle manager must preserve this ownership-validation capability; a generation-bearing ready event that cannot prove current ownership is stale and is ignored. Ordinary `MarkReady` and `MarkBootReady` publication remains synchronous; only cancel escalation uses detached publication.

The same session guard may cover cancel plus replacement-prompt acceptance, but it must be released after acceptance. It must never remain held while waiting for that prompt's complete event.

## Consequences

Delayed cancel-escalation events and delayed ordinary prompt completions cannot be reassigned to replacement turns, including after an execution is replaced and generation numbers restart. Detached publishers carry immutable values instead of reading mutable execution state later. Prompt dispatch attempts, agentctl terminal events, and ready events now have a lifecycle-local generation contract that intermediaries and event consumers must preserve. A failed dispatch may consume a generation; generations identify ownership and are not a count of successful turns.

Guard holders may still wait for bounded cancellation and prompt-dispatch acknowledgement. Concurrent parent interrupts become eligible immediately after a clarification retry is accepted and can either interrupt that recovered turn or back off if they snapshotted the superseded generation.

## Alternatives Considered

Publishing `agent.ready` synchronously was rejected because synchronous subscribers re-enter the already-held session guard. Reading the active orchestrator turn only when the handler begins was rejected because a delayed event can see the replacement turn as both its baseline and current turn. Posting an unversioned reconciliation callback after guard release was rejected because it would create a second completion path with separate ownership and error handling.

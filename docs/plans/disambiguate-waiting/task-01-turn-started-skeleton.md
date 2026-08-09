---
task: 01
title: "turn_started stream event skeleton"
wave: 0
status: done
spec_acs: AC-41a, AC-41b, AC-79a
---

# Task 01 — `turn_started` stream event skeleton

Ship the new `EventTypeTurnStarted` event end-to-end as a no-op: the orchestrator
receives it but takes no action yet. Wave 0 seam — must land before task-05 (attestation)
and is the prerequisite for the correct `turn_marker` in task-07 (projection).

## Files to change

### 1. `internal/agentctl/types/streams/agent.go`

Add one constant alongside the existing `EventType*` block:

```go
EventTypeTurnStarted EventType = "turn_started"
```

Cross-check: trace `EventTypeForegroundIdle` to verify placement and format.

### 2. `internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`

Emit `turn_started` inside `sendPrompt`, at the `beginPromptTurn` call site (line 140).
The call is `syncNotifQueueThen(afterBarrier)` placed **after** `beginPromptTurn` has
returned (so `asyncTurnMu` is not held when the barrier awaits).

Both the turn-start stamp (writing whatever `turn_marker` field the agentctl side carries,
e.g. the prompt generation) **and** the emit go inside the `afterBarrier` callback — on
the update worker — so the stamp precedes `conn.Prompt` and cannot be overtaken by a
still-queued prior-turn attestation (AC-41b, AC-79a).

Pattern (adapt to exact adapter types):

```go
// After beginPromptTurn returns, before conn.Prompt goroutine starts:
a.syncNotifQueueThen(func() {
    a.sendUpdate(AgentEvent{
        Type:             streams.EventTypeTurnStarted,
        SessionID:        sessionID,
        PromptGeneration: promptGeneration, // may be 0 for wakeup (AC-41b note)
    })
})
```

`promptGeneration == 0` for `fireWakeup` callers — this is expected and correct.
`cancellationOwnsStreamEvent` rejects only on a mismatch of two *present* values, so 0
is never rejected (AC-41b).

Do not emit at `sendPrompt` entry or at the outer callers `Prompt`, `PromptSteer`,
`fireWakeup`. All three callers of `sendPrompt` are covered by the single emission point.

### 3. `internal/agent/runtime/lifecycle/manager_events.go`

Add a `case streams.EventTypeTurnStarted:` in the same switch that handles
`EventTypeForegroundIdle` (line 761). For Wave 0, no action needed — just fall through
to `PublishAgentStreamEvent`:

```go
case streams.EventTypeTurnStarted:
    // Relayed to orchestrator for turn-marker accounting (task-05).
```

`ExecutionID` is stamped by the relay infrastructure, not by agentctl. This is why the
relay step cannot be skipped — without it the event is never delivered (spec note).

### 4. `internal/orchestrator/event_handlers_streaming.go`

Add a `case streams.EventTypeTurnStarted:` in `handleAgentStreamEvent` (after the
existing cases). For Wave 0, log and return without action:

```go
case streams.EventTypeTurnStarted:
    // Turn-marker increment implemented in task-05.
    return
```

This is the no-op placeholder that makes the event pipeline complete and testable.

## Tests to write

- **Adapter test** (alongside existing `adapter_prompt_test.go` or similar): verify that
  a `sendPrompt` call produces an `EventTypeTurnStarted` event on `updatesCh` **before**
  any `EventTypeComplete`, and that the event appears before the prompt goroutine completes.
  Use the existing fake-agent and `updatesCh` drain pattern.

- **Relay test** (`manager_events_test.go`): verify that an injected `EventTypeTurnStarted`
  stream event is forwarded via `PublishAgentStreamEvent` to the event bus. Mirror the
  pattern for `EventTypeForegroundIdle`.

- **Completeness note**: `internal/agentctl/server/api/agent_test.go` enumerates known
  stream actions. `turn_started` is **not** a WS action (it's a stream event from
  agentctl→backend), so it does not appear in that list — no change needed there.

## Acceptance criteria closed

- **AC-41a**: `turn_started` event type defined and relayed.
- **AC-41b** (partial): emission is inside `syncNotifQueueThen` callback, stamped before
  `conn.Prompt`, covers all three `sendPrompt` callers. The full AC-41b closes when
  task-05 implements the turn-marker increment and task-07 uses it in the projection.
- **AC-79a** (no-unintended-overtake): the FIFO ordering guarantee is established by
  placing both stamp and emit in the worker callback.

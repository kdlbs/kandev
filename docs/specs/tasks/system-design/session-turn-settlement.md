---
status: draft
system: tasks
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-001
---

# Session turn settlement design

## Ownership and requirement mapping

Tasks owns session execution lifecycle. The lifecycle manager implements this
contract under `apps/backend/internal/agent/runtime/lifecycle/`.

| Requirement | Design boundary |
| --- | --- |
| REQ-TASKS-SESSION-TURN-SETTLEMENT-001 | Callback lease, completion flow, and regression coverage below |

## Callback lease

`AgentExecution.withStartupAttempt` holds `startupCallbackMu.RLock` across
generation validation, callback mutation, and event publication. It captures
the attempt ID while that lease protects the startup identity.

`BindResumeAttempt` takes the exclusive lock before the execution-store lock.
Startup replacement also takes the exclusive lock. Retain this ordering and
the outer lease throughout completion processing.

Code inside a leased callback must use helpers that do not acquire another
startup callback lease. A second read lock can deadlock when an exclusive
writer waits for the first read lock. A generation check without a lease does
not protect subsequent mutation or publication.

## Completion flow

`handleAgentEventWithStartupGeneration`, `handleAgentEvent`, and
`handleAgentEventAfterContextReset` acquire the outer lease. Direct completion
callers use `handleCompleteEvent`, which also acquires it. These entry points
pass the captured attempt ID into `handleCompleteEventLeased`.

`claimPromptCompletion` retains its current admission rules. For an eligible
generation-zero success, `finishPromptCompletion` signals the completion
channel before `handleCompleteEventMarkState` publishes readiness.

The state helper calls `markReadyEventForExecution` with the leased execution,
`events.AgentReady`, synchronous publication, and `event.AttemptID`. It must
not call the public `MarkReady` wrapper from inside the lease. Document that
the internal state helper requires a held lease; direct tests must establish
that precondition or enter through `handleCompleteEvent`.

The existing Ready-to-Running fallback for an empty synthetic turn remains.
It publishes Running before Ready. The readiness helper retains status
persistence and captures the event payload before publication.

Numbered completions retain their prompt-generation claim and captured payload.
Errors retain `markCompletedWithTurnIDAndAttempt`; foreground-idle and stream
disconnect paths retain their leased completion-signal helper.

## Compatibility and failure behavior

This is a shared lifecycle correction. It adds no provider-specific branch,
wire field, schema, runtime flag, or new event type. A callback with an obsolete
startup generation remains rejected before state changes. An unnumbered
completion cannot bypass `dispatchedPromptPending`.

Ordinary public `MarkReady` and `MarkBootReady` callers retain their lease
acquisition. Ordinary turn-end publication remains synchronous, as specified
by [ADR 0035](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md).

The correction prevents new deadlocks at this boundary. It does not release
locks in an already-deadlocked backend. Existing workspace-refresh latency and
idle-reaper policy remain separate concerns.

## Verification boundary

Exercise the actual dispatcher and completion helper with a pending
`BindResumeAttempt` writer. Prove writer contention before releasing the
completion barrier; elapsed sleep alone does not establish contention.

Use a child test process with a bounded timeout for the deadlock regression.
This allows the parent test to terminate the faulty process without leaking
blocked goroutines. On success, join all goroutines before reading shared
state. Assert Ready publication, originating attempt identity, writer progress,
and subsequent prompt admission.

Cover both Running and already-Ready synthetic completions. Retain stale
startup, stale prompt, pending-dispatch, error, and foreground-idle cases.

## Implementation plans

- [Completion callback lease repair](../../../plans/completion-callback-lease/plan.md).

---
status: current
system: tasks
requirements:
  - REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001
---

# Passthrough Queued Prompt Dispatch System Design

## Purpose and boundaries

The task system owns the per-session serialization guard, queue reservation,
turn completion, and successor-turn state transition. The agent runtime owns
the passthrough execution status and lifecycle event payload. The in-memory
event bus remains synchronous because existing consumers depend on ordered
delivery.

This design covers the ordinary queued-prompt drain from `agent.ready`. Durable
lifecycle prompts already leave the ready handler's guard before their normal
dispatch pipeline begins.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `orchestrator.Service.handleAgentReady` owns turn completion, the
  per-session guard, ordinary queue reservation, and PTY dispatch ordering.
- `orchestrator.Service.deliverPassthroughPrompt` owns prompt chunk planning
  and PTY writes.
- `lifecycle.Manager` owns the transition of an `AgentExecution` to
  `AgentStatusRunning` and construction of the corresponding immutable
  `agent.running` payload.
- `backendapp.lifecycleAdapter` exposes the lifecycle transition through the
  orchestrator's narrow runtime boundary.
- `MemoryEventBus` continues to invoke the watcher subscriber synchronously.
  `handleAgentRunning` owns the task-session running transition and
  `on_turn_start` processing.

## Data and contracts

The lifecycle boundary provides a two-phase passthrough-running operation for
callers that already hold a session serialization guard:

1. Under the execution-store lock, revalidate the current session and
   passthrough execution, claim the Ready-to-Running transition, and capture an
   immutable `agent.running` payload.
2. Return a one-shot publication callback that emits the captured payload.

The existing immediate `MarkPassthroughRunning` operation uses the same
primitive and invokes the callback before returning. Existing terminal-input
and unguarded prompt paths therefore retain synchronous publication.

The orchestrator uses the two-phase form only for the ordinary passthrough
ready drain. Test doubles that do not publish lifecycle events can retain the
immediate marker contract.

## Control flow

1. `handleAgentReady` acquires the session guard, completes the current turn,
   and reserves one eligible ordinary queued prompt.
2. The orchestrator asks the lifecycle adapter to mark the execution running.
   Lifecycle updates runtime status and snapshots the event payload without
   publishing it.
3. While the guard is still held, the orchestrator writes all planned prompt
   chunks to the PTY. This preserves the existing exclusion against a
   concurrent cancel, manual drain, or prompt dispatch.
4. The ready handler releases the session guard.
5. The ready handler invokes the deferred publication callback synchronously.
   `MemoryEventBus` calls `handleAgentRunning`, which can now acquire the free
   guard, run `on_turn_start`, and project the session as running.

Publication is deferred by lock scope, not by a detached goroutine. The event
therefore retains deterministic ordering relative to the ready handler's
completion, and there is no unbounded publisher lifetime to manage.

## Failure and recovery

- In the two-phase ready-event path, a runtime-status transition failure
  prevents the PTY write. The existing immediate `deliverPassthroughPrompt`
  contract remains non-fatal for `MarkPassthroughRunning` failures.
- If a PTY write fails after the execution was marked running, the deferred
  callback still publishes after guard release. This preserves the existing
  status/event pairing and allows ordinary recovery paths to observe the
  runtime transition.
- Concurrent preparation calls observe the same locked transition, so at most
  one call claims Ready-to-Running and creates a publication.
- The publication callback uses an immutable payload captured at the status
  transition. Later execution mutation cannot relabel the event.
- The event bus remains synchronous. A handler error is logged through the
  existing bus behavior and does not reintroduce guard re-entry.

## Observability

The permanent regression test reproduces a synchronous running-event callback
from the passthrough marker. The test proves that the ready handler returns and
the PTY write occurs once. It also proves that the queue entry is consumed once
and the session guard is available afterward. Existing lifecycle tests prove
that immediate callers still publish once. These tests also prove that the
deferred callback carries the captured payload.

## Related decisions

- [Defer passthrough running publication until guard release](../../../decisions/2026-08-31-passthrough-running-publication.md)
- [Version AgentReady events by prompt generation](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)

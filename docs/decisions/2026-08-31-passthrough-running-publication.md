# ADR-2026-08-31-passthrough-running-publication: Defer Passthrough Running Publication Until Guard Release

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend

## Context

The in-memory event bus invokes subscribers synchronously to preserve event
ordering. An ordinary queued prompt for a passthrough session is selected while
`handleAgentReady` holds the session's cancellation and dispatch guard.
`MarkPassthroughRunning` updates runtime status and synchronously publishes
`agent.running`. The subscriber re-enters `handleAgentRunning` and tries to
acquire the same non-reentrant guard. The publishing goroutine then blocks on
its own lock indefinitely.

The guard cannot simply be removed from the ready drain. It serializes queue
selection and prompt acceptance against cancellation, manual drains, and other
turn-start paths. ADR-0035 established the related rule that an event whose
synchronous subscriber re-enters this guard must not be published while the
guard is held.

## Decision

The lifecycle manager separates the passthrough running transition from its
event publication for guarded callers. It updates `AgentExecution` status and
captures an immutable `agent.running` payload under the existing runtime
boundary, then returns a one-shot publication callback.

The ordinary passthrough ready drain marks the execution and writes the prompt
while holding the session guard. It invokes the publication callback
synchronously only after releasing that guard. Immediate callers continue to
use `MarkPassthroughRunning`, which performs both phases before returning.

The event bus remains synchronous. Publication moves across the lock boundary.
It does not move to an unowned goroutine.

## Consequences

The ready drain retains exactly-once queue selection and exclusion from
concurrent cancellation or prompt delivery. The running-event subscriber can
acquire the session guard, so the session does not deadlock and later lifecycle
operations remain responsive.

The lifecycle manager must atomically claim the runtime transition and snapshot
event data under the execution-store lock before it returns the deferred
callback. A caller must invoke the callback exactly once after it releases its
guard. This rule also applies when a later PTY write fails after the runtime
status transition.

## Alternatives Considered

- Make the entire in-memory event bus asynchronous. This would change ordering
  for every subscriber and create broader concurrency and shutdown semantics.
- Release and reacquire the session guard around the current synchronous
  marker. A queued cancellation could win that gap and the original dispatch
  could then write to a cancelled or replaced process.
- Publish from a detached goroutine. This avoids direct re-entry but weakens
  ordering, requires goroutine ownership, and needs stale-event protection for
  an avoidable scheduling delay.
- Skip `agent.running` for queued passthrough prompts. That would omit
  `on_turn_start` processing and leave persisted task-session state out of sync
  with the runtime.

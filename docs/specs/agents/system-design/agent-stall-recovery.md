---
status: draft
system: agents
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
created: 2026-09-02
owners:
  - Kandev
---

# Agent Stall Recovery System Design

## Purpose and boundaries

The agent system owns the prompt watchdog: the clock that decides a prompt is
inactive, the classification of that inactivity as advisory or never-started,
and the teardown of an execution whose prompt never started.

It does not own the persisted session and task state machine, the notice
rendering, or the task-scoped stop path. It calls the orchestrator's existing
launch-failure transition, reuses the existing message creator, and leaves
reaching an execution from a task ID to
[task stop reachability](../../tasks/system-design/task-stop-reachability.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-AGENT-STALL-RECOVERY-001` | [Prompt progress clock](#prompt-progress-clock), [Watchdog classification](#watchdog-classification), [Terminal teardown](#terminal-teardown), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **`AgentExecution` activity state** (`internal/agent/runtime/lifecycle`) holds
  the per-prompt progress timestamp, the never-started discriminator, and the
  activity epoch under one mutex.
- **The lifecycle event recorder** classifies each inbound agent frame as a turn
  event or a metadata frame and advances progress state only for turn events.
- **The prompt watchdog** in the session manager ticks once per minute, takes a
  consistent snapshot, and publishes at most one stall event per prompt
  generation.
- **The orchestrator stall handler** validates the snapshot's ownership,
  persists the notice, and, for a never-started prompt, applies the terminal
  transition and requests execution teardown.
- **The agent manager's stop entry point** performs teardown. It stops the
  runtime and deregisters the execution without writing session state.

## Data and contracts

The published stall payload is unchanged: task, session, and execution
identity, prompt generation, last-activity time, elapsed duration, activity
epoch, optional active-tool identity, and the `NeverStarted` discriminator.

The prompt progress timestamp is the single clock behind that payload. Exactly
four inputs advance it:

| Input | Meaning |
| --- | --- |
| Prompt dispatch | A new prompt is armed; the clock restarts and the discriminator is cleared |
| Turn event | Assistant text, reasoning, tool call, tool update, plan, or permission request |
| Terminal completion | The prompt's own completion or error frame |
| User steer | New user input delivered into the running turn |

A metadata frame advances nothing. It is still processed, published, and
recorded for its own purposes; it is not evidence of progress.

The discriminator remains separate from the clock. Prompt dispatch clears it;
a turn event or terminal completion sets it. A user steer restarts the clock
without setting it, because user input is not agent output.

## Control flow

1. Prompt dispatch arms the clock and clears the discriminator.
2. Each inbound frame is classified. A turn event advances the clock, sets the
   discriminator, and bumps the activity epoch. A metadata frame does neither.
3. Every minute the watchdog snapshots the clock, the discriminator, and the
   epoch together. Once the elapsed time reaches five minutes it publishes one
   stall event for the prompt generation and does not publish again.
4. The orchestrator rejects the event when the session is no longer running,
   when cancellation is in flight, or when the execution, prompt generation, or
   activity epoch is no longer current.
5. An accepted advisory event persists the neutral running notice and stops.
6. An accepted never-started event persists the terminal error notice, applies
   the launch-failure transition, and then requests execution teardown.

### Prompt progress clock

Because the clock advances only on the four inputs above, an adapter that emits
metadata frames while producing no turn output cannot postpone detection. The
never-started elapsed time is measured from prompt dispatch, which is the
behavior the requirement describes.

The same clock also feeds the ten-minute completion-signal watchdog. That
consumer inherits the corrected semantics: a signalled session that has gone
quiet apart from metadata frames becomes reclaimable on schedule instead of
never. No separate clock is introduced for it.

### Watchdog classification

Classification is a pure read of the snapshot. An advisory stall leaves task
state, session state, prompt admission, and process liveness untouched. A
never-started stall is terminal.

### Terminal teardown

Terminal handling is ordered: record the failure first, then tear down.

Recording first makes the durable outcome the launch-failure state the user
sees, and keeps it authoritative if teardown fails or races. Teardown then uses
the execution-scoped stop, which halts the runtime and deregisters the
execution without touching session rows, so it cannot replace the recorded
`FAILED` state with a cancellation state.

Teardown is requested with force, because a prompt that produced no frame is
not expected to answer a graceful protocol stop.

## Failure and recovery

- Teardown failure is logged with the session and execution identity. The
  session and task remain `FAILED`; the execution stays registered so a later
  task-scoped stop or startup cleanup can retry it.
- A late turn event that lands between the snapshot and the handler moves the
  activity epoch and the event is rejected, so a prompt that started late is not
  failed.
- Notice persistence failure is logged and does not block the terminal
  transition or the teardown.
- An already-removed execution reports not-found on teardown. That is the
  success condition, not an error, because no process remains.

## Persistence

No schema change. The clock, discriminator, and epoch stay in memory for the
lifetime of the execution. The durable record is the existing session state,
session error message, task state, and notice message.

## Security

The notice copy and the log fields keep their existing bounds: identifiers,
durations, and tool display names only. Teardown adds no new user input path
and no new externally reachable operation.

## Observability

The existing single stall log per prompt generation is retained and keeps its
`never_started` field. Terminal handling adds one log line for the teardown
result, carrying task, session, and execution identity plus the outcome, so an
orphan left behind by a failed teardown is greppable.

## Related decisions

- [ADR-2026-07-29-agent-stall-user-controlled-recovery](../../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)
- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)
- [ADR-2026-09-02-terminal-stall-owns-process-teardown](../../../decisions/2026-09-02-terminal-stall-owns-process-teardown.md)

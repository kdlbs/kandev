# ADR-2026-09-02-terminal-stall-owns-process-teardown: A Terminal Stall Owns Process Teardown, and the Execution Registry Is the Stop Authority

**Status:** accepted
**Date:** 2026-09-02
**Area:** backend

## Context

[ADR-2026-08-18-never-started-agent-stall-terminal](2026-08-18-never-started-agent-stall-terminal.md)
made a zero-event prompt terminal: the session and task move to `FAILED`. It
decided the classification but not the disposal. The implementation records the
failure and returns, so the agent process keeps running.

That leaves a live process attached to a session the database calls terminal.
The task-scoped stop resolves the sessions it should halt from a query whose
active-state set excludes terminal states, so it cannot see that session and
reports that no execution exists. The session-scoped stop, which resolves the
execution directly from the in-memory registry, still works. A live execution is
therefore unreachable through the operation a user actually reaches for.

Separately, the inactivity clock behind the classification advanced on every
inbound agent frame, including metadata frames such as usage and context-window
updates that a booting or hung adapter emits without doing any work. An adapter
in that state postponed its own detection indefinitely.

## Decision

Three rules, which are one rule seen from three sides: a record of terminal work
must eventually correspond to no running process. Rule 2's teardown is
detached and force-bounded at 30 seconds (`neverStartedStopTimeout`), so a
window exists between the `FAILED` write and the process actually exiting; a
retried or later stop (Rule 3) is what closes that window if the bounded
attempt itself fails.

1. **The inactivity clock measures progress, not traffic.** Only prompt
   dispatch, a turn event, the prompt's terminal completion, or new user input
   advances it. A metadata frame does not. The same clock serves the
   completion-signal watchdog, which inherits the corrected semantics.
2. **A terminal stall classification owns teardown.** The never-started path
   records the launch failure first, then stops the execution with force
   through the execution-scoped stop, which does not write session state. A
   failed teardown is logged and leaves the execution registered for a later
   attempt; it never downgrades the recorded `FAILED` state.
3. **The execution registry is the authority for a live execution that
   persisted session state alone cannot vouch for.** Active-state sessions
   remain a valid stop source in their own right; the registry is what the
   task-scoped stop also consults so a session the database calls terminal is
   not treated as unstoppable. It resolves the union of both. A session
   recovered only through the registry is stopped without a state transition, so
   its terminal state and error message survive.

## Consequences

A never-started stall disposes of its own process, so the orphan this ADR
describes is not created in the first place. Rule 3 is still required: it is the
recovery for an orphan created by any other path, and it is what makes the stop
operation honest rather than dependent on the session looking healthy.

The advisory notice now appears for a turn that emits only metadata frames. That
notice is non-destructive, and the situation it describes, waiting with no
visible progress, is exactly what it is for.

The task-scoped stop gains a read of the in-memory registry, so the task system
depends on one more read-only agent-manager capability. Stopping a task whose
only live work was an orphan now succeeds, and the caller's existing post-stop
task handling runs.

The correction does not make an unbounded ACP prompt call bounded. It bounds the
consequences: silence is now reported on time, disposed of, and recoverable.

## Alternatives Considered

- **Widen the active-state SQL set to include `FAILED` for the stop path.**
  Rejected. It answers "which sessions look stoppable" with a state list that
  would need to grow for every terminal state, and it still consults the wrong
  authority: the process is registered in memory, not in the state column.
- **Keep a second clock only for the never-started case.** Rejected. Two clocks
  over the same events invite the two answers to diverge, and the general clock
  would stay dishonest for its other consumer.
- **Tear down first, then record the failure.** Rejected. Teardown can publish
  lifecycle activity, and a crash or error between the two steps would leave a
  stopped process with no recorded explanation. Recording first keeps the
  durable outcome authoritative.
- **Have the stall handler call the task-scoped stop.** Rejected. That path
  writes a cancellation state and would overwrite the launch-failure state and
  message the requirement asks users to see.

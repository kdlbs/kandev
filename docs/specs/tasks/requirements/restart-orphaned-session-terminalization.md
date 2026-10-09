---
status: draft
system: tasks
created: 2026-09-20
owners:
  - kandev
---

# Session recovery after backend interruption

## Overview

A backend restart can interrupt an agent turn. The conversation remains available
for recovery. Opening Kandev must not cancel interrupted tasks or resume every
agent at once. Focusing a task restores its selected conversation. A new user
message continues the work without replaying the interrupted prompt.

## Requirements

### REQ-TASKS-RESTART-ORPHAN-SESSIONS-002: Interrupted conversations remain resumable

This requirement supersedes requirement 001's automatic cancellation policy.

- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.1:** After a backend interruption,
  an unarchived session that lost its execution shall remain recoverable.
  Reconciliation shall represent it as waiting for recovery, never cancelled
  solely because its execution is absent or its activity is old.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.2:** Opening Kandev shall not start
  all interrupted agents. Focusing an eligible task shall restore its selected
  agent and conversation under the existing auto-start preference.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.3:** Focusing a task shall not send
  or replay a prompt. A subsequent user message shall continue that conversation
  through normal prompt delivery, including during recovery startup.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.4:** Recovery shall preserve session,
  workspace, transcript, and available provider resume identity. Missing runtime
  records shall not cause cancellation or silent conversation replacement.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.5:** Surviving executions, recent
  launches, newer turns, and healthy siblings shall remain untouched. Unknown
  liveness or failed reads shall defer reconciliation.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.6:** Settling an interrupted turn
  shall not complete the task, advance its workflow, or report successful work.
  The session shall remain available for the next message.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.7:** Explicit stops, archive rules,
  authorization, capacity admission, and deferred-launch ownership shall retain
  their existing authority. Neither sweep shall override these rules.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-002.8:** Desktop and phone shall support
  focus, recovery, and follow-up messaging without an orphan-cancellation banner.
  Actual recovery failures shall retain explicit retry actions and stored history.

## Interruption visibility

Interrupted tasks retain the [interruption warning](interrupted-task-indicator.md)
until successful agent recovery. The warning uses the shared triangle and
warning color on sidebar rows and task cards, including phone surfaces.
Focus or an unsuccessful resume attempt alone does not clear it.

## Historical requirement

Requirement 001 and its criteria below record the superseded cancellation policy.
They must not drive new implementation. The pending
[recovery package](../../../plans/orphaned-session-open-recovery/plan.md)
replaces that policy with requirement 002.

### REQ-TASKS-RESTART-ORPHAN-SESSIONS-001: Sessions orphaned by a backend restart reach a terminal state

**Intent:** A backend restart must not leave task sessions stuck in an active
state forever. The system must self-heal: detect sessions whose backing actor
died and terminalize them within a bounded time.

**User story:** As a Kandev user, I want a session that lost its agent process
to a backend restart to show as stopped rather than stay "running" forever, so
that the task board stays truthful and the task can move on.

#### Acceptance criteria

- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.1:** When a backend process
  terminates while a task session is in `STARTING` or `RUNNING`, and no live
  execution backs that session, the system shall transition that session to
  `CANCELLED` within one reconciliation sweep after the launch grace window
  elapses, without any user action.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.2:** When a session in `STARTING` or
  `RUNNING` is backed by a live execution in the in-memory store, the system
  shall leave its state untouched, regardless of how stale its row is.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.3:** When a session's last write is
  more recent than the launch grace window, the system shall not consider it
  orphaned, so an in-flight launch whose execution has not yet reached the
  in-memory store is never reaped.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.4:** When the sweep terminalizes a
  session, the system shall record a cancellation reason that names the
  backend restart as the source, distinct from archive cancellations and user
  stops, so session history keeps the source visible.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.5:** When the sweep terminalizes a
  session, the system shall run the same post-cancellation effects as an
  archive cancellation: expire terminal clarifications, clear parked
  projections, release session-ceiling reservations, and publish a
  `session.state_changed` event for each session actually cancelled.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.6:** When the sweep cannot consult
  the in-memory execution store, it shall remain inert rather than
  terminalize sessions it cannot prove unbacked.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.7:** When a cancellation write fails,
  the system shall retry it on the next sweep tick, and shall never give up on
  a session for as long as the process runs.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.8:** When the sweep terminalizes one
  session of a task, the system shall leave the task's other sessions
  untouched, so healthy siblings (for example `WAITING_FOR_INPUT`) survive.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.9:** When a session row is refreshed
  between the sweep's candidate read and its cancellation write (an in-flight
  launch CAS-writing `STARTING` before it registers an execution), the system
  shall not terminalize that session; the staleness cutoff is re-asserted at
  write time inside the same statement as the transition.
- **AC-TASKS-RESTART-ORPHAN-SESSIONS-001.10:** When the sweep publishes a
  session cancellation event, the event shall carry the session's durable
  `is_primary` flag, so the status-summary projection keeps the durable
  primary-session assignment.

## Historical exclusions

- The sweep does not resume or replay interrupted turns. Later task opening
  follows the [orphan recovery contract](session-stall-visibility.md).
- Detecting orphans of already-archived tasks; the archived-task
  reconciliation pass owns those.
- Agent-side heartbeats or adapter-side turn-state persistence.
- Stopping a live agent process; the sweep only terminalizes DB rows of
  sessions with no live execution.

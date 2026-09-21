---
status: draft
system: tasks
created: 2026-09-20
owners:
  - kandev
---

# Restart-Orphaned Session Terminalization Requirements

## Overview

A task session records the lifecycle of one agent conversation turn. When the
backend process dies while a session's turn is open — a graceful restart, a
crash, or a kill — the component that would have transitioned the session to a
terminal state dies with it. No other actor retries that transition, so the
session row stays in an active state forever and the task appears to wait for
an agent that already finished.

The tasks system owns the task-session lifecycle, so it owns this
reconciliation: the system itself must detect and terminalize sessions that
lost their backing actor, without depending on the dead process, the agent, or
a human SQL edit.

## Terminology

- **Active session state:** A `task_sessions.state` value that implies work is
  in flight: `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`.
- **Terminal session state:** A state a session never leaves: `COMPLETED`,
  `FAILED`, or `CANCELLED`.
- **Orphaned session:** An active-state session whose transition actor is gone:
  the backend that would complete or fail its turn terminated, and no live
  agent execution backs the session anymore.
- **Live execution:** An agent execution the current backend process tracks in
  its in-memory execution store. A session backed by a live execution is not
  orphaned even when its row is stale.
- **Launch grace window:** The interval between a session row entering
  `STARTING` and its execution necessarily being visible in the in-memory
  store. A session inside this window is not eligible for orphan detection.
- **Reconciliation sweep:** The periodic pass that lists candidate sessions
  and terminalizes the orphaned ones.

## Requirements

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

## Out of scope

- Resuming or replaying a turn that was interrupted mid-flight; the orphaned
  session is terminalized, not resumed.
- Detecting orphans of already-archived tasks; the archived-task
  reconciliation pass owns those.
- Agent-side heartbeats or adapter-side turn-state persistence.
- Stopping a live agent process; the sweep only terminalizes DB rows of
  sessions with no live execution.

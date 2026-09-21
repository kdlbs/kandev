---
status: active
system: tasks
created: 2026-09-20
owners:
  - kandev
---

# Session Stall Visibility and Orphan Healing

## Overview

An unarchived task whose session is in an active DB state (CREATED, STARTING,
RUNNING, WAITING_FOR_INPUT) but whose backing execution is gone — after a
backend restart, a lost executor, or a launch that never registered — is
invisible to every event-driven surface: no request path owns the session,
no actor will ever advance it, and silence produces no signal. This document
defines the observable contract for detecting that state and healing it,
closing the incident where four finished review tasks sat silently in
`WAITING_FOR_INPUT` for hours with no operator-visible signal.

## Terms

- **Active session**: a `task_sessions` row in CREATED, STARTING, RUNNING,
  or WAITING_FOR_INPUT.
- **Live execution**: an execution registered in the backend's in-memory
  execution store for the session.
- **Event silence**: no persisted activity for the session — the newer of the
  session row's last update and its newest persisted message.
- **Stall episode**: the continuous period during which one session is
  active, execution-less, and event-silent past the stall threshold. An
  episode ends when the session leaves that state (terminal transition,
  healing, or a live execution reappearing).

## Requirements

### REQ-TASKS-SESSION-STALL-VISIBILITY-001: Stall detection and orphan healing

**Intent:** A task whose session looks active but has no live backing
execution is surfaced promptly and eventually healed, instead of stalling
silently forever.

#### Acceptance criteria

- **AC-TASKS-SESSION-STALL-VISIBILITY-001.1:** When an unarchived task holds
  an active session with no live execution and no persisted session activity
  for longer than the configurable stall threshold (default 2h), the system
  shall emit a `task.stalled` event and a warning log naming the task and
  its stalled sessions, at most once per session per stall episode.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.2:** When a session gains a live
  execution, transitions to a terminal state, or otherwise shows persisted
  activity again, its stall episode shall end, and a later stall of the same
  session shall be reported again as a new episode.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.3:** When an active session with
  no live execution has been silent for at least twice the stall threshold,
  and every active session of the same task is equally orphaned and silent,
  the system shall cancel those sessions with the reason `orphaned session`
  and publish the session-state change event clients observe.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.4:** The healing cancellation
  shall never cancel a session with a live execution, including one that
  registered while the sweep was evaluating the task, and shall never cancel
  a session outside the set the sweep classified as orphaned.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.5:** When a session's activity
  clock cannot be read (for example a transient database read failure), the
  system shall neither report a stall nor heal for that evaluation, rather
  than classifying silence from an incomplete clock.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.6:** When the stall detection
  threshold cannot be verified against the live-execution registry, the
  sweep shall do nothing rather than guess that executions are gone.

## Exclusions

- Detecting stalls of sessions that still have a live execution: the
  agent-runtime stall watchdog owns that case.
- Healing archived tasks' sessions: the archived-session reconciliation pass
  owns that case.
- Front-end presentation of `task.stalled`: any dashboard or notification UI
  built on the event is a separate initiative.

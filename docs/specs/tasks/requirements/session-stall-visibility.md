---
status: draft
system: tasks
created: 2026-09-20
owners:
  - kandev
---

# Session Stall Visibility and Orphan Healing

## Overview

Sessions with unfinished work can lose their execution after a restart or
executor failure. The system detects these stalls and settles the abandoned work.
A conversation waiting for user input can also have no execution after normal
idle cleanup. That condition is not a stall.

Opening a recoverable conversation restores its agent without requiring a
manual Resume action. The task system owns both classification and recovery.
The September 21 revision is implemented in the
[recovery plan](../../../plans/orphaned-session-open-recovery/plan.md).

## Terms

- **Active session**: a `task_sessions` row in CREATED, STARTING, RUNNING,
  or WAITING_FOR_INPUT.
- **Live execution**: an execution registered in the backend's in-memory
  execution store for the session.
- **Idle conversation**: a session waiting for user input with no unfinished
  turn. Its execution can be absent after idle cleanup or restart.
- **Stall candidate**: an active session that is not an idle conversation.
- **Event silence**: no persisted activity for the session — the newer of the
  session row's last update and its newest persisted message.
- **Stall episode**: the continuous period during which one session is
  a stall candidate, execution-less, and event-silent past the stall threshold. An
  episode ends when the session leaves that state (terminal transition,
  healing, or a live execution reappearing).

## Requirements

### REQ-TASKS-SESSION-STALL-VISIBILITY-001: Stall detection and orphan healing

**Intent:** A task whose session looks active but has no live backing
execution is surfaced promptly and eventually healed, instead of stalling
silently forever.

#### Acceptance criteria

- **AC-TASKS-SESSION-STALL-VISIBILITY-001.1:** When an unarchived task holds
  a stall candidate with no live execution and no persisted session activity
  for longer than the configurable stall threshold (default 2h), the system
  shall emit a `task.stalled` event and a warning log naming the task and
  its stalled sessions, at most once per session per stall episode.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.2:** When a session gains a live
  execution, transitions to a terminal state, or otherwise shows persisted
  activity again, its stall episode shall end, and a later stall of the same
  session shall be reported again as a new episode.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.3:** Superseded by
  `REQ-TASKS-RESTART-ORPHAN-SESSIONS-002`. Execution loss and prolonged silence
  shall lead to recoverable interruption settlement, not automatic cancellation.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.4:** The healing transition shall
  never alter a session with a live execution, including one that registered
  while the sweep was evaluating the task, and shall never alter a session
  outside the set the sweep classified as orphaned.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.5:** When a session's activity
  clock cannot be read (for example a transient database read failure), the
  system shall neither report a stall nor heal for that evaluation, rather
  than classifying silence from an incomplete clock.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.6:** When the stall detection
  threshold cannot be verified against the live-execution registry, the
  sweep shall do nothing rather than guess that executions are gone.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.7:** The system shall preserve idle
  conversations regardless of silence duration or execution absence. It shall
  not report them as stalled or cancel them as orphaned. If unfinished-turn
  evidence is unavailable, classification shall wait for a later evaluation.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.8:** When a user opens a recoverable
  orphan-cancelled conversation, the system shall automatically restore the agent
  in that session. This applies to existing cancellations and both reconciliation
  passes. Recovery shall preserve conversation history and workspace identity.
  It shall not replay a prompt, create a replacement session, or advance the workflow.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.9:** Automatic recovery shall honor
  auto-start prevention, archive status, authorization, capacity, and deferred
  launch ownership. Explicit user stops and unrelated terminal states shall
  retain their existing recovery rules. Repeated opening shall not duplicate execution.
- **AC-TASKS-SESSION-STALL-VISIBILITY-001.10:** Desktop and phone task views
  shall use normal recovery progress and conversation controls. Successful recovery
  shall remove the orphan warning. If recovery fails, existing recovery actions
  shall remain available without an automatic retry loop.

## Related contracts

- [Session-open recovery](queued-session-ownership.md)
- [Auto-start preference](prevent-agent-autostart-on-open.md)
- [Restart reconciliation](restart-orphaned-session-terminalization.md)
- [System design](../system-design/session-stall-visibility.md)

## Exclusions

- Detecting stalls of sessions that still have a live execution: the
  agent-runtime stall watchdog owns that case.
- Healing archived tasks' sessions: the archived-session reconciliation pass
  owns that case.
- Front-end presentation of `task.stalled`: any dashboard or notification UI
  built on the event is a separate initiative.

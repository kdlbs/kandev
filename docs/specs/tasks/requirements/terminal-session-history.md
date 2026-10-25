---
status: active
system: tasks
created: 2026-10-25
owners:
  - kandev
---

# Terminal Helper Session History Requirements

## Overview

Task owners need to remove completed helper sessions from active session lists
without losing their conversation evidence. Disposable sessions without
retained evidence can be deleted. The task system owns session lifecycle and
transcript retention; desktop, kanban preview, and mobile expose the same
history outcome.

## Requirements

### REQ-TASKS-TERMINAL-SESSION-HISTORY-001: Retain and reopen terminal helper history

**Intent:** Keep useful completed helper transcripts available while allowing
owners to clean up disposable sessions.

#### Acceptance criteria

- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.1:** When a cleanup request targets a session, the system shall authorize it against the caller's current primary session, task, workspace, and target identity.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.2:** The system shall allow cleanup only for a terminal, non-primary target; it shall preserve active sessions, primary sessions, and sessions that are the sole retained evidence for the task.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.3:** When a terminal helper has retained conversation evidence, cleanup shall archive its session and preserve that evidence with an explicit archive time and a durable cleanup receipt.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.4:** When a terminal helper has no retained conversation evidence, cleanup may hard-delete its disposable session state and shall record the cleanup outcome.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.5:** When a cleanup operation is retried after commit, the system shall return the original task-scoped receipt without repeating destructive effects.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.6:** Archived sessions shall not be eligible for primary selection, resume, or new prompt admission.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.7:** Desktop, kanban preview, and mobile shall hide archived sessions by default and let the user explicitly reveal and reopen their preserved conversation history.
- **AC-TASKS-TERMINAL-SESSION-HISTORY-001.8:** History controls shall have localized accessible names and remain touch-accessible on mobile.

## Out of scope

- Stopping or settling a live session as part of cleanup.
- Deleting task-owned workspace files, repositories, or branches.
- Restoring an archived session to an active or primary state.

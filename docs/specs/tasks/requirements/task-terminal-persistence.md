---
status: active
system: tasks
created: 2026-09-04
owners:
  - kandev
---

# Task terminal persistence and ownership requirements

## Overview

Ordinary task terminals are durable task-scoped resources. The task system owns
their identity and lifecycle. Each record belongs to one task and its execution
environment. These operations must remain available on every supported
persistence backend.

Different tasks can share one execution environment. Sharing that environment
does not transfer ownership of an ordinary terminal between tasks.

## Requirements

### REQ-TASKS-TASK-TERMINALS-001: Persistence backend parity

**Intent:** Users can create and manage ordinary task terminals without the
configured supported persistence backend changing the terminal lifecycle.

**User story:** As a Kandev user, I want task terminals to work on each supported
database.

#### Acceptance criteria

- **AC-TASKS-TASK-TERMINALS-001.1:** On SQLite and PostgreSQL, the system shall
  persist and return each new ordinary task terminal with its sequence and open
  state.
- **AC-TASKS-TASK-TERMINALS-001.2:** On SQLite and PostgreSQL, the system shall
  list matching task terminals in sequence order and apply the requested parked
  terminal filter.
- **AC-TASKS-TASK-TERMINALS-001.3:** On SQLite and PostgreSQL, each read, rename,
  state change, single removal, and task-wide removal shall produce the same
  persisted result.

### REQ-TASKS-TASK-TERMINALS-002: Task ownership during terminal use

**Intent:** Each task retains its own ordinary terminals when tasks share an
environment. The task terminal remains usable after navigation and restoration.

**User story:** As a user, I want each task to open its own terminal without
selecting a terminal from another task.

#### Acceptance criteria

- **AC-TASKS-TASK-TERMINALS-002.1:** For a requested task and environment, every
  listed ordinary terminal shall belong to that task and environment. Fixed
  and script terminals shall retain their environment scope.
- **AC-TASKS-TASK-TERMINALS-002.2:** Tasks sharing an environment shall retain
  independent ordinary terminal results and initialization state. Sessions of
  the same task and environment shall share those results.
- **AC-TASKS-TASK-TERMINALS-002.3:** When the default panel needs a terminal,
  the system shall preserve its valid open selection. Otherwise, it shall
  select the lowest sequence among eligible open ordinary terminals. An eligible
  terminal belongs to the current task and environment. After a successful empty eligible list,
  concurrent initializers shall create at most one replacement terminal.
- **AC-TASKS-TASK-TERMINALS-002.4:** After a successful list, a restored default
  panel with an invalid terminal shall recover through the same selection
  rule. Other invalid ordinary panels shall stop connecting. Recovery shall
  not rename, resume, park, or remove another task's terminal.
- **AC-TASKS-TASK-TERMINALS-002.5:** Pending or failed reads shall not admit
  another task's cached terminal or cause automatic terminal creation. Late
  reads and action completions shall not replace a newer task's selection.
- **AC-TASKS-TASK-TERMINALS-002.6:** A permitted ordinary terminal shall connect
  regardless of which task execution serves its shared environment. The system
  shall preserve environment execution permission, task/environment access,
  terminal ownership checks, and one-shot command consumption after authorization.
- **AC-TASKS-TASK-TERMINALS-002.7:** Desktop and phone terminal selection,
  reopening, and mutations shall obey the same ownership rules. A same-task
  session switch shall preserve the selected terminal and its scrollback.

## Out of scope

- Changing terminal numbering or concurrent sequence-allocation semantics.
- Changing PTY process lifetime, output buffering, or WebSocket transport.
- Changing how the web application presents a terminal persistence failure.
- Changing Quick Terminal or agent-login terminal behavior.
- Sharing ordinary terminal ownership between tasks.
- Changing terminal layout, touch controls, or navigation.

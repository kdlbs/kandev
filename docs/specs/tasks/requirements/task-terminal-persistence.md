---
status: active
system: tasks
created: 2026-09-04
owners:
  - kandev
---

# Task Terminal Persistence Requirements

## Overview

Ordinary task terminals are durable task-scoped resources. The task system owns
their identity and lifecycle. Each record belongs to one task and its execution
environment. These operations must remain available on every supported
persistence backend.

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

## Out of scope

- Changing terminal numbering or concurrent sequence-allocation semantics.
- Changing PTY process lifetime, output buffering, or WebSocket transport.
- Changing how the web application presents a terminal persistence failure.
- Changing Quick Terminal or agent-login terminal behavior.

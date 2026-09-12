---
status: draft
system: tasks
created: 2026-09-02
owners:
  - Kandev
---

# Task Stop Reachability Requirements

## Overview

Stopping a task must halt the agent processes that task started. Kandev records
a session state in the database and separately tracks live agent executions in
the running backend. When a session is marked terminal while its execution is
still registered and its process is still alive, a database-only view of "which
sessions are active" cannot see the work that is still running. The task then
presents a settled card that the user cannot stop, and the agent process keeps
holding its workspace, its runtime, and its provider quota.

The task system owns this contract because stopping a task is a task-lifecycle
operation. The agent system owns the execution registry that this contract
reads.

## Terminology

- **Registered execution:** An agent execution that the running backend still
  tracks for a session, independent of the session's persisted state.
- **Orphaned execution:** A registered execution whose session is already in a
  terminal persisted state.
- **Active persisted state:** A session state that a database query treats as
  live work, currently `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT`.

## Requirements

### REQ-TASKS-TASK-STOP-REACHABILITY-001: Task Stop Reachability

**Intent:** Make a stop request reach every agent process the task still owns,
so a terminal session state can never hide a live process from the user.

**User story:** As a user whose task failed while its agent kept running, I want
the task-scoped stop to halt that agent, so that I can reclaim the task without
restarting the backend.

#### Acceptance criteria

- **AC-TASKS-TASK-STOP-REACHABILITY-001.1:** When a task-scoped stop runs, the
  system shall stop every registered execution for that task, including an
  orphaned execution whose session is not in an active persisted state.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.2:** When a task-scoped stop finds at
  least one registered execution, it shall report success rather than
  "execution not found", regardless of the persisted session states involved.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.3:** When a task has neither a session
  in an active persisted state nor a registered execution, the stop shall
  continue to report that no execution was found and shall change nothing.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.4:** When a stop halts an orphaned
  execution, it shall preserve that session's terminal state and error message
  rather than overwriting them with a cancellation state.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.5:** When a stop halts a session that
  is in an active persisted state, the existing cancellation transition and its
  events shall be unchanged.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.6:** When a stop request is repeated
  after the executions are gone, it shall not fail on the already-removed
  executions and shall not create duplicate state transitions.
- **AC-TASKS-TASK-STOP-REACHABILITY-001.7:** When a stop halts only orphaned
  executions, the log shall identify each session and execution that was
  reached through the execution registry rather than through the active-session
  query.

## Out of scope

- Choosing when a session becomes terminal. The
  [agent stall recovery](../../agents/requirements/agent-stall-recovery.md)
  requirement owns the never-started classification that produced the first
  observed orphan.
- Session-scoped stop and the `agent.cancel` recovery path, which already
  resolve an execution directly and are unchanged.
- Reconciling executions left behind by a previous backend process; startup
  cleanup already owns that path.
- Making a stopped task's workflow step or task state different from the state
  that the existing stop path produces.

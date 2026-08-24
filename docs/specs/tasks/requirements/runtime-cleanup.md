---
status: draft
system: tasks
created: 2026-06-22
updated: 2026-08-08
owners:
  - cfl
---
# Task Runtime Cleanup Requirements

## Overview

Task lifecycle operations release runtime resources without deleting reusable task workspaces or
discarding the durable evidence needed to retry cleanup safely.

## Requirements

### REQ-TASKS-RUNTIME-CLEANUP-001: Task Runtime Cleanup

**Intent:** Make archive, delete, shutdown, and startup cleanup ownership-aware, durable, idempotent,
and safe when runtimes or task rows are already gone.

#### Acceptance criteria

- **AC-TASKS-RUNTIME-CLEANUP-001.1:** When a task is archived or deleted, the system shall stop every runtime recorded for that task before destructive workspace cleanup, using `executors_running` ownership rather than terminal session state alone.
- **AC-TASKS-RUNTIME-CLEANUP-001.2:** When a session is deleted, the system shall remove only that session and its references; the task-owned workspace, Git worktrees, branches, and reusable files shall remain until a task lifecycle operation requests cleanup.
- **AC-TASKS-RUNTIME-CLEANUP-001.3:** When a task environment is shared, cleanup shall stop the deleting task's runtimes but shall defer destructive environment or worktree teardown until no active session holds the shared environment, and shall never remove borrowed resources.
- **AC-TASKS-RUNTIME-CLEANUP-001.4:** When an agent or agentctl process does not stop within its grace period, the system shall terminate the complete process group and shall not report shutdown complete while descendants remain attached to PID 1.
- **AC-TASKS-RUNTIME-CLEANUP-001.5:** When startup reconciliation sees a stale runtime row, the system shall remove only a confirmed-dead local runtime, preserve alive, unknown, remote, or generically failed rows for retry, and emit bounded diagnostics for fail-closed outcomes.
- **AC-TASKS-RUNTIME-CLEANUP-001.6:** When lifecycle cleanup begins, the system shall persist an operation snapshot and retry state before mutating task state; repeated delivery shall reuse the durable job, and unarchiving shall prevent a pending archive retry from deleting active resources.

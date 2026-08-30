---
status: draft
system: tasks
created: 2026-06-22
updated: 2026-08-30
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
- **AC-TASKS-RUNTIME-CLEANUP-001.7:** When an archived task is unarchived after
  cleanup removed its physical worktree, resuming the task shall recreate or
  reactivate the recoverable task-owned worktree instead of requiring
  attach-only reuse of the deleted workspace.
- **AC-TASKS-RUNTIME-CLEANUP-001.8:** When dead-row repair loses its compare-and-set to a newer execution, reconciliation shall preserve the newer row without a warning. Other repair errors shall remain warnings.
- **AC-TASKS-RUNTIME-CLEANUP-001.9:** When terminal lifecycle cleanup removes a
  worktree registration, it shall delete a local branch only when the persisted
  owner is Kandev, no live Git worktree uses the branch, exactly one durable
  environment-repository row owns it, and its exact head is contained in the
  persisted intended integration ref. Every other case shall retain the branch.
- **AC-TASKS-RUNTIME-CLEANUP-001.10:** Before deleting an eligible integrated
  branch, cleanup shall persist its exact head SHA. Unarchive and worktree
  recreation shall restore a missing managed branch from that SHA before remote
  recovery, while branches with unpublished commits shall retain their original
  local ref and restore exactly.
- **AC-TASKS-RUNTIME-CLEANUP-001.11:** Managed branch compaction shall delete only
  one explicit local ref with an atomic expected-head compare-and-delete. It shall never
  delete remote refs, protected/base refs, inferred branch globs, externally
  owned refs, or refs with legacy, missing, or ambiguous ownership metadata.
- **AC-TASKS-RUNTIME-CLEANUP-001.12:** Terminal cleanup shall emit bounded
  attempted, deleted, and retained totals plus fixed retained-reason counts.
  Receipts and metrics shall not contain branch lists, repository contents, or
  credentials.
- **AC-TASKS-RUNTIME-CLEANUP-001.13:** When archive cleanup retains a managed
  branch because it is not yet integrated, storage maintenance shall revisit at
  most a fixed number of archived, inactive worktree rows per run. It shall
  revalidate that the task remains archived immediately before invoking the
  worktree manager's existing safety policy. A later unarchive shall restore a
  safely compacted branch from its exact recorded head.

---
status: draft
system: tasks
created: 2026-06-22
updated: 2026-09-10
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
- **AC-TASKS-RUNTIME-CLEANUP-001.9:** When task deletion reclaims a Git worktree, the system shall verify the exact recorded path, branch, and commit before mutation; preserve a checkout with tracked or untracked changes; preserve a clean local branch with commits not contained by the recorded base or repository default; recover idempotently when the checkout path is already absent; and keep failed registration or branch cleanup retryable.
- **AC-TASKS-RUNTIME-CLEANUP-001.10:** When a task deletion targets an owned
  worktree with tracked or untracked changes, the system shall require explicit
  discard consent before it mutates the task or starts resource cleanup.
- **AC-TASKS-RUNTIME-CLEANUP-001.11:** When discard consent is absent, the
  system shall return a typed conflict and preserve the task, worktree, branch,
  and local changes without a cleanup retry.
- **AC-TASKS-RUNTIME-CLEANUP-001.12:** When discard consent is present, the
  system shall remove dirty owned worktrees only after the pinned no-follow
  path handle confirms the exact owned path, no shared active environment
  reference exists, and all ownership, path, registration, branch, and commit
  identity checks pass. The existing unique branch preservation rule shall
  remain active.
- **AC-TASKS-RUNTIME-CLEANUP-001.13:** When a recorded worktree directory and
  its local branch are absent, cleanup preparation shall permit task archive
  or deletion. This applies to direct and cascade operations, including tasks
  with other healthy repositories. Remaining resources shall retain their
  ownership checks and cleanup guarantees. If the omitted identity's path or
  registration reappears before execution, cleanup shall remain retryable and
  shall not adopt the live checkout without an immutable identity captured
  during preparation.
- **AC-TASKS-RUNTIME-CLEANUP-001.14:** When every worktree in the selected
  deletion scope has no tracked or untracked local changes, the confirmation
  shall hide discard consent and permit deletion without it. A confirmed empty
  worktree inventory shall have the same behavior. Committed branch differences
  alone shall not require discard consent.
- **AC-TASKS-RUNTIME-CLEANUP-001.15:** When at least one selected worktree has
  local changes, deletion shall require an unchecked discard selection. The
  scope shall include descendants only when cascade is selected. Bulk selection
  shall consider every selected task and repository, including retained
  worktrees for tasks without sessions.
- **AC-TASKS-RUNTIME-CLEANUP-001.16:** While worktree inspection is pending or
  unavailable, the confirmation shall hide discard consent and disable deletion.
  An inspection failure shall provide a localized explanation and retry action.
  Changing the selection or cascade choice shall invalidate previous inspection
  results and consent. Reopening shall inspect again.
- **AC-TASKS-RUNTIME-CLEANUP-001.17:** Desktop and phone shall use the same
  consent conditions. Archive shall not show a discard selection for a clean
  workspace. Its independent subtask selection and cleanup behavior shall remain
  unchanged. A deletion rejected because changes appeared after inspection shall
  preserve the task and permit a fresh confirmation with explicit consent.

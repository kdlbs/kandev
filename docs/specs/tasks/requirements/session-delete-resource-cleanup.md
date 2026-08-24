---
status: draft
system: tasks
created: 2026-08-08
owners:
  - cfl
---
# Session Delete Preserves Task Workspaces Requirements

## Overview

A session is a conversation and execution reference inside a task; it is not the owner of the task's materialized workspace. A task may have multiple sessions sharing one `TaskEnvironment`, may intentionally have zero sessions, and may create a future session that reuses its existing files and Git state.

## Requirements

### REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001: Session Delete Preserves Task Workspaces

**Intent:** A session is a conversation and execution reference inside a task; it is not the owner of the task's materialized workspace. A task may have multiple sessions sharing one `TaskEnvironment`, may intentionally have zero sessions, and may create a future session that reuses its existing files and Git state.

#### Acceptance criteria

- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.1:** Deleting a session removes its session row and conversation history. Its `task_environment_id` reference disappears with the row; the task-owned environment is unchanged.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.2:** Deleting a session never removes a physical directory, runs `git worktree remove` or `git worktree prune`, or deletes a branch.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.3:** The rule applies when the deleted session is the task's only session and when it is one of several sessions sharing the workspace.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.4:** A task with zero sessions retains its `TaskEnvironment`, worktree directory, Git registration, branch, and uncommitted files.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.5:** A later session may reuse the retained workspace.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.6:** Task archive/delete, cascade archive/delete, workspace delete, quick-chat task expiry, and explicit task-environment reset remain the owners of physical cleanup.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.7:** Task lifecycle cleanup discovers task-owned resources without joining through a session row and executes asynchronously through the durable cleanup worker.
- **AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.8:** Resources borrowed by another task or referenced through a shared environment are preserved or transferred according to existing ownership rules.

## System design

The migrated technical source is split into [part 1](../system-design/session-delete-resource-cleanup.md).

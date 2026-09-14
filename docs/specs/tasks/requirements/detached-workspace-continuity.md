---
status: active
system: tasks
created: 2026-09-04
owners:
  - kandev
---

# Detached Workspace Continuity Requirements

## Overview

The task system owns the lifetime of task environments and task-owned
worktrees. Detaching a subtask must therefore preserve the workspace that its
sessions use and must separate that workspace's cleanup authority from the
former parent's lifecycle.

## Terminology

- **Workspace steward:** The current task whose identity owns a shared
  workspace group and its canonical task environment. Stewardship can move
  between active group members without changing the physical workspace.
- **Ownership generation:** A monotonically increasing value that identifies
  one assignment of a workspace group or task environment to a steward.
- **Stale cleanup:** Cleanup prepared for an owner or ownership generation that
  is no longer current.

## Requirements

### REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001: Detached task workspace continuity

**Intent:** Keep a detached task's running and resumable work intact when the
former parent is archived, deleted, retried, or recovered after restart.

#### Acceptance criteria

- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.1:** When a subtask that uses its
  parent's shared workspace is detached, the system shall make an active member
  of that shared workspace independent of the former parent's cleanup authority
  before reporting detachment success.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.2:** When detachment changes
  workspace stewardship, the hierarchy change and workspace-group and
  environment ownership changes shall either all become durable or none shall
  become durable.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.3:** When the former parent is
  archived or deleted after detachment, the detached task shall continue using
  the same canonical workspace without its active or resumable environment
  being stopped, removed, or made reusable by another task.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.4:** When cleanup or an ownership
  retry presents an owner or ownership generation that is no longer current,
  the system shall reject the destructive operation without changing the
  replacement workspace, environment, or task.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.5:** When the backend restarts
  after detachment, the detached task shall recover the same workspace binding,
  current steward, and ownership generation.

### REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002: Canonical subtask workspace attachment

**Intent:** Ensure every task-creation surface establishes the same durable
workspace relationship before a child can launch or be returned as completely
created.

#### Acceptance criteria

- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.1:** When any supported REST,
  WebSocket, MCP, plugin, workflow, or internal task-creation route creates a
  child task, the system shall apply the same parent workspace-policy resolution
  and membership attachment behavior.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.2:** When workspace attachment
  fails, the task creation shall fail without leaving a launchable child that
  lacks its required workspace membership.
- **AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.3:** When child creation,
  detachment, parent cleanup, and workspace stewardship changes overlap, the
  system shall serialize them or reject one operation without a partial
  hierarchy or ownership result.

## Out of scope

- Copying or provisioning a second physical workspace during detachment.
- Stopping or restarting the detached task's active sessions.
- Changing descendants, blockers, workflow placement, or repository selection.
- Allowing multiple physical owners for one canonical task environment.

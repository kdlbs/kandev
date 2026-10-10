---
status: active
system: ui
created: 2026-10-07
owners:
  - kandev
---

# Deleted Task Client State Requirements

## Overview

A long-running web or desktop window receives `task.deleted` for tasks removed
locally, on another device, or by the backend. The window must not keep the
deleted task's conversation history and runtime buffers for the rest of its
lifetime. Backend runtime cleanup stays owned by
[Task runtime cleanup](../../tasks/requirements/runtime-cleanup.md).

## Terminology

- **Session state:** The client store entries keyed by one task session:
  the session record, messages, turns, queue metadata, and the runtime buffers
  that `purgeSessionRuntimeState` owns.
- **Deleted task sessions:** Every session the store associates with the
  deleted task through the task's session list, normalized session records, or
  the task's primary session.

## Requirements

### REQ-UI-DELETED-TASK-CLIENT-STATE-001: Release deleted task session state

**Intent:** Keep client memory bounded by live tasks rather than by every task
the window has seen since it loaded.

#### Acceptance criteria

- **AC-UI-DELETED-TASK-CLIENT-STATE-001.1:** When the client receives
  `task.deleted`, it shall remove the session state of every deleted task
  session from the store.
- **AC-UI-DELETED-TASK-CLIENT-STATE-001.2:** Sessions that belong to other tasks
  shall keep their session state.
- **AC-UI-DELETED-TASK-CLIENT-STATE-001.3:** Existing `task.deleted` behavior
  (local storage cleanup, queue status, sidebar and kanban removal,
  notifications, and redirects) shall not change.

## Out of scope

- Archived tasks, whose history stays viewable from the archive.
- Evicting history for live sessions that no panel is viewing.
- Backend runtime and workspace cleanup.

## Implementation plans

- [Deleted task client state](../../../plans/deleted-task-client-state/plan.md)

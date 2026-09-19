---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Taskless Office Run Sessions

## Overview

Office owns lightweight routine execution and coordinator wakeups. These wakes
must execute without inventing task records. This makes explicit the taskless
behavior already described in the [scheduler design](../system-design/scheduler-01.md).
The user confirmed this behavior on September 17, 2026.

## Requirements

### REQ-OFFICE-TASKLESS-001: Managed taskless execution

**Intent:** A coordinator can inspect and manage its workspace on a periodic or
event wake without creating a task merely to host its own execution.

#### Acceptance criteria

- **AC-OFFICE-TASKLESS-001.1:** An eligible lightweight wake with no task shall start a real agent session, deliver its assembled prompt and permitted tools, and reach a visible terminal outcome without creating any task or task session.
- **AC-OFFICE-TASKLESS-001.2:** Every fire and retry shall use a fresh session. The existing routine-scoped or agent-scoped continuation summary shall carry context; a failed attempt shall not replace the last successful summary.
- **AC-OFFICE-TASKLESS-001.3:** Concrete and provider-routed profiles shall both work. Existing budget admission, capabilities, retry/backoff, coalescing and idle-skip rules shall apply. Periodic idle skipping shall not consume a manual or webhook wake.
- **AC-OFFICE-TASKLESS-001.4:** Run history shall show the exact session, runtime outcome, actual adapter/model and attributed usage. Duplicate or delayed events from an older attempt shall not complete, charge twice, or clear a newer attempt.
- **AC-OFFICE-TASKLESS-001.5:** Workspace pause, explicit run cancellation, agent disable/removal and workspace removal shall prevent new taskless launches and stop live taskless executions. Partial stop failures shall remain visible and retryable, including when another execution stopped successfully.
- **AC-OFFICE-TASKLESS-001.6:** After backend restart, unfinished attempts shall be reconciled against runtime evidence before replacement launch. A possibly live predecessor shall block a replacement until it is stopped or proven absent. Interrupted work shall have an explicit outcome; no taskless attempt shall remain claimed indefinitely solely because it has no task.
- **AC-OFFICE-TASKLESS-001.7:** A taskless session shall retain its workspace and capability boundaries. Task-specific decision tools shall reject it without a task context. Failed ownership lookup shall reject launch or access, never broaden authority.
- **AC-OFFICE-TASKLESS-001.8:** Existing task-bound launches and their task/session lifecycle shall retain their behavior. Historical unsupported taskless failures shall remain history and shall not be automatically replayed.

## Out of scope

Interactive taskless chat, synthetic tasks, new periodic producers, changing
coordinator cadence, and cross-fire ACP session resumption.

## System design

[Run-owned sessions](../system-design/taskless-run-sessions.md).

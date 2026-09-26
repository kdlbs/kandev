---
status: active
system: tasks
created: 2026-08-01
updated: 2026-08-24
owners:
  - cfl
---

# Queued Run Scheduling Requirements

## Overview

Workflow actions and Office automation can enqueue work without a user
clicking Start. Kandev must dispatch that work across workspaces while keeping
ordinary Kanban assignment manual, Office maintenance scoped, and shutdown
safe.

## Requirements

### REQ-TASKS-RUN-SCHEDULING-001: Queued run scheduling

**Intent:** Provide one durable, restart-safe queue consumer for explicit and
autonomous launches without creating workspace-local schedulers.

#### Acceptance criteria

- **AC-TASKS-RUN-SCHEDULING-001.1:** When queued runs exist in multiple
  workspaces, one backend-wide scheduler shall claim them in global request
  order while preserving the existing per-agent serialization rule.
- **AC-TASKS-RUN-SCHEDULING-001.2:** When a workflow explicitly uses
  `queue_run`, the system shall enqueue and process that work for any workflow
  style; an interactive user launch shall retain its existing launch path.
- **AC-TASKS-RUN-SCHEDULING-001.3:** When a task is an ordinary Kanban task,
  assigning a runner shall not make it autonomous; Office assignment and
  recovery shall act only on authoritative Office tasks.
- **AC-TASKS-RUN-SCHEDULING-001.4:** When Office configuration is absent, the
  system shall not run Office-only recovery or cron work on ordinary Kanban
  tasks.
- **AC-TASKS-RUN-SCHEDULING-001.5:** When a queue signal is missed or the
  process restarts, a periodic safety pass shall recover persisted queued work.
- **AC-TASKS-RUN-SCHEDULING-001.6:** When graceful shutdown begins, scheduler
  and cron work shall stop and join before the repository or database closes.
- **AC-TASKS-RUN-SCHEDULING-001.7:** When queued or retryable runs exist during
  shutdown, they shall remain recoverable after restart and shall not be
  deleted or terminally failed only because the process exits.
- **AC-TASKS-RUN-SCHEDULING-001.8:** When shutdown completes normally, the
  backend shall not emit scheduler database-closed errors or scheduler logs
  after the graceful-shutdown completion message.

## Out of scope

- Multiple active backend processes or distributed leader election.
- Per-workspace scheduler goroutines.
- Changes to FIFO ordering, retry policy, or scheduler status UI.

## System design

- [Queued Run Scheduling System Design](../system-design/run-scheduling.md)

## Related records

- [Global run scheduler ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md)
- [Run scheduler lifecycle](../../../plans/run-scheduler-lifecycle/plan.md)

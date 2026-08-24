---
status: draft
system: office
created: 2026-04-25
owners:
  - cfl
---
# Office Scheduler Requirements

## Overview

Kandev's base task scheduler is reactive: tasks enter the queue only when a user explicitly starts them or sends a prompt. Office adds autonomous agent operation, which requires the system to wake agents on its own when events happen (assignments, comments, blocker resolutions, approvals), on a schedule (routines), and on heartbeat ticks (periodic coordinator checks). Without an autonomous wakeup pipeline, every interaction needs a human to initiate it, and the cost / reliability story (idle skips, rate-limit retries, staleness, recovery) has nowhere to live.

Office supplies autonomous run producers and Office-specific maintenance. The persisted `runs` queue and its single backend-wide consumer are shared workflow infrastructure, not one scheduler per Office workspace. Shared ownership, scoping, and shutdown contract is defined by [run queue](../../tasks/requirements/run-scheduling.md) and [ADR-2026-08-01-global-run-scheduler-ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md).

## Requirements

### REQ-OFFICE-SCHEDULER-001: Office Scheduler

**Intent:** Kandev's base task scheduler is reactive: tasks enter the queue only when a user
explicitly starts them or sends a prompt. Office adds autonomous agent operation, which requires the
system to wake agents on its own when events happen (assignments, comments, blocker resolutions,
approvals), on a schedule (routines), and on heartbeat ticks (periodic coordinator checks). Without
an autonomous wakeup pipeline, every interaction needs a human to initiate it, and the cost /
reliability story (idle skips, rate-limit retries, staleness, recovery) has nowhere to live. Office
supplies autonomous run producers and Office-specific maintenance. The persisted `runs` queue and
its single backend-wide consumer are shared workflow infrastructure, not one scheduler per Office
workspace. Shared ownership, scoping, and shutdown contract is defined by [run
queue](../../tasks/requirements/run-scheduling.md) and
[ADR-2026-08-01-global-run-scheduler-ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md).

#### Acceptance criteria

- **AC-OFFICE-SCHEDULER-001.1:** One shared runs scheduler processes persisted work for every workspace.
- **AC-OFFICE-SCHEDULER-001.2:** Office event subscribers and unstarted-task recovery act only on tasks whose project/workflow identity makes `Task.IsFromOffice` true. A runner on an ordinary Kanban task is not sufficient.
- **AC-OFFICE-SCHEDULER-001.3:** Explicit workflow `queue_run` actions remain available to every workflow style and are not filtered by Office identity.
- **AC-OFFICE-SCHEDULER-001.4:** Office recovery is maintenance, not part of every five-second queue drain. When no workspace has adopted Office, it skips the task scan.
- **AC-OFFICE-SCHEDULER-001.5:** The shared runs scheduler and cron loop stop and join before database cleanup during graceful shutdown.
- **AC-OFFICE-SCHEDULER-001.6:** Has a `source` discriminator (see table below) plus a typed payload.
- **AC-OFFICE-SCHEDULER-001.7:** Carries an `idempotency_key` for source-level dedup within a 24-hour window.
- **AC-OFFICE-SCHEDULER-001.8:** Is coalesced into an in-flight run when one exists for the same agent (claim-time merge).

## System design

The migrated technical source is split into [part 1](../system-design/scheduler-01.md), [part 2](../system-design/scheduler-02.md).

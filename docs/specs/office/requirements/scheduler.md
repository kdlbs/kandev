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
- **AC-OFFICE-SCHEDULER-001.7:** Carries an `idempotency_key`. The queue uses a 24-hour lookup for recent duplicates and a durable unique key for persisted identities.
- **AC-OFFICE-SCHEDULER-001.8:** Is coalesced into an in-flight run when one exists for the same agent (claim-time merge).
- **AC-OFFICE-SCHEDULER-001.9:** A cron routine trigger's day-of-month and day-of-week fields are ORed when both are restricted, matching `crontab(5)` (`0 0 13 * 5` fires on the 13th of the month OR any Friday, not only Friday the 13th).
- **AC-OFFICE-SCHEDULER-001.10:** A cron routine trigger's wall-clock slot fires at most once across a DST transition: a slot that does not exist (spring-forward) is skipped, a slot that occurs twice (fall-back) fires only on its first occurrence, and no existing slot is ever lost. This holds for every IANA zone, including `Australia/Lord_Howe`, the only zone with a 30-minute DST shift.
- **AC-OFFICE-SCHEDULER-001.11:** A cron routine trigger that can never fire (an impossible date, or an empty expression) is rejected at trigger-create time with a client error, not accepted as a silent no-op or a wrong daily fallback.
- **AC-OFFICE-SCHEDULER-001.12:** A routine trigger's timezone defaults to UTC when not supplied; there is no workspace-level timezone.
- **AC-OFFICE-SCHEDULER-001.13:** A routine run in `task_created` with no linked task is not an active run. The next dispatch that finds it for its fingerprint closes it as `failed` and does not skip or coalesce into it. A heavy run whose linked task is live still gates its fingerprint.

### REQ-OFFICE-SCHEDULER-002: Assignment wake respects step auto-start

**Intent:** An Office task's current workflow step is authoritative for whether an agent should be
running at all. A step with no `auto_start_agent` on_enter action (Backlog, `events:{}`) means the
workflow has not yet decided to run this task — an assignee change, a task-created/task-updated
event, or the unstarted-task recovery sweep must not launch an agent outside that decision. Every
producer of a `task_assigned` run for an Office task's assignee (reactivity's assignee-change
handler, the event-subscriber path, and unstarted-task recovery) applies the same eligibility
predicate: the task's current workflow step must have an `auto_start_agent` on_enter action, or the
task must have no workflow step at all (a task outside any workflow keeps its prior behaviour). This
does not change how a legitimate auto-start step wakes its runner, and it does not touch Review/
Approval's separate reviewer/approver wake path.

#### Acceptance criteria

- **AC-OFFICE-SCHEDULER-002.1:** Assigning, reassigning, creating, or updating an Office task whose
  current workflow step has no `auto_start_agent` on_enter action queues zero `task_assigned` runs,
  across all three producers (reactivity assignee-change, event-subscriber queueing, unstarted-task
  recovery). The previous assignee's session interrupt (when reassigning) is unaffected — only the
  new wake is gated.
- **AC-OFFICE-SCHEDULER-002.2:** Assigning, reassigning, creating, or updating an Office task whose
  current workflow step has an `auto_start_agent` on_enter action queues exactly one `task_assigned`
  run per triggering event, unchanged from prior behaviour.
- **AC-OFFICE-SCHEDULER-002.3:** A task with no workflow step bound (`workflow_step_id` empty) is
  treated as eligible, preserving behaviour for tasks outside a workflow.
- **AC-OFFICE-SCHEDULER-002.4:** A step lookup failure (repository error, missing step getter, or a
  step ID that does not resolve) fails open — the wake is queued as before, and the skip decision is
  never silently swallowed: an eligible-vs-ineligible outcome is logged at Info, and a failed lookup
  is logged at Warn.

### REQ-OFFICE-SCHEDULER-003: Create-time assignee validation and runner seat

**Intent:** REQ-OFFICE-SCHEDULER-002 gates the wake on the step the task lands on; this requirement
gates what the wake is for. A task-create request may name an `assignee_agent_profile_id` to seat as
the task's runner. That name is caller-supplied and must be checked before any task row exists —
otherwise the system either seats a runner that cannot legitimately hold the seat (a profile scoped
to a different workspace, or a global/kanban-legacy profile with no workspace at all) or accepts the
request and silently drops the assignment, leaving the caller with a task that reports success but
has no runner and therefore never gets the REQ-OFFICE-SCHEDULER-002 wake. A profile is a valid
create-time assignee only if it names an Office agent instance scoped to the same workspace as the
task being created; this mirrors the eligibility filter `ListAgentInstances` already applies when
listing assignable profiles for a workspace, which does not gate on the profile's `enabled` flag
either — a profile the assignee picker already offers must remain assignable, so create-time
validation is not stricter than the picker it validates against. A non-workspace-scoped ("global")
profile is not eligible for the Office listing or this create-time contract. A separate legacy
workflow-override rule allows global profiles in that context; it does not apply to task assignees.
The assignee field also applies only to Office-owned tasks, identified by a project or the
workspace's canonical Office workflow. A regular Kanban workflow cannot accept an Office assignee.

#### Acceptance criteria

- **AC-OFFICE-SCHEDULER-003.1:** A create-task request naming an `assignee_agent_profile_id` that
  resolves to an Office agent instance scoped to the request's own workspace succeeds, and
  the created task's runner seat (`workflow_step_participants`, `role='runner'`) names that profile
  before the `task.created` event is published. This holds regardless of the profile's `enabled`
  flag.
- **AC-OFFICE-SCHEDULER-003.2:** A create-task request naming an `assignee_agent_profile_id` that
  does not resolve to any profile, resolves to a profile scoped to a different workspace, or resolves
  to a profile with no workspace (`WorkspaceID == ""`) is rejected with a client error before any
  task row is written. No partial task, no runner seat, and no wake are produced. A lookup failure
  unrelated to the profile's existence (e.g. a store error) is not treated as an invalid assignee and
  surfaces as a server error instead.
- **AC-OFFICE-SCHEDULER-003.3:** A duplicate create request sharing an already-used `external_id`
  returns the previously created task (the existing idempotent-create contract) without re-running
  this validation against the duplicate request's own `assignee_agent_profile_id` and without
  writing a second runner seat or a second wake.
- **AC-OFFICE-SCHEDULER-003.4:** A create-task request that omits `assignee_agent_profile_id` is
  unaffected: the task is created with no runner seat, exactly as before this requirement.
- **AC-OFFICE-SCHEDULER-003.5:** A create-task request that names an Office assignee for a task
  that is not Office-owned is rejected with a client error before any task row is written. The
  Office-owned check accepts a project-linked task or a task on the workspace's canonical Office
  workflow. No runner seat or assignment wake is produced for a rejected Kanban task.

## System design

The migrated technical source is split into [part 1](../system-design/scheduler-01.md), [part 2](../system-design/scheduler-02.md).

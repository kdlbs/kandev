---
status: building
created: 2026-08-01
owner: cfl
---

# Queued run scheduling

## Why

Workflow actions and Office automation can enqueue agent work without a user
clicking Start. Users need that work to be dispatched reliably across multiple
workspaces without making ordinary Kanban assignment autonomous, wasting
resources on Office-only scans when Office is unused, or producing database
errors during a normal shutdown.

## What

- Kandev has one backend-wide scheduler for the persisted `runs` queue. It
  serves every workspace and never creates one scheduler goroutine per
  workspace.
- Runs are claimed globally in request order while preserving the existing
  per-agent serialization rule. A busy workspace does not create another
  scheduler or bypass agent capacity checks.
- An explicit `queue_run` workflow action may enqueue work from any workflow
  style. User-initiated Kanban launches continue to bypass `runs` and launch
  through the interactive orchestrator path.
- Merely assigning a runner to an ordinary Kanban task does not make that task
  autonomous. Office assignment subscribers and unstarted-task recovery act
  only on authoritative Office tasks: tasks linked to an Office project or
  tasks whose workflow is the workspace's canonical `office_workflow_id`.
- Office-only maintenance is separate from queue draining. Unstarted-task
  recovery, Office routines, and Office budget work run only when their
  corresponding Office configuration exists. A deployment with Office enabled
  but no configured Office workflow performs no five-second Office task
  recovery scan.
- The queue scheduler remains event-driven: an in-process queue signal wakes it
  immediately, while a periodic safety pass recovers persisted work after a
  missed signal or process restart.
- Every scheduler or cron loop owns the goroutine it starts. `Stop` is
  idempotent, cancels future ticks, and waits for an in-progress tick and any
  handler fan-out to return.
- Graceful shutdown stops external intake, quiesces and joins queue/cron
  schedulers, stops the orchestrator and agent runtime, and only then closes
  event-bus, repository, and database resources.
- A normal signal-driven shutdown emits no `sql: database is closed` scheduler
  errors and does not log scheduler activity after `Graceful shutdown
  complete`.

Decision: [ADR-2026-08-01-global-run-scheduler-ownership](../../decisions/2026-08-01-global-run-scheduler-ownership.md).

Implementation plan: [run-scheduler-lifecycle](../../plans/run-scheduler-lifecycle/plan.md).

## Data model

No schema change is required.

- `runs` remains the durable, backend-wide queue and run record.
- `workspaces.office_workflow_id` plus a non-empty task `project_id` remain the
  authoritative persisted Office-task identity used by `Task.IsFromOffice`.
- Office routines, budget policies, and related configuration remain
  workspace-scoped in their existing tables.

## State machine

### Scheduler lifecycle

```text
stopped -> running -> stopping -> stopped
```

- `Start` moves `stopped -> running` and owns exactly one loop goroutine.
- A duplicate `Start` while running is a no-op.
- `Stop` moves `running -> stopping`, cancels the loop, waits for the active
  tick to drain, then moves to `stopped`.
- A duplicate `Stop` is a no-op.
- Parent-context cancellation follows the same drain path as `Stop`.

### Persisted run lifecycle

The existing `queued`, `claimed`, and terminal run states are unchanged.
Process shutdown does not create a new run state and does not mark queued work
failed. A claimed run interrupted by process exit is recovered through the
existing stale-claim policy after restart.

## Failure modes

| Condition | Required behavior |
|---|---|
| Queue signal is missed | The periodic safety pass eventually claims the persisted row. |
| No eligible run exists | The scheduler returns to waiting without an error. |
| Office is enabled but no Office workflow/configuration exists | Generic queue recovery remains available; Office-only recovery and cron work do not scan ordinary Kanban tasks. |
| A Kanban task has a runner but no explicit `queue_run` action | Office subscribers and recovery ignore it; the user or Kanban workflow controls launch timing. |
| Shutdown begins while the scheduler is idle | `Stop` cancels the wait and joins before the database closes. |
| Shutdown begins during a tick | Shutdown cancels future ticks and waits for the active tick and cron handler fan-out to return before repository cleanup. The backend does not impose an internal join deadline. |
| A handler does not return after cancellation | Graceful shutdown remains in progress and repository cleanup does not begin. The launcher or process supervisor may enforce its outer grace period and terminate the process, but the backend never closes SQLite underneath live scheduler work. |

## Persistence guarantees

- Queued and scheduled-retry runs survive restart.
- Shutdown never deletes or terminally fails a queued run merely because the
  process is exiting.
- Scheduler ownership state, contexts, timers, and in-memory queue signals do
  not survive restart; persisted rows are the recovery source of truth.
- Office activation state is derived from persisted workspace/workflow and
  Office configuration rather than an in-memory workspace list.

## Scenarios

- **GIVEN** two workspaces have queued runs for different agents, **WHEN** the
  global scheduler drains the queue, **THEN** it processes both workspaces in
  request order while allowing at most one claimed run per agent.
- **GIVEN** Office mode is enabled and no workspace has an Office workflow,
  Office project, active Office routine, or Office budget policy, **WHEN** the
  backend remains idle, **THEN** no Office unstarted-task recovery query runs
  on the five-second queue cadence and no task is launched.
- **GIVEN** an ordinary Kanban `TODO` task has a step runner, **WHEN** Office
  assignment subscribers and recovery execute, **THEN** no `task_assigned` run
  is created for that task.
- **GIVEN** an authoritative Office `TODO` task has a runner, was created inside
  the recovery lookback window, and has no prior queued, claimed, or finished
  run, **WHEN** Office recovery executes, **THEN** exactly one idempotent
  `task_assigned` run is queued.
- **GIVEN** a custom non-Office workflow explicitly executes `queue_run`,
  **WHEN** the row is persisted and signalled, **THEN** the global scheduler
  processes it even though the task is not an Office task.
- **GIVEN** the backend receives SIGINT while all schedulers are idle, **WHEN**
  graceful shutdown completes, **THEN** the database closes after every
  scheduler has stopped and the log contains no `database is closed` error.
- **GIVEN** a scheduler tick or cron handler is blocked inside database-backed
  work, **WHEN** graceful shutdown begins, **THEN** shutdown waits for that work
  to return before closing the database and prints no scheduler-stopping log
  after the completion message.
- **GIVEN** queued runs exist when the process exits, **WHEN** Kandev starts
  again, **THEN** the periodic safety pass resumes them without requiring an
  Office workspace to own a separate scheduler.

## Out of scope

- Multiple active backend processes or distributed leader election.
- Per-workspace scheduler goroutines.
- Changing global FIFO ordering or adding workspace fairness/quotas.
- Renaming the existing `office.run.*` WebSocket subjects.
- Changing retry counts, retry backoff, or agent cooldown policy.
- Adding a user-facing scheduler status page or new feature toggle.
- Replacing the five-second generic safety cadence with dynamic retry timers;
  this change removes Office-only work from that cadence but preserves its
  existing queue-recovery contract.

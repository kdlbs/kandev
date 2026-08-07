---
spec: docs/specs/tasks/run-scheduling.md
created: 2026-08-01
status: done
---

# Implementation Plan: Run Scheduler Lifecycle and Office Scoping

## Overview

The confirmed shutdown defect is caused by context cancellation running after
repository cleanup while the runs and cron loops are untracked. The change
first gives both loops owned, joinable lifecycles; then makes Office task
identity reusable and scopes autonomous assignment/recovery; then removes
Office recovery from the five-second queue tick; finally wires the owned loops
into graceful shutdown before SQLite cleanup. No schema, API, or frontend
change is required.

---

## Backend

### Owned scheduler lifecycles

- Update `internal/runs/scheduler.Scheduler` to own its derived context,
  goroutine, mutex, cancellation function, and wait group. `Start` and `Stop`
  become idempotent; `Stop` waits for an in-progress `RunProcessor.Tick`.
- Apply the same lifecycle to `internal/scheduler/cron.Loop`. Its `Stop` waits
  for `fanOut`, which already waits for every handler.
- Replace sleep-based shutdown assertions with channel synchronization and
  `testing/synctest` where practical.

### Authoritative Office-task scoping

- Export the existing task-repository Office predicate so the task projection
  and Office repository queries cannot drift.
- Extend `office/repository/sqlite.TaskExecutionFields` with
  `IsFromOffice` and use the shared predicate in its query.
- Restrict `ListUnstartedTasks` to authoritative Office tasks while preserving
  its lookback, runner, archive, and prior-run conditions.
- Make task-created/task-updated Office subscribers skip non-Office tasks even
  when a runner is present. Explicit workflow-engine `queue_run` actions remain
  unchanged and generic.

### Separate Office maintenance from queue draining

- Keep queued-run claim/dispatch, routing-unpark, and stale-claim recovery in
  the shared run processor.
- Remove `recoverUnstartedTasks` from
  `office/service.SchedulerIntegration.tick` and expose it through an Office
  recovery handler driven by the shared cron loop.
- Add a cheap repository capability check for authoritative Office adoption so
  the recovery handler skips the task scan when no workspace has an Office
  workflow/project. Keep handler activation data-derived so adding the first
  Office workflow works without restart or another scheduler goroutine.
- Give the processor/recovery loggers distinct `runs-processor` and
  `office-recovery` component names so operational logs reflect ownership.

### Graceful shutdown composition

- Return an owned scheduling runtime from backend startup rather than
  launching fire-and-forget goroutines.
- Thread its stop operation into `awaitShutdown` / `runGracefulShutdown` and
  invoke it after HTTP intake closes but before `orchestratorSvc.Stop`, agent
  runtime teardown, and `runCleanups`.
- Preserve the root application-context cleanup as a final fallback for other
  components, but do not rely on its cleanup-stack position to stop database
  users.
- Include scheduling stop failures in the graceful-shutdown error count and
  emit the completion log only after both loops have joined.

---

## Tests

- **What:** duplicate `Start`/`Stop`, parent cancellation, and Stop waiting for
  an active tick.
  **File:** `apps/backend/internal/runs/scheduler/scheduler_test.go`.
  **How:** channel-controlled fake processor plus `testing/synctest` for timer
  behavior.
- **What:** cron Stop waits for all handler fan-out and does not tick after
  return.
  **File:** `apps/backend/internal/scheduler/cron/cron_test.go`.
  **How:** blocking handlers coordinated by channels.
- **What:** Kanban tasks with runners are excluded while project-linked and
  canonical Office-workflow tasks are recoverable across multiple workspaces.
  **Files:** `apps/backend/internal/office/repository/sqlite/tasks_test.go`,
  `apps/backend/internal/office/service/event_subscribers_test.go`, and a
  focused Office recovery test beside `scheduler_recovery.go`.
  **How:** real in-memory SQLite repositories and synchronous event handlers.
- **What:** no Office configuration skips the Office task scan, while adding
  the first Office workflow activates the next recovery pass without restart.
  **File:** `apps/backend/internal/office/service/scheduler_recovery_test.go`.
  **How:** real repository with two maintenance passes before and after
  persisted Office-project adoption.
- **What:** graceful shutdown joins database-using schedulers before the pool
  cleanup and reports stop errors.
  **File:** `apps/backend/internal/backendapp/helpers_test.go` or a focused
  `shutdown_test.go` in the same package.
  **How:** ordered fakes and a blocking scheduler; assert no operation occurs
  after the fake database-close marker.

Exact task-level commands are recorded in the task files below.

---

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates; user authorization required):

- [x] [task-01-owned-loop-lifecycles](task-01-owned-loop-lifecycles.md)
- [x] [task-02-office-task-scoping](task-02-office-task-scoping.md)

Wave 2:

- [x] [task-03-office-maintenance-separation](task-03-office-maintenance-separation.md)

Wave 3:

- [x] [task-04-shutdown-composition](task-04-shutdown-composition.md)

The default execution order is sequential in the primary conversation. These
waves do not authorize subagents.

## Risks

- The Office scheduler spec predates the unified queue; implementation must
  preserve current `runs` behavior rather than reintroducing the retired
  `agent_wakeup_requests` design.
- Task identity must reuse the exact `Task.IsFromOffice` predicate. A subtly
  different repository filter could either wake Kanban tasks or strand Office
  project tasks.
- Stop must not hold a mutex while waiting for a goroutine that needs the same
  mutex to finish.
- Moving recovery to the 30-second cron cadence intentionally increases the
  worst-case repair latency for a missed assignment event while leaving normal
  signal-driven queue latency unchanged.

## Out of scope

- Frontend or E2E changes; this has no user-interface contract change.
- Full migration of prompt assembly, routing, and launch policy out of
  `internal/office/service`.
- Dynamic generic safety-timer/backoff redesign.

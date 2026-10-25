---
id: "03-office-maintenance-separation"
title: "Separate Office recovery maintenance"
status: done
wave: 2
depends_on: ["01-owned-loop-lifecycles", "02-office-task-scoping"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/run-scheduling.md"
---

# Task 03: Separate Office recovery maintenance

## Acceptance

- The five-second run processor tick drains queued runs and performs generic
  routing/stale-claim recovery without running the Office unstarted-task scan.
- Office unstarted-task recovery runs through the shared cron loop and skips
  its task query when no authoritative Office workspace/project exists.
- Adding the first Office workflow/project is observed on the next maintenance
  pass without restarting Kandev or creating a per-workspace goroutine.

## Verification

```bash
cd apps/backend && go test ./internal/office/service ./internal/scheduler/cron ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/scheduler_recovery.go`
- `apps/backend/internal/office/service/scheduler_recovery_test.go`
- `apps/backend/internal/office/repository/sqlite/tasks.go`
- `apps/backend/internal/backendapp/cron.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

- Task 01 provides the owned cron lifecycle.
- Task 02 provides authoritative Office-task identity and recovery filtering.

## Parallelism

`sequential`; it shares Office scheduler and backend wiring with its
dependencies and Task 04.

## Inputs

- Spec: Office-disabled/no-adoption and recovery scenarios.
- Plan: Separate Office maintenance from queue draining.
- Existing cron handler pattern: `internal/scheduler/cron.Handler`.

## Risks

- Normal queue signals must retain their existing sub-100ms path.
- Recovery moves from a five-second to a thirty-second maximum cadence; only
  missed-event repair latency changes.

## Output contract

Report the maintenance split, files changed, test command/result, blockers,
remaining risks, and update this task plus `plan.md` status in the same
conversation.

## Result

- Removed unstarted-task recovery from the five-second runs processor tick.
- Added `OfficeRecoveryHandler` to the shared cron loop with a persisted
  Office-adoption check, so Kanban-only installations skip the task scan and
  newly adopted Office projects activate without restart.
- Verification: `cd apps/backend && go test -race ./internal/office/service ./internal/scheduler/cron ./internal/backendapp -count=1` passed.

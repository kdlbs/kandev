---
id: "03-activity-gate-scheduler"
title: "Activity gate and scheduler core"
status: done
wave: 2
depends_on: ["01-durable-task-cleanup", "02-storage-persistence"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 03: Activity gate and scheduler core

Coordinate destructive maintenance with task, shell, script, test, and Docker-build activity.

## Acceptance

- Resource leases cover every activity named by the spec and maintain a deterministic quiet-period
  clock.
- Maintenance acquisition rechecks under lock; new task work cancels and drains a maintenance lease
  before admission.
- Disabled scheduling starts no destructive loop, the first enabled run waits a full interval, and
  busy intervals persist `skipped_busy`.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/activity ./internal/agent/runtime/lifecycle ./internal/system/storage
```

## Files likely touched

- `apps/backend/internal/agent/runtime/activity/coordinator.go`
- `apps/backend/internal/agent/runtime/activity/coordinator_test.go`
- lifecycle launch, prepare, stop, shell/process, script, and Docker-build call sites
- `apps/backend/internal/system/storage/service.go`
- `apps/backend/internal/system/storage/runner.go`
- `apps/backend/internal/system/storage/scheduler.go`

## Dependencies

- Task 01 supplies durable lifecycle cleanup behavior.
- Task 02 supplies settings and run persistence.

## Inputs

- Spec What and Scheduled maintenance state machine
- Lifecycle `Start`/`Stop` ownership conventions in `apps/backend/AGENTS.md`
- Existing System `jobs.Tracker` behavior

## Output contract

Report instrumented activity sources, race/cancellation tests, scheduler behavior, tests run,
blockers/risks, and update this task plus `plan.md` to done.

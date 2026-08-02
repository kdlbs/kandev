---
id: "04-shutdown-composition"
title: "Stop schedulers before database cleanup"
status: done
wave: 3
depends_on: ["01-owned-loop-lifecycles", "03-office-maintenance-separation"]
plan: "plan.md"
spec: "../../specs/tasks/run-scheduling.md"
---

# Task 04: Stop schedulers before database cleanup

## Acceptance

- Backend startup retains owned handles for the runs scheduler and cron loop;
  no scheduler is launched as an untracked `go Start(ctx)` call.
- Graceful shutdown stops and joins both loops before orchestrator/runtime
  teardown and before repository/database cleanup.
- Regression coverage proves an in-progress database-backed tick finishes
  before the close marker, scheduler stop errors affect `error_count`, and no
  scheduler log can appear after the completion log.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp ./internal/runs/scheduler ./internal/scheduler/cron ./internal/office/service
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/cron.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/backendapp/shutdown_test.go`

## Dependencies

- Task 01 provides owned Stop-and-wait loops.
- Task 03 provides the final run/cron composition returned from startup.

## Parallelism

`sequential`; this is the integration task and owns shared backend startup and
shutdown composition.

## Inputs

- Spec: failure modes and shutdown scenarios.
- Plan: Graceful shutdown composition.
- Confirmed root cause: cleanup callbacks run in reverse registration order,
  while root context cancellation was registered before database cleanup and
  therefore executed after it.

## Risks

- Stop schedulers before stopping the orchestrator they dispatch into.
- Preserve startup-failure cleanup through the existing root-context fallback.
- Do not close the logger before final scheduler-stop diagnostics are emitted.

## Output contract

Report the shutdown ordering, files changed, test command/result, blockers,
remaining risks, and update this task plus `plan.md` status in the same
conversation.

## Result

- Backend startup now retains an owned scheduling runtime for the runs and
  cron loops, with an idempotent cleanup fallback for startup failures.
- Graceful shutdown stops and joins cron/run scheduling before the
  orchestrator, lifecycle manager, and database cleanup; scheduler stop errors
  are included in `error_count`.
- Added a blocking shutdown regression proving database cleanup cannot run
  before scheduler stop completes.
- Verification: `cd apps/backend && go test -race ./internal/backendapp ./internal/runs/scheduler ./internal/scheduler/cron ./internal/office/service -count=1` passed.

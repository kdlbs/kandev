---
id: "01-owned-loop-lifecycles"
title: "Own scheduler loop lifecycles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/run-scheduling.md"
---

# Task 01: Own scheduler loop lifecycles

## Acceptance

- `runs/scheduler.Scheduler` and `scheduler/cron.Loop` each start at most one
  owned goroutine and expose idempotent stop behavior.
- Stop cancels future ticks and blocks until the currently executing processor
  tick or cron handler fan-out has returned.
- Parent-context cancellation drains through the same wait-group path without
  producing post-stop calls.

## Verification

```bash
cd apps/backend && go test ./internal/runs/scheduler ./internal/scheduler/cron
```

## Files likely touched

- `apps/backend/internal/runs/scheduler/scheduler.go`
- `apps/backend/internal/runs/scheduler/scheduler_test.go`
- `apps/backend/internal/scheduler/cron/cron.go`
- `apps/backend/internal/scheduler/cron/cron_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` relative to Task 02 only: the file sets are disjoint and no
schema or shared contract file changes.

## Inputs

- Spec: scheduler lifecycle and shutdown scenarios.
- Plan: Owned scheduler lifecycles.
- Pattern: `internal/integrations/healthpoll.Poller` Start/Stop ownership.

## Risks

- Avoid waiting while holding the lifecycle mutex.
- Preserve the under-100ms signal-driven dispatch test.

## Output contract

Report the lifecycle behavior, files changed, test command/result, blockers,
remaining risks, and update this task plus `plan.md` status in the same
conversation.

## Result

- `Scheduler` and `cron.Loop` now own their loop goroutines and expose
  idempotent `Start`/`Stop` methods; `Stop` waits for active processor ticks
  and cron handler fan-out.
- Added channel-controlled tests for duplicate starts, active-work draining,
  and parent-context cancellation.
- Verification: `cd apps/backend && go test -race ./internal/runs/scheduler ./internal/scheduler/cron -count=1` passed.

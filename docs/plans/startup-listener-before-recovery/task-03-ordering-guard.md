---
id: "03-ordering-guard"
title: "Guard the bind-before-startup ordering against regression"
status: done
wave: 3
depends_on: ["01-bind-before-startup", "02-gate-routes-while-starting"]
plan: "plan.md"
spec: "../../specs/startup-listener-before-recovery/spec.md"
parallelism: sequential
---

# Task 03: Guard the Ordering Against Regression

## Intent

The defect is an ordering invariant, and ordering invariants rot silently. The
existing `startup_order_test.go` pins orchestrator-before-automation-before-
GitHub but says nothing about the bind. Pin the new invariant, and make the
cost of a slow startup visible instead of mysterious.

## Acceptance

- `apps/backend/internal/backendapp/startup_order_test.go` is extended to
  assert that binding happens before orchestrator startup, in the same style as
  the existing ordering assertions.
- Startup logs make a long start diagnosable without a goroutine dump: log when
  the socket is bound, and when startup reconciliation begins and ends, with
  elapsed time and the number of tasks the lifecycle sweep will process.
- A sweep still running after a generous interval logs at `warn` with the count
  outstanding, so an operator sees "still recovering N tasks" rather than
  silence. This is a log-only signal; it must not cancel anything, because
  `context.WithoutCancel` means cancellation would not work
  (`task_operations.go:2308`).
- The spec's follow-up section is carried into a tracked issue or a new spec
  stub: bulk startup recovery inheriting the interactive 15-minute
  `AgentLaunchTimeout` per task (`timeouts.go:44`) is a real defect this plan
  deliberately does not fix.

## Regression test (write first, must fail)

Extend `apps/backend/internal/backendapp/startup_order_test.go` with an
assertion that the bind step is recorded before the orchestrator start step.
It fails against the pre-Task-01 ordering.

## Files likely touched

- `apps/backend/internal/backendapp/startup_order_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` (sweep
  start/end logging only; no behaviour change)

## Validation

```bash
cd apps/backend
go test ./internal/backendapp/... ./internal/orchestrator/... -count=1
make -C . lint
```

## Notes

Keep the sweep logging cheap. It runs before the watcher starts, so it cannot
publish events; plain structured logs are the right level here.

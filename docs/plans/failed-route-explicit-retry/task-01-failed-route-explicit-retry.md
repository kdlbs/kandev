---
id: "01-failed-route-explicit-retry"
title: "Allow explicit retry of a failed dynamic route"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.15
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Allow explicit retry of a failed dynamic route

## Summary

Let an explicit Retry recover a FAILED dynamic session whose route generation
is still `starting` with no owner, while preserving the stopped runtime history
row that normal cleanup keeps.

## In scope

- `dynamic_failed_retry.go` (new): `prepareFailedDynamicRouteRetry` and the
  stopped-history predicate.
- `service.go`: call it from `ApplyRouteAction` after the active-turn check.
- `dynamic_launch.go`: `resetDynamicRelaunchSessionForRecovery` re-checks the
  generation and allows FAILED to WAITING_FOR_INPUT for a manual prompt
  relaunch.
- `task_operations.go`: `scheduleTaskForSession` admits a FAILED task.

## Out of scope

Passive or timer reclamation, executor record deletion, retry policy, and the
backend route-action handler.

## Acceptance

1. Retry with the current generation on a FAILED session with a `starting`
   route, no live runtime, and only its stopped local history row marks the
   generation `action_required` and the retry proceeds; the row is preserved.
2. A live runtime, an absent executor row, or a non-stopped executor row returns
   `dynamic_route_pending` and leaves the route `starting`.
3. The manual relaunch reset reopens FAILED only for a manual prompt relaunch
   of the current generation.
4. A FAILED task can be rescheduled for the retried session.

## Verification

- `go test ./internal/orchestrator/ -run 'TestApplyRouteActionRetryRecoversFailedStartingRoute|TestScheduleTaskForSessionAdmitsRetryOfFailedTask'`
  fails on the base revision (`dynamic route recovery is pending`; `task ... is
  FAILED; cannot schedule session`) and passes with the change.
- `TestResetDynamicRelaunchSessionForRecoveryReopensOnlyManualFailedRetry`.
- Existing orchestrator package tests.

## Files likely touched

- `apps/backend/internal/orchestrator/dynamic_failed_retry.go` (new)
- `apps/backend/internal/orchestrator/dynamic_failed_retry_test.go` (new)
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-02.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
- [Plan](plan.md)

## Risks

Treating a live launch as stranded would duplicate a runtime; reopening a
terminal session from an automatic path could resurrect a cancelled task. Both
are fenced and covered by negative cases.

## Parallelism

`sequential`

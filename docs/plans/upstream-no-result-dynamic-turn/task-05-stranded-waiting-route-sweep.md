---
id: "05-stranded-waiting-route-sweep"
title: "Sweep a stranded WAITING_FOR_INPUT dynamic route at startup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.14
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---
# Task 05: Sweep a stranded WAITING_FOR_INPUT dynamic route at startup

## Summary

A session parked in WAITING_FOR_INPUT whose route is still `starting` and which has no `executors_running` row is never recovered: the startup orphan-route sweep skips it, and the ordinary session UI cannot release the route while no runtime backs it.
Let the sweep mark that session `action_required`, keeping every other exclusion.

## Evidence and cause

- `reconcileOrphanedDynamicStartingRoute` (`orchestrator/dynamic_policy_recovery.go`) returns early unless `isOrphanableDynamicSessionState` holds, which accepts only STARTING and IDLE.
- Task 04's `completeReadyStrandedStartingRoute` handles the opposite branch, a live prompt-ready runtime, so it does not recover a route without a runtime.

## In scope

- `apps/backend/internal/orchestrator/dynamic_policy_recovery.go`: `dynamicStartingRouteIsStranded` and the sweep condition.
- `apps/backend/internal/orchestrator/dynamic_route_recovery_test.go`: drop WAITING_FOR_INPUT from the non-orphan state list.
- `apps/backend/internal/orchestrator/dynamic_stranded_starting_route_test.go`: cover the missing-row and live-executor branches through the durable startup sweep.

## Out of scope

- A periodic sweep; the sweep still runs only at startup.
- Changes to resume-ready activation and explicit recovery from Task 04.
- Frontend changes.

## Acceptance

- A WAITING_FOR_INPUT session on a `starting` route with no executor row becomes `action_required` with its route; the session state is unchanged.
- A live executor row, a read error, a stale generation, and other parked states leave the session and route unchanged.

## Tests

| AC suffix | Test file and test |
| --- | --- |
| 001.14 | `apps/backend/internal/orchestrator/dynamic_stranded_starting_route_test.go`: `TestReconcileOrphanedDynamicStartingRoutes_SweepsStrandedWaitingRoute`, `TestReconcileOrphanedDynamicStartingRoutes_KeepsWaitingRouteWithLiveExecutor` |

## Validation

```bash
cd apps/backend
go test ./internal/orchestrator -run 'TestReconcileOrphanedDynamicStartingRoutes' -count=1
```

## Implementation result

The startup predicate and durable-sweep regression tests are implemented. The
existing generation and status fences remain authoritative, and the projection
write leaves the session state unchanged. Current-head hosted verification and
review outcomes are tracked externally in the owning task's recovery evidence;
implementation completion does not declare those delivery gates complete.

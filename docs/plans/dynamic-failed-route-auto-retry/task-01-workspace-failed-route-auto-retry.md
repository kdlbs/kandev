---
id: "01-workspace-failed-route-auto-retry"
title: "Workspace opt-in automatic failed-route retry"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.27
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.28
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Workspace opt-in automatic failed-route retry

## Summary

Add `dynamic_failed_route_auto_retry_enabled` (default off) and
`dynamic_failed_route_auto_retry_max` (1 to 3, default 1) to workspaces. When
the setting is on, a provider-capacity failure that leaves a dynamic task route
in `action_required` or `waiting` without an eligible candidate schedules one
automatic Retry through the existing route-action transaction.

## In scope

- Workspace model, SQLite columns and migrations, validation, partial and
  full-row updates, HTTP and WebSocket update requests, DTO, workspace events,
  and boot state.
- `dynamic/failed_route_retry.go`: the durable chain in the route policy state,
  due-time rule, per-generation schedule, consumption, carry to new
  generations, and reset.
- `dynamic_failed_route_retry.go`: the failure and mid-turn hooks, the shared
  recovery timer, restart re-arm, admission, and the chain reset on a completed
  turn.
- The resolver's `auto_retry` action with the `auto_failed_route_retry` reason,
  and Retry reselection for a waiting route.
- Workspace settings toggle and limit, shown while dynamic agent routing is
  enabled, with locale copy for every shipped locale.

## Out of scope

FAILED sessions, launch failures, account-block holds, non-capacity and
unclassified codes, Office routing, utility invocations, a new runtime flag,
and a pending-retry indicator on the recovery banner.

## Acceptance

1. With the setting off, an exhausted route keeps manual recovery.
2. With the setting on, the failure schedules one retry due at the later of one
   minute and the earliest usable circuit expiry, and exactly one automatic
   `retry` runs at that time.
3. The limit, a repeated same-candidate same-code failure, a stale generation,
   a superseded execution, an ineligible code, a duplicate failure, and the
   setting turned off before the retry do not submit another retry.
4. A pending retry is re-armed after restart; a consumed one is not.
5. With mid-turn failover also on, its Try next runs first and the retry
   applies only after the route is waiting.
6. The settings default to off and 1, round-trip through storage and the API,
   keep omitted values, and reject a limit outside 1 to 3.

## Verification

- `go test ./internal/orchestrator/ -run FailedRouteAutoRetry`
- `go test ./internal/agent/runtime/dynamic/ -run FailedRouteRetry`
- `go test ./internal/agent/runtime/ -run "AutoRetry|ScheduleFailedRouteRetry|AutoTryNext"`
- `go test ./internal/backendapp/ -run ResolverRouteAction`
- `go test ./internal/task/repository/sqlite/ -run WorkspaceFailedRouteAutoRetry`
- `vitest` for `workspace-dynamic-routing-auto-retry.test.tsx` and
  `workspace-edit-save-auto-retry.test.ts`
- Playwright `e2e/tests/settings/workspace-failed-route-auto-retry.spec.ts`
- `validateCoverage` returns `covered` for the changed paths and this package.

## Files likely touched

- `apps/backend/internal/agent/runtime/dynamic/failed_route_retry.go` (new)
- `apps/backend/internal/agent/runtime/dynamic/engine.go`, `circuit.go`, `types.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver_failed_route_retry.go` (new)
- `apps/backend/internal/orchestrator/dynamic_failed_route_retry.go` (new)
- `apps/backend/internal/orchestrator/dynamic_policy_recovery.go`,
  `dynamic_midturn_failover.go`, `dynamic_launch.go`, `event_handlers_agent.go`
- `apps/backend/internal/backendapp/dynamic_routing.go`, `boot_state_routes.go`
- `apps/backend/internal/task/` workspace model, DTO, handlers, service, and
  SQLite repository files
- `apps/web/app/settings/workspace/` edit form, save handler, draft, and save
  contributor
- `apps/web/components/settings/workspaces/workspace-dynamic-routing-card.tsx`
- `apps/web/src/locales/*/workspaces.json`
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-02.md`

## Dependencies

The mid-turn provider failover work (automatic route actions and the workspace
dynamic routing card) and the existing manual Retry transaction.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Safety contract](../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
- [Plan](plan.md)

## Risks

An automatic retry repeats provider work; the setting is off by default and
capped. The retry is at most once across a crash.

## Parallelism

`sequential`

---
id: "01-workspace-midturn-failover"
title: "Workspace opt-in mid-turn provider failover"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.17
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.22
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Workspace opt-in mid-turn provider failover

## Summary

Add `dynamic_midturn_failover_enabled` to workspaces, default off. When it is
on, a classified provider-capacity failure of a current dynamic task attempt
that already produced output or effects performs one automatic Try next per
failed route generation through the existing route-action transaction.

## In scope

- Workspace model, SQLite column and migration, partial and full-row updates,
  HTTP and WebSocket update requests, DTO, workspace events, and boot state.
- `dynamic_midturn_failover.go`: qualifying codes, current-attempt fence,
  per-generation and per-execution claim, repeated-failure stop, chain reset on
  turn completion, and the detached worker that reuses the route-action lock
  and settled-turn check.
- `RouteActionRequest.Automatic`, the resolver's `auto_try_next` action with
  the `auto_provider_failover` reason, and the automatic launch origin.
- Workspace settings toggle, shown while dynamic agent routing is enabled, with
  locale copy for every shipped locale.

## Out of scope

New provider classification, candidate order or circuit changes, a new runtime
flag, Office routing, utility invocations, and durable chain persistence.

## Acceptance

1. With the setting off, a mid-turn classified rate limit keeps manual recovery.
2. With the setting on, the same failure submits exactly one automatic
   `try_next` for the failed generation, and the successor's continuation is the
   bounded durable package.
3. A repeated same-code failure on a candidate already tried in the chain, an
   exhausted candidate list, a stale generation, a superseded execution, another
   prompt generation, unknown evidence, or a duplicate delivery does not advance
   the route.
4. Unclassified, user, runtime, transport, credential, and account-restriction
   failures keep their existing behavior.
5. The setting defaults off, round-trips through storage and the API, and an
   omitted update keeps the saved value.

## Verification

- `go test ./internal/orchestrator/ -run "MidTurn|ApplyAutomaticRouteAction"`
- `go test ./internal/agent/runtime/ -run AutoTryNext`
- `go test ./internal/backendapp/ -run ResolverRouteAction`
- `go test ./internal/task/repository/sqlite/ -run WorkspaceMidTurnFailover`
- `vitest` for `workspace-dynamic-routing-card.test.tsx` and
  `workspace-edit-save-midturn.test.ts`
- Playwright `e2e/tests/settings/workspace-midturn-failover.spec.ts`
- `validateCoverage` returns `covered` for the changed paths and this package.

## Files likely touched

- `apps/backend/internal/orchestrator/dynamic_midturn_failover.go` (new)
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/backendapp/dynamic_routing.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/task/` workspace model, DTO, handlers, service, and
  SQLite repository files
- `apps/web/app/settings/workspace/workspace-edit-client.tsx`
- `apps/web/app/settings/workspace/workspace-edit-save.ts`
- `apps/web/components/settings/workspaces/workspace-dynamic-routing-card.tsx` (new)
- `apps/web/src/locales/*/workspaces.json`
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-02.md`

## Dependencies

None. The manual Try next transaction and continuation package already exist.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Safety contract](../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
- [Plan](plan.md)

## Risks

An automatic handoff after effects can repeat work; the successor must inspect
durable state first, and the setting is off by default. The chain is
process-local and is forgotten on restart.

## Parallelism

`sequential`

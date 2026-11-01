---
id: "01-circuit-backoff-escalation"
title: "Escalate circuit backoff for repeated failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Escalate circuit backoff for repeated failures

## Summary

Behind `features.dynamicCircuitBackoffEscalation`, lengthen the circuit backoff
for consecutive qualifying failures of one resource and persist the count.

## In scope

- `CircuitRegistry.OpenFailure`, the ladder, the same-incident rule, probe
  advance, and restore of the count.
- `ResetCircuitFailures` on the engine and resolver, called from the
  orchestrator when a turn completes without a provider error.
- `routingerr.Error.ResetHintInferred` for clock-only reset notices.
- `dynamic_resource_circuits.failure_count` with an additive migration and UTC
  deadlines.
- The runtime flag in config, registry, `profiles.yaml`, frontend defaults, and
  `docs/public/configuration.md`, wired into the circuit registry at startup.

## Out of scope

New UI, per-candidate ladder settings, qualifying-code changes, and probe lease
changes.

## Acceptance

1. Flag off: every open and failed probe uses the fixed minute and persisted
   counts are ignored.
2. Flag on: repeated opens follow 1m, 5m, 15m, 30m, 1h, 2h, 4h (cap); a
   failure inside the open window does not advance the count.
3. A failed probe advances the count; a successful probe keeps it, so a
   launch followed by a mid-turn failure, three times, opens for 1m, 5m, then
   15m; a clean turn completion resets the next open to 1m.
4. A later reset hint wins; an inferred hint is capped at 24 hours; an explicit
   provider reset time is not.
5. The count round-trips through the store, survives restart, and migrates in
   with a zero default.

## Verification

- `go test ./internal/agent/runtime/dynamic/ ./internal/runtimeflags/ ./internal/profiles/`
- `go test ./internal/task/repository/sqlite/ -run 'Dynamic|Circuit|Route'`
- `go test ./internal/persistence/storeconformance/ -run AddsCircuitFailureCount`
  (PostgreSQL runs only with `KANDEV_TEST_POSTGRES_DSN`)
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  and `validateCoverage` for the changed paths.

## Files likely touched

- `apps/backend/internal/agent/runtime/dynamic/circuit.go`
- `apps/backend/internal/agent/runtime/dynamic/circuit_backoff.go` (new)
- `apps/backend/internal/agent/runtime/dynamic/engine.go`
- `apps/backend/internal/agent/runtime/dynamic/failure_reset.go` (new)
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/dynamic_failure_attribution.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/task/repository/sqlite/dynamic_route.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/backendapp/services.go`
- `apps/web/lib/state/slices/features/types.ts`
- `docs/public/configuration.md`
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-01.md`
- `docs/plans/dynamic-circuit-backoff-escalation/plan.md`
- `docs/plans/dynamic-circuit-backoff-escalation/task-01-circuit-backoff-escalation.md`

## Dependencies

Stacked on the mid-turn circuit change in `docs/plans/dynamic-midturn-circuit-open/`.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-01.md)
- [Plan](plan.md)

## Risks

A recovered candidate can stay skipped up to four hours; concurrent failures of
one incident must not climb the ladder at once.

## Parallelism

`sequential`

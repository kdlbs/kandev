---
id: "01-midturn-circuit-open"
title: "Open the candidate circuit for mid-turn provider failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Open the candidate circuit for mid-turn provider failures

## Summary

Charge a qualifying classified failure that may not advance the route to the
shared circuit of the candidate that produced it, so later selections skip the
unhealthy candidate while the failed turn keeps its recovery surface.

## In scope

- `Engine.RecordFailureCircuit` fences the failure to the current route
  generation and candidate and opens the qualifying circuit.
- `ProfileExecutionResolver.RecordFailureCircuit` loads the logical profile and
  calls the engine.
- `routeDynamicAgentFailureWithEvidence` calls it on its ineligible branch.

## Out of scope

Route advancement, successor launch, generation or status changes, backoff
policy, and a runtime flag.

## Acceptance

1. A mid-turn qualifying failure opens the candidate's circuit and the next
   selection skips it.
2. The route generation, candidate, and status stay unchanged and the call
   reports the failure as not routed.
3. A stale generation, an old candidate, or a predecessor execution opens no
   circuit.
4. An unclassified or non-qualifying failure opens no circuit, and an account
   block is still held only through the existing hold.

## Verification

- `go test ./internal/agent/runtime/dynamic/ ./internal/agent/runtime/`
- `go test ./internal/orchestrator/ -run 'Dynamic|AccountBlock'`
- With the orchestrator call removed, the mid-turn test fails with a closed
  circuit; with it, all tests pass.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  and `validateCoverage` for the changed paths.

## Files likely touched

- `apps/backend/internal/agent/runtime/dynamic/failure_circuit.go` (new)
- `apps/backend/internal/agent/runtime/dynamic/failure_circuit_test.go` (new)
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/orchestrator/dynamic_failure_attribution.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/dynamic_midturn_circuit_test.go` (new)
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-02.md`
- `docs/plans/dynamic-midturn-circuit-open/plan.md`
- `docs/plans/dynamic-midturn-circuit-open/task-01-midturn-circuit-open.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
- [Plan](plan.md)

## Risks

Charging a successor for its predecessor's failure, or opening a circuit for a
non-provider error, would disable a healthy candidate.

## Parallelism

`sequential`

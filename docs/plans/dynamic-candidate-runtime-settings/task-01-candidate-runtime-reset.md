---
id: "01-candidate-runtime-reset"
title: "Drop previous candidate runtime settings on a switch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.18
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Drop previous candidate runtime settings on a switch

## Summary

When a dynamic session moves to a different concrete candidate, stop the next
candidate from launching with the model, permission mode, and ACP options that
the previous provider reported.

## In scope

- `orchestrator/dynamic_runtime_config.go` (new): switch detection, the guarded
  runtime reset, and workflow-declared mode lookup.
- `persistDynamicLaunchDecision` and the launch-resolution projections use the
  reset when the concrete profile changes.
- The manual route action selection uses the exported switch persister, wired in
  `backendapp/main.go`.
- `sessionSettingsEventIsStale` rejects mode/model events from another concrete
  profile.

## Out of scope

Provider mode mapping, profile defaults, explicit overrides, ACP selector
snapshots, and repair of sessions that already failed.

## Acceptance

1. After a candidate switch the effective runtime config carries neither the
   previous model nor the previous mode; explicit overrides remain.
2. A workflow step's declared session mode is restored after the switch.
3. A same-candidate retry keeps its runtime settings.
4. A late mode or models event from the previous candidate does not restore its
   mode or model.
5. A switch against a changed session revision is rejected.

## Verification

- The tests listed in the [plan](plan.md#verification) fail on the previous
  source where they cover changed behavior, and pass with this change.
- Orchestrator and backendapp package results match the base revision apart
  from the new tests.

## Files likely touched

- `apps/backend/internal/orchestrator/dynamic_runtime_config.go` (new)
- `apps/backend/internal/orchestrator/dynamic_candidate_runtime_settings_test.go` (new)
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/backendapp/dynamic_routing.go`
- `apps/backend/internal/backendapp/main.go`
- `docs/specs/agents/requirements/dynamic-agent-routing.md`
- `docs/specs/agents/system-design/dynamic-agent-routing-01.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-01.md)
- [Plan](plan.md)

## Risks

Removing explicit overrides, fencing events without an execution profile, or
overwriting a concurrently updated session would each regress existing
behavior.

## Parallelism

`sequential`

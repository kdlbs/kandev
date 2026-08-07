---
id: "02-original-session-initialization"
title: "Capture the original session configuration before workflow overrides"
status: done
wave: 2
depends_on: ["01-workflow-contract-and-validation"]
plan: "plan.md"
spec: "../../specs/workflow-session-settings/spec.md"
---

# Task 02: Capture the Original Session Configuration Before Workflow Overrides

## Acceptance

- The first task session receives immutable original-session provenance; later primary/session switches cannot change that identity.
- A conservative legacy resolver identifies an original only when the earliest non-workflow-switch session is unambiguous.
- The original effective model and every advertised select option are persisted once after profile settings settle and before any workflow/runtime override layer applies.
- The new snapshot remains distinct from `acp_config_baseline`, `runtime_config`, `runtime_config_overrides`, and mutable `AgentProfileSnapshot` fields.
- ACP initialization accepts separate profile and runtime/workflow layers, preserves existing override/resume behavior, records the original snapshot synchronously, and applies later layers before the first prompt.
- Process recreation, reset, and backend restart never replace the snapshot.

## TDD sequence

1. Add failing metadata helper, executor provenance, legacy-resolution, and write-once tests.
2. Add failing lifecycle tests that assert provider default -> profile -> original capture -> override -> prompt ordering.
3. Split the initialization inputs into typed layers and add a narrow recorder callback/interface.
4. Re-run existing model-selection, config-option, reset, and resume tests to catch ordering regressions.

## Verification

```bash
cd apps/backend && go test ./internal/task/models ./internal/task/service ./internal/orchestrator/executor ./internal/agent/runtime/...
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/models_test.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_profile.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- Related lifecycle, executor, service, and streaming tests

## Dependencies

Task 01.

## Inputs

- ADR `2026-08-01-workflow-session-original-configuration`.
- Existing provider baseline and runtime override metadata helpers.
- Existing `PrepareSession`, `effectiveSessionRuntimeConfig`, `InitializeAndPrompt`, and `publishSettledConfigOptions` flow.

## Output contract

Implemented with `origin=task_initial` provenance and the write-once `original_effective_config` snapshot. The first task session is identified by creation order rather than mutable primary ownership; legacy sessions use the earliest unambiguous non-workflow-switch row. ACP initialization now applies profile settings, emits the profile-settled original snapshot, then applies runtime overrides before the first prompt. Snapshot persistence is compare-and-set and survives later provider events and restarts. Focused lifecycle, executor, streaming, and orchestrator tests passed.

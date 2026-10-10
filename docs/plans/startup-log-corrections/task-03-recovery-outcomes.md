---
id: "03-recovery-outcomes"
title: "Recovery outcome attribution"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 03: Recovery outcome attribution

## Summary

Explain backend recovery omissions using evidence from the enumeration pass.
Keep progress, stop decisions, guards, inventory, and session state unchanged.

## In scope

- Add optional detailed recovery results and registry aggregation without
  changing the existing ExecutorBackend contract for non-participating backends.
- Standalone reports no matching instance only after successful enumeration;
  enumeration failure, proven correlation refusal, and deadline/cancel outcomes
  remain distinct where evidence is available.
- Account by backend and exact candidate identity; Manager reconstruction wins
  over provisional producer outcomes for returned instances.
- Keep missing/unreported/plugin outcomes unknown and reset reports per pass.
- Cover zero/unknown inventory, mixed surviving and missing instances, failed
  enumeration, mixed runtimes, duplicates, deadline, and a repeated Start.

## Out of scope

- No cleanup or liveness inference, retry changes, process stops, fabricated
  N/N progress, or suppression of the below-total warning.
- No new provider recovery support or plugin wire-contract change.

## Acceptance

- The real standalone empty-enumeration fixture reports no matching instances;
  failed enumeration reports failure, not absence. Current code reports both as
  unknown, so the new regression must fail before implementation.
- Each known candidate is counted once across retracked, classified refusal,
  and unknown, including mixed backend and partial recovery fixtures.
- Existing recovery guards, retained outcomes, record preservation, and progress
  tests pass; backend implementations without detailed reports still work.

## Verification

Run the regression first and record its expected failure, then implement and run:

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/startup -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'RecoverySummary|RecoverAll|StandaloneExecutorRecoverInstances|ManagerStart|RecoveryGuard' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_registry.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_correlation.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_outcomes_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_sessions_step_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_registry_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_test.go`

## Dependencies

None. Execute in manifest order in the primary session.

## Risks

A successful empty control-server inventory proves no matching managed instance there, not process death. Never classify plugin rows from standalone evidence; background stop completion must not mutate the completed pass report.

## Parallelism

`sequential`

## Inputs

- [Plan and incident evidence](plan.md).
- [Requirement REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001](../../specs/platform/requirements/runtime-failure-attribution.md).
- [System design](../../specs/platform/system-design/runtime-failure-attribution.md).

## Results

- `DetailedRecoveryBackend` and `RecoverInstancesDetailed` implemented on `StandaloneExecutor`, classifying `no_matching_instance` and `enumeration_failed` outcomes.
- `ExecutorRegistry.RecoverAllDetailed` aggregates candidate outcomes only for backends that own each record.
- `recoveryOutcomeSummary` includes `not_retracked_no_matching_instance` and `not_retracked_enumeration_failed` fields, ensuring candidate count equals retracked plus classified not-retracked plus unknown.
- Tests in `manager_recovery_outcomes_test.go` and `manager_recovery_sessions_step_test.go` pass with race detector enabled.

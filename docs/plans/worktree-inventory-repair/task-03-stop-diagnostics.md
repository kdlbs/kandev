---
id: "03-stop-diagnostics"
title: "Classify absent executions before failed-stop warnings"
status: complete
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-003.2
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 03: Classify absent executions before failed-stop warnings

## Summary

Preserve the existing idempotent stop classification while removing a misleading
warning emitted before the result is classified. Real runtime teardown failures
must keep their warning and error identity.

## In scope

- `Executor.StopExecution` classification before logging.
- Wrapped lifecycle and public runtime absence sentinels; preserve caller-visible
  error-chain compatibility. Keep empty-ID behavior separate from confirmed absence.
- Zap observer tests and the cleanup stop boundary's refusal on real failures.

## Out of scope

Session-level liveness policy, string matching, broad log suppression, or changing
stop execution into unconditional success.

## Acceptance

1. A failing regression first proves that a wrapped typed absence emits WARN;
   after the fix it emits no failed-stop warning and remains classifiable.
2. Transport failure, deadline, and generic error controls still produce WARN
   and remain real failures at the cleanup boundary. A matching message string
   without a typed sentinel does not count as absence.

## Verification

```bash
(cd apps/backend && go test -race ./internal/orchestrator/executor -run 'TestStopExecution' -count=1)
(cd apps/backend && go test -race ./internal/task/service -run 'Stop|Cleanup.*(Runtime|Execution|Manifest)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- Proposed `apps/backend/internal/orchestrator/executor/executor_stop_diagnostics_test.go`
- Existing `executor_interaction_test.go` for classification fixtures; use a new
  test file because the existing file exceeds the new-file size guidance
- A focused task-service cleanup regression file only if needed for the boundary

## Dependencies

None technically; execute after Task 02 in this package's sequential sequence.
Read `StopExecution`, `runtimeStopAlreadyComplete`, and
`TestStopExecution_PreservesRuntimeFailureClassification` before writing the regression.

## Risks

An unrelated stop error must never acquire the public runtime not-found sentinel.

## Parallelism

`sequential`

## Results

The new observer regression first reproduced WARN for wrapped lifecycle and public runtime absence. Classification now precedes WARN and preserves typed error chains. Race tests for TestStopExecution and focused cleanup runtime/source-manifest boundaries pass; transport, deadline, and matching-text errors remain failures.

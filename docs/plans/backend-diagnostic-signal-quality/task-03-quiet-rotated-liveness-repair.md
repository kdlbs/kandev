---
id: "03-quiet-rotated-liveness-repair"
title: "Quiet rotated liveness repair"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/runtime-cleanup.md"
---

# Task 03: Quiet rotated liveness repair

Treat a lost liveness compare-and-set as expected reconciliation convergence.

## Scope

- Classify `models.ErrExecutionRotated` before the warning branch.
- Return success without changing the newer execution row.
- Keep the warning and returned error for unrelated repair failures.

## Exclusions

- Do not change the compare-and-set repository operation.
- Do not retry against the newer execution identity.
- Do not change row deletion or resume-safety rules.

## Traceability

- `REQ-TASKS-RUNTIME-CLEANUP-001`
- `AC-TASKS-RUNTIME-CLEANUP-001.8`
- Design: `docs/specs/tasks/system-design/runtime-cleanup.md`

## Acceptance

- `models.ErrExecutionRotated` returns success and emits no warning.
- `models.ErrExecutorRunningNotFound` keeps its existing successful behavior.
- An unrelated repair error returns the error and emits one warning.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator -run TestRepairDeadRowLivenessLogSeverity -count=1
```

The rotated case must fail before the production change because the current
branch emits a warning before it returns success.

## Files likely touched

- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/reconcile_liveness_test.go`

## Dependencies

None.

## Results

Moved the expected `models.ErrExecutionRotated` and existing missing-row
classification ahead of the warning branch. The newer execution row remains
unchanged, while unrelated repair errors still return and emit one warning.

Verification:

```text
cd apps/backend && go test ./internal/orchestrator -run TestRepairDeadRowLivenessLogSeverity -count=1
Go test: 3 passed in 1 packages
```

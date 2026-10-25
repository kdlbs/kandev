---
id: "01-cleanup-inspection"
title: "Cleanup inspection failures"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-TASKS-RUNTIME-CLEANUP-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.9
  - AC-TASKS-RUNTIME-CLEANUP-001.32
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 01: Cleanup inspection failures

## Summary

Preserve Git inspection failures instead of translating them to absent branches.
Make existing cleanup warnings identify the failed stage and reason while keeping
all resources, snapshots, and retry behavior unchanged.

## In scope

- Correct `branchExists` missing-reference classification with quiet verification.
- Attribute branch/registration inspection failures and ownership/commit refusals.
- Propagate stage, reason, and attempt into existing cleanup worker diagnostics.
- Cover valid absence, missing executable/cwd, invalid repository, cancellation,
  deadline, changed commit, and another task's registration with isolated fixtures.

## Out of scope

- No live-data repair, worktree deletion outside temporary fixtures, new cleanup
  states, retries, or ownership rules.
- No unconditional “not found means removed” fallback.

## Acceptance

- A missing ref returns false without error; start failure and fatal Git errors
  return errors. The new regression fails on the existing non-context fallback.
- Mixed cleanup targets preserve conflicting/failed resources and their exact
  snapshots while unrelated safe targets can follow existing cleanup behavior.
- Error reasons survive wrapping, dirty-worktree policy still works, and bounded
  and cascade retry schedules retain existing behavior.

## Verification

Run the regression first and record its expected failure, then implement and run:

```bash
(cd apps/backend && go test ./internal/worktree ./internal/task/service -count=1)
(cd apps/backend && go test -race ./internal/worktree ./internal/task/service -run 'BranchExists|CleanupInspection|RetryTaskResourceCleanupJob|RetryCascadeCleanupJob|MissingWorktree' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `apps/backend/internal/worktree/manager_cleanup_inspection_test.go (new)`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_inspection_test.go (new)`

## Dependencies

None. Execute in manifest order in the primary session.

## Risks

Changing false-absence behavior affects other branch callers; keep real absent refs compatible. Generic ENOENT cannot prove that Git itself is missing.

## Parallelism

`sequential`

## Inputs

- [Plan and incident evidence](plan.md).
- [Requirement REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001](../../specs/platform/requirements/runtime-failure-attribution.md).
- [Requirement REQ-TASKS-RUNTIME-CLEANUP-001](../../specs/tasks/requirements/runtime-cleanup.md).
- [System design](../../specs/platform/system-design/runtime-failure-attribution.md).
- [System design](../../specs/tasks/system-design/runtime-cleanup.md).

## Results

- `branchExists` uses quiet ref verification and returns `(false, nil)` only on exit code 1 (verified absent ref), returning errors for startup failure, fatal exits, and context timeouts.
- `CleanupInspectionError` attaches structured stage and reason to inspection failures during branch and registration audits.
- Task cleanup workers log inspection stage, reason, and attempt without logging sensitive command output or raw environments.
- Tests in `manager_cleanup_inspection_test.go` and `resource_cleanup_inspection_test.go` pass with race detector enabled.

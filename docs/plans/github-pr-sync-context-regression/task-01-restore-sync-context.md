---
id: "01-restore-sync-context"
title: "Restore GitHub PR sync context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 01: Restore GitHub PR Sync Context

## Intent

Restore the missing task and pull-request context in the final sync helper.
Preserve the existing persistence, reconciliation, and publication order.

## Acceptance

- `apps/backend/internal/github` compiles without undefined identifiers.
- `SyncTaskPR` persists the task PR before comparison-target reconciliation.
- The comparison observer receives the same task ID and PR identity as the sync.
- Existing stale-writer event behavior remains unchanged.

## Red Test First

Add `TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation` in
`service_comparison_target_sync_test.go`. The current package must fail to
compile because the helper does not receive `taskID` or `status.PR`.

## Implementation

- Extend `persistAndPublishTaskPRSync` with `taskID string` and `pr *PR`.
- Pass `taskID` and `status.PR` from `SyncTaskPR`.
- Pass `tp.TaskID` and `status.PR` from unwatched lifecycle reconciliation.
- Use those parameters for `reconcileComparisonTargetFromSync`.
- Update direct helper calls in existing tests.
- Do not change storage, reconciliation, or event behavior.

## Files Likely Touched

- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_unwatched.go`
- `apps/backend/internal/github/service_comparison_target_sync_test.go`
- `apps/backend/internal/github/service_pr_outcome_sync_test.go`
- `docs/plans/github-pr-sync-context-regression/plan.md`
- `docs/plans/github-pr-sync-context-regression/task-01-restore-sync-context.md`

## Dependencies

None.

## Parallelism

`sequential`. The source and tests share one helper contract.

## Inputs

- `docs/specs/pr-outcome-attribution/spec.md`, section `Sync writer`
- `docs/specs/platform/workspace-git-status.md`, section
  `Repository-qualified comparison targets`
- `docs/decisions/2026-08-19-repository-qualified-comparison-targets.md`

## Verification

Run these commands from the repository root:

```bash
cd apps/backend && \
GOCACHE=/tmp/kandev-go-build-cache go test ./internal/github -run 'TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation|TestPersistAndPublishTaskPRSync_PublishesReReadValueNotStaleInMemoryOne' && \
GOCACHE=/tmp/kandev-go-build-cache make build
```

Then run this command from the repository root:

```bash
git diff --check
```

## Output Contract

Report the changed files, exact command results, blockers, and remaining risks.
Update this task and `plan.md` in the same conversation.

## Results

- RED: `GOCACHE=/tmp/kandev-go-build-cache go test ./internal/github -run
  TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation` failed with the
  expected undefined `taskID` and `status` compiler errors.
- GREEN: `GOCACHE=/tmp/kandev-go-build-cache go test ./internal/github -run
  TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation` passed.
- Targeted regression command passed:
  `GOCACHE=/tmp/kandev-go-build-cache go test ./internal/github -run
  "TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation|TestPersistAndPublishTaskPRSync_PublishesReReadValueNotStaleInMemoryOne"`.
- Backend build passed:
  `GOCACHE=/tmp/kandev-go-build-cache make build`.
- `gofmt -w` completed for all changed Go files.
- `git diff --check` passed.
- Build output was written to ignored files under `apps/backend/bin`.
- No temporary source files or E2E assets were created.

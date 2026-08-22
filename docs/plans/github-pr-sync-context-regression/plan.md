---
spec: docs/specs/pr-outcome-attribution/spec.md
created: 2026-08-22
status: implemented
---

# Implementation Plan: Restore GitHub PR Sync Context

## Overview

The pull-request outcome change extracted the final sync write into a helper.
The helper still uses `taskID` and `status.PR`, but its signature does not
receive either value. As a result, the GitHub package does not compile.

Pass the task and pull-request context into the helper. Keep the existing order:
persist the task PR, reconcile the comparison target, then publish the committed
row when semantic data changed.

## Confirmed Root Cause

Commit `9178780ba` extracted `persistAndPublishTaskPRSync` from `SyncTaskPR`.
The extracted body kept this call:

```go
s.reconcileComparisonTargetFromSync(ctx, taskID, status.PR)
```

The new helper signature does not receive `taskID` or `status`. The focused
backend build therefore stops with these compiler errors:

```text
internal/github/service_pr_watch.go:1024:43: undefined: taskID
internal/github/service_pr_watch.go:1024:51: undefined: status
```

Use this reproduction command when the default Go cache is read-only:

```bash
cd apps/backend && GOCACHE=/tmp/kandev-go-build-cache make build-kandev
```

## Backend

### Sync writer context

Update `apps/backend/internal/github/service_pr_watch.go`:

- Add the task ID and pull-request payload to
  `persistAndPublishTaskPRSync`.
- Pass `taskID` and `status.PR` from `SyncTaskPR`.
- Pass `tp.TaskID` and `status.PR` from unwatched lifecycle reconciliation.
- Keep comparison-target reconciliation after `UpdateTaskPR` succeeds.
- Keep event publication after comparison-target reconciliation.
- Return immediately when `UpdateTaskPR` fails.

### Regression coverage

Add `apps/backend/internal/github/service_comparison_target_sync_test.go`.
Use a comparison-target observer that reads the stored row during its callback.
The test proves that the write occurs first and that the observer receives the
correct task and pull-request identity.

Update the direct helper call in
`apps/backend/internal/github/service_pr_outcome_sync_test.go`. Pass explicit
task and pull-request context through the helper contract.

Update the unwatched lifecycle caller in
`apps/backend/internal/github/service_pr_unwatched.go` with the same context.

## Tests

- **What:** the GitHub package compiles after the helper receives its required
  context.
  **File:** `apps/backend/internal/github/service_pr_watch.go`.
  **How:** compile the package through the focused Go test command.
- **What:** a normal sync persists the task PR before comparison reconciliation.
  **File:** `apps/backend/internal/github/service_comparison_target_sync_test.go`.
  **How:** use a fake observer that reads the stored row in its callback.
- **What:** the helper still publishes the committed row after a stale write.
  **File:** `apps/backend/internal/github/service_pr_outcome_sync_test.go`.
  **How:** run the existing deterministic stale-writer regression test.

## Verification Results

- RED: `GOCACHE=/tmp/kandev-go-build-cache go test ./internal/github -run
  TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation` failed at the
  expected undefined `taskID` and `status` compiler errors.
- GREEN: the focused regression test passed.
- Targeted tests: the regression test and stale-writer test passed.
- Backend build: `GOCACHE=/tmp/kandev-go-build-cache make build` passed.
- Formatting: `gofmt -w` completed for all changed Go files.
- `git diff --check` passed.
- Build output was written to ignored files under `apps/backend/bin`.
- No temporary source files or E2E assets were created.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-restore-sync-context](task-01-restore-sync-context.md) (done)

The task is sequential because the source and tests share one package contract.

## Risks And Out Of Scope

- The fix does not change pull-request field sourcing or storage schemas.
- The fix does not change comparison-target matching rules.
- The fix does not change event payloads or event ordering.
- The fix does not change the root Makefile output suppression.
- The read-only default Go cache remains an environment concern.

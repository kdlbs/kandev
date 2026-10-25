---
id: "02-worktree-initialization-severity"
title: "Worktree initialization log severity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/expected-runtime-log-severity.md"
---

# Task 02: Worktree initialization log severity

## Acceptance

- `ErrEnvironmentNotResolved` remains a successful persistence skip and emits
  the existing structured message at debug level, not warning level.
- The manager does not remove or otherwise alter the newly materialized
  physical worktree on this expected path.
- Other store errors retain their existing cleanup and error behavior.

## Verification

```bash
cd apps/backend && go test ./internal/worktree -run 'TestPersistWorktree_(EnvironmentNotResolved|OtherStoreError)' -count=1
```

## Files likely touched

- `apps/backend/internal/worktree/manager_state.go`
- `apps/backend/internal/worktree/manager_state_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Spec: initial materialization and persistence failure scenarios.
- Existing `ErrEnvironmentNotResolved` contract in
  `apps/backend/internal/worktree/errors.go`.
- Existing `Manager.persistWorktree` cleanup boundary.

## Output contract

Report the severity change, persistence assertion, exact test result, files
changed, blockers, and task/plan status updates.

## Results

Changed only the `ErrEnvironmentNotResolved` branch in `persistWorktree` from
warning to debug severity. The branch still returns success without cleanup;
unrelated store errors remain wrapped and trigger the existing cleanup path.

- RED: `cd apps/backend && go test ./internal/worktree -run
  'TestPersistWorktree_(EnvironmentNotResolved|OtherStoreError)' -count=1`
  failed because the expected branch logged at `warn` instead of `debug`.
- GREEN: the same command passed after the severity change and after `gofmt`.

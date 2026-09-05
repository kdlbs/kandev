---
id: "01-isolate-reused-pr-refs"
title: "Isolate reused pull-request refs"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
---

# Task 01: Isolate reused pull-request refs

## Summary

Prepare each pull-request head through a PR-scoped internal ref. Preserve any
same-named local branch that contains different history.

## In scope

- Fetch and verify a pull-request head without updating the named local branch.
- Create a unique task branch when the named branch has unrelated history.
- Preserve Git admission, timeout, credential, and cancellation behavior.

## Out of scope

- Provider-history reconciliation after launch.
- Remote branch mutation.
- Changes to non-pull-request local branch refresh.

## Acceptance

- Two pull requests can reuse one head-branch name without a fetch conflict.
- The new worktree starts at the requested pull-request head.
- The prior local branch and its commit remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/worktree -race
```

## Files likely touched

- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_pr_head_collision_test.go`

## Dependencies

None.

## Risks

- Ref cleanup must not remove a user-owned ref.
- A local-ahead branch must keep its local-only commits.

## Parallelism

`sequential`

## Inputs

- Task launch failure recovery design, pull-request checkout isolation.
- Remote contribution task requirement `AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.2`.
- ADR `2026-08-10-remote-contribution-head-drift`.

## Results

- Added a PR-scoped `origin/pr/<N>` fetch ref with verification and typed
  `ErrWorkspaceCheckoutFailed` failures.
- PR worktrees now create a unique local branch when the requested source name
  already exists, without resetting or updating that branch.
- Recreate restores deleted PR branches from the isolated ref.
- Verification passed: `go test ./internal/worktree` (368 passed).

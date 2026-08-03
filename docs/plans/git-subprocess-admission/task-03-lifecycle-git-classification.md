---
id: "03-lifecycle-git-classification"
title: "Lifecycle Git classification"
status: done
wave: 2
plan: "plan.md"
spec: "../../specs/platform/git-subprocess-admission.md"
depends_on:
  - "01-class-aware-admission"
---

# Task 03: Lifecycle Git Classification

## Acceptance

- Every backend lifecycle Git subprocess passes through `GitLifecycle` admission.
- No owned backend lifecycle package uses the old classless shared helper; the
  GitHub CLI throttle remains unchanged.
- Lifecycle cancellation starts no subprocess and preserves arguments,
  credentials, rollback, error wrapping, and cleanup guarantees.

## Verification

```bash
# Direct Go commands are required because the backend Makefile has no focused
# package target.
cd apps/backend && go test ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/task/service -run 'Test.*Git.*(Admission|Class|Cancel)' -count=1
cd apps/backend && go test ./internal/common/subproc ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/task/service -count=1
cd apps/backend && ! rg -n 'subproc\.Git\(\)' internal/worktree internal/repoclone internal/agent/runtime/lifecycle --glob '*.go' --glob '!*_test.go'
```

Compilation verifies the new required class argument on run helpers; the final
search verifies the owned packages no longer use the classless throttle accessor.

## Files Likely Touched

- `apps/backend/internal/worktree/git_throttle.go`
- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- Focused sibling tests for cancellation and classification

## Dependencies

Task 01 (`01-class-aware-admission`).

## Parallelism

Parallel-safe with Tasks 02 and 05 after Task 01. This task owns backend
lifecycle packages; the other tasks own agentctl API/process and diagnostics.

## Inputs

- Spec section: explicit work classes.
- Plan section: Backend lifecycle operations.
- Task 01 typed helpers and the lifecycle-package call-site search recorded in
  the design investigation.

## Output Contract

Report the lifecycle call-site classification, red tests, files changed, exact
command outcomes, cleanup evidence, residual risks, and synchronized task/plan
status.

## Results

- Migrated worktree, repository-clone, and local runtime preparation Git calls to
  `GitLifecycle`, including explicit acquire-before-execution-timeout paths.
- Moved the worktree capacity seam into its test file so production lifecycle
  packages have no classless `subproc.Git()` usage.
- `cd apps/backend && go test ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/task/service -run 'Test.*Git.*(Admission|Class|Cancel)' -count=1` — 1 passed.
- `cd apps/backend && go test ./internal/common/subproc ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/task/service -count=1` — passed.
- `cd apps/backend && ! rg -n 'subproc\.Git\(\)' internal/worktree internal/repoclone internal/agent/runtime/lifecycle --glob '*.go' --glob '!*_test.go'` — no production matches.
- `cd apps/backend && go test -race ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/repoclone ./internal/task/service -run 'Test.*Git.*(Admission|Class|Cancel)' -count=1` — 1 passed.

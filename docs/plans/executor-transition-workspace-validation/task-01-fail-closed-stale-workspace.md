---
id: "01-fail-closed-stale-workspace"
title: "Fail closed on stale executor-transition workspaces"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/additional-session-workspace-reuse.md"
---

# Task 01: Fail Closed on Stale Executor-Transition Workspaces

## Acceptance

- An executor-type mismatch cannot forward or later recover the previous task
  environment's workspace path, repository inventory, or runtime handles.
- A repo-backed local/worktree launch admits only a workspace tied to the
  selected environment with complete canonical inventory and a matching Git
  checkout.
- Missing, non-Git, and path-mismatched workspaces return a typed recoverable
  error before execution creation and agent startup.
- Valid same-executor additional sessions and repository-less tasks retain
  current behavior.
- Failure leaves filesystem contents, Git state, environment rows, session
  history, and canonical repositories unchanged.
- Current task file/change projections cannot select the rejected workspace.

## Implementation outline

1. Add a launch-scoped validated-environment identity/admission result rather
   than inferring validity from a non-empty path.
2. Make executor mismatch drive fresh environment selection/materialization or
   typed failure before lifecycle launch.
3. Add a lifecycle defense-in-depth guard before `createExecution` for
   repo-backed local/worktree sessions.
4. Keep typed failure projection bounded and path-free.
5. Add regression tests before changing behavior, then run focused package
   tests and the spec linter.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'Test.*Executor.*Mismatch|Test.*Workspace.*Reuse|Test.*Executor.*Transition'
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*Workspace.*Execution|Test.*Workspace.*Ready'
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_execute_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
- workspace-info provider and task-status projection tests selected during the
  regression-first implementation.

## Output contract

Report the exact commit/tree, regression tests, focused test receipts, spec
lint receipt, residual risks, and the runtime rebind verification result. Mark
this task and `plan.md` done only after the implementation and verification
both pass.

## Current receipt

Implementation and focused verification pass on 2026-08-29. The source build
passes. The task stays `in_progress` until an isolated v0.92.1 runtime proves a
fresh local session binds the canonical repository and the rejected legacy path
is never projected or started.

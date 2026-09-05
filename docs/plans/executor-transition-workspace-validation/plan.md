---
spec: docs/specs/tasks/system-design/additional-session-workspace-reuse.md
created: 2026-08-29
status: in_progress
---

# Implementation Plan: Executor Transition Workspace Validation

## Overview

Make executor transitions select a workspace owned by the requested executor
and fail closed before execution creation when a repo-backed workspace is
missing, non-Git, path-mismatched, or not backed by complete canonical
inventory. Preserve existing tasks and all filesystem evidence.

## Confirmed root cause

- `reuseExistingEnvironment` returns early when `env.ExecutorType` differs from
  `req.ExecutorType`, so old runtime handles are not copied.
- That return does not change the durable environment/session selection or
  invalidate the old `workspace_path`.
- `ensureWorkspaceExecutionLocked` later accepts any non-empty
  `WorkspaceInfo.WorkspacePath` and creates an execution without proving a
  repo-backed path is the selected Git checkout.
- The observed v0.92.1 runtime consequently skipped worktree-environment reuse,
  created a local execution at the removed legacy task path, and launched the
  agent with `has_worktree=false`.

## Invariants

- Keep the existing task, repository, session history, and canonical repository.
- Do not delete, move, reset, clean, checkout, or rewrite the stale path.
- Do not use a session path or cached change projection as workspace authority.
- Do not start an agent until the selected environment passes admission.
- Do not add a second workspace ledger or database table.

## Implementation waves

1. [ ] [task-01-fail-closed-stale-workspace](task-01-fail-closed-stale-workspace.md) — implementation and focused verification pass; isolated runtime rebind remains

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor -run 'Test.*Executor.*Mismatch|Test.*Workspace.*Reuse|Test.*Executor.*Transition'
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*Workspace.*Execution|Test.*Workspace.*Ready'
python3 scripts/lint-spec-files.py --all
```

## Risks

- Treating every missing path as an executor transition could break legitimate
  first materialization; validation must be tied to repo-backed selected
  environments and launch phase.
- Host Git inspection is not valid for remote executors; their existing
  executor-owned inventory remains the authority.
- A defense-in-depth lifecycle guard must not convert repository-less tasks or
  quick chats into false failures.

## Implementation receipt

Implemented locally on 2026-08-29. The patch clears stale session workspace
authority on executor transition, rebinds the single durable environment only
after a successful target-executor launch, removes stale backend-specific
runtime/worktree projections, and validates repo-backed host paths before both
launch preparation and cold execution creation.

Passing verification:

- Full `internal/orchestrator/executor` package.
- Focused lifecycle local-preparer, workspace-admission, and workspace-execution
  tests.
- Focused task-service workspace-info tests.
- Backend `make build`, including macOS and Linux agentctl binaries and Kandev.
- Full specification lint and `git diff --check`.

The full lifecycle package retains four failures reproduced unchanged on the
clean v0.92.1 base: three stale-worktree error-classification expectations on
the external-volume layout and one macOS Unix-socket path-length failure. They
are not introduced by this implementation. Runtime rebind against an isolated
data/config root remains required before this plan can be marked complete.

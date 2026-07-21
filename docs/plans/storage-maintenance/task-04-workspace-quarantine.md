---
id: "04-workspace-quarantine"
title: "Workspace quarantine and unarchive"
status: done
wave: 2
depends_on: ["01-durable-task-cleanup", "02-storage-persistence"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 04: Workspace quarantine and unarchive

Classify both task-directory layouts safely, quarantine confirmed orphans, and integrate restore
with unarchive.

## Acceptance

- Complete inventory protects active worktrees, environments, executions, scratch task roots, and
  ancestors; any inventory/path uncertainty performs no move.
- New roots carry ownership markers; confirmed old candidates move atomically to tracked quarantine
  and support restore, permanent deletion, and restart reconciliation.
- Unarchive cancels pending archive cleanup, restores a task-matched quarantine entry before branch
  probing, and never deletes historical recovery metadata or Git branches.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage/workspaces ./internal/worktree ./internal/task/service ./internal/task/handlers
```

## Files likely touched

- `apps/backend/internal/system/storage/workspaces/` provider and tests
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- PR #1687 branch-recovery tests and new quarantine integration tests

## Dependencies

- Task 01 for cleanup cancellation.
- Task 02 for quarantine persistence.

## Inputs

- Spec Task cleanup, Unarchive compatibility, and workspace scenarios
- ADR 0009 fail-closed GC semantics
- Existing `office/infra/gc.go` and tests as migration inputs, not a parallel final service
- PR #1687 merged behavior

## Output contract

Report inventory sources, path safety rules, legacy layout behavior, unarchive outcomes, tests run,
blockers/risks, and update this task plus `plan.md` to done.

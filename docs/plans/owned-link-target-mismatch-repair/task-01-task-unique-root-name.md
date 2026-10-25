---
id: "01-task-unique-root-name"
title: "Task-unique task-root directory name"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 01: Task-unique task-root directory name

Make the `~/.kandev/tasks/<taskDir>` root name derive from the task ID so distinct tasks — even with
identical titles — normally resolve to different task roots, while the ownership marker rejects any
residual collision, and so the resume fallback recomputes the same name the initial launch would have
produced.

## Acceptance

- A new pure helper in `apps/backend/internal/worktree/config.go` produces a short, filesystem-safe,
  lowercase, collision-resistant suffix from a task ID that is stable across repeated calls for the
  same ID and normally differs for different IDs.
- The two task-root call sites build the task directory name from the task ID rather than
  `SmallSuffix(3)`, so two tasks with the same title normally produce different task-root names and a
  residual collision is blocked by the ownership marker.
- `SmallSuffix` and its existing branch-slug callers are unchanged; `SemanticWorktreeName`'s signature
  is unchanged.

## Verification

```bash
cd apps/backend
go test ./internal/worktree/... ./internal/orchestrator/executor/...
golangci-lint run ./internal/worktree/... ./internal/orchestrator/executor/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/worktree/config.go` (add `TaskDirSuffix(taskID string) string`)
- `apps/backend/internal/worktree/config_test.go` (suffix determinism + same-title-different-root tests)
- `apps/backend/internal/orchestrator/executor/executor_execute.go` (~line 1465)
- `apps/backend/internal/orchestrator/executor/executor_resume.go` (`resolveResumeTaskDirName`, ~line 1288)

## Dependencies

None.

## Parallelism

`sequential`.

## Inputs

- Spec: Persistence guarantees paragraph on per-task task-root uniqueness, and the same-title-tasks
  scenario.
- Plan: Backend Area 1.
- Existing `SemanticWorktreeName` (`config.go:452`) and `SmallSuffix` (`config.go:364`).
- Root cause: task ID is never incorporated; random 3-char suffix has no cross-task uniqueness.

## Output contract

Summary, files changed, tests run, blockers, risks, and task/plan status updates in the same
conversation. Reconcile **Files likely touched** with the actual diff before marking done.

## Results

- Added `TaskDirSuffix(taskID string) string` to `apps/backend/internal/worktree/config.go`: a
  deterministic, collision-resistant 8-char lowercase suffix over `branchSuffixAlphabet`, derived
  from `sha256(taskID)`. Empty ID returns empty. `SmallSuffix` and `SemanticWorktreeName`'s signature
  are unchanged.
- Updated both task-root call sites to pass `worktree.TaskDirSuffix(task.ID)` instead of
  `worktree.SmallSuffix(3)`: `executor_execute.go:1465` and `resolveResumeTaskDirName`
  (`executor_resume.go:1288`).
- Added `TestTaskDirSuffix` (stable, lowercase-alphanumeric, representative different IDs, empty-ID)
  and `TestSemanticWorktreeNameTaskUnique` (same title + different task IDs → different roots for the
  covered IDs) to
  `config_test.go`. Both failed red before the helper existed (build error: `undefined: TaskDirSuffix`)
  and pass after.
- Commands:
  - `go test ./internal/worktree/ -run 'TestTaskDirSuffix|TestSemanticWorktreeNameTaskUnique'` → build
    error red before helper; `ok` after.
  - `go test ./internal/worktree/... ./internal/orchestrator/executor/...` → all `ok`.
- External side-effect boundaries: None. Pure helper plus two in-process call-site edits; no
  filesystem, network, or DB writes introduced.

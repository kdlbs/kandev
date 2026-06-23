---
id: "01-backend-child-completion-repository"
title: "Backend child completion repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/subtask-completion-trigger.md"
---

# Task 01: Backend child completion repository

## Acceptance

- The task repository exposes active direct child completion rows for a parent
  task without importing Office repository code.
- Terminal-state evaluation treats `COMPLETED`, `FAILED`, and `CANCELLED` as
  terminal.
- Archived and ephemeral children do not block the parent completion check.

## Verification

```bash
make -C apps/backend test ARGS="./internal/task/repository/..."
```

## Files Likely Touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_repository_test.go`

## Dependencies

None.

## Inputs

- Spec: `docs/specs/tasks/subtask-completion-trigger.md`, sections `What`,
  `Data model`, `Scenarios`.
- Existing patterns: `TaskRepository.ListChildren`,
  `office/repository/sqlite.AreAllChildrenTerminal`, and
  `CountOpenWatcherCreatedTasks` terminal-state SQL.

## Output Contract

When finished, update this task frontmatter to `status: done`, update the
checkbox in `plan.md`, and report:

- Summary of repository API and SQL changes.
- Files changed.
- Tests run and their result.
- Any blockers or follow-up risks.

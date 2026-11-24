---
id: "01-plan-files"
title: "Deliver files with task plan comments"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-COMMENT-FILES-001
acceptance_criteria:
  - AC-TASKS-COMMENT-FILES-001.1
  - AC-TASKS-COMMENT-FILES-001.2
  - AC-TASKS-COMMENT-FILES-001.3
  - AC-TASKS-COMMENT-FILES-001.4
  - AC-TASKS-COMMENT-FILES-001.5
  - AC-TASKS-COMMENT-FILES-001.6
system_design:
  - ../../specs/tasks/system-design/comment-attachment-references.md
---

# Deliver files with task plan comments

## Scope and acceptance

Extend plan comment CRUD, repository transactions, snapshots, direct admission, queued Run, execution formatting and plan composer controls. Exercise removal, restart, conflict and session switching.

Prove that two comments with equal filenames reach the mock agent with different readable bytes. Induce one failed upload and one failed send. Check that retry retains each file with its comment. Cover keyboard and touch submission. Keep clarification routing under its current owner.

## UI preview

```text
Desktop popover                 Phone sheet
[Selected text]                 [Selected text]
[Comment field]                 [Comment field]
[Attach (2)]  [Save]            [Attach (2)]     [Save]

File list open
screen.png   Ready       Remove
notes.txt    Failed      Retry   Remove
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/plan_comment.go`
- `apps/backend/internal/task/service/plan_comment_service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_plan_comment.go`
- `apps/web/components/shared/selection-comment/selection-comment-composer.tsx`

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service ./internal/orchestrator/messagequeue ./internal/agent/runtime/lifecycle)
(cd apps/web && pnpm e2e:run task-plan-comments.spec.ts)
```

## Results

The four plan-file repository regressions passed. They cover ordered snapshots, expiration, atomic replacement, mixed-message admission and replay. Desktop Send kept a saved attachment through reload and delivered it to the selected secondary session. Phone Run delivered its attachment to the primary session. The full SQLite, plancommenttx, plancomments, task service and message queue package suites passed with a tmpfs temporary directory.

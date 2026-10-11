---
id: "04-verification"
title: "Portable adoption verification"
status: in_progress
wave: 4
depends_on: [3]
plan: "plan.md"
spec: "../../specs/tasks/requirements/portable-workspace-adoption.md"
---

# Task 04: verification

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: docs/public/git-operations.md; task-owned regression tests.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: Task 03.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted checks passed; see plan and task-local logs. Independent final candidate verification remains pending.

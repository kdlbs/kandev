---
id: "02-publication"
title: "Portable adoption publication"
status: done
wave: 2
depends_on: [1]
plan: "plan.md"
spec: "../../specs/tasks/requirements/portable-workspace-adoption.md"
---

# Task 02: publication

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: apps/backend/internal/task/models/workspace_adoption.go; apps/backend/internal/task/checkoutadoption; apps/backend/internal/task/repository/sqlite/task_environment_adoption*.go.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: Task 01.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted behavioral tests and changed-code lint passed; see plan verification results and task-local logs.

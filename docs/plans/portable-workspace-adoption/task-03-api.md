---
id: "03-api"
title: "Portable adoption api"
status: done
wave: 3
depends_on: [2]
plan: "plan.md"
spec: "../../specs/tasks/requirements/portable-workspace-adoption.md"
---

# Task 03: api

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: apps/backend/internal/task/service/workspace_adoption.go; apps/backend/internal/task/handlers/workspace_adoption*.go.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: Task 02.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted behavioral tests and changed-code lint passed; see plan verification results and task-local logs.

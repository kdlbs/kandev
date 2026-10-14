---
id: "03-api"
title: "Portable adoption api"
status: done
wave: 3
depends_on: [2]
plan: "plan.md"
requirements:
  - REQ-TASKS-PORTABLE-ADOPTION-001
  - REQ-TASKS-PORTABLE-ADOPTION-002
  - REQ-TASKS-PORTABLE-ADOPTION-003
system_design:
  - ../../specs/tasks/system-design/portable-workspace-adoption.md
acceptance_criteria:
  - AC-TASKS-PORTABLE-ADOPTION-001.1
  - AC-TASKS-PORTABLE-ADOPTION-001.2
  - AC-TASKS-PORTABLE-ADOPTION-001.3
  - AC-TASKS-PORTABLE-ADOPTION-002.1
  - AC-TASKS-PORTABLE-ADOPTION-002.2
  - AC-TASKS-PORTABLE-ADOPTION-002.3
  - AC-TASKS-PORTABLE-ADOPTION-002.4
  - AC-TASKS-PORTABLE-ADOPTION-003.1
  - AC-TASKS-PORTABLE-ADOPTION-003.2
  - AC-TASKS-PORTABLE-ADOPTION-003.3
  - AC-TASKS-PORTABLE-ADOPTION-003.4
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

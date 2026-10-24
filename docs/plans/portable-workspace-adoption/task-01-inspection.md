---
id: "01-inspection"
title: "Portable adoption inspection"
status: done
wave: 1
depends_on: []
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

# Task 01: inspection

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: apps/backend/internal/worktree/workspace_adoption*.go.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: None.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted behavioral tests and changed-code lint passed; see plan verification results and task-local logs.

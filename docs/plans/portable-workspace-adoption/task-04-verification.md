---
id: "04-verification"
title: "Portable adoption verification"
status: done
wave: 4
depends_on: [3]
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

# Task 04: verification

- Acceptance: satisfy corresponding explicit authority, preserved identity and durable
  publication requirements in the linked specification. Keep operator deployment policy local.
- Files likely touched: docs/public/git-operations.md; task-owned regression tests.
- Verification: `cd apps/backend && go test -tags fts5 ./internal/worktree ./internal/task/handlers ./internal/task/repository/sqlite -run TestWorkspaceAdoption`.
- Dependencies: Task 03.
- Parallelism: sequential; MAIN sole writer.
- Output: changed files, exact test evidence and remaining limits.

## Results

MAIN targeted race checks and normal commit hooks passed. Independent Review and
QA passed source candidate 7cb27830c637ba2969c99271b9e982f3a29093ea; see the plan
for exact commands, session IDs, report paths and limitations. MAIN resolved both
Review findings with permanent regression coverage. Final documentation-only
candidate verification is recorded in Kandev plan metadata to avoid a self-SHA
documentation loop. No PR, deployment or local-overlay retirement is claimed.

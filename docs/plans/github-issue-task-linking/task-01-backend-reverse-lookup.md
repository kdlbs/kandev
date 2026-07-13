---
id: "01-backend-reverse-lookup"
title: "Backend issue-link reverse lookup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 01: Backend Issue-Link Reverse Lookup

## Acceptance

- A workspace-scoped endpoint returns valid metadata-backed GitHub issue links grouped by task ID.
- Manual-link and issue-watch metadata resolve to the same owner/repo/number key.
- Invalid metadata and tasks from other workspaces do not produce links.

## Verification

`cd apps/backend && rtk go test ./internal/github/...`

## Files Likely Touched

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_test.go`
- `apps/backend/internal/github/service_task_issue.go`
- `apps/backend/internal/github/service_task_issue_test.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_task_issue_test.go`

## Output Contract

Report the API shape, normalization behavior, files changed, tests run, blockers, and residual risks; mark this task done in this file and `plan.md`.

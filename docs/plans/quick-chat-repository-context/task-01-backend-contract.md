---
id: "01-backend-contract"
title: "Backend quick-chat repository contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-repository-context.md"
---

# Task 01: Backend quick-chat repository contract

**Acceptance:** plural repository inputs preserve order and branches; legacy singleton input
remains accepted; repo-backed starts always launch with the worktree executor.

**Verification:** `make -C apps/backend test` from the repository root.

**Files likely touched:** `apps/backend/internal/task/handlers/task_http_handlers.go`,
`apps/backend/internal/task/handlers/task_http_handlers_test.go`.

**Dependencies:** None.

**Output contract:** summarize contract changes, tests run, files changed, risks, and status.

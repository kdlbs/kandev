---
id: "01-backend-author-type-filter"
title: "Add author_type filter to the paginated messages query"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/last-prompt-pinning-regressions/spec.md"
---

# Task 01: Add `author_type` filter to the paginated messages query

- **Acceptance:** `GET /api/v1/task-sessions/:id/messages?author_type=user` returns only user-authored messages (newest-first with `sort=desc`, still cursor-paginated); `author_type=agent` returns only agent-authored ones; omitting the param leaves current behavior unchanged. Invalid values are rejected with 400. The WS list path is unaffected (it never sets the filter).
- **Verification:** First write the repository + handler tests and confirm the filter case fails (no filtering yet), then implement and re-run: `cd apps/backend && PATH=/tmp/go/bin:$PATH go test ./internal/task/repository/... ./internal/task/handlers/...` and the lint gate `PATH=/tmp/go/bin:$PATH make lint`.
- **Files likely touched:** `apps/backend/internal/task/models/models.go`, `internal/task/repository/sqlite/message.go`, `internal/task/repository/message_repository_test.go`, `internal/task/service/service_requests.go`, `internal/task/service/service_messages.go`, `internal/task/handlers/message_handlers.go`, existing handler/repository test files.
- **Dependencies:** None.
- **Parallelism:** sequential — shared query contract with the frontend.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.

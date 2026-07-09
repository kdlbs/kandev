---
id: "01-backend-candidate-query"
title: "Backend candidate query"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 01: Backend Candidate Query

## Acceptance

- `repository.TaskRepository` exposes `ListExpiredQuickChatTasks(ctx, cutoff)` and SQLite implements it.
- The query matches only true quick chats: ephemeral, no workflow, not archived, not config-mode, not automation-run origin, no active sessions.
- Last activity uses the greater of `tasks.updated_at` and newest `task_sessions.updated_at`.

## Verification

- `cd apps/backend && go test ./internal/task/repository -run 'TestSQLiteRepository_ListExpiredQuickChatTasks'`

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_repository_test.go`
- Optional `apps/backend/internal/db/dialect/time.go`
- Optional `apps/backend/internal/db/dialect/dialect_test.go`

## Dependencies

None.

## Inputs

- Spec sections: Data model, API surface, Failure modes, Scenarios.
- Existing patterns: `ListTasksForAutoArchive` in `apps/backend/internal/task/repository/sqlite/task.go`; `excludeConfigModePredicate` in the same file; active session state filters in `apps/backend/internal/task/repository/sqlite/session.go`; SQL portability helpers in `apps/backend/internal/db/dialect/`.

## Output contract

Update this task status to `done`, update the Wave 1 checkbox in `plan.md`, and report the files changed, tests run, and any discovered timestamp behavior.

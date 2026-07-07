---
id: "02-backend-expiration-service"
title: "Backend expiration service"
status: done
wave: 2
depends_on: ["01-backend-candidate-query"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 02: Backend Expiration Service

## Acceptance

- The task service runs a daily quick-chat expiration loop with a hardcoded 7-day idle retention window.
- Each expired candidate is deleted through `Service.DeleteTask`, preserving existing workspace directory cleanup and `task.deleted` publishing.
- Candidate-list errors delete nothing; individual delete failures are logged and do not stop later candidates.

## Verification

- `cd apps/backend && go test ./internal/task/service -run 'TestService_QuickChatExpiration|TestService_DeleteTask'`
- `cd apps/backend && go test ./internal/backendapp -run 'Test.*QuickChat'` if startup wiring gets focused coverage there.

## Files likely touched

- New `apps/backend/internal/task/service/quick_chat_expiration.go`
- `apps/backend/internal/task/service/service_test.go`
- Optional focused test file `apps/backend/internal/task/service/quick_chat_expiration_test.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

- Task 01 must land first so the service can call `ListExpiredQuickChatTasks`.

## Inputs

- Spec sections: What, State machine, Failure modes, Persistence guarantees.
- Existing patterns: `StartAutoArchiveLoop` / `runAutoArchive` in `apps/backend/internal/task/service/auto_archive.go`; `DeleteTask` cleanup path in `apps/backend/internal/task/service/service_tasks.go`.

## Output contract

Update this task status to `done`, update the Wave 2 checkbox in `plan.md`, and report the files changed, tests run, and any cleanup/event behavior verified.

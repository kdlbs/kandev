---
id: "03-conflict-adapters-and-watcher-deferral"
title: "Conflict adapters and review-watch deferral"
status: done
wave: 3
depends_on: ["02-task-service-enforcement"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 03: Conflict Adapters and Review-Watch Deferral

## Acceptance

- HTTP task creation returns `409 Conflict` for the typed WIP error.
- WebSocket and MCP task creation return `ErrorCodeConflict` and preserve the
  actionable step/limit message.
- GitHub review task creation treats WIP rejection as deferred capacity:
  releases the PR reservation, assigns no task ID, starts no task, and permits a
  later retry to succeed.
- An admitted GitHub review task created directly in an auto-start `Review`
  step remains in that step during startup and active work; `agent.boot_ready`
  does not advance it, and the first genuine turn completion moves it to the
  configured next step exactly once.
- Non-capacity task-creation failures retain their current error behavior.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/handlers -run 'Test.*CreateTask.*WIP' -count=1
go test -tags fts5 ./internal/mcp/handlers -run 'TestHandleCreateTask.*WIP' -count=1
go test -tags fts5 ./internal/orchestrator -run 'Test(CreateReviewTask_.*|GitHubWatcherTaskCreateError|ReviewWatchLifecycle_.*)' -count=1
```

## Files likely touched

- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`

## Dependencies

Task 02.

## Evidence

HTTP, WebSocket, and MCP focused tests pass. GitHub review capacity rejection
coverage confirms reservation release without task assignment or auto-start;
the lifecycle regression confirms boot-ready stays in Review and the first
genuine turn completion advances once.

## Parallelism

`sequential`

## Inputs

- Task 02's service error behavior.
- Existing `isMoveConflict`, `isTaskCreateValidationError`, and WebSocket error
  mappings.
- Existing review reservation release test in
  `event_handlers_github_review_test.go`.
- Existing boot-ready/turn-complete separation in
  `event_handlers_agent.go` and its regression tests.

## Output contract

Mark this task `in_progress` before the RED tests and `done` after GREEN and
refactor. Update `plan.md` and report each adapter's observed error contract,
review reservation/retry evidence, watcher lifecycle-ordering evidence, files
changed, exact test results, blockers, and risks.

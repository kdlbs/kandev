---
id: "04-cancellation-projection-contract"
title: "Cancellation projection contract"
status: pending
wave: 2
depends_on: ["03-backend-cancellation-lifecycle"]
plan: "plan.md"
spec: "../../specs/ui/cancel-turn-progress.md"
---

# Task 04: Cancellation projection contract

## Acceptance

- Full and summary task-session DTOs always serialize `cancellation_pending`, including explicit
  false, from the narrow orchestrator provider without a repository/schema write.
- Task-detail boot state, task-session REST list/get responses, and the initial session-subscription
  snapshot expose the provider's current value, so a fresh client is correct before another live
  transition occurs.
- `task_session.cancellation_changed` maps to `session.cancellation_changed` and is delivered only
  to clients subscribed to that session; publishing failure cannot fail cancellation.

## Verification

```bash
(cd apps/backend && go test ./internal/task/dto ./internal/task/handlers ./internal/backendapp ./internal/gateway/websocket -run 'Cancellation|CancelPending|SessionDataProvider')
make -C apps/backend test
```

Follow TDD: add explicit-false DTO and fresh-load assertions first, then wire provider enrichment and
the session-scoped semantic notification until each boundary passes.

## Files likely touched

- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/cancellation_pending_test.go`
- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/gateway/websocket/task_notifications_test.go`

## Dependencies

Task 03.

## Parallelism

Sequential. It exposes Task 03's state and defines the contract Task 05 consumes.

## Inputs

- Spec: `API surface`, `Permissions`, `Failure modes`, and reload/second-client scenarios.
- Plan: `Session wire contract and routing` and `Boot, REST, and subscription reconciliation`.
- Existing patterns: `ForegroundActivityProvider`, `EnrichForegroundActivity`, task-handler provider
  discovery, `addTaskDetailSessionsState`, `buildSessionDataProvider`, and
  `TaskSessionActivityChanged` routing.

## Output contract

Report every contract shape and enrichment boundary, explicit-false and routing test evidence,
exact files/commands, blockers/risks, and synchronize this task plus `plan.md` in the same primary
conversation.

## Results

Pending. Record every exact command and outcome, `git diff --check`, and confirm there is no schema
migration, durable marker, cross-workspace broadcast, or external side effect.

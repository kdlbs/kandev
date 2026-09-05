---
id: "03-persist-attachment-reports"
title: "Persist session attachment reports"
status: pending
wave: 3
depends_on: ["02-observe-mcp-attachment"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 03: Persist session attachment reports

## Acceptance

- Lifecycle stamps agentctl evidence with the live execution and drops stale
  execution/attempt events without affecting prompt activity.
- Orchestrator reduces events into the bounded history under one session
  metadata key and publishes `session.mcp_status_updated`.
- The gateway broadcasts status only to authorized session/workspace clients.
- Task-detail boot state hydrates the same current/history report after reload.
- Frontend session-runtime state accepts boot and WebSocket reports, keeps
  concurrent sessions separate, and clears status when a session is removed.
- Persistence failure is logged without changing task/session/agent state.

## Verification

- `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/gateway/websocket ./internal/backendapp ./internal/task/...`
- `cd apps && pnpm --filter @kandev/web exec vitest run lib/ws/handlers/session-mcp-status.test.ts lib/state/slices/session-runtime/session-runtime-slice.test.ts`

Write RED tests for stale-attempt rejection, bounded persistence, WS routing,
boot hydration, concurrent-session isolation, live replacement, and session
cleanup before adding production handlers.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/gateway/websocket/session_notifications.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/merge-strategies.ts`
- `apps/web/lib/ws/handlers/session-mcp-status.ts`
- `apps/web/lib/ws/handlers/session-mcp-status.test.ts`

## Dependencies

- Task 02 produces ordered evidence events.

## Parallelism

Sequential. Task 04 requires the current persisted/effective report, and Task
05 requires the boot/WS store contract.

## Inputs

- Existing `session.models_updated` event, metadata, boot-state, and frontend
  slice pattern
- Existing session-scoped WebSocket broadcaster
- Task 01 history reducer and bounds

## Output contract

Report stale-event handling, metadata and WS keys, boot-state shape, frontend
store behavior, RED/GREEN evidence, exact tests, files changed, blockers, and
risks. Mark this task `done` and update its plan checkbox.

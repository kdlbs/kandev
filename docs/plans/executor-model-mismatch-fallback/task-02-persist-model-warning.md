---
id: "02-persist-model-warning"
title: "Persist model-selection warnings"
status: done
wave: 2
depends_on: ["01-executor-model-decision"]
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 02: Persist model-selection warnings

## Acceptance

- Every default or explicit fallback emits structured requested, effective, agent, executor, and reason data.
- The orchestrator persists one warning status message for each session-start decision, and reload hydration returns it.
- Event replay or message failure does not create duplicates or stop the task launch.

## TDD scenarios

1. RED: Add lifecycle event tests for explicit fallback, provider default, unknown effective model, and unsupported selection.
2. RED: Add orchestrator tests for status metadata, idempotency, replay, and persistence errors.
3. RED: Add gateway tests for the provider-neutral live event and old-event compatibility.
4. GREEN: Generalize the lifecycle event and persist the semantic warning message.
5. REFACTOR: Keep the event payload, stored metadata, and WebSocket payload on one shared shape.

## Verification

- `cd apps/backend && go test -tags fts5 -run 'Test.*ModelSelectionWarning|Test.*ModelFallback' ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/gateway/websocket`
- `cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/gateway/websocket`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/gateway/websocket/session_notifications.go`
- `apps/backend/internal/gateway/websocket/session_notifications_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

- Task 01 supplies the typed model decision.

## Parallelism

Sequential after Task 01.
This task defines the persisted and live warning contracts.

## Inputs

- The typed decision from Task 01.
- The current `session_model_fallback` event.
- The existing `CreateSessionMessage` warning path.
- The existing task message repository and hydration flow.

## Output contract

Report the event shape, message metadata, idempotency method, RED evidence, GREEN evidence, and test results.

## Risks

- The lifecycle event can arrive before the session identity is linked.
- A blank turn ID must use the existing session-status message convention.
- Compatibility consumers can still depend on `session.model_fallback`.

## Results

Implemented structured warning events, idempotent best-effort persistence,
provider-neutral WebSocket notification, and durable metadata hydration. The
orchestrator and gateway suites passed with 2,270 tests; the combined backend
targeted suite passed with 7,299 tests.

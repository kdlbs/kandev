---
id: "06-derive-mcp-handler-inventory"
title: "Derive MCP handler inventory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 06: Derive MCP handler inventory

## Acceptance

- The WebSocket dispatcher exposes a concurrency-safe handler count.
- MCP handler registration logs the measured before/after delta and contains no
  manual count constant or provider-specific increments.
- Tests cover dispatcher counting and the full registration delta without
  duplicating a brittle expected arithmetic expression in production code.

## Verification

- `cd apps/backend && go test -race ./pkg/websocket ./internal/mcp/handlers`

Add the dispatcher and registration-count tests first; capture the stale current
count as regression evidence before removing manual bookkeeping.

## Files likely touched

- `apps/backend/pkg/websocket/handler.go`
- `apps/backend/pkg/websocket/handler_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`

## Dependencies

None.

## Parallelism

Parallel-safe with Tasks 01 and 04. It owns WebSocket dispatcher and MCP handler
registration files, not MCP server tool registration.

## Inputs

- Existing dispatcher lock and handler map
- Existing `RegisterHandlers` manual `count` bookkeeping
- Spec `Handler inventory observability`

## Risks

- Do not expose or iterate the handler map; return only its size under `RLock`.
- Counting is diagnostic and must not influence registration success.

## Output contract

Report the stale-count evidence, new measured-delta behavior, files changed,
exact verification result, and risks. Mark this task `done` and update its plan
checkbox in the same conversation.

## Result

- RED evidence: the registration-delta test exposed the stale manual count —
  the nil-dependency registration path logged `28` while the dispatcher
  actually gained `31` unique handlers.
- Added a lock-protected `Dispatcher.HandlerCount` diagnostic and changed MCP
  registration logging to measure the dispatcher before and after registration.
  All manual count constants and increments were removed; registration behavior
  is otherwise unchanged.
- Added dispatcher replacement/count coverage and full MCP registration-delta
  coverage.
- Verification: `cd apps/backend && go test -race ./pkg/websocket ./internal/mcp/handlers`
  passed (322 tests).
- Risk: the diagnostic is a unique-action delta, so pre-existing action
  replacements are intentionally not counted as new dispatcher entries.

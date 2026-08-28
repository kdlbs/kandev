---
id: "01-invalidate-reset-context-usage"
title: "Invalidate context-window state on successful reset"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/context-window-reset-freshness.md"
---

# Task 01: Invalidate context-window state on successful reset

## Intent

Remove the previous provider conversation's context-window reading from persistence and every active frontend cache once a reset succeeds, while retaining it when reset fails.

## Acceptance

- A successful shared reset clears `task_sessions.metadata.context_window` and the initiating client's `contextWindow.bySessionId[sessionId]` entry.
- `session.state_changed` clears cached usage only when `metadata` or `session_metadata` explicitly contains an empty `context_window`; unrelated metadata does not clear it.
- Failed resets retain the prior persisted and frontend reading.
- If the provider reset succeeds but metadata clearing fails, Kandev logs the persistence failure, keeps the reset response successful, and the initiating client still receives an explicit in-memory clear.

## TDD sequence

1. Extend `TestProcessOnEnterResetAgentContext` with successful-clear, failed-retention, and successful-provider-reset plus failed-metadata-clear assertions; run it and confirm the successful-clear assertion fails.
2. Add frontend handler cases for `context_window: null` and unrelated metadata; run them and confirm the null case fails.
3. Add the reset-button success/failure cache tests; run them and confirm the success case fails.
4. Implement the smallest backend and frontend changes, rerun the exact checks, and refactor without widening behavior.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_triggers_test.go`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/components/task/chat/reset-context-button.tsx`
- `apps/web/components/task/chat/reset-context-button.test.tsx`

## Dependencies

None.

## Parallelism

`sequential` — backend persistence, frontend event invalidation, and initiating-client invalidation form one behavioral boundary and should converge before E2E work.

## Verification

- `cd apps/backend && go test -run TestProcessOnEnterResetAgentContext ./internal/orchestrator`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/agent-session.test.ts components/task/chat/reset-context-button.test.tsx`

## Inputs

- Spec `What`, `Persistence guarantees`, `Failure modes`, and first four scenarios.
- Existing model-change invalidation in `apps/web/lib/ws/handlers/session-models.ts`.
- Existing context persistence in `apps/backend/internal/orchestrator/event_handlers_git.go`.

## Output contract

Result: RED reproduced the persisted-cache bug and the frontend null-handling gap. GREEN passed `go test -v -run TestProcessOnEnterResetAgentContext ./internal/orchestrator` (9 tests) and `pnpm --filter @kandev/web test -- --run lib/ws/handlers/agent-session.test.ts components/task/chat/reset-context-button.test.tsx` (47 tests). The reset boundary now clears persisted usage after successful reset, explicit null session metadata clears subscribed clients, a metadata-clear failure keeps the provider reset successful while clearing the in-memory snapshot, and the initiating client clears only after a successful response.

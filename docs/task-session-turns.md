# Task Session Turns: Table + Flow

This document describes how to add a turn table that groups all messages from a single agent turn, how messages link to a turn, and how turn updates propagate through the backend → event bus → WebSocket → frontend store.

## Goals
- Add a `task_session_turns` table with at least `id`, `started_at`, `completed_at`.
- Ensure every `task_session_message` points to the turn it belongs to.
- Turn creation/completion follows existing app flow and broadcasts to subscribed clients.

## Definitions
- **Turn**: A single prompt/response cycle for a task session, starting when a prompt is sent and ending when the agent completes/cancels/errors.
- **Message**: A persisted item in `task_session_messages` (user prompt, agent text, tool call, status, error).

## Schema Changes (SQLite)
Add a new table and link messages to it.

### New Table: `task_session_turns`
Suggested columns (minimum + useful metadata):
- `id` TEXT PRIMARY KEY (UUID).
- `task_session_id` TEXT NOT NULL (FK → `task_sessions.id`).
- `task_id` TEXT NOT NULL (denormalized for query fanout).
- `started_at` DATETIME NOT NULL.
- `completed_at` DATETIME (nullable).
- `created_at` DATETIME NOT NULL.
- `updated_at` DATETIME NOT NULL.
- `metadata` TEXT DEFAULT '{}' (optional, for provider-specific data like Codex `turn_id`).

Indexes:
- `idx_turns_session_id` on `(task_session_id)`.
- `idx_turns_session_started` on `(task_session_id, started_at)`.
- `idx_turns_task_id` on `(task_id)`.
- Optional: partial index for active turn lookup `(task_session_id) WHERE completed_at IS NULL`.

### Link Messages to Turns
Add a `turn_id` column to `task_session_messages`:
- `turn_id` TEXT (FK → `task_session_turns.id`).
- Index on `(turn_id)`.
- Keep nullable for legacy messages; new writes should set it.

Files to update:
- `apps/backend/internal/task/repository/sqlite.go` (schema + migration via `ensureColumn`).
- `apps/backend/internal/task/models/models.go` (add `Turn` model, add `TurnID` to `Message`).
- `apps/backend/pkg/api/v1/task.go` and `apps/web/lib/types/http.ts` (API types include `turn_id`).

## Backend Flow (Turn Lifecycle)
Turns should align with the prompt lifecycle already handled in the orchestrator and message services.

### Create Turn
Create a turn when a prompt begins:
- **Entry point**: `apps/backend/internal/orchestrator/service.go` in `PromptTask` before calling `executor.Prompt`.
- Create `task_session_turns` row with `started_at = now`.
- Persist the new `turn_id` in memory for the rest of the prompt lifecycle.
  - Suggested: update `task_sessions.metadata.active_turn_id` and/or hold in the `Executor`/`AgentExecution` context.

### Attach Messages to Turn
Ensure all messages created while a prompt is in-flight use the active `turn_id`.

Where messages are created today:
- User prompt: `apps/backend/internal/task/handlers/message_handlers.go` → `MessageController.CreateMessage`.
- Agent output + tool events: `apps/backend/internal/orchestrator/service.go` (`saveAgentTextIfPresent`, `handleToolCallEvent`, `handleToolUpdateEvent`, error/status creation).

Expected behavior:
- The user prompt message should reference the newly created `turn_id`.
- Subsequent agent/tool/status messages should use the same `turn_id`.
- If the session is resumed and no active turn exists, create a new turn before message creation.

Implementation sketch:
- Add `TurnID` to `CreateMessageRequest` (service/controller layers).
- Teach `messageCreator.CreateAgentMessage/CreateToolCallMessage/CreateSessionMessage` to accept a `turn_id` or resolve the current active turn by `session_id`.
- When the orchestrator receives agent stream events, it should pass `turn_id` to message creation calls.

### Complete Turn
Close the turn when the agent finishes or the turn is cancelled:
- **Completion**: `apps/backend/internal/orchestrator/service.go` in `handleAgentStreamEvent` when `eventType == "complete"`.
- **Cancellation**: `apps/backend/internal/orchestrator/service.go` in `CancelAgent`.
- **Errors**: Treat `eventType == "error"` as terminal if it ends the prompt; set `completed_at`.

Action:
- Update `task_session_turns.completed_at = now`, `updated_at = now`.
- Clear `task_sessions.metadata.active_turn_id` (if used).

## Event Bus and WebSocket Broadcast
Turn updates should follow the same path as existing message/session events.

### Event Types
Add new event types in `apps/backend/internal/events/types.go`:
- `TurnStarted = "turn.started"` (or `task_session.turn.started`).
- `TurnCompleted = "turn.completed"`.
- Optional `TurnUpdated = "turn.updated"` for partial updates.

### Publish Events
Publish turn events when you create/complete a turn:
- Location: task service or orchestrator service (same layer that writes to DB).
- Payload fields (minimum):
  - `turn_id`
  - `session_id`
  - `task_id`
  - `started_at`
  - `completed_at`

### Broadcast to WebSocket Subscribers
Wire up in `apps/backend/internal/gateway/websocket/task_notifications.go`:
- Subscribe to the new event types.
- Broadcast to the session channel (same as `session.message.added`).

Add action constants in `apps/backend/pkg/websocket/actions.go`:
- `ActionSessionTurnStarted = "session.turn.started"`
- `ActionSessionTurnCompleted = "session.turn.completed"`
- Optional `ActionSessionTurnUpdated = "session.turn.updated"`

## Frontend Flow
Frontend store and handlers should mirror the current message flow.

### WebSocket Handlers
Add a new WS handler file (or extend existing):
- `apps/web/lib/ws/handlers/turns.ts` to handle:
  - `session.turn.started`
  - `session.turn.completed`
- Store updates should be keyed by `session_id`.

### Store Shape
Add a `turns` slice in `apps/web/lib/state/store.ts`:
- `turns.bySession[sessionId] = Turn[]`
- `turns.activeBySession[sessionId] = Turn | null` (optional for UI)

### Message Rendering
Messages already render from `messages.bySession[sessionId]`. With `turn_id`:
- Allow grouping in UI by turn.
- Allow filtering by active turn.
- Show turn duration in the UI (e.g., "running for 42s") by comparing `started_at` to now for active turns.

### SSR Data
If needed for initial render:
- Add an HTTP endpoint to list turns for a session.
- SSR fetch in `apps/web/app/task/[id]/page-client.tsx` or equivalent (see `docs/session-page-data-flow.md`).

## Migration / Backfill
Existing messages will not have `turn_id`:
- Keep `turn_id` nullable.
- Optional backfill: create one turn per session using min/max message timestamps and assign messages with NULL `turn_id` to that turn.

## Edge Cases
- **Multiple prompts queued**: Ensure only one active turn per session. If prompts can overlap, add a `state` column or queue ID.
- **Agent error before messages**: Start turn on prompt, complete on error even if no agent messages.
- **Permission requests**: If the agent pauses for permission, the turn remains open; completion happens after prompt completion or cancellation.
- **Concurrency**: Multiple sessions can run in parallel without conflict as long as turns and broadcasts stay scoped by `session_id` (and `active_turn_id` is per session).

## Touchpoints Summary
Backend:
- `apps/backend/internal/task/models/models.go` (Turn model, Message.TurnID)
- `apps/backend/internal/task/repository/sqlite.go` (schema + queries)
- `apps/backend/internal/task/repository/interface.go` (turn methods)
- `apps/backend/internal/task/service/service.go` (turn create/complete, message association)
- `apps/backend/internal/orchestrator/service.go` (prompt start/complete)
- `apps/backend/internal/events/types.go` (turn events)
- `apps/backend/internal/gateway/websocket/task_notifications.go` (broadcast)
- `apps/backend/pkg/websocket/actions.go` (actions)

Frontend:
- `apps/web/lib/ws/handlers/turns.ts` (WS handlers)
- `apps/web/lib/state/store.ts` (turns slice)
- `apps/web/lib/types/http.ts` (Turn + Message.turn_id)
- `apps/web/components/task/chat/*` (optional turn grouping UI)

Docs:
- `docs/WEBSOCKET_API.md` (document new turn actions and payloads).

---
status: draft
system: office
requirements:
  - REQ-OFFICE-LIVE-UPDATES-001
created: 2026-05-02
owners:
  - cfl
---
# Office Live Updates System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-LIVE-UPDATES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-LIVE-UPDATES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Failure modes

| Dependency / scenario | Observable behavior |
|---|---|
| **WS network drop** | Socket transitions `open → closed → reconnecting`. Pending request promises stay armed until `cleanupPendingRequests()` rejects them at the reconnect cap. Subscription maps are retained. On `open` the client flushes the buffered outbound queue, then re-issues every `subscribe` / `focus` / `user.subscribe` / `run.subscribe` frame from `resubscribe()`. **No replay** of missed notifications — surfaces refetch on next event or stay stale until then. |
| **Reconnect cap exceeded** | After `maxAttempts` (default 10) consecutive failures with exponential backoff capped at 30s, status moves to `error`, pending requests are rejected with `WebSocket connection closed`, and no further automatic reconnects occur. The [WebSocket connectivity warning](../../ui/requirements/ws-connectivity-warning.md) remains red; the user must reload to recover. |
| **Server send buffer full** | `client.send` is a 256-deep buffer. When full, `sendBytes` logs `Client send buffer full, dropping message` and the message is dropped for that client only. Other clients still receive it. No retry, no replay; consumer must reconcile on next event. |
| **Frontend handler throws** | The hub's frontend WS client invokes handlers in a `forEach`; an unhandled throw skips remaining handlers for that event but does not tear down the socket. |
| **Event bus publish during shutdown** | `MemoryEventBus.Publish` returns `event bus is closed` after `Close()`. The publisher (orchestrator / office service) logs and continues; the broadcast is dropped. WS clients see no notification for that event. |
| **Event bus subscription error in handler** | `OfficeEventBroadcaster.subscribe` logs `failed to build office ws notification` and returns nil to the bus — handler errors never propagate back to the publisher. |
| **Cross-workspace event leaks past server** | Frontend `isCurrentWorkspace(payload)` discards it. No store mutation occurs. Refetch is not triggered. |
| **Optimistic comment — server returns 5xx / network error** | Pending row removed from thread, draft text and any attached file are restored to the input, send button re-enables, and a toast (`Failed to send comment - please try again.`) surfaces. No automatic retry. |
| **Optimistic comment — server confirms but WS event never arrives** | Pending row stays in `awaiting_agent` indefinitely. A page reload reconciles against the REST list. No client-side timeout flips it to `failed` once the POST succeeded. |
| **`office.run.queued` arrives before the user comment refetch lands** | The badge waits — `triggerRefetch("comments:<taskId>")` invalidates the comment fetch and the badge renders once the next list response includes `runId` / `runStatus`. |
| **Agent reply lands before run finishes** | The per-comment run-status badge hides reactively when any agent reply for the task arrives (`office.comment.created` with `author_type != "user"`), even if the run is still `claimed`. |
| **Backend restart mid-session** | Every client transitions to `reconnecting` after `pongWait` (60s). Subscriptions are restored on the next open. In-flight notifications between the bus and the socket are lost. |
| **Slow consumer (frontend tab in background)** | Browser may throttle the WS but the connection persists. Notifications queue in the OS-level buffer until the tab resumes; on resume the handlers replay in arrival order. No client-side dedup. |
| **Duplicate notifications** | The frontend tolerates re-delivery — handlers are idempotent (status patches converge; refetch triggers debounce per page). Same UUID seen twice in `office.comment.created` does **not** spawn two rows because the optimistic UUID match deduplicates. |
| **WS disabled / blocked at the network edge** | `setStatus("error")` after first failure; reconnect loop runs to cap. Out of scope: a polling fallback. Surfaces stay frozen on last-known-good data until the user reloads. |

## Persistence guarantees

### Survives a kandev process restart

- All entity rows that drive UI state: `tasks`, `task_sessions`, `office_comments`, `office_runs`, `office_run_events`, `office_activity`, `office_approvals`, `office_agents`, `provider_health_state`, `office_route_attempts`. On reconnect the frontend refetches each surface from REST and resumes streaming from there.
- Event-bus subject registry is **rebuilt on boot** by `RegisterEventSubscribers` (office) and `RegisterOfficeNotifications` (WS) — not persisted, but deterministically reconstructed.

### Does NOT survive a kandev process restart

- `Hub.clients`, `taskSubscribers`, `sessionSubscribers`, `userSubscribers`, `runSubscribers`, `sessionMode` — all in-memory, cleared on shutdown via `closeAllClients()`.
- `bus.MemoryEventBus.subscriptions` — process-local channels.
- `WebSocketClient.pendingRequests` and `pendingQueue` on the frontend — rejected on `disconnect()` cleanup.
- Any notification mid-flight on the bus when the bus is closed.
- The "Sending..." / "Awaiting agent" sticker on a pending optimistic comment — the row is wiped on reload; the REST refetch produces the canonical thread.

### Does NOT survive a WS reconnect

- Missed notifications during the gap. There is no replay window, no event sequence number, no last-event-id header. Surfaces reconcile by re-reading the server state via REST (driven by the `setOfficeRefetchTrigger` plumbing) plus any new notifications that arrive after `open`.
- The "initial session-data snapshot" pushed on `session.subscribe` / `session.focus` is re-sent each time those frames are re-issued from `resubscribe()`.

### Survives a WS reconnect

- Frontend subscription intent (`subscriptions`, `sessionSubscriptions`, `sessionFocusCounts`, `userSubscriptionCount`, `runSubscriptions` maps). `resubscribe()` replays every entry as a fresh subscribe frame on `open`.
- All Zustand store slices not specifically invalidated by a refetch trigger. The store is not cleared on reconnect.

### TTL / retention

- No event log retention. There is no replay window — past-tense notifications are gone the moment the gateway hands them off.
- No client-side cache of WS messages beyond what individual store slices choose to keep.
- The optimistic-comment client UUID is held only for the lifetime of the pending row; once `office.comment.created` reconciles or the row is dropped on failure, it is forgotten.

## Scenarios

- **GIVEN** a user viewing the tasks list, **WHEN** an agent creates a subtask via `mcp.create_task`, **THEN** the new task appears in the list within ~1 second without a page refresh, driven by `office.task.created`.

- **GIVEN** a user viewing the dashboard, **WHEN** an agent completes a task and moves it to REVIEW, **THEN** the `Tasks In Progress` count decreases and `Recent Activity` shows the status change, driven by `office.task.status_changed` and `office.activity.created`.

- **GIVEN** a user viewing the dashboard for workspace A, **WHEN** an unrelated workspace B fires `office.task.created`, **THEN** the dashboard does NOT refetch and no request goes to `/api/v1/office/workspaces/A/dashboard`.

- **GIVEN** a user viewing the inbox, **WHEN** an agent requests approval, **THEN** the inbox count badge increments and the approval appears in the list, driven by `office.approval.created`.

- **GIVEN** a user viewing a task detail page, **WHEN** the agent posts a comment via `kandev comment add`, **THEN** the comment appears in the Chat tab without refreshing.

- **GIVEN** the CEO has 1 running session, **WHEN** another task starts and the agent now has 2 sessions, **THEN** the sidebar agent row badge updates to `2 live` within 2 seconds without a page refresh.

- **GIVEN** the CEO has 2 running sessions, **WHEN** both complete, **THEN** the sidebar indicator returns to the idle status dot within 2 seconds.

- **GIVEN** the dashboard agent cards panel is showing the CEO as `finished`, **WHEN** a wakeup lands and the CEO's session enters `RUNNING`, **THEN** within ~1 second of `session.state_changed -> RUNNING` the card flips to `Live now` with a pulsing dot and the task pill.

- **GIVEN** the CEO's session reaches `IDLE`, **WHEN** the dashboard agent cards panel receives `session.state_changed`, **THEN** the card flips back to `Finished {relativeTime}` without manual refresh.

- **GIVEN** a task is reassigned from agent A to agent B, **WHEN** the next render occurs, **THEN** the task pill moves from agent A's card to agent B's card on the dashboard agent cards panel.

- **GIVEN** the user is on `/office` and a task is in progress, **WHEN** the agent transitions the task to `done`, **THEN** the `Tasks In Progress` count decrements and the `Run Activity` chart updates the current-day bar, driven by `office.task.status_changed`.

- **GIVEN** the user types "looks good" and clicks send, **WHEN** the request is in flight, **THEN** the comment appears at the bottom of the thread with faded styling and a `Sending...` indicator within 50 ms, and the send button is disabled.

- **GIVEN** a pending comment is showing, **WHEN** the server returns 201, **THEN** the pending styling is removed and the comment renders with the server-provided author and timestamp; no visible flash or layout shift.

- **GIVEN** a pending comment is showing, **WHEN** the server returns 500, **THEN** the pending comment is removed, the draft text is restored to the textarea, the send button re-enables, and a toast says `Failed to send comment - please try again.`

- **GIVEN** the assignee agent is paused, **WHEN** the user opens the task chat, **THEN** the input area shows `Agent is paused - resume it for replies` before the user types anything.

- **GIVEN** the assignee agent is paused, **WHEN** the user submits a comment, **THEN** the comment is saved and shows `Queued - agent paused` instead of `Sending...`.

- **GIVEN** the inline "agent paused" notice is showing, **WHEN** the user resumes the agent, **THEN** the notice disappears within 2 seconds without a page reload, driven by `office.agent.updated`.

- **GIVEN** the user posted a comment and a session is `RUNNING` for this task, **WHEN** the comment confirms, **THEN** it shows `Agent is replying...` with a typing-style indicator until the agent posts a reply comment.

- **GIVEN** the user posted a comment and the assignee agent is busy running 2 other tasks, **WHEN** the comment confirms, **THEN** it shows `Awaiting agent (2 ahead)`.

- **GIVEN** a user comment is showing `Awaiting agent`, **WHEN** an agent reply comment for this task arrives via `office.comment.created`, **THEN** the awaiting indicator disappears.

- **GIVEN** a user comment carries an `office.run.queued` event, **WHEN** the run progresses to `claimed`, **THEN** the per-comment run-status badge updates live without a refresh.

- **GIVEN** a per-comment run-status badge is showing `failed`, **WHEN** an agent reply for the task arrives, **THEN** the badge hides.

- **GIVEN** a task has an active session, **WHEN** the user opens the task detail page, **THEN** the page header shows `<spinner /> Working` next to the title, and the inline session entry appears at its chronological position in the comments timeline, expanded by default.

- **GIVEN** an active session entry is rendered, **WHEN** the session reaches a terminal state, **THEN** the page-header `Working` indicator disappears and the inline entry collapses to a one-line summary that stays in the timeline.

- **GIVEN** an active session entry's transcript is streaming, **WHEN** new message chunks arrive and the user is already at the bottom of the chat, **THEN** the chat container auto-scrolls; **WHEN** the user has scrolled up, **THEN** the chat container does not yank focus.

## Out of scope

- Polling fallbacks of any kind. If the WS connection is down, surfaces stay as-is until the connection recovers and the next event arrives.
- Cross-workspace event subscriptions. A client only receives events for its active workspace.
- Replacing the Zustand `refetchTrigger` mechanism with React Query / SWR.
- Optimistic UI updates outside of user-initiated comments (dashboard metrics, agent state, task properties beyond comment send all wait for server confirmation via event).
- Animating chart bar transitions on update.
- Retry-on-error UI for failed comment sends (clicking a "retry" button on a failed comment). Draft restoration covers the common case.
- Auto-resuming a paused agent when the user posts a comment. The notice invites manual resume; a combined "send + resume" action is a future iteration.
- Editing user comments after submission.
- Optimistic rendering of agent-generated comments. Those flow through the WS stream and are not user-initiated.
- Live streaming of agent transcripts inside dashboard agent cards. Card expanded run rows are header-only in v1; embedding `<AdvancedChatPanel>` per row is a follow-up.
- A global "N agents working" badge in the topbar.
- Per-task progress percentages.
- Click-to-jump from a sidebar live badge directly into a specific running session.

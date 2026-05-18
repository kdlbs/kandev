---
status: draft
created: 2026-05-02
owner: cfl
---

# Office Live Updates

## Why

Every office page initially fetches data on mount and never updates after that. When an agent completes a task, changes status, posts a comment, or fans out a subtask, the user sees stale data until they manually refresh. The office model lets agents trigger other agents, so it is normal for several agents to be running concurrently; without real-time updates the user cannot see fan-out happen, cannot tell if a submitted comment was actually received, and cannot tell whose turn it is to act.

This spec captures the shared live-update contract across the office UI: WS event forwarding, per-page subscription scopes, sidebar live-agent indicators, dashboard reactivity, the cross-surface UX-parity rules that follow from those, and the optimistic-comment lifecycle.

Note on history: `office-ux-parity` was largely a retroactive-unification effort - it took live-presence affordances that had already shipped piecewise (sidebar per-agent badge, inline session entries in the task chat) and unified them across the task page, the sidebar Dashboard row, and the properties panel. Its requirements are folded into the baseline below rather than tracked as a separate surface.

## What

### A. WS event forwarding model

The backend already publishes internal events on its event bus (`task.updated`, `task.moved`, `office.comment.created`, `agent.completed`, ...). An office WS handler subscribes to office-relevant events and forwards them to connected clients scoped by the client's active workspace. The frontend WS client receives these events and updates the Zustand store; all pages read from the store, so they re-render automatically.

Forwarded events:

| Backend event | WS action | Store update | Surfaces |
|---|---|---|---|
| `task.created` | `office.task.created` | Append to `office.tasks.items` | Tasks list, dashboard task count, Recent Tasks |
| `task.updated` | `office.task.updated` | Patch task in `office.tasks.items` | Tasks list, dashboard Recent Tasks, task detail properties |
| `task.moved` | `office.task.moved` | Update task status | Tasks list, dashboard metrics |
| `office.task.status_changed` | `office.task.status_changed` | Update task status | Tasks list, dashboard, task detail header |
| `office.comment.created` | `office.comment.created` | Append to comments | Task detail chat |
| `agent.completed` | `office.agent.completed` | Update agent status | Agents list, dashboard, task detail |
| `agent.failed` | `office.agent.failed` | Update agent status | Agents list, dashboard |
| `office.agent.updated` | `office.agent.updated` | Patch agent (status, identity) | Sidebar, agent detail, dashboard cards |
| `office.approval.created` | `office.approval.created` | Increment inbox count | Inbox, dashboard approvals card |
| `office.approval.resolved` | `office.approval.resolved` | Update inbox | Inbox |
| `office.wakeup.queued` | `office.wakeup.queued` | Update agent run state | Agent detail runs tab |
| `office.activity.created` | `office.activity.created` | Prepend to activity feed | Dashboard recent activity, Activity page |
| `session.state_changed` | `session.state_changed` | Patch `taskSessions` by id | Dashboard agent cards, sidebar live badges, task page header, inline session entries |
| `session.message.added` / `.updated` | (same) | Append/patch streamed messages | Inline session transcript on task detail |
| `office.run.queued` | `office.run.queued` | Patch comment run status (`queued`) | Per-comment run badge |
| `office.run.processed` | `office.run.processed` | Patch comment run status (`claimed | finished | failed | cancelled`) | Per-comment run badge |

### B. Subscription scopes

Each page subscribes only to events it consumes. The WS client deduplicates subscriptions automatically.

| Surface | Subscribes to |
|---|---|
| Dashboard | `task.created`, `task.moved`, `task.updated`, `task.status_changed`, `agent.completed`, `agent.failed`, `activity.created`, `approval.created`, `session.state_changed`, `office.agent.updated` |
| Tasks list | `task.created`, `task.updated`, `task.moved`, `task.status_changed` |
| Task detail | `comment.created`, `task.updated`, `task.status_changed`, `session.state_changed`, `session.message.added`, `session.message.updated`, `run.queued`, `run.processed` |
| Agents list | `agent.completed`, `agent.failed`, `wakeup.queued`, `agent.updated`, `session.state_changed` |
| Agent detail | `agent.completed`, `wakeup.queued`, `activity.created`, `agent.updated` |
| Inbox | `approval.created`, `approval.resolved` |
| Activity | `activity.created` |
| Sidebar (global) | `session.state_changed`, `office.agent.completed`, `office.agent.failed`, `office.agent.updated` |

### C. Workspace scoping

Events are scoped by workspace ID. The WS handler forwards an event to a client only if the event's `workspace_id` matches the client's active workspace. Clients send their active workspace ID on connect; this is already available from user settings.

Either (a) the office broadcaster filters by `workspace_id` on the server side, or (b) every forwarded payload includes `workspace_id` and the client-side office WS handler filters before dispatching to the store. The implementation picks one; the observable contract is the same: a client viewing workspace A MUST NOT see refetches triggered by events from workspace B.

### D. Sidebar live-agent indicators

The office sidebar lists agents. Each agent row in `SidebarAgentsList` shows a live indicator when the agent has one or more active sessions:

- A pulsing blue dot (`animate-pulse`).
- A small text badge with the active-session count (e.g. `2 live`).

When the agent has zero active sessions, the indicator collapses back to the static status dot already in place. No layout shift.

The sidebar Dashboard row also carries a workspace-wide `* N live` pill, where `N` is the total number of `RUNNING | WAITING_FOR_INPUT` sessions in the workspace. Visual is identical to the per-agent badge.

Active sessions are counted per-agent (not globally) for the per-agent rows. The count source is derived client-side from the existing `taskSessions` store keyed by `agent_instance_id`, kept fresh by the WS session events the client already receives - no extra fields on the agent payload required.

### E. Dashboard reactivity

The dashboard surfaces specified in `dashboard.md` update without page refresh as events arrive:

- `office.task.created`, `office.task.updated`, `office.task.moved`, `office.task.status_changed` cause refetch / re-render of: `Recent Tasks`, `Tasks In Progress`, the `Run Activity` chart, and the `Recent Activity` feed.
- `office.agent.completed` and `office.agent.failed` update the `Agents Enabled` card subtitle (running / paused / errors line).
- `session.state_changed`, `office.task.updated`, `office.agent.updated` cause the per-agent cards panel to refetch `GET /api/v1/office/workspaces/:wsId/agent-summaries` and replace its state. No optimistic updates - the server is the source of truth and the response is small (N agents x <=5 sessions each).

The plumbing uses the existing `OfficeEventBroadcaster` -> `useOfficeRefetch("dashboard")` pattern. No polling, no `setInterval`, no React Query refetch intervals.

### F. Task detail live presence

While a task has at least one active session (`state in {RUNNING, WAITING_FOR_INPUT}`):

- The task page header shows a small `<IconLoader2 animate-spin /> Working` indicator next to the task title. Clicking it scrolls the timeline so the active session entry is visible. Hidden when no active session, with no layout reservation.
- An **inline session entry** appears at its chronological position in the comments timeline (one entry per session for the task, ordered by `session.startedAt`):
  - Active session entry is expanded by default. Header reads `RUNNING * Working * for {elapsed} * ran {N} commands`. Body embeds `<AdvancedChatPanel taskId sessionId hideInput />`. `{N}` is derived from `messages.bySession[sessionId]` filtered for `type === "tool_call"`.
  - Completed session entry (`COMPLETED | FAILED | CANCELLED`) is collapsed by default to `{Agent} worked for {duration} * ran {N} commands`. Click re-expands the full transcript.
- For tasks with more than 50 sessions, only the 50 most recent render inline; an explicit "Show older sessions" link expands the rest.
- Auto-scroll: when a new active session entry first appears, scroll the chat container to the bottom **only if** the user is already at the bottom (within ~80px). Same rule for new streaming message chunks.

All of this is driven by `session.state_changed`, `session.message.added`, `session.message.updated`, `office.comment.created`, and `office.task.updated`. No polling.

### G. Optimistic comments

User comments on a task render optimistically before server confirmation. The lifecycle has three observable states:

1. **Sending** - between the user clicking send and the server confirming. The comment appears at the bottom of the thread within 50 ms with reduced opacity and a `Sending...` sub-label. The send button is disabled while the submission is in flight.
2. **Awaiting agent** - server confirmed the comment is persisted, but the assignee agent has not yet replied. The "awaiting" sub-label adapts to the agent's actual situation:
   - **Agent paused / stopped / pending_approval** -> `Queued - agent paused` with a link to the agent.
   - **Agent currently working on this task** (a session for this task is `RUNNING`) -> `Agent is replying...` with a typing-style indicator.
   - **Agent currently busy with N other tasks** -> `Awaiting agent ({N} ahead)`, where N comes from `selectActiveSessionsForAgent` over the existing `taskSessions` store (same selector the sidebar uses).
   - **Agent idle, not paused** -> `Awaiting agent` (default).
3. **Resolved** - the assignee agent posts a reply comment after this user comment. The waiting affordance disappears.

When the server confirms the comment (`office.comment.created` for the same payload), pending styling resolves to normal with no visible flash or layout shift. When the server fails (network error, 5xx, validation error), the pending comment is removed from the thread, the draft text is restored to the textarea, the send button re-enables, and a toast surfaces the failure.

File attachments and pasted images follow the same lifecycle: pending appears immediately, error restores both the text and the file selection to the input.

A pending comment is matched against the server-confirmed comment by a client-generated UUID echoed back in the WS event payload.

### H. Per-comment run-status badge

Each user comment on a task carries an associated run-status badge driven by `office.run.queued` and `office.run.processed`. The badge renders five states (`queued`, `claimed`, `finished`, `failed`, `cancelled`) gated on no later agent reply existing for the task. When an agent reply lands (via `office.comment.created`), the badge hides reactively.

The badge data flows through `CommentDTO`, which carries `runId`, `runStatus`, and `runError` joined through `office_runs.idempotency_key = "task_comment:<comment_id>"` via a batched lookup.

### I. Agent-paused input notice

When the assignee agent (or the workspace CEO, for unassigned tasks) is paused, the chat input area shows a single inline notice above the textarea: `Agent is paused - resume it for replies` with a link to the agent. The notice appears before the user types and updates reactively from `office.agent.updated` / `office.agent.status_changed`. When the user resumes the agent, the notice disappears within 2 seconds without a page reload.

### J. UX parity rules

The same live-presence affordances appear on every surface a task or agent is referenced:

- Sidebar Dashboard row, sidebar per-agent rows, dashboard agent cards, and the task page header all surface live state from the same WS-driven `taskSessions` store. There is no surface where an agent looks idle on one page and live on another.
- `office.task.updated` is a single generic event for every property mutation (priority, project, parent, assignee, blockers add / remove, participants add / remove). Status and comment events keep their existing dedicated channels (`office.task.status_changed`, `office.comment.created`). Frontend property panels subscribe via the office-task subscription path and patch the local cache by re-fetching the task DTO.
- Property-panel edits on the task detail page are fully optimistic with rollback + toast on failure. No inline per-field error state.
- No surface introduces a `setInterval` or polling fallback. If the WS connection is down, surfaces stay as-is until the connection recovers and the next event arrives.

## Data model

Live updates are **not** a persisted feature surface — there is no `live_subscriptions` table, no event log replay buffer, no per-client cursor. The contract is built entirely from already-persisted entities (`tasks`, `task_sessions`, `office_runs`, `office_comments`, `office_approvals`, `office_activity`, `office_agents`, `provider_health_state`, `office_route_attempts`) plus three pieces of in-memory state on the backend and one piece on the frontend.

### Backend in-memory state (gateway hub)

Maintained by `*Hub` in `internal/gateway/websocket/hub.go`. None of this survives a process restart.

```
Hub
  clients                map[*Client]bool                  // all connected sockets
  taskSubscribers        map[taskID]map[*Client]bool       // task.* notifications
  sessionSubscribers     map[sessionID]map[*Client]bool    // session.*, shell, git, file notifications
  userSubscribers        map[userID]map[*Client]bool       // user.settings.updated
  runSubscribers         map[runID]map[*Client]bool        // run.event.appended (run detail page)
  sessionMode            *sessionModeTracker               // focus → fast/slow poll mode
```

```
Client
  ID                    string         // server-generated socket id
  conn                  *websocket.Conn
  send                  chan []byte    // 256-deep outbound buffer
  subscriptions         map[taskID]bool
  sessionSubscriptions  map[sessionID]bool
  sessionFocus          map[sessionID]bool   // strict subset of sessionSubscriptions
  userSubscriptions     map[userID]bool
  runSubscriptions      map[runID]bool
  closed                bool
```

All maps are guarded by `Hub.mu` / `Client.mu`. Subscription state is shared between the per-client map (for cleanup on disconnect) and the per-key map (for fan-out on broadcast). The two are kept consistent under `Hub.mu.Lock()`.

### Backend broadcaster state

`OfficeEventBroadcaster` (`internal/gateway/websocket/office_notifications.go`) holds one `bus.Subscription` per office event subject. `SessionStreamBroadcaster` holds one per session-stream subject. Both are cleaned up when the parent `ctx` cancels.

### Event bus subjects

In-memory `MemoryEventBus` (`internal/events/bus/memory.go`) maintains `map[subject][]*memorySubscription`. Subjects are flat strings (e.g. `office.comment.created`) except for the per-id fan-out path `office.run.event_appended.<runID>` (built by `events.BuildOfficeRunEventSubject`). Subscriptions are not persisted; a process restart loses every subscription and the bus re-registers them on next boot via the same `RegisterEventSubscribers` / `RegisterOfficeNotifications` calls.

### Forwarded event payload

Every office event published to the bus is forwarded by `OfficeEventBroadcaster` as a `ws.Message`:

```
ws.Message
  id          string             // empty for notifications
  type        "notification"
  action      string             // e.g. "office.comment.created"
  payload     json.RawMessage    // event-specific shape; MUST include workspace_id when scoped
  timestamp   time.Time          // server clock, UTC
  metadata    map[string]string  // optional
```

The payload `workspace_id` field is the scoping key. Office event-payload structs (`TaskMovedData`, `TaskUpdatedData`, `TaskStatusChangedData`, etc., defined in `internal/office/service/event_subscribers.go`) embed `WorkspaceID` as JSON `workspace_id`. Routing-related payloads (`OfficeProviderHealthChanged`, `OfficeRouteAttemptAppended`, `OfficeRoutingSettingsUpdated`) MUST include `workspace_id` so the frontend filter can scope them. Events without a `workspace_id` (legacy or genuinely workspace-agnostic) are treated as in-scope by the frontend filter.

### Frontend client state

`WebSocketClient` (`apps/web/lib/ws/client.ts`) holds ref-counted subscription maps:

```
WebSocketClient
  status                "idle"|"connecting"|"open"|"closed"|"error"|"reconnecting"
  subscriptions         Map<taskID, count>
  sessionSubscriptions  Map<sessionID, count>
  sessionFocusCounts    Map<sessionID, count>   // strict subset
  userSubscriptionCount number
  runSubscriptions      Map<runID, count>
  pendingRequests       Map<requestId, {resolve, reject, timeout}>
  pendingQueue          string[]                 // outbound frames buffered while not open
  reconnectAttempts     number
```

The store layer (`apps/web/lib/state/slices/office/office-slice.ts`) holds a single `officeRefetchTrigger: string` field that pages watch. Office WS handlers either patch the store directly (task status, agent status, routing) or call `setOfficeRefetchTrigger(type)` to invalidate a page-scoped fetch, where `type` is one of `"dashboard" | "tasks" | "agents" | "inbox" | "activity" | "comments:<taskId>" | "task:<taskId>" | "runs" | "routines" | "costs" | "approvals"`.

## API surface

WS events listed in section A above. The agent-summaries endpoint that backs dashboard agent cards is documented in `dashboard.md`. No HTTP endpoints are introduced by the live-updates surface itself beyond the per-property mutation events already covered by `PATCH /tasks/:id` and the comment-run lifecycle endpoints.

### Subscription control frames

The frontend issues these as `type: "request"` frames; the backend replies with `type: "response"` `{success: true, ...}` or `type: "error"`.

| Action | Payload | Effect |
|---|---|---|
| `task.subscribe` | `{task_id}` | Adds client to `taskSubscribers[task_id]`. |
| `task.unsubscribe` | `{task_id}` | Removes client from `taskSubscribers[task_id]`. |
| `session.subscribe` | `{session_id}` | Adds client to `sessionSubscribers[session_id]`; server pushes an initial session-data snapshot (git status). Triggers session-mode recomputation. |
| `session.unsubscribe` | `{session_id}` | Removes client; recomputes session mode. |
| `session.focus` | `{session_id}` | Marks the session as actively viewed by this client; lifts polling to fast mode and re-pushes the session-data snapshot. |
| `session.unfocus` | `{session_id}` | Releases focus; debounced fallback to slow or paused mode. |
| `user.subscribe` | `{user_id?}` | Subscribes to `user.settings.updated`. `user_id` must equal `store.DefaultUserID` (single-user model) or the server returns `ErrorCodeForbidden`. |
| `user.unsubscribe` | `{user_id?}` | Inverse of `user.subscribe`. |
| `run.subscribe` | `{run_id}` | Subscribes to `run.event.appended` for one office run. Server replays no state — caller fetches the snapshot via REST. |
| `run.unsubscribe` | `{run_id}` | Inverse of `run.subscribe`. |

## State machine

### WS connection (frontend)

Tracked by `WebSocketClient.status`. Server-side, the connection is just an open `websocket.Conn`; the state machine below is the observable surface used by hooks and the connection indicator in the topbar.

| State | Entered when | Outgoing transitions |
|---|---|---|
| `idle` | Client constructed, `connect()` not yet called. | `connect()` → `connecting`. |
| `connecting` | `connect()` called and `new WebSocket(url)` issued. | `socket.onopen` → `open`. `socket.onerror` → `error`. `socket.onclose` → `closed` (then auto-reconnect logic). |
| `open` | `socket.onopen` fired. Reconnect attempts reset to 0. Queued frames are flushed; `resubscribe()` re-sends every entry in the subscription maps. | `socket.onclose` → `closed`. `disconnect()` → `closed`. |
| `closed` | `disconnect()` called or socket closed and reconnect is disabled / cap reached. Pending requests are rejected with `WebSocket connection closed`. | `connect()` → `connecting`. |
| `error` | `socket.onerror` fired, or reconnect cap exceeded. Pending requests are rejected. | `connect()` → `connecting`. |
| `reconnecting` | Socket closed unexpectedly, reconnect is enabled, attempts < cap. A timer is armed with exponential backoff (initial 1s, multiplier 1.5, max 30s, cap 10 attempts). | Timer fires → `connecting`. `disconnect()` → `closed`. |

Server-side ping/pong: every `pingPeriod` (54s = 60s pong-wait × 0.9) the server sends a WS ping; missing the pong before `pongWait` (60s) closes the connection. Max inbound frame is 32 MiB (raised to accommodate base64 image attachments).

### Subscription state (per `(client, key)` pair)

| State | Entered when | Outgoing transitions |
|---|---|---|
| `unsubscribed` | Initial state, or after `*.unsubscribe`. No fan-out. | `*.subscribe` → `subscribed`. |
| `pending` | `*.subscribe` called while `status != "open"`. Frame buffered in `pendingQueue`; refcount incremented; intent recorded in subscription map. | Status becomes `open` → frame flushed → `subscribed`. `disconnect()` → `unsubscribed`. |
| `subscribed` | Server responded `{success:true}` to `*.subscribe`. Client appears in the matching `Hub` map; broadcasts fan out. | `*.unsubscribe` (last ref drops) → `unsubscribed`. Socket closes → backend `unsubscribed`, frontend records intent retained for `resubscribe()` on reconnect. |

No per-message ack: notifications are fire-and-forget. Only request frames (those carrying an `id`) get a paired `response`/`error` and only when the client originated the request — broadcast notifications carry no `id` and the client never acks.

### Session-mode tracker (per session)

Driven by `session.focus` / `session.unfocus` and subscriber counts. Listeners on the backend toggle agent polling cadence per workspace.

| State | Trigger to enter |
|---|---|
| `paused` | No subscribers remain. |
| `slow` | At least one subscriber, zero focused. |
| `fast` | At least one focused subscriber. Server upgrades workspace poll cadence. |

Transitions out of `fast` are debounced (see `hub_session_mode.go`) to absorb tab-switch churn.

### Optimistic comment lifecycle

| State | Entered when | Visible affordance |
|---|---|---|
| `sending` | User clicks send. Local row appended within 50 ms. | Faded row, `Sending...` sub-label, send button disabled. |
| `awaiting_agent` | POST returns 2xx **or** matching `office.comment.created` arrives (whichever first). | One of: `Queued - agent paused`, `Agent is replying...`, `Awaiting agent (N ahead)`, `Awaiting agent`. |
| `resolved` | Assignee agent posts a reply comment to the same task (`office.comment.created` with `author_type != "user"`). | Awaiting indicator disappears. |
| `failed` | POST returns non-2xx **and** no matching `office.comment.created` has been seen. | Pending row removed; draft restored; toast surfaced. |

Matching pending row to confirmation: the client embeds a generated UUID in the create-comment payload; the server echoes it back in the `office.comment.created` WS event so the optimistic row replaces (rather than duplicates) the confirmed one.

## Permissions

The kandev backend runs in a single-user model (`store.DefaultUserID = "default-user"`). Authorization rules below describe the observable contract; multi-user enforcement would extend them.

- **Workspace scoping** — every office WS notification carries `workspace_id` in its payload (when scoped). The current implementation broadcasts office notifications to every connected client and the frontend filters by `workspace_id === workspaces.activeId`. The observable contract is: a client viewing workspace A MUST NOT act on events originating in workspace B. Either server-side filtering or client-side filtering satisfies the contract.
- **User subscription** — `user.subscribe` accepts only the caller's own user id. Subscribing to another user's id returns WS error `forbidden` (`ws.ErrorCodeForbidden`, "cannot subscribe to another user"). Symmetric rule for `user.unsubscribe`.
- **Task / session / run subscriptions** — no per-key authorization in the current implementation. Any connected client can subscribe to any `task_id`, `session_id`, or `run_id`. Information-leak risk is bounded by the single-user model. A future multi-user iteration MUST gate `task.subscribe` / `session.subscribe` / `run.subscribe` on workspace membership; the gate point is `Hub.SubscribeToTask` etc.
- **Agent-originated subscriptions** — agentctl-initiated MCP tool calls reach the backend over a tunnelled WS connection but do **not** carry a separate identity over this surface; agents cannot subscribe to other agents' streams because subscription frames are scoped per-socket and agents never open the user-facing office WS.
- **Subscription requests are not approvals** — there is no approval / review gate on subscribing. Once authorized to connect, a client may subscribe and unsubscribe freely.

## Failure modes

| Dependency / scenario | Observable behavior |
|---|---|
| **WS network drop** | Socket transitions `open → closed → reconnecting`. Pending request promises stay armed until `cleanupPendingRequests()` rejects them at the reconnect cap. Subscription maps are retained. On `open` the client flushes the buffered outbound queue, then re-issues every `subscribe` / `focus` / `user.subscribe` / `run.subscribe` frame from `resubscribe()`. **No replay** of missed notifications — surfaces refetch on next event or stay stale until then. |
| **Reconnect cap exceeded** | After `maxAttempts` (default 10) consecutive failures with exponential backoff capped at 30s, status moves to `error`, pending requests are rejected with `WebSocket connection closed`, and no further automatic reconnects occur. The topbar surfaces a connection indicator; the user must reload to recover. |
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

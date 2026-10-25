---
status: draft
system: office
requirements:
  - REQ-OFFICE-LIVE-UPDATES-001
created: 2026-05-02
owners:
  - cfl
---
# Office Live Updates System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-LIVE-UPDATES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-LIVE-UPDATES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

Server-side ping/pong: every `pingPeriod` (54s = 60s pong-wait × 0.9) the server sends a WS ping; missing the pong before `pongWait` (60s) closes the connection. Max inbound frame is 32 MiB. Current prompt files are staged over authenticated HTTP and only bounded descriptors cross this socket; legacy inline base64 attachments remain subject to the frame and compatibility limits.

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
- **Task / session / run subscriptions** — auth-enabled connections apply resource-specific access hooks before registration. Task and session use `AuthorizeTaskAccess` and `AuthorizeSessionAccess`. Runs resolve workspace and use `AuthorizeWorkspaceAccess`, denying an empty workspace. Auth-disabled connections retain single-user behavior. See `apps/backend/AGENTS.md`.
- **Agent-originated subscriptions** — agentctl-initiated MCP tool calls reach the backend over a tunnelled WS connection but do **not** carry a separate identity over this surface; agents cannot subscribe to other agents' streams because subscription frames are scoped per-socket and agents never open the user-facing office WS.
- **Subscription requests are not approvals** — there is no approval / review gate on subscribing. Once authorized to connect, a client may subscribe and unsubscribe freely.

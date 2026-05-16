---
status: draft
created: 2026-05-02
owner: cfl
needs-upgrade: [data-model, state-machine, permissions, failure-modes, persistence-guarantees]
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

## API surface

WS events listed in section A above. The agent-summaries endpoint that backs dashboard agent cards is documented in `dashboard.md`. No HTTP endpoints are introduced by the live-updates surface itself beyond the per-property mutation events already covered by `PATCH /tasks/:id` and the comment-run lifecycle endpoints.

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

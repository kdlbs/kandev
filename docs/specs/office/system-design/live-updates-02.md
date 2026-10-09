---
status: draft
system: office
requirements:
  - REQ-OFFICE-LIVE-UPDATES-001
  - REQ-OFFICE-LIVE-UPDATES-002
created: 2026-05-02
updated: 2026-10-06
owners:
  - cfl
---
# Office Live Updates System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-LIVE-UPDATES-001` during migration and defines the mounted diagnostic read and provider-health publication lifecycle for `REQ-OFFICE-LIVE-UPDATES-002`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-LIVE-UPDATES-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-OFFICE-LIVE-UPDATES-002` | [Current-workspace diagnostic reads](#current-workspace-diagnostic-reads) (`.1` through `.8`); [Provider-health snapshot publication](#provider-health-snapshot-publication) (`.9`, `.10`) |

## Current-workspace diagnostic reads

Office owns the workspace-specific diagnostic projection and its mounted read
lifecycle. [Dynamic routing ownership](../../../decisions/2026-08-13-dynamic-agent-profile-routing.md)
remains authoritative for execution policy. The archived [Office routing
requirements](../requirements/routing.md) are not an input to this correction.
The remaining diagnostic hooks and consumers use the following bounded
contract while those surfaces exist; this design does not extend their life
or restore Office-owned routing policy.

### Components and data ownership

- `apps/web/hooks/domains/office/use-routing-preview.ts::useRoutingPreview`
  reads via `getRoutingPreview(workspaceId)` and selects
  `office.routing.preview.byWorkspace[workspaceId]`.
- `apps/web/hooks/domains/office/use-provider-health.ts::useProviderHealth`
  reads via `getProviderHealth(workspaceId)` and selects
  `office.providerHealth.byWorkspace[workspaceId]`.
- `StateProvider` owns the real `createAppStore` context; nested providers
  reuse the parent store. The existing `setRoutingPreview` and
  `setProviderHealth` actions publish only into the captured store/workspace.
  Data stays in these existing entries. Loading, error, selection lifetime,
  and request sequence stay inside each hook instance.
- `src/office-routes.tsx` registers `/office/workspace/routing` (both hooks),
  `/office/agents` (`AgentsPageClient`, preview), and `/office`
  (`OfficePageClient` containing `ProviderHealthCard`, health). Each passes
  `workspaces.activeId`. `AgentCard` selects its workspace's preview by agent
  ID. Diagnostic displays depend on existing routing-config visibility gates.

API clients remain the `office-extended-api` re-exports of
`office-routing-api`. The existing GET endpoints are
`/api/v1/office/workspaces/:wsId/routing/preview` and `/routing/health`.
Their response shapes, backend authorization, and store schema do not change.
Stable empty selector arrays remain shared constants.

### Selection and request lifetime

Remove the instance-wide successful-fetch latch. A selection-driven effect
initiates one automatic read for each committed non-empty selection lifetime,
including re-entry to a previously selected workspace. Its dependencies are
the selected workspace and stable scoped refresh callback, not response data,
success, error, or loading state. A failure therefore does not create a retry
loop, and another store write does not create another automatic read.

Within each hook, use instance-local ownership and monotonically increasing
request identity. Ownership includes the selected workspace, owning store,
and committed selection lifetime. Invalidate earlier requests at the committed
selection/unmount boundary before promise settlements can publish; a layout
effect with cleanup is suitable. Reset current error/loading at that boundary
and keep the no-workspace result neutral. Do not mutate ownership refs during
render. A workspace string comparison alone cannot fence A-to-B-to-A races.

Automatic and manual reads use the same admission and settlement path. A
refresh bound to a departed selection must not change the current selection's
request state. Capture workspace/store and a fresh request identity before
transport; set current loading and clear its error. After awaiting transport,
check both ownership and newest request identity before every data, error, or
loading-completion publication, including `catch` and `finally`. Publish a
valid success to its captured workspace key, with the existing `?? []`
normalization. A valid failure uses the existing localized error fallback and
retains data. Obsolete settlements are no-ops, including cache writes to the
departed workspace. Unmount invalidates only that instance's requests.

Each instance may issue its own read even when another instance shares its
store/workspace. No module-wide fetched marker, shared generation map,
deduplication, or new coordinator is introduced. Latest-request ordering is
within an instance. Separate instances retain ordinary existing store-write
semantics; cross-instance same-key ordering is outside this contract.

### Existing live updates and mobile

`lib/ws/handlers/office.ts` independently upserts current-workspace provider
health on `office.provider.health_changed`. Routing-settings events invalidate
the existing configuration entry. `useOfficeWorkspaceData` loads agents,
projects, inbox, and meta, not these diagnostic snapshots. Dashboard refetch
and live events can mitigate visible symptoms; missing hook reads do not mean
every dashboard remains stale. The workspace-selection correction (`.1`
through `.8`) excluded HTTP-versus-event arbitration. The later bounded
provider-health publication amendment below covers `.9` and `.10` only.

This is state-only frontend work. Existing desktop sidebar and phone
`OfficePageNav` workspace pickers both drive the same store selection and
diagnostic hooks. No viewport branch, touch action, scroll owner, navigation,
markup, or copy changes. Real-hook/real-store tests satisfy the narrow
state-only exception in mobile parity; no new mobile E2E or visual preview is
needed for this correction.

### Verification boundary

Exercise both production hooks under the real `StateProvider` and
`createAppStore`, mocking only their transport functions. Select workspaces
through the actual store and observe both hook state and workspace-keyed
entries. Use distinct response payloads and deferred promises to test
selection, overlap, invalidation, empty results, current failures, manual
recovery, independent instances/stores, and unmount. Do not replace the store,
mock publication actions, or test only a helper predicate. There are no new
metrics or persistence writes beyond the existing diagnostic entries.

## Provider-health snapshot publication

### Boundary and existing identity

This amendment applies only to `useProviderHealth`, not routing preview,
run attempts, routing policy, or a general HTTP/WS ordering protocol. Keep
the committed selection/unmount and newest-request guards above intact.
The broader migrated documents remain draft; the prior workspace-selection
package remains an implemented historical record with its original exclusions.

`ProviderHealth` in `lib/state/slices/office/routing-types.ts` identifies a row
within a workspace by `(provider_id, scope, scope_value)`. Provider, model,
and tier scopes are distinct even for the same provider. `workspace_id` is
optional payload metadata; the captured workspace entry is the enclosing
identity. Do not collapse keys to provider ID or concatenate unescaped strings
that can collide. Compare the three fields directly or encode a tuple safely.

`createAppStore` uses Immer. `upsertProviderHealth` replaces the matching row
object or appends a new one, preserving untouched row references. The
registered `office.provider.health_changed` handler extracts a new row and
calls that action after its active-workspace filter. Legacy events without
`workspace_id` target the active workspace; invalid provider/scope payloads
do not publish. There is no live row-deletion action/event on this path.
`setProviderHealth` is an immediate synchronous whole-entry replacement;
its signature and atomic publication remain unchanged.

### Request-local reconciliation

Use the hook's owning store API (`useAppStoreApi`) to capture the actual
workspace rows immediately before each admitted GET. Read from the store,
not the render's selected array: back-to-back live updates and manual reads
may occur before React rerenders. Retain the immutable row references for
that request only. Capture before transport, and fence settlement with the
existing `isCurrent` check before reading current rows or publishing.

At accepted success, read that same store/workspace's current rows. A current
row is protected when its key was absent at request start or its object
reference differs from the start row for that key. Preserve the entire
current row, including error, retry, backoff, and diagnostic metadata; do not
choose by severity or compare optional server dates. Repeated events with
equal values still replace identity and count as observations during the read.

Build one result from `res.health ?? []`: for each snapshot key, substitute
its protected current row when present, otherwise take the snapshot row.
Append protected current keys absent from the snapshot once. Unchanged
current rows absent from the snapshot are omitted. An empty snapshot thus
retains only protected rows; a fresh empty read with no intervening changes
clears the entry. Keep returned order for snapshot keys and current order
for appended keys. Publish once through the existing setter without an
`await`, subscription, timer, or other scheduling boundary between the
current read, reconciliation, and setter. A later WS event uses its normal
upsert path immediately.

Each new read captures a new baseline. An event observed before that read
does not permanently pin its row. Unrelated workspace writes and unrelated
store writes cannot protect this request's keys. The hook's callback depends
on its owning store as well as workspace/action ownership, so replacement
of the context store invalidates departed work rather than publishing into it.
No module cache, retained event journal, new action, or schema field is needed.

### Isolation and limits

Each hook retains independent request/error/loading state. Separate stores
with the same workspace string do not share baselines or results. Two pending
consumers of one store can both preserve a live row observed since their own
admission; one consumer's unmount does not invalidate its sibling. Reconciliation
retains protected object identity when another consumer's response includes
that row, allowing an earlier outstanding read to continue recognizing it.

The comparison deliberately preserves any current row replaced during the
request, including a sibling consumer's accepted snapshot. It does not infer
the source of a change or impose latest-response order between consumers.
A later admitted fresh read can supersede earlier live health; no event
journal is added to resurrect a row already superseded by that accepted
read. Removed rows have no live tombstones on the current path; this amendment
does not add deletion arbitration. These limits retain `.7` and avoid a
universal timestamp/revision, cross-tab, or server freshness guarantee.

Valid failures retain all current data, including live upserts, and use the
existing error/loading settlement. Obsolete success, rejection, or finally
is still a no-op. Empty selection, A-to-B-to-A, stale refresh callbacks,
StrictMode cleanup/setup, and unmount continue to obey `.1` through `.8`.
There are no backend, API, persistence, classification, authorization,
metrics, or polling changes.

### Consumers and verification

`ProviderHealthCard` on `/office` collapses by provider for display;
`ProviderHealthBanner` on `/office/workspace/routing` displays non-healthy
rows. Both consume the same keyed hook state. Their display grouping must
not become the reconciliation key. Desktop and phone use these existing
surfaces and shared state. This is purely state/data publication: no markup,
copy, layout, navigation, touch, scrolling, or viewport-dependent behavior
changes. Targeted real-context tests satisfy mobile parity's state/data
exception; no new visual preview, browser/build, or mobile E2E is required.

The regression boundary is the production hook, real `StateProvider` /
`createAppStore`, real API client, and registered Office WS handler, replacing
only fetch transport. Observe returned health and actual workspace entries,
not a mocked action or a copied merge predicate. Deferred initial/manual GETs
must fail before the correction because stale healthy rows replace already
observed degraded health, with unaffected snapshot rows still hydrating.
Cover full keys, metadata, live insertions/omissions, empty snapshots, events
before/during/after reads, ordinary fresh reads, failures/overlaps, independent
workspaces/stores/consumers, and lifecycle guards. Settle and join all deferred
work even on assertion failure. The delivery matrix and commands belong in
the [bounded implementation package](../../../plans/office-provider-health-snapshot-loads/plan.md).

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

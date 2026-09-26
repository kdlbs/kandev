---
spec: docs/specs/ui/requirements/quick-chat-idle-dot.md
created: 2026-08-16
status: complete
---

# Implementation Plan: Quick Chat Idle Dot

## Overview

A red dot on the five Quick Chat entry icons (sidebar rail item, sidebar
shortcut, tablet header button, mobile header button, mobile task-switcher
sheet button) whenever a quick chat session completes a turn while the dialog
is closed. Purely frontend: `session.state_changed` and `session.turn.completed`
are broadcast to every connected client (their payloads carry no
`workspace_id`, so the gateway's workspace-broadcast path falls back to global
delivery), so the client observes every completion with no new WS
subscriptions or backend changes; per-workspace dots rely on session-tab
membership and the selector, not transport scoping. Implementation order:
unseen-idle state in the UI slice → marking hooks in the two WS handlers → dot
rendering in the entry components → E2E.

---

## Frontend

### State — `apps/web/lib/state/slices/ui/`

- `types.ts` (`QuickChatState`): add
  `unseenIdleByWorkspace: Record<string, Record<string, true>>` — workspace id
  → session id → marker. Workspace ownership lives in the key, so it survives
  hydration of a different workspace's session list (see Hydration — non-destructive union).
  Also add `lastSettledAtBySession: Record<string, string>` — session id →
  `updated_at` of the last observed settle transition (replay guard for
  `session.state_changed` marking; see Marking hooks).
  Also add `syncRevisionByWorkspace: Record<string, number>` — per-workspace mutation epoch, incremented by EVERY local server-backed tab mutation that could make an in-flight resync response stale: WS-driven upserts (`upsertQuickChatSessionFromEvent`), `addQuickChatSession` (insert AND existing-row update), the `openQuickChat` push/existing branches, `removeQuickChatSession` (direct-delete success path), `removeQuickChatSessionsForTask`, AND the optimistic rename flow (`renameQuickChatSession` / `persistQuickChatRename` — bumped BEFORE the server request, so a resync in flight cannot apply the old server name after the optimistic rename). The reconnect resync (`useQuickChatResync`) captures it before issuing the HTTP list fetch and DISCARDS a stale response (revision moved while in flight) — checked before EVERY response side effect, including `setTaskSession` for the returned `task_sessions` rows — so neither a WS-upserted tab, a locally-deleted tab, nor an optimistic rename can be clobbered by an older list. Also add returned `task_sessions` rows are guarded PER-ROW by `updated_at`: `useQuickChatResync` skips applying any returned row whose live store row has a NEWER `updated_at` (the live WS row wins). This is workspace-precise — `session.state_changed` broadcasts globally with no `workspace_id`, and an unrelated workspace's state traffic must NOT discard a valid resync response.
  Also add `tombstonedSessions: Record<string, { workspaceId: string; tombstonedAt: string }>` — client
  tombstones for deleted session identities (value carries the workspace AND
  an ISO timestamp so the 60-minute age-prune is implementable), set by `removeQuickChatSession`,
  `removeQuickChatSessionsForTask`, AND the Quick Chat dialog's delete-tab flow
  (`use-quick-chat-modal.ts` `handleConfirmClose` calls `removeQuickChatSession`
  after the successful `deleteQuickChatTask` — NOT bare `closeQuickChatSession`,
  which does not tombstone; setup-tab closes keep `closeQuickChatSession` since
  setup ids never receive task events). `syncQuickChatFromTaskEvent` /
  `upsertQuickChatSessionFromEvent` SKIP tombstoned session ids, so a
  late/out-of-order `task.created`/`task.updated` cannot resurrect a deleted
  tab (DeleteSession publishes no `session.deleted` broadcast, and the task
  lifecycle subscription is ordered independently of the HTTP resync).
  Tombstone clearing is POSITIVE-CONFIRMATION ONLY: a tombstone is cleared
  solely when an authoritative workspace list for the SAME workspace
  (`workspaceId` stored with the tombstone) positively CONTAINS the session
  (it is legitimately live again); an omission NEVER clears it (an older
  task event can still be in flight after the list), and another workspace's
  list NEVER touches it. Age-pruned like the ledger (60-minute retention
  bound, keyed on `tombstonedAt`).
  Also add `sessionOwnership: Record<string, { taskId?: string; workspaceId: string }>` —
  durable in-memory sessionId → task/workspace index, populated at EVERY
  session-insertion path — the INITIAL BOOT path (`mergeQuickChatState` in
  `apps/web/lib/state/default-state.ts`: `createAppStore` runs `mergeInitialState`
  before composing the store, so boot-loaded sessions never pass through
  `hydrateUI`), `addQuickChatSession`, `upsertQuickChatSessionFromEvent`
  (BOTH branches of the central `upsertQuickChatSession` reducer — a task
  event updating an existing row also refreshes its ownership via the
  centralized `indexSessionOwnership` helper),
  the `openQuickChat` real-session push branch (`ui-slice.ts` pushes a freshly
  started session directly, not via `addQuickChatSession`), `reconcileQuickChatSessions`
  (inserted/adopted server sessions, and refreshed for survivors), and
  `hydrateUI` — and pruned on `closeQuickChatSession`,
  `removeQuickChatSessionsForTask`, and revision-guarded reconcile/resync (NOT by hydration).
  Without it, `removeQuickChatSessionsForTask(taskId)` — the only signal for
  task delete/archive — cannot clean a marker once the session row left the
  current `sessions` list (e.g. marked workspace-A session, workspace-B
  hydration replaces A's row, then A's task-deleted event: the reducer finds
  nothing and A's marker survives until a later A sync). Boot-loaded,
  `openQuickChat`, or reconcile tabs whose ownership entry is missing have the
  same hole on task deletion.
- `ui-slice.ts` (`defaultUIState.quickChat`): default `unseenIdleByWorkspace: {}`.
- `ui-slice.ts` actions:
  - `markQuickChatUnseenIdle(sessionId: string, workspaceId: string)` — sets `unseenIdleByWorkspace[workspaceId][sessionId] = true`.
  - `clearQuickChatUnseenIdle()` — clears all workspaces (used by `openQuickChat`).
  - `clearQuickChatUnseenIdle(sessionId: string, workspaceId: string)` — deletes one entry.
  - `removeQuickChatSession(sessionId: string)` — LOOKUP PRECEDENCE is `quickChat.sessions` THEN `sessionOwnership` (an ownership entry is authoritative ownership, NOT unknown, even when the row is absent after cross-workspace hydration): removes the tab (whichever workspace), the marker from the owning workspace's map, the `sessionOwnership` entry, tombstones non-setup ids, bumps `syncRevisionByWorkspace`, runs the ledger age-prune, and re-points the active tab / closes the modal exactly like `closeQuickChatSession` when the removed session was active. A genuinely unknown id (no row, no ownership, not setup-prefixed) is a no-op.
- **Generic session-delete hook**: the backend has no dedicated `session.deleted` notification (DeleteSession publishes only the deleted-session error event), so the frontend delete flows are the cleanup points: `useSessionActions.remove` (`apps/web/hooks/domains/session/use-session-actions.ts:148`) calls `removeQuickChatSession(sessionId)` after `removeTaskSession(taskId, sessionId)`, AND `sessions-dropdown.tsx`'s `handleDeleteSession` (direct `client.request("session.delete")` + `loadSessions(true)`) calls `removeQuickChatSession(sessionId)` after a successful delete (before the refresh). The Quick Chat modal's `handleConfirmClose` routes the NON-setup branch through `removeQuickChatSession` (after successful `deleteQuickChatTask`, and ALSO for the no-taskId case — a non-setup id whose taskId cannot be resolved is still cleaned and tombstoned via the ownership lookup, not left to bare `closeQuickChatSession`); only setup-tab closes keep `closeQuickChatSession`. Without these, deleting a quick chat session from any surface leaves the tab, marker, and ownership entry until a later resync, and a delayed global state event keeps contributing to the stale tab.
- `buildOpenQuickChatAction`: clear all markers on open (`clearQuickChatUnseenIdle()`); populate `sessionOwnership` for the real-session push branch (a freshly started session is inserted directly into `quickChat.sessions`, bypassing `addQuickChatSession`).
- `buildQuickChatActions`:
  - `setActiveQuickChatSession`: clear that session's marker (workspace from the tab entry).
  - `closeQuickChatSession`: clear the closing session's marker.
  - `removeQuickChatSessionsForTask`: clear markers for the removed sessions via the `sessionOwnership` index (session ids by taskId → delete their markers from their workspace maps, drop their ownership entries), THEN filter the current sessions list — works even when the session row is absent from the list after cross-workspace hydration (reducer in `quick-chat-sync.ts`, workspace-scoped).
  - `syncQuickChatSessions`: `reconcileQuickChatSessions` in `quick-chat-sync.ts` drops that workspace's markers for sessions the server list no longer contains, populates/refreshes ownership for inserted/adopted sessions and survivors, TOMBSTONES each omitted non-setup session (with workspace + timestamp) BEFORE dropping its ownership, and clears tombstones ONLY for same-workspace sessions POSITIVELY present in the list (omission and other workspaces never clear). The OMISSION IDENTITY SET is the union of the prior workspace's session rows AND the workspace-scoped `sessionOwnership` entries — a session known only through `sessionOwnership` (its row was dropped by a cross-workspace hydration) is still omitted-and-tombstoned when the hydrated list lacks it; otherwise an empty A-list sync after a B-hydration would prune A's ownership without a tombstone and a delayed task event could re-upsert the deleted tab.
- **Public store surface**: `apps/web/lib/state/store.ts` manually enumerates the quick chat actions on `AppState` (the section declares individual `UIA["…"]` entries, not `UISliceActions`); the four new root actions (`markQuickChatUnseenIdle`, `clearQuickChatUnseenIdle`, `recordQuickChatSettled`, `removeQuickChatSession`) MUST be added to that explicit list or components/handlers selecting them from `useAppStore` will not typecheck.
- **Code-quality limits (AGENTS.md)**: `ui-slice.ts` is already 568/600 lines and `buildQuickChatActions` ~98/100 — the new ownership-index, tombstone, revision-epoch, ledger-pruning, marker-cleanup, and `removeQuickChatSession` transitions MUST be extracted into `quick-chat-sync.ts` or a dedicated `quick-chat-lifecycle.ts` module (pure reducers/helpers + thin action wrappers), keeping `ui-slice.ts` ≤600 lines and every function ≤100 lines (enforced by `apps/web/eslint.config.mjs`).
- New selector `apps/web/lib/state/slices/ui/quick-chat-unseen-selectors.ts`:
  `selectQuickChatHasUnseenIdle(state: AppState, workspaceId: string | null | undefined): boolean` — true when the workspace's marker map is non-empty; returns false for nullish or unknown workspace ids (every entry-point caller passes `string | null | undefined` — `useAppStore((s) => s.workspaces.activeId)` and the header `workspaceId?` props). Stable empty fallback, no fresh literals.
- Boot payload + typed fixtures: `apps/web/app/page.tsx` (`loadWorkspaceState` → `initialState.quickChat`), the `QuickChatState` builders in `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`, and the full-state `draft.quickChat` assignments in `apps/web/lib/state/hydration/hydrator.test.ts` gain ALL FIVE fields with empty defaults (`unseenIdleByWorkspace: {}`, `lastSettledAtBySession: {}`, `sessionOwnership: {}`, `syncRevisionByWorkspace: {}`, `tombstonedSessions: {}`) — the boot payload must explicitly reset every ephemeral map, and full-typed `QuickChatState` constructions must carry every required field.
- Hydration is a NON-DESTRUCTIVE MERGE (workspace-keyed): `hydrateUI` (`apps/web/lib/state/hydration/hydrator.ts`) merges the hydrated sessions list into `draft.quickChat.sessions` — UNION: sessions absent from the live list are ADDED (ownership indexed for them); sessions already present KEEP the live tab's fields (live wins on overlap — `QuickChatSession` has no `updated_at`, `StateHydrator` re-hydrates on every SSR `initialState` change, and a stale SSR payload captured before a `task.updated` or the optimistic rename must not regress the newer live name/taskId); tombstoned ids filtered OUT — and NEVER prunes markers/ownership and NEVER tombstones omissions (it only unions sessions and filters existing tombstones; it neither creates nor clears them). `StateHydrator` re-hydrates on every SSR `initialState` change (`state-hydrator.tsx:19-24`), and an SSR list captured before either a local delete OR a WS-upserted live tab must not destroy the current truth: pruning markers/ownership, tombstoning omitted sessions, and positive tombstone clearing happen ONLY on the revision-guarded resync path (`syncQuickChatSessions` → reconcile, after `useQuickChatResync`'s epoch + task-session-revision checks). The hydrated workspace (for ownership population and tombstone filtering) derives from `state.workspaces?.activeId` (boot payload provides it via `buildBaseState`, verified `apps/web/app/page.tsx:63-67`); when absent (partial payload) hydration still merges sessions but skips workspace-scoped side effects fail-safe.

### Marking hooks — WS handlers

- `apps/web/lib/ws/handlers/agent-session.ts` (`registerTaskSessionHandlers`, `session.state_changed`): after the existing fan-out, call a local `maybeMarkQuickChatUnseenIdle(store, sessionId, previousState, newState)` that marks only when (a) the session id is in `store.getState().quickChat.sessions` (its workspace id comes from that tab entry), (b) `!store.getState().quickChat.isOpen`, and (c) the transition is a settle transition (previous STARTING/RUNNING → new IDLE/WAITING_FOR_INPUT/COMPLETED/FAILED/CANCELLED; previous state = store row FIRST, falling back to the payload's typed `old_state` only when no row exists — boot hydration drops `task_sessions`). The REPLAY guard is a per-session settle-generation ledger, NOT the store row: `quickChat.lastSettledAtBySession: Record<sessionId, string>` records the `updated_at` of EVERY settle transition observed for ANY session (kanban and quick chat alike, regardless of `isOpen` and regardless of quick-chat membership — recording happens BEFORE the membership check), and a settle marks only when its `updated_at` is newer than the ledger entry (or no entry). The ledger is a MONOTONIC MAX per session: a settle with `updatedAt <= recordedAt` is suppressed AND never overwrites the entry — an out-of-order delayed older settle must not regress the ledger, or a later replay of the newer event would falsely mark after the dialog cleared the dot. Recording before membership is required for the event-before-tab guarantee: a settle that arrives before its tab exists records the generation, so adding the tab later and replaying the original event cannot mark (the ledger already has that `updated_at`). The store row is unreliable for replay detection because `hydrateSession` runs `deepMerge(draft.taskSessions, state.taskSessions)` unconditionally — an in-flight SSR snapshot can regress a settled row back to RUNNING. A settle whose payload lacks `updated_at` has NO stable generation (the fallback persistence path can publish without it — `persistStrictTaskSessionState` passes nil when the refreshed row's `UpdatedAt` is zero and the publisher omits the key): it FAILS CLOSED — never marks and never records the ledger, because a replayable ungenerationed settle must not raise a dot. The ledger is age-bounded: the pure helper `pruneStaleSettledLedger(bySession, now)` (60-minute retention window — far longer than the event-before-tab tab-upsert, reconnect, and hydration-regression replay windows) runs at `recordQuickChatSettled` AND at `closeQuickChatSession`, `removeQuickChatSessionsForTask`, `reconcileQuickChatSessions`, and `hydrateUI` pruning; entries younger than the window are never dropped on those paths (still needed for the replay window), older entries are evicted, so a long-lived client cannot accumulate a key per settled kanban session. The ledger is ALSO size-capped at a documented maximum (500 entries): when the cap is exceeded, the oldest entries by timestamp are evicted — the practical replay window is seconds, so the cap only ever evicts entries far older than any realistic replay, and age window + size cap bound memory deterministically regardless of workload. Define the two state sets locally (do not import from `hooks/domains/session/use-session-messages.ts` — avoid a `hooks → lib/ws` import edge).
- `apps/web/lib/ws/handlers/turns.ts` (`registerTurnsHandlers`, `session.turn.completed`): capture `const wasCompleted = turns.bySession[sessionId]?.some(t => t.id === payload.id && t.completed_at)` BEFORE the existing `addTurn`/`completeTurn` calls (both mutate the store, so a post-mutation check would see every delivery as already completed); then, after `completeTurn`, mark only when (a) quick chat session, (b) dialog closed, and (c) `!wasCompleted` — a re-delivered completion of an already-recorded turn must not mark (spec re-delivery failure mode). All `session.turn.completed` events are treated uniformly: `AbandonOpenTurns` (resume/startup cleanup, rejected-message cleanup) publishes the same payload shape with no abandonment discriminator (`had_output` is forced true), so an abandoned closure marks like any completion — accepted, see spec failure mode.
- **Receipt-time semantics (spec failure mode)**: both handlers evaluate `quickChat.isOpen` at handling time. `session.state_changed` and `session.turn.completed` arrive over separate event-bus subscriptions, and the gateway broadcasts both to every connected client (payloads carry no `workspace_id` — see `routeBroadcast`/`BroadcastToWorkspace` fallback), so their relative order is not guaranteed; a completion handled while the dialog is open never marks, one handled after it closed marks.

### Entry-point dots

- `apps/web/components/app-sidebar/app-sidebar-nav-item.tsx`: add `dot?: boolean` prop; when true, wrap the icon in a `relative` span with an absolutely positioned dot (`h-2 w-2 rounded-full bg-red-500 ring-2 ring-background`, top-right corner, `aria-hidden`, `data-testid="quick-chat-unseen-dot"`).
- `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`: Quick Chat nav item gets `dot={selectQuickChatHasUnseenIdle(state, workspaceId)}` (only rendered when collapsed).
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`: `RowActionButton` gains an optional `dot?: boolean` (same absolute dot over the `h-3.5 w-3.5` icon); the Quick Chat shortcut passes the selector result.
- `apps/web/components/kanban/kanban-header.tsx` (`TabletQuickActions`): wrap the `IconMessageCircle` in a relative span with the dot when the selector is true.
- `apps/web/components/kanban/kanban-header-mobile.tsx`: same for the mobile button.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` (`TaskSwitcherSurfaceHeader`, `mobile-sheet-quick-chat`): same for the mobile task-switcher sheet button.

No i18n impact: the dot is `aria-hidden` and carries no copy; labels/tooltips are unchanged.

---

## Tests

- **Marking via `session.state_changed`** — `apps/web/lib/ws/handlers/agent-session.test.ts` (extend existing describe blocks; store builder gains `quickChat` state):
  - table-driven over BOTH accepted active sources × every accepted settle target — for each S ∈ {STARTING, RUNNING} and T ∈ {IDLE, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED}: S→T marks when closed + tab exists, AND a replay of the same event (identical `updated_at`, row or ledger already settled) does NOT mark; plus the reverse (any settled → active) never marks.
  - dialog OPEN + RUNNING→IDLE → no marker (event-before-close ordering); the settle still records `lastSettledAtBySession` so a replay after close does not mark either.
  - no store row + payload `old_state: RUNNING` → IDLE → marker set (old_state fallback; boot hydration has no task-session row for quick chat tabs); with `old_state` absent → falls back to the store row.
  - non-quick-chat (kanban) session + closed + RUNNING→IDLE → no marker.
  - stale-hydration between replay: settle RUNNING→IDLE (mark) → `openQuickChat` clears → simulate `hydrateSession` regressing the task-session row back to RUNNING (`deepMerge`) → replay the ORIGINAL event verbatim (same `old_state`, same `updated_at`) → NO mark, because the ledger records the earlier settle (row regression is irrelevant).
  - ledger re-arm: settle at t1 (mark) → open clears → new settle at t2 > t1 → mark again; replaying t2 after another clear → no mark.
  - MISSING `updated_at` (fail-closed): settle without `updated_at`, closed + tab exists → NO mark and NO ledger record; after row regression + replay of the same event → still no mark — an ungenerationed settle must never raise a dot.
  - ledger bound: an entry within the 60-minute retention window still suppresses an event-before-tab replay; `pruneStaleSettledLedger` with a fixed `now` evicts entries older than the window; re-arm still works after eviction of the old entry; eviction also runs on close/remove/reconcile/hydration paths (young entries preserved there).
  - ledger monotonic max: settle t1 → settle t2 (t2 > t1) → `openQuickChat` clears → `deepMerge` regresses the row → a DELAYED t1 arrives → no mark AND the ledger stays t2 (never regressed); replaying t2 → still no mark.
  - ledger size cap: inserting beyond the 500-entry cap evicts the oldest entries by timestamp; entries within the cap are retained (event-before-tab replay still suppressed).
  - eviction-then-replay pin (accepted behavior, spec failure mode): a `state_changed` duplicate delivered AFTER its generation was age-evicted or cap-evicted marks like a new settle — pins the qualified re-delivery guarantee (suppression holds within the ledger retention only).
- **Abandoned closures (accepted behavior, spec failure mode)** — `turns.test.ts`: a `session.turn.completed` whose `completed_at` equals `started_at` (the backend's `isAbandonedTurnCompletion` shape) for a known quick-chat tab with the dialog closed marks like any completion — pins the intentional decision NOT to filter on the discriminator. The session did settle idle and the state_changed settle transition raises the same signal.
- **Re-arm (spec scenario 5)** — `agent-session.test.ts` or `turns.test.ts`: quick chat session marks, `openQuickChat` clears all, dialog closes, a second NEW `session.turn.completed` (or RUNNING→IDLE) arrives → marker is set again. Pins that clear-on-open does not permanently disarm a session.
- **Event-before-tab + re-delivery (spec failure modes)** — `agent-session.test.ts` / `turns.test.ts`: a settle or turn-completed event for a session absent from `quickChat.sessions` does not mark; for `state_changed` the settle generation is still RECORDED (membership-agnostic ledger), so after the tab is later added via `upsertQuickChatSessionFromEvent`/`addQuickChatSession`, replaying the original event does NOT mark; kanban sessions never render a marker (membership still fails) and their replay stays suppressed. For `turn.completed` this pins the pre-mutation `wasCompleted` snapshot: first delivery of a turn → marks (if closed + tab exists); duplicate delivery → no mark (turn already recorded); duplicate after `openQuickChat` cleared the marker → marker stays cleared.
- **Receipt-time ordering** — `agent-session.test.ts` / `turns.test.ts`: handler-level determinism for both orderings — event handled while dialog open → no mark; same event handled after dialog closed → mark (close-before-event). The separate `TurnCompleted`/`TaskSessionStateChanged` gateway subscriptions make cross-channel order undefined, so the handlers must be order-independent per event.
- **Marking via `session.turn.completed`** — `apps/web/lib/ws/handlers/turns.test.ts`:
  - quick chat session + dialog closed → marker set.
  - quick chat session + dialog open → no marker.
  - non-quick-chat session → no marker.
- **State actions + selector** — new `apps/web/lib/state/slices/ui/quick-chat-unseen.test.ts` and `quick-chat-unseen-selectors.test.ts`:
  - `markQuickChatUnseenIdle(sessionId, workspaceId)` sets in that workspace's map; `clearQuickChatUnseenIdle()` clears all workspaces; `clearQuickChatUnseenIdle(sessionId, workspaceId)` clears one.
  - `openQuickChat` clears all markers.
  - `setActiveQuickChatSession` clears that session's marker.
  - `closeQuickChatSession` clears the closing session's marker + ownership entry and runs the ledger age-prune.
  - generic session-delete regression: a quick chat session removed via `useSessionActions.remove`'s success path OR `sessions-dropdown.tsx`'s `handleDeleteSession` → tab, marker, and ownership entry all removed, ledger pruned; a delayed `session.state_changed`/`turn.completed` for the deleted session does NOT mark (membership fails); deleting a non-quick-chat session is a no-op.
  - `removeQuickChatSessionsForTask` clears markers of that task's sessions only (workspace-scoped).
  - `syncQuickChatSessions` reconcile drops that workspace's markers for sessions absent from the server list; two-workspace case: markers in A and B, sync A → only A pruned.
  - ownership-index regression: mark workspace-A session → hydrate B (A's marker AND `sessionOwnership` entry preserved) → `removeQuickChatSessionsForTask(A-task)` → A's marker removed and ownership entry dropped even though A's session row is absent from the current sessions list; deleting B's task does not touch A's markers; `closeQuickChatSession` / revision-guarded reconcile/resync drop ownership entries for the removed sessions; hydration PRESERVES A's ownership entry (non-destructive).
  - ownership-index insertion regressions: a tab inserted via the `openQuickChat` real-session push branch, a session inserted/adopted by `reconcileQuickChatSessions`, and a session loaded at INITIAL BOOT (`mergeQuickChatState`), each gain ownership entries at insertion; for all three, mark → `removeQuickChatSessionsForTask` → marker and ownership removed (cross-device resync included; the boot case: boot-loaded A session → mark → hydrate B (A row dropped) → delete A task → marker removed).
  - selector: true for a non-empty marker map of the workspace; false for other workspaces, for nullish/unknown workspace ids, and when nothing marked; stable empty fallback.
  - resync staleness regression: a deferred `listQuickChatSessions` response with an intervening WS task/session event (tab upserted + marker set, epoch bumped) is DISCARDED by `useQuickChatResync` — the tab and marker survive, reconcile never runs with the stale list; a response with no intervening mutation applies normally; a response arriving after reconnect without mutations still reconciles.
  - delete-vs-resync regression: a resync fetch started before a direct session delete (tab removed, epoch bumped) returns the old list afterward → discarded, the deleted tab is NOT re-added (reconcile never runs with the stale list).
  - tombstone regressions: after `removeQuickChatSession` / `removeQuickChatSessionsForTask` / the modal delete-tab flow, a late `task.created`/`task.updated` (or `upsertQuickChatSessionFromEvent`) for the tombstoned session id is IGNORED — no tab re-added, marker stays gone; a tombstone is cleared ONLY by a same-workspace authoritative list that POSITIVELY contains the session (then a genuinely new event applies); an authoritative list OMITTING the session does NOT clear it (delayed-old-event-after-authoritative-omission); a workspace-B list never clears a workspace-A tombstone; tombstones age-prune like the ledger (60 minutes, keyed on `tombstonedAt` — timestamp present and pruned).
  - reconcile-omission tombstone: a session omitted by a resync list is tombstoned BEFORE its ownership drops — a delayed `task.created`/`task.updated` after the omission is still blocked (deletion observed via list before the `task.deleted` event; cross-device `session.delete` covered). The omission set is the union of the workspace's session rows AND its `sessionOwnership` entries: A → hydrate B (A's rows dropped, ownership kept) → sync/hydrate A with an EMPTY list → A's ownership-only sessions are tombstoned before pruning, and a late task event for one is still blocked.
  - removal precedence: `removeQuickChatSession` on a row-absent-but-ownership-known session cleans marker + ownership + tombstones + bumps the revision (no stale-response re-add); the modal's non-setup no-taskId branch routes through the same cleanup (tombstone) instead of bare `closeQuickChatSession`.
  - hydration regressions: hydration is NON-DESTRUCTIVE — a deferred `StateHydrator` run with an SSR list captured before a local delete does NOT re-admit a deleted tab (tombstoned ids filtered) NOR clear its tombstone; an SSR list captured before a WS-upserted live tab does NOT drop or tombstone it (union merge preserves it; later lifecycle events still apply); an SSR list captured before a `task.updated` or the OPTIMISTIC RENAME does NOT overwrite the live tab's newer name/taskId (live wins on overlap); partial hydration payload without an active workspace id still merges sessions fail-safe. All pruning/tombstoning/positive-clearing is exercised on the revision-guarded resync.
  - task-session row-freshness regression: a deferred resync response with stale `task_sessions` (captured before a `session.state_changed` RUNNING→IDLE) applies ONLY the rows that are still fresh — the older RUNNING row for that session is SKIPPED (its live row has a newer `updated_at`), while other rows and the tab list apply; an unrelated workspace's `session.state_changed` traffic does NOT discard a valid resync response.
  - rename-vs-resync regression: an optimistic rename bumps the epoch before `persistQuickChatRename`; a resync in flight is discarded and cannot revert the tab to the old server name.
  - ownership-refresh regression: updating an EXISTING quick chat session row via `openQuickChat` (existing branch) or `addQuickChatSession` (existing branch) with a new/changed `taskId` refreshes its `sessionOwnership` entry — `removeQuickChatSessionsForTask` with the new taskId removes the marker; a row whose ownership lacks or has a stale taskId is covered.
- **Resync-driven pruning (non-destructive hydration)** — `apps/web/lib/state/hydration/hydrator.test.ts` + resync tests: hydration is a NON-DESTRUCTIVE union (fixtures compile with all fields; hydrating with a server session list that omits a marked session does NOT prune anything and does NOT resurrect re-upserted tabs); pruning of that workspace's markers/ownership for omitted sessions, omission-tombstoning (union identity set incl. ownership-only sessions), and positive tombstone clearing are exercised on the REVISION-GUARDED resync (`syncQuickChatSessions`/reconcile) — including the A/B cross-workspace cases (B-only resync preserves A's marker while pruning B's removed sessions; a later A resync behaves correctly with A retained and with A omitted).
- **Ephemeral reset (spec scenario 8)** — `mergeInitialState` unit test (or the hydration tests): a previous client state carrying markers and ledger entries, merged with a boot payload that omits them, yields an empty `unseenIdleByWorkspace` and `lastSettledAtBySession` — pins that a page reload (fresh store from default + boot payload) shows no dot.
- **Component rendering**:
  - `apps/web/components/app-sidebar/app-sidebar-primary-nav.test.tsx`: collapsed rail Quick Chat item renders the dot when the selector is true, not otherwise.
  - `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`: Quick Chat shortcut renders the dot when unseen, not otherwise.
  - `apps/web/components/kanban/kanban-header-mobile.test.tsx`: mobile button dot.
  - `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`: `mobile-sheet-quick-chat` button dot.
  - Tablet header (`kanban-header.tsx`) covered by the E2E tablet case.

Command form (from `apps/`): `pnpm --filter @kandev/web test -- --run <file1> <file2> …` per task, then the full touched-file set.

---

## E2E Tests

New specs under `apps/web/e2e/tests/chat/` using the existing helpers
(`quick-chat-helpers.ts`: `openQuickChatWithAgent`, `sendQuickChatMessage`) and
`helpers/causal-waits.ts` `watchWs`. All dot assertions MUST be scoped to the
entry under test — `getByTestId("quick-chat-unseen-dot")` matches every entry
simultaneously once a marker is active (strict-mode ambiguous), so each
assertion uses the named button's locator, e.g.
`shortcut.getByTestId("quick-chat-unseen-dot")`, and surfaces are asserted
individually:

- **Desktop (chromium)** — `quick-chat-idle-dot.spec.ts`: GIVEN no quick chat,
  THEN no dot on `sidebar-quick-chat-shortcut`. Capture `session_id` (and
  `task_id`) from the `POST /api/v1/workspaces/<id>/quick-chat` response
  (pattern: `entity-reference-composer.spec.ts` — the shared helpers return
  only the dialog). Call `watchWs(page)` before the first `goto`, then arm
  `const completed = ws.waitForEvent("session.turn.completed", { where: (p) => p.session_id === sessionId })`
  BEFORE sending `/slow 8s` (the wait must be armed before the event fires —
  `watchWs` buffers nothing, the gateway has no replay); send, close the
  dialog, await `completed`, THEN the dot is visible on the shortcut. Reopen →
  dot gone. Re-arm: arm a SECOND `waitForEvent` for the same session, send the
  second message WHILE the dialog is open, close the dialog while it is
  pending, await `completed`, THEN the dot reappears (spec scenario 5 — the
  composer only accepts input while the dialog is open).
- **Tablet (chromium)** — same lifecycle against `tablet-quick-chat-button`
  using the `tabletTestPage` fixture (900×900, `hasTouch: true`); a fine-pointer
  desktop context at ~1024 renders compact desktop, not the tablet header.
- **Mobile (mobile-chrome)** — `mobile-quick-chat-idle-dot.spec.ts` (the
  `mobile-chrome` project collects only `mobile-*.spec.ts`): same lifecycle
  against `mobile-quick-chat-button` (tap-driven, `quick-chat-close`), plus the
  sheet entry — navigation must happen BEFORE the marker exists (markers are
  ephemeral and lost on `page.goto`): seed a task
  (`apiClient.seedTask`, pattern `mobile-quick-chat-entry.spec.ts`), navigate
  to `/t/${taskId}` and wait for the session page, tap `mobile-sheet-quick-chat`
  (opens the Quick Chat setup dialog), select an agent, arm the waitForResponse
  for `POST /api/v1/workspaces/<id>/quick-chat` immediately BEFORE clicking
  `quick-chat-start` (that click issues the POST; the sheet tap itself does
  not), capture `session_id` from the response, send `/slow 8s` and close the
  dialog, await the completion, then reopen the sheet and assert the dot on
  `mobile-sheet-quick-chat`.

Run (single shell, from `apps/web`): `pnpm e2e:raw -g "quick chat idle dot" &&
pnpm e2e:raw --project=mobile-chrome -g "quick chat idle dot"`.

---

## Verification Results

Targeted state, hydration, handler, and component tests passed. Typecheck and lint passed.
Desktop/tablet Chromium and mobile-chrome idle-dot E2E runs passed.

---

## Implementation Waves

Sequential (state → handlers → UI → E2E; each builds on the previous).

- [x] [task-01-unseen-idle-state](task-01-unseen-idle-state.md)
- [x] [task-02-ws-marking-hooks](task-02-ws-marking-hooks.md)
- [x] [task-03-entry-point-dots](task-03-entry-point-dots.md)
- [x] [task-04-idle-dot-e2e](task-04-idle-dot-e2e.md)

## Open Questions

None.

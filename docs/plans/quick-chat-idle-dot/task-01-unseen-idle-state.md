---
id: "01-unseen-idle-state"
title: "Unseen-idle state in the UI slice"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-chat-idle-dot.md"
---

# Task 01: Unseen-idle state in the UI slice

Add the client-only unseen marker to the quick chat UI state, its actions, and
the per-workspace selector. Markers are keyed by workspace
(`unseenIdleByWorkspace[workspaceId][sessionId]`) so workspace ownership
survives hydration of a different workspace's session list.

- **Acceptance:**
  1. `QuickChatState.unseenIdleByWorkspace` defaults to `{}` and survives unrelated ui-slice writes; `lastSettledAtBySession` defaults to `{}` (the state_changed replay-guard ledger, written by task 02).
  2. Every structural `QuickChatState` site compiles and carries ALL FIVE fields with empty defaults: the boot payload in `apps/web/app/page.tsx`, the typed fixture builders in `quick-chat-sync.test.ts`, the full-state `draft.quickChat` assignments in `apps/web/lib/state/hydration/hydrator.test.ts`, AND `defaultUIState.quickChat` — `unseenIdleByWorkspace: {}`, `lastSettledAtBySession: {}`, `sessionOwnership: {}`, `syncRevisionByWorkspace: {}`, `tombstonedSessions: {}`.
  3. `markQuickChatUnseenIdle(sessionId, workspaceId)` sets `unseenIdleByWorkspace[workspaceId][sessionId] = true`; `clearQuickChatUnseenIdle()` clears all workspaces; `clearQuickChatUnseenIdle(sessionId, workspaceId)` clears one entry.
  4. Opening quick chat, activating a session, closing a session tab, removing a task's sessions, and server-reconciling the session list all clear the right markers (scenarios 4, 6, 7 in the spec).
  5. `selectQuickChatHasUnseenIdle(state, workspaceId: string | null | undefined)` is true exactly when that workspace's marker map is non-empty; false for nullish/unknown workspace ids; stable empty fallback (no fresh literals).
  6. Reconcile is workspace-scoped: markers in workspace A and B, then `syncQuickChatSessions(A, …)` — A's pruned markers clear, B's markers are preserved.
  7. `hydrateUI` is a NON-DESTRUCTIVE union: hydrated sessions absent from the live list are added (ownership indexed), overlapping sessions KEEP the live tab's fields (live wins — a stale SSR payload captured before a `task.updated` or the optimistic rename must not regress the newer live name/taskId), tombstoned ids are filtered; a marked session omitted by the hydrated list is NOT pruned by hydration and a later `upsertQuickChatSessionFromEvent` re-adding the tab is not suppressed by hydration; other workspaces' marker maps are untouched. Marker/ownership pruning for omitted sessions happens on the revision-guarded resync (verified for a later A resync both with A's session retained and with it omitted).
  8. Ephemeral reset (spec scenario 8): a `mergeInitialState`/hydration unit test proves that markers, ledger entries, ownership, revision counters, and tombstones from a previous client state are empty/default after merging a boot payload that omits them (fresh store from default + boot payload = no dot after reload).
  9. `sessionOwnership` index (sessionId → { taskId?, workspaceId }) is populated at EVERY insertion path: the initial boot path (`mergeQuickChatState` in `apps/web/lib/state/default-state.ts`), `addQuickChatSession`, `upsertQuickChatSessionFromEvent`, the `openQuickChat` real-session push branch, `reconcileQuickChatSessions` (inserted/adopted + refreshed survivors), and `hydrateUI`; pruned by `closeQuickChatSession`, `removeQuickChatSessionsForTask`, `removeQuickChatSession`, and revision-guarded reconcile/resync (NOT by hydration). `removeQuickChatSessionsForTask(taskId)` removes markers via the index (works even when the session row is absent from the current `sessions` list after cross-workspace hydration): mark A → hydrate B (A's marker and ownership preserved) → remove A's task → A's marker gone, B's markers untouched. Regressions for the boot-loaded, direct-POST-open, and reconcile-added insertion paths: mark → remove task → marker and ownership removed.
  10. `removeQuickChatSession(sessionId)` removes the tab (if present), marker, and ownership entry via the sessions-then-ownership lookup, tombstones non-setup ids, bumps the revision, and re-points the active tab / closes the modal like `closeQuickChatSession`; the generic session-delete flows — `useSessionActions.remove` (after `removeTaskSession`), `sessions-dropdown.tsx`'s `handleDeleteSession` (after the direct `session.delete`), AND the modal's non-setup delete-tab/no-taskId branches — call it, so deleting a quick chat session from ANY surface cleans tab + marker + ownership immediately (a delayed global state event for the deleted session never marks).
  11. The four new root actions are declared on `AppState` in `apps/web/lib/state/store.ts` (explicit quick-chat section), so `useAppStore` selections typecheck.
  12. Ownership pruning happens on the REVISION-GUARDED RESYNC, scoped per workspace: `reconcileQuickChatSessions` prunes `sessionOwnership` ONLY for entries whose `workspaceId === hydratedWorkspace` AND whose session is absent from the server list; other workspaces' ownership entries survive (a later task-deleted event for them can still clean their markers via the index). Hydration does not prune ownership.
  13. `syncRevisionByWorkspace` is incremented by every WS-driven session mutation; `useQuickChatResync` discards a stale list response (revision moved while the HTTP fetch was in flight) so a deferred response cannot clobber a WS-upserted tab/marker; regression covers deferred-response-with-intervening-event.
  14. `sessions-dropdown.tsx`'s `handleDeleteSession` calls `removeQuickChatSession` on success (before refresh) and NOT on request failure; covered by a focused `sessions-dropdown.test.ts` (or an extracted lifecycle hook test).
  15. `syncRevisionByWorkspace` is incremented by EVERY local server-backed tab mutation (WS upserts incl. both upsert branches, `addQuickChatSession` insert/existing-update, `openQuickChat` push/existing branches, `removeQuickChatSession`, `removeQuickChatSessionsForTask`, AND the optimistic rename flow — bumped BEFORE `persistQuickChatRename`); `useQuickChatResync` guards the returned `task_sessions` rows PER-ROW by `updated_at` (a returned row whose live store row is newer is skipped, so a deferred response never overwrites a newer `session.state_changed` row) and discards a stale tab list via the epoch — checked before EVERY response side effect including `setTaskSession`; unrelated-workspace state traffic never discards a valid response. A deferred list can neither clobber a WS-upserted tab, re-add a locally-deleted tab, revert an optimistic rename, nor overwrite a newer session row.
  16. `tombstonedSessions` (sessionId → { workspaceId, tombstonedAt }) is set by `removeQuickChatSession`, `removeQuickChatSessionsForTask`, and revision-guarded reconcile/resync OMISSION of a non-setup session, AND the Quick Chat dialog delete-tab flow (`use-quick-chat-modal.ts` `handleConfirmClose` calls `removeQuickChatSession` after the successful `deleteQuickChatTask`; setup-tab closes keep `closeQuickChatSession`); `syncQuickChatFromTaskEvent`/`upsertQuickChatSessionFromEvent` skip tombstoned ids. A tombstone clears ONLY on a same-workspace authoritative list that POSITIVELY contains the session; omission never clears (delayed old events after the list stay blocked — including omission-then-late-event) and other workspaces' lists never touch it; age-pruned like the ledger (60 minutes, keyed on `tombstonedAt`).
  17. Updating an EXISTING quick chat session row (`openQuickChat` / `addQuickChatSession` existing branches AND the central `upsertQuickChatSession` reducer's existing branch via `indexSessionOwnership`) with a new/changed `taskId` refreshes its `sessionOwnership` entry (ownership refresh is an invariant of every session-row update, not just insertion).
  18. Hydration is a NON-DESTRUCTIVE MERGE: the hydrated sessions list unions with the existing list (LIVE WINS on overlap — a stale SSR payload must not regress a newer live name/taskId; tombstoned ids filtered; ownership populated for newly added sessions); hydration NEVER prunes markers/ownership and NEVER tombstones omissions — a deferred `StateHydrator` run with an SSR list captured before a local delete does not re-admit the tab nor clear its tombstone, and an SSR list captured before a WS-upserted live tab does not drop or tombstone it. All pruning, omission-tombstoning, and positive tombstone clearing happen on the revision-guarded resync (union omission identity set: workspace session rows AND workspace-scoped `sessionOwnership` entries — A → hydrate B → resync A empty → ownership-only sessions tombstoned before pruning; late task events still blocked).
  19. The boot payload's `initialState.quickChat` explicitly resets ALL five ephemeral fields to empty defaults; every full-typed `QuickChatState` construction carries all five required fields (`unseenIdleByWorkspace`, `lastSettledAtBySession`, `sessionOwnership`, `syncRevisionByWorkspace`, `tombstonedSessions`).
  20. `removeQuickChatSession` lookup precedence is `quickChat.sessions` THEN `sessionOwnership` (ownership is authoritative even when the row is absent): a row-absent-but-ownership-known session is cleaned (marker + ownership + tombstone + revision bump); the modal's non-setup no-taskId branch routes through the same cleanup instead of bare `closeQuickChatSession`; a genuinely unknown id is a no-op.

- **Verification:**
  ```sh
  cd apps && pnpm --filter @kandev/web test -- --run \
    lib/state/slices/ui/quick-chat-unseen.test.ts \
    lib/state/slices/ui/quick-chat-unseen-selectors.test.ts \
    lib/state/slices/ui/quick-chat-session.test.ts \
    lib/state/slices/ui/quick-chat-sync.test.ts \
    lib/state/slices/ui/ui-slice.test.ts \
    lib/state/hydration/hydrator.test.ts \
    hooks/domains/session/use-session-actions.test.ts \
    components/task/sessions-dropdown.test.ts \
    hooks/use-quick-chat-resync.test.ts \
    && pnpm --filter @kandev/web run typecheck
  ```

- **Files likely touched:**
  - `apps/web/lib/state/slices/ui/types.ts` — `QuickChatState` fields (`unseenIdleByWorkspace: Record<string, Record<string, true>>`, `lastSettledAtBySession: Record<string, string>`, `sessionOwnership: Record<string, { taskId?: string; workspaceId: string }>`) + action signatures.
  - `apps/web/lib/state/slices/ui/ui-slice.ts` — default state (all three fields), `markQuickChatUnseenIdle(sessionId, workspaceId)`, `clearQuickChatUnseenIdle()` / `clearQuickChatUnseenIdle(sessionId, workspaceId)`, `removeQuickChatSession(sessionId)` (tab + marker + ownership + ledger prune + active-tab fallback), `recordQuickChatSettled(sessionId, updatedAt)` (ledger writer; prunes entries older than the 60-minute retention window via pure helper `pruneStaleSettledLedger(bySession, now)`), ownership population in `addQuickChatSession` / `upsertQuickChatSessionFromEvent` / the `openQuickChat` real-session push branch (`buildOpenQuickChatAction`), ownership pruning in `closeQuickChatSession`, hook into `setActiveQuickChatSession`.
  - `apps/web/hooks/domains/session/use-session-actions.ts` — `remove` success path calls `removeQuickChatSession(sessionId)` after `removeTaskSession(taskId, sessionId)`.
  - `apps/web/components/task/sessions-dropdown.tsx` — `handleDeleteSession` calls `removeQuickChatSession(sessionId)` after the successful direct `session.delete` (before `loadSessions(true)`).
  - `apps/web/hooks/use-quick-chat-resync.ts` — captures `syncRevisionByWorkspace[workspaceId]` before the HTTP fetch, discards a stale tab list when the epoch moved, and applies the returned `task_sessions` rows PER-ROW: a row whose live store row has a newer `updated_at` is skipped (checking BEFORE every side effect, including `setTaskSession`).

  - `apps/web/components/quick-chat/use-quick-chat-modal.ts` (or the rename helper) — the optimistic rename flow bumps `syncRevisionByWorkspace` before `persistQuickChatRename`.
  - `apps/web/components/quick-chat/use-quick-chat-modal.ts` — `handleConfirmClose` routes the NON-setup branch through `removeQuickChatSession(sessionId)` (tombstones) after the successful `deleteQuickChatTask`, and the non-setup no-taskId branch uses the same cleanup (via the ownership lookup); only setup-tab closes keep `closeQuickChatSession`.
  - `apps/web/lib/state/slices/ui/ui-slice.ts` — default state (all five fields) and THIN action wrappers only: the ownership-index, tombstone, revision-epoch, ledger-pruning, marker-cleanup, and `removeQuickChatSession` state transitions are EXTRACTED into `quick-chat-sync.ts` / a dedicated `quick-chat-lifecycle.ts` module (pure reducers + helpers) so `ui-slice.ts` stays ≤600 lines and every function ≤100 lines (AGENTS.md caps, enforced by `apps/web/eslint.config.mjs`).
  - `apps/web/lib/ws/handlers/quick-chat.ts` — `syncQuickChatFromTaskEvent` (and the `upsertQuickChatSessionFromEvent` path) skips tombstoned session ids.
  - `apps/web/lib/state/store.ts` — the four new root actions are added to AppState's EXPLICIT quick-chat action declarations (the section enumerates individual `UIA["…"]` entries; components/handlers select them from `useAppStore`).
  - `apps/web/hooks/domains/session/use-session-actions.test.ts` — the mocked `useAppStore` gains `removeQuickChatSession`; assert it is called after `removeTaskSession` on success and NOT on WS failure.
  - `apps/web/lib/state/slices/ui/quick-chat-sync.ts` — `reconcileQuickChatSessions` prunes that workspace's markers AND ownership entries for sessions no longer in the server list, and populates/refreshes ownership for inserted/adopted sessions and survivors; `removeQuickChatSessionsForTask` prunes markers and ownership via the `sessionOwnership` index (by taskId) before filtering sessions; new pure helpers `pruneUnseenIdleMarkers(byWorkspace, workspaceId, keepSessionIds)` (revision-guarded reconcile/remove only — hydrateUI never prunes) and `pruneStaleSettledLedger(bySession, now)` (shared with the ledger action).
  - `apps/web/lib/state/hydration/hydrator.ts` — `hydrateUI` performs a NON-DESTRUCTIVE union (new hydrated sessions added with ownership indexed; overlapping sessions keep the LIVE tab's fields — live wins on overlap; tombstoned ids filtered); no marker/ownership pruning, no omission tombstoning (those live in the revision-guarded resync).
  - `apps/web/lib/state/default-state.ts` — `mergeQuickChatState` populates `sessionOwnership` for boot-loaded sessions (initial boot bypasses `hydrateUI`).
  - `apps/web/app/page.tsx` — boot payload `initialState.quickChat` explicitly gains ALL FIVE fields with empty defaults: `unseenIdleByWorkspace: {}`, `lastSettledAtBySession: {}`, `sessionOwnership: {}`, `syncRevisionByWorkspace: {}`, `tombstonedSessions: {}`.
  - `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts` — typed `QuickChatState` builder gains `unseenIdleByWorkspace: {}`, `lastSettledAtBySession: {}`, `sessionOwnership: {}`, `syncRevisionByWorkspace: {}`, `tombstonedSessions: {}`, .
  - `apps/web/lib/state/hydration/hydrator.test.ts` — full-state `draft.quickChat` assignments gain `unseenIdleByWorkspace: {}`, `lastSettledAtBySession: {}`, `sessionOwnership: {}`, `syncRevisionByWorkspace: {}`, `tombstonedSessions: {}`, .
  - `apps/web/lib/state/slices/ui/quick-chat-unseen-selectors.ts` — new selector accepting `string | null | undefined` (returns false for nullish), mirroring the empty-fallback pattern of `lib/state/slices/office/selectors.ts`.
  - New tests: `quick-chat-unseen.test.ts`, `quick-chat-unseen-selectors.test.ts` next to the sources; resync-staleness coverage in a new `hooks/use-quick-chat-resync.test.ts` (deferred response + intervening WS tab mutation → tab list discarded; deferred response + intervening `session.state_changed` → the stale returned row for that session is SKIPPED via the per-row `updated_at` guard while fresh rows apply; unrelated-workspace state traffic does NOT discard the response; deferred response vs optimistic rename → discarded, tab keeps the new name); `components/task/sessions-dropdown.test.ts` gains the delete-cleanup cases (cleanup on success before refresh, none on failure); modal delete-tab regression (successful `deleteQuickChatTask` → `removeQuickChatSession` tombstones; local close followed by delayed/missing `task.deleted` then late `task.updated` → tab NOT resurrected); tombstone clearing semantics (positive-confirmation-only on the revision-guarded resync; omission retains; A-deleted/B-resync never clears A's tombstone; age-prune).

- **Dependencies:** None.
- **Parallelism:** sequential.

- **Inputs:**
  - Spec: What bullets 2–4, State machine table (workspace-keyed), Scenarios 4/6/7.
  - Plan: "State" section.
  - Existing patterns: `apps/web/lib/state/slices/ui/ui-slice.ts` (`buildQuickChatActions`, `buildOpenQuickChatAction`), `apps/web/lib/state/slices/ui/quick-chat-sync.ts` reducers, office selector empty-fallback constants.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks; update task + plan statuses in the same conversation.

## Results

Implemented workspace-scoped ephemeral markers, settlement deduplication, lifecycle cleanup,
hydration defaults, and selector coverage. Targeted state and hydration tests passed.

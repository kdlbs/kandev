---
id: "02-ws-marking-hooks"
title: "WS handler marking hooks"
status: complete
wave: 1
depends_on: ["01-unseen-idle-state"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-chat-idle-dot.md"
---

# Task 02: WS handler marking hooks

Mark quick chat sessions as unseen-idle in the two globally broadcast WS
handlers, guarded by dialog-open state and session membership.

- **Acceptance:**
  1. `session.state_changed` marks a quick chat session when the transition settles an active session (STARTING/RUNNING → IDLE/WAITING_FOR_INPUT/COMPLETED/FAILED/CANCELLED), the session id is in `quickChat.sessions`, and the dialog is closed; it never marks for non-settle transitions, replays, kanban sessions, or an open dialog. Previous state resolves from the store row FIRST, falling back to the payload's `old_state` only when no row exists (boot hydration drops `task_sessions`). The replay guard is the per-session settle-generation ledger `quickChat.lastSettledAtBySession`, recorded on EVERY settle transition BEFORE the quick-chat membership check (membership-agnostic — kanban sessions record too) and regardless of `isOpen`; a settle marks only when its `updated_at` is newer than the ledger entry. The ledger is a MONOTONIC MAX per session: `updatedAt <= recordedAt` is suppressed and never overwrites, so an out-of-order delayed older settle cannot regress the ledger (otherwise a later replay of the newer event would falsely mark after clear). Recording-before-membership is what makes event-before-tab replay-safe: a settle that arrives before its tab exists records the generation, so adding the tab later and replaying the original event cannot mark. A settle whose payload lacks `updated_at` has no stable generation (the backend fallback persistence path can publish without it) and FAILS CLOSED: never marks and never records. The ledger is age-bounded — `pruneStaleSettledLedger(bySession, now)` (60-minute window) runs at `recordQuickChatSettled`, at close/remove/reconcile, and opportunistically during hydration — age-eviction only, entries younger than the window are never dropped — so a long-lived client cannot accumulate a key per settled kanban session. The ledger is needed because `hydrateSession`'s unconditional `deepMerge(draft.taskSessions, …)` can regress a settled row back to RUNNING.
  2. `session.turn.completed` captures `wasCompleted` (turn id already recorded completed in `turns.bySession[sessionId]`) BEFORE `addTurn`/`completeTurn` and marks only when the quick chat session's tab exists, the dialog is closed, and `!wasCompleted`; never marks kanban sessions, an open dialog, or re-delivered completions of an already-completed turn.
  3. Both stay idempotent (repeated events do not duplicate state). All `session.turn.completed` events are treated uniformly: the abandonment discriminator EXISTS in the payload (`completed_at === started_at`, per `isAbandonedTurnCompletion`) but is INTENTIONALLY ignored — an abandoned closure marks like any completion (accepted, spec failure mode); no filtering by `had_output`, timestamps, or other fields.
  4. Re-arm (spec scenario 5): after `openQuickChat` clears all markers and the dialog closes, a second NEW settle/turn-completed event for the same session marks it again.
  5. Event-before-tab + re-delivery (spec failure modes): a settle or turn-completed event for a session absent from `quickChat.sessions` never marks it — including when the tab is added afterwards via `upsertQuickChatSessionFromEvent`/`addQuickChatSession`, when the same turn-completed is delivered again (pre-mutation `wasCompleted` snapshot: the first delivery recorded the turn), and when a duplicate arrives after `openQuickChat` cleared the marker (a replayed completion must not re-arm it).
  6. Receipt-time semantics (spec failure mode): the handlers evaluate `quickChat.isOpen` at handling time; the two broadcast channels are separate event-bus subscriptions (both fanned out to all connected clients — the payloads carry no `workspace_id`), so each handler must be order-independent — event handled while open → no mark; same event handled after close → mark.

- **Verification:**
  ```sh
  cd apps && pnpm --filter @kandev/web test -- --run \
    lib/ws/handlers/agent-session.test.ts \
    lib/ws/handlers/turns.test.ts
  ```

- **Files likely touched:**
  - `apps/web/lib/ws/handlers/agent-session.ts` — `maybeMarkQuickChatUnseenIdle(store, sessionId, previousState, newState)` + call site in `session.state_changed`; local STARTING/RUNNING and IDLE/WAITING_FOR_INPUT/COMPLETED/FAILED/CANCELLED sets (do NOT import `isTurnSettleTransition` from `hooks/domains/session/use-session-messages.ts` — keeps `lib/ws` free of a hooks import edge; the sets match its definitions).
  - `apps/web/lib/ws/handlers/turns.ts` — quick-chat + closed-dialog guard in `session.turn.completed` (after `completeTurn`).
  - `apps/web/lib/ws/handlers/agent-session.test.ts`, `apps/web/lib/ws/handlers/turns.test.ts` — the production hooks call ROOT store actions (`markQuickChatUnseenIdle(sessionId, workspaceId)`, `recordQuickChatSettled(sessionId, updatedAt)`, `clearQuickChatUnseenIdle`) plus `quickChat` state, so the existing builders MUST be extended with the exact wiring: `turns.test.ts`'s `makeStore()` (currently `SessionSlice` + `quickChat: { sessions: [] }`) and `agent-session.test.ts`'s `makeStore()` mock (currently no quickChat state at all) each gain `quickChat: { isOpen, sessions, unseenIdleByWorkspace, lastSettledAtBySession }` and the three root actions — either wired from the real ui-slice (`createAppStore`) or as `vi.fn()` mocks that mutate a fixture `quickChat` object (specify which; the mock approach matches the existing convention and lets tests assert both the marker map and the ledger). Include: the settle-transition matrix table-driven over BOTH active sources × every accepted target — for each S ∈ {STARTING, RUNNING} and T ∈ {IDLE, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED}: S→T marks when closed + tab exists, AND a replay of the same event (identical `updated_at`) does NOT mark; reverse (settled → active) never marks; re-arm (mark → `openQuickChat` → close → second NEW event with newer `updated_at` → marked again; replaying that second event after another clear → no mark); event-before-tab (settle with no tab → ledger recorded but no mark → `upsertQuickChatSessionFromEvent` adds the tab → replay the original event → still no mark; kanban session settle → ledger recorded, never a marker); stale-hydration between replay (settle → open clears → `deepMerge` regresses the row to RUNNING → replay the original event verbatim → no mark, ledger suppresses); turn-completed re-delivery with the pre-mutation `wasCompleted` snapshot (first delivery of a turn → marks when closed + tab exists; duplicate → no mark; duplicate after `openQuickChat` cleared → marker stays cleared); state_changed old_state fallback (known tab, no store row, `old_state: RUNNING` → IDLE → marks; with `old_state` absent → falls back to the store row); missing `updated_at` fail-closed (settle without `updated_at`, closed + tab exists → no mark, no ledger record; row regression + replay → still no mark); ledger bound (entry within the 60-minute window suppresses an event-before-tab replay; `pruneStaleSettledLedger(bySession, fixedNow)` evicts entries older than the window; eviction also runs on close/remove/reconcile/hydration paths with young entries preserved; re-arm still works after eviction); ledger monotonic max (settle t1 → settle t2 → open clears → row regressed → delayed t1 → no mark AND ledger stays t2; replay t2 → still no mark); ledger size cap (inserting beyond the 500-entry cap evicts the oldest entries by timestamp; entries within the cap retained); eviction-then-replay pin (a duplicate delivered after its generation was age-evicted or cap-evicted marks like a new settle — accepted, spec-qualified re-delivery guarantee); settle while open records the ledger but marks nothing, and its replay after close does not mark; receipt-time orderings (event while open → no mark; same event after close → mark); abandoned closure (turn.completed with `completed_at === started_at` for a known tab + closed dialog → marks — pins the intentional non-filtering).

- **Dependencies:** task 01 (actions `markQuickChatUnseenIdle` must exist).
- **Parallelism:** sequential.

- **Inputs:**
  - Spec: What bullet 2, Failure modes (WS disconnect, event-before-tab).
  - Plan: "Marking hooks" section.
  - Existing code: `apps/web/lib/ws/handlers/agent-session.ts` (`registerTaskSessionHandlers` shape, `isStaleSessionStateEvent` guard, `existingSession?.state` before upsert), `apps/web/lib/ws/handlers/turns.ts` (`session.turn.completed` flow), `apps/web/hooks/domains/session/use-session-messages.ts` (`ACTIVE_SESSION_STATES` / `SETTLED_SESSION_STATES` values to mirror).

- **Output contract:** summary, files changed, tests run with counts, blockers, risks; update task + plan statuses in the same conversation.

## Results

Implemented state-transition and turn-completion marker hooks. Handler tests passed.

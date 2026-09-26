---
id: "08-conditional-pin-sync"
title: "Conditional pin sync"
status: done
wave: 5
depends_on: ["07-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 08: Conditional pin sync

Gate the automatic Todos pin on the "Only pin when todo list is not empty"
preference: when it is on, the sync hook must not add the `todos` panel while
the active session's todo list is empty, using the same two-source todo
fallback the panel content uses (live `sessionTodos.bySessionId` first, then
persisted `todo` messages via `buildTodoItems`). The sub-option never removes
an existing panel and never affects manual adds.

- **Acceptance:**
  1. `resolveConditionalTodoPanelAction` returns `"none"` (not `"add"`) when
     `onlyPinWhenNotEmpty` is true and `todoListNotEmpty` is false and the
     panel is absent; `"add"` when the list is non-empty; and never
     `"remove"` as a result of the sub-option (removal stays gated solely on
     `showTodoListPanel`).
  2. `useSyncTodoPanel` subscribes to the active session's todo state (live
     slice + persisted messages) and re-runs the sync when it changes, so the
     panel appears as soon as todo entries arrive (live WS or hydrated
     history).
  3. Unit tests go red before implementation and green after;
     `pnpm run typecheck` passes.
- **Verification:** `cd apps/web && pnpm exec vitest run components/task/dockview-todo-panel-sync.test.ts && pnpm run typecheck`
  (fresh worktree bootstrap first if needed: `cd apps && pnpm install --frozen-lockfile`).
- **Files likely touched:**
  - `apps/web/components/task/dockview-todo-panel-sync.ts`
  - `apps/web/components/task/dockview-todo-panel-sync.test.ts`
- **Dependencies:** Task 07 (store field `showTodoListPanelOnlyWhenNotEmpty`
  exists in `UserSettingsState`).
- **Parallelism:** `sequential`.
- **Inputs:** Spec What / Scenarios sections
  (`docs/specs/ui/requirements/agent-todo-list-panel.md`); plan's "Conditional-pin sync"
  section; `apps/web/components/task/todos-panel-content.tsx` as the exact
  two-source fallback reference; `buildTodoItems`
  (`apps/web/hooks/use-processed-messages.ts`).

Implementation notes:

- `resolveConditionalTodoPanelAction` gains `onlyPinWhenNotEmpty` and
  `todoListNotEmpty` params; insert the new guard after the
  restoring/maximized guards and only in the add path:
  `if (params.onlyPinWhenNotEmpty && !params.todoListNotEmpty) return "none";`.
- `syncConditionalTodoPanel` options gain the same two params and pass them
  through.
- In `useSyncTodoPanel`, derive the active session's todo state:
  - live: `useAppStore((s) => (sessionId ? s.sessionTodos.bySessionId[sessionId] : undefined))`
  - persisted: `useAppStore((s) => (sessionId ? s.messages.bySession[sessionId] : EMPTY))`
    then `useMemo(() => buildTodoItems(messages), [messages])`
  - `todoListNotEmpty = (live?.length ?? 0) > 0 || messageTodos.length > 0`
  Read `live.userSettings.showTodoListPanelOnlyWhenNotEmpty` inside the
  effect (alongside the existing `showTodoListPanel` read), pass both new
  values into `syncConditionalTodoPanel`, and add `todoListNotEmpty` +
  `onlyPinWhenNotEmpty` to the effect dependency array.
- Do not mount `useSessionMessages` here — the desktop workbench's chat
  panel always fetches the session's messages; subscribing to the store
  slice is enough and avoids duplicating fetch/subscription side effects.
- Test additions: extend the `it.each` table with
  `{ showTodoListPanel: true, onlyPinWhenNotEmpty: true, todoListNotEmpty: false, panelExists: false }`
  → `"none"` and the non-empty variant → `"add"`; add a
  `syncConditionalTodoPanel` case asserting `addPanel` is not called with the
  sub-option on and an empty list; assert an existing panel is untouched
  (no `close`) in that case.

## Results

Red first: extended `resolveConditionalTodoPanelAction`'s `it.each` table
with 5 sub-option rows (empty list + sub on → `"none"`; non-empty → `"add"`;
existing panel untouched; sub off → unchanged `"add"`; master off still
`"remove"`) and added two `syncConditionalTodoPanel` cases (no addPanel with
empty list + sub on; addPanel with non-empty list). `pnpm exec vitest run
components/task/dockview-todo-panel-sync.test.ts` → 2 failed (the empty-list
suppression rows returned `"add"`/called `addPanel`).

Implemented in `components/task/dockview-todo-panel-sync.ts`:
- `resolveConditionalTodoPanelAction` gains `onlyPinWhenNotEmpty` +
  `todoListNotEmpty` params; new guard `if (params.onlyPinWhenNotEmpty &&
  !params.todoListNotEmpty) return "none";` after the restoring/maximized
  guards, in the add path only — never affects `"remove"` or an existing
  panel.
- `syncConditionalTodoPanel` options gain both params and pass them through.
- `useSyncTodoPanel` subscribes to the active session's todo state
  (`sessionTodos.bySessionId[sessionId]` live + `messages.bySession[sessionId]`
  persisted via `buildTodoItems`), computes
  `todoListNotEmpty = liveTodos.length > 0 || buildTodoItems(messages).length > 0`,
  reads `live.userSettings.showTodoListPanelOnlyWhenNotEmpty`, passes both
  into `syncConditionalTodoPanel`, and adds `onlyPinWhenNotEmpty` +
  `todoListNotEmpty` to the effect deps so the panel appears as soon as todo
  entries arrive. No `useSessionMessages` mount (chat panel fetches messages).
- Test `DEFAULT_OPTIONS` widened with `onlyPinWhenNotEmpty: false,
  todoListNotEmpty: false` so existing `syncConditionalTodoPanel` calls stay
  assignable.

Commands:
- `cd apps/web && pnpm exec vitest run components/task/dockview-todo-panel-sync.test.ts`
  → 23 passed (2 red → green).
- `cd apps/web && pnpm run typecheck` (with
  `NODE_OPTIONS=--max-old-space-size=4096`) → clean.

### Reviewer-feedback refinement (adversarial review, APPROVE qualified)

The DeepSeek V4 Pro adversarial review (sub-task
`4c855132-fc54-4e8c-924e-3a45bcda460b`) found 3 items; two were acted on:

- **N1 (minor, suspected)** — the inner RAF callback captured the
  render-time `todoListNotEmpty` memo; a WS todo event landing between render
  and rAF dispatch (~16ms) could make the pin decision one frame stale.
  Fixed by extracting `todoListNotEmptyForSession(liveTodos, messages)` (the
  panel's exact two-source fallback, exported so the contract is testable)
  and recomputing it inside the callback from the dispatch-time
  `appStore.getState()` snapshot. The render-time memo stays as the effect's
  change signal in the dep array. Added 4 unit tests pinning the helper
  (empty, live-wins, persisted-fallback, empty-live-array-falls-through).
- **N3 (nit, confirmed)** — dropped the now-unnecessary
  `as unknown as Partial<BackendMessageMap[...]>` cast in
  `lib/ws/handlers/users.test.ts` (the field is statically typed).
- **N2 (nit, confirmed)** — card-level `data-settings-dirty` on both toggles;
  fixed in the round-2 pass as F1 (per-field dirty flags, see Task 07
  results).
- **F2 (minor, suspected, round 2)** — the render-time `todoListNotEmpty`
  memo and the RAF-dispatch recompute each called `buildTodoItems` (two O(n)
  scans per effect cycle). Fixed by removing the memo entirely: the effect's
  change signal is now the raw `liveTodos`/`messages` slice identities in the
  dependency array (they only change when the underlying todo/message data
  actually changes), so the predicate is computed exactly once per sync — in
  the dispatch callback, from the dispatch-time snapshot. `useMemo` import
  dropped.

Post-fix verification: sync+WS unit tests 49 passed; `pnpm run typecheck`
clean; `make fmt` + `make lint` clean; E2E
`todo-list-panel.spec.ts` 8/8 passed (web rebuilt). Committed as follow-up
`fix:` on the feature branch.

### Round-4 reviewer feedback (OMP GPT Luna) and disposition

- **Finding 1 (major, confirmed by reviewer): "branch contains unrelated
  changes"** — review-base artifact, not a defect: the reviewer diffed
  against the stale local `main` ref (c0f5750b); against `origin/main` the
  branch is exactly the four feature commits / 31 files
  (`git diff origin/main...HEAD --stat`). No action; round-5 brief instructs
  the reviewer to use `origin/main` as the base.
- **Finding 2 (major, confirmed): malformed persisted todo metadata crashes
  `buildTodoItems`** — `metadata.todos` as a non-array (object/primitive)
  throws `TypeError: ...?.todos?.map is not a function`, and `[null]`
  elements throw on `item.text`. The round-1 fix made this a sync hot-path
  call, so a malformed message would throw inside the RAF callback and the
  sync (including panel removal with the master pref off) would silently
  fail; the same throw also hits the panel content and chat processing, and
  violates the spec's "malformed todo message falls back to an empty state".
  Fixed by making `buildTodoItems` total: runtime `typeof`/`in` narrowing
  instead of inline casts, `Array.isArray(todos)` guard, and a type-guard
  filter dropping null/primitive entries. Added malformed-shape unit tests
  (non-array object, primitive, `[null, 42, valid]`, primitive metadata,
  missing metadata) at both the `buildTodoItems` and
  `todoListNotEmptyForSession` levels (2 red → green). Committed as follow-up
  `fix:`.

### Round-5 reviewer feedback (OMP GPT Luna) and disposition — all five fixed

1. **F1 (major): sessionless tasks bypass visibility sync** — `useSyncTodoPanel`
   early-returned on `!sessionId`, so a `todos` tab materialized from a saved
   layout on a task without a session could never be removed by the master
   preference. Fixed: the guard now requires only `taskId`/`workspaceId`/`hasApi`;
   a null session is treated as an empty todo list (never enables an add) while
   still allowing the removal path.
2. **F2 (major): malformed todo metadata still crashed chat rendering** —
   `TodoMessage`'s own `normalizeTodos` (non-array `todos`, `[null]` entries,
   `previous_todo_snapshots` non-array) threw, bypassing the round-4
   `buildTodoItems` hardening. Fixed: `normalizeTodos` is now total (unknown
   input, `Array.isArray`, type-guard filter dropping null/primitive/empty-text
   entries), `parseTodos`/`parseSnapshots` narrow via `typeof`/`in` instead of
   inline casts, and a new `todo-message.test.tsx` pins all malformed shapes
   (5 tests, red → green).
3. **F3 (major): delayed PATCH could clobber a newer WS settings update** — the
   save callback unconditionally echoed the submission into the store; a WS
   push landing mid-flight would be reverted. Fixed: snapshot the store before
   the PATCH and only echo when it has not drifted; otherwise the server's own
   settings broadcast owns convergence. New component test with a deferred
   save + mid-flight store update (red → green).
4. **F4 (minor): empty-text entries counted as non-empty** — the round-4
   type-guard filter (`typeof item.text === "string"`) kept `{ text: "" }`,
   which the pre-fix code dropped; a blank entry could trigger pinning. Fixed:
   require `item.text.length > 0` in `buildTodoItems` (and the mirrored
   `TodoMessage` filter); regression tests at both levels.
5. **F5 (minor): unbounded transcript scan when the sub-option is off** — the
   sync computed `buildTodoItems` (O(n) copy+scan) on every message-array
   update even when `onlyPinWhenNotEmpty` is false. Fixed: the predicate is
   only computed when the sub-option is on (and a session exists); otherwise
   `false` is passed (the decision ignores it).

Verification: 846 tests across the touched areas (chat, settings, sync,
processed-messages, SSR/WS) pass; `pnpm run typecheck`, `make fmt`, `make
lint` clean; E2E `todo-list-panel.spec.ts` 8/8 (web rebuilt). Committed as
follow-up `fix:`.

### Round-6 reviewer feedback (OMP GPT Luna) and disposition — all three fixed

1. **G1 (major): malformed snapshot entries still crashed TodoMessage** —
   `parseSnapshots` validated array-ness but not entries, so
   `previous_todo_snapshots: [null]` threw in `SnapshotHistory` on expand.
   Fixed: entries are filtered to non-null objects; malformed `snapshot.todos`
   was already neutralized by the unknown-safe `normalizeTodos`. New tests
   click the "Earlier updates" toggle with `[null, 42, valid]` and malformed
   snapshot `todos` (red → green).
2. **G2 (major): sessionless tasks could still receive an automatic Todos tab**
   — the round-5 fix enabled removal for null sessions but the add path still
   fired when the sub-option was off (the resolver ignored the empty-list
   predicate). Fixed: `resolveConditionalTodoPanelAction` gained a required
   `hasActiveSession` param gating the add path; the hook passes
   `sessionId !== null`. Removal for sessionless tasks is preserved. New
   decision-table rows (never-add, sessionless-remove, sessionless-keeps) and
   two `syncConditionalTodoPanel` tests (red → green).
3. **G3 (major): delayed save could leave the UI falsely clean and stale** —
   the round-5 drift guard skipped the store echo but still ran
   `setSaved(submitted)`, overwriting the newer WS hydration baseline so
   `isDirty` flipped false against a stale submission. Fixed: on drift the
   save returns without touching `saved`, keeping the draft dirty against the
   newer baseline so the user can reconcile; the F3 test now asserts the
   dirty flag stays on (red → green).

Verification: 853 tests across the touched areas pass; `pnpm run typecheck`,
`make fmt`, `make lint` clean; E2E `todo-list-panel.spec.ts` 8/8 (web
rebuilt). Committed as follow-up `fix:`.

### Round-9 reviewer feedback (OMP GPT Luna) and disposition

1. **J1 (major): full transcript scan on every message update while the
   sub-option is on** — analyzed, NOT fixed by the suggested incremental
   tracker: a prototype tracker that only inspected the newest message after
   a todo-free scan was demonstrated UNSOUND — when the first sync runs
   before the messages fetch completes (`[]`), then persisted history with a
   mid-array `todo` message and a plain final message lands, the tracker
   returns false forever and the auto-pin for reopened completed tasks (a
   passing E2E scenario) breaks. A sound incremental version requires the
   messages store to expose todo-relevant derived state, which it does not
   (out of scope). The current behavior is already bounded: the predicate is
   computed only when the opt-in sub-option is on, and the live
   `sessionTodos` slice short-circuits the message scan while the session
   has live entries. Documented in the hook comment; the E2E
   persisted-history pin continues to pass.
2. **J2 (minor): hook tests did not model rAF cancellation or frame
   boundaries** — FIXED: the hook-test harness now uses stable frame IDs
   backed by a pending-callback map, `cancelAnimationFrame` deletes the
   pending callback, and frames flush one at a time (outer schedules inner
   for the next flush). Added a between-frames test: a preference change
   after the outer frame cancels the pending inner, and exactly zero sync
   side effects fire for the stale state.

Verification: 862 tests across the touched areas pass; `pnpm run typecheck`,
`make fmt`, `make lint` clean; E2E `todo-list-panel.spec.ts` 8/8 (web
rebuilt). Committed as follow-up `fix:`/`test:`.

Blockers/risks: none.

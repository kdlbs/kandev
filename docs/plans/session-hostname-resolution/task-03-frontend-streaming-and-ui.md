---
id: session-hostname-resolution-03
title: Frontend hostname streaming and security-page UI
status: pending
wave: 3
depends_on: [session-hostname-resolution-01, session-hostname-resolution-02]
plan: docs/plans/session-hostname-resolution/plan.md
spec: docs/specs/session-hostname-resolution/spec.md
---

# Frontend hostname streaming and security-page UI

## Inputs

The [spec](../../specs/session-hostname-resolution/spec.md) (Frontend,
Streaming sections), Task 01's `userSettings.resolveSessionHostnames`, Task
02's sessions `hostname` field and `auth.session.hostname.resolved` WS
action. Existing patterns: `components/settings/agent-generated-task-title-settings.tsx`
(Switch card), `lib/ws/handlers/users.ts` (store handler),
`lib/state/slices/auth` (auth slice), `components/settings/account/security-settings.tsx`
(SessionsCard).

## TDD sequence

1. Add failing tests: auth slice `setSessionHostname` upsert AND ordering —
   the shared entry comparator rejects a delayed older event (newer event
   then delayed older event keeps the newer), durable beats null, ties to
   non-empty, identical entries keep the existing one; and
   `setAuthState` semantics — **same identity preserves `sessionHostnames`,
   a different identity (or null/logout) clears it AND increments
   `sessionHostnamesEpoch`**, and
   **`clearAuthenticated` (the 401 path) also clears it and bumps the
   epoch**; the session-hostnames WS handler **ignores events whose store
   epoch differs from its registration epoch** (stale-identity regression:
   deliver an old-user notification after identity change, map stays empty);
   WS handler
   `auth.session.hostname.resolved` updates the store map; SessionsCard
   component tests (column hidden while off, shown while on, `N/A` for empty
   values — including the cached-no-answer case that renders `N/A` with no
   spinner while fresh, and updates when a post-`recentResolveWindow` retry
   yields a valid answer, streamed value overrides response value, **newer
   `resolved_at` wins: a retained map entry is replaced by a newer
   sessions-response entry after a WS gap (event → disconnect → list
   refresh regression), a fresh streamed event beats an in-flight stale
   response, and a transient/unknown empty never masks a durable response
   value**, toggle PATCHes the setting and flips the column); `useSessionsList` refetches on
   window focus and on the `false -> true` setting transition (same-tab
   toggle AND cross-tab `user.settings.updated` push both trigger exactly one
   reload, with trailing coalescing — a trigger during an in-flight fetch
   sets a pending flag and runs exactly one trailing reload after it
   settles, and generation-guarded responses discard stale results; the
   toggle PATCH is awaited before the store flip, so the transition
   refetch always runs against a persisted-true gate — deferred-PATCH
   test); the WS client lifecycle test proves an identity change disposes
   and recreates the client with a fresh handler registration at the new
   epoch (reactive identity selector, not a `store.getState()` read); store
   composition: `createAppStore({
   sessionHostnames })` keeps the supplied map (default-state + overrides
   paths). **Router integration:** obtain the handlers from
   `registerWsHandlers(store)` and apply a synthetic
   `auth.session.hostname.resolved` notification, asserting the store map
   updates — a missing `...registerSessionHostnamesHandlers(store)` entry
   in `router.ts` otherwise passes every direct-handler test and the
   `N/A`-tolerant E2E while the streaming path is dead.
2. Implement the store/handler/type changes, then the UI, keeping tests
   green.
3. Run `pnpm run i18n:pseudo` after adding locale keys and re-run
   `i18n:ratchet`.

## Implementation

- `apps/web/lib/api/domains/auth-api.ts`: `AuthSession` gains
  `hostname: string` and `hostname_resolved_at: string | null`.
- `apps/web/lib/types/backend.ts`: add
  `"auth.session.hostname.resolved": BackendMessage<"auth.session.hostname.resolved", SessionHostnameResolvedPayload>`
  with
  `SessionHostnameResolvedPayload = { ip: string; hostname: string; resolved_at: string | null }`.
- `apps/web/lib/state/slices/auth/types.ts` + `auth-slice.ts`:
  `sessionHostnames: Record<string, SessionHostnameEntry>` (default `{}`)
  with `SessionHostnameEntry = { hostname: string; resolvedAt: string |
  null }`, `sessionHostnamesEpoch: number` (default `0`), and
  `setSessionHostname(ip, hostname, resolvedAt)`.
  **Shape rule (exact path, no nesting):** `AuthSliceState` gains
  `sessionHostnames` and `sessionHostnamesEpoch` as **sibling keys of
  `auth`** — the slice creator spreads `defaultAuthState` into the ROOT
  store, so these keys become top-level `AppState` fields; they must NEVER
  be placed inside the `auth` object (nesting under `auth` is erased by
  `setAuthState`, which replaces `draft.auth` wholesale). Declare both
  explicitly in `apps/web/lib/state/store.ts`'s `AppState` type (alongside
  the existing `auth: (typeof defaultAuthState)["auth"]`), in
  `apps/web/lib/state/default-state.ts` (`defaultState`), and in
  `apps/web/lib/state/store-overrides.ts` (`buildStateOverrides`). The auth
  actions mutate these root fields directly through the immer draft:
  `setAuthState` clears the map + bumps the epoch on identity change (and
  preserves both on same-identity refresh), `clearAuthenticated` clears +
  bumps, and `setSessionHostname` applies the entry comparator (below).
  Selectors and WS handlers read/write
  `state.sessionHostnames` / `state.sessionHostnamesEpoch` /
  `state.setSessionHostname(...)` — never `auth.sessionHostnames`.
  **Entry comparator (single source of truth):** `setSessionHostname` is
  NOT a blind upsert — it applies one shared `compareEntries(a, b)`
  (newer `resolvedAt` wins; a durable entry (non-null `resolvedAt`) beats a
  transient one (null); ties break to the non-empty hostname; both empty or
  identical → keep the existing entry) and only replaces the map entry when
  the incoming event wins. This fences **event-vs-event reordering**: a
  delayed older event can never overwrite a newer map entry when delivery
  is reordered, and no HTTP snapshot is required to fix the row. Cell
  resolution reuses the same comparator against the session's response
  entry. **Compare `resolvedAt` strings lexicographically** (the fixed-width
  UTC
  format is chronologically ordered as a string); do NOT use
  `Date.parse`, which truncates sub-millisecond digits and would tie two
  changes within the same millisecond. Add tests: newer event then a
  delayed older event (map keeps the newer), durable-beats-null, tie to
  non-empty, and a sub-millisecond ordering test.
  **Shape rule (exact path):** `sessionHostnames` is a **top-level sibling
  of `auth`** in the auth slice state and in `AppState`
  (`{ auth: {...}, sessionHostnames: {...} }`) — never nested under `auth`
  and never addressed as `auth.sessionHostnames`. `setAuthState` replaces
  `draft.auth` wholesale, so a nested map would be erased by every
  login/`me` refresh. Update `apps/web/lib/state/store.ts` (`AppState`
  declares `auth: (typeof defaultAuthState)["auth"]` explicitly) with the
  new slice state shape; `defaultAuthState` must include
  `sessionHostnames: {}`. **State composition:** also add the field to
  `apps/web/lib/state/default-state.ts` (`defaultState`) and
  `apps/web/lib/state/store-overrides.ts` (`buildStateOverrides`) — store
  construction spreads `defaultState`, then slices, then overrides, so a
  caller-supplied `sessionHostnames` in `createAppStore({...})` would
  otherwise be overwritten by the slice default. Selectors and WS handlers
  read/write `state.sessionHostnames` / `state.setSessionHostname(...)`.
  **Identity policy (decided):** the map survives disable/enable and
  same-identity refreshes; `setAuthState` **clears it when the authenticated
  identity changes** (the new `auth.user` id differs, or the user becomes
  null on logout), and `clearAuthenticated` (invoked on every 401 by
  `apps/web/src/main.tsx`) **also clears it** — both identity boundaries
  must reset the map so a session-expiry or logout can never leave one
  user's session IPs seeded for the next user.
  **Identity fence for delayed WS events (required):** the gateway strips
  `user_id` from `auth.session.hostname.resolved`, so a frame queued from
  the prior user is indistinguishable from a fresh one once delivered.
  Add a `sessionHostnamesEpoch: number` to the auth slice, **incremented
  whenever the map is cleared on an identity transition**; the
  `registerSessionHostnamesHandlers(store)` registration captures the epoch
  at registration time and **ignores any event when the store's current
  epoch differs** (a stale handler generation can never repopulate the
  cleared map). Belt-and-braces: the WS client is also recreated when the
  authenticated identity changes (see `useWebSocket`/`ws-connector` below),
  dropping transport-queued frames from the old connection. Regression
  test: populate the map, change identity via `setAuthState` (map cleared,
  epoch bumped), then invoke the OLD handler registration with a synthetic
  `auth.session.hostname.resolved` event — the map must stay empty; the
  same event through a registration captured at the new epoch applies.
- New `apps/web/lib/ws/handlers/session-hostnames.ts`:
  `registerSessionHostnamesHandlers(store)` handling the new action (with
  the epoch guard: ignore events whose store epoch differs from the
  registration epoch); register
  it in `apps/web/lib/ws/router.ts`.
- `apps/web/lib/ws/use-websocket.tsx` / `apps/web/components/ws-connector.tsx`:
  recreate the client when the authenticated identity changes. **Reactivity
  requirement (both files must change):** reading `store.getState()` inside
  the existing `[store, url]`-keyed effect does NOT trigger a rerender, so
  the effect never re-runs on `setAuthState`/`clearAuthenticated`.
  `WebSocketConnector` must select the identity reactively — `useAppStore((s)
  => s.auth.user?.id ?? null)` plus the `authenticated` flag — and pass it
  (or a memoized identity string) as an explicit identity key into
  `useWebSocket`, which includes it in the effect dependencies. The auth
  gate does NOT unmount the connector on a user-id-only change
  (`auth-gate.tsx` keys on mode+authenticated), so the identity key is the
  only trigger — the lifecycle test MUST cover a **user-id-only
  transition** (authenticated stays `true`, e.g. another user logs in on
  the same instance), not just logout/login. The effect's cleanup disposes
  the old client (unsubscribes + disconnect); the new `WebSocketClient`
  starts with empty subscription maps, so re-establish every
  component-owned subscription the old client held (`subscribeUser` re-runs
  on connect; session/run subscriptions are re-sent by their owning hooks
  or via the client's resubscribe path — verify in the lifecycle test). Add
  a lifecycle test: a user-id-only `setAuthState` disposes the old client
  and creates a new one whose handler registration is captured at the new
  epoch.
- **Status-callback fencing (required):** the old client's status
  callbacks — its `onclose` (unconditional `disconnected`) **and `onerror`
  (unconditional `error`)** — must not be able to run
  after the replacement client has connected; an unguarded shared
  `connectionStatus` write from the disposed client can overwrite
  `connected` with `disconnected`/`error` (or clear subscriptions) after the
  new
  client is live, leaving app state stale while the transport is up.
  `useWebSocket` must either detach the old client's status callbacks in
  the effect cleanup or pass an active-client/generation guard so
  callbacks from a disposed client are ignored. Lifecycle regression:
  delay the old client's `onclose` **and `onerror`** until after the
  replacement's
  `onopen`/connected — assert `connectionStatus` stays `connected` and the
  replacement's subscriptions survive.
- **Hook-owned subscription re-establishment (required):** not every
  subscription is client-held and recreated automatically. Hooks that
  subscribe via `getWebSocketClient()` with effect deps that do NOT include
  the client identity (e.g. `use-system-metrics-subscription` with `[enabled]`,
  `office run live sync` with `[runId, status]`) silently keep their
  subscription on the disposed client after an identity-triggered
  replacement, leaving those surfaces stale. Expose a reactive client
  generation (or reuse the identity key) and include it in every
  hook-owned subscription effect — at minimum system metrics and office run
  live sync — with a regression test proving a replacement client receives
  those subscriptions after a user-id-only transition. Audit
  `getWebSocketClient()` call sites for other hook-owned subscriptions with
  the same gap.
- `apps/web/components/settings/account/security-settings.tsx`:
  - New `ResolveHostnamesCard` (or a settings row in the sessions card
    header) with a `Switch` bound to `userSettings.resolveSessionHostnames`,
    immediate `updateUserSettings({ resolve_session_hostnames })` save and
    store update on toggle, `data-testid="account-resolve-hostnames-switch"`,
    switch row at least 44 px tall.
  - `SessionsCard`: render the **Hostname** `<TableHead>` + cells only while
    the toggle is on. Cell resolution compares the streamed map entry
    (`{hostname, resolvedAt}`) with the session's response entry
    (`hostname`, `hostname_resolved_at`): **newer `resolved_at` wins; a
    durable entry (non-null) beats a transient/unknown one (null); ties
    break to the non-empty value; both empty → `N/A`**
    (`t("account:hostnameNotAvailable")`). This reconciles the retained map
    against newer list snapshots after a WebSocket gap while still letting a
    fresh streamed event beat an in-flight stale response. **No pending
    placeholder:** an empty value is indistinguishable from a cached
    no-answer, so it always renders `N/A`; a streamed event replaces it when
    the lookup completes. Read `state.sessionHostnames` (top-level) via
    `useAppStore`.
  - `useSessionsList`: refetch on window `focus` (in addition to mount) and
    on the `false -> true` setting transition (see below), so sessions
    created elsewhere appear and backend-cached hostnames render without a
    full reload. No session-created WS event is added.
  - **Reload protocol (trailing coalescing + generation guard):** a single
    reload function with (a) an in-flight flag, (b) a `pendingTrigger`
    boolean, and (c) a monotonically increasing generation counter. A
    trigger (focus or enable-transition) that arrives while a fetch is in
    flight sets `pendingTrigger` instead of starting a second fetch; when
    the in-flight fetch settles, one more reload runs if `pendingTrigger`
    was set — no trigger is ever dropped. Responses carry the generation
    they started with; only the latest generation's response is applied
    (stale responses from earlier generations are ignored), so an older
    slow response can never overwrite newer rows. Tests: focus during an
    in-flight request and enable-transition during an in-flight request
    each produce exactly one trailing reload; a stale response arriving
    after a newer one is discarded.
  - **Enable-transition refetch (sequenced after persistence):** the toggle
    card's PATCH handler **awaits `updateUserSettings(...)` and applies the
    PATCH response to the store** — it must NOT optimistically flip the
    setting before the PATCH commits. The single effect watching
    `userSettings.resolveSessionHostnames` then observes the `false -> true`
    transition only after persistence, and its refetch hits a backend whose
    settings gate is already `true` (an un-sequenced optimistic update
    races: the refetch GET can run while the durable setting is still
    `false`, returning empty hostnames and scheduling no DNS, with no later
    trigger to recover). This same effect is the ONLY refetch path (same-tab
    and cross-tab `user.settings.updated` behave identically); it uses
    trailing coalescing — a trigger during an in-flight fetch sets a pending
    flag and runs exactly one trailing reload after it settles — and a
    generation guard discards stale responses. Add a deferred-PATCH test
    asserting the refetch GET occurs after the PATCH commits and lookups
    start.
  - Keep component files under the 600-line/100-line lint limits; extract the
    column into a small sub-component if needed.
- **Mobile composition:** the toggle row is a standard touch target; the
  sessions table remains the sole internal scroll owner; adding the Hostname
  column must not introduce document-level horizontal overflow on the Pixel 5
  path. No mobile-specific page or modal.
- i18n: add `account.json` keys to every catalog
  (`en`, `pseudo`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`) under
  **`apps/web/src/locales/`** (the actual catalog directory):
  `resolveDeviceHostnames`, `resolveDeviceHostnamesDescription`, `hostname`,
  `hostnameNotAvailable` (English value `N/A`). Regenerate pseudo with
  `pnpm run i18n:pseudo`; never hand-edit pseudo output.

## Files likely touched

- `apps/web/lib/api/domains/auth-api.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/auth/types.ts`
- `apps/web/lib/state/slices/auth/auth-slice.ts`
- `apps/web/lib/state/slices/auth/auth-slice.test.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/store-overrides.ts`
- `apps/web/lib/ws/handlers/session-hostnames.ts`
- `apps/web/lib/ws/handlers/session-hostnames.test.ts`
- `apps/web/lib/ws/router.test.ts` (or existing router test file)
- `apps/web/lib/ws/router.ts`
- `apps/web/lib/ws/use-websocket.tsx` (identity-dependent client lifecycle)
- `apps/web/components/ws-connector.tsx`
- `apps/web/components/settings/account/security-settings.tsx`
- `apps/web/components/settings/account/security-settings.test.tsx`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/account.json`

## Acceptance

1. Toggle off by default: no Hostname column renders.
2. Toggling on PATCHes the setting and shows the column immediately with
   cached values or `N/A`; every empty value renders `N/A` with no pending
   spinner, and streamed events replace it.
3. A streamed `auth.session.hostname.resolved` event updates matching rows
   without a reload; cell resolution is by `resolved_at` (newer wins,
   durable beats transient, ties to non-empty), so a WS gap is reconciled by
   the next list snapshot, a fresh event beats a stale in-flight response,
   and an unchanged cached no-answer row renders `N/A` (its first
   absent → no-answer resolution DID emit an event with a persisted
   `resolved_at`; subsequent same-value refreshes publish nothing until the
   row's freshness expires and a retry finds a valid answer, which emits
   the changed hostname).
4. Toggling off hides the column; toggling on again restores it with the
   in-memory cached map values before new streams arrive.
5. `setAuthState` preserves `sessionHostnames` for the same identity and
   clears it when the identity changes (different user id or logout);
   `clearAuthenticated` (401 path) clears it too; the auth slice shape
   change compiles with `store.ts`'s explicit `auth` AppState type; every
   reference uses the top-level `sessionHostnames` path, never
   `auth.sessionHostnames`.
6. The `false -> true` setting transition (same tab or cross-tab) triggers
   a guarded sessions refetch; backend-cached hostnames render even when the
   list was last loaded while the toggle was off. Triggers during an
   in-flight fetch coalesce into exactly one trailing reload; stale
   responses never overwrite newer rows.
7. The sessions list refetches on window focus (same coalescing protocol); a
   session created elsewhere appears without a full reload.
8. All new copy passes `i18n:ratchet` and the pseudo locale renders; catalog
   files live under `apps/web/src/locales/`.
9. `AuthSession` and the WS payload types match the backend wire shapes.
10. Mobile: toggle and table remain reachable/touchable at Pixel 5 width
    with no document horizontal overflow.

## Verification

```sh
(cd apps/web && pnpm exec vitest run lib/state/slices/auth/auth-slice.test.ts lib/ws/handlers/session-hostnames.test.ts components/settings/account/security-settings.test.tsx lib/api/domains/auth-api.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
git diff --check
```

## Dependencies

Tasks 01 and 02 (setting, sessions `hostname`, WS action).

## Risks

- Nesting `sessionHostnames` under `auth` lets `setAuthState` erase it on
  every login/`me` refresh; keep it a top-level sibling of `auth` in the
  slice and `AppState`, address it only as `sessionHostnames`, and pin the
  preservation-in-same-identity / clear-on-identity-change semantics in
  tests.
- A toggle handler that reloads sessions itself (instead of the single
  setting-transition effect) double-fetches and behaves differently for
  same-tab vs cross-tab; the effect watching the store value is the one
  refetch path.
- Forgetting `default-state.ts` / `store-overrides.ts` in the state shape
  change makes `createAppStore({ sessionHostnames })` silently drop the
  supplied map (slices spread after the default state).
- A pending spinner is not representable: empty values (unresolved AND
  cached no-answer) render identically. An unchanged no-answer row emits no
  further events while fresh, but after `recentResolveWindow` a retry may
  replace the empty with a valid answer and publish it; never render a
  pending state; empty always means `N/A` until such a changed event or a
  newer list snapshot.
- Column visibility and the toggle must read the same store value; a local
  copy that does not update on the WS `user.settings.updated` push leaves the
  column stuck.
- A retained map with no ordering information can permanently override a
  newer sessions response after a WS gap; entries must carry `resolvedAt`
  and cells must compare `resolved_at` (newer wins, durable beats
  transient), with the event → disconnect → list-refresh regression test.
- A focus/transition refetch that drops triggers while a fetch is in flight
  misses the only refresh for that event (stale list forever); use trailing
  coalescing (pending flag + exactly one post-settle reload) and a
  generation guard so stale responses never overwrite newer rows.
- Missing pseudo entries fail `i18n:check`; regenerate, do not hand-edit.
- Forgetting `apps/web/lib/state/store.ts` in the slice-shape change breaks
  typecheck (AppState pins the `auth` field shape).

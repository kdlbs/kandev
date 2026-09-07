---
spec: docs/specs/session-hostname-resolution/spec.md
created: 2026-08-16
status: draft
---

# Implementation Plan: Optional reverse-DNS hostnames for account sessions

## Outcome

A per-user **Resolve device hostnames** toggle on
`/settings/account/security`, off by default. Enabling it shows a Hostname
column on the sessions table; the backend reverse-resolves the user's session
IPs through the OS resolver (`net.LookupAddr`) in the background, caches
results per IP in the configured database, and streams changed values to the
page over the existing WebSocket gateway. No-answer IPs and generic `.in-addr.arpa` /
`.ip6.arpa` answers render `N/A`. Disabling hides the column; re-enabling
restores cached values instantly and streams only deltas.

## Fixed decisions

- Wire setting `resolve_session_hostnames: boolean`; frontend state
  `userSettings.resolveSessionHostnames`. Missing stored values and initial
  payloads default to `false`; PATCH omission preserves the current value.
- Turning the setting on publishes the persisted user-settings event. The
  hostname resolver consumes that event and starts background resolution for
  the user's session IPs. The sessions list response also triggers resolution
  for uncached/new IPs and carries the cached `hostname` per session.
- New cache table `auth_hostname_cache`, owned by the hostnames cache store
  (`internal/auth/hostnames/store.go`) whose `initSchema` creates it (new
  table, so `CREATE TABLE IF NOT EXISTS` suffices; no `ADD COLUMN`
  migration).
- Cache is global by IP; `hostname == ""` means no answer and is cached.
  `recentResolveWindow = 30s` skips re-query of freshly resolved IPs;
  `lookupTimeout = 3s`; `lookupWorkers = 4`; in-flight IPs are deduplicated
  with an interested-user set per IP.
- Streaming: the internal event-bus event is `auth.session.hostname.resolved`
  with payload `{user_id, ip, hostname, resolved_at}` (fixed 9-digit
  RFC3339 UTC or null) — the `UserEventBroadcaster` routes by `user_id` and
  strips it — and the WS action `auth.session.hostname.resolved` carries the
  reduced `{ip, hostname, resolved_at}` to the client; published only when
  the hostname value changed
  versus the cache. Every successful lookup (valid or `IsNotFound`)
  persists and refreshes `resolved_at` via `Set` even when the hostname is
  unchanged, so `recentResolveWindow` engages for stable and no-answer rows.
  Cache/job keys are canonical IPs; the event `ip` is each interested
  (user, raw stored session-IP spelling) pair so frontend row matching needs
  no IP parsing; one user with two spellings of the same IP gets events for
  both. The sessions response carries `hostname` and `hostname_resolved_at`
  per session so the frontend can order snapshots against streamed events.
- Frontend keeps an in-memory per-IP streamed map in the auth store slice as
  a **top-level `sessionHostnames` sibling of `auth`** (never nested, never
  addressed as `auth.sessionHostnames`; also wired through
  `default-state.ts` and `store-overrides.ts` so caller-supplied maps
  survive store construction). Entries are `{hostname, resolvedAt}`
  (fixed 9-digit RFC3339 UTC or null). It survives disable/enable and
  same-identity
  refreshes and is **cleared when the authenticated identity changes**
  (setAuthState with a different user, logout, and the 401/clearAuthenticated
  path). The
  The SessionsCard shows the Hostname column only while the toggle is on; cell
  resolution compares `resolved_at` (**newer wins; durable beats transient;
  ties to non-empty; both empty → `N/A`**), which reconciles the retained
  map against newer list snapshots after a WebSocket gap while still letting
  a fresh streamed event beat an in-flight stale response. The SAME
  comparator lives in `setSessionHostname` (not a blind upsert) so a
  delayed older event can never overwrite a newer map entry. There is
  **no pending spinner**: empty values always render `N/A`. A first
  absent → no-answer resolution IS a change and publishes (an `IsNotFound`
  negative with a persisted `resolved_at`; a transient DNS error over an
  absent cache with `resolved_at: null`); a subsequent same-value cached
  no-answer only refreshes `resolved_at` via `Set` and publishes nothing,
  so an unchanged settled no-answer row emits no further events and renders
  `N/A` **until its freshness expires (`recentResolveWindow`) and a retry
  replaces the empty with a valid answer, which publishes the changed
  hostname**. A
  `false -> true` setting transition
  (same tab or cross-tab) refetches the sessions list once (single guarded
  effect with trailing coalescing — a trigger during an in-flight fetch
  schedules exactly one post-settle reload — and a generation guard that
  discards stale responses), so backend-cached hostnames render even when
  the list was last loaded while off; the list also refetches on window
  focus with the same protocol. No session-created push event.
- The settings gate fails closed: a settings read error behaves exactly like
  the toggle being off (empty hostname map, no lookups, logged) — DNS must
  never run while opt-in is unknown.
- DNS errors are classified: `*net.DNSError` with `IsNotFound` (NXDOMAIN /
  no PTR, the common no-answer case) is a successful negative answer
  persisted as `hostname = ""`; timeout/temporary/server errors are never
  persisted and are published `""` only for IPs with no cache entry (never
  over an existing cached value, which would mask it client-side).
- Resolver API contract: `HostnamesForSessionIPs(ctx, userID string, ips
  []string) map[string]CacheEntry` — raw-input-keyed, `ErrNotFound` treated
  as a successful absent baseline, all degradations (settings/cache errors)
  logged internally, never returns an error and never fails the sessions
  handler.
- Production wiring calls `Start(ctx)` explicitly, registers `Close` as
  cleanup, queue-subscribes the resolver to the persisted
  `UserSettingsUpdated` event, and threads the resolver to `registerRoutes`
  via the `Services` struct (`types.go`).
- The toggle hook is fire-and-forget: `TriggerUserSessionsResolved(userID)`
  enqueues on a resolver-owned trigger queue and returns immediately (PATCH
  never blocks on settings/session/cache reads); the trigger goroutine and
  workers run on the resolver-owned context and `Close` joins them.
- The lookup function is injectable for tests; default is
  `net.DefaultResolver.LookupAddr`. No HTTP/API calls in the lookup path.
- The resolver depends on a narrow `Cache` interface
  (`GetMany`/`Get`/`Set` over `CacheEntry{Hostname string, ResolvedAt
  *time.Time}`, absent rows absent from results, `ErrNotFound` treated as a
  successful absent baseline) for deterministic error injection and
  `recentResolveWindow`; timestamps are fixed 9-digit fractional-second
  RFC3339 UTC text (lexicographically comparable, so same-millisecond
  changes still order). A `Set` error publishes
  **nothing** (changed-only semantics; the next trigger retries the persist,
  and a later successful `Set` that changes the cache publishes);
  `resolved_at: null` in events is reserved for transient DNS errors over an
  absent cache (empty strings), never for failed writes. Admission records
  the observed cache value per
  pending IP; a worker that finds an IP fresh within
  `recentResolveWindow` still publishes the current value to interested
  users when it differs from what the requester saw (stale-read →
  concurrent completion → enqueue must not silently drop the update), and
  re-checks the settings gate at dequeue so **DNS never runs while the
  toggle is off** (disabled users' interests are dropped; a job with no
  enabled interested users is skipped).
  Session-IP providers return raw validated spellings; canonicalization
  stays inside the resolver. Gateway tests assert `resolved_at` (fixed-width
  RFC3339 and null) survives both memory and NATS transports; a frontend
  router integration test pins `registerSessionHostnamesHandlers` in
  `registerWsHandlers`.

## Backend

### Task 01 — portable setting contract

- `AppStatusBarEnabled`-style field `ResolveSessionHostnames bool` in
  `internal/user/models/models.go`.
- Response + PATCH `*bool` in `internal/user/dto/dto.go`, controller mapping,
  `applyBasicSettings` in `internal/user/service/service.go`, event payload
  key `resolve_session_hostnames` in `publishUserSettingsEvent`.
- Store: default false, JSON marshal key, `*bool` scan decode, overwrite only
  when present (`internal/user/store/sqlite.go`).
- Boot mapping `resolveSessionHostnames` in
  `internal/backendapp/boot_state_routes.go`.
- Frontend: HTTP types (`http-user-settings.ts`), `UserSettingsState` field,
  shared mapper in `lib/ssr/user-settings.ts` (default false, omission
  preserves), revision-ordered WS handler fixture updates.
- Tests: model/DTO/service/store/boot/event round trip; frontend mapper and
  WS handler tests.

### Task 02 — backend reverse-DNS service

- New package `internal/auth/hostnames`:
  - `store.go`: database-portable cache store (`auth_hostname_cache`),
    `Get`/`Set`;
  - `normalize.go`: `NormalizeHostname([]string) string` (first-valid-record:
    strip trailing dot, reject empty and `.in-addr.arpa`/`.ip6.arpa`
    suffixes);
  - `resolver.go`: `Resolver` with `Start`/`Close` worker pool (goleak),
    in-flight + interested-user dedup, `recentResolveWindow` guard,
    `HostnamesForSessionIPs(ctx, userID, ips)` (enabled gate, cached values,
    schedules lookups) and fire-and-forget
    `TriggerUserSessionsResolved(userID)` (owned trigger queue, PATCH never
    blocks); changed-only event publishing with `IsNotFound` persisting as a
    cached no-answer and timeout/error → `""` not persisted.
- `auth.Service.SessionIPs(ctx, userID)` (distinct raw spellings of the
  user's sessions, skipping empty/invalid).
- `events` constant `AuthSessionHostnameResolved`.
- `pkg/websocket/actions.go`: `ActionSessionHostnameResolved`.
- `gateway/websocket/user_notifications.go`: subscribe the new event to the
  new action.
- `auth/httpapi`: `RegisterRoutes(router, svc, resolver, log)`; `listSessions`
  returns per-session `hostname` and schedules resolution.
- `user/service`: publish the complete persisted setting through the existing
  `UserSettingsUpdated` event.
- `backendapp`: construct the resolver (store over `dbPool`, session-IPs =
  `authSvc`, settings gate over `repos.User`, event bus, log), subscribe it to
  setting updates, and pass it to `authhttpapi.RegisterRoutes`.
- Tests: store, normalize, resolver (dedup, changed-only, interested-user
  routing, disabled gate, recent-window, timeout-not-persisted), goleak,
  httpapi sessions payload, and persisted settings event handling.

## Frontend

### Task 03 — streaming and UI

- `AuthSession` type gains `hostname: string` and
  `hostname_resolved_at: string | null` (fixed 9-digit timestamp or null).
- `BackendMessageMap` entry + `SessionHostnameResolvedPayload` (`{ip,
  hostname, resolved_at}`); WS handler
  `lib/ws/handlers/session-hostnames.ts` registered in `router.ts` updating
  top-level `sessionHostnames`; router integration test pins the
  registration.
- Auth slice: `sessionHostnames: Record<string, SessionHostnameEntry>` with
  `SessionHostnameEntry = { hostname: string; resolvedAt: string | null }`
  and `setSessionHostname(ip, hostname, resolvedAt)` (default empty).
- `security-settings.tsx`: toggle card (Switch, direct save via
  `updateUserSettings`, `data-testid`), SessionsCard Hostname column gated on
  the setting, cell resolution by `resolved_at` (newer wins, durable beats
  transient, ties to non-empty, both empty → `N/A`; lexicographic string
  comparison, never `Date.parse`).
- i18n: `account.json` keys (`resolveDeviceHostnames`,
  `resolveDeviceHostnamesDescription`, `hostname`, `hostnameNotAvailable`) in
  all catalogs; regenerate pseudo.
- Tests: auth slice action, WS handler, router registration, mapper,
  component test for toggle + column rendering + streamed update +
  resolved_at ordering (WS-gap reconciliation, same-millisecond ordering) +
  N/A.

## E2E and verification

### Task 04 — E2E and final verification

- Playwright spec under `apps/web/e2e/tests/auth/` — files there run in the
  `auth` project (backend restarted with `KANDEV_FEATURES_AUTH: true`).
  **Setup is `setupAdmin(...)` only for the main flow** (`/auth/setup`
  creates the admin session and sets the cookie, so the baseline is exactly
  one row — do NOT call `login` before navigation, which would create a
  second baseline row); the second `login(...)` is reserved for the
  focus-refetch step. Reset the setting to `false` in `afterEach`:
  toggle off default (no Hostname column), toggle on (column appears
  immediately; each row's cell is hostname-or-`N/A` — no WS-event waits,
  the worker backend cache persists across tests and e2e logins share one
  loopback IP so first-resolution events are not guaranteed), toggle off
  (column hidden), toggle on again (column reappears with cached values),
  setting survives reload, and focus-refetch discovers a second login
  (login again through the same context's request client, navigate a second
  same-context page, `bringToFront` page 1, row count grows by one).
- Mobile: `tests/auth/mobile-account-security-hostnames.spec.ts` (runs in the
  `mobile-chrome` project; the auth project's `testIgnore` excludes
  `mobile-*`) covering the same enable/disable/re-enable flow and column
  rendering at Pixel 5 viewport.
- Run backend focused tests + lint, frontend focused tests, `i18n:check` +
  `i18n:ratchet`, typecheck, lint, targeted E2E (auth + mobile-chrome
  projects), `git diff --check`.

## Implementation waves

Execution is sequential in the primary conversation.

| Wave | Task | Status | Gate |
|---|---|---|---|
| 1 | [01 portable setting contract](task-01-portable-setting-contract.md) | pending | `resolve_session_hostnames` round trips backend→boot→frontend with default false. |
| 2 | [02 backend reverse-DNS service](task-02-backend-reverse-dns-service.md) | pending | Lookup, cache (GetMany/CacheEntry), changed-only streaming and sessions `hostname`/`hostname_resolved_at` fields green (fail-closed settings gate, IsNotFound negative caching, ErrNotFound-as-absent, transient-error masking rule, fire-and-forget trigger, off-state empty map, pending-set admission, resolver-owned contexts, IP canonicalization + raw-IP event keys, Cache-interface error semantics, production Start wiring + settings-event subscription, gateway routing tests, schema replay); toggle still has no UI. |
| 3 | [03 frontend streaming and UI](task-03-frontend-streaming-and-ui.md) | pending | Toggle + Hostname column (empty always N/A, no spinner) + streamed updates + focus-refetch, desktop and mobile. |
| 4 | [04 E2E and final verification](task-04-e2e-and-final-verification.md) | pending | Auth-project desktop E2E + mobile-chrome E2E (persistence/column behavior, no WS-event waits) and full checks pass. |

Task 02 depends on Task 01 (the resolver reads the setting); Task 03 depends
on 01 + 02; Task 04 depends on 03. No tasks are parallel-safe.

## Verification order

If `apps/node_modules` is absent, run once before frontend checks:

```sh
(cd apps && pnpm install --frozen-lockfile)
```

Each task runs its focused commands. Final verification:

```sh
(cd apps/backend && go test ./internal/auth/... ./internal/user/... ./internal/gateway/websocket/... ./internal/backendapp/... ./pkg/websocket/...)
make -C apps/backend lint
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/session-hostnames.test.ts lib/state/slices/auth/auth-slice.test.ts components/settings/account/security-settings.test.tsx)
(cd apps/web && pnpm e2e:run --project auth tests/auth/account-security-hostnames.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-account-security-hostnames.spec.ts)
git diff --check
```

## Risks

- A backend or frontend default that stays `true` would resolve IPs for users
  who never opted in. Pin default-false at every boundary (store, boot,
  mapper, state), and gate the sessions-response map so cached hostnames
  never leak while the toggle is off.
- Forgetting the sessions-response `hostname` field or the event key in the
  complete settings event breaks hydration/streaming silently; pin both in
  tests.
- DNS behavior varies by environment (no PTR, `localhost`, synthesized
  names); E2E must assert column presence/absence and hostname-or-`N/A`
  cells, never an exact hostname, and must NOT wait for WS events (fresh
  cache and unchanged results intentionally emit nothing).
- Multi-user: streaming must be routed per user; the interested-user set
  prevents user B from missing updates when user A's lookup for a shared IP
  completes.
- Rapid toggle cycles must not hammer DNS; the `recentResolveWindow` guard
  and in-flight dedup cover this. Admission must never drop a required IP:
  workers drain a pending set rather than a drop-on-full queue.
- Lookups must run on a resolver-owned context (canceled by `Close`), never
  a request context, or they die with the HTTP response.
- The auth slice's `sessionHostnames` map must be a top-level sibling of
  `auth` (never nested, never `auth.sessionHostnames`), or `setAuthState`
  (login/me refresh) erases it; `store.ts` must be updated with the slice
  shape.
- A cache interface that cannot distinguish absent from cached-empty or
  expose `ResolvedAt` makes no-answer caching and `recentResolveWindow`
  unimplementable; the resolver's `Cache` returns `CacheEntry` and absent
  rows are absent from `GetMany` results.
- Raw session-IP spellings must survive to the event payload (per user, per
  spelling), or hook-triggered resolutions publish canonical `ip` values the
  frontend cannot match; providers return raw validated spellings and only
  the resolver canonicalizes for job/cache keys.
- A settings-gate read error must fail closed (no lookups, empty map); a
  default-to-enabled fallback would run DNS while opt-in is unknown.
- A pending spinner is not representable (cached no-answer rows emit no
  event): empty always renders `N/A`, and E2E must not depend on WS events
  (the worker backend cache persists across tests and e2e logins share one
  loopback IP, so first-resolution/changed events are not guaranteed).
- The pending set needs the complete-then-recheck invariant: workers must
  re-drain after every completion or IPs admitted while all workers are
  busy strand forever.
- Without the enable-transition refetch, enabling after a list load that
  happened while off shows `N/A` forever (the off-state response carried no
  cached hostnames and changed-only events may never come); the single
  guarded effect watching the setting is the fix.
- The in-memory map must clear on identity change (different user or
  logout) or one user's session IPs seed another user's rows; keep the
  disable/enable survival separate and test both.
- `createAppStore({ sessionHostnames })` silently drops the map unless the
  field is added to `default-state.ts` and `store-overrides.ts` (slices
  spread after the default state).
- E2E focus-refetch needs a second page in the same context that actually
  navigates (the `login` helper never focuses a page), then page 1
  `bringToFront`; it must not be skippable.
- The common no-PTR case surfaces as `*net.DNSError` with `IsNotFound`, not
  as records; failing to classify it means no-answer IPs are never cached
  and re-queried on every trigger. Persist `IsNotFound` as a cached no-answer
  and only treat timeout/temporary/server errors as non-persisted.
- Transient failures must not mask durable values: the backend publishes
  transient `""` only for IPs with no cache entry, and the frontend orders
  by `resolved_at` (newer wins, durable beats transient), or a stale-cache +
  transient-error + re-enable cycle pins rows to `N/A`.
- Without a per-IP timestamp, the retained streamed map can permanently
  override a newer sessions response after a WS gap (events during a
  disconnect are lost and never replayed); both the API and the WS payload
  must carry `resolved_at` and cells must compare it.
- `ErrNotFound` must be treated as an absent baseline (never as a generic
  cache error) or first-resolution transient behavior and negative caching
  break; pin the distinction in resolver tests.
- A same-value successful lookup that skips `Set` never refreshes
  `resolved_at`, so `recentResolveWindow` never engages and stable/no-answer
  rows re-query on every trigger; successful lookups always persist and
  refresh the timestamp, while publishing stays changed-only.
- A missing `UserSettingsUpdated` subscription lets a successful PATCH stop
  before it starts lookup work. The resolver event-handler test pins that
  boundary without adding feature knowledge to the user settings service.
- Clearing `sessionHostnames` on identity change is not enough: a delayed
  WS frame from the prior user (the gateway strips `user_id`) can
  repopulate the cleared map. Fence the stream with a
  `sessionHostnamesEpoch` handler guard plus WS client recreation on
  identity change, and pin the stale-event regression test. Client
- Client recreation must be reactive (an identity selector in `WebSocketConnector`
  / `useWebSocket` deps — a `store.getState()` read inside the effect does
  not rerender), must cover user-id-only transitions (the auth gate does
  not unmount the connector on those), and must re-establish client-held
  subscriptions — and the disposed client's status callbacks (`onclose`
  AND `onerror`) must be fenced (detached or generation-guarded) so a
  delayed callback cannot overwrite the replacement's `connected` status.
  Hook-owned subscriptions obtained via `getWebSocketClient()` (system
  metrics, office run live sync, others) must include the reactive client
  generation in their deps or they silently stay on the disposed client.
- Streamed hostname notifications fan out to the user's own WS connections
  (all tabs); the spec's non-goal is rendering-scoped — only the security
  page renders the data, other tabs retain but never show it (this is what
  powers instant re-enable/cross-tab cached results).
- The enable-transition refetch must run after the PATCH commits (apply the
  PATCH response to the store, no optimistic flip), or the refetch GET can
  race a still-false settings gate and permanently miss cached hostnames
  and DNS scheduling.
- A `Set`-error publish without explicit semantics makes an unpersisted
  value look durable (or linger transient); publish nothing on `Set` error
  and let the retry persist-then-publish.
- The interested-user set must be snapshot+cleared exactly once on every
  job terminal path (publish, no-change, dequeue-gate skip, cache errors,
  nil bus) or stale routing state accumulates unboundedly under
  session/IP/user churn; in-flight merges are the only retained interests.
- The timestamp formatter must be an exported `hostnames` API
  (`FormatTimestamp`/`ParseTimestamp`) used by the store, resolver, and
  httpapi handler — unexported helpers would force layout duplication
  across packages and drift the fixed-width ordering contract.
- The controller's field-by-field PATCH mapping can silently drop
  `resolve_session_hostnames` while DTO/service/store tests pass; pin the
  pointer mapping in `controller_test.go`.
- A forgotten production `Start(ctx)` passes every unit test (they call
  Start themselves) and every E2E (which tolerates `N/A`); the backendapp
  wiring smoke test is the only guard.
- A missing router registration (`registerSessionHostnamesHandlers`) passes
  direct-handler tests and `N/A`-tolerant E2E while the streaming path is
  dead; the router integration test is the guard.
- Without a dequeue-time settings recheck, DNS can run after the toggle is
  disabled for work admitted while it was on; re-check at dequeue, drop
  disabled users' interests, and skip jobs with no enabled users.
- `Date.parse`/`RFC3339Nano` cannot order same-millisecond changes; the
  fixed-width 9-digit fraction keeps timestamps lexicographically
  comparable in Go and JS.
- The desktop E2E must use an isolated `KANDEV_DATABASE_PATH` like the
  mobile spec; the auth worker is serial/shared and `backend.restart`
  preserves the DB, so leaked sessions/cache/admin state makes row-count
  and setup assertions order-dependent.
- A synchronous hook or a naive goroutine in `UpdateUserSettings` blocks the
  PATCH or dies with the request context; the hook must enqueue on the
  resolver-owned trigger queue and return immediately.
- E2E focus-refetch must create the second session by calling `login` again
  through the same context's request client (navigation alone reuses the
  cookie and adds no session row), then navigate a second same-context page
  for a real blur/focus, then `bringToFront` page 1; it must not be
  skippable.
- The mobile spec runs in `mobile-chrome`, which does not inherit the auth
  project's backend restart; it must restart auth-enabled with an isolated
  DB itself or the page never loads sessions.
- The sessions-list refetch must use trailing coalescing + generation
  guard: a trigger dropped during an in-flight fetch loses the only refresh
  for that event, and an older slow response can overwrite newer rows.
- E2E specs under `tests/auth/` only run in the `auth` project; auth-mode
  setup (`setupAdmin`/`login`) and per-test cleanup are required, and the
  mobile spec must be a separate `mobile-*.spec.ts` file for the
  `mobile-chrome` project.

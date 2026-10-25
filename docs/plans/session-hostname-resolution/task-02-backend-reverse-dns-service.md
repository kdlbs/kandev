---
id: session-hostname-resolution-02
title: Backend reverse-DNS lookup, cache, and streaming
status: pending
wave: 2
depends_on: [session-hostname-resolution-01]
plan: docs/plans/session-hostname-resolution/plan.md
spec: docs/specs/session-hostname-resolution/spec.md
---

# Backend reverse-DNS lookup, cache, and streaming

## Inputs

The [spec](../../specs/session-hostname-resolution/spec.md) (Lookup pipeline,
Streaming, Persistence, Failure modes sections) and Task 01's
`resolve_session_hostnames` setting. Existing patterns:
`internal/auth/store` schema init, `internal/gateway/websocket/user_notifications.go`
broadcaster, `internal/integrations/healthpoll` Start/Stop goroutine shape,
backend AGENTS.md schema-replay rule (ADR 0027).

## TDD sequence

1. Add failing tests for `NormalizeHostname` (trailing dot, empty,
   `.in-addr.arpa`/`.ip6.arpa` suffix rejection, first-valid-record
   selection: a generic first record followed by a valid second record
   yields the valid one).
2. Add failing tests for the cache store (`CacheEntry` round trip via
   `GetMany`, absent-vs-cached-empty distinction, `resolved_at` fixed 9-digit
   RFC3339 UTC text
   round trip, and a schema replay test: fresh-DB `initSchema` plus
   re-running `initSchema` on the same DB is a no-op; include Postgres-gated
   fresh + replay coverage because the store is wired over the production
   `dbPool`, mirroring the auth store test harness).
3. Add failing tests for the resolver:
   - disabled gate: setting off returns an **empty map** (no cached values)
     and schedules nothing;
   - **gate read error:** the settings provider returning an error behaves
     exactly like disabled — empty map, no lookups scheduled, no DNS
     queries, logged (fail closed, opt-in unknown means no lookups);
   - cached values returned immediately when enabled (via one `GetMany` call,
     not per-IP reads);
   - **absent vs cached-empty:** an IP with no cache row schedules a lookup;
     an IP cached as no-answer (`hostname == ""` with a `ResolvedAt` within
     `recentResolveWindow`) does not;
   - **stale/fresh:** an IP cached within `recentResolveWindow` is not
     re-queried; an older one is;
   - in-flight dedup registers an interested user instead of a second
     lookup;
   - **raw-IP key contract:** triggering with `::ffff:192.0.2.10` and
     `192.0.2.10` runs ONE job (canonical dedup) and publishes two events,
     each carrying the triggering user's own raw spelling — including **two
     sessions of the SAME user** with different raw spellings of the same
     canonical IP (both rows must receive events);
   - **interested-set lifecycle:** snapshot+clear each IP's interested
     pairs exactly once on every terminal path (publish, no-change,
     dequeue-gate skip, cache-fresh equal, cache-read error, `Set` error,
     nil bus), with a churn/cleanup test asserting no completed interest
     survives and in-flight merges are retained;
   - a changed result persists + publishes once to all interested users; an
     unchanged result publishes nothing;
   - **DNS error classification:** a `*net.DNSError` with `IsNotFound`
     (NXDOMAIN/no PTR — the common no-answer case) is a successful negative
     result: persisted as `hostname = ""` and published when changed, then
     skipped by `recentResolveWindow` on the next trigger; a timeout /
     temporary / server error yields `""` **not persisted**, and is
     published **only when the IP has no cache entry** — never over an
     existing cached value (regression: stale cache `old.example` +
     transient error publishes nothing, and re-enable/list still returns
     `old.example`);
   - **Set-on-success:** a same-value successful lookup (valid or
     `IsNotFound`) still `Set`s and refreshes `resolved_at` (publishing
     nothing when the hostname is unchanged), so the very next trigger is
     skipped by `recentResolveWindow` — a stale timestamp that never
     refreshes would re-query on every trigger;
   - **cached-empty requery:** a cached no-answer row whose freshness has
     expired (`ResolvedAt` older than `recentResolveWindow`) is re-queried
     on the next trigger; when the retry finds a valid answer the change
     persists and publishes (regression: cached-empty → later valid
     answer after the window);
   - **fire-and-forget trigger:** `TriggerUserSessionsResolved(userID)`
     returns immediately (does not block on settings/session/cache reads),
     the work runs on the resolver-owned context, and `Close()` joins the
     trigger goroutine (goleak);
   - **admission never drops**: more distinct IPs than workers all resolve
     eventually (deterministic `N > lookupWorkers` pending-set drain test
     with blocked/fake lookups proving the complete-then-recheck invariant);
     IPs admitted while all workers are busy are not stranded;
   - **stale-read-after-completion race:** user B's snapshot read sees the
     old cache value, a concurrent job persists a new value (now fresh
     within `recentResolveWindow`), and B then enqueues from its stale read
     — the worker must skip the DNS query but publish the current cache
     value to B (their observed value differs), never silently drop the
     update;
   - **context ownership**: canceling the request context does not cancel the
     in-flight lookup, and `Close()` joins workers (goleak);
   - **IP canonicalization**: invalid IPs are skipped; equivalent IPv6
     spellings produce one cache row and one job;
   - **cache-error semantics via the `Cache` interface**: a `GetMany` error
     on the API path returns empty entries for the affected IPs and
     schedules nothing; on completion **`ErrNotFound` is a successful absent
     baseline (absent + transient error publishes `""` without persisting;
     absent + valid result persists and publishes) while **non-`ErrNotFound`
     cache errors** suppress both persist and publish; a `Set` error
     publishes **nothing** (changed-only semantics; the next trigger
     retries the persist, and a later successful `Set` that changes the
     cache publishes) and
     does not panic; a nil event bus still returns cached values;
   - **entry semantics:** `CacheEntry` covers unknown (absent from map /
     `ResolvedAt` nil), cached-no-answer (`ResolvedAt` non-nil, `Hostname`
     `""`), and durable (`ResolvedAt` non-nil, `Hostname` non-empty); the
     handler emits `hostname_resolved_at` JSON `null` when `ResolvedAt` is
     nil;
4. Add failing httpapi test: `GET /api/v1/auth/sessions` includes per-session
   `hostname` keyed by raw IP; with the toggle off every `hostname` is `""`
   even when the cache has values (no leak); a cache/settings failure still
   returns 200 with `hostname: ""` (the resolver method never errors);
   user-service test: PATCH to `true` fires the resolver hook, and a
   non-canonical stored IP triggered through the hook (before any sessions
   GET) publishes an event with the raw stored spelling.
5. Add failing gateway tests in `user_notifications_test.go`: memory and
   NATS-shaped delivery of `auth.session.hostname.resolved` routes to the
   `user_id` owner only, strips `user_id` from the payload, and leaves
   `ip`, `hostname`, **and `resolved_at`** intact — for both a fixed
   9-digit RFC3339 UTC timestamp and a JSON `null` value (both must survive
   both transports;
   a dropped `resolved_at` breaks the frontend newer-wins reconciliation).
   Also assert the **internal event map the broadcaster receives contains
   `user_id`** (routing depends on it; an implementer publishing only
   `{ip, hostname, resolved_at}` would dead-end the stream).
6. Add a failing resolver event-handler test: a persisted
   `UserSettingsUpdated` event with `resolve_session_hostnames: true` queues a
   session pass for its user. A false or malformed event does not queue work.
7. Implement the smallest production code that makes the tests pass.

## Implementation

- New package `apps/backend/internal/auth/hostnames`:
  - `store.go`: `CacheEntry{Hostname string; ResolvedAt *time.Time}` —
    `ResolvedAt` is **`*time.Time`** (nil = no row/unknown; a cached
    no-answer has a non-nil `ResolvedAt` with `Hostname == ""`); `Store`
    over writer/reader `*sqlx.DB` with `initSchema` creating
    `auth_hostname_cache (ip TEXT PRIMARY KEY, hostname TEXT NOT NULL
    DEFAULT '', resolved_at TEXT NOT NULL)`; implements a narrow `Cache`
    interface: `GetMany(ctx, ips []string) (map[string]CacheEntry, error)`
    (one dialect-safe `IN` query, chunked to stay under parameter limits;
    **absent IPs are simply absent from the map**, so cached-no-answer
    (`hostname == ""` present) is distinguishable from never-resolved),
    `Get(ctx, ip) (CacheEntry, error)` (sentinel `ErrNotFound`), and
    `Set(ctx, ip, hostname, resolvedAt time.Time)`. **Time representation:**
    define ONE shared timestamp formatter as an **exported API of the
    `hostnames` package** — `hostnames.FormatTimestamp(time.Time) string`
    and `hostnames.ParseTimestamp(string) (time.Time, error)` — backed by a
    single `timestampLayout` constant
    (`"2006-01-02T15:04:05.000000000Z07:00"`). The helpers must be exported
    because the `httpapi` handler (a different package) emits the
    `hostname_resolved_at` API field, and the resolver emits the WS event
    `resolved_at`; unexported helpers would force duplication of the layout
    and risk drifting from the exact fixed-width UTC ordering contract. Use
    them **everywhere**: store reads and writes, the sessions API
    `hostname_resolved_at` field, and WS event `resolved_at`.
    `FormatTimestamp` writes
    `resolvedAt.UTC().Format(timestampLayout)` (UTC, so the zone is always
    `Z`, exactly 9 fractional digits). The fixed width makes the
    stored/emitted strings **lexicographically comparable in chronological
    order** in both Go and JavaScript (unlike `time.RFC3339`, which drops
    fractions, and `time.RFC3339Nano`, which trims trailing zeros — either
    would break string ordering for same-second or same-millisecond
    changes). Add sub-millisecond ordering tests at the store, handler, and
    gateway layers asserting the exact 9-digit output, **plus a
    cross-package test proving the handler emits the same helper output as
    the store (one formatter, one contract)**. This is
    dialect-portable
    across the SQLite/Postgres `db.Pool` — scanning a raw TEXT column
    directly into `time.Time` is not — and matches the explicit-parse
    pattern used elsewhere in the codebase. New table, so `CREATE TABLE IF
    NOT EXISTS` in `initSchema` is the whole migration; it must be
    replay-safe (see TDD step 2), with **required** Postgres fresh+replay
    coverage because the resolver is wired over the production `dbPool`
    (not conditional).
  - `normalize.go`: `NormalizeHostname(names []string) string` — examine
    records in order and return the **first one that normalizes to a valid
    hostname**: trim trailing dot; reject empty names and names ending in
    `.in-addr.arpa` / `.ip6.arpa` (case-insensitive); if none is valid
    return `""`.
  - `resolver.go`: `Resolver` with constants `lookupWorkers = 4`,
    `lookupTimeout = 3s`, `recentResolveWindow = 30s`; injectable
    `LookupAddr` defaulting to `net.DefaultResolver.LookupAddr`; depends on
    the `Cache` interface (not the concrete store) so tests can inject read
    and write failures.
    - **Admission (no drops, no missed updates):** pending entries carry
      the **observed cache value** the trigger read (`pending
      map[canonicalIP]observedCacheEntry`) plus the interested users; the
      worker loop selects on the wakeup channel / stop channel, then drains
      pending.
    - **Dequeue-time privacy gate:** immediately before calling
      `LookupAddr`, the worker re-reads the settings gate for every
      interested user (fail closed on error) and **drops users who disabled
      the toggle since admission**; if no interested user remains enabled,
      the job is skipped entirely — **no DNS query runs while the toggle is
      off**, even for work admitted while it was on. Jobs whose DNS already
      started are "in flight" and may complete (their events go only to the
      still-interested users). Regression test: enable → admit an IP →
      disable before dequeue → `LookupAddr` is never called and no event is
      published. For each admitted IP: if already in flight, merge the
      interested users into the running job; if the cache is now within
      `recentResolveWindow` (fresh), skip the DNS query but **compare the
      current cached hostname against the observed hostname and publish the
      current value to the job's interested users when they differ** — a
      user who
      scheduled from a stale snapshot must still learn of the change that
      happened since their read; if the observed hostname equals the current
      cached hostname, publish nothing (the diff compares the **hostname**,
      never the timestamp, which refreshes on every successful lookup).
      Otherwise run the lookup. **Wakeup invariant:** after completing a
      lookup, each worker must re-check and drain `pending` again before
      blocking on the wakeup channel — the wakeup channel only wakes *idle*
      workers; progress while all workers are busy comes from the
      complete-then-recheck loop. This is the **complete-then-recheck**
      invariant's companion: IPs are never discarded because the queue is
      full, never stranded because every completion re-drains, and never
      silently skipped past a value the requester already saw.
    - **Context ownership:** workers and the trigger goroutine run on a
      resolver-owned context derived at `Start` and canceled by `Close()`.
      Request/service contexts are used only for the synchronous cache and
      settings reads inside `HostnamesForSessionIPs`; they never reach
      `net.LookupAddr` and never outlive their request.
    - In-flight set + interested-user set under one mutex. The interested
      set is keyed by canonical IP and maps
      `userID -> set[rawIP]` (every triggering session's stored spelling,
      so one user with two spellings of the same IP keeps both):
      `Start(ctx)` spawning workers + `Close()` joining them (idempotent,
      goleak-verified).
    - **Interested-set lifecycle (bounded, tested):** each job **snapshots
      and clears** its IP's interested pairs exactly once when it resolves
      OR is skipped, on every terminal path — publish, no-change
      (observed hostname equals current), dequeue-gate skip (no enabled
      users), cache-fresh observed-diff equal, non-`ErrNotFound` cache-read
      error, `Set` error, and nil event bus. Only interests merged into an
      already-running job (in-flight dedup) survive until that job's own
      terminal path. A user who triggers again after completion re-registers
      fresh. This prevents unbounded stale routing state under
      session/IP/user churn; add a churn/cleanup test that triggers,
      completes, re-triggers from many users/IPs and asserts the set never
      retains completed interests.
    - **Fire-and-forget trigger:** the resolver owns a buffered trigger
      queue plus a pending-user set (same no-drop pattern as the IP pending
      set); `TriggerUserSessionsResolved(userID string)` enqueues the user
      and returns immediately. A trigger goroutine (spawned by `Start`,
      joined by `Close`) drains the queue on the resolver-owned context:
      read the settings gate (fail closed on error), fetch the user's raw
      session IPs, and schedule lookups. The PATCH hook path never blocks on
      settings/session/cache reads and never inherits the request context.
    - `HostnamesForSessionIPs(ctx, userID string, ips []string)
      map[string]CacheEntry` — **exact contract:** keyed by the raw input
      spelling (the handler maps `m[session.IP]` directly with no
      canonicalization of its own), one entry per valid input IP
      (`Hostname` `""` and `ResolvedAt` nil when no hostname is known),
      invalid/empty input IPs absent from the map.
      The method **never returns an error and never fails the caller**:
      every degradation is logged internally — settings gate read error →
      empty map, no lookups (fail closed); cache `GetMany` error → empty
      entries for the affected IPs, no lookups scheduled for them (unknown
      baseline); per-IP invalid entries skipped. When the setting is **off
      or unreadable, the map is empty** (cached values must not leak and
      DNS must never run while opt-in is unknown). When on: validates each
      IP with `net.ParseIP` (skipping invalid), keeps the **raw spelling**
      for interested registration, canonicalizes (`ip.String()`) for
      cache/job keys, reads the cache with **one `GetMany` call**, returns
      cached hostnames with their `ResolvedAt`, and schedules lookups for
      IPs that are absent or stale. The synchronous portion is one settings
      read + one cache query (fast); scheduling is async.
    - On completion: canonical IP key; compare with the cache. Every
      **successful** lookup — valid records and `IsNotFound` negatives —
      **persists via `Set` (refreshing `resolved_at`) even when the hostname
      is unchanged**, so `recentResolveWindow` actually engages for
      no-answer and stable rows; a same-value success that skipped `Set`
      would leave a stale timestamp and re-query on every trigger. Publish
      `events.AuthSessionHostnameResolved` `{user_id, ip, hostname,
      resolved_at}` **only when the hostname value changed** (first
      resolution counts as changed), where the published `ip` is **each
      interested (user, raw spelling) pair** (one event per pair) and
      `resolved_at` is the persisted row's
      fixed 9-digit RFC3339 UTC time, formatted with the shared exported
      `hostnames.FormatTimestamp` helper (or `null` for a non-persisted
      transient result).
      **`ErrNotFound` is a successful absent baseline, not an error**: with
      an absent row a transient failure may publish `""` (nothing to mask)
      and a valid/`IsNotFound` result persists normally. Only
      **non-`ErrNotFound` cache errors** on completion skip persist +
      publish (baseline unknown). A `Set` error logs and publishes
      **nothing**: the value was not persisted, so changed-only semantics
      do not admit an event (publishing a transient value would outlive its
      own retry and make the frontend ordering inconsistent); the next
      trigger retries the persist, and a later successful `Set` that
      changes the cache publishes normally. `resolved_at: null` stays
      reserved for transient DNS errors streamed over an absent cache
      (empty-string events), never for failed writes.
      Neither ever panics or fails the caller.
      Result classification:
      - valid record(s): hostname from `NormalizeHostname` (may be `""` for
        generic answers);
      - `*net.DNSError` with `IsNotFound`: successful negative answer,
        `hostname = ""`, **persisted**;
      - other errors (timeout/temporary/server): `""` **not persisted**,
        and published **only when the IP has no cache entry** — a transient
        failure must never stream `""` over an existing cached value,
        because the client treats the streamed map as authoritative and a
        masking empty would pin the row to `N/A` until identity reset.
      A nil event bus is guarded (skip publish, keep persisting).
- `apps/backend/internal/auth/service.go`: add
  `SessionIPs(ctx, userID) ([]string, error)` — the user's session IPs as
  **raw stored spellings**: skip empty values, validate with `net.ParseIP`
  (skip invalid), dedupe exact string duplicates only. Canonicalization is
  the resolver's job (job/cache keys); raw spellings must survive for event
  payloads.
- `apps/backend/internal/events/types.go`: `AuthSessionHostnameResolved =
  "auth.session.hostname.resolved"`.
- `apps/backend/pkg/websocket/actions.go`: `ActionSessionHostnameResolved =
  "auth.session.hostname.resolved"`.
- `apps/backend/internal/gateway/websocket/user_notifications.go`: add
  `b.subscribe(eventBus, events.AuthSessionHostnameResolved,
  ws.ActionSessionHostnameResolved)`. The resolver publishes a
  `map[string]interface{}` with literal keys **`user_id`, `ip`, `hostname`,
  and `resolved_at`** (matching the map-shaped convention of
  `publishUserSettingsEvent`) so both memory and NATS transports route by
  `user_id`, the broadcaster strips it correctly, and the frontend receives
  the timestamp it needs for newer-wins reconciliation.
- `apps/backend/internal/auth/httpapi/handlers.go`:
  `RegisterRoutes(router, svc, resolver, log)`; `listSessions` builds items
  from `resolver.HostnamesForSessionIPs(ctx, userID, ips)` (raw-keyed, so
  no canonicalization in the handler; the method never returns an error):
  `"hostname": entry.Hostname` (or `""`) and
  `"hostname_resolved_at": <hostnames.FormatTimestamp(entry.ResolvedAt),
  or null>` when the entry
  has a `ResolvedAt`; resolver degradations are already logged inside the
  resolver and never fail the sessions response. Handler test: a
  cache/settings failure still returns HTTP 200 with `hostname: ""` and
  `hostname_resolved_at: null` per session, and a durable entry emits the
  exact 9-digit fixed-width string via the exported helper.
- `apps/backend/internal/user/service/service.go`: publish the complete
  persisted settings snapshot through the existing `UserSettingsUpdated`
  event. The service has no hostname-resolver dependency.
- `apps/backend/internal/backendapp`:
  - Construct the resolver in `startServices` right after
    `services.Auth` is set (all of `dbPool`, `repos`, `eventBus`, `log`,
    `ctx`, `addCleanup` are in scope): store over `dbPool` writer/reader,
    `SessionIPs` = `services.Auth`, settings gate = adapter over
    `repos.User.GetUserSettings(ctx, userID)`.
  - **Call `resolver.Start(ctx)` explicitly** after construction; on error,
    log and abort startup. Register `addCleanup(resolver.Close)` only after
    a successful `Start`.
  - Queue-subscribe `resolver.HandleUserSettingsUpdated` to
    `events.UserSettingsUpdated`. The shared queue prevents duplicate DNS work
    across backend replicas. Register the subscription cleanup after the
    resolver cleanup so shutdown unsubscribes before it closes the resolver.
  - Keep the event handler small and idempotent. It reads the persisted user
    ID and enabled flag, then calls `TriggerUserSessionsResolved`.
  - Thread the resolver to the routes: attach it to `Services`
    (`SessionHostnameResolver *hostnames.Resolver`; the `Services` struct is
    declared in **`apps/backend/internal/backendapp/types.go`**, not
    `services.go`), which is already passed through
    `startAgentInfrastructure` → `startGatewayAndServe` →
    `buildHTTPServer` → `registerRoutes`; `routeParams` reads it from
    `p.services.SessionHostnameResolver` (or add an explicit parameter to
    every signature in that chain — pick one, do not leave the plumbing
    implicit). Extend the `authhttpapi.RegisterRoutes` call in
    `registerRoutes` and update all call sites/tests.

## Files likely touched

- `apps/backend/internal/auth/hostnames/store.go`
- `apps/backend/internal/auth/hostnames/timestamp.go` (exported
  `FormatTimestamp`/`ParseTimestamp` + `timestampLayout`)
- `apps/backend/internal/auth/hostnames/store_test.go`
- `apps/backend/internal/auth/hostnames/normalize.go`
- `apps/backend/internal/auth/hostnames/normalize_test.go`
- `apps/backend/internal/auth/hostnames/resolver.go`
- `apps/backend/internal/auth/hostnames/resolver_test.go`
- `apps/backend/internal/auth/hostnames/goleak_test.go`
- `apps/backend/internal/auth/service.go`
- `apps/backend/internal/auth/service_test.go`
- `apps/backend/internal/auth/httpapi/handlers.go`
- `apps/backend/internal/auth/httpapi/handlers_test.go`
- `apps/backend/internal/gateway/websocket/user_notifications.go`
- `apps/backend/internal/gateway/websocket/user_notifications_test.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/backendapp/main.go` (or `auth.go`)
- `apps/backend/internal/backendapp/types.go` (Services struct field)
- `apps/backend/internal/backendapp/services.go` (provider wiring)
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/<provider or wiring test file>` (Start/cleanup/routing smoke test)

## Acceptance

1. `HostnamesForSessionIPs` returns cached values immediately (one `GetMany`
   call) and schedules lookups **only when the setting is confirmed on**;
   with the setting off **or unreadable** it returns an empty map, schedules
   nothing, and no cached hostname reaches the sessions response.
2. A changed resolution persists to `auth_hostname_cache` and publishes
   exactly one event per interested
   (user, raw-spelling)
   pair — including two sessions of one user with different spellings of the
   same IP; unchanged hostname resolutions publish nothing. **The internal
   event-bus map is `{user_id, ip, hostname, resolved_at}`** (the
   `UserEventBroadcaster` routes by `user_id` and strips it); the WS payload
   delivered to the client is `{ip, hostname, resolved_at}`. Gateway/wiring
   tests must assert the exact internal map (with `user_id`) is what the
   broadcaster receives. Every successful
   lookup (including same-value and `IsNotFound`) refreshes `resolved_at`
   via `Set`, so `recentResolveWindow` engages for stable and no-answer
   rows.
3. DNS result classification: `IsNotFound` (NXDOMAIN/no PTR) persists as a
   cached no-answer (`""` with `resolved_at`) and is skipped by
   `recentResolveWindow`; timeout/temporary/server errors are never
   persisted and are published `""` only for IPs with no cache entry — a
   stale cache plus transient error publishes nothing, so the durable value
   survives re-enable and list refreshes.
4. `TriggerUserSessionsResolved(userID)` returns immediately (PATCH latency
   unaffected by settings/session/cache reads), runs on the resolver-owned
   context, and `Close()` joins the trigger goroutine and workers (goleak).
5. Admission never drops an IP: `N` distinct IPs with `N > lookupWorkers`
   all resolve; rapid re-triggers dedupe via in-flight +
   `recentResolveWindow` (driven by `CacheEntry.ResolvedAt`); equivalent IP
   spellings share one cache row and one job; absent and cached-empty IPs
   are treated distinctly (absent schedules, fresh cached-empty does not).
6. Request-context cancellation does not cancel lookups; `Close()` joins
   workers with no goroutine leaks (goleak).
7. Invalid IPs are skipped; cache read/write errors and a nil event bus
   follow the spec Failure modes (degrade, never fail the sessions API or
   panic). A `Set` error publishes nothing (changed-only semantics; the next
   trigger retries the persist, and a later successful `Set` that changes
   the cache publishes). `CacheEntry.ResolvedAt`
   is `*time.Time`: nil = unknown (handler emits JSON `null`), non-nil
   covers both cached-no-answer (`Hostname == ""`) and durable values.
   `resolved_at: null` in events is reserved for transient DNS errors over
   an absent cache (empty strings), never for failed writes.
8. The resolver is started in production wiring: `Start(ctx)` is called
   after construction (abort on error), `Close` is registered as cleanup,
   and the resolver reaches `registerRoutes`/`RegisterRoutes` through the
   startup chain; a backendapp wiring smoke test asserts Start/cleanup/
   route-threading (unit tests that call `Start` themselves are not
   sufficient evidence).
9. `GET /api/v1/auth/sessions` includes `hostname` and
   `hostname_resolved_at` (fixed 9-digit RFC3339 UTC or `null`) for every
   session, and both
   are empty/null while the toggle is off; non-canonical stored spellings
   still resolve (canonical lookup key, raw display key), including when the
   toggle hook fires before any sessions GET.
10. PATCH `resolve_session_hostnames: true` triggers
    `TriggerUserSessionsResolved` for the user; the PATCH response is
    unaffected by lookup results.
11. The WS gateway routes `auth.session.hostname.resolved` (`{ip, hostname,
    resolved_at}`) to the user in `user_id`, stripped from the payload, for
    both memory and NATS-shaped deliveries (gateway tests added).
12. The `auth_hostname_cache` schema is replay-safe: fresh DB and
    same-DB re-run of `initSchema` both succeed, including Postgres-gated
    fresh + replay coverage (the store is wired over the production
    `dbPool`); `resolved_at` fixed 9-digit RFC3339 UTC text round trips through both
    dialects.

## Verification

```sh
(cd apps/backend && go test -race ./internal/auth/... ./internal/user/... ./internal/gateway/websocket/... ./internal/backendapp/... ./pkg/websocket/...)
make -C apps/backend lint
git diff --check
```

## Dependencies

Task 01 (the resolver reads `resolve_session_hostnames`).

## Risks

- Publishing an event per interested user for a shared IP is required;
  publishing only to the triggering user drops updates for concurrent users,
  and the event `ip` must be each (user, raw-spelling) pair or the frontend
  row match breaks. A `map[userID]rawIP` interested set silently drops a
  user's second spelling of the same IP — use a per-user set of raw
  spellings.
- A resolver that drops jobs when busy strands IPs with no later trigger:
  use a pending set drained by workers, never a drop-on-full queue — and
  workers must re-drain `pending` after every completion (the wakeup channel
  only wakes idle workers), or excess IPs strand while all workers are busy.
- Storing `resolved_at` in a dialect-typed column and scanning it directly
  into `time.Time` is not portable across the SQLite/Postgres `db.Pool`;
  store fixed 9-digit RFC3339 UTC text (one shared formatter) and parse
  explicitly, with required Postgres fresh+replay coverage.
- Lookups on a request context die with the HTTP response: all DNS work runs
  on the resolver-owned context canceled by `Close`.
- Uncached `hostname` values can leak while the toggle is off: gate the
  returned map, not just scheduling.
- Publishing transient `""` over an existing cache entry masks the durable
  value in the client (the streamed map is authoritative there); transient
  errors publish `""` only for IPs with no cache entry.
- An explicit `Start(ctx)` is easy to omit in production wiring; unit tests
  that call Start themselves will pass while the real server never drains
  pending lookups. Pin the production Start + cleanup registration in the
  wiring change.
- A per-IP cache read per session row turns the sessions list into an N+1
  path: use `GetMany`.
- A cache interface that cannot distinguish absent from cached-empty, or
  lacks `ResolvedAt`, makes `recentResolveWindow` and no-answer caching
  unimplementable: reads must return `CacheEntry{Hostname, ResolvedAt}` and
  absent rows must be absent from the result map.
- Canonicalizing in `SessionIPs` (the toggle-hook provider) loses the raw
  stored spelling, so a hook-triggered completion publishes a canonical `ip`
  the frontend cannot match: the provider returns raw validated spellings;
  canonicalization stays inside the resolver.
- Resolver tests cannot inject cache failures against a concrete store: the
  resolver must depend on the narrow `Cache` interface.
- A struct-typed event without a literal `user_id` key is unroutable through
  NATS: publish a `map[string]interface{}` with literal keys and pin memory +
  NATS-shaped gateway tests.
- `goleak` will fail if `Start`/`Close` are not idempotent and joined;
  follow the healthpoll Start/Stop shape.
- A new table that only works on a fresh DB fails the repo's replay rule:
  cover fresh + same-DB `initSchema` replay in the store tests.

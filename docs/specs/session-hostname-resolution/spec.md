---
slug: session-hostname-resolution
title: Optional reverse-DNS hostnames for account sessions
status: draft
---

# Optional reverse-DNS hostnames for account sessions

## Problem

The account security page (`/settings/account/security`) lists signed-in
devices with their IP address. IPs are opaque; users often want to know which
hostname an IP belongs to (their own machines, a corporate proxy, a cloud
host) without leaving the page. Resolving every session IP unconditionally
would leak per-IP DNS traffic and column width for users who do not need it,
so the feature is an opt-in toggle, off by default.

## Goal

- A per-user toggle **Resolve device hostnames** on the account security page,
  disabled by default.
- When enabled, the sessions table gains a **Hostname** column. The backend
  reverse-resolves each session IP through the OS-configured DNS resolver
  (never an external API or hardcoded resolver) in the background and streams
  results to the page as they complete.
- IPs with no PTR answer, or with only a synthesized generic
  `<reversed-ip>.in-addr.arpa` / `.ip6.arpa` name, display **N/A**.
- Disabling hides the column. Re-enabling restores it immediately with cached
  results, then the backend re-runs the lookups and streams only values that
  changed since the cache.

## Non-goals

- No DNS lookups happen while the toggle is off. Hostname data is rendered
  **only** on the account security page (no other UI surface reads or shows
  it). Transport note: streamed notifications are delivered to the
  authenticated user's own WebSocket connections (all their tabs), because
  the gateway fan-outs per user; other tabs retain the in-memory map but
  never render it, which is what makes re-enable and cross-tab navigation
  show cached results instantly. A page-scoped subscription that delivers
  only to the security page is a possible future narrowing, not part of
  this change.
- No external DNS-over-HTTPS, geolocation, or enrichment APIs. Only the OS
  resolver (`net.LookupAddr`) is used.
- No change to how sessions are created, stored, or revoked.
- No per-session hostname persistence: the cache is keyed by IP (reverse DNS
  is an IP property, identical for every session that shares it).

## User stories

1. Alice signs in from `192.0.2.10`. On the security page she enables the
   toggle. The Hostname column appears; the row shows `N/A` (the empty
   value) until the lookup completes, then the hostname replaces it without
   a reload.
2. Bob enables the toggle while one of his devices is behind a NAT that has
   no PTR record. The row shows `N/A`.
3. Alice disables the toggle. The column disappears. She enables it again
   five minutes later: the column reappears with the previously resolved
   hostnames immediately, then re-resolves in the background; only rows whose
   hostname changed update.
4. Carol signs in from a new IP while the toggle is already on. The next
   time the security page's session list refreshes (window focus returns or
   the page reloads) the new session's row appears and its hostname resolves
   in the background and streams into the row. Live discovery of sessions
   created elsewhere while the page stays focused is out of scope (no
   session-created push event).

## Contracts

### User setting

- Wire field: `resolve_session_hostnames: boolean` (top-level user settings,
  snake_case on the wire).
- Frontend state: `userSettings.resolveSessionHostnames: boolean`.
- Default: `false` for missing stored values and initial payloads.
- PATCH semantics: omitted field preserves the current value; explicit
  `true`/`false` is durable. Changing it to `true` triggers background
  resolution for the user's session IPs (fire-and-forget; the PATCH response
  is unaffected by lookup results).
- The setting is broadcast in the complete `user.settings.updated` WebSocket
  event so other tabs and the boot hydration path stay in sync through the
  existing shared mapper.

### Sessions API

`GET /api/v1/auth/sessions` (unchanged auth/permission requirements) adds two
fields per session:

```json
{ "id": "...", "ip": "192.0.2.10", "hostname": "mail.example.com", "hostname_resolved_at": "2026-08-16T12:00:00.123456789Z", ... }
```

- `hostname` is always present: the cached value when known, `""` when
  unknown or when the toggle is off. When the toggle is off the resolution
  path is gated end to end: no lookups are scheduled **and** the returned
  map is empty, so no cached hostname ever leaks into the response.
- `hostname_resolved_at` is the cache row's `resolved_at` when a cache row
  exists, `null` otherwise. It is formatted with the **same fixed 9-digit
  fractional-second RFC3339 UTC layout used everywhere**
  (`2006-01-02T15:04:05.000000000Z07:00`, e.g.
  `"2026-08-16T12:00:00.123456789Z"`), so the field is lexicographically
  comparable in chronological order and lets the frontend order list
  snapshots against streamed events (see Frontend).
- A background resolution pass is scheduled for any of the user's session IPs
  that are not cached, not in flight, and not resolved in the last
  `recentResolveWindow`, but only when the toggle is on.

### Reverse-DNS cache (backend)

- New SQLite table `auth_hostname_cache`, owned by the hostnames cache store
  (`apps/backend/internal/auth/hostnames/store.go`), which runs its own
  `CREATE TABLE IF NOT EXISTS` in its `initSchema` (a new table needs no
  `ADD COLUMN` migration); it is the sole consumer of the table:
  `ip TEXT PRIMARY KEY, hostname TEXT NOT NULL DEFAULT '', resolved_at TEXT NOT NULL`.
- Global by IP (PTR answers do not vary per user). `hostname == ""` means
  "no answer" and is itself cached so re-enables do not re-spin a spinner;
  **cache presence** (a row exists) is what distinguishes "resolved to no
  answer" from "never resolved", and `resolved_at` drives the
  `recentResolveWindow` re-query guard.
- Cache rows never expire for display. Re-resolution happens when the toggle
  is turned on and when new IPs appear on the sessions list.

### Lookup pipeline

- Session IPs are validated with `net.ParseIP` and canonicalized
  (`ip.String()`: v4-mapped IPv6 becomes dotted-quad, IPv6 is lowercased)
  before they become cache keys, job keys, or `net.LookupAddr` inputs.
  Invalid or empty IPs are skipped entirely.
- `net.LookupAddr` (OS resolver) per IP, using a **resolver-owned context**
  canceled by `Close()`, never the triggering request's context (which ends
  when the HTTP response completes). Request/service contexts are used only
  for synchronous cache and settings reads.
- Records are examined in resolver order; the **first record that normalizes
  to a valid hostname** is used (see normalization below). If none does, the
  IP has no answer.
- Normalization: trim trailing dot; a name that is empty or ends in
  `.in-addr.arpa` / `.ip6.arpa` (case-insensitive) is not a valid hostname.
- Bounded background workers (constant `lookupWorkers = 4`) process a
  **pending set** that workers drain; admission never drops an IP. Pending
  entries carry the **observed cache value** the trigger read. One job per
  IP; in-flight IPs are deduplicated; a second trigger for the same IP
  records the requesting user as interested instead of starting a second
  lookup. Immediately before calling `LookupAddr`, the worker re-reads the
  settings gate for every interested user (fail closed on error) and drops
  users who disabled the toggle since admission; if no interested user
  remains enabled, the job is skipped — **DNS never runs while the toggle is
  off**, even for work admitted while it was on. When a worker finds an IP
  already fresh within `recentResolveWindow`, it skips the DNS query but
  **compares the current cache value against the observed value and
  publishes the current value to the job's interested users when they
  differs** — a user who scheduled from a stale snapshot must still learn of
  the change since their read (a read → concurrent completion → enqueue
  interleaving must never silently drop the update). The diff compares the
  cached **hostname** (not the timestamp, which now refreshes on every
  successful lookup).
- Per-lookup timeout (`lookupTimeout = 3s`). Every **successful** lookup —
  valid records, and `*net.DNSError` with `IsNotFound` (NXDOMAIN, no PTR
  records) as a successful negative answer — **persists its result and
  refreshes `resolved_at` even when the hostname is unchanged**, so the
  `recentResolveWindow` re-query guard actually engages (a stale row that
  never refreshes its timestamp would be re-queried on every trigger).
  Publishing stays changed-only (hostname value differs). DNS errors are
  classified:
  - `IsNotFound` → `hostname = ""`, persisted as a cached no-answer;
  - timeout, temporary, and server errors are treated as no answer for the
    UI but are **not persisted**. They are streamed as `""` **only when the
    IP has no cache entry at all**; when a durable cache entry exists
    (hostname or no-answer), a transient failure is logged and **nothing is
    published**, so a transient blip can never mask an existing cached
    hostname in the client (the durable value stands until a real change or
    the next successful lookup).
- `recentResolveWindow = 30s`: an IP resolved within this window is not
  re-queried on a new trigger (anti-thrash for rapid toggle cycles; the OS
  resolver's own cache makes repeated lookups cheap but unnecessary).

### Streaming

- New event `auth.session.hostname.resolved` on the event bus, payload
  `{user_id, ip, hostname, resolved_at}` (fixed 9-digit RFC3339 UTC string
  for a persisted result, `null` for a non-persisted transient result),
  published **only when the resolved hostname value differs
  from the cache** (a first resolution with an empty cache counts as a
  change; a failed lookup that yields `""` against an absent cache also
  counts as a change). `resolved_at` travels with the event so the gateway
  and frontend can preserve newer-wins ordering; dropping it breaks
  reconciliation.
- **Key contract:** cache keys and job keys are the canonical IP
  (`net.ParseIP(...).String()`); the event's `ip` is the **raw stored session
  IP of the triggering user's session** (the same string the frontend already
  holds as `AuthSession.ip`), so the frontend matches events to rows without
  any IP parsing. When several users — or one user with several sessions —
  trigger the same canonical IP with different raw spellings, each
  (user, raw spelling) pair receives an event carrying that raw spelling.
  The resolver canonicalizes internally for job/cache keys but returns
  **raw-input-keyed** results: the sessions handler looks up
  `m[session.IP]` directly with no canonicalization of its own, so a
  non-canonical stored spelling (e.g. `::ffff:192.0.2.10`) still finds its
  cached hostname.
- New WebSocket notification action `auth.session.hostname.resolved`, payload
  `{ip, hostname, resolved_at}` (the gateway strips `user_id`), routed to
  every user that triggered a lookup for that IP (a user-triggered lookup
  registers the user as interested with their raw IP spelling; all interested
  users receive the update). `resolved_at` is the RFC3339 UTC time the
  result was persisted with a **fixed 9-digit fractional-second field**
  (e.g. `2026-08-16T12:00:00.123456789Z`, lexicographically comparable in
  chronological order), or `null` **only for a transient DNS
  error/timeout streamed over an IP with no cache entry** (an empty-string
  event with nothing durable to mask). A failed cache write publishes
  nothing, so `null` never denotes a failed-write hostname.
- The frontend merges streamed values into a per-IP map keyed by the raw
  session IP and re-renders matching rows.

### Frontend

- Toggle card on `/settings/account/security`: label **Resolve device
  hostnames**, description stating that lookups use the system DNS resolver
  and apply to the IPs of signed-in devices. Saves immediately through
  `PATCH /api/v1/user/settings` (page-local save, consistent with the rest of
  the account page).
- Sessions table: the **Hostname** column renders only while the toggle is
  on. Cell value resolution compares the streamed map entry
  (`{hostname, resolvedAt}` — camelCase frontend state field; the wire
  field `resolved_at` is reserved for the WS payload/API) with the
  session's response entry (`hostname`, `hostname_resolved_at`): **newer
  `resolvedAt` wins; a
  durable entry (non-null `resolvedAt`) beats a transient/unknown one
  (null); ties break to the non-empty value; both empty → `N/A`**. This is
  what makes list snapshots authoritative over stale streamed entries after
  a WebSocket gap (the cache's `resolved_at` is newer than the retained map
  value) while still letting a fresh streamed event beat an in-flight stale
  response. There is **no pending spinner state**: an empty value is
  indistinguishable from a cached no-answer, so it always renders `N/A`, and
  a streamed event replaces it as soon as the lookup completes (an
  unchanged cached no-answer row renders `N/A` until the row's freshness
  expires and a retry finds a valid answer — see `recentResolveWindow`; it
  does not stay `N/A` forever by construction). The streamed map survives
  disable/enable cycles in the client store, which is what makes re-enable
  show cached results before the backend re-query streams deltas.
- The sessions list refreshes when the window regains focus (and on mount /
  manual reload), so sessions created elsewhere appear without a full reload;
  their IPs are then resolved by the existing list-fetch trigger. No
  session-created push event is added.
- When the toggle transitions to on — same tab or another tab (the
  `user.settings.updated` push updates the store) — the sessions list is
  refetched once. This is what renders backend-cached hostnames when the
  list was last loaded while the toggle was off (those responses carried
  `hostname: ""`), and it also discovers sessions created meanwhile.
- Identity policy for the in-memory streamed map: it survives disable/enable
  cycles and same-identity refreshes, and is **cleared when the
  authenticated identity changes** — logout, a different user logging in, or
  the 401/session-expiry path (`clearAuthenticated`) — so one user's session
  IPs never seed another user's rows. Because the gateway strips `user_id`
  from streamed events, the stream is fenced on identity transitions: a
  handler-generation epoch guard ignores events from a prior identity, and
  the WS client is recreated on identity change so transport-queued frames
  are dropped — a delayed old-user event can never repopulate the cleared
  map.
- `N/A` is localized copy (`account:hostnameNotAvailable`); all new copy goes
  through `t()` and every locale catalog including pseudo.

### Mobile

- The toggle row is a standard 44 px+ touch target; the sessions table remains
  the sole internal scroll owner with no document-level horizontal overflow
  when the Hostname column is added.
- The account security page is reachable on the Pixel 5 path; a
  `mobile-*.spec.ts` in the `mobile-chrome` project covers enable/disable,
  column presence, and N/A/cached rendering (same env-robust assertions as
  the desktop spec).

## Permissions

- The toggle lives in per-user settings; `PATCH /api/v1/user/settings` is
  already identity-scoped.
- The sessions endpoint and the new streaming action are user-scoped: the
  gateway routes `auth.session.hostname.resolved` with `Hub.BroadcastToUser`
  to the interested user's own connections only; the payload never contains a
  `user_id`.
- The cache is populated only from the user's own session IPs; no endpoint
  exposes the cache directly.

## State machine

- Toggle off: no lookups, no column, sessions response carries `hostname:
  ""`.
- Toggle on: sessions list triggers a resolution pass; per-IP jobs run once
  (dedup by in-flight + `recentResolveWindow`); each changed result is
  streamed; the column renders cached + streamed values.
- Toggle off: lookups already in flight complete and update the cache, but
  their events are only sent to interested users; the column hides.
- Toggle on again: cached values render immediately from the client store and
  the sessions response; a new resolution pass streams only deltas.

## Failure modes

- Settings-gate read error: the resolver cannot confirm the toggle is on, so
  it **fails closed** — returns an empty hostname map, schedules no lookups,
  and logs. DNS queries must never run while opt-in is unknown.
- DNS timeout/temporary/server error: cell shows `N/A` only when nothing
  durable is known; nothing persisted; streamed `""` only when the IP has no
  cache entry, never over an existing cached value; the next trigger (new
  session list load or toggle) retries. A `*net.DNSError` with `IsNotFound`
  is NOT an error: it is a successful negative answer persisted as
  `hostname = ""` (cached no-answer).
- Cache read error on the sessions API path: the affected IPs return `""`
  (no hostname), no lookups are scheduled for them (baseline unknown), the
  request is not failed, and the error is logged.
- Cache read error on lookup completion: the baseline is unknown, so the
  result is neither persisted nor published (a change cannot be verified);
  the error is logged and the next trigger retries.
- Cache write error on lookup completion: logged; **nothing is published**
  — the value was not persisted, so changed-only semantics do not admit an
  event (a transient publish could outlive its own retry and the frontend
  ordering would treat it inconsistently). The next trigger retries the
  persist; a later successful `Set` that changes the cache publishes
  normally. `resolved_at: null` remains reserved for transient DNS
  errors streamed over an absent cache (empty-string events), never for
  failed writes.
- Event bus unavailable (nil): no stream; cached values still flow through
  the sessions response.
- Invalid/empty session IP (e.g. sessions recorded before IP capture):
  skipped, cell shows `N/A`.
- Multi-user instance: user A's lookup for an IP is shared with user B only
  when B also triggered a lookup for that same IP (B is registered as
  interested, with B's raw IP spelling); the cache itself is shared by IP,
  which is safe because PTR answers are not user-specific.

## Persistence guarantees

- The toggle is durable in the user settings JSON blob (default false).
- The hostname cache is durable in `auth_hostname_cache` and survives backend
  restarts, which is what makes "re-enable shows cached results" hold even
  after a restart.
- The frontend streamed map is in-memory only; a page reload re-hydrates from
  the sessions response (backend cache) and the next lookup pass.

## Data model

```sql
CREATE TABLE IF NOT EXISTS auth_hostname_cache (
    ip          TEXT PRIMARY KEY,           -- canonical IP literal
    hostname    TEXT NOT NULL DEFAULT '',   -- "" = no answer (N/A)
    resolved_at TEXT NOT NULL               -- RFC3339 UTC text; drives recentResolveWindow
);
```

`resolved_at` is stored as RFC3339 UTC **text with a fixed 9-digit
fractional-second field** (dialect-portable across the SQLite/Postgres
`db.Pool`) and parsed explicitly on read; fixed width keeps the strings
lexicographically comparable, so changes within the same second — or the
same millisecond — still order correctly.

## API surface

- `PATCH /api/v1/user/settings` accepts `resolve_session_hostnames: boolean`.
- `GET /api/v1/auth/sessions` per-session response gains `hostname: string`
  and `hostname_resolved_at: string | null`.
- WS notification `auth.session.hostname.resolved`: `{ip, hostname,
  resolved_at}`.
- No new HTTP endpoints.

## Acceptance criteria

1. Toggle defaults to off for new users, existing settings blobs, and
   initial boot payloads; toggling persists and survives reload.
2. Enabling shows the Hostname column immediately (cached values or `N/A`),
   and streamed results fill in rows without a reload; there is no pending
   spinner state (empty always renders `N/A`). Enabling also refetches the
   sessions list so backend-cached hostnames render even when the list was
   last loaded while the toggle was off (same tab or another tab).
3. Disabling hides the column; re-enabling within the same session shows
   cached values immediately and streams only changed values.
4. Sessions created elsewhere appear on the next list refresh (window
   focus, the enable-transition refetch, or reload) and their hostnames then
   resolve and stream into the new rows.
5. The in-memory hostname map survives disable/enable and same-identity
   refreshes and is cleared when the authenticated identity changes
   (logout, different user, or the 401/`clearAuthenticated` path).
6. No-answer IPs and generic `.in-addr.arpa` / `.ip6.arpa` answers render
   `N/A`.
7. Lookups use the OS resolver only: the lookup function is injectable and
   defaults to `net.DefaultResolver.LookupAddr`; no HTTP/API calls exist in
   the lookup path.
8. `GET /api/v1/auth/sessions` includes `hostname` (cached value or `""`)
   without auth-mode changes.
9. Streaming events are routed to the triggering user's own connections only
   and never include `user_id`.
10. Streamed event `ip` values match the raw session IP spelling the
    frontend already holds, so rows update without client-side IP parsing;
    the sessions handler looks up cached hostnames by the raw stored
    spelling (the resolver canonicalizes internally), so non-canonical
    spellings still resolve, and equivalent IP spellings still share one
    cache row and one lookup job.
11. Cache read/write failures degrade per the Failure modes section without
    failing the sessions API or panicking (nil event bus included).
12. A transient DNS failure never masks a durable cached hostname: the
    backend publishes transient `""` only for IPs with no cache entry, and
    the frontend compares `resolved_at` (durable beats transient; newer
    wins), so a stale-cache + transient-error + re-enable cycle still
    restores the cached value.
13. After a WebSocket gap, a newer sessions response reconciles the retained
    streamed map: an entry whose cache `resolved_at` is newer than the
    retained map value replaces it (unit regression: event → disconnect →
    list refresh), and a fresh streamed event still beats an in-flight stale
    response. `resolved_at` uses fractional-second precision so two changes
    within one second still order correctly.
14. A user who scheduled resolution from a stale snapshot still receives the
    current cache value when it changed since their read (admission
    observed-value diff), so a read → concurrent completion → enqueue
    interleaving never silently drops the streamed update.
15. New frontend copy is localized (all catalogs + pseudo) and passes
    `i18n:ratchet`.

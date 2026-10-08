---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
created: 2026-10-05
updated: 2026-10-07
owners:
  - kandev
---

# Disk Usage Query Cache

## Purpose and boundaries

This design defines the frontend ownership and recovery path for the disk
snapshot covered by [System pages](../requirements/system-page.md). The
system-page system owns the disk-usage behavior and its endpoint lifecycle.
Platform owns the shared QueryClient and identity helper used by the web app;
the [SystemInfo Query design](../../platform/system-design/system-info-query-cache.md)
describes the current pilot. The
[disk-usage backend design](system-page-01.md#disk-usage-cache) remains the
source for server cache and walk behavior.

This is a behavior-preserving state migration. It does not change the HTTP
contract, disk walk, system-job protocol, System Status layout, or responsive
composition.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-SYSTEM-PAGE-001`, `AC-SYSTEM-PAGE-SYSTEM-PAGE-001.2` | [Control flow](#control-flow) and [Freshness and recovery](#freshness-and-recovery) |

## Components and responsibilities

- `fetchDiskUsage` and `refreshDiskUsage` in `apps/web/lib/api/domains/system-api.ts`
  remain the HTTP transport for the existing GET and POST endpoints. GET uses
  `cache: "no-store"` and passes Query's `AbortSignal` through
  `ApiRequestOptions.init.signal`.
- The stable authenticated-app QueryClient owns the disk response and GET
  request, loading, and error lifecycle. `useDiskUsage` reads and refreshes
  this cache; it does not copy the response to Zustand. Separate refresh POST
  feedback remains local mutation state while preserving the current card error
  presentation.
- The central `system.job.update` handler and `system.jobs` map continue to
  own the system-job stream. A single disk-specific bridge observes that map
  and translates terminal `disk-walk` job changes into invalidation of the
  current disk query. It is not a general event bus and does not move jobs into
  Query.
- `DiskUsageCard` keeps its current refresh action, error, loading, and data
  presentation. Desktop and phone use the same card and interaction order; no
  viewport-dependent behavior changes.
- `DiskUsageService` and the shared jobs tracker remain the backend owners of
  the cache and walk. The service coalesces refresh calls while a disk walk is
  active and publishes the existing terminal job update.

## Data and identity

The query key is resource-specific and includes the shared identity fields:
normalized full backend API URL, page `bootId`, auth mode, authenticated
status, and user ID. It is not workspace-scoped. The implementation uses the
identity helper reviewed and landed by QUERY-03 rather than copying its
normalization or creating a disk-only identity model. Only the disk-usage query
prefix is eligible for obsolete-identity cancellation and removal.

The request returns the existing `DiskUsageResponse`:

- `data` is the cached breakdown or `null` while no result exists.
- `computing` reports an active or stale refresh.
- `home_dir` remains available to the existing card.

The existing `system.jobs` map is retained across authentication clearing and
system hydration. `createAppStore` can start with jobs from its initial state;
`hydrateState` deep-merges supplied job entries and does not clear absent IDs;
`clearAuthenticated` and `setAuthState` do not clear the map. The store exposes
`clearSystemJob`, but it has no production caller. The bridge reads the current
map before subscribing and treats those retained rows as its baseline. It
rebuilds that baseline on a full identity change or bridge remount instead of
replaying existing terminal rows into the new identity. The current runtime
has no production path that replaces or prunes the complete jobs map after
store construction. If a future path
replaces the map while the bridge is mounted, it must give the bridge a new
baseline before subsequent events are processed; an arbitrary replacement is
not treated as a sequence of terminal job events.

Deduplication state is a FIFO of at most 64 terminal disk-walk job IDs for the
current full query identity. It is reset on identity change and seeded from up
to 64 retained terminal rows in current map enumeration order when the bridge
mounts or establishes a new baseline. A repeated terminal update whose
previous Zustand row is already terminal is also ignored, including after its
ID leaves the FIFO. This transition check prevents a replayed retained row
from causing duplicate reads without extending the deduplication history
beyond its bound. After an ID leaves the FIFO, row removal or a nonterminal
replay can erase the store's terminal evidence. A later terminal replay then
cannot be distinguished from a new completion and can revalidate. The current
production store does not prune the complete job map, but it upserts full rows
without rejecting state regression. Success and failure both revalidate the exact
current resource key. Other job kinds do not invalidate it.

The bridge subscribes synchronously to the owning Zustand store. React render
batching must not hide a terminal update followed immediately by a running
replay. The baseline and subscription use the same store and lifecycle
generation. The bridge owns only its bounded FIFO and subscription metadata,
without a second retained job map. Hydration before subscription belongs to
the baseline. Later deep-merged additions follow the same transition rules as
other store updates because rows contain no delivery-source marker.

For a burst with N accepted terminal updates, at most N replacement GETs start
in addition to any GET already pending. Duplicate, retained-terminal, and
unrelated updates add zero GETs. Coalescing is allowed only if the final read
starts after the last accepted terminal update. A GET that predates that
update cannot satisfy recovery. The 64-ID FIFO remains bounded during bursts.

The backend API URL is resolved from the page's startup environment or current
origin and has no runtime setter. `WebSocketConnector` captures its URL once
per page; the WebSocket effect recreates on user-ID changes, not on auth-mode
or authenticated-status changes alone, and replaced sockets reject late
frames. Reconnect re-subscribes existing streams but does not replay system
jobs. This design does not expand socket lifecycle behavior. If backend
configuration becomes mutable within a page, the event source must be
re-audited before that change is supported.

## Control flow

1. When the disk query has no snapshot, `useDiskUsage` performs the existing
   GET. A cold backend response starts the lazy walk and returns
   `data: null, computing: true`.
2. An explicit **Refresh** keeps the existing sequence: POST to the refresh
   endpoint, then GET the authoritative response. The backend coalesces a
   concurrent refresh onto the active disk-walk job. If the GET says
   `computing=true`, periodic reads continue until a successful response says
   otherwise.
3. The bridge watches job-map changes, filters `kind="disk-walk"`, and handles
   only terminal `succeeded` or `failed` updates. It cancels and invalidates
   only the exact query key captured for the current identity so the result is
   read again after either terminal outcome. If cancellation has an
   asynchronous continuation, it checks that the captured identity and bridge
   baseline/subscription generation remain current before invalidating. A
   continuation from an obsolete A subscription cannot act on a new A
   subscription after an A→B→A transition.
4. The bridge replaces an in-flight first GET with no cached data as well as a
   cached-data refetch by calling exact-key `cancelQueries` followed by
   `invalidateQueries`. Deferred-response tests against the installed TanStack
   Query version prove that the older result cannot clear or overtake the
   post-event read. The GET transport consumes Query's native signal; no custom
   request coordinator is needed.
5. The identity provider keeps the QueryClient and app shell stable. On an
   identity change it cancels and immediately removes only obsolete
   disk-usage query entries. A query for identity A cannot render as B; an
   A-to-B-to-A transition creates or fetches A's current entry without a
   delayed cleanup deleting it.
6. Refresh captures its exact key and hook lifecycle generation before POST.
   After POST settles, it verifies both before GET or local feedback changes.
   An obsolete successful POST cannot start a GET, including after A→B→A or
   hook unmount/remount. Cancellation of a POST does not undo a server walk.

## Freshness and recovery

The following options are explicit so Query defaults do not add network
triggers that the existing hook did not have:

| Option | Value | Reason |
| --- | --- | --- |
| `staleTime` | `Infinity` | Matches the existing in-memory Zustand snapshot, which is fetched when absent and otherwise remains cached until explicit refresh, a terminal job update, or conditional computing recovery. This choice is based on that behavior, not copied from SystemInfo. |
| `gcTime` | `Infinity` | Retains the snapshot across System Status route unmounts, as the Zustand value does. |
| `refetchOnMount` | `true` | Fetches when there is no data or when a live event marked the cached query invalid; a fresh cached snapshot alone does not trigger a new read. |
| `refetchOnWindowFocus` | `false` | The current hook has no focus-triggered read. |
| `refetchOnReconnect` | `false` | The current hook has no reconnect-triggered read. |
| `networkMode` | `"always"` | The current direct fetch attempts immediately when offline and surfaces a fetch error instead of waiting for connectivity. |
| `retry` | `false` | The current hook has no automatic retry for a failed request. |
| `refetchInterval` | `1500` ms only while the latest query data says `computing=true`; otherwise disabled | Preserves the conditional recovery interval. It is periodic and has no bounded attempt count. |
| `refetchIntervalInBackground` | `true` | Preserves the mounted hook's interval while the page is backgrounded, subject to browser timer throttling. |

The interval recovers a completion event missed after the client has observed
`computing=true`, including a cold first GET and the GET following explicit
refresh. It stops after a successful response reports `computing=false`, or
when the bridge/query observer unmounts or its identity is removed. If a
refetch fails while cached data still says `computing=true`, that data remains
the interval condition and another GET is attempted at the next 1.5-second
tick; retries are not separately enabled. If the entire job and terminal event
are missed while the cached response says `computing=false`, this interval
cannot discover it. Reconnection does not promise system-job replay or add a
new unconditional disk refresh. A fresh query, explicit refresh, or observed
terminal job event remains authoritative.

The bridge remains mounted with the authenticated app provider so an event can
mark an inactive disk query invalid without issuing an inactive background
read. The next mount refetches that invalidated query. If the job event is
lost while the socket is disconnected, no new reconnect request is added.

No query exists until a disk consumer mounts. Terminal events before that
mount do not create queries or issue GETs. For an existing inactive query,
cancellation must preserve last-good data and leave the query invalidated.
Tests cover leaving the route between cancellation and invalidation, then
returning. They also cover StrictMode subscription replay with a deferred GET
and cancellation continuation. After replay, an accepted terminal update still
leads to an authoritative read. Subscription cleanup removes every listener
and prevents obsolete continuations from operating on a replacement query.

## Failure and security

GET failures remain visible through the existing card error row. Query keeps a
previous snapshot available alongside a refetch error, matching the current
hook's retained Zustand value. `isLoading` represents every active GET,
including background, manual, and event-triggered reads, rather than only an
initial pending request. `reload` and `refresh` continue to resolve after
handling failures. The refresh POST error remains visible in the same card
error row through separate local mutation feedback; a late POST result from an
obsolete identity cannot set or clear current-identity feedback. Logout
unmounts the authenticated Query provider; Query cancellation reaches the
fetch transport through its native signal. Backend, auth, and boot identity
changes cannot read data under a different key. No workspace or user-facing
copy is added.

The card exposes one error value. A new refresh clears prior POST feedback
and hides a previous GET error while POST is pending.
Each GET attempt also clears prior POST feedback and hides a previous GET
error while that GET runs, matching the existing `reload` behavior. A failed
POST shows its error and does not start its follow-up GET. A successful POST
followed by a failed GET shows the GET error. Successful GET recovery removes
the error. Current POST feedback takes precedence over an older Query error.
Query remains the sole owner of GET error state. The hook derives presentation
from Query state and separate POST feedback without a local GET error copy.

## Persistence and observability

The query cache is in-memory and page-scoped; the backend's in-memory
two-hour cache remains unchanged and authoritative. No snapshot is added to
the boot payload or persisted to browser storage. Existing API errors and
backend job events remain the available diagnostics; this migration adds no
metrics or event logs.

## Related decisions

- [Disk Usage Query Invalidation](../../../decisions/2026-10-05-disk-usage-query-invalidation.md)
- [SystemInfo Query Cache](../../platform/system-design/system-info-query-cache.md)

## Implementation plans

- [Disk usage Query migration](../../../plans/disk-usage-query-migration/plan.md)

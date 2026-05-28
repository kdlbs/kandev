# 0011: TanStack Query as the canonical server-state layer

**Status:** accepted
**Date:** 2026-05-24
**Area:** frontend

## Context

Before this migration, all server state in the Next.js frontend lived in hand-rolled Zustand slices. Each domain invented its own hydration pattern: SSR fetches seeded the slice in `StateProvider`, WS handlers patched the slice directly or toggled a coarse `officeRefetchTrigger` string to kick off a re-fetch. No cross-domain conventions existed for cache invalidation, deduplication, SSR dehydration/rehydration, request retry, or background revalidation. Every new domain repeated the same brittle scaffolding.

Specific pain points:

- **Cache invalidation** — `officeRefetchTrigger` was a bag of strings; components watched the whole bag and re-fetched on any change regardless of domain relevance.
- **SSR / client coherence** — initial payload was piped through `StateProvider` initialState rather than through a shared dehydrated query cache, making it easy for the client to diverge silently.
- **No deduplication** — multiple components could fire identical fetch requests in the same render cycle with no built-in guard.
- **Retries** — handled ad-hoc or not at all per slice.
- **Devtools** — no visibility into cache state without custom DevTools or logging.

## Decision

Adopt **TanStack Query (v5)** as the canonical server-state layer. Zustand remains for purely client-side UI state (connection status, selection, optimistic comment lifecycle, etc.).

### Per-domain file layout

Each domain gets three files under `apps/web/lib/query/`:

| File | Purpose |
|---|---|
| `keys/<domain>.ts` (or a section in `keys.ts`) | Typed query-key factories |
| `query-options/<domain>.ts` | `queryOptions()` definitions — fetch function + key together |
| `bridge/<domain>.ts` | WS → TQ cache bridge: subscribes to the WS client and calls `setQueryData` / `invalidateQueries` on matching events |

### Ring-buffered streams

High-frequency streams (shell output, process stdout, terminal chunks) append to a ring buffer in `lib/query/streams/ring.ts` rather than calling `setQueryData` per chunk. TanStack Query's per-update subscriber notification is a perf cliff for streams that emit hundreds of chunks per second. The ring buffer accumulates chunks and exposes a stable reference; consumers read via `useQuery` on a normal cadence.

### Bridge mount and WS subscription

`<QueryBridge />` (rendered from `apps/web/app/layout.tsx`) mounts all domain bridges. Each bridge calls `subscribeWebSocketClient` (from `lib/ws/connection.ts`) to register event handlers on the shared WS client. `subscribeWebSocketClient` returns a cleanup function and queues subscriptions that arrive before the WS client is ready, so bridges mounted before `<WebSocketConnector />` do not race.

### SSR dehydration / hydration

Server components call the relevant `queryOptions` fetchers and pass the dehydrated cache via `<HydrationBoundary>`. The client rehydrates without a second network round-trip. This replaces the `StateProvider` initialState pattern.

## Migration waves

| Wave | Domains |
|---|---|
| 0 | Foundation: `QueryClientProvider`, `QueryBridge` component, `subscribeWebSocketClient`, ring-buffer primitive |
| 1 | Features, comments, integrations, automations, workspace, settings |
| 2 | Jira, Linear, GitHub, GitLab |
| 3 | Kanban |
| 4 | Office (dashboard, tasks, agents, inbox, activity, approvals, routines) |
| 5 | Session messages, turns, runtime, streams (shell/process/terminal) |

## Transitional Zustand mirrors

Bridges are intentionally transitional. Per-domain Zustand slices that had consumers at migration time were kept as **mirrors**: the bridge writes into TQ via `setQueryData`, and a bridge-side effect also writes the same data into the Zustand slice so legacy consumers keep working without an immediate rewrite.

**Fully removed (no mirror):** `features`, `automations`. All consumers were migrated in the same wave.

**Mirrors remaining:** `comments`, `github`, `gitlab`, `kanban`, `linear`, `office`, `session`, `session-runtime`, `settings`, `workspace`. Each mirror slice will be deleted once every consumer has been pointed at the corresponding `useQuery` hook.

> Callers that still read from a mirror slice directly (rather than through `useQuery`) will see stale or empty data if the bridge has not yet written its first update into the slice. This is the expected transitional state. Prefer reading from queries.

## Consequences

**Better:**

- Cache invalidation is explicit and scoped — `invalidateQueries({ queryKey: officeKeys.dashboard(wsId) })` targets exactly one cache entry.
- Automatic deduplication: concurrent `useQuery` calls for the same key share one in-flight request.
- SSR dehydration/hydration via `dehydrate` / `<HydrationBoundary>` is a first-class pattern; no more custom `StateProvider` piping.
- Retries, stale-while-revalidate, and background refetch are opt-in per query option rather than per-slice DIY.
- TanStack Query Devtools provide cache visibility in dev.

**Worse (transitional):**

- Two sources of truth during overlap: TQ cache + Zustand mirror. Divergence is possible if a write path misses the mirror update.
- Mount-order sensitivity: `<QueryBridge />` must be inside `<QueryClientProvider>`. If the layout tree changes, bridge subscriptions may register late.
- Increased per-domain file count (`keys`, `query-options`, `bridge`) versus the previous single-slice file.

**Still ahead:**

- Sweep all remaining components reading from mirror slices; redirect them to `useQuery` with the matching `queryOptions`.
- Delete bridge files and mirror slices once all consumers have migrated — the bridge layer is scaffolding, not permanent architecture.
- Evaluate whether ring-buffer streams should become a first-class `useInfiniteQuery` pattern once the migration stabilises.

## References

- `apps/web/lib/query/` — keys, query-options, bridge, streams, provider
- `apps/web/lib/query/bridge/index.ts` — `<QueryBridge />` component
- `apps/web/lib/query/streams/ring.ts` — ring-buffer primitive
- `apps/web/lib/ws/connection.ts` — `subscribeWebSocketClient`
- `apps/web/app/layout.tsx` — `<QueryBridge />` mount point
- `apps/web/lib/state/slices/` — surviving mirror slices

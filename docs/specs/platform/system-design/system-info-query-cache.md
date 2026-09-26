---
status: draft
system: platform
requirements: []
created: 2026-09-26
updated: 2026-09-26
owners:
  - kandev
---

# SystemInfo Query Cache

## Purpose and boundaries

This design records the bounded frontend ownership contract for the read-only
SystemInfo resource. It defines an internal state and cache boundary and adds
no independent product requirement. The [backend restart page recovery
design](backend-restart-page-recovery.md) owns process-generation detection and
the reload-required behavior.

The pilot covers only `GET /api/v1/system/info` and the About view. Zustand
continues to own database, jobs, storage, metrics, backups, and other System
state. The [ownership decision](../../../decisions/2026-09-26-system-info-query-cache-ownership.md)
records rationale and alternatives.

## Components and responsibilities

- `SystemInfoQueryProvider` lives in the authenticated app branch. It creates
  one stable QueryClient for the current backend and auth identity.
- `useSystemInfo` reads and refreshes the SystemInfo Query cache. It does not
  write data back to Zustand.
- `AboutCard` remains the presentation consumer and preserves its current
  loading and metadata display.
- `fetchSystemInfo` and `fetchJson` remain the transport path. The query passes
  its scoped request AbortSignal through `RequestInit.signal`; no API transport
  contract changes.
- `BackendGenerationGuard` remains the owner of reconnect identity checks. The
  self-update and restart flows keep their explicit no-store reads.

## Data and identity

The query key includes the canonical full backend API base URL, the document's
stable `bootId`, auth mode, authenticated state, and user ID. SystemInfo is not
scoped to a workspace. The query provider is keyed by the same identity and
creates its QueryClient once in React state. A different identity gets a
different client, so a late response from the old client cannot become visible
to the new identity. Auth-gated navigation unmounts the provider when the app
shell is left. Provider teardown aborts pending requests and clears that
client's cache. A microtask guard prevents React StrictMode's effect replay
from disposing the active client during development.

The Go boot payload is unchanged and does not include SystemInfo. It supplies
only `runtime.bootId` for the existing restart guard and query identity. The
About view fetches SystemInfo lazily from the existing authenticated endpoint
when the query first mounts.

## Control flow and freshness

On the About view's first mount, the query calls
`fetchSystemInfo({ cache: "no-store", init: { signal } })`. TanStack Query
deduplicates concurrent consumers and retains one in-memory snapshot. Loading,
error, and explicit refresh state come from the query. The provider-owned
AbortSignal flows through the existing `RequestInit.signal` support in
`fetchJson`; unmounting or changing identity aborts pending query work before
its client is discarded.

All fields in the SystemInfo response are fixed for a backend process: build
metadata, runtime version/platform values, process start time, and process ID.
The query can keep that snapshot fresh for the page generation. Automatic retry
and reconnect refetch stay disabled to preserve the current one-attempt UI read
and avoid a duplicate request beside the restart guard. An explicit refresh
refetches the resource. On every successful WebSocket connection,
`BackendGenerationGuard` continues its independent no-store request. A changed
boot ID enters the existing reload-required flow, and the next document gets a
new identity and cache.

## Security and failure handling

Failed query requests remain visible through the hook's error value; the About
view keeps its current empty metadata fallback. Manual refresh is also the
retry path. No new data is added to public or authenticated boot payloads.

## Related decisions

- [Give SystemInfo One Query Cache Owner](../../../decisions/2026-09-26-system-info-query-cache-ownership.md)

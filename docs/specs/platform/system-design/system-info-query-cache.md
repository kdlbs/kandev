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
  TanStack Query's observer AbortSignal through `RequestInit.signal`; no API
  transport contract changes.
- `BackendGenerationGuard` remains the owner of reconnect identity checks. The
  self-update and restart flows keep their explicit no-store reads.

## Data and identity

The query key includes the canonical full backend API base URL, the document's
stable `bootId`, auth mode, authenticated state, and user ID. SystemInfo is not
scoped to a workspace. The query provider is keyed by the same identity and
creates its QueryClient once in React state. A different identity gets a
different client, so a late response from the old client cannot become visible
to the new identity. Auth-gated navigation unmounts the provider when the app
shell is left. When the last observer leaves during a pending request, TanStack
cancels the query through the observer signal consumed by the query function.
The old client is detached with its provider; no custom request controller,
effect cleanup, or StrictMode replay guard is used.

The Go boot payload is unchanged and does not include SystemInfo. It supplies
only `runtime.bootId` for the existing restart guard and query identity. The
About view fetches SystemInfo lazily from the existing authenticated endpoint
when the query first mounts.

## Control flow and freshness

On the About view's first mount, the query calls
`fetchSystemInfo({ cache: "no-store", init: { signal } })`, with `signal` from
TanStack Query's query function context. TanStack Query deduplicates concurrent
consumers and retains one in-memory snapshot. Loading, error, and explicit
refresh state come from the query. If identity changes or logout removes the
last observer while a request is pending, TanStack cancels the query through
that signal. A request that has already completed needs no cancellation.

All fields in the SystemInfo response are fixed for a backend process: build
metadata, runtime version/platform values, process start time, and process ID.
The query can keep that snapshot fresh for the page generation. The query uses
`networkMode: "always"` so an initial request runs even when the browser reports
offline. Automatic retries and reconnect refetch stay disabled, so a settled
offline failure is not resumed on reconnect. Concurrent consumers share a
request.
In development StrictMode, observer replay may cancel an in-flight request and
start a replacement when the observer returns. The query follows TanStack's
signal lifecycle without a second request controller. An explicit refresh
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

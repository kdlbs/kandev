# ADR-2026-09-26-system-info-query-cache-ownership: Give SystemInfo One Query Cache Owner

**Status:** accepted
**Date:** 2026-09-26
**Area:** frontend

## Context

The System > About view reads the read-only `GET /api/v1/system/info` resource.
Its hook currently stores the response in the broad Zustand system slice and
keeps separate React loading and error state. This creates multiple owners for
one server snapshot and request lifecycle. Other system settings data has
different update and mutation contracts and remains in Zustand.

The SystemInfo endpoint requires an authenticated request when authentication
is enabled. The boot payload carries only the process `bootId`; it does not
contain a SystemInfo snapshot. The shared shell already makes a no-store
SystemInfo request after each successful WebSocket connection to compare that
ID and require a page reload after a process change.

## Decision

- TanStack Query owns the SystemInfo server snapshot and request state used by
  the About view. Zustand does not mirror query results. All other System slice
  fields and actions remain in Zustand.
- The SystemInfo query key includes the canonical full backend API base URL,
  page `bootId`, auth mode, authenticated state, and user ID. It is not
  workspace-scoped.
- The authenticated application shell owns one stable QueryClient per such
  identity. The client is created once per identity in React state. The cache
  is in memory and is discarded when the identity-scoped provider unmounts or
  changes identity. Its request AbortController is shared by requests in that
  client lifetime, so StrictMode effect replay does not cancel and duplicate
  the initial request. Provider teardown aborts pending requests and clears
  the old client's cache.
- The boot payload is unchanged. It supplies only the stable page `bootId` used
  to construct the query identity. The About view lazily fetches SystemInfo
  from the existing authenticated endpoint when it mounts.
- `fetchSystemInfo` remains the transport owner. The query passes its scoped
  request AbortSignal through the existing `RequestInit` path. Explicit refresh uses
  Query refetch. Automatic query retries and reconnect refetch are disabled so
  the About view retains its current one-attempt behavior and does not duplicate
  the restart guard's required no-store request.
- Every field in the SystemInfo response is fixed for one backend process:
  build values and `started_at` are captured at startup, while Go version, OS,
  architecture, and `boot_id` are process constants. The query may treat the
  response as fresh for that page generation; an explicit refresh remains
  available.
- The backend generation guard remains responsible for checking every
  successful WebSocket connection. A changed `bootId` still requires a full
  page reload; the query cache is not invalidated on ordinary reconnects.
- This decision covers only SystemInfo. Other System resources and later query
  migrations require their own ownership review. It does not add a generic
  WebSocket-to-query bridge or a query ownership lint rule.

## Consequences

The About view has one authoritative server snapshot and one source of loading,
error, deduplication, and refresh state. Its first HTTP request occurs only
when the About view mounts. The restart guard continues to make its separate
uncached request because that request proves process identity on reconnect.

TanStack Query is introduced as a bounded pilot. Its use here does not establish
a default owner for every server-backed Zustand slice.

## Alternatives Considered

- Keep SystemInfo in Zustand and add shared request bookkeeping. This preserves
  a second server-state owner and duplicates behavior TanStack Query already
  provides.
- Keep fetching in each consumer with local state. This cannot deduplicate
  simultaneous consumers or share a single cached snapshot.
- Add a full SystemInfo snapshot to the boot payload. No such snapshot exists
  today, and introducing one would expand a separate bootstrap contract for
  this bounded UI cache pilot.
- Use a global SystemInfo key with no backend or auth identity. This can reuse a
  response across a backend or authenticated-user change.
- Refetch the Query cache on every reconnect. The existing generation guard
  already performs the required uncached check, and a new backend generation
  requires a document reload.

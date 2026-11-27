# ADR-2026-09-27-database-stats-query-cache-ownership: Give Database Statistics One Query Cache Owner

**Status:** accepted
**Date:** 2026-09-27
**Area:** frontend

## Context

The Data & Logs database card reads a mutable server snapshot. Its hook keeps a
copy in Zustand and separate React loading/error state, then runs its own
mount, polling, and retry effects. The Backups section also reads
`backup_directory` from the same Zustand field to render its resolved-directory
description. These are the two current consumers. The resource already has a
process-local server snapshot with a 15-minute measurement lifetime and
explicit refresh endpoint. SystemInfo has a TanStack Query owner, but its
immutable process snapshot and infinite freshness are not a suitable policy
for database stats.

The architecture maintenance tracker selects database statistics as the next
bounded server-state migration. The endpoint is installation-wide. Its GET and
refresh routes stay in the existing member-accessible system route group, while
destructive database maintenance routes remain admin-only.

The boot payload provides a backend boot ID but no database-statistics
snapshot.

## Decision

- TanStack Query owns the browser snapshot and GET request lifecycle for
  database statistics. Zustand no longer stores DatabaseStats or provides a
  setter for it. Other System resources retain their current owners.
- `DatabasePanel` calls `useDatabaseStats` once and passes that result to both
  DatabaseStatsCard and the Backups description. DatabaseStatsCard is
  presentational with respect to this resource and does not create a second
  observer; the Backups description continues to use `backup_directory` from
  the shared result.
- The database query key contains the canonical full backend API base URL, page
  boot ID, auth mode, authenticated state, and user ID. It is not
  workspace-scoped.
- The authenticated app branch keeps its stable QueryClient across identity
  changes. Its cleanup targets obsolete SystemInfo and database-statistics
  entries, cancels and immediately removes them with the same filter, and
  preserves unrelated Query entries. The provider is not keyed by identity.
- The database query consumes TanStack's AbortSignal through the existing API
  wrapper. It uses the existing authenticated GET and refresh routes and adds
  no boot-payload data.
- Database freshness follows the server snapshot state and
  logical_stats_measured_at: revalidate on mount; poll pending or refreshing
  every two seconds; poll stale, unavailable, or read-error states every 30
  seconds; and revalidate a ready snapshot at its 15-minute expiry. Polling
  stops when the last observer leaves. Automatic retry, focus refresh, and
  reconnect refresh remain disabled. The query still attempts a request while
  offline.
- Keep the Query entry for the authenticated app branch lifetime so a last
  successful response remains available across SPA navigation. A failed
  refetch keeps that data visible and exposes the error. Explicit reload
  refetches the same entry. Explicit retry calls the existing refresh endpoint
  and then reads status; neither helper rejects its existing UI caller on a
  request error.

## Consequences

One DatabasePanel observer supplies both current consumers, while one Query
entry owns the cached response and GET lifecycle. The existing server snapshot
continues to provide data across full document reloads. Browser state remains
ephemeral and scoped to the current backend, process generation, and auth
identity. Query freshness remains distinct from cache retention: mutable
logical totals use the backend measurement timestamp and explicit polling,
while other Query resources keep their own policies.

The provider cleanup list must grow only when an independently reviewed System
resource is migrated. This decision does not establish TanStack Query as the
owner for all server-backed Zustand state or add a generic WebSocket-to-Query
bridge.

## Alternatives Considered

- Keep Zustand and the hook's local request state. This preserves existing
  ownership but leaves a second server cache and custom polling lifecycle in
  place.
- Reuse SystemInfo's process-lifetime freshness. Database logical totals can
  change and already expose a measured timestamp, pending/refreshing states,
  and a 15-minute expiry.
- Use one global database-statistics query key. This could reuse data across a
  backend, process generation, or authenticated user change.
- Key the QueryClientProvider subtree by auth or backend identity. This would
  discard unrelated shell and form state when identity changes.
- Put database statistics in the boot payload. The resource is already loaded
  lazily from its authenticated endpoint and its backend service supplies the
  cross-reload snapshot.

# ADR-2026-10-05-disk-usage-query-invalidation: Keep disk snapshot and job-stream ownership separate

**Status:** accepted
**Date:** 2026-10-05
**Area:** frontend

## Context

The System Status disk-usage hook currently keeps the backend snapshot in the
general Zustand System slice, local loading and error state in React, observes
terminal jobs from the shared `system.jobs` map, and polls while the server
reports a walk in progress. A Query migration needs to give the disk snapshot
one owner without moving the system-job stream or promising recovery for an
event the client never observed.

## Decision

- TanStack Query owns the disk-usage response and GET request, loading, and
  error state. Separate refresh POST feedback remains local mutation state so
  the current card error presentation is preserved. The disk snapshot is not
  mirrored into Zustand.
- The existing WebSocket handler and Zustand `system.jobs` map remain the sole
  owners of system-job delivery and storage. One disk-specific bridge watches
  that map and invalidates only the exact disk query for the current full
  backend, boot, and auth identity after a terminal `disk-walk` success or
  failure.
- The query identity uses the shared normalized full backend URL, page boot
  ID, auth mode, authenticated state, and user ID, without workspace scope.
  The bridge's terminal-ID FIFO is bounded to 64 IDs for the current identity,
  resets on identity changes, and treats retained initial terminal jobs as its
  baseline rather than replaying them. Existing terminal store rows suppress
  duplicate terminal transitions after their IDs have left the FIFO.
- The existing 1.5-second recovery interval runs only while the most recent
  successful disk snapshot says `computing=true`. It is periodic, not a capped
  retry count. It does not recover a whole unseen job when cached data remains
  `computing=false`; reconnect does not add job replay or an unconditional
  refresh.
- Query option values and the in-flight invalidation mechanism are specified
  and tested against the installed library. The bridge must cancel and replace
  both no-data and cached-data GETs so a response started before a terminal
  event cannot become authoritative after it. A delayed cancellation
  continuation must verify its captured identity and bridge subscription
  generation before acting, including across A→B→A.
- The hook preserves its caller contract: `reload` and `refresh` resolve after
  handled failures, `isLoading` includes refetch GETs, last-good data survives
  GET errors, and GET or refresh POST errors remain visible through the
  existing card error row.
- Refresh POST continuations use the captured query identity and hook lifecycle
  generation. An obsolete POST result cannot start a GET or alter current
  feedback, even after A→B→A or hook remount.

## Consequences

There is one owner for the disk snapshot and one owner for the system-job
stream. Unrelated System resources, the shell, and the backend job protocol do
not change. The terminal observer has bounded deduplication memory and does
not replay retained jobs when auth identity changes. Tests must cover the
retained-row transition guard as well as the FIFO boundary. After FIFO
eviction, row removal or a nonterminal replay can erase retained terminal
evidence. A later terminal replay can then revalidate. This is a bounded
deduplication contract, without an unlimited at-most-once guarantee.

The bridge uses a synchronous store subscription so React batching cannot hide
terminal transitions. It retains no second job map. In a burst of N accepted
terminal updates, at most N replacement reads start. Duplicate and unrelated
updates add none. Any coalesced read must start after the last accepted update.
Inactive queries retain invalidation until a consumer returns. Before the
first consumer mounts, the bridge does not create a query or start a read.

The installed TanStack Query 5.104.0 deferred-response tests confirmed that
exact-key `cancelQueries` followed by `invalidateQueries` replaces both a
no-data initial GET and a cached-data refetch. The tests resolve an already
aborted GET with stale `computing=false` and `computing=true` responses and
verify the authoritative post-event response remains cached.

The Query cache retains an in-memory snapshot across route unmounts to match
Zustand's process-lifetime value. It does not refetch on focus or reconnect.
The conditional polling interval continues after errors while retained data
still says `computing=true`, and stops only after an authoritative
`computing=false` response or observer cleanup.

The event bridge depends on the current app's static-per-page backend URL and
authenticated socket lifecycle. A future runtime-mutable backend URL must
reconcile the socket source with Query identity before it is supported.

## Alternatives considered

- Keep the snapshot and request lifecycle in Zustand and add more React
  bookkeeping. This preserves multiple owners and repeats server-state cache
  responsibilities.
- Move `system.jobs` into Query. Jobs are a shared high-frequency event stream
  used by other System UI; disk usage must not take ownership of it.
- Add a global WebSocket-to-Query event bus. The only required consumer is the
  disk snapshot, so a resource-specific bridge has a smaller lifetime and
  clearer invalidation key.
- Poll continuously or refresh on every reconnect. Continuous reads ignore
  the server's `computing` signal; reconnect refresh adds a trigger the
  existing hook does not have and cannot prove that a missed job was replayed.
- Keep an unbounded set of all terminal IDs. It suppresses arbitrarily old
  replay but grows for the lifetime of the app; the bounded FIFO and retained
  store-transition check handle ordinary duplicate and evicted-ID cases with
  fixed observer memory.
- Rely on `invalidateQueries` alone during an in-flight first request. The
  selected implementation explicitly cancels the captured exact key first,
  then invalidates it to ensure an authoritative active read starts.

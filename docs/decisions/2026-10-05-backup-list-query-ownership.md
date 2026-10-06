# ADR-2026-10-05-backup-list-query-ownership: Give the Backup List One Query Owner

**Status:** accepted
**Date:** 2026-10-05
**Area:** frontend

## Context

The System Backups view reads one finite server resource through `useBackups`.
The hook mirrors the list in the Zustand System slice and keeps local request state. The create flow polls the list and depends on reload
resolving the resulting SnapshotInfo array or rethrowing the original error.
Delete and other System operations can also change which snapshots the endpoint
returns. A second cache owner would make list freshness and mutation recovery
dependent on synchronizing two stores.

The existing SystemInfo Query pilot is process-immutable and uses an infinite
freshness window. Its policy is not suitable for a mutable backup list. QUERY-02
established the shared mutable System Query identity and cleanup contract; its
frontend migration merged in PR #4225 at
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8` before QUERY-03 implementation.

## Decision

- TanStack Query owns the backup-list response and request lifecycle. The
  Zustand System slice does not mirror this resource.
- The backup query uses the shared stable QueryClient and the exact identity,
  cancellation, and obsolete-resource cleanup contract established by the
  merged QUERY-02 implementation. It does not create an independent provider,
  remount the shell, or clear unrelated Query entries.
- The list is treated as mutable with finite freshness and no automatic request
  retries. Explicit reload performs a fresh read, resolves with
  SnapshotInfo[], and rejects with the original request error. A failed read
  does not replace successful cached data. an empty successful response is a
  loaded empty list.
- Request lifecycle is explicit: `staleTime` is 30 seconds and inactive
  `gcTime` is five minutes. `refetchOnMount` is `"always"`. Focus/reconnect
  refetch only when stale. `retry` is false, `networkMode` is `"always"`, and
  interval refetching is disabled. Mount-always intentionally replaces the shared
  Zustand `loaded` guard so revisiting Backups can observe external maintenance
  writes. Freshness does not control retention, and `gcTime` does not schedule
  a request. The backup list does not reuse DatabaseStats polling.
- List-changing operations refresh only this backup resource. A successful
  delete refreshes it. a failed delete leaves its prior data intact. The
  existing create poll continues to use reload. Tool Payload Retention correlates a locally observed attempt with the policy revision and captured query identity.
  A save candidate must require new preparation. The
  preparation state, not operation ID, identifies success because success
  replaces the backup operation with a cleanup operation. A matched attempt
  refreshes once on ready/failed, or when a previously observed pending/running
  attempt ends through cancellation or policy replacement. Historical status,
  carried historical ready state, or unrelated cleanup do not refresh it.
  Skip choice creates no new attempt but can settle an older active backup attempt.
  A settled-revision marker prevents repeated responses from re-arming an attempt.
  Clear local correlation state on full identity or caller lifetime changes. Factory reset
  refreshes the current backup identity when its result may have published a
  listed snapshot. Restore does not change list files: after its existing
  required relaunch, the new process identity reads the list again.
- Every writer captures its initiating identity and lifetime. Cancel earlier
  reads of that exact key before a read after the operation result.
  This also covers an initial pending read without cached data.
  Late operation responses cannot refresh another identity or recreate obsolete entries.
  Delete awaits reload errors. Retention/reset refresh errors preserve the operation result.
- GET /api/v1/system/backups remains member-readable. Create, download, restore,
  and delete remain admin-only. The Query migration changes neither API
  semantics nor these authorization checks.

An initial historical ready/failed status has no safe attempt identity without a qualifying save response or observed pending/running state. It does not invalidate. The existing backup-list mount, stale focus/reconnect,
and explicit reload triggers observe changes that cannot be attributed safely.

## Consequences

Concurrent observers share one authoritative list and Query request lifecycle.
The hook preserves loaded state after refetch failure and exposes read errors.
BackupsTable retains its existing local mutation-error display.
The migration removes backup-only Zustand values, setters, and duplicate
effects. Mutation errors retain the last successful data, while a successful
list-changing operation triggers an authoritative read scoped to the current
backend, process, and auth identity.

The shared identity and cleanup policy follows QUERY-02's merged code. Reset
and restore keep their existing quit/relaunch behavior. External startup or operator-run maintenance can create
snapshots without a browser event. the list observes those changes on its next
fresh mount or explicit reload. No polling loop or generic WebSocket bridge is
added.

The resolved Backups directory remains supplied by the DatabasePanel
useDatabaseStats observer. It is separate from the backup-list query and must
continue to satisfy the existing path display and copy checks.

## Alternatives Considered

- Keep Zustand as the owner and add query-like request sharing. This preserves
  duplicate server-state ownership and reimplements behavior already provided
  by TanStack Query.
- Keep independent hook-local requests. Concurrent consumers can issue
  duplicate reads and can replace the list in response order.
- Reuse SystemInfo's infinite freshness policy. A backup list changes after
  create, delete, retention preparation, reset, startup, and maintenance.
- Clear the whole QueryClient or key the provider by identity. This can discard
  unrelated active resources or remount shell forms. The shared provider's
  targeted identity cleanup is the boundary for this migration.
- Add a generic WebSocket-to-Query bridge. The existing list changes have
  bounded mutation and process-identity paths. a general bridge would broaden
  the resource migration without evidence that it is needed.

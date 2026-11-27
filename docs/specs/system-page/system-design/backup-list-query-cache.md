---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
created: 2026-10-05
updated: 2026-10-06
owners:
  - kandev
---

# Backup List Query Cache System Design

## Purpose and boundaries

The system-page system owns the technical contract for the snapshot list used
by the Backups section. This design moves that finite list and its request
lifecycle to TanStack Query. It does not change snapshot storage, retention,
operation behavior, restart flow, or authorization.

The design uses the existing backup API and the shared QueryClient. It follows
the identity, request cancellation, and obsolete-query cleanup contract merged
with QUERY-02 in PR #4225 at `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`. It
does not define a second identity system or redesign the shared provider.

## Requirement mapping

| Requirement                                 | Design section                                                |
| ------------------------------------------- | ------------------------------------------------------------- |
| REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001         | Components and responsibilities, Data and contracts, Security |
| REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001 | Components and responsibilities, Failure and recovery         |
| REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002      | Components and responsibilities, Mobile behavior              |

This migration preserves the existing operation and presentation contracts. It adds no product
requirement. Mount revalidation is an explicit freshness change, described below. The legacy system-page design remains the source for existing
snapshot list and operation behavior until its requirements migration extracts
those criteria.

## Components and responsibilities

- The shared System Query provider owns one stable QueryClient for the mounted
  authenticated application branch. It applies the identity and cleanup policy
  established by QUERY-02. QUERY-03 registers only the backup-list query family
  for that targeted policy.
- The backup query key distinguishes the current backend, process generation,
  and auth identity according to QUERY-02. It has no workspace dimension.
- The query function calls `fetchBackups` with the key's captured API base URL,
  `cache: "no-store"`, and TanStack Query's `AbortSignal`. The existing transport
  supports these options. A later backend configuration change cannot redirect
  an old query request to a new backend.
- useBackups exposes the query data and state to BackupsTable. BackupsTable
  retains its existing table, loading, empty, permission, create-polling, and
  delete flows.
- The tool-payload-retention status hook tracks locally submitted or observed active backup
  preparations and requests a targeted list refresh when that attempt ends.
  Factory-reset job observation also requests a targeted refresh at terminal
  state because reset can publish a listed snapshot. Neither path publishes a
  new event or alters backend behavior.
- DatabasePanel keeps its single useDatabaseStats observer as the source of the
  resolved backup_directory displayed above BackupsTable. That metadata is not
  copied into the backup-list query.

## Data and contracts

The authoritative response remains GET /api/v1/system/backups, represented as
SnapshotInfo[]. fetchBackups normalizes an absent snapshots field to an empty
array. The backup query is mutable and uses finite freshness, with automatic
request retries disabled. Its initial read is scoped to the active
backend/process/auth identity. changing identity does not remount the shell.

useBackups.reload preserves Promise<SnapshotInfo[]> behavior:

- A successful request updates the authoritative Query entry and resolves with
  the list from that request. Query structural sharing can retain equal cached
  objects, so reload does not promise array reference identity. An empty successful response is [] and is distinct from
  loading or failure.
- An unsuccessful request rejects with the original thrown error. It does not
  replace successful cached data with an empty list.
- Initial loading, successful empty state, read error, background refresh, and
  explicit reload remain distinct. Mutation errors remain local to their
  existing callers.

Capture the active scope when `reload()` is invoked, after the identity's
layout commit has advanced its generation. Do not retain a render-time scope
that can already be obsolete in the same committed render. A writer captures
its full identity and caller lifetime when it starts; every create-poll read
uses that captured scope and stops after an identity or lifetime change. This
prevents a pending old-identity request from either aborting a new create poll
or redirecting an old writer's poll to a different identity.

### Hook and consumer state contract

Keep `backups`, `loaded`, `isLoading`, `error`, and `reload` in the hook contract.
`backups` is the current identity's data or `[]` before its first success.
`loaded` means that the current entry has successful data, including `[]`.
It stays true after a failed refetch. Do not derive it from `isSuccess` alone.
`isLoading` follows `isFetching`, including explicit reload and background reads.
A disabled query without an eligible identity does not show a loading spinner.
`error` keeps the existing Error-message or String conversion and clears after recovery.

Never use another identity's data as `placeholderData` or `initialData`.
The boot payload supplies no backup list. A changed identity starts without the old list.
BackupsTable currently shows local create/delete errors and does not consume the hook's read error.
Preserve that presentation boundary. A new initial-read error control is outside this migration.
A first-read failure must remain distinguishable from a successful empty list in the hook.

### Request ordering and mutation boundaries

An explicit reload bypasses the freshness window. Concurrent readers can share an eligible in-flight read for the same key.
Reload must reject request errors, rather than return a Query result object containing an error.
With observer `refetch`, use `throwOnError: true` and return its successful data.
Do not turn cancellation or absent data into a successful `[]`.

A list-changing operation creates a request boundary after its accepted or terminal result.
Cancel the exact key's earlier request before the authoritative read or invalidation.
This rule also applies before the first successful list read, when the cache has no data.
TanStack's default `cancelRefetch` does not replace every initial pending request.
Do not rely on that default to exclude a response from before the mutation.
Native Query cancellation must reject a late result even if a test transport ignores its signal.
Ordinary concurrent reloads can share a request. A later writer can supersede that request through cancellation.
A superseded caller must not treat cancellation as a successful list or a completed create.

Delete keeps its existing awaited reload and propagates follow-up read errors.
Create establishes the boundary after POST acceptance, then uses the existing reload poll.
Retention and reset use exact-key invalidation with active-only refetch.
An inactive entry becomes stale without a new read. An absent entry is not created by invalidation.
A refresh error stays in the backup query and preserves its prior data.
It must not change a successful retention save/cancel response or a terminal reset result into a mutation error.

Capture the full query identity and the caller's lifetime at operation start.
Before each follow-up read, invalidation, or local result update, check that both remain current.
An identity transition ends that lifetime, including a rapid A-to-B-to-A transition.
A late DELETE, create response, reset job update, or retention response cannot refresh the new identity or recreate an obsolete entry.
Stop the old create poll after unmount, logout, or identity change.
Keep unrelated forms mounted. Do not introduce a second list cache or an independent request controller.

### Query freshness, retention, and request triggers

Use explicit TanStack Query v5 options for this mutable resource:

| Option                 | Backup-list policy | Effect                                                                                                                                                                                                      |
| ---------------------- | ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `staleTime`            | 30 seconds         | Successful data is fresh for 30 seconds. This is a freshness window, not a timer and not cache retention.                                                                                                   |
| `gcTime`               | 5 minutes          | After the last observer unmounts, retain the inactive entry for five minutes, then allow garbage collection. This bounds inactive cache retention. it does not trigger a fetch.                             |
| `refetchOnMount`       | `"always"`         | Every new observer mount performs an authoritative read even inside `staleTime`, so a later Backups visit sees snapshots created by external maintenance. Concurrent observers share the in-flight request. |
| `refetchOnWindowFocus` | `true`             | Refetch only when data is stale and the window regains focus.                                                                                                                                               |
| `refetchOnReconnect`   | `true`             | Refetch only when data is stale and the browser reconnects.                                                                                                                                                 |
| `retry`                | `false`            | Each automatic or explicit request is one attempt. existing callers still receive the original read error.                                                                                                  |
| `networkMode`          | `"always"`         | Attempt requests to the local Kandev backend even when browser online detection reports offline. this preserves direct-fetch behavior.                                                                      |
| `refetchInterval`      | disabled           | Do not poll the backup list. External writers are observed on mount, stale focus/reconnect, or explicit reload.                                                                                             |

The five-minute `gcTime` begins only while there are no observers. It is
separate from the 30-second freshness window. Retained data remains available
while a mount-triggered request runs and after a failed refetch. after garbage
collection, a later mount starts with the normal initial-loading state. The
five-minute retention keeps the small last-good list across short route
changes and bounds inactive cache lifetime. The 30-second freshness window
avoids repeated focus/reconnect reads during brief returns. it does not schedule
a request. The current Zustand `loaded` guard suppresses later mount reads after one
successful load. `refetchOnMount: "always"` intentionally changes that behavior to observe
out-of-process maintenance writes. it adds no timer and Query still deduplicates
concurrent consumers. Explicit `reload` always makes an authoritative request
regardless of freshness.

The list changes through these observed paths:

| Operation                          | List effect and refresh behavior                                                                                                                                                                                                                                                                                                                                                                                                             |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Manual create from Backups         | POST acceptance does not mean the file exists. Keep the 250 ms / 15 second reload poll. Each reload performs an authoritative read, returns the list for the new-name check, and propagates read failures to the existing caller. End the old poll on caller lifetime or identity change.                                                                                                                                                    |
| Delete from Backups                | Refresh the exact current backup query after DELETE succeeds. A rejected DELETE does not invalidate or clear the list. A failed follow-up read retains the previous successful list and surfaces the existing caller error.                                                                                                                                                                                                                  |
| Download                           | No list change and no invalidation.                                                                                                                                                                                                                                                                                                                                                                                                          |
| Restore                            | Restore writes the selected snapshot over the active database but does not create or remove a listed file. Preserve the existing quit/relaunch requirement. The new process identity fetches under its own key. a failed restore keeps the current list.                                                                                                                                                                                     |
| Factory reset                      | The reset job creates a protected pre-reset snapshot. Capture the initiating identity and returned reset job ID. Settle that pair once at succeeded/failed, through either WS state or the existing polling fallback. Refresh only while that lifetime remains current, including failure after snapshot publication. A later quit/relaunch uses the new process identity and reads again.                                                   |
| Tool Payload Retention preparation | A locally observed backup-choice attempt can publish a manual snapshot. Refresh the exact backup query once when that attempt reaches ready or failed, or when an observed pending/running attempt ends through cancellation or policy replacement. A failure or cancellation can follow publication. Ignore historical terminal status and unrelated cleanup. Skip choice creates no attempt but can settle an older active backup attempt. |
| Startup snapshot                   | Startup creates an automatic snapshot before the page's first list read. The initial query read observes it.                                                                                                                                                                                                                                                                                                                                 |
| Operator-run database maintenance  | The backend maintenance command can create an automatic snapshot outside the SPA and publishes no browser invalidation event. The next fresh mount or explicit reload observes it. this migration adds no polling or generic WebSocket bridge.                                                                                                                                                                                               |

### Tool Payload Retention attempt correlation

The retention status does not contain a stable backup-attempt ID. It contains
the policy revision and preparation state/choice. A successful preparation
replaces the backup Operation with a cleanup Operation, while cancel clears
the preparation choice and can leave the Operation cancelled. The filesystem
rename can publish the manual snapshot just before cancellation or a policy
replacement prevents finishPreparation from recording ready.

Before a backup-choice save, capture its full backup-query identity and
expected next policy revision (`submitted revision + 1`). Keep this as a
candidate only for that request and a change that requires new preparation.
Use the existing `needsBackupReview` logic and the saved policy baseline.
If extraction is necessary, keep one predicate shared with the draft hook.
Do not infer a new preparation from `backup_choice` alone.
`Service.Save` can retain an old ready preparation while it increments the policy revision for a change without new review.
An unchanged or less aggressive enabled policy with a carried ready state must not invalidate.
Without a trustworthy baseline, only observed pending/running can establish a new attempt.
A terminal response alone then remains uncorrelated.
For a qualifying candidate, establish an attempt only from a matching successful save response.
The response must have the expected revision, backup choice, and pending, running, ready, or failed state. Inspect the save response itself so a preparation that is already
terminal when save returns is not missed. A rejected or nonmatching save
response keeps its existing error/result contract and discards the candidate.
A status read that first observes pending/running with backup choice can
independently establish an observed attempt even if this hook did not send the
save. Retain that attempt while its matching revision reports pending or
running.
Do not correlate by Operation ID: that ID can change from the backup
operation to the cleanup operation on success.

The safe observation boundary is deliberate. An initial historical ready/failed status cannot distinguish a new writer from old state without a qualifying save or previously observed pending/running attempt.
The API exposes no stable attempt ID or event.
Do not invalidate from that status alone. The backup list's mount, stale
focus/reconnect, and explicit-reload paths remain responsible for observing
external changes in this uncorrelated case.

When a matched attempt reports ready or failed, invalidate once.
Also settle an observed pending/running attempt on cancellation to none or replacement by a newer policy revision.
The latter refresh covers a snapshot that was renamed before cancellation or
replacement. Remove the active attempt before requesting invalidation.
Retain the highest settled revision as a bounded marker for the current lifetime.
A repeated save or delayed pending/running response at that revision or an older revision cannot re-arm an attempt.
A later qualifying backup-choice save at a newer revision is a separate attempt.
Process each accepted status or mutation response through one correlation path.
Keep the existing read-generation and mutation serialization guards.
A read from before a mutation cannot regress its response.
On replacement, settle the old attempt before a candidate for the newer revision.
When the full query identity or caller lifetime changes, clear all correlation records.
Reject late status, save, or cancel responses through the lifetime/generation and current-identity checks.

Do not create a record for skip-choice saves, analysis/cleanup commands, or
initially loaded historical ready/failed states. A later backup-choice save
that requires new preparation at a newer policy revision is a distinct attempt.
Before invalidation, check that the captured full identity and lifetime remain current.
Ignore late status or mutation results from an obsolete identity.
Cancel any read from before the operation boundary. Then invalidate the captured exact backup query key only. Query invalidation refreshes an active list observer and
marks an inactive entry stale. it does not issue a request for another
identity.

An identity transition also ends the retention request owner's epoch.
Reset remote status, errors, pending state, and accepted-operation tracking for the new identity.
Old request finalizers must not unlock a new request or clear its pending state.
Use the existing lifetime mechanism for these guards. Do not remount the shell.
Guard the draft hook's post-save updates too. Its Save contributor currently awaits the remote result before it updates the draft and clears choice.
A preserved save return value from an obsolete lifetime cannot update the current draft or choice.
The backup-attempt record remains local bookkeeping, not a second owner for retention status.

This integration does not migrate retention status into Query. It does not
change the existing save, cancel, status-polling, or error contracts. Query
invalidation is scoped to the backup list and cannot replace the returned
retention status or mutation result.

Only backup-list entries are invalidated or removed. In-flight list requests use
native Query cancellation. Identity cleanup cancels and removes only obsolete
backup-list entries using the QUERY-02 landed contract. It must not clear
unrelated active resources or retain a delayed removal callback that can
remove a newly current identity after a rapid identity transition.

## Control flow

BackupsTable and any concurrent useBackups observers select the same
identity-scoped query. TanStack Query deduplicates the request and publishes
one list to all observers. Explicit reload obtains an authoritative read even
when the prior result is fresh. It can share an eligible in-flight request.
Reload returns data or rejects as described above.

After a successful DELETE, cancel the captured key's earlier read and await reload under the current lifetime.
Mutation failure leaves the query data untouched. Tool Payload Retention and
Factory Reset only invalidate this key at the correlated terminal boundaries
described above. Restore relies on the required process relaunch to move to a
new identity. No WebSocket event is added or generalized.

## Failure and recovery

- A first-read failure does not trigger automatic Query retries. It remains an
  error state and does not become a successful empty list.
- A failed background or explicit read leaves the last successful list
  available. reload still rejects the original error for callers such as the
  create poll.
- A failed DELETE does not refresh the list. A failed refresh after a
  successful DELETE retains the previous data while exposing the refresh
  error.
- Backend URL, process generation, auth mode/state, or user changes follow the
  exact QUERY-02 identity transition and cancellation behavior. Logout
  unmounts the authenticated provider as designed there.
- Query identity changes do not key or remount the application shell, so
  unrelated shell and form state remains mounted.

## Persistence

The Query cache is in-memory only. No backup-list values are added to the boot
payload, local storage, or persisted Zustand state. The backend remains the
durable owner of snapshot files.

## Security

GET /api/v1/system/backups remains member-readable. Create, download, restore,
and delete remain admin-only. UI action gates and backend authorization remain
unchanged. Auth identity remains part of the shared query identity so a cached
list is not reused across a different authenticated context.

The list continues to expose snapshot names, sizes, modification times, and
classification only. This migration does not add filesystem paths or backup
content to the response.

## Observability

This migration adds no logs, metrics, or events. Existing backend jobs and
status polling remain the operation progress signals.

## Mobile behavior

The Data & Logs page keeps its current desktop and phone composition. The same
BackupsTable and query state serve both viewports. No layout, navigation,
touch-target, or scroll-owner change is proposed. The existing phone
member-gating E2E remains the mobile rendered check. hook and consumer tests
cover the state-only changes.

## Verification

Hook and provider tests cover concurrent observers, identity transitions,
native request cancellation, no retries, stale-data preservation, and reload
resolution/rejection. Query-policy tests cover `staleTime=30s`, `gcTime=5m`,
mount-always, stale-only focus/reconnect, offline local reads, disabled
retries, and no interval through controlled timers and focus/online events, including a later mount after an external maintenance
write. Consumer tests cover create polling and successful/failed delete.
Retention tests cover save responses already at ready/failed and failure after visible publication.
Repeated terminal polls must invalidate once. Cancellation to none and policy replacement must settle an observed attempt, including surviving snapshot publication. Also cover initial historical terminal state, skip
choice, unrelated cleanup, a later distinct attempt, active/inactive list
observers, and deferred status/save/cancel responses across backend/auth
identity changes. Reset tests cover both terminal states, duplicate WS/poll observations, and deferred job acceptance across identity changes.
Read-race tests defer an initial list request with no cached data, complete a writer, then resolve the old request.
Only the read after the operation boundary can populate the list.
Also cover cached refetch races, one remaining observer, StrictMode replay, and A-to-B-to-A transitions.
Retention tests include carried historical ready state, same-revision re-arming, older-revision responses, and a failed list refresh after successful save/cancel.
A deferred save must not update the new identity's retention draft or clear its choice.
Restore keeps its existing quit/relaunch contract. System-slice tests and a source search prove the old backup
fields, setter, and barrel export are removed without resetting unrelated System or shell
state. Desktop and mobile settings E2E retain list visibility, directory copy,
permissions, and the phone layout.

## Related documents

- [Backup guidance requirement](../requirements/backup-location-actions.md)
- [Database statistics requirement](../requirements/database-statistics-snapshot.md)
- [Data and Storage pages requirement](../requirements/system-data-storage-pages.md)
- [Existing backup operation design](backup-location-actions.md)
- [SystemInfo pilot baseline](../../platform/system-design/system-info-query-cache.md) (its immutable freshness policy does not apply)
- [Migration tracker](../../../architecture-maintenance/server-state-migrations.md)
- [TanStack Query cancellation](https://tanstack.com/query/v5/docs/framework/react/guides/query-cancellation)
- [Ownership decision](../../../decisions/2026-10-05-backup-list-query-ownership.md)
- [Implementation plan](../../../plans/system-backup-query/plan.md)

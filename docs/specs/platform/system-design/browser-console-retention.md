---
status: current
system: platform
requirements:
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
---

# Browser Console Retention System Design

## Purpose and boundaries

This design makes browser-log retention and explicit bundle snapshots
incremental. It preserves the existing diagnostic limits, identity partitions,
and memory fallback. The capture notification adds a relative time budget. The
IndexedDB schema and normal write transaction do not change.

## Requirement mapping

| Requirement                                  | Design section                                                                                                                                                                       |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001` | [IndexedDB model](#indexeddb-model), [Write transaction](#write-transaction), [Per-tab drain ownership](#per-tab-drain-ownership), [Migration and recovery](#migration-and-recovery) |
| `REQ-PLATFORM-DIAGNOSTIC-LOGGING-001` | [Incremental capture reads](#incremental-capture-reads) |

## IndexedDB model

`apps/web/lib/logger/indexeddb-store.ts` keeps the database name
`kandev-diagnostic-logs-v1` so existing profiles retain their history. The
schema version remains 2.

The `entries` store keeps its `identity_scope` and `timestamp_ms` indexes. The
metadata object store owns one retention record with these fields:

```text
key: "retention"
count: number
bytes: number
```

The totals cover all identity partitions. They use each entry's stored `bytes`
field. This field contains the serialized size of the entry.

## Write transaction

Each append opens one `readwrite` transaction over the `entries` and metadata
stores. It performs these steps in order:

1. Read the retention totals.
2. Add valid entries and add their count and bytes to the totals.
3. Use a bounded `timestamp_ms` cursor to delete only entries before the
   three-day cutoff. Subtract each deleted entry from the totals.
4. If a count or byte limit is still exceeded, walk the oldest entries only
   until both limits hold.
5. Write the updated totals and commit.

The transaction makes entries and totals one atomic state. IndexedDB serializes
overlapping `readwrite` transactions whose store scopes overlap. Tabs therefore
cannot commit stale retention totals or exceed a shared limit after commit.

The normal within-limit path does not open an unrestricted cursor. It touches
the new batch, the expired prefix, and only the oldest entries that it must
evict. `clear()` uses both stores in one transaction and writes zero totals.

## Per-tab drain ownership

`apps/web/lib/logger/runtime.ts` owns one active drain promise. Idle and timeout
callbacks request work from the same drain loop. They do not start a second
`store.append()` while the first call is pending.

Each staged entry receives a local increasing sequence number. Each drain
request records the newest sequence number that it owns. The drain stops after
it processes that fixed prefix. Entries that arrive later start a new drain.

At capture receipt, the runtime records the current sequence number. It also
takes a bounded prepared-entry snapshot from the memory buffer. The capture
then joins the active drain and requests persistence through the recorded
sequence number. Entries that arrive after receipt do not extend this wait.

The capture waits at most one second for this fixed drain prefix. If the wait
expires, the capture uses the memory snapshot that it took at receipt. The
active persistence drain continues. This fallback does not change the normal
storage mode or erase staged entries. The upload reports `storage_mode: memory`
and records `flush_timeout: true` in its final capture metadata.

The console interception path remains synchronous and bounded. It only adds a
reference-free entry to staging and schedules the loop.

## Incremental capture reads

After the fixed-prefix drain completes, the runtime selects one snapshot source
for the capture. IndexedDB mode reads through the existing `timestamp_ms`
index. Memory mode reads the prepared entries from the receipt snapshot.

Before the first IndexedDB page, the runtime records the highest persisted
object-store primary key. This persisted high-water key is an upper boundary for
the capture. Every page excludes rows with a larger primary key, so entries
written after receipt cannot enter a later page even when their timestamps sort
before earlier rows. A capture with no persisted rows uses an empty IndexedDB
source.

Each IndexedDB page uses one readonly transaction. A continuation token contains
the timestamp index key and the object-store primary key. This pair gives a
stable order when entries have equal timestamps.

The cursor scans forward from that token and includes only the requested
identity. The store holds at most 10,000 entries or 20 MiB across all
identities. Thus, one complete capture scans no more than the existing global
retention limit. A new write index is not necessary.

The cursor stops before the next matching entry exceeds the requested page
size. The browser uploads the page before it opens the next transaction. Thus,
the browser does not clone or sort the complete identity partition.

Persisted and memory records carry their prepared UTF-8 byte count. Page
accounting reuses that count. The request encoder performs the one required
payload serialization.

The first page uses a 128 KiB entry-data target. Later pages use the existing
800 KiB target. Both targets use a lower value when the server advertises a
smaller maximum. This layout adds at most one request to a maximum-size capture
and gives slow links a smaller first transfer.

The notification supplies `capture_timeout_ms`. The frontend converts this
duration to a local deadline with `performance.now()`. The frontend checks this
deadline before each page and upload. An `AbortController` cancels an active
upload when the local deadline expires.

The backend still closes collection at its absolute `capture_deadline`. It can
reject a late request even when transport delay leaves time in the frontend
budget. The frontend does not retry this request.

If an IndexedDB read fails before upload, the capture uses its receipt-time
memory snapshot. If a read fails after upload, the capture stops. It does not
switch sources and duplicate entries.

### Burst collection and entry preparation

The first staged entry opens one 250 ms collection window. Browser idle time
cannot start persistence before that window closes. When the window closes,
the runtime starts one drain and fills each transaction up to the existing
50-entry or 256 KiB limit. A diagnostic snapshot cancels the collection wait
and joins the same drain immediately.

The runtime prepares each accepted entry once after it adds the identity
scope. This preparation creates the detached entry and its exact encoded byte
size. The memory buffer, staging queue, and IndexedDB store reuse that prepared
entry and byte size. They do not repeat `JSON.stringify()` or UTF-8 encoding at
each layer.

The 500-entry and 2 MiB staging limits remain exact. The collection window
does not delay the original console call, increase the staging limits, or
change which log levels a diagnostic bundle contains.

## Migration and recovery

The version-2 upgrade transaction creates the metadata store and walks existing
entries once. It calculates the count and byte totals before the transaction
commits. This capture change does not open a database upgrade.

If a version-2 database lacks a valid retention record, the next write rebuilds
the totals in one repair transaction before it accepts normal incremental
maintenance. This is a recovery path, not a per-batch scan.

If a write, migration, or repair fails, the existing runtime degrades to the
bounded memory store and increments persistence-failure metadata. It does not
retry in a tight loop or delay the original console call.

## Test strategy

Store tests cover the version-1 to version-2 total rebuild, a normal append
without an unrestricted cursor, expired-prefix deletion, oldest-first count and
byte eviction, atomic clear, and two writers using the shared transaction
scope. Vitest uses the dev-only `fake-indexeddb` package so these tests exercise
real IndexedDB request and transaction behavior without a product dependency.

Runtime tests hold the first append promise while more entries arrive. They
show that append concurrency stays at one. They also prove that capture
drains only its fixed sequence prefix. Fake-time tests prove the one-second
memory fallback and continued background persistence.

Capture tests populate more than one page across identities and timestamp ties.
They prove that continuation has no gaps or duplicates. They also prove that
the browser uploads the 128 KiB first page before it reads the next page.

Protocol tests prove that the backend sends both deadline fields. Fake-time
frontend tests use a wall-clock skew and a monotonic clock. They prove that the
relative budget controls capture and cancels an active upload.

## Related decisions

- [File-backed diagnostic bundles](../../../decisions/2026-07-30-file-backed-diagnostic-bundles.md)

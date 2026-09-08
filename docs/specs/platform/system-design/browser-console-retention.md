---
status: current
system: platform
requirements:
  - REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001
---

# Browser Console Retention System Design

## Purpose and boundaries

This design makes browser-log retention incremental. It preserves the existing
diagnostic limits, identity partitions, capture protocol, and memory fallback.
It changes IndexedDB bookkeeping and the per-tab drain coordinator.

## Requirement mapping

| Requirement                                  | Design section                                                                                                                                                                       |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001` | [IndexedDB model](#indexeddb-model), [Write transaction](#write-transaction), [Per-tab drain ownership](#per-tab-drain-ownership), [Migration and recovery](#migration-and-recovery) |

## IndexedDB model

`apps/web/lib/logger/indexeddb-store.ts` keeps the database name
`kandev-diagnostic-logs-v1` so existing profiles retain their history. The
IndexedDB schema version increases from 1 to 2.

The existing `entries` store and its `identity_scope` and `timestamp_ms`
indexes remain. A new metadata object store owns one retention record with
these fields:

```text
key: "retention"
count: number
bytes: number
```

The totals cover all identity partitions. They use each entry's stored `bytes`
field, which is the existing serialized-size measure.

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

`apps/web/lib/logger/runtime.ts` owns one in-flight drain promise. Idle and
timeout callbacks request work from the same drain loop. They do not start a
second `store.append()` while the first call is pending.

The loop removes one bounded batch, waits for its append transaction, and then
continues if staging still contains entries. `flushStaging()` awaits that same
loop before a diagnostic snapshot. It does not create a parallel writer.

The console interception path remains synchronous and bounded. It only adds a
reference-free entry to staging and schedules the loop.

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

The version-2 upgrade transaction creates the metadata store and walks the
existing entries once to calculate count and bytes. It writes the retention
record before the upgrade commits. An interrupted upgrade rolls back as one
IndexedDB transaction.

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
confirm that append concurrency stays at one and that a snapshot waits for the
same drain. Fake-time tests also prove that browser idle time cannot split one
collection window into one-entry transactions, and that a snapshot bypasses
the wait. Entry-preparation tests prove that the memory and IndexedDB paths use
the same encoded byte count.

## Related decisions

- [File-backed diagnostic bundles](../../../decisions/2026-07-30-file-backed-diagnostic-bundles.md)

# ADR-2026-10-05-sqlite-writer-transaction-admission: Acquire SQLite writers at transaction entry

**Status:** accepted
**Date:** 2026-10-05
**Area:** backend

## Context

Managed conversation deletion must validate current retained identity, revision,
hierarchy and cleanup authority in the same native transaction that installs
its exclusive owner barrier. A second native transaction removes the task and
records the exact operation's deleted marker. Neither transaction can read a
stale snapshot before acquiring its writer. External environment, canvas and
resource work stays outside those transactions.

At the diagnostic proofbase, the shared SQLite factory opened a single-connection
writer in WAL mode with a 5-second busy timeout and deferred transactions. Independent pools
can still contend. Actual pinned mattn v1.14.33/SQLite 3.51.1 diagnostic evidence
shows immediate SQLITE_BUSY (code/extended code 5) from deferred transactions
using zero-row primary-key updates, zero-row title updates, and an actual-row
title update. An actual-row no-op also invokes real FTS/audit triggers when it
succeeds. These are not interchangeable writer-admission mechanisms.

The supported driver `_txlock=immediate` comparison waited inside actual
`SQLiteConn.begin`, remained there across a real holder mutation and independent
BUSY probe, then read the committed revision after holder release. A separate
cancelled comparison settled with rollback after release. That diagnostic-only
exit 0 does not establish production acceptance or cancellation latency while
the holder remains locked. ROOT subsequently reviewed this factory boundary and released its
implementation in the same primary. Production acceptance and delivery
remain governed by the single work order.

## Decision

Configure the existing `internal/db.OpenSQLite` writer DSN with the pinned
SQLite driver's supported `_txlock=immediate`. Preserve its existing path,
foreign-key, WAL, synchronous, busy-timeout and pool settings. Every transaction
started on that factory's writer acquires the SQLite writer at BEGIN, before
predicate reads. `database/sql` and `sqlx` continue owning genuine transactions;
no manual transaction facade, driver mutation, transaction restart within a
Tx, per-feature pool, row-touch substitute, or application busy-retry loop.

Keep `OpenSQLiteReader` read-only and deferred, with its existing separate
four-connection pool and private cache. Read snapshots belong on that reader.
`ReadOnly: true` on the writer is not an escape hatch: the pinned driver's
`BeginTx` delegates to its configured begin and ignores transaction options.
Keep PostgreSQL READ COMMITTED and existing hierarchy/current-row lock order.
An arbitrary injected deferred pool is outside this scheduling guarantee; its
native current guards must still fail safely BUSY/Unavailable before effects,
without task-ID fallback. Direct autocommit statements keep their current
SQLite semantics; this decision governs transaction entry.

Native managed predicates, owner CAS and the deletion marker remain inside the
actual `sqlx.Tx`. The hierarchy helper and its eligibility rules remain; its
zero-row statement does not supply the independent-pool waiting guarantee.
Committed PREPARED admission remains reversible authorization/exclusion, not
physical deletion. Admitted preparation can retain existing partial effects on
failure. Only the atomic deleted marker proves removal and allows activation.

## Caller and transaction audit

Read-only source inventory covers factory calls, transaction entry sites and
their important ownership/callback paths. It is not compatibility test evidence.
The inventory receipt contains 423 matching lines, including declarations and
comments, rather than 423 independently verified transactions.

| Actual owner/path | Compatibility assessment |
| --- | --- |
| `persistence/provider.go` | Shared startup writer and separate reader use the two factories; sqlx wrapping and `db.NewPool` do not override mode. Metadata/version checks, pre-migration snapshot and schema owners retain their order. |
| `persistence/sqlite_selection.go`, `maintenance/run.go`, `maintenance/compact.go` | Candidate inspection, dry run and compact inspection use readers. Owned maintenance execution uses the writer and local SQL chunk transactions. Ownership lock, backup and compact behavior stay intact. |
| `testutil/sqlite_template.go`, `testutil/storeconformance/engine.go` | Factory-created templates checkpoint and close before copying. Many fixtures alias one DB as reader/writer: their read-only options become immediate too. Aliases cannot prove real production reader progress. Change only necessary affected fixture construction, never assertions to disguise failure. |
| Task hierarchy and `InspectManagedDeletion` | Read-only authority checks deliberately use writer transactions. They now reserve at BEGIN, including SQLite inspection before rollback. No effects precede admission; failed inspection of an uncertain operation remains unavailable/fail-closed. |
| Task conversation/sidebar/plan snapshots, Office comments, Automation exports | Snapshot helpers use `r.ro`/`s.ro`; real separate reader construction preserves WAL progress. Temporary sidebar scratch remains connection-local with read-only main DB. Automation's file-backed export fixture uses both factories. |
| Agent settings/Office startup migrations | Profile/budget rebuilds do local SQL through one transaction, not nested writer checkout. Office routine rebuild changes foreign_keys on an owned connection before BeginTx and restores it after settlement. Transactional profile reads use the supplied handle. |
| Office coordinator/configsync/scheduler and Task session/runtime | Current SQL operations retain one supplied transaction. Coordinator admission uses a derived 5-second context; busy entry can consume that bound, with existing caller-cancellation/contention classification. No new instant-cancel claim. |
| Plugin state/CommandStore/instances and Canvas | Revision/receipt/admission callbacks use the supplied Tx and Tx-specific methods; no nested writer transaction was found in reviewed callback paths. Notifications follow settlement. |
| `plugins/instances.RemoveArtifactIfUnreferenced` | Existing filesystem callback is already inside a transaction AFTER a real cleanup-job UPDATE acquires the writer. Earlier BEGIN admission adds no new external-I/O lock span after that write; leave this existing cleanup engine intact. New managed deletion still performs no external I/O under SQL locks. |
| GitHub ResetPRWatch, Secrets transfer, Task inventory repair | Existing owned-connection explicit BEGIN IMMEDIATE or separately configured immediate offline writer remains unchanged. They do not nest BeginTx in an open manual transaction; do not replicate these into a new framework. Read-only backup sources remain read-only. |

The pinned driver maps `_txlock=immediate` to `BEGIN IMMEDIATE` in `sqlite3.go`,
returns a real `SQLiteTx` only after successful entry, and exposes `ConnBeginTx`
in `sqlite3_go18.go`. sqlx v1.4.0 `BeginTxx` delegates to `DB.BeginTx` and wraps
its returned Tx; it does not rewrite the transaction mode. No production driver
registration overriding this boundary was found. Same-process single-writer
checkout already serializes its users; the change moves independent writers'
reservation to BEGIN, including writer transactions which only read. No concrete
source blocker was found; compatibility and production wait tests remain gates.

## Cancellation and connection ownership

On entry error, use the actual BeginTxx error and no nonexistent Tx rollback;
`database/sql` releases its connection on failed driver begin. Cancellation
before entry must yield no predicates, barrier or effects. Cancellation during
a native busy wait may settle only as the busy handler returns, up to the
configured 5-second timeout plus bounded scheduling overhead. Do not promise
immediate interruption. Test this while the holder remains locked; the earlier
diagnostic released its holder and did not prove that latency.

A driver begin can win near cancellation. If a genuine Tx is returned, check
context before business admission, defer normal rollback, and join automatic
context rollback or explicit settlement before closing/reusing the pool.
`database/sql` schedules rollback for a returned context-bound Tx; do not depend
on it without observing settlement. Cancellation during pending BEGIN must not reach committed admission or
external effects; native predicate/commit errors retain their actual outcome
classification. Test failed-BEGIN reuse, busy
expiry, cancelled entry, post-entry rollback/cancel and successful reuse through
the actual factory. No manual BEGIN/ROLLBACK recovery or pooled-connection leak.
Commit uncertainty after native admission or final deletion retains the existing
owner/marker reconciliation rules; cancellation does not grant deletion authority.

## Consequences

Independent writers now wait at supported native transaction entry before
current reads. Short read-only work placed on the writer can contend with other
writers earlier; callers should choose the existing reader for snapshots.
Startup/migrations and injected alias fixtures receive the same transaction
policy. Separate WAL readers must continue progressing while a writer holds its
transaction and another writer waits. Unrelated ordinary-task and legacy cleanup
semantics remain, with this explicitly widened writer scheduling compatibility
scope. Focused factory, snapshot, startup, maintenance, state and conformance
checks are required before delivery; this source audit is not their substitute.

Platform owns this operational invariant; Plugins owns the managed lifetime
outcomes. Reconcile the existing Platform persistence design and focused Plugins
supplement rather than creating another incident requirement or work order.
No schema, wire, UI, locale, setting, new dependency or cleanup engine is added.
Plan a concise backend AGENTS note and correct the existing writer-pool comment
so single-connection serialization is not described as eliminating all BUSY.
Public-doc audit retains the plugin admitted-failure wording and adds concise
writer/read-pool guidance to the existing backend-development explanation; no new operator configuration is needed.

## Alternatives considered

- Deferred zero-row UPDATE reservation: actual isolated comparisons returned
  immediate BUSY; changing the updated column did not establish waiting.
- Updating the retained row as a reservation: also failed to wait and can fire
  real FTS/audit triggers; modifies behavior without solving admission.
- Per-feature immediate pool or typed manual BEGIN adapter: duplicates connection
  ownership and cannot safely preserve genuine sqlx/database/sql Tx semantics.
- ROLLBACK;BEGIN inside Tx, driver-internal mutation or generic transaction
  framework: violates supported ownership or expands scope without necessity.
- Busy retries, longer external SQL locks or bare guarded deletion: do not couple
  lifecycle authority/current state safely and honestly with cleanup effects.

## References

- [Platform persistence design](../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-writer-transaction-admission)
- [Plugins requirements](../specs/plugins/requirements/managed-deletion-admission.md)
- [Plugins design](../specs/plugins/system-design/managed-deletion-admission.md#sqlite-writer-factory-boundary)
- [Single work order](../plans/managed-deletion-admission/task-01-guard-managed-deletion.md#sqlite-factory-extension)

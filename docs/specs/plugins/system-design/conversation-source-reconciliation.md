---
status: current
system: plugins
created: 2026-09-16
owners:
  - kandev
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
---

# Conversation source reconciliation

## Purpose and authority

This design replaces the durable conversation transport introduced by PR #3588.
The [Host requirement](../requirements/prompt-history-extraction-host.md) owns the public boundary.
The [Host prerequisite design](prompt-history-extraction-host.md) retains authority for panels, navigation, display adapters, safe DTOs, and plugin lifecycle.
This document replaces its journal, version, fixed-cutoff, ACK, poison, and delivery-retention sections.
It also replaces the transport mechanics in [Conversation recovery](conversation-recovery.md).
The public current-state outcomes of recovery remain required.

The [decision](../../../decisions/2026-09-16-conversation-source-reconciliation.md) explains the storage tradeoff.
The implementation and its focused tests now match this design.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-002 | Source reads, Scope and authorization, Facade reconciliation |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-005 | Compatibility, Core delivery |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-006 | Revision model, Incremental delivery, Facade reconciliation |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-007 | Upgrade and cleanup, Operational evidence |

## Current boundaries

- `internal/task/repository/sqlite/conversation_source.go` owns the revision table, revision-only triggers, and source-backed reads.
- `internal/task/repository/sqlite/conversation_receipts.go` returns transient mutation receipts from transaction-bound message and turn writers.
- `internal/plugins/conversation_handlers.go` owns source route authorization, safe DTO mapping, and revision guards.
- `internal/gateway/websocket/conversation_delivery.go` publishes post-commit v2 receipts to core and plugin scopes.
- `internal/gateway/websocket/client.go` rejects stale ordered session subscriptions after cutover.
- `apps/web/lib/plugins/conversation-source-scope.ts` and `conversation-host.tsx` own source reads, revision reconciliation, and terminal removal.
- `apps/web/lib/ws/client.ts` owns core source projection and recovery coordination.
- `apps/web/hooks/domains/session/use-session-messages.ts` and the existing message/turn hydration owners repair core state.

## Revision model

Use a `conversation_session_revisions` table in the task repository.
It contains `session_id` as primary key and a nonnegative `BIGINT revision`.
A foreign key removes the revision with its session. No payload, event ID, timestamp history, or per-message version is stored.
An existing session without a revision row reads as revision zero.
Initialization creates schema and triggers without enumerating messages or turns.

Small database triggers advance the session revision on relevant message and turn inserts, updates, and deletes.
They replace payload-copy triggers at final cutover.
They cover direct SQL, tool-payload retention, bulk operations, and FK cascades without relying on event publication.
A move between sessions advances both surviving session revisions.
Trigger upserts select an existing session before insertion, so cascading session deletion cannot recreate an orphan revision.
Session absence itself is the terminal signal. No durable deletion tombstone is needed.

Source mutation and revision increment share the transaction.
Concurrent PostgreSQL writers serialize through atomic row upserts and updates.
Callers that control multiple session mutations acquire revision locks in deterministic session order.
Arbitrary bulk SQL can still deadlock on PostgreSQL. A deadlock aborts the whole transaction and uses the existing caller retry policy.
SQLite uses its existing single-writer contract.
A failed mutation or trigger rolls back both source and revision changes.
Tests cover real multi-connection PostgreSQL writes and cascades.

A process boot epoch supplements the database revision in Host-only responses.
A changed epoch forces reconciliation after restart or restore.
Revision tokens serialize as decimal strings to avoid JavaScript integer precision loss.
No client can select a past revision for an as-of read.
No message content hash, hash column, hash backfill, or per-write content hashing is added.
Cryptographic signatures on authorization and cursor tokens remain unrelated to content hashes.

### Mutation receipts

Normal message and turn writers return a transient mutation receipt from the source transaction.
It contains the session, base revision, committed revision, and every operation represented by that revision interval.
Operations carry stable IDs, previous selection/order fields, and the resulting persisted record or deletion identity.
The receipt is an in-memory result. No receipt, changed-ID journal, or old payload is persisted.

The transaction creates the revision row lazily when absent, then locks it before reading its base value.
It captures the final revision and source result before commit, after all relevant triggers run.
Publication occurs only after commit. A later database read cannot assign a revision to an older event payload.
This prevents a concurrent writer from making incomplete data appear current.

The normal single-record writer accounts for one revision increment.
A transaction with several increments must describe the complete interval, or publish a reset notification instead of a certified change batch.
Bulk writers and cascades without complete receipts use reset or periodic discrepancy detection.
Direct SQL still increments the revision even without any receipt or bus publication.
It therefore leaves a detectable gap. A process crash after commit has the same recovery behavior.
Cross-session moves produce a receipt per surviving session and preserve deterministic lock order.

Migrate message creation, content append, tool updates, permission/clarification updates, and turn completion to this receipt boundary.
Do not duplicate original source writes to capture receipts.
Uninstrumented exceptional paths remain correct through recovery, but normal streaming cannot use reset as its default.

## Source reads

Extend `ConversationReader` through the task service and repository interfaces.
Remove plugin SQL access after caller migration.
The implemented methods read a message page, a turn page, and a revision/status tuple.
Private expansion routes are `GET /api/plugins/{pluginId}/conversation/v2/task-sessions/{sessionId}/{messages|turns|revision}`.
The existing binding route remains unchanged. Final source readers remain on these v2 paths.
Core readers expose revision metadata through their existing authorized first-party handlers.
Each page reads source rows, session existence, and revision in one short database transaction.
SQLite uses a consistent read transaction. PostgreSQL uses read-only repeatable-read isolation.
No transaction spans HTTP requests.

Message order stays `(created_at,id)`. Preserve durable `prompt_seq`, tri-state task selection, author filters, and sort direction.
Message and turn queries use bounded pages with a maximum of 100 rows.
Turn keysets use `(started_at,id)` and the same task selection and revision binding.
The Host assembles turn pages internally while the public turns hook keeps its existing shape.
Safe DTO mapping occurs after authorized source reads. System blocks and arbitrary metadata remain excluded.

The Host-only v2 page envelope adds `{epoch, revision, cursor, hasMore}` to the existing records.
Signed cursors bind the user, plugin generation, session, task selection, filter, sort, page size, keyset position, epoch, and expiry.
The current 10-minute cursor lifetime remains a bound, not a snapshot retention promise.
Cursors identify a query and keyset boundary, not a frozen revision.
A continuation separately sends the scope's fully applied revision as `expected_revision`.
The page transaction compares that revision with its source revision.
A mismatch returns private `409/reconciliation_required`, with no partial page commit.
The Host first drains already received contiguous batches and retries once with its newer applied revision.
If coverage is incomplete, it repairs the loaded range before continuation.
Ordinary applied updates therefore do not invalidate every pagination cursor or force a full refresh.
The Host handles that condition internally. Exhausted recovery maps to existing retryable `upstream_failure`.
Malformed or mismatched tokens remain `400/invalid_query`. Expired tokens trigger one fresh authorized read cycle in the Host.
No cutoff renewal route or stored token cursor is needed after migration.

## Scope and authorization

Preserve authenticated plugin binding, active-generation checks, and `api_read:messages`.
Every read, revision check, and subscription applies current session authorization.
No task or session selection can expand through a caller-provided token.
Bindings remain separate from pagination. The signing key remains because binding and cursor signatures still need it.

The cache key includes panel, plugin generation, session, tri-state task selection, authors, sort, and page size.
Each panel owns its request generation and abort controller.
Unsubscribe, disable, uninstall, reload, and session switch cancel reads and invalidate pending commits.

Fresh inaccessible or nonexistent lookups return the same `404/not_found` envelope.
For a handle with a previous successful authorized read, a later 404 closes the handle and stops reads.
It retains committed rows and exposes terminal `removed` state with the not-found error.
This state means that the scope is no longer available. It does not distinguish deletion from revoked access.
A 401 closes authorization with `unauthenticated`. No background retries continue after either terminal authorization outcome.
No durable receipt, retained deletion payload, or existence-disclosing lookup is added.

## Incremental delivery

Use the Host-only v2 subscription actions `session.conversation.subscribe` and
`session.conversation.unsubscribe` with `protocol_version: 2`.
Plugin requests carry plugin ID, generation, binding token, and Host-created scope identity.
Core requests carry their distinct Host-created scope identity. The server rejects mixed identity branches.
The server binds plugin scopes to their task selection and author filters before readiness.
Readiness identifies the subscription, process epoch, and current revision. It does not prove that a client loaded the source state.

`session.conversation.changed` carries `{epoch, base_revision, revision, operations}` and the bound subscription identity.
Each operation is a message or turn upsert, or a removal by ID.
Upserts carry the current full safe DTO, not a text append delta.
Repeating an upsert replaces the same ID instead of appending text again.
Payloads are transient network data, not database copies or retained event history.

The gateway projects receipts through the scope's authorization and selection rules.
An update that leaves a filter becomes removal. An update that enters a filter becomes an upsert.
An unrelated change sends an empty operations list for its revision interval.
That empty batch proves the interval has no effect on the selection without exposing another task's IDs or content.
Only a validated, complete receipt can produce such a coverage batch.
The plugin sees safe DTOs only. Core scopes use their existing richer authorized projections.

The gateway can coalesce adjacent, fully accounted intervals for at most 100 ms.
It can retain only the last operation per entity when the batch preserves the final selected state and all covered revisions.
It must not coalesce across a missing interval or omit a relevant removal.
Uncertain coalescing produces reset, not a false claim of coverage.
An empty batch can be coalesced with adjacent covered batches without a payload read.

### Applied revision and discrepancy checks

Each scope tracks `appliedRevision` and `observedRevision` separately.
A successfully committed source snapshot establishes `appliedRevision = R`.
Only complete contiguous change batches or another source snapshot can advance it.
A revision check or readiness message advances only `observedRevision`.

For a batch with base B and final revision R:

- If B equals `appliedRevision`, apply all selected operations atomically, then set `appliedRevision = R`.
- If R is no greater than `appliedRevision`, discard it as already covered.
- If B exceeds `appliedRevision`, buffer briefly for an out-of-order predecessor, without claiming freshness.
- If the interval partially overlaps applied state, repair rather than infer which operations are safe to skip.
- If the epoch or scope differs, reject the batch and use the appropriate lifecycle recovery.

An empty operations list follows the same rules. It advances coverage without changing records or triggering history reads.
After revision 10, receiving only a batch covering 11 to 12 cannot hide the missing 10 to 11 batch.
A check reporting revision 12 also cannot advance the applied revision from 10.

Revision observations use `session.conversation.changed` with `check: true`, equal
`base_revision` and `revision`, and an empty operations list. These frames report
source state without proving delivery of any revision interval.
A `terminal: true` frame closes only the identified subscription when its session
or authorization is no longer available. It contains no message or turn payload.

One gateway worker checks revision and existence for subscribed sessions every five seconds.
It batches bounded session-ID queries, rechecks authorization, and reads no payloads.
No subscribers means no checks. Checks equal to the applied revision cause no history reads.
A newer observed revision waits up to one second for pending delivery before requesting recovery.
An older revision or different epoch triggers a fresh authorized snapshot.
Reconnect and visibility return also reconcile before claiming freshness.

Per-scope buffering is bounded by one second, 256 operations, and 1 MiB of encoded payload, whichever limit occurs first.
A larger batch, overflow, missing predecessor, malformed batch, or explicit reset schedules source recovery.
Large individual messages remain readable through source pages. The bounded live path can reset for them.
The existing socket backpressure path disconnects if it cannot deliver a reset safely.
No durable ACK, replay log, poison queue, or permanent per-consumer cursor is introduced.

## Facade reconciliation

### Initial load and repair

Subscribe before reading source records. Buffer transient batches within the limits above during hydration.
Fetch the requested message range and turn pages at one common revision R.
If pages disagree, discard the staged cycle and start a fresh cycle.
Commit matching message pages, turns, cursor, and revision atomically for the scope.
Discard buffered batches already covered by R, then apply contiguous later batches.
Compare the resulting applied revision with the latest observed revision before claiming freshness.
A gap schedules repair. Polling detects loss after the final comparison.

Repair replaces the loaded range by authoritative IDs, including deletions.
For a deleted boundary record, retain its key values as the range boundary.
Fetch from the current head through the retained boundary, with the equivalent upper boundary for ascending order.
A changed ordering key requires range repair unless the projection can prove complete range membership.
Do not initialize or mutate `messages.bySession` for a plugin read.

### Normal live changes

A matching message upsert replaces or adds that ID in deterministic order.
A removal deletes that ID. A turn upsert changes only that turn and the derived duration state.
Existing rows outside the loaded range remain unloaded.
New head records enter the loaded range without discarding older loaded records.
A move across selection boundaries uses the receipt's previous selection fields to remove obsolete entries.
Unchanged filtered projections receive coverage-only batches and perform no history read.

Live content updates do not refresh messages or turns in bulk.
A prompt-only panel ignores agent output content, while its turn subscription still receives completion changes.
Unusual range-membership changes, missing coverage, and explicit reset use repair.

### Pagination and failure

`loadMore` extends the range by one requested page using the applied revision and signed keyset cursor.
Buffer batches during the request. Commit the page at its returned revision, then apply contiguous later batches.
Concurrent calls join. The result counts newly projected IDs against the original call's committed view.
A cursor does not require renewal merely because a fully applied message update advanced the revision.

Only one repair runs per scope. Additional discrepancies set one dirty bit.
Automatic repair starts at most once per second per scope.
Each cycle attempts at most three consistent reads with existing bounded retry delays.
Persistent churn retains committed state and exposes retryable failure rather than false freshness.
The next scheduled check or explicit retry starts another bounded cycle.
Reconnect, expiry, authorization loss, removal, and generation changes retain the lifecycle rules in this design.

## Core delivery

Core consumers use the same source-backed v2 subscription as plugin consumers.
Preserve low-latency streaming through the same transaction-bound mutation receipts and applied-revision accounting.
Core projections retain rich DTOs and optimistic-message rules. Plugin projections remain sanitized and cache-isolated.
Use the existing core hydration coordinator only for initial load or repair.
A normal contiguous event updates its ID and does not invoke broad hydration.

Remove the old journal adapter and avoid parallel legacy-plus-v2 projection of the same message.
Other non-conversation WebSocket traffic remains outside this revision protocol.
Transport deduplication is in-memory. The system does not promise durable exactly-once event delivery.

Recovery refreshes messages and turns and removes stale cached server rows.
Preserve pending local messages and invalidate unloaded cache windows under existing pagination ownership.
Buffer only bounded transient batches during recovery, then discard covered revisions and apply contiguous successors.
Consumers without a mounted hydration owner remain dirty until an owner mounts.
Terminal session removal cancels hydration and prevents resurrection.

Audit all message and turn publishers when removing journal fanout.
Normal source writers need complete receipts, not a second best-effort payload path with independently sampled revisions.
Include task deletion, workspace cascades, Quick Chat, Office, workflow cleanup, and direct repository deletion in revision/existence tests.
Exceptional deletions without receipts recover through the revision check or session absence.
No new universal deletion outbox is required for a current-state view.

## Compatibility

The public SDK hook and task-panel shapes do not change.
`PLUGIN-API.md` must describe current-state pagination and the replacement Host-only wire protocol at cutover.
Private wire v2 is the production conversation transport after this sequential
implementation package.
The final release contains one synchronization implementation and no fallback journal.
No intermediate work order is independently released.

An old browser with v1 state receives an explicit unsupported-protocol outcome after cutover.
An existing browser tab requires a full reload to fetch the matching frontend. Document this upgrade step.
Compatibility-only legacy fixtures do not define the production conversation contract.
Reconnect and reload reauthorize plugins before data access.
Old plugin bundles still use the compatible Host facade supplied by the current application.
The existing minimum Host version is not changed merely because an internal transport changes.

Desktop remains a dockview panel. Mobile retains Panels navigation, its inset picker, full-height panel, and one scroller.
No markup, controls, touch behavior, navigation layout, or copy change is proposed.
Existing mobile fixture tests prove the shared data path with touch navigation.
No new UI preview is necessary for this internal state replacement.

## Upgrade and cleanup

The package ships one forward migration, not a bare Git revert.
Stop old backend processes before migration. Existing startup backup and migration admission remain in force.
No mixed-version writers or rolling old/new backend deployment is supported during this cutover.

Replace `initConversationJournalSchema` in schema initialization before it can run a historical backfill.
The cutover transaction removes the exact legacy SQLite triggers and PostgreSQL triggers/functions.
It installs revision-only triggers and removes the five legacy tables:
`conversation_session_streams`, `conversation_session_events`, `conversation_message_versions`,
`conversation_turn_versions`, `conversation_journal_meta`, plus no unrelated tables.
Use catalog-aware, explicit names. Do not use a wildcard or PostgreSQL `CASCADE` that can remove unrelated dependencies.
All source rows, indexes, and prompt sequences remain intact.
Use the repository migration context and propagate failures before readiness.

Fresh, fully migrated, partially backfilled, and partially present legacy schemas take the same idempotent path.
Failure injection between DDL steps proves rollback. Reopen and repeat prove replay safety.
Large legacy table removal can still cause one-time I/O. No row decoding, copying, DELETE loop, or automatic VACUUM is permitted.

Remove event worker composition, journal DB injection, and every runtime open of the Host event SQLite file.
After database cutover commits, remove only `.host/session-events.sqlite` and its `-wal` and `-shm` siblings.
Validate that each target is the expected regular file under the configured plugin root. Refuse symlinks and unexpected paths.
Do not remove `.host` itself, `conversation-token.key`, plugin storage, packages, approvals, or unrelated files.
File cleanup is idempotent and runs without opening or parsing the legacy database.
A file cleanup failure logs the exact non-content path and leaves an explicit retry on the next startup.
It does not restart the obsolete writer or roll back the successful primary cutover.

Existing backup policy remains unchanged. PostgreSQL backups remain operator-managed.
SQLite DROP frees pages for reuse but does not guarantee a smaller database file.
Disk reclamation uses the existing operator compaction workflow after backup.
Never trigger startup VACUUM to improve a size screenshot.
Downgrade after cleanup requires a pre-upgrade backup and matching binary, not a journal recreation migration.

## Operational evidence

Log migration duration, cleanup completion/failure, and revision-refresh failures without payloads or credentials.
Use existing structured logging. No session IDs as metric labels and no new diagnostics dashboard.

A disposable benchmark records 10k and 100k messages with fixed 1 KiB content before and after cutover.
Record database/WAL bytes, allocated/free pages, cold and repeated startup time, and source-write latency.
Also record history-read count and bytes for a large loaded prompt range during agent streaming.
After initial hydration, 100 contiguous agent updates must cause zero history rereads in a prompt-only scope.
Matching message updates and turn completion must update by ID without a bulk refresh.
Injected delivery loss must cause bounded repair and restore correct visible state.
Separate backup time, cleanup time, and ordinary boot time.
Structural gates require zero legacy payload tables/files after successful cleanup and no per-message plugin-history writes.
Revision-row count cannot exceed surviving session count and cannot grow with repeated writes to one session.
Timing results inform review. They do not invent a hardware-independent startup SLA.

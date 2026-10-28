---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-004
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
---

# Backend agent delivery retention system design

## Boundary and evidence

Platform owns retained delivery evidence; Tasks owns schema, lifecycle authenticates traffic, and orchestrator owns queue/workflow authority.
Extends [delivery](durable-agent-delivery.md), [reattachment](durable-agent-reattachment.md) and [stream processing](durable-agent-stream-processing.md).
The [checkpoint ADR](../../../decisions/2026-10-05-backend-delivery-retirement-checkpoints.md) records replay; the [barrier ADR](../../../decisions/2026-10-10-delivery-event-retention-barriers.md) defines scoped serialization.
Contiguous checkpoints replace permanent generic receipts; parent delivery boundaries remain authoritative.

[Payload proof](backend-agent-delivery-retention-payloads.md) and
[control eligibility](delivery-stream-storage.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-DELIVERY-RETENTION-001 | Policy and protected evidence; data model |
| REQ-PLATFORM-DELIVERY-RETENTION-002 | Hash contract; replay and ACK recovery |
| REQ-PLATFORM-DELIVERY-RETENTION-003 | Settlement dependencies; serialization and crashes |
| REQ-PLATFORM-DELIVERY-RETENTION-004 | Session/task retirement and orphan cleanup |
| REQ-PLATFORM-DELIVERY-RETENTION-005 | Worker budgets; storage bounds; physical reclamation |
| REQ-PLATFORM-DELIVERY-RETENTION-006 | Migration and rollback; operations and surfaces |

## Policy and protected evidence

Eligibility depends on durable dependencies, including stopped/idle/archived/disconnected sessions.
Capture database/UTC time per batch; future timestamps and backward clock movement postpone cleanup. Durations never override dependencies.

| Category | Logical policy | Evidence retained |
| --- | --- | --- |
| Nonterminal inbox payload | Remove one hour after projection, only through contiguous projected sequence and after its canonical/required effect commits. | Envelope, versioned normalized hash and projection time until replay-window expiry; then checkpoint. |
| Terminal inbox payload | Same grace, but only after full terminal settlement and required workflow completion/no-op are proved. | Immutable terminal summary/effect for session lifetime; recent envelope/hash, then checkpoint. |
| Prompt payload | Remove one hour after fully settled completed/failed/cancelled outcome. | Original ID, session/incarnation/harness/owner generation, dispatch attempt, hash, state, outcome, times and terminal association. |
| Generic completed `agent_delivery.event` receipt | Delete only with retirement of its associated contiguous inbox prefix. | Recent hash receipt or retirement checkpoint prevents event reapplication. |
| Terminal effect | Keep compact identity/outcome/turn and completion state for session lifetime, including superseded terminals. | Late exact block binding, latest-sequence selection, queue settlement and terminal deduplication. |
| Workflow effect | Keep the existing authoritative receipt until its owning turn/session is explicitly deleted. | Durable effect key and completed state; pending/unknown effects remain protected. |
| Cursor | Keep for every retained stream/generation until explicit session deletion. | Received, projected, remote high-water and new retirement prefix; never reset to zero. |

Keep at most 10,000 eligible compact events/stream or 24 hours, whichever expires first.
Raw grace, unprojected and protected records are additional; limits never cross protected gaps.
Submission/terminal/workflow summaries have no TTL while their session exists; generic streaming receipts are not permanent.

Projected nonterminal output from an active or uncertain prompt can lose redundant raw content only after canonical persistence and its required effect commits.
Keep its submission payload and terminal/recovery dependencies.
Unprojected tool updates, permission state, turn/configuration events or unknown event/effect types remain intact unless the consumer proves durable application.
The collector has an explicit known-type eligibility classifier. An unknown state/type is a skip, never a best guess.

### Settlement dependencies

For terminal and prompt payload cleanup, require all of the following:

1. Exact terminal identity is within the contiguous projected cursor.
2. Submission state/outcome matches authoritative terminal evidence, including cancellation and superseding terminals.
3. Terminal effect is `completed` with a non-null completion time. `pending` and `block_resolved` are protected.
4. No unresolved Queue or Send Now claim refers to that dispatch attempt/submission.
5. No matching open recovery block or reconnecting/uncertain notice requires this evidence.
6. Required workflow intents are durably applied or a durable guarded no-op records why no transition applies.

Persist submission `fully_settled_at` only at this boundary, plus immutable terminal stream/sequence/turn binding.
This differs from existing `settledAt` in terminal settlement, which timestamps outcome/block resolution rather than all dependencies.
A superseded terminal alone cannot prove full settlement. Preserve terminal-first completion when prompt RPC returns after projection/pruning. Use `session.go` structural JSON CAS for notice updates; reordered keys must not defeat exact-value matching.

Workflow receipts currently lack reliable session identity. New receipts must bind their owning turn/session before any ownership-based cleanup.
Backfill only through a unique existing canonical turn, without parsing arbitrary effect keys as authority.
Unbound legacy receipts remain; report their count. Do not expire them by age.
The retention worker never resolves blocks.
Task 04 adds the missing no-op writer to `workflowEffectForTurn`. Failed provider turns certify no successful workflow action; deferred recovery still pins needed evidence.
Each evaluated terminal records a session/incarnation/harness/submission/turn-bound authoritative outcome: applied transition or guarded no-op.
Reuse the existing workflow effect key/type and `Outcome` field; fill its existing identity fields and commit `completed` plus timestamp. Do not create a second receipt store.
No-workflow/quick-chat sessions and scheduler-owned Office terminals certify `not_required`; workflow-routed Office turns still require their actual workflow decision.
Successful evaluation with no actions or only RequiresApproval-skipped actions records `no_automatic_transition`; it does not grant approval or consume a later human action.
Clarification, missing completion signals, queued-input drainage, pending moves and read/engine failures remain deferred, never no-op.
The writer rechecks exact turn/generation and workflow-policy revision under existing admission plus shared delivery locks; losing that comparison leaves evidence protected.
Persist the receipt with its guarded decision; receipt failure blocks full settlement. Recovery reuses the original bound decision, never evaluates against a successor step.
Retention never creates workflow receipts.

## Data model and hash contract

Extend existing task-owned tables additively:

- Inbox: `content_hash`, `hash_version`, `payload_pruned_at`, `payload_bytes`.
- Submission: `payload_pruned_at`, `payload_bytes`, `fully_settled_at`, `terminal_stream_id`, `terminal_sequence`, `terminal_turn_id`.
- Cursor: `retired_sequence BIGINT NOT NULL DEFAULT 0`.
- Task-owned `agent_delivery_cleanup_jobs`: session ID, exact incarnation, retirement time, phase, last-key progress and bounded error code. One unique job per deleted identity.
- Task-owned `agent_delivery_retention_state`: singleton format version, `pruning_started_at`, sweep phase and typed keyset positions. These are recovery/progress records, not a flag.

NOT NULL BLOB/BYTEA payloads use empty bytes after pruning, with explicit `payload_pruned_at`.
A missing marker with empty bytes is not a tombstone. Payload loaders return typed `payload_pruned` for callers that need content.
Summary readers continue returning state/hash/identity without reconstructing a prompt from canonical history.
Add bounded metadata-only pagination for adoption; do not decode historical prompt payloads to establish active submission identity.
Unknown format/version fails before prompt admission or cleanup.

Hash version 1 covers a length-framed identity envelope and SHA-256 of normalized transport content.
Frame stream, sequence, session, incarnation, harness generation, submission, event type and terminal bit.
Reuse the exact semantics of `sameAgentDeliveryPayload` and `stripLifecycleDeliveryFields`.
Only reconstructed `ControlTurnID`, `TurnID`, `CanonicalMessageID`, `CanonicalProjection` and `CanonicalMessageAppend` are omitted.
Adapter content stays strict, including source diagnostics omitted from canonical notifications; never hash reconstructed canonical output. Use current typed AgentEvent decoding/marshaling.
For undecodable JSON use a distinct tagged raw-byte hash; equal raw bytes remain duplicates and altered raw bytes conflict.
Version and framing prevent ambiguity between raw and normalized bytes. Prompt hashes retain their existing immutable composition algorithm.

Hash on receive; validate hashes/bytes before atomic expired-prefix deletion, or backfill before retained-payload clearing.
Normalization failure or a mismatched persisted hash keeps the payload and exposes a bounded corruption error.
Do not change old duplicate semantics accidentally when replacing byte comparison.

## Replay and ACK recovery

Authenticate instance/environment owner generation, session, incarnation, registered harness and exact stream before checkpoints. Historical replay follows owner policy without runtime mutation.
Pass execution/owner/session/incarnation/harness/stream/submission into shared SQL admission. Payloads/hashes confer no authority. Keep submission owner generation/hash.

Connect updates before ACP for MCP. Buffer retained frames until negotiation and SQL commit for generation one or an explicit interrupted-native successor. Cancellable waits cap at thirty seconds.
Publish native identity before configuration; check owners before receipt, projection or ACK.
Admit the final handover prompt in SQL and bind its initial ID to the owner/generation; failures block dispatch. Retries use fresh immutable IDs.
Persist the Kandev turn and task/session/execution/incarnation/harness/stream/submission before dispatch. Recovery requires matching SQL and peer proof; mismatches block adoption, absent bindings never invent a turn.
Recovery and unmarked missing authority fail closed; events never create authority.

Within one serialized receive transaction, apply this order:

1. Validate authenticated owner/session/generation and deletion admission. Do not create a cursor merely because a row is absent.
2. Load the existing stream checkpoint or create it only for a verified new stream whose first sequence is valid.
3. If `sequence <= retired_sequence`, return typed `delivery_replay_expired` before insertion. Do not acknowledge the incoming event as an identical duplicate.
4. If a row exists with raw payload, use the current identity/content check. With a compact row, compare versioned hash and envelope.
5. For an identical compact duplicate, return `inserted=false` and the committed projected cursor. Do not invoke projection or manufacture terminal effects.
6. For a conflict, return the existing conflict family. For genuinely new later events, use normal contiguous receive and projection.

Missing received receipts are corruption. Missing cursors fail closed, except [verified bootstrap receipt](durable-agent-stream-processing.md#resumed-bootstrap-recovery).
Projection checks compact duplicates/expired prefixes under shared serialization, including callers holding an event fetched before cleanup; no canonical/effect reapplication.

Maintain `0 <= retired_sequence <= projected_sequence <= received_sequence`.
The retired prefix advances only over contiguous, projected, eligible rows whose completed generic effects can be removed atomically.
Keep terminal and workflow summaries even after their inbox row retires.
Gaps and pending dependencies stop the prefix; never leap using MAX(sequence). Until Task 04 supplies full settlement, every terminal stays protected and stops retirement.

Wire v1 stays unchanged: backend expiry is local, distinct from agentctl `cursor_expired`.
Lifecycle closes obsolete replay, authenticates the same owner, reads SQL projected P, sends fenced ACK(P) bounded by peer high-water, and reconnects after P. Never derive ACK from expired content or reset to zero.
Remote ACK confirmation is unnecessary for deletion: retained projection/checkpoints support lost-request/response retry.
Peer rejection, incompatible identity, high-water below P, missing cursor or remote expiration past P keeps recovery blocked. Only the exact-submission reconciler settles outcomes/notices; independent causes remain.
Cleanup/expiry never dispatches recovery. Existing provider continuation retains its independent evidence, authorization and fresh submission IDs; cleanup grants no authority. Initial-prompt metadata/receipts remain outside deletion scope.

## Serialization and crash consistency

All delivery mutations use transaction helpers: receive/project, submission, settlement, block binding and retention. No in-memory mutex replaces SQL.
SQLite uses `db.OpenSQLite` immediate BEGIN and separate readers; no manual BEGIN, row-touch substitute or new pool. PostgreSQL uses neutral advisory keys.
Use existing hierarchy/workspace/step/task and Queue order before delivery sessions, sorted streams and submission/effect rows.
Receive and canonical projection chain turn, delivery-session and stream locks in one lock-only command; hierarchy locks never follow.
No SQL lock crosses HTTP or event-bus work.
Lock by textual session ID before testing incarnation; retirement and recreation share this barrier.
Historical mutations compare `harness_session_generations`, not current-generation defaults.

PostgreSQL reads use fresh READ COMMITTED snapshots after locks; event lock-only commands use ordered MATERIALIZED CTEs.
Nonterminal receive and exact completed generic projection share session barriers; turn/stream locks stay exclusive. Other effects, terminal/authority writers and prompt-selected sessions stay exclusive. Mixed modes follow sorted sessions without upgrades. Per-pool guards do not serialize foreground.
Cleanup uses bounded try-lock/lock-timeout. Foreground retains existing deadlines.
Keep SQLite snapshots on `OpenSQLiteReader`. Cancelled native BEGIN can wait up to the existing 5-second busy timeout; check context before predicates and join failed entry/rollback before reuse.
Unique constraints and CAS remain defensive.

Validate all loaded raw candidates under locks. Delete only the contiguous expired prefix directly; compact other eligible rows with persisted hashes. Deletion, checkpoint, marker and progress commit together. Rollback preserves originals; commit exposes hashes or checkpoint-backed absence.
Lost-response retries are idempotent. Revalidate deletion evidence on the primary writer; stale replicas cannot qualify rows.

Failpoints cover hash validation/backfill, payload clearing or direct-retirement validation, deletion, checkpoint, marker and progress.
Terminal crash coverage must include projection, block_resolved, Queue ACK, Send Now ACK, notice clear and final terminal completion.
All phases retain the original submission/dispatch association.
## Session/task retirement and orphan cleanup

A large stream cannot be purged in a foreground transaction.
Exact-incarnation deletion first creates a retirement job, then commits canonical deletion under the existing task/session locks.
The job targets every delivery incarnation belonging to the deleted textual session ID.
Creation cannot reuse that session ID while its job is pending; after purge it requires a fresh incarnation.
This rule avoids an unbounded historical-generation inventory before its FK cascade.
Deterministic UUIDv5 primary IDs remain: ordinary `agent_conversations.go:createPrimarySession`, managed `managed_conversation_settings.go` through repository `repairManagedPrimaryTx`. Guard both paths under their existing admission.
Ordinary executor and Office creation supply fresh UUIDs; E2E creation leaves ID empty for repository generation. No import/restore ID writer was found.
Apply the admission check to every repository creation variant; test deterministic conversation reconciliation returns a retryable retirement conflict until purge completes.
A delayed deletion with a mismatched live incarnation fails before scheduling a job.

New workflow receipts bind their canonical session/turn on write. Task 04 backfills uniquely attributable legacy receipts before deletion integration.
Capture still-knowable legacy turn associations before canonical cascades; never parse arbitrary keys as authority.
Keep unidentifiable legacy global receipts and report their count.

Wire retirement into session purge, `deleteTaskWithVacatedStep` (including `FinalizeManagedDeletion`), workspace and ephemeral cascades. Managed retirement commits with the final delete/committed marker, never reversible reserved/prepared admission; external cleanup stays outside SQL.
Use existing task/session deletion events. Payload-only cleanup emits no task/session mutation event.
Archive, Stop and runtime parking never create deletion jobs.

Purge inbox/effects, then submissions, cursors and job. Split owned and cursor-attributed effects. Bound cursor pages with `(session_id, stream_id)`; commit `last_stream_id` only after the page has no effects. Empty cursors count as work and outlive receipts.
Recheck session absence each transaction; live rows suspend purge. All four attributable categories reach zero without grace.
Every INSERT requires live session/incarnation under lock, preventing resurrection after job removal. Preserve fresh-incarnation replacements.

Find orphans through bounded session-leading index seeks before checking live ownership. The singleton `last_session_id` cursor advances for live and unknown identities as well as orphans. Event discovery owns separate receipt and compaction cursors. Errors or current-incarnation mismatch do not prove historical absence.
Protect/report unknown provenance; authoritative orphans create durable purge jobs.
Update `internal/backendapp/e2e_reset.go` so fixture reset stops cleanup before deleting data and clears jobs in its documented order.

## Worker budgets and indexes

`internal/task/deliveryretention` owns one Start(ctx)/Stop worker and a shared repository batch operation.
Start background work without an artificial delay; readiness does not await the sweep.
Stop cancels, drains its WaitGroup and supports Stop-to-Start. Include package goleak coverage.
Restore/reset must stop this worker before pool close.

Acquire `maintenance.ForPool(pool).TryAcquire()` per batch; release before waiting. Visit retention, orphan and retirement once per pass; repeat phases with committed cleanup or forward discovery. Recheck quiet phases next pass.
Background event/prompt leases target 50 candidates under configured ceilings. Skip busy maintenance without reporting corruption. Offline execution owns exclusive backend access.
Budgets apply to candidate scanning as well as changed rows:

| Limit | Default | Interpretation |
| --- | --- | --- |
| Candidate rows per transaction | 200 | Includes skipped/protected candidates, not just deletions |
| Serialized payload bytes read/changed | 4 MiB | Use metadata lengths before loading payloads |
| Admitted SQL work | 250 ms | Admit only with a full lease remaining; cancel/rollback on expiry |
| Pass duration | 5 s | Do not start another batch after the deadline |
| Between batches | No sleep | Release locks, check cancellation |
| Idle / productive cadence | 60 s / immediate | [Continuation rules](backend-agent-delivery-retention-throughput.md) |

Use one 250 ms batch context for BEGIN and SQL; admit only when it fits remaining pass time. Never reset it after entry. SQLite native BEGIN can outlast cancellation up to its 5-second busy timeout plus scheduling overhead. Check cancellation before predicates; join entry/rollback before Stop or reuse. Measure admission separately.
The byte budget fits 2 MiB events.
Skip over-budget legacy payloads with diagnostics rather than loading them repeatedly.
Persist fair scan progress; select retirement jobs by indexed oldest `updated_at`, then identity. Committed phase progress rotates jobs.
No OFFSET, per-tick global COUNT, full JSON scan or entire-session submission reload.
Discovery indexes due projection times for raw payloads and stream sequences for compact receipts. Locked reload evaluates proofs and bounded payloads. Separate cursors and alternating priority share 200 candidates. Recent rows cannot skip due payloads.

Add indexes after additive migrations:

- Inbox `(session_id, incarnation_id, stream_id, sequence)` for retirement; `(stream_id, projected_at, sequence) WHERE projected_at IS NULL` for replay.
- Sparse terminal frontier `(stream_id, sequence) WHERE terminal <> 0` avoids scanning nonterminal events to find an unresolved terminal.
- Eligible payload scan `(projected_at, stream_id, sequence)` restricted to `payload_pruned_at IS NULL AND projected_at IS NOT NULL` on both engines.
- Submission `(fully_settled_at, id)` for unpruned terminal payloads; `(session_id, incarnation_id, id)` for purge.
- Effects: `(stream_id, sequence, state)` proofs; nonempty-SID `(session_id, incarnation_id)` identity and `(session_id, effect_key)` purge indexes. Probe with `session_id <> ''`.
- Terminal lookup `(effect_type, submission_id, stream_id, sequence)` for current MAX and exact binding.
- Cursors `(session_id, stream_id)` for bounded attribution; jobs `(phase, session_id, incarnation_id)` and unique identity.

Check indexed query plans on both engines. Hash and update progress once per stream.
MVCC/TOAST and WAL need ordinary maintenance; no worker VACUUM or full-table UPDATE.

Transient lock/busy/serialization errors rollback and retry next pass with jittered cancellable backoff capped at five minutes.
Disk-full, hash corruption, unsupported format or missing required schema suspend destructive work and emit a structural error.
Reset backoff/report recovery only after successful committed cleanup; readiness permits a probe. Keep all recovery guards.

## Migration and rollback

Schema remains required in both transport modes under the existing task owner, descriptor and conformance adapter.
`initSchema` precedes `runMigrations`: new-column indexes follow ADD COLUMN. Use ADR 0027 error classifiers.
Column, index and format readiness block worker and admission on migration failure. Repair prior full indexes atomically under the same name, including unprojected replay membership.

Fresh DBs have format 1 and zero retired sequences. Upgrades add default-zero columns without destructive content backfill.
Existing settlement proofs are unknown until normal reconciliation verifies them; age cannot invent `fully_settled_at`.
Old-schema dry-run returns retention schema unavailable, with no migrations or file creation.
Offline execute takes its verified backup before schema initialization, then validates format and invokes the same predicates.

SQLite's verified upgrade snapshot is the rollback artifact; retain an external copy beyond its two-backup rotation. See [snapshot copy](sqlite-snapshot-copy.md).
PostgreSQL has no automatic upgrade snapshot: public guidance requires a verified `pg_dump` before the pruning upgrade and a tested restore. No pg_dump runs at startup.
Test interrupted schema initialization and repeated startup on old and fresh databases.
Preserve the previous-stable fixture; add active, uncertain and partially settled records to an exact pre-retention fixture.

Release N ships Tasks 01-02: format-1 readers/writers, hashes/checkpoints, compact readers, retirement admission and deletion journaling, without a collector or destructive maintenance.
N+1 activates Tasks 03-08 only after the real N binary passes compatibility tests. N is the rollback target; no flag is used.
N preserves jobs/checkpoints and refuses retired-ID recreation; its paused collector leaves jobs pending until forward upgrade. N must not rehydrate pruned payloads or overwrite newer settlement metadata.
Both release artifacts must be tested on the N+1 database for recovery, new writes, session deletion and subsequent forward upgrade.
Retention is irreversible within the live database once pruning starts.
Rollback readers retain format 1, hashes/checkpoints, deletion fencing and no-resend guards; down migrations cannot restore raw data.

Merged main `d7a44e96ede9`, original `f2668750bf6a` and other pre-retention binaries lack retention readers and can reinterpret deleted rows as new input.
They are explicitly unsupported against a pruned DB; do not claim the marker can make those old binaries refuse startup.
A rollback to them requires stopped admission, resolved/retired work and a verified *pre-pruning* backup, with explicit lost-history implications.
A snapshot can lag external actions. Restoring it cannot justify automatic replay of those actions.
New binaries reject newer formats before readiness. Rollback tests require a real reader/writer binary.

## Operations and surfaces

Stable startup configuration uses `internal/common/config/catalog.go` and `source.go`:

| YAML key under `tasks.deliveryRetention` | Default | Validation |
| --- | --- | --- |
| `payloadGrace` | `1h` | 1 minute to 30 days |
| `replayAge` | `24h` | 1 hour to 30 days |
| `replayEvents` | `10000` | 100 to 100000 |
| `interval` | `1m` | 10 seconds to 1 hour |
| `batchRows` | `200` | 1 to 200 |
| `batchBytes` | `4194304` | 1 MiB to 4 MiB |

Transaction/pass limits remain fixed safety budgets. Zero and negative values fail configuration validation.
Catalog aliases use prefix `KANDEV_TASK_DELIVERY_RETENTION_` with suffixes `PAYLOAD_GRACE`, `REPLAY_AGE`, `REPLAY_EVENTS`, `INTERVAL`, `BATCH_ROWS` and `BATCH_BYTES`, respectively.
No enable boolean, profiles override, SQLite feature toggle or runtime flag is introduced.
Configured values apply on restart and do not reach agentctl child environments.

Offline `maintenance database` adds separate projected-event payload, settled-prompt payload, event-receipt and retired-session categories.
Read-only previews bound retired rows before aggregation across tables and mark sample limits. Pending jobs make execution partial.
Execute calls the shared eligibility/batch logic, not stale candidate-ID deletion without revalidation.
Existing three categories retain their safety behavior. Delivery cleanup can partially complete across bounded transactions; report per-category progress and backup path.
Compaction remains the existing explicitly requested SQLite offline stage. PostgreSQL uses the live worker and operator database tooling; this CLI remains SQLite-only.

Existing Database compaction and desktop/phone history, Stop and state-only retry cover user actions. Cleanup visibility uses logs, with no new Database field or rendered UI.
Routine cleanup adds no notice; expiry maps to existing localized recovery presentation without raw errors.
Operator guidance lives in `docs/public/{operations,configuration,cli,sessions-and-review}.md`; check README and screenshot impact.

## Observability and ownership

Task repository owns database measurements and batch results.
Each cadence emits an aggregate `agent_delivery.retention.*` snapshot: progress, bytes, sample bounds and skip/error reasons, including idle, protected-only and busy passes.
Structural errors use WARN/ERROR; routine snapshots use INFO.
Expvar supplements these logs only in dev/pprof: `/debug/vars` registration is gated in `internal/debug/pprof.go`. There is no production metrics HTTP endpoint.
Offline dry-run supplements logs and database tooling. Journal metric producers remain unchanged.

| Proposed metric | Unit and bounded dimensions |
| --- | --- |
| `agent_delivery_retention_rows_total` | Rows; category `inbox_payload`, `prompt_payload`, `event_receipt`, `orphan`, `deleted_session` |
| `agent_delivery_retention_bytes_total` | Logically removed bytes; category `inbox`, `prompt` |
| `agent_delivery_retention_batches_total` | Batches; result `committed`, `busy`, `deadline`, `error` |
| `agent_delivery_retention_skipped_total` | Candidates; reason `unprojected`, `unsettled`, `claim`, `recovery`, `unknown`, `oversized` |
| `agent_delivery_retention_errors_total` | Errors; reason `lock`, `storage`, `hash`, `schema`, `format` |
| `agent_delivery_retention_backlog_rows` | Periodically bounded/estimated eligible backlog; -1 if unavailable, with sample-limit indicator |
| `agent_delivery_retention_oldest_eligible_age_seconds` | Age; -1 if unavailable |
| `agent_delivery_retention_last_success_unixtime` | UTC seconds; 0 before first successful batch |
| `agent_delivery_retention_batch_duration_seconds` | Fixed buckets through 0.25 seconds plus overflow |

Counters advance after commit; crashes may lose counters, not durable progress. Pinned/unbound counts distinguish capped samples from exact totals.
Keep lag semantics and monotonic positions. Metric labels exclude all identities, paths and payloads.
Access-controlled diagnostics may include needed IDs, never bodies, prompt hashes or credentials.

## Storage bounds and physical reclamation

For a healthy stream, let R be events/second, E average serialized event bytes, G payload grace, L cleanup delay and H compact receipt bytes.
Let N be recent-window count and A its age. Approximate redundant event storage is:

`R * (G + L) * E + min(N, R * A) * H + pinned_event_bytes + cleanup_backlog_bytes`.

At 20/s, 1 KiB, 3600s grace and 60s delay: about 71.5 MiB raw plus 2.44 MiB compact content (N=10000, H=256B), excluding indexes/pages.
At 4 KiB: about 286 MiB raw. The 32B digest plus envelope estimate needs Task 08 measurement.
A million 1 KiB events contain 976.6 MiB; expired transport receipts must not scale with that lifetime count.

Prompt redundancy is terminal prompt rate times `(G + L)` times average prompt bytes, plus fully protected prompt bytes.
Compact evidence grows with submissions, terminal outcomes, turns and streams.
Canonical data follows existing policies. Pins, overload and unavailable cleanup can grow without a fixed cap; work budgets are not quotas.
Alert on eligible age and pinned backlog. Preserve unresolved evidence regardless of size.

SQLite deletion makes pages reusable; ordinary DELETE does not guarantee a smaller file or forensic erasure.
Use the existing verified offline compaction for shrinkage, with staging capacity and rollback artifacts.
See [SQLite VACUUM](https://www.sqlite.org/lang_vacuum.html).
PostgreSQL UPDATE/DELETE leaves dead versions; ordinary vacuum/autovacuum reuses space, while full rewrites require stronger locks and extra disk.
No VACUUM FULL runs in the worker. See [PostgreSQL routine vacuuming](https://www.postgresql.org/docs/18/routine-vacuuming.html).
Backups, WAL and retained copies remain outside logical payload counts and can retain removed bytes.

## Validation and related decisions

The [plan](../../../plans/backend-agent-delivery-retention/plan.md) assigns TDD, engine, fault and storage evidence. Precedents:

- [Harness/session store boundaries](../../../decisions/2026-09-10-durable-harness-session-boundaries.md).
- [Generation-fenced environment ownership](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md).
- [Durable queue admission receipts](../../../decisions/2026-09-14-durable-queue-admission-receipts.md).
- [Replayable schema migrations](../../../decisions/0027-replayable-schema-migrations.md).
- [SQLite writer admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md): reuse native transactions and honest cancellation bounds.
- [Install-wide storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md): host-resource scheduling is separate from internal redundant transport cleanup.
- [PostgreSQL locking](https://www.postgresql.org/docs/18/explicit-locking.html): use transaction-scoped locks, never a session lock leaked into a pool.

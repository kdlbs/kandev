---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
---

# Durable delivery write cost system design

## Purpose and boundaries

The SQLite backend has one writer connection (`internal/db.OpenSQLite`,
`SetMaxOpenConns(1)`). Every interactive mutation and every durable-delivery
write is serialized through it. This design bounds what durable delivery spends
on that writer. It changes the commit pattern and the location of the
power-loss durability boundary. It does not change journal semantics, ACK
cumulative scheduling, projection identity, or the projection-before-ACK rule
in [stream processing](durable-agent-stream-processing.md).

The decision and rejected alternatives are recorded in
[ADR 2026-10-29 coalesced delivery durability barrier](../../../decisions/2026-10-29-coalesced-delivery-durability-barrier.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| AC-PLATFORM-DELIVERY-WRITE-COST-001.1 | Measured baseline; Verification |
| AC-PLATFORM-DELIVERY-WRITE-COST-001.2 | Combined receipt and projection |
| AC-PLATFORM-DELIVERY-WRITE-COST-001.3, AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1 | Durability barrier; Journal release gate |
| AC-PLATFORM-DELIVERY-WRITE-COST-001.4 | Writer synchronous level |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 | Combined receipt and projection |

## Measured baseline

`BenchmarkDeliveryWriterContention`
(`internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go`)
streams synthetic sessions at 100 events/s each against a file-backed database
opened with the production factory and times `Repository.CreateTask`. On the
diagnostic host (loaded, roughly 4.5 ms per fsync), the current path committed
one receive transaction per event plus one projection transaction per batch,
each with WAL `synchronous=FULL`:

| Variant | Sessions | CreateTask p95 | Sustained events/s (offered) |
| --- | --- | --- | --- |
| Current, FULL | 0 | 9 ms | n/a |
| Current, FULL | 4 | 58 to 134 ms | 57 to 111 (400) |
| Current, FULL | 8 | 61 to 113 ms | 205 to 253 (800) |
| Current, NORMAL | 8 | 26 to 32 ms | 646 to 680 (800) |

The writer is saturated below the offered load, so interactive writes queue
behind a permanent delivery backlog. Both the commit count and the per-commit
fsync contribute.

The table was recorded with the draft synchronous producer. The committed
benchmark decouples the offered rate from writer backpressure and reports
`offered-events/s` and `delivery-%` alongside the metrics below.

## Combined receipt and projection

Today `StreamManager.prepareAgentEvent` admits each durable event through
`Repository.ReceiveAgentDeliveryEvent` (one writer transaction) before the
per-stream worker in `stream_event_processor.go` batches compatible events for
`Repository.ProjectCanonicalAgentDeliveryEvents` (a second transaction).

The batch path changes as follows:

- For an event the processor will batch (`streamEventProcessor.batchable`),
  preparation keeps its reader-pool checks (`deliveryEventIsStale`,
  `deliveryEffectAlreadyApplied`) and identity validation but does not write.
  The prepared event carries a deferred-receipt marker.
- `flushBatch` calls a new repository method,
  `ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects,
  remoteHighWater)`, which opens one writer transaction, takes the union of the
  receive and projection lock sets through `lockAgentDeliveryEventWritesMode`,
  runs the existing `receiveAgentDeliveryEventTx` for each event in sequence
  order, reloads the cursor, runs `projectCanonicalDeliveryBatchTx`, and
  commits once. It returns one result per event, including whether the event
  was already projected before this call.
- An event already projected before the call is reported as a duplicate. The
  processor schedules its cumulative ACK and does not notify, matching the
  current duplicate path. A received-but-unprojected event is projected
  normally.
- A typed admission failure from the combined call, including
  `ErrAgentDeliveryReplayExpired` and owner or sequence validation errors, rolls
  the whole transaction back. The processor then replays that batch's events
  one at a time through the existing two-phase path so expiry reconciliation,
  typed recovery, and error reporting stay unchanged. Because the combined
  transaction committed nothing, the fallback starts from the same SQL state.
- Terminal, tool-call, shell-output, permission, and owner-change events keep
  the existing individual path. Batches never contain a terminal event.

PostgreSQL uses the same method. Its advisory locks are acquired once for the
combined transaction in the existing sorted order.

The ACK remains scheduled only after the combined commit, so
AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1 holds: an acknowledged event is
already in the inbox.

## Writer synchronous level

`internal/db.OpenSQLite` returns to `_synchronous=NORMAL`. In WAL mode NORMAL
keeps every commit durable across process termination; only commits not yet
synchronized can be lost on host power loss or an operating-system crash. A
checkpoint synchronizes the WAL before it is backfilled or reset.

Reader DSNs are unchanged. The previous statement that every writer connection
must be FULL is replaced by the barrier below.

## Durability barrier

`internal/db` gains `DurabilityBarrier`, constructed over the shared writer
`*sql.DB`. `Await(ctx)` returns only after a barrier transaction that started
after the call has committed with WAL `synchronous=FULL`. A FULL commit
synchronizes the whole WAL file, so it also makes every earlier NORMAL commit
durable.

A barrier run:

1. Acquires the single writer connection with `DB.Conn`.
2. Executes `PRAGMA synchronous=FULL` on that connection.
3. Commits an upsert of the `durability_barrier` key in the existing
   `kandev_meta` table (owned by `internal/persistence`), writing a
   monotonically increasing generation. A real row change is required: a
   transaction that writes no WAL frame performs no synchronization. No new
   table or schema owner is added.
4. Executes `PRAGMA synchronous=NORMAL` and releases the connection.

If step 2, 3, or 4 fails, the connection is discarded with
`Conn.Raw(... driver.ErrBadConn)` so the pool reopens it with the NORMAL DSN, and
the waiters receive the error.

Coalescing: callers that arrive while no run is in flight start one after a
coalescing window of at most 10 ms. Callers that arrive while a run is in flight
wait for the next run, because the in-flight run may have started before their
commit. One run therefore serves every caller that arrived before it started.
Cancellation of one waiter does not cancel a run that other waiters depend on:
each run executes in a short-lived goroutine, so a caller whose context ends
stops waiting while the shared run continues. The barrier owns no long-lived
goroutine.

On PostgreSQL, `Await` returns immediately: commits already use synchronous
commit, and the existing note that deployments with disabled persistence cannot
claim the power-loss guarantee remains.

The task repository exposes the barrier as
`Repository.AwaitAgentDeliveryDurability(ctx) error`.

## Journal release gate

Every backend request that lets agentctl discard journal data passes through
one lifecycle helper, `deliveryJournalReleaser`, which calls
`AwaitAgentDeliveryDurability` and then the agentctl client method. The current
call sites are:

| Site | Request |
| --- | --- |
| `durable_delivery_ack.go` cumulative ACK worker | `AcknowledgeDelivery` |
| `streams.go` duplicate durable event ACK | `AcknowledgeDelivery` |
| `agent_delivery_retention.go` | `AcknowledgeDelivery` |
| `session.go`, `native_resume_delivery.go` | `RetireDeliverySubmission` |
| `interrupted_submission_retirement.go` | `RetireInterruptedDeliverySubmission` |

A barrier failure is treated like a failed ACK or retire request: the ACK
worker keeps its highest pending cursor and retries with its existing jittered
backoff and bounded error categories (new category `durability_barrier`), and
retire callers return the error to their existing retry or recovery owner.
ACK latency increases by at most the coalescing window plus one fsync. That is
not on an interactive path.

A structural test in `internal/agent/runtime/lifecycle` fails if production
code outside the releaser calls `AcknowledgeDelivery`,
`RetireDeliverySubmission`, or `RetireInterruptedDeliverySubmission` on an
agentctl client, so a new release path cannot bypass the gate.

## Observability

- `agent_delivery_durability_barrier_total` with closed `result` keys
  `success` and `error`. It counts awaited journal releases, so a coalesced run
  can serve several counts.
- `agent_delivery_durability_barrier_seconds` with fixed buckets through 1 s.
  The duration includes the caller's coalescing wait.
- `refreshAgentDeliveryLag` is unchanged by this design.

## Verification

`BenchmarkDeliveryWriterContention` keeps a `baseline` variant that reproduces
the pre-change pattern (separate receive and projection commits, FULL) and adds
the combined and final variants. Each stream offers 100 events/s from a
decoupled producer (reported as `offered-events/s`), and its ACK worker calls
`AwaitAgentDeliveryDurability` every 20 ms from a separate goroutine. The
`final` variant reports the delivered share as `delivery-%`. AC-PLATFORM-DELIVERY-WRITE-COST-001.1 is evaluated from
one run that contains both the baseline and the final variant. The benchmark is
evidence, not a CI gate.

On Linux, `strace -f -c -e trace=fsync,fdatasync` over the final variant confirms
that fsync calls scale with barrier runs rather than with commits.

---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
---

# Delivery retention throughput design

## Boundary

This extends the [retention design](backend-agent-delivery-retention.md) for scheduling.
Eligibility, hash proofs, terminal/prompt protection, replay authority, persisted
cursors and repository transactions remain owned by that design and its payload
companion. It changes neither canonical history nor recovery authority.

## Scheduling

Remove the artificial initial delay. Start under the existing worker lifecycle. Keep the
50-candidate background lease, configured ceilings,
4 MiB byte bound, 250 ms transaction admission and 5-second pass budget. Release the maintenance gate
between batches and check cancellation; introduce no timed yield.

A pass returns an internal ContinuationPending hint only when it exhausts its budget
with committed cleanup or forward discovery progress and queued phase work remaining.
Check admission budget before popping the next phase so a deadline cannot erase
pending work. Repeat productive phases fairly across retention, orphans and retirements.
Never infer continuation from global counts or cursor magnitude alone.

Continue immediately after such a productive pass, without creating a timer. Use configured Interval
(default 60 seconds) after idle/exhausted passes and busy-only passes. Existing
structural/transient failure backoff takes precedence and cancellation interrupts all
waits. A protected frontier that advances can continue; wrapping without forward
progress drains the phase queue and returns to idle cadence. Persisted repository
progress makes restart safe; this scheduling hint is process-local and disposable.

A batch deadline halves that phase's process-local candidate ceiling, down to one,
for subsequent attempts. It never raises the configured ceiling or the 50-candidate
event ceiling. Other phases keep their own limits. Failed transactions still count
as failures and use the existing retry backoff; parent cancellation and other errors
do not reduce the ceiling. A new worker starts from the configured limits. This
allows a heavy page to make progress within the unchanged transaction and byte
budgets without weakening its eligibility proofs or persisting tuning state.
Batch-failure logs include the phase, previous and next candidate ceilings, and
the repository error; pass snapshots retain their aggregate failure accounting.

Configured interval becomes the idle/recheck interval, not a minimum delay between
productive passes. No additional setting or runtime flag is introduced. Aggregate
structured pass logs include continuation_pending and existing failure/busy counts, without IDs.

## Discovery across immediate passes

The repository returns the committed before/after discovery frontier vector in
`AgentDeliveryRetentionBatchResult`, alongside its batch-local
`DiscoveryAdvanced` hint. The vector contains compaction
`(projected_at, stream_id, sequence)`, receipt `(stream_id, sequence)` and
prompt submission ID frontiers. Use explicit validity for an absent compaction
frontier. Build both vectors from the same progress transformation used for the
transactional write, including empty/wrapped prompt and receipt resets. Publish
these values only after commit; rollback returns no progress evidence.

The worker keeps a component-wise maximum vector for one uninterrupted chain
of immediate passes. Seed it from the first committed batch's before vector.
Count discovery as productive only if at least one after frontier exceeds its
corresponding maximum, then advance each maximum independently. A wrapping
frontier never lowers that source's maximum. Phase preference changes are not
frontier movement. Comparing only to the previous batch, or resetting maxima at
each five-second pass, lets staggered wraps repeatedly certify old records.

Keep the maxima across `ContinuationPending` passes, including passes with
actual removals. Dispose of them when entering an idle, busy or error wait,
on cancellation or on worker restart. Persisted scan positions remain the
restart authority and allow lower keys, newly eligible rows and records with
changed protection to be revisited on later idle cycles. The vector is
process-local scheduling evidence, never deletion or recovery authority; do not
persist it, log its identifiers or add metric labels for its contents.

Committed cleanup continues to requeue its phase independently of discovery.
Retain the existing phase rotation, admission limits and error precedence.
A static protected-only inventory must eventually produce no novel frontier
and drain the retention phase, even when its three sources wrap out of phase.
A stream with new higher-key candidates may continue discovering within the
existing pass bounds. A later idle cycle must still revisit earlier keys.

## Evidence and criteria

- AC-PLATFORM-DELIVERY-RETENTION-005.1/.005.5/.005.6: fake-clock productive continuation,
  protected forward discovery, wrapped exhaustion, phase fairness, admitted budgets,
  cancellation and maintenance gate contention.
- AC-PLATFORM-DELIVERY-RETENTION-006.3/.006.6: aggregate scheduling logs and existing
  small SQLite/PostgreSQL historical sweep fixtures, including rollback and restart.
- AC-PLATFORM-DELIVERY-RETENTION-006.5: existing desktop/phone history and recovery E2E.

Tests use small fixtures and controlled clocks. Large historical snapshots are opt-in
operational experiments, never million-row CI fixtures. Higher throughput does not
promise a total storage quota or physical file shrinkage.

## Implementation plans

[Database maintenance throughput](../../../plans/database-maintenance-throughput/plan.md).

[Protected discovery continuation repair](../../../plans/delivery-retention-protected-sweep/plan.md).

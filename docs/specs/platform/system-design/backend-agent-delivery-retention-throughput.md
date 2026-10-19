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

Configured interval becomes the idle/recheck interval, not a minimum delay between
productive passes. No additional setting or runtime flag is introduced. Aggregate
structured pass logs include continuation_pending and existing failure/busy counts, without IDs.

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

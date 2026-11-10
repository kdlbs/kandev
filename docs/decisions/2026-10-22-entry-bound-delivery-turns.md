# ADR-2026-10-22-entry-bound-delivery-turns: Entry-bound durable output turns

**Status:** accepted
**Date:** 2026-10-22
**Area:** backend, workflow

## Context

Issue #4705 exposes a producer that commits real output turns without the
workflow stamp completion requires. The existing stamp also includes a ledger
entry ID, but completion currently checks only the step ID. A task revisiting
the same step can therefore accept work from an earlier entry. The requested
repair explicitly requires atomic stamping, stale-entry rejection, and retained
replay on both SQLite and PostgreSQL.

Turn attribution began as best-effort telemetry. Durable canonical projection
already has transactional retry and cannot safely treat missing authorization
evidence as successful completion. Projection holds session/delivery locks;
blocking on a task row afterwards can conflict with task-first workflow writers.

## Decision

New canonical output turns commit their step and entry stamps in the same
transaction as their output, effect receipts, and projection cursor. Attribution
errors roll back that transaction. Existing ordinary telemetry-only turn
creation keeps its best-effort behavior. Existing turns are never re-stamped.

Use the same task-row locking discipline for attribution. On PostgreSQL the
fallback creator attempts that lock without waiting; contention rolls back the
projection and retries outside all locks with cancellable, capped backoff.
Do not add a blocking task-after-session lock edge or a global lock-order refactor.

Completion claims and automatic transitions are fenced by workflow entry as
well as step. The existing JSON bag stores the originating entry, and existing
move-CAS context guards enforce it at commit. Legacy absence cannot authorize a
known entry. Zero-entry tasks remain distinguishable from unknown evidence.
No request gains authority to complete an earlier turn or to create history.

This decision extends [ADR 0015](0015-explicit-completion-signal-for-auto-advance.md)
without changing its final-action and question-barrier rules. Implementation and validation are recorded in the
[repair package](../plans/agent-delivery-turn-step-stamp/plan.md).

## Consequences

Self-woken output can complete its correct entry. A -> B -> A remains stale for
the original turn, and delayed signals cannot advance a later visit. Older
unattributed turns/signals may require a new user-started turn after upgrade.
Contention temporarily delays projection while retaining its inbox event;
cancellation must terminate retry. No schema migration or historical backfill
is required. Real multi-connection PostgreSQL evidence is mandatory.

## Alternatives Considered

- Search backwards for a stamped turn, or read the current step outside the
  transaction: loses the identity of the actual work and admits stale completion.
- Commit an unstamped canonical turn after a read error: permanently loses the
  evidence completion needs even though the durable event could have retried.
- Add a blocking task lock beneath delivery locks: introduces a lock-order cycle.
- Reorder every turn/reconstruction/workflow lock: expands a focused repair into
  a system-wide concurrency migration.
- Compare only step IDs: cannot distinguish separate visits to the same step.
- Backfill existing turns or signals from current state: fabricates provenance.

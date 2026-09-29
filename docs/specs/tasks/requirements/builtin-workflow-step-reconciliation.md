---
status: active
system: tasks
created: 2026-09-28
owners:
  - kandev
---

# Built-in workflow step reconciliation requirements

## Overview

At startup the task repository reconciles `workflow_steps.events` on
system-owned workflow rows that were materialized before their embedded
template declared an action. Two reconcilers do this today: the participant
seat reconciler (`ensure_participant_seat` on-enter actions) and the agent-error
reconciler (`queue_run` on-agent-error actions). Each reconciler edits one row
at a time with an optimistic read-modify-write and a bounded retry budget, and
each is documented as best-effort: a single un-reconciled step must never stop
the backend from starting.

That promise currently covers only a compare-and-swap miss (the row changed
between read and write). A transient database contention error returned by
the same attempt (SQLite "database is locked" / "database table is locked", or
a PostgreSQL serialization, deadlock, or lock-not-available error) is
propagated immediately. Schema initialization stops at the first failing step,
so the backend exits with "failed to initialize schema". The most likely
trigger is SQLite's WAL snapshot conflict: a deferred transaction reads the
row, another process commits a write, and the reconciler's `UPDATE` then fails
with "database is locked" even inside the 5-second busy timeout. A fresh
transaction cures that case.

This requirement makes transient contention a retryable outcome of a
reconciliation attempt, under the same bounded budget and the same
warn-and-continue exhaustion behavior that a compare-and-swap miss already
has.

## Terms

- **Reconciliation target:** one (`workflow_steps` row, declared action) pair.
  A reconciler visits every declared action its template step declares (a seat
  role for the participant seat reconciler, a normalized `queue_run` action for
  the agent-error reconciler) and, for each, every matching row. A row whose
  step declares two managed actions is two targets.
- **Reconciliation attempt:** one read-modify-write of one reconciliation
  target's row in its own database transaction.
- **Row retry budget:** the maximum number of reconciliation attempts for one
  reconciliation target in one reconciler run. It is 5 for both current
  reconcilers. Each target has its own budget; targets on the same row do not
  share one.
- **Compare-and-swap miss (CAS miss):** the attempt's guarded `UPDATE` affected
  zero rows because the stored `events` value no longer equals the value the
  attempt read.
- **Transient contention error:** an error from beginning the transaction,
  reading the row, writing the row, reading the affected-row count, or
  committing, that the repository's existing contention classifier recognizes:
  an error whose text contains `database is locked` or
  `database table is locked`, or a PostgreSQL error with SQLSTATE `40001`,
  `40P01`, or `55P03`. Wrapped errors are classified by the same rule.
- **Non-transient error:** every other error from an attempt, including
  context cancellation, a missing table, malformed stored JSON, and a
  marshalling failure.

## Requirements

### REQ-TASKS-BUILTIN-WORKFLOW-RECONCILE-001: Transient contention does not block startup

**Intent:** A built-in workflow step reconciler that meets transient database
contention retries the row in a fresh transaction and, if contention outlasts
the row retry budget, leaves the row unchanged and lets startup continue.

#### Acceptance criteria

- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.1:** When a reconciliation attempt
  returns a transient contention error at any database operation, the
  reconciler shall roll back that attempt's transaction and start the next
  attempt for the same row in a new transaction that re-reads the stored
  `events` value.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.2:** When a later attempt for the
  same row, within the row retry budget, reads a row that still lacks the
  declared action and its write commits, the stored `events` value shall
  contain the declared action exactly once and every action the concurrent
  writer committed in between.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.3:** A transient contention error
  and a CAS miss shall each consume one attempt from the same row retry budget.
  The reconciler shall not add a sleep or backoff between attempts; waiting on
  a held lock stays with the driver's configured busy timeout.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.4:** When every attempt in the row
  retry budget ends in a transient contention error or a CAS miss, the
  reconciler shall leave that row's stored `events` value as the last committed
  value, emit exactly one warning for that reconciliation target, return
  success for that target, and continue with the next target and the remaining
  startup steps.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.5:** The exhaustion warning shall
  keep its existing message and fields and add `retry_cause`, whose value is
  `lock_contention` when the final attempt ended in a transient contention
  error and `cas_conflict` when it ended in a CAS miss. When `retry_cause` is
  `lock_contention`, the warning shall also carry the final attempt's error in
  the standard `error` field. `retry_cause` is a closed set of those two
  values.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.6:** When a reconciliation attempt
  returns a non-transient error, the reconciler shall return that error without
  a further attempt, preserving today's behavior: schema initialization fails
  and reports it.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7:** When the startup context is
  canceled between attempts, the next attempt's error shall be treated as
  non-transient and returned, so cancellation is never retried or swallowed.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.8:** The participant seat
  reconciler and the agent-error reconciler shall both satisfy
  AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.1 through
  AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7. Any later reconciler that reuses
  the shared step-events read-modify-write shall receive the same attempt
  outcome contract and cannot compile against the pre-change outcome shape.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.9:** A reconciler run in which no
  attempt returns a transient contention error and no attempt ends in a CAS
  miss shall produce the same stored `events` values and log output as before
  this change.
- **AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.10:** Each reconciliation
  target's outcome shall depend only on that target's attempts. A target
  skipped after exhaustion shall not change whether, or how, any other target
  is reconciled, so the order in which rows are visited does not affect the
  stored result.

## Exclusions

- Lock tolerance for any other schema-initialization step. Every other write
  step in schema initialization (for example the built-in step flag healing
  that runs just before these reconcilers) remains fatal on a lock error. Those
  steps do not have a read-then-write upgrade that a fresh transaction cures.
- A total startup-time bound under a persistently held lock. With the SQLite
  busy timeout of 5 seconds, a row under a held write lock can take up to its
  row retry budget times that timeout before it is skipped. A per-run
  contention circuit breaker is not part of this change.
- Changing the row retry budget, the transaction isolation mode (for example
  `_txlock=immediate`), or the SQLite DSN.
- Reconciliation of user-owned workflows (`is_system = 0`), which the
  reconcilers already leave alone.
- The comment fan-out reconciler proposed in PR #3963 (open, not on `main`).
  It inherits this contract through AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.8
  when it lands; this change does not add it.

## System design

See [built-in workflow step reconciliation](../system-design/builtin-workflow-step-reconciliation.md).

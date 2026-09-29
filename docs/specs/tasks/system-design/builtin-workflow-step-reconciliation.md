---
status: current
system: tasks
created: 2026-09-28
requirements:
  - REQ-TASKS-BUILTIN-WORKFLOW-RECONCILE-001
---

# Built-in workflow step reconciliation system design

## Context and boundaries

The task repository (`apps/backend/internal/task/repository/sqlite`) runs the
built-in workflow step reconcilers as unconditional steps of
`initSchemaContext` (`base_schema.go`), after `healBuiltinWorkflowStepFlags`:

1. `healBuiltinWorkflowStepParticipantSeats` (`builtin_workflow_seat_reconcile.go`)
2. `healBuiltinWorkflowStepOnAgentError` (`builtin_workflow_agent_error_reconcile.go`)

`initSchemaContext` returns the first step error. `NewWithDB` wraps it as
"failed to initialize schema" and backend startup exits. That fatal path is
unchanged; this design only changes which attempt errors reach it.

Each reconciler walks embedded templates in declaration order, collects target
rows with `findSystemOwnedWorkflowSteps` once per declared action
(`healStepParticipantSeatRole` per seat role, `healStepOnAgentErrorAction` per
normalized `queue_run` action), and then runs a per-target retry loop
(`healParticipantSeatRowWithRetry`, `healAgentErrorRowWithRetry`) for each
(row, declared action) reconciliation target, with a budget of 5 (`maxParticipantSeatReconcileAttempts`, `maxAgentErrorReconcileAttempts`).
Each attempt calls the shared read-modify-write `tryHealWorkflowStepEvents`
(`builtin_workflow_reconcile.go`) through a per-reconciler wrapper
(`tryHealParticipantSeatRow`, `tryHealAgentErrorRow`) that also hosts the
existing test-only CAS failpoints.

The same repository code runs on SQLite and PostgreSQL. The SQLite writer pool
has one connection with `_busy_timeout=5000` and WAL (`internal/db/sqlite.go`).
The transaction is started with `BeginTxx(r.migrationContext(), nil)`, which is
a deferred transaction on SQLite: the `SELECT` takes a read snapshot and the
`UPDATE` must upgrade it. If another process commits in between, SQLite returns
`SQLITE_BUSY_SNAPSHOT` ("database is locked") without invoking the busy
handler. Only a new transaction clears it.

## Attempt outcome contract

`tryHealWorkflowStepEvents` changes from `(applied, retry bool, err error)` to
`(stepEventsHealOutcome, error)`:

| Field | Meaning |
| --- | --- |
| `applied bool` | The row already had the action, the row does not exist, or the write committed. |
| `retryCause stepEventsRetryCause` | Empty when no retry is wanted; otherwise `cas_conflict` or `lock_contention`. |
| `retryErr error` | The wrapped database error when `retryCause` is `lock_contention`; nil otherwise. |

A non-nil returned `error` is always non-transient and always terminal for the
row. The signature change is deliberate: any caller written against the old
three-value shape, including the comment fan-out reconciler in open PR #3963,
fails to compile until it adopts the outcome contract (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.8).

Classification happens at every database error site inside the helper:
`BeginTxx`, the `SELECT`, the `UPDATE`, `RowsAffected`, and `Commit`. At each
site the helper wraps the error with its existing message, then calls the
existing `isRetryableStateRaceError` (`task.go`). A match returns
`{retryCause: lock_contention, retryErr: wrapped}, nil`; a miss returns the
wrapped error as today. `isRetryableStateRaceError` already matches the SQLite
texts and PostgreSQL SQLSTATEs `40001`, `40P01`, `55P03`, including through
`%w` wrapping. `sql.ErrNoRows`, JSON parse, and JSON marshal paths are not
database contention and keep their current results. A zero-row `UPDATE`
returns `{retryCause: cas_conflict}`.

The deferred `tx.Rollback()` already runs on every early return, so a retried
attempt never reuses its transaction.

The per-reconciler wrappers return the helper outcome unchanged. Their CAS
failpoints return `{retryCause: cas_conflict}` without touching the database.

## Retry loop

Both per-target loops follow one rule set; the implementer can extract a shared
loop helper if it keeps each function inside the Go lint limits, and each
reconciler keeps its own warning fields.

- On `error != nil`: return it immediately (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.6).
  Context cancellation reaches this branch because a canceled
  `migrationContext` fails the next `BeginTxx` with a non-classified error
  (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7).
- On `applied`: return nil.
- On a non-empty `retryCause`: remember the outcome and start the next attempt
  immediately, with no sleep (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.1,
  AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.3). Both causes consume one attempt.
- On an empty `retryCause` without `applied` (not produced today): return nil,
  matching the current `applied || !retry` exit.
- After the budget is exhausted: emit the existing warning with its existing
  message and fields, plus `zap.String("retry_cause", ...)` from the last
  outcome and, for `lock_contention`, `zap.Error(last.retryErr)`; return nil
  (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.4, AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.5).

The retry budget, the loop state, and the exhaustion warning belong to one
reconciliation target: a (row, declared action) pair. This is the existing loop
structure and it does not change. A row whose template step declares two
managed actions (for example two seat roles) runs two independent loops, and
can therefore produce up to two warnings, each carrying its own action fields
(`role`, or `target` and `reason`). Today's embedded templates declare at most
one managed action of each kind per step.

Targets are processed one at a time: actions in template declaration order,
and for each action the rows in the order `findSystemOwnedWorkflowSteps`
returns them. That query (`builtin_workflow_step_rows.go`) has no `ORDER BY`,
so the row visit order is unspecified; this change neither adds nor relies on
one, because each target's loop state is local to that target and a skipped
target does not stop later targets
(AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.10). Re-running a
reconciler is idempotent because each mutator returns `applied` when the
action is already present. Template iteration order and healthy-path log
output do not change when no attempt meets a transient contention error or a
CAS miss (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.9).

## Concurrency

Within one process the reconcilers run serially inside `initSchemaContext` on
the single writer connection, so the concurrent writer is another process on
the same database file (a second backend instance or a CLI write) or, on
PostgreSQL, another session. The CAS guard keeps a concurrent write intact:
every retried attempt re-reads the committed value and re-applies the mutator
to it (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.2).

## Test seam

Two test-only fields on `Repository`, both nil in production and documented
next to the existing failpoints in `base.go`:

- `workflowStepEventsAfterReadHook func(stepID string)` is called by
  `tryHealWorkflowStepEvents` after the `SELECT` succeeds and before the
  mutator and `UPDATE`. It drives the real-driver regressions.
- `workflowStepEventsDBFailpoint func(stepID string, site stepEventsDBSite) error`
  is called immediately before each database operation of an attempt. The
  closed set of sites is `begin`, `read`, `write`, `rows_affected`, and
  `commit`. A non-nil return replaces that operation: the operation is not
  performed and the returned error is handled exactly as that operation's own
  error would be (wrapped with the site's existing message, then classified).
  At `begin` no transaction exists, so nothing is rolled back; at every later
  site the deferred rollback runs as usual. It drives the per-site and
  cancellation regressions, because only the `UPDATE` site can be driven into
  a real driver lock deterministically.

The concurrent writer is a second `*sql.DB` opened on the same database file
with `sql.Open("sqlite3", "file:<path>?_busy_timeout=5000&_foreign_keys=on")`,
`SetMaxOpenConns(1)`, and closed in `t.Cleanup`. It deliberately omits
`_cache=shared`, so it takes file locks independently of the repository
writer; the WAL journal mode is persistent in the file and is inherited. The
file path comes from a fixture helper alongside
`newRepoForBuiltinWorkflowTests` that returns the repository and its database
path; both reconcilers' regressions use this one recipe. Tests use the hook to
commit a different valid `events` JSON value to the same row through that
connection. Each committed value adds a marker action of a type the
reconciler under test does not manage, so the regression can assert that the
concurrent writer's content survives. The repository transaction's `UPDATE` then fails with a real driver
contention error. This reproduces the production failure without a synthetic
error. If the driver returns an error text that `isRetryableStateRaceError`
does not match, that is a specification defect to route back, not a reason to
extend the classifier silently.

Required regressions:

| Scenario | Hook behavior | Expected result |
| --- | --- | --- |
| Transient contention at the write | Commits once, on the first attempt only | Nil return; declared action present once; the hook's marker action present; no warning |
| Persistent contention | Commits a distinct value on every attempt | Nil return; row equals the hook's last value; exactly one warning with `retry_cause=lock_contention` and a non-empty `error` |
| Continuation past a skipped row | Persistent on row A only | Whole reconciler returns nil; row B reconciled |
| CAS exhaustion (existing failpoint) | None | Existing assertions hold; warning adds `retry_cause=cas_conflict` and no `error` |
| Non-transient error | None; row holds malformed `events` JSON | Error returned after one attempt |

Per-site regressions, using `workflowStepEventsDBFailpoint`. Classification
lives in the shared helper, so these may run through either reconciler's
per-target loop; each counts attempts by the number of `begin` site calls:

| Scenario | Failpoint behavior | Expected result |
| --- | --- | --- |
| Transient error at each site (one subtest per site) | Returns `errors.New("database is locked")` at that site on attempt 1 only | Nil return; 2 attempts; declared action present once; no warning |
| Non-transient error at each site (one subtest per site) | Returns a distinct non-classified error at that site on attempt 1 | Returned error satisfies `errors.Is` for the injected error; 1 attempt; row `events` unchanged; no warning |
| Cancellation between attempts (both reconcilers) | The test replaces `repo.migrate` with `db.NewRequiredMigrateLoggerContext(repo.db, nil, ctx)` on a cancellable `ctx`; at `write` on attempt 1 the failpoint calls `cancel()` and returns `errors.New("database is locked")` | Returned error satisfies `errors.Is(err, context.Canceled)`; exactly 2 `begin` site calls and no second `write`; row `events` unchanged; no warning |

The cancellation scenario runs for both reconcilers because each has its own
loop and error branch (AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7).

A table test for `isRetryableStateRaceError` covers the two SQLite texts, the
three PostgreSQL SQLSTATEs, a `%w`-wrapped instance of each, a different
PostgreSQL SQLSTATE, `context.Canceled`, and nil.

Both reconcilers need the transient and persistent regressions, because each
has its own loop and warning fields.

## Observability

The only new signal is the `retry_cause` field, plus `error` for
`lock_contention`, on the existing per-target exhaustion warning. No metric is
added, and no identifier becomes a label.

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.1 | Attempt outcome contract; Retry loop; Test seam (per-site transient scenarios) |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.2 | Concurrency; Test seam |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.3 | Retry loop |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.4 | Retry loop; Test seam (persistent contention scenario) |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.5 | Retry loop; Observability |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.6 | Attempt outcome contract; Retry loop; Test seam (per-site non-transient scenarios) |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7 | Retry loop; Test seam (cancellation scenario) |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.8 | Attempt outcome contract |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.9 | Retry loop |
| AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.10 | Retry loop; Test seam (continuation scenario) |

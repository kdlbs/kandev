---
status: current
system: office
requirements:
  - REQ-OFFICE-COORDINATOR-INSTALL-001
---

# Office Coordinator Install Idempotency System Design

## Purpose and boundaries

Onboarding pre-installs a `Coordinator heartbeat` routine for a coordinator
agent. The installer previously matched an existing routine on workspace +
assignee + canonical name **and** a cron trigger with the exact canonical
expression — a mutable-state match that let a half-finished install (routine
row created, trigger creation failed) or a later trigger edit make the
installer duplicate the routine on the next run. This design redefines
identity as workspace + assignee + canonical name only, completes a
half-finished install instead of duplicating it, and serializes install
attempts across every process sharing the database so two concurrent
installs can never both observe "absent." It does not touch dispatch or any
other trigger-management path; the schedule-state classification it uses to
report an already-scheduled routine is owned by
[routine-schedule-state.md](routine-schedule-state.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COORDINATOR-INSTALL-001` | [Identity and decision flow](#identity-and-decision-flow), [Locking](#locking), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **`RoutineService.CreateDefaultCoordinatorRoutine`**
  (`internal/office/routines/service.go`) — the sole entry point, called by
  `internal/office/agents/service.go` and
  `internal/office/onboarding/service.go`. Runs the routine phase and the
  trigger phase as two separately-locked, separately-committed steps.
- **`Repository.InstallCoordinatorRoutine` /
  `Repository.EnsureCoordinatorTrigger`**
  (`internal/office/repository/sqlite/coordinator_install.go`) — the two
  locked, transactional read-decide-write boundaries the service phases run
  inside.
- **`Repository.WithCoordinatorInstallLock`** (same file) — the shared
  cross-process serialization mechanism both boundaries use, keyed by an
  opaque lock key.
- **`decideCoordinatorTrigger` / `selectOrCreateCoordinatorRoutine`**
  (`internal/office/routines/service.go`) — the decision logic each locked
  boundary runs: create-or-select for the routine phase, create-or-leave for
  the trigger phase.

## Data and contracts

### Identity

`(workspace_id, assignee_agent_profile_id, name=CoordinatorRoutineName)` is
the entire identity. `hasAnyCronTrigger` — any cron trigger, enabled or not,
canonical expression or not — is the only fact the trigger phase uses to
decide whether to act; it never reads `enabled`, `next_run_at`, or the
trigger's expression.

### Lock keys

- `CoordinatorInstallLockKey(workspaceID, agentID, canonicalName)` —
  `"office-coordinator-install:|<workspace>|<agent>|<name>"`, guards the
  routine phase.
- `CoordinatorInstallTriggerLockKey(routineID)` —
  `"office-coordinator-install-trigger:<routine_id>"`, guards the trigger
  phase. Deliberately a distinct namespace so a routine ID can never collide
  with an identity key.
- Both are exported functions, not exported namespace constants, so no
  caller reassembles a key itself — the same rationale
  `internal/workflow/repository.ParticipantRoleSeatLockKey` uses.

### `models.CoordinatorInstallTx`

The tx-scoped writer interface `decide` callbacks use:
`ListTriggersByRoutineID`, `CreateRoutine`, `CreateRoutineTrigger` — all
transaction-bound, so a `decide` callback cannot escape the lock's
transaction boundary.

### `models.ErrCoordinatorInstallContention`

The sentinel error `WithCoordinatorInstallLock` returns when
`coordinatorInstallLockTimeout` (5s, compile-time constant, no
configuration surface) is reached before the lock is won —
distinguished from the caller's own context ending by comparing which
context is actually expired (see
[Failure and recovery](#failure-and-recovery)).

## Control flow

### Identity and decision flow

`CreateDefaultCoordinatorRoutine` runs two phases in sequence, both against
the same captured `now`:

1. **Routine phase** — `InstallCoordinatorRoutine` opens the identity lock,
   lists every routine matching identity (ordered `created_at, id`), and
   hands them to `selectOrCreateCoordinatorRoutine`: zero matches creates a
   routine; one or more selects the earliest, logging a warning (and an
   `expvar` observation) if more than one matched.
2. **Trigger phase** — once the routine phase has committed,
   `EnsureCoordinatorTrigger` opens the routine's own trigger lock, reads its
   triggers, and hands them to `decideCoordinatorTrigger`: any existing cron
   trigger (of any state) is left untouched and its schedule state logged
   (via the shared classifier); no cron trigger creates the canonical one,
   with `next_run_at` computed from the single `now` the outer call captured.

The two phases are separate transactions on purpose: a trigger-creation
failure must never roll back an already-committed routine.

### Locking

`WithCoordinatorInstallLock` runs `fn` inside one transaction, bounded to
`coordinatorInstallLockTimeout` via a context derived from (not identical to)
the caller's context:

- **SQLite:** the writer connection is the only open connection
  (`internal/db.OpenSQLite`'s `SetMaxOpenConns(1)`), and SQLite's own
  file-level lock plus `busy_timeout` already serializes a second process's
  write transaction — opening the transaction is the whole mechanism, the
  same pattern `internal/workflow/repository.EnsureRoleSeat` already
  establishes.
- **PostgreSQL:** a plain transaction does not by itself stop two callers
  both reading "absent" under `READ COMMITTED`, so the transaction
  additionally holds `pg_try_advisory_xact_lock(hashtextextended(lockKey,
  0))`, acquired via a bounded poll (`coordinatorInstallLockPollInterval`,
  25ms) rather than the blocking `pg_advisory_xact_lock`, so the same hard
  acquisition bound applies on both dialects. The lock releases automatically
  at commit/rollback.

The transaction commits when `fn` returns nil, rolls back otherwise.

## Failure and recovery

`reportCoordinatorLockFailure` classifies every non-nil outcome from either
locked phase into exactly one reportable condition, checked in this order:

1. `decision.errored` — `decide` already reported at its own failure site
   (`routine_create_failed`, `trigger_create_failed`, or
   `next_occurrence_failed`); nothing reported again here, even though the
   busy-text/deadline heuristic below could otherwise also match `decide`'s
   own error text.
2. `errors.Is(err, models.ErrCoordinatorInstallContention)` — the lock
   timeout was reached → `contention`.
3. `errors.Is(err, context.Canceled) || errors.Is(err,
   context.DeadlineExceeded)` on the **caller's own** context → `cancelled`,
   distinguishable from contention because `classifyCoordinatorInstallWaitErr`
   checks `callerCtx.Err()` before falling back to the bounded-context
   classification.
4. `!decision.ran` — the pre-`decide` read (identity lookup or trigger read)
   failed before `decide` ever ran → the caller-supplied fallback condition
   (`lookup_failed` or `trigger_read_failed`).
5. Otherwise — `decide` ran and returned nil, so the failure is the
   transaction's own commit → `commit_failed`.

`classifyCoordinatorInstallWaitErr` folds `context.DeadlineExceeded`,
`sql.ErrTxDone` (what `database/sql` surfaces from `Commit` when the bounded
context's own deadline already auto-rolled the transaction back), and the
SQLite `"database is locked"` busy text into the contention bucket, but only
after confirming the caller's own context has not itself ended.

## Persistence

No new tables. Both phases write only `office_routines` and
`office_routine_triggers` through the existing tx-scoped repository methods.
`internal/persistence/requiredstores` / `storeconformance` already register
office's schema owner; this design adds no new one.

## Security

No new authorization surface — the install path runs from onboarding and
agent-provisioning flows that already own the workspace/agent identity they
pass in; an empty `workspace_id` or `agent_id` is rejected before any lock is
acquired.

## Observability

`office_coordinator_install_conditions_total` (`expvar.Map`,
`internal/office/routines/coordinator_install_metrics.go`), labelled
`condition;workspace;assignee`. Conditions: `empty_identity`,
`lookup_failed`, `trigger_read_failed`, `duplicate_matches`,
`schedule_state`, `trigger_completed`, `routine_created`,
`next_occurrence_failed`, `trigger_create_failed`, `routine_create_failed`,
`contention`, `cancelled`, `commit_failed`. Each names a condition
*observed*, not a call that completed successfully — a call ultimately
rejected still counts the conditions it detected before rejecting. Also
emitted as structured (`zap`) log lines at the same sites.

## Related decisions

None — this design does not introduce a durable boundary or contract beyond
what REQ-OFFICE-COORDINATOR-INSTALL-001 itself specifies.

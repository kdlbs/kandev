package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// coordinatorInstallLockTimeout is the hard, non-configurable bound
// AC-OFFICE-COORDINATOR-INSTALL-001.9 requires on acquiring the
// cross-process coordinator-install lock. It is a compile-time constant on
// purpose: no environment variable, configuration key, or runtime flag may
// expose it, because both of the installer's callers run on a user-facing
// onboarding path where an unbounded wait is indistinguishable from a hang.
const coordinatorInstallLockTimeout = 5 * time.Second

// coordinatorInstallLockPollInterval bounds how often the PostgreSQL path
// retries its non-blocking advisory-lock attempt while waiting out
// coordinatorInstallLockTimeout. Not exposed to operators for the same
// reason as the bound itself.
const coordinatorInstallLockPollInterval = 25 * time.Millisecond

const coordinatorInstallLockNamespace = "office-coordinator-install:"

// coordinatorInstallTriggerLockNamespace is deliberately distinct from
// coordinatorInstallLockNamespace so a routine id can never collide with an
// (workspace, assignee, canonical name) identity key.
const coordinatorInstallTriggerLockNamespace = "office-coordinator-install-trigger:"

// CoordinatorInstallLockKey derives the shared lock key a coordinator
// install acquires before reading or writing a (workspace, assignee,
// canonical name) identity's routine, so two concurrent installs for the
// same identity never both observe "absent"
// (AC-OFFICE-COORDINATOR-INSTALL-001.8/.9). One exported function, not an
// exported namespace constant, so no caller reassembles the key itself —
// mirrors internal/workflow/repository.ParticipantRoleSeatLockKey's
// rationale.
func CoordinatorInstallLockKey(workspaceID, agentID, canonicalName string) string {
	return strings.Join([]string{coordinatorInstallLockNamespace, workspaceID, agentID, canonicalName}, "|")
}

// CoordinatorInstallTriggerLockKey derives the lock key EnsureCoordinatorTrigger
// acquires before reading or writing a specific routine's triggers, so two
// concurrent calls for the same routine never both create a canonical
// trigger (AC-OFFICE-COORDINATOR-INSTALL-001.8/.9). Kept separate from
// CoordinatorInstallLockKey because the trigger phase runs in its own
// transaction, after the routine's own transaction has already committed —
// see EnsureCoordinatorTrigger.
func CoordinatorInstallTriggerLockKey(routineID string) string {
	return coordinatorInstallTriggerLockNamespace + routineID
}

// InstallCoordinatorRoutine looks up every existing routine at the given
// (workspace, assignee, canonical name) identity and hands them to decide,
// all inside one transaction that is serialized — across every process
// sharing this database, not merely other goroutines in this one
// (AC-OFFICE-COORDINATOR-INSTALL-001.8/.9) — against every other install
// for the same identity. matches is supplied in office_routines.created_at,
// id order (empty when none exist). decide may use the given tx-scoped
// writer to create the routine; any error it returns aborts the
// transaction, leaving the database unchanged.
//
// Deliberately does not also create the routine's canonical trigger: a
// trigger-creation failure must never roll back an already-decided routine
// (AC-OFFICE-COORDINATOR-INSTALL-001.12), so trigger creation is
// EnsureCoordinatorTrigger's own, separately-committed transaction.
//
// See WithCoordinatorInstallLock for the locking mechanism and how
// ErrCoordinatorInstallContention and ctx cancellation are distinguished.
func (r *Repository) InstallCoordinatorRoutine(
	ctx context.Context,
	workspaceID, agentID, canonicalName string,
	decide func(ctx context.Context, matches []*models.Routine, tx models.CoordinatorInstallTx) error,
) error {
	return r.WithCoordinatorInstallLock(ctx, CoordinatorInstallLockKey(workspaceID, agentID, canonicalName),
		func(txCtx context.Context, tx *sqlx.Tx) error {
			matches, err := r.listMatchingCoordinatorRoutinesTx(txCtx, tx, workspaceID, agentID, canonicalName)
			if err != nil {
				return err
			}
			return decide(txCtx, matches, &coordinatorInstallTxWriter{r: r, tx: tx})
		})
}

// EnsureCoordinatorTrigger reads routine's existing triggers and hands them
// to decide, inside its own transaction serialized — across every process
// sharing this database — against every other call for the same routine id
// (AC-OFFICE-COORDINATOR-INSTALL-001.8/.9). Separate from
// InstallCoordinatorRoutine's transaction so a trigger-creation failure can
// never undo the routine InstallCoordinatorRoutine already committed
// (AC-OFFICE-COORDINATOR-INSTALL-001.4/.12); a caller runs this only after
// InstallCoordinatorRoutine has returned a routine successfully. decide may
// use the given tx-scoped writer to create the canonical trigger; any error
// it returns aborts this transaction only, leaving the routine (and any
// pre-existing triggers) unchanged.
func (r *Repository) EnsureCoordinatorTrigger(
	ctx context.Context,
	routineID string,
	decide func(ctx context.Context, triggers []*models.RoutineTrigger, tx models.CoordinatorInstallTx) error,
) error {
	return r.WithCoordinatorInstallLock(ctx, CoordinatorInstallTriggerLockKey(routineID),
		func(txCtx context.Context, tx *sqlx.Tx) error {
			writer := &coordinatorInstallTxWriter{r: r, tx: tx}
			triggers, err := writer.ListTriggersByRoutineID(txCtx, routineID)
			if err != nil {
				return err
			}
			return decide(txCtx, triggers, writer)
		})
}

func (r *Repository) listMatchingCoordinatorRoutinesTx(
	ctx context.Context, tx *sqlx.Tx, workspaceID, agentID, canonicalName string,
) ([]*models.Routine, error) {
	var routines []*models.Routine
	err := tx.SelectContext(ctx, &routines, tx.Rebind(`
		SELECT * FROM office_routines
		WHERE workspace_id = ? AND assignee_agent_profile_id = ? AND name = ?
		ORDER BY created_at, id
	`), workspaceID, agentID, canonicalName)
	if err != nil {
		return nil, err
	}
	if routines == nil {
		routines = []*models.Routine{}
	}
	return routines, nil
}

// coordinatorInstallTxWriter adapts this transaction's queries to
// models.CoordinatorInstallTx.
type coordinatorInstallTxWriter struct {
	r  *Repository
	tx *sqlx.Tx
}

func (w *coordinatorInstallTxWriter) ListTriggersByRoutineID(
	ctx context.Context, routineID string,
) ([]*models.RoutineTrigger, error) {
	var triggers []*models.RoutineTrigger
	err := w.tx.SelectContext(ctx, &triggers, w.tx.Rebind(
		`SELECT * FROM office_routine_triggers WHERE routine_id = ? ORDER BY created_at, id`), routineID)
	if err != nil {
		return nil, err
	}
	if triggers == nil {
		triggers = []*models.RoutineTrigger{}
	}
	return triggers, nil
}

func (w *coordinatorInstallTxWriter) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	return w.r.CreateRoutineTx(ctx, w.tx, routine)
}

func (w *coordinatorInstallTxWriter) CreateRoutineTrigger(ctx context.Context, t *models.RoutineTrigger) error {
	return w.r.CreateRoutineTriggerTx(ctx, w.tx, t)
}

// WithCoordinatorInstallLock runs fn once inside a single database
// transaction serialized, for the given lock key, against every other
// process sharing this database (AC-OFFICE-COORDINATOR-INSTALL-001.9). The
// key is opaque here — see CoordinatorInstallLockKey and
// CoordinatorInstallTriggerLockKey for the two keying schemes callers use.
//
// On SQLite this is exactly the pattern
// internal/workflow/repository.EnsureRoleSeat already establishes for the
// same class of problem: the writer connection is the only open connection
// (internal/db.OpenSQLite's db.SetMaxOpenConns(1)) and SQLite's own
// file-level lock (that same package's busy_timeout) already serializes a
// second process's write transaction, so opening the transaction is the
// whole mechanism — no advisory lock exists on SQLite, and none is needed.
//
// On PostgreSQL, a plain transaction does not by itself stop two callers
// both reading "absent" under READ COMMITTED, so fn's transaction
// additionally holds a transaction-scoped advisory lock keyed on identity
// (pg_try_advisory_xact_lock, released automatically at commit/rollback).
// It is acquired via a bounded poll rather than the blocking
// pg_advisory_xact_lock precedent uses, so the hard acquisition bound
// applies identically on both dialects rather than only on SQLite's
// already-bounded busy_timeout.
//
// The transaction commits when fn returns nil and rolls back otherwise.
// Returns models.ErrCoordinatorInstallContention when
// coordinatorInstallLockTimeout is reached before the lock is won. Returns
// ctx.Err() when the caller's
// own context was already done — distinguishable from contention because
// the bound is enforced on a context derived from, not identical to, the
// caller's context, so the two can be told apart by which one is actually
// expired.
func (r *Repository) WithCoordinatorInstallLock(
	ctx context.Context,
	lockKey string,
	fn func(ctx context.Context, tx *sqlx.Tx) error,
) error {
	boundCtx, cancel := context.WithTimeout(ctx, coordinatorInstallLockTimeout)
	defer cancel()

	tx, err := r.db.BeginTxx(boundCtx, nil)
	if err != nil {
		return classifyCoordinatorInstallWaitErr(ctx, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if dialect.IsPostgres(r.db.DriverName()) {
		if lockErr := acquirePostgresAdvisoryLock(boundCtx, tx, r.db, lockKey); lockErr != nil {
			return classifyCoordinatorInstallWaitErr(ctx, lockErr)
		}
	}

	if err := fn(boundCtx, tx); err != nil {
		return classifyCoordinatorInstallWaitErr(ctx, err)
	}
	if err := tx.Commit(); err != nil {
		return classifyCoordinatorInstallWaitErr(ctx, err)
	}
	committed = true
	return nil
}

// acquirePostgresAdvisoryLock polls a non-blocking transaction-scoped
// advisory lock until it is won or ctx is done. Using the non-blocking
// pg_try_advisory_xact_lock in a poll loop (rather than the blocking
// pg_advisory_xact_lock) is what lets the caller bound acquisition at an
// exact, dialect-independent duration.
func acquirePostgresAdvisoryLock(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, lockKey string) error {
	for {
		var acquired bool
		if err := tx.QueryRowContext(ctx, db.Rebind(
			"SELECT pg_try_advisory_xact_lock(hashtextextended(?, 0))"), lockKey,
		).Scan(&acquired); err != nil {
			return err
		}
		if acquired {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(coordinatorInstallLockPollInterval):
		}
	}
}

// sqliteBusyErrText matches internal/db's own SQLITE_BUSY detection
// (internal/db/migration_errors.go), duplicated here rather than exported
// cross-package for one string check.
const sqliteBusyErrText = "database is locked"

// classifyCoordinatorInstallWaitErr distinguishes the caller's own context
// having already ended from the bounded wait's own deadline being reached:
// the latter is contention (AC-OFFICE-COORDINATOR-INSTALL-001.13), the
// former is cancellation, reported distinguishably from both contention and
// the plain read/write failures of AC-OFFICE-COORDINATOR-INSTALL-001.7.
// sql.ErrTxDone is included in the contention bucket because it is what
// database/sql surfaces from Commit when boundCtx's own deadline already
// auto-rolled the transaction back underneath it — the same exhausted-wait
// outcome as the deadline case, just observed one call later.
func classifyCoordinatorInstallWaitErr(callerCtx context.Context, err error) error {
	if callerCtx.Err() != nil {
		return callerCtx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrTxDone) ||
		strings.Contains(err.Error(), sqliteBusyErrText) {
		return models.ErrCoordinatorInstallContention
	}
	return err
}

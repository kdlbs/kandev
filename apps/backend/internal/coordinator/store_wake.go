package coordinator

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	wakeRetention = 30 * 24 * time.Hour
	turnRetention = 90 * 24 * time.Hour
)

// takeWakeLock takes the per-coordinator wake lock on tx. On PostgreSQL it is a
// transaction-scoped advisory lock; on SQLite the single writer connection
// already serialises write transactions, so it is a no-op. It must run before
// the coordinator row is read for update.
func (s *Store) takeWakeLock(ctx context.Context, tx coordinatorExec, coordinatorID string) error {
	if !dialect.IsPostgres(s.db.DriverName()) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended('coordinator_wake:' || ?, 0))`), coordinatorID); err != nil {
		return fmt.Errorf("take wake lock: %w", err)
	}
	return nil
}

// WithWakeLock runs fn in one write transaction that holds the coordinator's
// wake lock, then its row lock. It returns ErrNotFound without running fn when
// the coordinator row is absent; an error from fn, or a cancelled ctx, rolls
// back and is returned. fn must not call WithWakeLock again.
func (s *Store) WithWakeLock(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) error {
	if dialect.IsPostgres(s.db.DriverName()) {
		return s.withWakeLockPostgres(ctx, coordinatorID, fn)
	}
	return s.withCoordinatorLockSQLite(ctx, coordinatorID, fn)
}

func (s *Store) withWakeLockPostgres(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin wake lock: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.takeWakeLock(ctx, tx, coordinatorID); err != nil {
		return err
	}
	if err := lockCoordinatorRow(ctx, tx, s.db.Rebind, coordinatorID, true); err != nil {
		return err
	}
	if s.afterLock != nil {
		s.afterLock(ctx)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit wake lock: %w", err)
	}
	return nil
}

// deleteCoordinatorPhase3Rows deletes every phase 3 row of the coordinator
// set selected by scope, denials first because they hang off their turn.
// scope is a coordinator_id predicate ("= ?" or "IN (SELECT ...)") with args.
func deleteCoordinatorPhase3Rows(ctx context.Context, tx *sqlx.Tx, scope string, args ...any) error {
	stmts := []string{
		`DELETE FROM coordinator_turn_calls WHERE turn_id IN (SELECT id FROM coordinator_turns WHERE coordinator_id ` + scope + `)`,
		`DELETE FROM coordinator_unattended_denials WHERE turn_id IN (SELECT id FROM coordinator_unattended_turns WHERE coordinator_id ` + scope + `)`,
	}
	for _, table := range []string{
		"coordinator_unattended_turns", "coordinator_wakes", "coordinator_class_changes",
		"coordinator_class_reviews", "coordinator_pending_changes", "coordinator_turns",
		"coordinator_outcomes", "coordinator_feedback", "coordinator_moveback_seen",
	} {
		stmts = append(stmts, `DELETE FROM `+table+` WHERE coordinator_id `+scope)
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, tx.Rebind(stmt), args...); err != nil {
			return fmt.Errorf("delete coordinator phase 3 rows: %w", err)
		}
	}
	return nil
}

// A delivered or superseded wake is the only record that an episode was
// handled, so it is deleted only once its task can no longer become an own
// task again: no approved create_task proposal of the coordinator names it, or
// the task is ephemeral or gone. An archived task keeps its wakes.
const pruneWakesSQL = `
	DELETE FROM coordinator_wakes
	WHERE status IN ('delivered', 'superseded') AND updated_at < ?
	  AND NOT EXISTS (
		SELECT 1 FROM coordinator_proposals p
		WHERE p.coordinator_id = coordinator_wakes.coordinator_id AND p.task_id = coordinator_wakes.task_id
		  AND p.kind = 'create_task' AND p.status = 'approved')`

const pruneWakesWithTasksSQL = `
	DELETE FROM coordinator_wakes
	WHERE status IN ('delivered', 'superseded') AND updated_at < ?
	  AND (NOT EXISTS (
			SELECT 1 FROM coordinator_proposals p
			WHERE p.coordinator_id = coordinator_wakes.coordinator_id AND p.task_id = coordinator_wakes.task_id
			  AND p.kind = 'create_task' AND p.status = 'approved')
		OR NOT EXISTS (
			SELECT 1 FROM tasks t WHERE t.id = coordinator_wakes.task_id AND t.is_ephemeral = 0))`

// PruneWakeState is the phase 3 retention pass. In one transaction it deletes
// the denials of turns finished more than 90 days ago, those turns, then
// delivered or superseded wakes last updated more than 30 days ago whose task
// can no longer become an own task. Pending wakes and open turns are never
// deleted. It returns the turn and wake rows
// deleted and is idempotent for one now.
func (s *Store) PruneWakeState(ctx context.Context, now time.Time) (turns, wakes int64, err error) {
	tasksExist, err := db.TableExistsContext(ctx, s.db, "tasks")
	if err != nil {
		return 0, 0, fmt.Errorf("check tasks table for wake pruning: %w", err)
	}
	pruneWakes := pruneWakesSQL
	if tasksExist {
		pruneWakes = pruneWakesWithTasksSQL
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin prune wake state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	turnCutoff := now.Add(-turnRetention)
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM coordinator_unattended_denials WHERE turn_id IN (
			SELECT id FROM coordinator_unattended_turns WHERE finished_at IS NOT NULL AND finished_at < ?)`), turnCutoff); err != nil {
		return 0, 0, fmt.Errorf("prune unattended denials: %w", err)
	}
	res, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM coordinator_unattended_turns WHERE finished_at IS NOT NULL AND finished_at < ?`), turnCutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("prune unattended turns: %w", err)
	}
	if turns, err = res.RowsAffected(); err != nil {
		return 0, 0, fmt.Errorf("count pruned turns: %w", err)
	}
	res, err = tx.ExecContext(ctx, tx.Rebind(pruneWakes), now.Add(-wakeRetention))
	if err != nil {
		return 0, 0, fmt.Errorf("prune wakes: %w", err)
	}
	if wakes, err = res.RowsAffected(); err != nil {
		return 0, 0, fmt.Errorf("count pruned wakes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit prune wake state: %w", err)
	}
	return turns, wakes, nil
}

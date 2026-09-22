package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// PromoteCoordinatorSuccessor atomically makes successor the sole primary and
// fences predecessor. Queue transfer is coordinated by the orchestrator while
// both queue admissions are held; this transaction is its reversible prepare.
func (r *Repository) PromoteCoordinatorSuccessor(
	ctx context.Context,
	taskID, predecessorID, successorID, predecessorIncarnation, successorIncarnation, operationID string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCoordinatorHandoffTask(ctx, tx, r.db.DriverName(), r.db.Rebind, taskID); err != nil {
		return false, err
	}
	predecessor, err := readCoordinatorHandoffSession(ctx, tx, r.db.Rebind, predecessorID)
	if err != nil {
		return false, err
	}
	successor, err := readCoordinatorHandoffSession(ctx, tx, r.db.Rebind, successorID)
	if err != nil {
		return false, err
	}
	if coordinatorHandoffReplay(predecessor, successor, taskID, operationID) {
		return false, tx.Commit()
	}
	if !validCoordinatorHandoff(predecessor, successor, taskID, predecessorIncarnation, successorIncarnation) {
		return false, models.ErrCoordinatorHandoffConflict
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET is_primary = 0, updated_at = ? WHERE task_id = ?
	`), now, taskID); err != nil {
		return false, fmt.Errorf("clear coordinator primary: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET is_primary = 1, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ?
		  AND state IN ('IDLE', 'WAITING_FOR_INPUT')
	`), now, successorID, taskID, successorIncarnation)
	if err != nil {
		return false, fmt.Errorf("promote coordinator successor: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return false, models.ErrCoordinatorHandoffConflict
	}
	result, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET route_generation = route_generation + 1,
		    route_state = ?, route_reason = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ?
		  AND route_state = ''
	`), models.TaskSessionRouteStateCoordinatorHandoffFenced, operationID, now,
		predecessorID, taskID, predecessorIncarnation)
	if err != nil {
		return false, fmt.Errorf("fence coordinator predecessor: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return false, models.ErrCoordinatorHandoffConflict
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// RollbackCoordinatorSuccessor restores the exact predecessor only when the
// operation still owns the fence and successor remains primary.
func (r *Repository) RollbackCoordinatorSuccessor(
	ctx context.Context,
	taskID, predecessorID, successorID, predecessorIncarnation, successorIncarnation, operationID string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCoordinatorHandoffTask(ctx, tx, r.db.DriverName(), r.db.Rebind, taskID); err != nil {
		return err
	}
	predecessor, err := readCoordinatorHandoffSession(ctx, tx, r.db.Rebind, predecessorID)
	if err != nil {
		return err
	}
	successor, err := readCoordinatorHandoffSession(ctx, tx, r.db.Rebind, successorID)
	if err != nil {
		return err
	}
	if !coordinatorHandoffReplay(predecessor, successor, taskID, operationID) ||
		predecessor.incarnation != predecessorIncarnation || successor.incarnation != successorIncarnation {
		return models.ErrCoordinatorHandoffConflict
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET is_primary = 0, updated_at = ? WHERE task_id = ?
	`), now, taskID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions
		SET is_primary = 1, route_generation = route_generation + 1,
		    route_state = '', route_reason = '', updated_at = ?
		WHERE id = ? AND task_id = ? AND queue_incarnation_id = ?
		  AND route_state = ? AND route_reason = ?
	`), now, predecessorID, taskID, predecessorIncarnation,
		models.TaskSessionRouteStateCoordinatorHandoffFenced, operationID)
	if err != nil {
		return err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return models.ErrCoordinatorHandoffConflict
	}
	return tx.Commit()
}

type coordinatorHandoffSession struct {
	taskID, incarnation, state, routeState, routeReason string
	primary                                             bool
}

func readCoordinatorHandoffSession(
	ctx context.Context, tx interface {
		QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	}, rebind func(string) string, sessionID string,
) (coordinatorHandoffSession, error) {
	var session coordinatorHandoffSession
	err := tx.QueryRowContext(ctx, rebind(`
		SELECT task_id, queue_incarnation_id, state, is_primary, route_state, route_reason
		FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&session.taskID, &session.incarnation, &session.state,
		&session.primary, &session.routeState, &session.routeReason)
	if errors.Is(err, sql.ErrNoRows) {
		return session, models.ErrCoordinatorHandoffConflict
	}
	return session, err
}

func lockCoordinatorHandoffTask(
	ctx context.Context, tx interface {
		QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	}, driver string, rebind func(string) string, taskID string,
) error {
	query := `SELECT id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(driver) {
		query += ` FOR UPDATE`
	}
	var found string
	if err := tx.QueryRowContext(ctx, rebind(query), taskID).Scan(&found); err != nil {
		return models.ErrCoordinatorHandoffConflict
	}
	return nil
}

func coordinatorHandoffReplay(
	predecessor, successor coordinatorHandoffSession, taskID, operationID string,
) bool {
	return predecessor.taskID == taskID && successor.taskID == taskID && successor.primary &&
		predecessor.routeState == models.TaskSessionRouteStateCoordinatorHandoffFenced &&
		predecessor.routeReason == operationID
}

func validCoordinatorHandoff(
	predecessor, successor coordinatorHandoffSession,
	taskID, predecessorIncarnation, successorIncarnation string,
) bool {
	return predecessor.taskID == taskID && successor.taskID == taskID && predecessor.primary &&
		!successor.primary && predecessor.incarnation == predecessorIncarnation &&
		successor.incarnation == successorIncarnation && predecessor.routeState == "" &&
		(successor.state == string(models.TaskSessionStateIdle) ||
			successor.state == string(models.TaskSessionStateWaitingForInput))
}

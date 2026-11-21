package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) RecoverTaskSessionQueue(
	ctx context.Context,
	scope messagequeue.QueueRecoveryScope,
) (*messagequeue.QueueRecoveryResult, error) {
	if scope.TaskID == "" || scope.WorkspaceID == "" || scope.SourceSessionID == "" ||
		scope.DestinationSessionID == "" || scope.SourceSessionID == scope.DestinationSessionID {
		return nil, messagequeue.ErrQueueRecoveryUnauthorized
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin authorized queue recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateQueueRecoveryScopeTx(ctx, tx, scope); err != nil {
		return nil, err
	}
	result, err := messagequeue.RecoverSessionQueueInTransaction(ctx, tx, r.db, scope)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit authorized queue recovery: %w", err)
	}
	return result, nil
}

func (r *Repository) validateQueueRecoveryScopeTx(ctx context.Context, tx *sqlx.Tx, scope messagequeue.QueueRecoveryScope) error {
	if err := r.lockQueueRecoveryScopeTx(ctx, tx, scope); err != nil {
		return err
	}
	return r.validateQueueRecoverySessionsTx(ctx, tx, scope)
}

func (r *Repository) lockQueueRecoveryScopeTx(ctx context.Context, tx *sqlx.Tx, scope messagequeue.QueueRecoveryScope) error {
	locked, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET updated_at = updated_at WHERE id = ? AND workspace_id = ? AND archived_at IS NULL
	`), scope.TaskID, scope.WorkspaceID)
	if err != nil {
		return fmt.Errorf("lock task for queue recovery: %w", err)
	}
	if rows, err := locked.RowsAffected(); err != nil || rows != 1 {
		return messagequeue.ErrQueueRecoveryUnauthorized
	}
	locked, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET updated_at = updated_at
		WHERE task_id = ? AND id IN (?, ?)
	`), scope.TaskID, scope.SourceSessionID, scope.DestinationSessionID)
	if err != nil {
		return fmt.Errorf("lock queue recovery sessions: %w", err)
	}
	if rows, err := locked.RowsAffected(); err != nil || rows != 2 {
		return messagequeue.ErrQueueRecoveryUnauthorized
	}
	return nil
}

func (r *Repository) validateQueueRecoverySessionsTx(ctx context.Context, tx *sqlx.Tx, scope messagequeue.QueueRecoveryScope) error {
	var destinationPrimary bool
	err := tx.GetContext(ctx, &destinationPrimary, r.db.Rebind(`
		SELECT is_primary FROM task_sessions WHERE id = ? AND task_id = ?
	`), scope.DestinationSessionID, scope.TaskID)
	if errors.Is(err, sql.ErrNoRows) || !destinationPrimary && err == nil {
		return messagequeue.ErrQueueRecoveryUnauthorized
	}
	if err != nil {
		return fmt.Errorf("read queue recovery destination: %w", err)
	}
	var source struct {
		IsPrimary bool                    `db:"is_primary"`
		State     models.TaskSessionState `db:"state"`
	}
	err = tx.GetContext(ctx, &source, r.db.Rebind(`
		SELECT is_primary, state FROM task_sessions WHERE id = ? AND task_id = ?
	`), scope.SourceSessionID, scope.TaskID)
	if errors.Is(err, sql.ErrNoRows) || source.IsPrimary && err == nil {
		return messagequeue.ErrQueueRecoveryUnauthorized
	}
	if err != nil {
		return fmt.Errorf("read queue recovery source: %w", err)
	}
	if source.State != models.TaskSessionStateCompleted && source.State != models.TaskSessionStateFailed &&
		source.State != models.TaskSessionStateCancelled {
		return messagequeue.ErrQueueRecoveryTargetNotTerminal
	}
	return nil
}

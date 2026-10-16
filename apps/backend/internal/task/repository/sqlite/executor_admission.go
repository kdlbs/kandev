package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// AdmitExecutorUse defines the ownership cutover without holding a database
// transaction across remote provisioning or prompt delivery.
func (r *Repository) AdmitExecutorUse(ctx context.Context, executorID, profileID, parentOwner, profileOwner, sessionID, actorID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockExecutorOwnershipTx(ctx, tx, executorID); err != nil {
		return err
	}
	var current string
	if err := tx.GetContext(ctx, &current, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executors WHERE id = ? AND deleted_at IS NULL"), executorID); err != nil {
		return err
	}
	if current != parentOwner {
		return models.ErrExecutorOwnershipChanged
	}
	if profileID != "" {
		if err := tx.GetContext(ctx, &current, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executor_profiles WHERE id = ? AND executor_id = ?"), profileID, executorID); err != nil {
			return err
		}
		if current != profileOwner {
			return models.ErrExecutorOwnershipChanged
		}
	}
	if sessionID != "" {
		result, err := tx.ExecContext(ctx, tx.Rebind("UPDATE task_sessions SET executor_actor_user_id = ? WHERE id = ?"), actorID, sessionID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return models.ErrTaskSessionNotFound
		}
	}
	return tx.Commit()
}

// GetTaskExecutorActor restores trusted attribution for work deferred before a
// session exists. Request metadata is never an identity source.
func (r *Repository) GetTaskExecutorActor(ctx context.Context, taskID string) (string, error) {
	var actor string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind("SELECT executor_actor_user_id FROM tasks WHERE id = ?"), taskID).Scan(&actor)
	return actor, err
}

// HasExecutorRecord includes retired rows so an alias cannot replace a resource.
func (r *Repository) HasExecutorRecord(ctx context.Context, id string) (bool, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind("SELECT COUNT(*) FROM executors WHERE id = ?"), id).Scan(&count)
	return count != 0, err
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) DeleteExecutorIfOwnershipUnmodified(ctx context.Context, id, owner string) error {
	return r.deleteExecutor(ctx, id, &owner)
}

// A delete and an ownership transfer share the parent lock and cutover.
func (r *Repository) DeleteExecutorProfileIfOwnershipUnmodified(ctx context.Context, profile *models.ExecutorProfile, parentOwner *string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockExecutorOwnershipTx(ctx, tx, profile.ExecutorID); err != nil {
		return err
	}
	var owner string
	if err := tx.GetContext(ctx, &owner, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executor_profiles WHERE id = ? AND executor_id = ?"), profile.ID, profile.ExecutorID); err != nil {
		return err
	}
	if owner != profile.OwnerUserID {
		return models.ErrExecutorOwnershipChanged
	}
	err = tx.GetContext(ctx, &owner, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executors WHERE id = ? AND deleted_at IS NULL"), profile.ExecutorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if parentOwner == nil {
		if err == nil {
			return models.ErrExecutorOwnershipChanged
		}
	} else if err != nil || owner != *parentOwner {
		return models.ErrExecutorOwnershipChanged
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind("DELETE FROM executor_profiles WHERE id = ?"), profile.ID); err != nil {
		return err
	}
	return tx.Commit()
}

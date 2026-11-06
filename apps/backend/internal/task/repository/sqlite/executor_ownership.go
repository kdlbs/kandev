package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ensureExecutorOwnershipSchema() error {
	if err := r.ensureExecutorActorSchema(); err != nil {
		return err
	}
	for _, table := range []string{"executors", "executor_profiles"} {
		if err := r.migrate.Apply(table+".owner_user_id", "ALTER TABLE "+table+" ADD COLUMN owner_user_id TEXT"); err != nil {
			return fmt.Errorf("add %s ownership: %w", table, err)
		}
	}
	return nil
}

func (r *Repository) lockExecutorOwnershipTx(ctx context.Context, tx *sqlx.Tx, executorID string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "executor-ownership:"+executorID)
		return err
	}
	return nil
}

func (r *Repository) checkProfileParentOwnerTx(ctx context.Context, tx *sqlx.Tx, executorID, owner string, expected *string) error {
	var parent string
	if err := tx.GetContext(ctx, &parent, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executors WHERE id = ? AND deleted_at IS NULL"), executorID); err != nil {
		return err
	}
	if expected != nil && parent != *expected {
		return models.ErrExecutorOwnershipChanged
	}
	if parent != "" && owner != "" && parent != owner {
		return models.ErrExecutorOwnershipInvalid
	}
	return nil
}

func (r *Repository) profileOwnershipForUpdateTx(ctx context.Context, tx *sqlx.Tx, profile *models.ExecutorProfile) (string, error) {
	if err := r.lockExecutorOwnershipTx(ctx, tx, profile.ExecutorID); err != nil {
		return "", err
	}
	var owner string
	if err := tx.GetContext(ctx, &owner, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executor_profiles WHERE id = ?"), profile.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", repoerrors.ErrExecutorProfileNotFound, profile.ID)
		}
		return "", err
	}
	var expectedParent *string
	if intent := profile.OwnershipIntent; intent != nil {
		if owner != intent.ExpectedOwnerUserID {
			return "", models.ErrExecutorOwnershipChanged
		}
		if intent.OwnerUserID != nil {
			owner = *intent.OwnerUserID
		}
		expectedParent = intent.ExpectedParentOwnerUserID
	}
	if err := r.checkProfileParentOwnerTx(ctx, tx, profile.ExecutorID, owner, expectedParent); err != nil {
		return "", err
	}
	return owner, nil
}

func (r *Repository) executorOwnershipForUpdateTx(ctx context.Context, tx *sqlx.Tx, executor *models.Executor) (string, error) {
	if err := r.lockExecutorOwnershipTx(ctx, tx, executor.ID); err != nil {
		return "", err
	}
	var owner string
	if err := tx.GetContext(ctx, &owner, tx.Rebind("SELECT COALESCE(owner_user_id, '') FROM executors WHERE id = ? AND deleted_at IS NULL"), executor.ID); err != nil {
		return "", err
	}
	if intent := executor.OwnershipIntent; intent != nil {
		if owner != intent.ExpectedOwnerUserID {
			return "", models.ErrExecutorOwnershipChanged
		}
		if intent.OwnerUserID != nil {
			owner = *intent.OwnerUserID
		}
	}
	if owner != "" {
		var count int
		err := tx.GetContext(ctx, &count, tx.Rebind("SELECT COUNT(*) FROM executor_profiles WHERE executor_id = ? AND owner_user_id IS NOT NULL AND owner_user_id <> ?"), executor.ID, owner)
		if err != nil {
			return "", err
		}
		if count != 0 {
			return "", models.ErrExecutorOwnershipInvalid
		}
	}
	return owner, nil
}

func (r *Repository) ensureExecutorActorSchema() error {
	return r.migrate.Apply("task_sessions.executor_actor_user_id", "ALTER TABLE task_sessions ADD COLUMN executor_actor_user_id TEXT NOT NULL DEFAULT ''")
}

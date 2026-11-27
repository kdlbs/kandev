package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/physicalclaim"
)

// guardTaskEnvironmentRepoTx locks both identities before checking a replacement.
func guardTaskEnvironmentRepoTx(ctx context.Context, tx *sqlx.Tx, row *models.TaskEnvironmentRepo) error {
	var oldID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(worktree_id,'') FROM task_environment_repos WHERE id = ?`), row.ID).Scan(&oldID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := physicalclaim.Lock(ctx, tx, oldID, row.WorktreeID); err != nil {
		return err
	}
	var currentID string
	var currentLive bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(worktree_id,''),
 deleted_at IS NULL AND status <> 'deleted' FROM task_environment_repos WHERE id = ?`), row.ID).Scan(&currentID, &currentLive)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if currentID != oldID {
		return models.ErrWorkspaceReuseUnsafe
	}
	slot := physicalclaim.Slot{ID: row.ID, EnvironmentID: row.TaskEnvironmentID, RepositoryID: row.RepositoryID, BranchSlug: row.BranchSlug}
	historical := err == nil && !currentLive
	if historical {
		// A unique live owner outranks retained history. Resolve still rejects
		// multiple live owners and groups that contain only competing history.
		_, err = physicalclaim.Resolve(ctx, tx, oldID)
	} else {
		err = physicalclaim.Check(ctx, tx, oldID, slot)
	}
	if err != nil {
		return err
	}
	if historical && row.WorktreeID == oldID && (row.DeletedAt != nil || row.Status == worktreeRepoStatusDeleted) {
		return nil
	}
	return physicalclaim.Check(ctx, tx, row.WorktreeID, slot)
}

// lockEnvironmentPhysicalClaims takes the complete sorted lock set before any
// individual slot changes, so a multi-repository transition cannot invert it.
func lockEnvironmentPhysicalClaims(ctx context.Context, tx *sqlx.Tx, existing, incoming []*models.TaskEnvironmentRepo) error {
	ids := make([]string, 0, len(existing)+len(incoming))
	for _, rows := range [][]*models.TaskEnvironmentRepo{existing, incoming} {
		for _, row := range rows {
			if row != nil {
				ids = append(ids, row.WorktreeID)
			}
		}
	}
	if err := physicalclaim.Lock(ctx, tx, ids...); err != nil {
		return err
	}
	for _, row := range existing {
		var currentID string
		err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(worktree_id,'') FROM task_environment_repos WHERE id = ?`), row.ID).Scan(&currentID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && currentID != row.WorktreeID) {
			return models.ErrWorkspaceReuseUnsafe
		}
		if err != nil {
			return err
		}
		if err := guardTaskEnvironmentRepoTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) guardEnvironmentPhysicalClaims(ctx context.Context, tx *sqlx.Tx, environmentIDs ...string) error {
	var all []*models.TaskEnvironmentRepo
	for _, id := range environmentIDs {
		rows, err := listTaskEnvironmentReposTx(ctx, r, tx, id)
		if err != nil {
			return err
		}
		all = append(all, rows...)
	}
	return lockEnvironmentPhysicalClaims(ctx, tx, all, nil)
}

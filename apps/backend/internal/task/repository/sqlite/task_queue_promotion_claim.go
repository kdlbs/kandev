package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ClaimQueuePromotionEntry removes the pending promotion only while the
// supplied ledger entry is still current. The task hierarchy lock serializes
// this read and claim with task moves on both SQLite and PostgreSQL.
func (r *Repository) ClaimQueuePromotionEntry(ctx context.Context, taskID string, entryID int64) (bool, error) {
	if taskID == "" || entryID <= 0 {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskHierarchy(ctx, tx, nil, []string{taskID}); err != nil {
		return false, err
	}
	if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return false, err
	}
	current, err := r.getLatestTaskStepTransitionIDTx(ctx, tx, taskID)
	if err != nil {
		return false, fmt.Errorf("read promotion entry: %w", err)
	}
	if current != entryID {
		return false, nil
	}
	claimed, err := r.removeTaskMetadataKeyWithExecutor(ctx, tx, taskID, models.MetaKeyQueuePromotionPending)
	if err != nil || !claimed {
		return claimed, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

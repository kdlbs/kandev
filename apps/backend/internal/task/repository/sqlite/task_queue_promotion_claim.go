package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ClaimQueuePromotionEntry removes a pending promotion only while its entry
// identity is current. A legacy zero-ID claim can remove only an unpinned
// marker. The task hierarchy lock serializes these checks with task moves.
func (r *Repository) ClaimQueuePromotionEntry(ctx context.Context, taskID string, entryID int64) (bool, error) {
	if taskID == "" || entryID < 0 {
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
	allowed, err := r.canClaimQueuePromotionEntryTx(ctx, tx, taskID, entryID)
	if err != nil {
		return false, fmt.Errorf("check promotion entry: %w", err)
	}
	if !allowed {
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

func (r *Repository) canClaimQueuePromotionEntryTx(ctx context.Context, tx stepTransitionTx, taskID string, entryID int64) (bool, error) {
	if entryID == 0 {
		pinned, err := r.queuePromotionEntryPinnedTx(ctx, tx, taskID)
		return !pinned, err
	}
	current, err := r.getLatestTaskStepTransitionIDTx(ctx, tx, taskID)
	if err != nil {
		return false, err
	}
	return current == entryID, nil
}

func (r *Repository) queuePromotionEntryPinnedTx(ctx context.Context, tx stepTransitionTx, taskID string) (bool, error) {
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT metadata FROM tasks WHERE id = ?`), taskID).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	metadata := decodeTaskMetadata(raw)
	marker, ok := metadata[models.MetaKeyQueuePromotionPending].(map[string]interface{})
	if !ok {
		return false, nil
	}
	_, pinned := marker[models.QueuePromotionEntryTransitionIDKey]
	return pinned, nil
}

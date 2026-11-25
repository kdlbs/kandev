package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

const planCommentAttachmentDeliveryMode = "path"

func (r *Repository) replacePlanCommentFiles(ctx context.Context, tx *sqlx.Tx, comment *models.TaskPlanComment) error {
	if comment.AttachmentIDs == nil {
		return nil
	}
	if len(comment.AttachmentIDs) > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	previous := &models.TaskPlanComment{ID: comment.ID}
	if err := plancommenttx.ReadAttachments(ctx, tx, r.db, comment.TaskID, []*models.TaskPlanComment{previous}); err != nil {
		return err
	}
	if err := r.retainPlanCommentFiles(ctx, tx, comment, previous.AttachmentIDs); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_plan_comment_attachments WHERE comment_id = ?`), comment.ID); err != nil {
		return fmt.Errorf("replace plan comment attachment relations: %w", err)
	}
	for position, id := range comment.AttachmentIDs {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO task_plan_comment_attachments (comment_id, attachment_id, position)
			VALUES (?, ?, ?)
		`), comment.ID, id, position); err != nil {
			return fmt.Errorf("store plan comment attachment relation: %w", err)
		}
	}
	return releaseDetachedPlanCommentFiles(ctx, tx, comment.TaskID)
}

func (r *Repository) retainPlanCommentFiles(
	ctx context.Context, tx *sqlx.Tx, comment *models.TaskPlanComment, previous []string,
) error {
	if len(comment.AttachmentIDs) == 0 {
		return nil
	}
	var workspaceID string
	err := tx.GetContext(ctx, &workspaceID, tx.Rebind(`SELECT workspace_id FROM tasks WHERE id = ? AND archived_at IS NULL`), comment.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrAttachmentClaimConflict
	}
	if err != nil {
		return fmt.Errorf("read workspace for plan comment attachments: %w", err)
	}
	identity, _ := authn.IdentityFromContext(ctx)
	now := time.Now().UTC()
	existing := make(map[string]struct{}, len(previous))
	for _, id := range previous {
		existing[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(comment.AttachmentIDs))
	var size int64
	for _, id := range comment.AttachmentIDs {
		if !recordUniqueAttachmentID(seen, id) || id == "" {
			return models.ErrAttachmentClaimConflict
		}
		_, linked := existing[id]
		bytes, err := retainPlanCommentFile(ctx, tx, id, comment.TaskID, workspaceID, identity.UserID, linked, now)
		if err != nil {
			return err
		}
		size += bytes
		if size > models.MaxMessageAttachmentBytes {
			return models.ErrAttachmentTotalTooLarge
		}
	}
	return nil
}

func retainPlanCommentFile(
	ctx context.Context, tx *sqlx.Tx, id, taskID, workspaceID, ownerID string, linked bool, now time.Time,
) (int64, error) {
	attachment, err := loadAttachmentForClaim(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	if attachment.WorkspaceID != workspaceID || attachment.DeliveryMode != planCommentAttachmentDeliveryMode {
		return 0, models.ErrAttachmentClaimConflict
	}
	if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
		return 0, models.ErrAttachmentTooLarge
	}
	if linked {
		if attachment.TaskID != taskID || attachment.State != models.AttachmentStatePlanComment {
			return 0, models.ErrAttachmentClaimConflict
		}
		return attachment.SizeBytes, nil
	}
	if ownerID == "" || attachment.OwnerID != ownerID {
		return 0, models.ErrAttachmentClaimConflict
	}
	return attachment.SizeBytes, claimPlanCommentFile(ctx, tx, taskID, id, now)
}

func claimPlanCommentFile(ctx context.Context, tx *sqlx.Tx, taskID, id string, now time.Time) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_message_attachments SET task_id = ?, state = ?, updated_at = ?
		WHERE id = ? AND state = ? AND expires_at > ?
		AND (task_id = '' OR task_id = ?) AND session_id = '' AND message_id = '' AND queue_id = ''
	`), taskID, models.AttachmentStatePlanComment, now, id, models.AttachmentStateStaged, now, taskID)
	if err != nil {
		return fmt.Errorf("retain plan comment attachment: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return models.ErrAttachmentClaimConflict
	}
	return nil
}

func releaseDetachedPlanCommentFiles(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_message_attachments SET state = ?, expires_at = ?, updated_at = ?
		WHERE task_id = ? AND state = ?
		AND NOT EXISTS (
			SELECT 1 FROM task_plan_comment_attachments r WHERE r.attachment_id = task_message_attachments.id
		)
	`), models.AttachmentStateExpired, now, now, taskID, models.AttachmentStatePlanComment)
	if err != nil {
		return fmt.Errorf("release detached plan comment files: %w", err)
	}
	return nil
}

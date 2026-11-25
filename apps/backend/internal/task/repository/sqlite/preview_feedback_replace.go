package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
)

type previewScreenshotRow struct {
	Kind         models.TaskPreviewFeedbackKind `db:"kind"`
	Comment      string                         `db:"comment"`
	AttachmentID sql.NullString                 `db:"screenshot_attachment_id"`
}

// ReplaceTaskPreviewFeedbackScreenshot swaps a saved screenshot item's
// rectangle and image at an exact version. The new staged image is claimed
// and the previous one expired in the same transaction.
func (r *Repository) ReplaceTaskPreviewFeedbackScreenshot(
	ctx context.Context,
	change *models.TaskPreviewFeedbackScreenshotReplacement,
	ownerID, workspaceID string,
) (*models.TaskPreviewFeedbackSnapshot, []*models.TaskMessageAttachment, error) {
	if err := previewfeedback.ValidateScreenshotReplacement(
		change.CaptureRect, change.ScreenshotAttachmentID, change.Comment,
	); err != nil {
		return nil, nil, err
	}
	tx, release, err := r.beginPlanCommentTx(ctx, change.TaskID)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, change.TaskID); err != nil {
		return nil, nil, err
	}
	current, err := r.previewScreenshotRow(ctx, tx, change)
	if errors.Is(err, sql.ErrNoRows) {
		snapshot, conflictErr := r.commitPreviewFeedbackConflict(ctx, tx, change.TaskID)
		return snapshot, nil, conflictErr
	}
	if err != nil {
		return nil, nil, err
	}
	if current.Kind != models.TaskPreviewFeedbackScreenshot || !current.AttachmentID.Valid {
		return nil, nil, previewfeedback.ErrCaptureInvalid
	}
	previous, err := r.getAttachmentInTx(ctx, tx, current.AttachmentID.String)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	if err := r.claimPreviewScreenshot(
		ctx, tx, change.TaskID, change.ScreenshotAttachmentID, ownerID, workspaceID, now,
	); err != nil {
		return nil, nil, err
	}
	comment := current.Comment
	if change.Comment != "" {
		comment = change.Comment
	}
	if err := r.writePreviewScreenshot(ctx, tx, change, comment, now); err != nil {
		return nil, nil, err
	}
	released, err := r.expirePreviewScreenshots(ctx, tx, previous)
	if err != nil {
		return nil, nil, err
	}
	if err := r.validatePreviewFeedbackCollection(ctx, tx, change.TaskID); err != nil {
		return nil, nil, err
	}
	snapshot, err := r.incrementAndCommitPreviewFeedback(ctx, tx, change.TaskID)
	return snapshot, released, err
}

func (r *Repository) previewScreenshotRow(
	ctx context.Context,
	tx *sqlx.Tx,
	change *models.TaskPreviewFeedbackScreenshotReplacement,
) (*previewScreenshotRow, error) {
	row := &previewScreenshotRow{}
	err := tx.GetContext(ctx, row, r.db.Rebind(`
		SELECT kind, comment, screenshot_attachment_id FROM task_preview_feedback
		WHERE id = ? AND task_id = ? AND version = ?
	`), change.ItemID, change.TaskID, change.ExpectedVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read preview screenshot item: %w", err)
	}
	return row, err
}

func (r *Repository) writePreviewScreenshot(
	ctx context.Context,
	tx *sqlx.Tx,
	change *models.TaskPreviewFeedbackScreenshotReplacement,
	comment string,
	now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_preview_feedback
		SET comment = ?, capture_rect_json = ?, screenshot_attachment_id = ?,
			version = version + 1, updated_at = ?
		WHERE id = ? AND task_id = ? AND version = ?
	`), comment, string(change.CaptureRect), change.ScreenshotAttachmentID, now,
		change.ItemID, change.TaskID, change.ExpectedVersion); err != nil {
		return fmt.Errorf("replace preview screenshot: %w", err)
	}
	return nil
}

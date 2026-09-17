package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const previewFeedbackSelectCols = `
	id, task_id, kind, comment, source_kind,
	COALESCE(source_session_id, ''), source_label, COALESCE(source_path, ''),
	page_route, page_title, COALESCE(selected_text, ''),
	COALESCE(text_anchor_json, ''), COALESCE(element_snapshot_json, ''),
	COALESCE(capture_rect_json, ''), COALESCE(screenshot_attachment_id, ''),
	version, created_at, updated_at`

// ListTaskPreviewFeedback returns one authoritative task-owned snapshot.
func (r *Repository) ListTaskPreviewFeedback(
	ctx context.Context,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, taskID); err != nil {
		return nil, err
	}
	return r.commitPreviewFeedbackRead(ctx, tx, taskID)
}

// CreateTaskPreviewFeedback inserts a caller-identified capture and claims its optional screenshot.
func (r *Repository) CreateTaskPreviewFeedback(
	ctx context.Context,
	item *models.TaskPreviewFeedback,
	ownerID, workspaceID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if err := previewfeedback.ValidateItem(item); err != nil {
		return nil, err
	}
	fingerprint, err := previewFeedbackFingerprint(item)
	if err != nil {
		return nil, err
	}
	tx, release, err := r.beginPlanCommentTx(ctx, item.TaskID)
	if err != nil {
		return nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, item.TaskID); err != nil {
		return nil, err
	}
	storedFingerprint, err := r.previewFeedbackAdmission(ctx, tx, item.ID)
	if err != nil {
		return nil, err
	}
	if storedFingerprint != "" {
		if storedFingerprint == fingerprint {
			return r.commitPreviewFeedbackRead(ctx, tx, item.TaskID)
		}
		return r.commitPreviewFeedbackConflict(ctx, tx, item.TaskID)
	}

	now := time.Now().UTC()
	if err := r.claimPreviewScreenshot(ctx, tx, item, ownerID, workspaceID, now); err != nil {
		return nil, err
	}
	item.Version = 1
	item.CreatedAt = now
	item.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_preview_feedback (
			id, task_id, kind, comment, source_kind, source_session_id, source_label,
			source_path, page_route, page_title, selected_text, text_anchor_json,
			element_snapshot_json, capture_rect_json, screenshot_attachment_id,
			version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), item.ID, item.TaskID, item.Kind, item.Comment, item.SourceKind,
		nullableString(item.SourceSessionID), item.SourceLabel, nullableString(item.SourcePath),
		item.PageRoute, item.PageTitle, nullableString(item.SelectedText),
		previewNullableJSON(item.TextAnchor), previewNullableJSON(item.ElementSnapshot),
		previewNullableJSON(item.CaptureRect), nullableString(item.ScreenshotAttachmentID),
		item.Version, item.CreatedAt, item.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert task preview feedback: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_preview_feedback_admissions
			(id, task_id, request_fingerprint, created_at) VALUES (?, ?, ?, ?)
	`), item.ID, item.TaskID, fingerprint, now); err != nil {
		return nil, fmt.Errorf("insert task preview feedback admission: %w", err)
	}
	if err := r.validatePreviewFeedbackCollection(ctx, tx, item.TaskID); err != nil {
		return nil, err
	}
	return r.incrementAndCommitPreviewFeedback(ctx, tx, item.TaskID)
}

// UpdateTaskPreviewFeedback edits only the user comment; captured page evidence stays immutable.
func (r *Repository) UpdateTaskPreviewFeedback(
	ctx context.Context,
	taskID, itemID, comment string,
	expectedVersion int64,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if err := previewfeedback.ValidateComment(comment); err != nil {
		return nil, err
	}
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, taskID); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_preview_feedback
		SET comment = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND task_id = ? AND version = ?
	`), comment, time.Now().UTC(), itemID, taskID, expectedVersion)
	if err != nil {
		return nil, fmt.Errorf("update task preview feedback: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return r.commitPreviewFeedbackConflict(ctx, tx, taskID)
	}
	if err := r.validatePreviewFeedbackCollection(ctx, tx, taskID); err != nil {
		return nil, err
	}
	return r.incrementAndCommitPreviewFeedback(ctx, tx, taskID)
}

// DeleteTaskPreviewFeedback removes one exact item and expires its task-owned screenshot claim.
func (r *Repository) DeleteTaskPreviewFeedback(
	ctx context.Context,
	taskID, itemID string,
	expectedVersion int64,
) (*models.TaskPreviewFeedbackSnapshot, []*models.TaskMessageAttachment, error) {
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, taskID); err != nil {
		return nil, nil, err
	}
	attachment, err := r.previewScreenshotForItem(ctx, tx, taskID, itemID, expectedVersion)
	if err != nil {
		return nil, nil, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_preview_feedback WHERE id = ? AND task_id = ? AND version = ?
	`), itemID, taskID, expectedVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("delete task preview feedback: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		snapshot, conflictErr := r.commitPreviewFeedbackConflict(ctx, tx, taskID)
		return snapshot, nil, conflictErr
	}
	released, err := r.expirePreviewScreenshots(ctx, tx, attachment)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := r.incrementAndCommitPreviewFeedback(ctx, tx, taskID)
	return snapshot, released, err
}

// ClearTaskPreviewFeedback removes the exact rendered collection revision.
func (r *Repository) ClearTaskPreviewFeedback(
	ctx context.Context,
	taskID string,
	expectedRevision int64,
) (*models.TaskPreviewFeedbackSnapshot, []*models.TaskMessageAttachment, error) {
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.ensurePreviewFeedbackCollection(ctx, tx, taskID); err != nil {
		return nil, nil, err
	}
	var current int64
	if err := tx.GetContext(ctx, &current, r.db.Rebind(`
		SELECT revision FROM task_preview_feedback_collections WHERE task_id = ?
	`), taskID); err != nil {
		return nil, nil, fmt.Errorf("read task preview feedback revision: %w", err)
	}
	if current != expectedRevision {
		snapshot, conflictErr := r.commitPreviewFeedbackConflict(ctx, tx, taskID)
		return snapshot, nil, conflictErr
	}
	attachments, err := r.listPreviewScreenshots(ctx, tx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_preview_feedback WHERE task_id = ?
	`), taskID); err != nil {
		return nil, nil, fmt.Errorf("clear task preview feedback: %w", err)
	}
	released, err := r.expirePreviewScreenshots(ctx, tx, attachments...)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := r.incrementAndCommitPreviewFeedback(ctx, tx, taskID)
	return snapshot, released, err
}

func (r *Repository) ensurePreviewFeedbackCollection(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_preview_feedback_collections (task_id, revision, created_at, updated_at)
		VALUES (?, 0, ?, ?) ON CONFLICT (task_id) DO NOTHING
	`), taskID, now, now)
	if err != nil {
		return fmt.Errorf("ensure task preview feedback collection: %w", err)
	}
	return nil
}

func (r *Repository) previewFeedbackAdmission(ctx context.Context, tx *sqlx.Tx, itemID string) (string, error) {
	var fingerprint string
	err := tx.GetContext(ctx, &fingerprint, r.db.Rebind(`
		SELECT request_fingerprint FROM task_preview_feedback_admissions WHERE id = ?
	`), itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read task preview feedback admission: %w", err)
	}
	return fingerprint, nil
}

func previewFeedbackFingerprint(item *models.TaskPreviewFeedback) (string, error) {
	return plancomments.Fingerprint(struct {
		ID                     string                               `json:"id"`
		TaskID                 string                               `json:"task_id"`
		Kind                   models.TaskPreviewFeedbackKind       `json:"kind"`
		Comment                string                               `json:"comment"`
		SourceKind             models.TaskPreviewFeedbackSourceKind `json:"source_kind"`
		SourceSessionID        string                               `json:"source_session_id"`
		SourceLabel            string                               `json:"source_label"`
		SourcePath             string                               `json:"source_path"`
		PageRoute              string                               `json:"page_route"`
		PageTitle              string                               `json:"page_title"`
		SelectedText           string                               `json:"selected_text"`
		TextAnchor             json.RawMessage                      `json:"text_anchor"`
		ElementSnapshot        json.RawMessage                      `json:"element_snapshot"`
		CaptureRect            json.RawMessage                      `json:"capture_rect"`
		ScreenshotAttachmentID string                               `json:"screenshot_attachment_id"`
	}{
		item.ID, item.TaskID, item.Kind, item.Comment, item.SourceKind,
		item.SourceSessionID, item.SourceLabel, item.SourcePath, item.PageRoute,
		item.PageTitle, item.SelectedText, item.TextAnchor, item.ElementSnapshot,
		item.CaptureRect, item.ScreenshotAttachmentID,
	})
}

func previewNullableJSON(value json.RawMessage) interface{} {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func (r *Repository) claimPreviewScreenshot(
	ctx context.Context,
	tx *sqlx.Tx,
	item *models.TaskPreviewFeedback,
	ownerID, workspaceID string,
	now time.Time,
) error {
	if item.ScreenshotAttachmentID == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_message_attachments
		SET task_id = ?, session_id = '', state = ?, updated_at = ?
		WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
			AND mime_type = 'image/png' AND kind = 'image' AND delivery_mode = 'prompt'
			AND size_bytes > 0 AND size_bytes <= ?
	`), item.TaskID, models.AttachmentStateClaimed, now, item.ScreenshotAttachmentID,
		ownerID, workspaceID, models.AttachmentStateStaged, previewfeedback.MaxScreenshotBytes)
	if err != nil {
		return fmt.Errorf("claim preview screenshot: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return models.ErrAttachmentClaimConflict
	}
	return nil
}

func (r *Repository) previewScreenshotForItem(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, itemID string,
	expectedVersion int64,
) (*models.TaskMessageAttachment, error) {
	var attachmentID sql.NullString
	err := tx.GetContext(ctx, &attachmentID, r.db.Rebind(`
		SELECT screenshot_attachment_id FROM task_preview_feedback
		WHERE id = ? AND task_id = ? AND version = ?
	`), itemID, taskID, expectedVersion)
	if errors.Is(err, sql.ErrNoRows) || !attachmentID.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read preview screenshot reference: %w", err)
	}
	return r.getAttachmentInTx(ctx, tx, attachmentID.String)
}

func (r *Repository) listPreviewScreenshots(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]*models.TaskMessageAttachment, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT a.* FROM task_message_attachments a
		JOIN task_preview_feedback f ON f.screenshot_attachment_id = a.id
		WHERE f.task_id = ?
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list preview screenshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var attachments []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			return nil, fmt.Errorf("scan preview screenshot: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate preview screenshots: %w", err)
	}
	return attachments, nil
}

func (r *Repository) getAttachmentInTx(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
) (*models.TaskMessageAttachment, error) {
	attachment := &models.TaskMessageAttachment{}
	err := tx.GetContext(ctx, attachment, r.db.Rebind(`
		SELECT * FROM task_message_attachments WHERE id = ?
	`), id)
	if err != nil {
		return nil, fmt.Errorf("read preview screenshot: %w", err)
	}
	return attachment, nil
}

func (r *Repository) expirePreviewScreenshots(
	ctx context.Context,
	tx *sqlx.Tx,
	attachments ...*models.TaskMessageAttachment,
) ([]*models.TaskMessageAttachment, error) {
	now := time.Now().UTC()
	var released []*models.TaskMessageAttachment
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_message_attachments SET state = ?, expires_at = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND state = ?
		`), models.AttachmentStateExpired, now, now, attachment.ID,
			attachment.TaskID, models.AttachmentStateClaimed); err != nil {
			return nil, fmt.Errorf("expire preview screenshot: %w", err)
		}
		attachment.State = models.AttachmentStateExpired
		attachment.ExpiresAt = now
		attachment.UpdatedAt = now
		released = append(released, attachment)
	}
	return released, nil
}

func (r *Repository) incrementAndCommitPreviewFeedback(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_preview_feedback_collections
		SET revision = revision + 1, updated_at = ? WHERE task_id = ?
	`), now, taskID)
	if err != nil {
		return nil, fmt.Errorf("increment task preview feedback revision: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, repoerrors.ErrTaskNotFound
	}
	return r.commitPreviewFeedbackRead(ctx, tx, taskID)
}

func (r *Repository) validatePreviewFeedbackCollection(
	ctx context.Context,
	q sqlx.QueryerContext,
	taskID string,
) error {
	snapshot, err := r.readPreviewFeedbackSnapshot(ctx, q, taskID)
	if err != nil {
		return err
	}
	return previewfeedback.ValidateCollection(snapshot.Items)
}

func (r *Repository) commitPreviewFeedbackConflict(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	snapshot, err := r.commitPreviewFeedbackRead(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	return snapshot, repoerrors.ErrTaskPreviewFeedbackChanged
}

func (r *Repository) commitPreviewFeedbackRead(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	snapshot, err := r.readPreviewFeedbackSnapshot(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit task preview feedback transaction: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) readPreviewFeedbackSnapshot(
	ctx context.Context,
	q sqlx.QueryerContext,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	snapshot := &models.TaskPreviewFeedbackSnapshot{TaskID: taskID, Items: make([]*models.TaskPreviewFeedback, 0)}
	if err := sqlx.GetContext(ctx, q, &snapshot.Revision, r.db.Rebind(`
		SELECT revision FROM task_preview_feedback_collections WHERE task_id = ?
	`), taskID); err != nil {
		return nil, fmt.Errorf("read task preview feedback collection: %w", err)
	}
	rows, err := q.QueryContext(ctx, r.db.Rebind(`
		SELECT `+previewFeedbackSelectCols+`
		FROM task_preview_feedback WHERE task_id = ? ORDER BY created_at, id
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task preview feedback: %w", err)
	}
	items := make([]*models.TaskPreviewFeedback, 0)
	for rows.Next() {
		item := &models.TaskPreviewFeedback{}
		var textAnchor, elementSnapshot, captureRect string
		if err := rows.Scan(
			&item.ID, &item.TaskID, &item.Kind, &item.Comment, &item.SourceKind,
			&item.SourceSessionID, &item.SourceLabel, &item.SourcePath,
			&item.PageRoute, &item.PageTitle, &item.SelectedText,
			&textAnchor, &elementSnapshot, &captureRect, &item.ScreenshotAttachmentID,
			&item.Version, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task preview feedback: %w", err)
		}
		item.TextAnchor = previewRawJSON(textAnchor)
		item.ElementSnapshot = previewRawJSON(elementSnapshot)
		item.CaptureRect = previewRawJSON(captureRect)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task preview feedback: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task preview feedback rows: %w", err)
	}
	for _, item := range items {
		if item.ScreenshotAttachmentID == "" {
			continue
		}
		item.ScreenshotAttachment, err = r.getAttachmentQuery(ctx, q, item.ScreenshotAttachmentID)
		if err != nil {
			return nil, err
		}
	}
	snapshot.Items = items
	return snapshot, nil
}

func (r *Repository) getAttachmentQuery(
	ctx context.Context,
	q sqlx.QueryerContext,
	id string,
) (*models.TaskMessageAttachment, error) {
	attachment := &models.TaskMessageAttachment{}
	if err := sqlx.GetContext(ctx, q, attachment, r.db.Rebind(`
		SELECT * FROM task_message_attachments WHERE id = ?
	`), id); err != nil {
		return nil, fmt.Errorf("read task preview screenshot descriptor: %w", err)
	}
	return attachment, nil
}

func previewRawJSON(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}

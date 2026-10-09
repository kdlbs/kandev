// Package previewfeedbacktx resolves and consumes task-owned preview feedback
// inside the final message or queue admission transaction.
package previewfeedbacktx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Resolution is the immutable evidence selected under the task admission lock.
type Resolution struct {
	TaskID  string
	Content string
	Items   []*models.TaskPreviewFeedback
	Before  *models.TaskPreviewFeedbackSnapshot
}

// FeedbackChangedError carries the authoritative snapshot for client recovery.
type FeedbackChangedError struct {
	Snapshot *models.TaskPreviewFeedbackSnapshot
}

func (e *FeedbackChangedError) Error() string {
	return repoerrors.ErrTaskPreviewFeedbackChanged.Error()
}
func (e *FeedbackChangedError) Unwrap() error { return repoerrors.ErrTaskPreviewFeedbackChanged }

// Resolve validates exact task-owned references and formats their visible prompt section.
func Resolve(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID, content string,
	refs []models.TaskPreviewFeedbackRef,
) (*Resolution, error) {
	snapshot, err := readSnapshot(ctx, tx, db, taskID)
	if err != nil {
		return nil, err
	}
	items, ok := resolveReferences(snapshot, refs)
	if !ok {
		return nil, &FeedbackChangedError{Snapshot: snapshot}
	}
	formatted, err := previewfeedback.AppendMarkdown(content, items)
	if err != nil {
		return nil, err
	}
	return &Resolution{TaskID: taskID, Content: formatted, Items: items, Before: snapshot}, nil
}

// Consume conditionally deletes exact versions and advances the collection revision.
func Consume(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	resolved *Resolution,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	if resolved == nil || len(resolved.Items) == 0 {
		return nil, errors.New("resolved preview feedback is required")
	}
	query := `DELETE FROM task_preview_feedback WHERE task_id = ? AND (`
	args := []interface{}{resolved.TaskID}
	for index, item := range resolved.Items {
		if index > 0 {
			query += ` OR `
		}
		query += `(id = ? AND version = ?)`
		args = append(args, item.ID, item.Version)
	}
	query += `)`
	result, err := tx.ExecContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("consume task preview feedback: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != int64(len(resolved.Items)) {
		return nil, &FeedbackChangedError{Snapshot: resolved.Before}
	}
	result, err = tx.ExecContext(ctx, db.Rebind(`
		UPDATE task_preview_feedback_collections SET revision = revision + 1
		WHERE task_id = ?
	`), resolved.TaskID)
	if err != nil {
		return nil, fmt.Errorf("advance task preview feedback revision: %w", err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, repoerrors.ErrTaskNotFound
	}
	return readSnapshot(ctx, tx, db, resolved.TaskID)
}

func resolveReferences(
	snapshot *models.TaskPreviewFeedbackSnapshot,
	refs []models.TaskPreviewFeedbackRef,
) ([]*models.TaskPreviewFeedback, bool) {
	if snapshot == nil || len(refs) == 0 {
		return nil, false
	}
	wanted := make(map[string]int64, len(refs))
	for _, ref := range refs {
		if ref.ID == "" || ref.Version <= 0 {
			return nil, false
		}
		if _, duplicate := wanted[ref.ID]; duplicate {
			return nil, false
		}
		wanted[ref.ID] = ref.Version
	}
	items := make([]*models.TaskPreviewFeedback, 0, len(refs))
	for _, item := range snapshot.Items {
		if version, selected := wanted[item.ID]; selected {
			if item.Version != version {
				return nil, false
			}
			items = append(items, item)
		}
	}
	return items, len(items) == len(refs)
}

func readSnapshot(
	ctx context.Context,
	q sqlx.QueryerContext,
	db *sqlx.DB,
	taskID string,
) (*models.TaskPreviewFeedbackSnapshot, error) {
	snapshot := &models.TaskPreviewFeedbackSnapshot{TaskID: taskID, Items: []*models.TaskPreviewFeedback{}}
	if err := sqlx.GetContext(ctx, q, &snapshot.Revision, db.Rebind(`
		SELECT revision FROM task_preview_feedback_collections WHERE task_id = ?
	`), taskID); err != nil {
		return nil, fmt.Errorf("read task preview feedback collection: %w", err)
	}
	rows, err := q.QueryContext(ctx, db.Rebind(`
		SELECT f.id, f.task_id, f.kind, f.comment, f.source_kind,
		       COALESCE(f.source_session_id, ''), f.source_label, COALESCE(f.source_path, ''),
		       f.page_route, f.page_title, COALESCE(f.selected_text, ''),
		       COALESCE(f.text_anchor_json, ''), COALESCE(f.element_snapshot_json, ''),
		       COALESCE(f.capture_rect_json, ''), COALESCE(f.screenshot_attachment_id, ''),
		       f.version, f.created_at, f.updated_at,
		       COALESCE(a.id, ''), COALESCE(a.owner_id, ''), COALESCE(a.workspace_id, ''),
		       COALESCE(a.task_id, ''), COALESCE(a.session_id, ''), COALESCE(a.message_id, ''),
		       COALESCE(a.queue_id, ''), COALESCE(a.name, ''), COALESCE(a.mime_type, ''),
		       COALESCE(a.kind, ''), COALESCE(a.delivery_mode, ''), COALESCE(a.size_bytes, 0),
		       COALESCE(a.storage_key, ''), COALESCE(a.state, ''), a.expires_at, a.created_at, a.updated_at
		FROM task_preview_feedback f
		LEFT JOIN task_message_attachments a ON a.id = f.screenshot_attachment_id
		WHERE f.task_id = ? ORDER BY f.created_at, f.id
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task preview feedback for delivery: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		item := &models.TaskPreviewFeedback{}
		attachment := &models.TaskMessageAttachment{}
		var textAnchor, elementSnapshot, captureRect string
		var attachmentExpires, attachmentCreated, attachmentUpdated sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.TaskID, &item.Kind, &item.Comment, &item.SourceKind,
			&item.SourceSessionID, &item.SourceLabel, &item.SourcePath,
			&item.PageRoute, &item.PageTitle, &item.SelectedText,
			&textAnchor, &elementSnapshot, &captureRect, &item.ScreenshotAttachmentID,
			&item.Version, &item.CreatedAt, &item.UpdatedAt,
			&attachment.ID, &attachment.OwnerID, &attachment.WorkspaceID,
			&attachment.TaskID, &attachment.SessionID, &attachment.MessageID,
			&attachment.QueueID, &attachment.Name, &attachment.MimeType,
			&attachment.Kind, &attachment.DeliveryMode, &attachment.SizeBytes,
			&attachment.StorageKey, &attachment.State, &attachmentExpires,
			&attachmentCreated, &attachmentUpdated,
		); err != nil {
			return nil, fmt.Errorf("scan task preview feedback for delivery: %w", err)
		}
		item.TextAnchor = rawJSON(textAnchor)
		item.ElementSnapshot = rawJSON(elementSnapshot)
		item.CaptureRect = rawJSON(captureRect)
		if item.ScreenshotAttachmentID != "" {
			if attachment.ID == "" {
				return nil, models.ErrAttachmentNotFound
			}
			attachment.ExpiresAt = attachmentExpires.Time
			attachment.CreatedAt = attachmentCreated.Time
			attachment.UpdatedAt = attachmentUpdated.Time
			item.ScreenshotAttachment = attachment
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task preview feedback for delivery: %w", err)
	}
	return snapshot, nil
}

func rawJSON(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}

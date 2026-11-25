package plancommenttx

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ReadAttachments loads canonical task-owned file descriptors in comment order.
func ReadAttachments(ctx context.Context, q sqlx.QueryerContext, db *sqlx.DB, taskID string, comments []*models.TaskPlanComment) error {
	if len(comments) == 0 {
		return nil
	}
	byID := make(map[string]*models.TaskPlanComment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}
	rows, err := q.QueryContext(ctx, db.Rebind(`
		SELECT r.comment_id, r.attachment_id, a.name, a.mime_type, a.kind, a.delivery_mode, a.size_bytes
		FROM task_plan_comment_attachments r
		JOIN task_plan_comments c ON c.id = r.comment_id
		JOIN task_message_attachments a ON a.id = r.attachment_id
		WHERE c.task_id = ?
		ORDER BY r.comment_id, r.position
	`), taskID)
	if err != nil {
		return fmt.Errorf("read plan comment attachment descriptors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var commentID string
		var file v1.MessageAttachment
		if err := rows.Scan(&commentID, &file.AttachmentID, &file.Name, &file.MimeType, &file.Type, &file.DeliveryMode, &file.SizeBytes); err != nil {
			return fmt.Errorf("scan plan comment attachment descriptor: %w", err)
		}
		if comment := byID[commentID]; comment != nil {
			comment.Attachments = append(comment.Attachments, file)
			comment.AttachmentIDs = append(comment.AttachmentIDs, file.AttachmentID)
		}
	}
	return rows.Err()
}

// HasFiles reports whether the accepted plan comments include attachments.
func (resolved *Resolution) HasFiles() bool {
	if resolved == nil {
		return false
	}
	for _, comment := range resolved.Comments {
		if len(comment.Attachments) > 0 {
			return true
		}
	}
	return false
}

// Files returns ordered canonical descriptors with their owning comment IDs.
func (resolved *Resolution) Files() []v1.MessageAttachment {
	if resolved == nil {
		return nil
	}
	var files []v1.MessageAttachment
	for _, comment := range resolved.Comments {
		for _, file := range comment.Attachments {
			file.CommentID = comment.ID
			files = append(files, file)
		}
	}
	return files
}

// TransferFiles moves pending plan files to their admitted message or queue.
func (resolved *Resolution) TransferFiles(ctx context.Context, tx *sqlx.Tx, sessionID, messageID, queueID string) error {
	now := time.Now().UTC()
	for _, file := range resolved.Files() {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET state = ?, session_id = ?, message_id = ?, queue_id = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND state = ? AND delivery_mode = 'path'
			AND session_id = '' AND message_id = '' AND queue_id = ''
			AND workspace_id = (SELECT workspace_id FROM tasks WHERE id = ?)
		`), models.AttachmentStateClaimed, sessionID, messageID, queueID, now, file.AttachmentID,
			resolved.TaskID, models.AttachmentStatePlanComment, resolved.TaskID)
		if err != nil {
			return fmt.Errorf("transfer plan comment attachment to delivery: %w", err)
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

// ValidateFileBatch enforces one shared limit for plan and existing message files.
func ValidateFileBatch(ctx context.Context, tx *sqlx.Tx, files []v1.MessageAttachment, claimedIDs []string) error {
	if len(files) > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	seen := make(map[string]struct{}, len(files))
	count := len(files)
	var total int64
	for _, file := range files {
		size, err := fileSize(ctx, tx, file, seen)
		if err != nil {
			return err
		}
		total += size
	}
	for _, id := range claimedIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		size, err := fileSize(ctx, tx, v1.MessageAttachment{AttachmentID: id}, seen)
		if err != nil {
			return err
		}
		total += size
		count++
	}
	if count > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	if total > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	return nil
}

func fileSize(ctx context.Context, tx *sqlx.Tx, file v1.MessageAttachment, seen map[string]struct{}) (int64, error) {
	size := int64(base64.StdEncoding.DecodedLen(len(file.Data)) - len(file.Data) + len(strings.TrimRight(file.Data, "=")))
	if file.AttachmentID != "" {
		if _, duplicate := seen[file.AttachmentID]; duplicate {
			return 0, models.ErrAttachmentClaimConflict
		}
		seen[file.AttachmentID] = struct{}{}
		if err := tx.GetContext(ctx, &size, tx.Rebind(`SELECT size_bytes FROM task_message_attachments WHERE id = ?`), file.AttachmentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, models.ErrAttachmentNotFound
			}
			return 0, fmt.Errorf("read message attachment size: %w", err)
		}
	}
	if size < 0 || size > models.MaxMessageAttachmentBytes {
		return 0, models.ErrAttachmentTooLarge
	}
	return size, nil
}

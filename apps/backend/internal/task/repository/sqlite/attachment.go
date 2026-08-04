package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

const attachmentSelectColumns = `id, owner_id, workspace_id, task_id, session_id,
	message_id, queue_id, name, mime_type, kind, delivery_mode, size_bytes,
	storage_key, state, expires_at, created_at, updated_at`

func (r *Repository) CreateMessageAttachment(ctx context.Context, attachment *models.TaskMessageAttachment) error {
	if attachment.ID == "" {
		attachment.ID = uuid.NewString()
	}
	if attachment.StorageKey == "" {
		attachment.StorageKey = attachment.ID
	}
	now := time.Now().UTC()
	if attachment.CreatedAt.IsZero() {
		attachment.CreatedAt = now
	}
	attachment.UpdatedAt = now
	if attachment.State == "" {
		attachment.State = models.AttachmentStateStaged
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_message_attachments
			(id, owner_id, workspace_id, task_id, session_id, message_id, queue_id,
			 name, mime_type, kind, delivery_mode, size_bytes, storage_key, state,
			 expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), attachment.ID, attachment.OwnerID, attachment.WorkspaceID, attachment.TaskID,
		attachment.SessionID, attachment.MessageID, attachment.QueueID, attachment.Name,
		attachment.MimeType, attachment.Kind, attachment.DeliveryMode, attachment.SizeBytes,
		attachment.StorageKey, attachment.State, attachment.ExpiresAt, attachment.CreatedAt,
		attachment.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create message attachment: %w", err)
	}
	return nil
}

func (r *Repository) GetMessageAttachment(ctx context.Context, id string) (*models.TaskMessageAttachment, error) {
	attachment := &models.TaskMessageAttachment{}
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE id = ?
	`), id).StructScan(attachment)
	if err == sql.ErrNoRows {
		return nil, models.ErrAttachmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message attachment: %w", err)
	}
	return attachment, nil
}

func (r *Repository) ListMessageAttachments(ctx context.Context, ids []string) ([]*models.TaskMessageAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i := range ids {
		args[i] = ids[i]
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE id IN (`+placeholders+`)
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list message attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			return nil, fmt.Errorf("scan message attachment: %w", err)
		}
		out = append(out, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message attachments: %w", err)
	}
	return out, nil
}

func (r *Repository) ClaimMessageAttachments(ctx context.Context, ids []string, ownerID, workspaceID, taskID, sessionID string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	selection, err := r.selectAttachmentsForClaim(ctx, tx, ids, ownerID, workspaceID, taskID, sessionID)
	if err != nil {
		return err
	}
	if err := r.checkAttachmentClaimAggregate(ctx, tx, ids, selection, ownerID, workspaceID, taskID, sessionID); err != nil {
		return err
	}
	if err := r.markAttachmentsClaimed(ctx, tx, selection.claimIDs, ownerID, workspaceID, taskID, sessionID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment claim: %w", err)
	}
	return nil
}

type attachmentClaimSelection struct {
	claimIDs     []string
	selectedSize int64
}

func (r *Repository) selectAttachmentsForClaim(ctx context.Context, tx *sqlx.Tx, ids []string, ownerID, workspaceID, taskID, sessionID string) (attachmentClaimSelection, error) {
	selection := attachmentClaimSelection{}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		seen[id] = struct{}{}
		var attachment models.TaskMessageAttachment
		err := tx.GetContext(ctx, &attachment, tx.Rebind(`
			SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE id = ?
		`), id)
		if errorsIsNoRows(err) {
			return attachmentClaimSelection{}, models.ErrAttachmentNotFound
		}
		if err != nil {
			return attachmentClaimSelection{}, fmt.Errorf("load attachment for claim: %w", err)
		}
		if attachment.OwnerID != ownerID || attachment.WorkspaceID != workspaceID {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		switch attachment.State {
		case models.AttachmentStateStaged:
			if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
				return attachmentClaimSelection{}, models.ErrAttachmentTooLarge
			}
			selection.selectedSize += attachment.SizeBytes
			selection.claimIDs = append(selection.claimIDs, id)
		case models.AttachmentStateClaimed:
			if attachment.TaskID != taskID || attachment.SessionID != sessionID {
				return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
			}
		default:
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
	}
	return selection, nil
}

func (r *Repository) checkAttachmentClaimAggregate(ctx context.Context, tx *sqlx.Tx, ids []string, selection attachmentClaimSelection, ownerID, workspaceID, taskID, sessionID string) error {
	var existingCount, existingSize int64
	claimPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	aggregateQuery := `
		SELECT COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM task_message_attachments
		WHERE owner_id = ? AND workspace_id = ? AND task_id = ? AND session_id = ?
		  AND state IN (?, ?)
		  AND id NOT IN (` + claimPlaceholders + `)`
	aggregateArgs := []interface{}{ownerID, workspaceID, taskID, sessionID,
		models.AttachmentStateStaged, models.AttachmentStateClaimed}
	aggregateArgs = append(aggregateArgs, idsToInterfaces(ids)...)
	if err := tx.QueryRowxContext(ctx, tx.Rebind(aggregateQuery), aggregateArgs...).Scan(&existingCount, &existingSize); err != nil {
		return fmt.Errorf("check attachment claim aggregate: %w", err)
	}
	if existingCount+int64(len(selection.claimIDs)) > int64(models.MaxMessageAttachmentCount) || existingSize+selection.selectedSize > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	return nil
}

func (r *Repository) markAttachmentsClaimed(ctx context.Context, tx *sqlx.Tx, ids []string, ownerID, workspaceID, taskID, sessionID string) error {
	now := time.Now().UTC()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = ?, session_id = ?, state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
		`), taskID, sessionID, models.AttachmentStateClaimed, now, id, ownerID, workspaceID, models.AttachmentStateStaged); err != nil {
			return fmt.Errorf("claim attachment: %w", err)
		}
	}
	return nil
}

func (r *Repository) DeleteMessageAttachment(ctx context.Context, id, ownerID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_message_attachments WHERE id = ? AND owner_id = ?
	`), id, ownerID)
	if err != nil {
		return fmt.Errorf("delete message attachment: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return models.ErrAttachmentNotFound
	}
	return nil
}

func (r *Repository) MarkExpiredMessageAttachments(ctx context.Context, now time.Time) ([]*models.TaskMessageAttachment, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE state = ? AND expires_at <= ?
	`), models.AttachmentStateStaged, now)
	if err != nil {
		return nil, fmt.Errorf("list expired attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var expired []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			return nil, fmt.Errorf("scan expired attachment: %w", err)
		}
		expired = append(expired, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired attachments: %w", err)
	}
	if len(expired) == 0 {
		return nil, nil
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_message_attachments SET state = ?, updated_at = ?
		WHERE state = ? AND expires_at <= ?
	`), models.AttachmentStateExpired, now, models.AttachmentStateStaged, now); err != nil {
		return nil, fmt.Errorf("mark expired attachments: %w", err)
	}
	return expired, nil
}

func errorsIsNoRows(err error) bool { return err == sql.ErrNoRows }

func idsToInterfaces(ids []string) []interface{} {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

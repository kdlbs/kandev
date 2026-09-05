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

	now := time.Now().UTC()
	selection, err := r.selectAttachmentsForClaim(ctx, tx, ids, ownerID, workspaceID, taskID, sessionID, now)
	if err != nil {
		return err
	}
	if selection.selectedSize > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	if err := r.markAttachmentsClaimed(ctx, tx, selection.claimIDs, ownerID, workspaceID, taskID, sessionID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment claim: %w", err)
	}
	return nil
}

// ClaimQueuedMessageAttachments marks only newly staged rows with queueID.
// Existing claims for the same task/session remain usable but are not adopted,
// so a failed admission cannot roll back another accepted message's claim.
func (r *Repository) ClaimQueuedMessageAttachments(
	ctx context.Context,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, queueID string,
) error {
	if len(ids) == 0 {
		return nil
	}
	if queueID == "" {
		return models.ErrAttachmentClaimConflict
	}
	if len(ids) > models.MaxMessageAttachmentCount {
		return models.ErrTooManyAttachments
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued attachment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	selection, err := r.selectQueuedAttachmentsForClaim(
		ctx, tx, ids, ownerID, workspaceID, taskID, sessionID, queueID, now,
	)
	if err != nil {
		return err
	}
	if selection.selectedSize > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	if err := r.markQueuedAttachmentsClaimed(
		ctx, tx, selection.claimIDs, ownerID, workspaceID, taskID, sessionID, queueID, now,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queued attachment claim: %w", err)
	}
	return nil
}

func (r *Repository) selectQueuedAttachmentsForClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, queueID string,
	now time.Time,
) (attachmentClaimSelection, error) {
	selection := attachmentClaimSelection{}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !recordUniqueAttachmentID(seen, id) {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		attachment, err := loadAttachmentForClaim(ctx, tx, id)
		if err != nil {
			return attachmentClaimSelection{}, err
		}
		if attachment.OwnerID != ownerID || attachment.WorkspaceID != workspaceID {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		if err := addQueuedAttachmentToClaim(&selection, attachment, id, taskID, sessionID, queueID, now); err != nil {
			return attachmentClaimSelection{}, err
		}
	}
	return selection, nil
}

func addQueuedAttachmentToClaim(
	selection *attachmentClaimSelection,
	attachment *models.TaskMessageAttachment,
	id, taskID, sessionID, queueID string,
	now time.Time,
) error {
	if attachment.State == models.AttachmentStateStaged {
		return addStagedAttachmentToClaim(selection, attachment, id, now)
	}
	if attachment.State != models.AttachmentStateClaimed ||
		attachment.TaskID != taskID ||
		(attachment.SessionID != "" && attachment.SessionID != sessionID) ||
		attachment.MessageID != "" ||
		(attachment.QueueID != "" && attachment.QueueID != queueID) {
		return models.ErrAttachmentClaimConflict
	}
	return nil
}

func (r *Repository) markQueuedAttachmentsClaimed(
	ctx context.Context,
	tx *sqlx.Tx,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, queueID string,
	now time.Time,
) error {
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = ?, session_id = ?, queue_id = ?, state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
		`), taskID, sessionID, queueID, models.AttachmentStateClaimed, now,
			id, ownerID, workspaceID, models.AttachmentStateStaged)
		if err != nil {
			return fmt.Errorf("claim queued attachment: %w", err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

// RestoreQueuedMessageAttachments returns only rows provisionally claimed by
// this queue admission to staged state. A pre-existing claim has no queueID and
// is deliberately untouched.
func (r *Repository) RestoreQueuedMessageAttachments(
	ctx context.Context,
	ids []string,
	ownerID, taskID, sessionID, queueID string,
) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued attachment restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = '', session_id = '', message_id = '', queue_id = '', state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND task_id = ? AND session_id = ?
			  AND queue_id = ? AND state = ?
		`), models.AttachmentStateStaged, now, id, ownerID, taskID, sessionID,
			queueID, models.AttachmentStateClaimed); err != nil {
			return fmt.Errorf("restore queued attachment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queued attachment restore: %w", err)
	}
	return nil
}

func (r *Repository) ClaimDirectMessageAttachments(
	ctx context.Context,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, messageID string,
) error {
	if len(ids) == 0 {
		return nil
	}
	if messageID == "" || len(ids) > models.MaxMessageAttachmentCount {
		return models.ErrAttachmentClaimConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin direct attachment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	selection, err := r.selectDirectAttachmentsForClaim(
		ctx, tx, ids, ownerID, workspaceID, taskID, sessionID, messageID, now,
	)
	if err != nil {
		return err
	}
	if selection.selectedSize > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	if err := r.markDirectAttachmentsClaimed(
		ctx, tx, selection.claimIDs, ownerID, workspaceID, taskID, sessionID, messageID, now,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit direct attachment claim: %w", err)
	}
	return nil
}

func (r *Repository) selectDirectAttachmentsForClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, messageID string,
	now time.Time,
) (attachmentClaimSelection, error) {
	selection := attachmentClaimSelection{}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !recordUniqueAttachmentID(seen, id) {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		attachment, err := loadAttachmentForClaim(ctx, tx, id)
		if err != nil {
			return attachmentClaimSelection{}, err
		}
		if attachment.OwnerID != ownerID || attachment.WorkspaceID != workspaceID {
			return attachmentClaimSelection{}, models.ErrAttachmentClaimConflict
		}
		if err := addDirectAttachmentToClaim(&selection, attachment, id, taskID, sessionID, messageID, now); err != nil {
			return attachmentClaimSelection{}, err
		}
	}
	return selection, nil
}

func addDirectAttachmentToClaim(
	selection *attachmentClaimSelection,
	attachment *models.TaskMessageAttachment,
	id, taskID, sessionID, messageID string,
	now time.Time,
) error {
	if attachment.State == models.AttachmentStateStaged {
		return addStagedAttachmentToClaim(selection, attachment, id, now)
	}
	if attachment.State != models.AttachmentStateClaimed ||
		attachment.TaskID != taskID ||
		(attachment.SessionID != "" && attachment.SessionID != sessionID) ||
		attachment.QueueID != "" ||
		(attachment.MessageID != "" && attachment.MessageID != messageID) {
		return models.ErrAttachmentClaimConflict
	}
	return nil
}

func (r *Repository) markDirectAttachmentsClaimed(
	ctx context.Context,
	tx *sqlx.Tx,
	ids []string,
	ownerID, workspaceID, taskID, sessionID, messageID string,
	now time.Time,
) error {
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = ?, session_id = ?, message_id = ?, state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
		`), taskID, sessionID, messageID, models.AttachmentStateClaimed, now,
			id, ownerID, workspaceID, models.AttachmentStateStaged)
		if err != nil {
			return fmt.Errorf("claim direct attachment: %w", err)
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

func (r *Repository) RestoreDirectMessageAttachments(
	ctx context.Context,
	ids []string,
	ownerID, taskID, sessionID, messageID string,
) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin direct attachment restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = '', session_id = '', message_id = '', queue_id = '', state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND task_id = ? AND session_id = ?
			  AND message_id = ? AND state = ?
			  AND NOT EXISTS (SELECT 1 FROM task_session_messages WHERE task_session_messages.id = ?)
		`), models.AttachmentStateStaged, now, id, ownerID, taskID, sessionID,
			messageID, models.AttachmentStateClaimed, messageID); err != nil {
			return fmt.Errorf("restore direct attachment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit direct attachment restore: %w", err)
	}
	return nil
}

type attachmentClaimSelection struct {
	claimIDs     []string
	selectedSize int64
}

func recordUniqueAttachmentID(seen map[string]struct{}, id string) bool {
	if _, exists := seen[id]; exists {
		return false
	}
	seen[id] = struct{}{}
	return true
}

func loadAttachmentForClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	id string,
) (*models.TaskMessageAttachment, error) {
	attachment := &models.TaskMessageAttachment{}
	err := tx.GetContext(ctx, attachment, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE id = ?
	`), id)
	if errorsIsNoRows(err) {
		return nil, models.ErrAttachmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load attachment for claim: %w", err)
	}
	return attachment, nil
}

func addStagedAttachmentToClaim(
	selection *attachmentClaimSelection,
	attachment *models.TaskMessageAttachment,
	id string,
	now time.Time,
) error {
	if !attachment.ExpiresAt.IsZero() && !attachment.ExpiresAt.After(now) {
		return models.ErrAttachmentClaimConflict
	}
	if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTooLarge
	}
	selection.selectedSize += attachment.SizeBytes
	selection.claimIDs = append(selection.claimIDs, id)
	return nil
}

func (r *Repository) selectAttachmentsForClaim(ctx context.Context, tx *sqlx.Tx, ids []string, ownerID, workspaceID, taskID, sessionID string, now time.Time) (attachmentClaimSelection, error) {
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
		if err := addAttachmentToClaim(&selection, &attachment, id, taskID, sessionID, now); err != nil {
			return attachmentClaimSelection{}, err
		}
	}
	return selection, nil
}

func addAttachmentToClaim(selection *attachmentClaimSelection, attachment *models.TaskMessageAttachment, id, taskID, sessionID string, now time.Time) error {
	switch attachment.State {
	case models.AttachmentStateStaged:
		if !attachment.ExpiresAt.IsZero() && !attachment.ExpiresAt.After(now) {
			return models.ErrAttachmentClaimConflict
		}
		if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes {
			return models.ErrAttachmentTooLarge
		}
		selection.selectedSize += attachment.SizeBytes
		selection.claimIDs = append(selection.claimIDs, id)
		return nil
	case models.AttachmentStateClaimed:
		if attachment.TaskID != taskID ||
			(attachment.SessionID != "" && attachment.SessionID != sessionID) {
			return models.ErrAttachmentClaimConflict
		}
		return nil
	default:
		return models.ErrAttachmentClaimConflict
	}
}

func (r *Repository) markAttachmentsClaimed(ctx context.Context, tx *sqlx.Tx, ids []string, ownerID, workspaceID, taskID, sessionID string) error {
	now := time.Now().UTC()
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET task_id = ?, session_id = ?, state = ?, updated_at = ?
			WHERE id = ? AND owner_id = ? AND workspace_id = ? AND state = ?
		`), taskID, sessionID, models.AttachmentStateClaimed, now, id, ownerID, workspaceID, models.AttachmentStateStaged)
		if err != nil {
			return fmt.Errorf("claim attachment: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("claim attachment rows affected: %w", err)
		}
		if affected != 1 {
			return models.ErrAttachmentClaimConflict
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

func (r *Repository) DeleteClaimedMessageAttachments(ctx context.Context, ids []string, ownerID, taskID, sessionID string) ([]*models.TaskMessageAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []interface{}{ownerID, taskID, sessionID, models.AttachmentStateClaimed}
	args = append(args, idsToInterfaces(ids)...)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claimed attachment release: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE owner_id = ? AND task_id = ? AND session_id = ? AND state = ?
		  AND id IN (`+placeholders+`)
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list claimed attachments for release: %w", err)
	}
	var released []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan claimed attachment for release: %w", err)
		}
		released = append(released, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate claimed attachments for release: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close claimed attachments for release: %w", err)
	}
	for _, attachment := range released {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			DELETE FROM task_message_attachments
			WHERE id = ? AND owner_id = ? AND task_id = ? AND session_id = ? AND state = ?
		`), attachment.ID, ownerID, taskID, sessionID, models.AttachmentStateClaimed)
		if err != nil {
			return nil, fmt.Errorf("release claimed attachment: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("release claimed attachment rows affected: %w", err)
		}
		if affected != 1 {
			return nil, models.ErrAttachmentClaimConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claimed attachment release: %w", err)
	}
	return released, nil
}

func (r *Repository) DeleteMessageAttachmentsByTask(ctx context.Context, taskID string) ([]*models.TaskMessageAttachment, error) {
	if taskID == "" {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin task attachment cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE task_id = ?
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task attachments for cleanup: %w", err)
	}
	var attachments []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan task attachment for cleanup: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate task attachments for cleanup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task attachments for cleanup: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_message_attachments WHERE task_id = ?`), taskID); err != nil {
		return nil, fmt.Errorf("delete task attachments: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit task attachment cleanup: %w", err)
	}
	return attachments, nil
}

func (r *Repository) MarkExpiredMessageAttachments(ctx context.Context, now time.Time) ([]*models.TaskMessageAttachment, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin expired attachment cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE state = ? AND expires_at <= ?
	`), models.AttachmentStateStaged, now)
	if err != nil {
		return nil, fmt.Errorf("list expired attachments: %w", err)
	}
	var expired []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan expired attachment: %w", err)
		}
		expired = append(expired, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate expired attachments: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close expired attachments: %w", err)
	}
	transitioned := make([]*models.TaskMessageAttachment, 0, len(expired))
	for _, attachment := range expired {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments SET state = ?, updated_at = ?
			WHERE id = ? AND state = ? AND expires_at <= ?
		`), models.AttachmentStateExpired, now, attachment.ID, models.AttachmentStateStaged, now)
		if err != nil {
			return nil, fmt.Errorf("mark expired attachment: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("mark expired attachment rows affected: %w", err)
		}
		if affected == 1 {
			attachment.State = models.AttachmentStateExpired
			attachment.UpdatedAt = now
			transitioned = append(transitioned, attachment)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit expired attachment cleanup: %w", err)
	}
	return transitioned, nil
}

func errorsIsNoRows(err error) bool { return err == sql.ErrNoRows }

func idsToInterfaces(ids []string) []interface{} {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

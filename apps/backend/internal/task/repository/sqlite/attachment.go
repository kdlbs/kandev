package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
	queueLockPresent, err := r.queueSessionLockTablePresent(ctx)
	if err != nil {
		return fmt.Errorf("check queue session lock schema: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if queueLockPresent {
		if err := messagequeue.LockSessionInTransaction(ctx, tx, r.db, sessionID); err != nil {
			return fmt.Errorf("lock attachment claim session: %w", err)
		}
	}
	if taskID != "" {
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE tasks SET updated_at = updated_at WHERE id = ?
		`), taskID)
		if err != nil {
			return fmt.Errorf("lock attachment task %q: %w", taskID, err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
	}
	if sessionID != "" {
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
			return err
		}
	}
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

func (r *Repository) claimDirectMessageAttachmentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	claim *messagequeue.QueueAttachmentClaim,
	taskID, sessionID, messageID string,
) error {
	if claim == nil || len(claim.IDs) == 0 {
		return nil
	}
	if claim.OwnerID == "" || claim.WorkspaceID == "" || messageID == "" || len(claim.IDs) > models.MaxMessageAttachmentCount {
		return models.ErrAttachmentClaimConflict
	}
	now := time.Now().UTC()
	selection, err := r.selectDirectAttachmentsForClaim(
		ctx, tx, claim.IDs, claim.OwnerID, claim.WorkspaceID, taskID, sessionID, messageID, now,
	)
	if err != nil {
		return err
	}
	if selection.selectedSize > models.MaxMessageAttachmentBytes {
		return models.ErrAttachmentTotalTooLarge
	}
	if err := r.markDirectAttachmentsClaimed(
		ctx, tx, selection.claimIDs, claim.OwnerID, claim.WorkspaceID, taskID, sessionID, messageID, now,
	); err != nil {
		return err
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

func (r *Repository) DeleteClaimedMessageAttachments(
	ctx context.Context, ids []string, ownerID, taskID, sessionID string,
) ([]*models.TaskMessageAttachment, error) {
	return r.deleteClaimedMessageAttachments(ctx, ids, ownerID, taskID, sessionID, true)
}

func (r *Repository) DeleteClaimedMessageAttachmentsByTaskSession(
	ctx context.Context, ids []string, taskID, sessionID string,
) ([]*models.TaskMessageAttachment, error) {
	return r.deleteClaimedMessageAttachments(ctx, ids, "", taskID, sessionID, false)
}

//nolint:cyclop,funlen // Attachment release coordinates ownership, queue locks, and transactional cleanup.
func (r *Repository) deleteClaimedMessageAttachments(
	ctx context.Context,
	ids []string,
	ownerID, taskID, sessionID string,
	ownerRequired bool,
) ([]*models.TaskMessageAttachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	queueTablePresent, err := r.tableExists("queued_messages")
	if err != nil {
		return nil, fmt.Errorf("probe queue attachment references: %w", err)
	}
	queueLockPresent := false
	if sessionID != "" {
		queueLockPresent, err = r.queueSessionLockTablePresent(ctx)
		if err != nil {
			return nil, fmt.Errorf("check queue session lock schema: %w", err)
		}
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claimed attachment release: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockClaimedAttachmentReleaseTx(ctx, tx, sessionID, queueLockPresent); err != nil {
		return nil, err
	}
	if sessionID != "" {
		if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
			return nil, err
		}
	}
	if queueTablePresent {
		if err := lockAttachmentQueueSession(ctx, tx, r.db.DriverName(), sessionID); err != nil {
			return nil, err
		}
	}
	referenced, err := referencedAttachmentIDsTx(ctx, r, tx, taskID, sessionID, queueTablePresent)
	if err != nil {
		return nil, err
	}
	releasable := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := referenced[id]; ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		releasable = append(releasable, id)
	}
	if len(releasable) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(releasable)), ",")
	ownerClause := ""
	args := []interface{}{taskID, sessionID, models.AttachmentStateClaimed}
	if ownerRequired {
		ownerClause = "owner_id = ? AND "
		args = append([]interface{}{ownerID}, args...)
	}
	args = append(args, idsToInterfaces(releasable)...)
	released, err := listClaimedAttachmentsForReleaseTx(
		ctx, tx, ownerClause, attachmentSessionScopeClause(sessionID), placeholders, args,
	)
	if err != nil {
		return nil, err
	}
	if err := deleteClaimedAttachmentsTx(
		ctx, tx, released, ownerClause, attachmentSessionScopeClause(sessionID),
		ownerID, ownerRequired, taskID, sessionID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claimed attachment release: %w", err)
	}
	return released, nil
}

func (r *Repository) lockClaimedAttachmentReleaseTx(
	ctx context.Context, tx *sqlx.Tx, sessionID string, queueLockPresent bool,
) error {
	if !queueLockPresent {
		return nil
	}
	if err := messagequeue.LockSessionInTransaction(ctx, tx, r.db, sessionID); err != nil {
		return fmt.Errorf("lock attachment release session %s: %w", sessionID, err)
	}
	return nil
}

func lockAttachmentQueueSession(
	ctx context.Context,
	tx *sqlx.Tx,
	driverName, sessionID string,
) error {
	if sessionID == "" || !dialect.IsPostgres(driverName) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ($1)
		ON CONFLICT(session_id) DO NOTHING
	`, sessionID); err != nil {
		return fmt.Errorf("ensure attachment queue session lock %q: %w", sessionID, err)
	}
	var lockedSessionID string
	if err := tx.GetContext(
		ctx,
		&lockedSessionID,
		`SELECT session_id FROM queue_session_locks WHERE session_id = $1 FOR UPDATE`,
		sessionID,
	); err != nil {
		return fmt.Errorf("lock attachment queue session %q: %w", sessionID, err)
	}
	return nil
}

func listClaimedAttachmentsForReleaseTx(
	ctx context.Context,
	tx *sqlx.Tx,
	ownerClause, sessionScopeClause, placeholders string,
	args []interface{},
) ([]*models.TaskMessageAttachment, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE `+ownerClause+`task_id = ? AND `+sessionScopeClause+` AND state = ?
		  AND id IN (`+placeholders+`)
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("list claimed attachments for release: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var released []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			return nil, fmt.Errorf("scan claimed attachment for release: %w", err)
		}
		released = append(released, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed attachments for release: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close claimed attachments for release: %w", err)
	}
	return released, nil
}

func deleteClaimedAttachmentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	released []*models.TaskMessageAttachment,
	ownerClause, sessionScopeClause, ownerID string,
	ownerRequired bool,
	taskID, sessionID string,
) error {
	for _, attachment := range released {
		deleteArgs := []interface{}{attachment.ID}
		if ownerRequired {
			deleteArgs = append(deleteArgs, ownerID)
		}
		deleteArgs = append(deleteArgs, taskID, sessionID, models.AttachmentStateClaimed)
		result, err := tx.ExecContext(ctx, tx.Rebind(`
			DELETE FROM task_message_attachments
			WHERE id = ? AND `+ownerClause+`task_id = ? AND `+sessionScopeClause+` AND state = ?
		`), deleteArgs...)
		if err != nil {
			return fmt.Errorf("release claimed attachment: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("release claimed attachment rows affected: %w", err)
		}
		if affected != 1 {
			return models.ErrAttachmentClaimConflict
		}
	}
	return nil
}

func attachmentSessionScopeClause(sessionID string) string {
	if sessionID == "" {
		return "session_id = ?"
	}
	return "(session_id = ? OR session_id = '')"
}

type attachmentReference struct {
	AttachmentID string `json:"attachment_id"`
}

func referencedAttachmentIDsTx(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	taskID, sessionID string,
	queueTablePresent bool,
) (map[string]struct{}, error) {
	referenced := make(map[string]struct{})
	//nolint:nestif // durable queue and transcript references use separate optional tables.
	if queueTablePresent {
		rows, err := tx.QueryxContext(ctx, tx.Rebind(`
			SELECT attachments_json FROM queued_messages WHERE task_id = ? AND session_id = ?
		`), taskID, sessionID)
		if err != nil {
			return nil, fmt.Errorf("list queued attachment references: %w", err)
		}
		for rows.Next() {
			var encoded string
			if err := rows.Scan(&encoded); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan queued attachment references: %w", err)
			}
			var attachments []attachmentReference
			if err := json.Unmarshal([]byte(encoded), &attachments); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("decode queued attachment references: %w", err)
			}
			addAttachmentReferences(referenced, attachments)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate queued attachment references: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close queued attachment references: %w", err)
		}
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT metadata FROM task_session_messages WHERE task_id = ? AND task_session_id = ?
	`), taskID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list transcript attachment references: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, fmt.Errorf("scan transcript attachment references: %w", err)
		}
		var metadata struct {
			Attachments []attachmentReference `json:"attachments"`
		}
		if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
			return nil, fmt.Errorf("decode transcript attachment references: %w", err)
		}
		addAttachmentReferences(referenced, metadata.Attachments)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transcript attachment references: %w", err)
	}
	return referenced, nil
}

func (r *Repository) queuedAttachmentIDsForTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT attachments_json FROM queued_messages WHERE task_id = ?
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list queued attachment references for archive: %w", err)
	}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan queued attachment references for archive: %w", err)
		}
		var attachments []attachmentReference
		if err := json.Unmarshal([]byte(encoded), &attachments); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("decode queued attachment references for archive: %w", err)
		}
		addAttachmentReferences(ids, attachments)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate queued attachment references for archive: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close queued attachment references for archive: %w", err)
	}
	return ids, nil
}

// releaseUnreferencedTaskAttachmentClaimsTx stages queue-owned claims that no
// transcript message references. Only IDs present in queuedIDs are eligible:
// a direct prompt Claim commits before its transcript row exists, so staging
// every unreferenced claimed row would release an in-flight direct claim whose
// message inserts after the archive commits. Queue admission claims atomically
// with its queue row, so the pre-purge queued set is the exact ownership
// boundary. Rows are collected before any UPDATE so no cursor stays open
// across statements on a single-connection Postgres transaction.
func (r *Repository) releaseUnreferencedTaskAttachmentClaimsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	queuedIDs map[string]struct{},
) error {
	if len(queuedIDs) == 0 {
		return nil
	}
	referenced := make(map[string]struct{})
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT metadata FROM task_session_messages WHERE task_id = ?
	`), taskID)
	if err != nil {
		return fmt.Errorf("list task attachment references for archive: %w", err)
	}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan task attachment references for archive: %w", err)
		}
		var metadata struct {
			Attachments []attachmentReference `json:"attachments"`
		}
		if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode task attachment references for archive: %w", err)
		}
		addAttachmentReferences(referenced, metadata.Attachments)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate task attachment references for archive: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close task attachment references for archive: %w", err)
	}
	var unreferenced []string
	for id := range queuedIDs {
		if _, ok := referenced[id]; !ok {
			unreferenced = append(unreferenced, id)
		}
	}
	now := time.Now().UTC()
	for _, attachmentID := range unreferenced {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_message_attachments
			SET state = ?, expires_at = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND state = ?
		`), models.AttachmentStateStaged, now, now, attachmentID, taskID, models.AttachmentStateClaimed); err != nil {
			return fmt.Errorf("release task attachment claim for archive: %w", err)
		}
	}
	return nil
}

func (r *Repository) deleteUnreferencedSessionAttachmentClaimsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID string,
) ([]*models.TaskMessageAttachment, error) {
	// The session, its transcript, and its queue rows are deleted by this
	// transaction. Return descriptors before deleting their registry rows so
	// the caller can remove private bytes after commit.
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+`
		FROM task_message_attachments
		WHERE task_id = ? AND session_id = ? AND state = ?
	`), taskID, sessionID, models.AttachmentStateClaimed)
	if err != nil {
		return nil, fmt.Errorf("list deleted-session attachment claims: %w", err)
	}
	var attachments []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan deleted-session attachment claim: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate deleted-session attachment claims: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close deleted-session attachment claims: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM task_message_attachments
		WHERE task_id = ? AND session_id = ? AND state = ?
	`), taskID, sessionID, models.AttachmentStateClaimed); err != nil {
		return nil, fmt.Errorf("delete deleted-session attachment claims: %w", err)
	}
	return attachments, nil
}

func addAttachmentReferences(target map[string]struct{}, attachments []attachmentReference) {
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			target[attachment.AttachmentID] = struct{}{}
		}
	}
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

// DeleteMessageAttachmentsBySession removes claimed attachment descriptors
// owned by a deleted task session. Staged uploads have no session binding and
// remain governed by their expiry cleanup.
func (r *Repository) DeleteMessageAttachmentsBySession(ctx context.Context, taskID, sessionID string) ([]*models.TaskMessageAttachment, error) {
	if taskID == "" || sessionID == "" {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin session attachment cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments
		WHERE task_id = ? AND session_id = ?
	`), taskID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session attachments for cleanup: %w", err)
	}
	var attachments []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan session attachment for cleanup: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate session attachments for cleanup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close session attachments for cleanup: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM task_message_attachments WHERE task_id = ? AND session_id = ?
	`), taskID, sessionID); err != nil {
		return nil, fmt.Errorf("delete session attachments: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit session attachment cleanup: %w", err)
	}
	return attachments, nil
}

func (r *Repository) DeleteMessageAttachmentsByWorkspaceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	workspaceID string,
) ([]*models.TaskMessageAttachment, error) {
	if workspaceID == "" {
		return nil, nil
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT `+attachmentSelectColumns+` FROM task_message_attachments WHERE workspace_id = ?
	`), workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace attachments for cleanup: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var attachments []*models.TaskMessageAttachment
	for rows.Next() {
		attachment := &models.TaskMessageAttachment{}
		if err := rows.StructScan(attachment); err != nil {
			return nil, fmt.Errorf("scan workspace attachment for cleanup: %w", err)
		}
		attachments = append(attachments, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace attachments for cleanup: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_message_attachments WHERE workspace_id = ?`), workspaceID); err != nil {
		return nil, fmt.Errorf("delete workspace attachments: %w", err)
	}
	return attachments, nil
}

func (r *Repository) TransferMessageAttachments(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	attachmentIDs []string,
) error {
	if taskID == "" || oldSessionID == "" || newSessionID == "" ||
		oldSessionID == newSessionID || len(attachmentIDs) == 0 {
		return nil
	}
	uniqueAttachmentIDs := uniqueAttachmentIDs(attachmentIDs)
	queueLockPresent, err := r.queueSessionLockTablePresent(ctx)
	if err != nil {
		return fmt.Errorf("check queue session lock schema: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer session attachments: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if queueLockPresent {
		if err := messagequeue.LockSessionPairInTransaction(
			ctx, tx, r.db, oldSessionID, newSessionID,
		); err != nil {
			return fmt.Errorf("lock transfer session pair: %w", err)
		}
	}
	locations, err := transferAttachmentLocations(ctx, tx, taskID, uniqueAttachmentIDs)
	if err != nil {
		return err
	}
	sourceAttachmentIDs, err := validateTransferAttachmentLocations(
		taskID, oldSessionID, newSessionID, uniqueAttachmentIDs, locations,
	)
	if err != nil {
		return err
	}
	if err := updateTransferSessionAttachments(
		ctx, tx, taskID, oldSessionID, newSessionID, sourceAttachmentIDs,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transfer session attachments: %w", err)
	}
	return nil
}

func uniqueAttachmentIDs(attachmentIDs []string) []string {
	unique := make([]string, 0, len(attachmentIDs))
	seen := make(map[string]struct{}, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		if _, ok := seen[attachmentID]; ok {
			continue
		}
		seen[attachmentID] = struct{}{}
		unique = append(unique, attachmentID)
	}
	return unique
}

func transferAttachmentLocations(
	ctx context.Context, tx *sqlx.Tx, taskID string, attachmentIDs []string,
) (map[string]string, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(attachmentIDs)), ",")
	args := make([]interface{}, 0, 2+len(attachmentIDs))
	args = append(args, taskID, models.AttachmentStateClaimed)
	for _, attachmentID := range attachmentIDs {
		args = append(args, attachmentID)
	}
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT id, session_id
		FROM task_message_attachments
		WHERE task_id = ? AND state = ? AND id IN (`+placeholders+`)
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("inspect transfer session attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	locations := make(map[string]string, len(attachmentIDs))
	for rows.Next() {
		var attachmentID, sessionID string
		if err := rows.Scan(&attachmentID, &sessionID); err != nil {
			return nil, fmt.Errorf("scan transfer session attachment: %w", err)
		}
		locations[attachmentID] = sessionID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transfer session attachments: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close transfer session attachments: %w", err)
	}
	return locations, nil
}

func validateTransferAttachmentLocations(
	taskID, oldSessionID, newSessionID string, attachmentIDs []string, locations map[string]string,
) ([]string, error) {
	sourceAttachmentIDs := make([]string, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		sessionID, ok := locations[attachmentID]
		if !ok || (sessionID != "" && sessionID != oldSessionID && sessionID != newSessionID) {
			return nil, fmt.Errorf(
				"transfer session attachments: attachment %q is not a claimed attachment for task %q in either transfer session",
				attachmentID, taskID,
			)
		}
		if sessionID == oldSessionID {
			sourceAttachmentIDs = append(sourceAttachmentIDs, attachmentID)
		}
	}
	return sourceAttachmentIDs, nil
}

func updateTransferSessionAttachments(
	ctx context.Context, tx *sqlx.Tx, taskID, oldSessionID, newSessionID string, attachmentIDs []string,
) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(attachmentIDs)), ",")
	args := make([]interface{}, 0, 5+len(attachmentIDs))
	args = append(args, newSessionID, time.Now().UTC(), taskID, oldSessionID, models.AttachmentStateClaimed)
	for _, attachmentID := range attachmentIDs {
		args = append(args, attachmentID)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_message_attachments
		SET session_id = ?, updated_at = ?
		WHERE task_id = ? AND session_id = ? AND state = ?
		  AND id IN (`+placeholders+`)
	`), args...)
	if err != nil {
		return fmt.Errorf("transfer session attachments: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count transferred session attachments: %w", err)
	}
	if affected != int64(len(attachmentIDs)) {
		return fmt.Errorf("transfer session attachments: expected %d claimed source attachments, moved %d", len(attachmentIDs), affected)
	}
	return nil
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

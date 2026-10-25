package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"sort"
	"time"
)

const attachmentCleanupSchema = `
	CREATE TABLE IF NOT EXISTS queue_attachment_cleanups (
		session_id       TEXT NOT NULL,
		current_session_id TEXT NOT NULL DEFAULT '',
		entry_id         TEXT NOT NULL,
		operation_id     TEXT NOT NULL DEFAULT '',
		task_id          TEXT NOT NULL,
		owner_id         TEXT NOT NULL DEFAULT '',
		lease_id         TEXT NOT NULL DEFAULT '',
		remove_entry     INTEGER NOT NULL DEFAULT 0,
		claim_pending    INTEGER NOT NULL DEFAULT 0,
		entry_fingerprint TEXT NOT NULL DEFAULT '',
		attachments_json TEXT NOT NULL DEFAULT '[]',
		created_at       TIMESTAMP NOT NULL,
		PRIMARY KEY (session_id, entry_id, operation_id)
	)
`

func (r *sqliteRepository) ensureAttachmentCleanupSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, attachmentCleanupSchema); err != nil {
		return fmt.Errorf("ensure attachment cleanup schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN owner_id TEXT NOT NULL DEFAULT ''`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup owner: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN remove_entry INTEGER NOT NULL DEFAULT 0`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup remove-entry state: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN claim_pending INTEGER NOT NULL DEFAULT 0`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup claim-pending state: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN entry_fingerprint TEXT NOT NULL DEFAULT ''`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup entry fingerprint: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_attachment_cleanups ADD COLUMN current_session_id TEXT NOT NULL DEFAULT ''`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add attachment cleanup current session: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE queue_attachment_cleanups
		SET current_session_id = session_id
		WHERE current_session_id = ''
	`); err != nil {
		return fmt.Errorf("backfill attachment cleanup current session: %w", err)
	}
	return nil
}

func (r *sqliteRepository) UpsertAttachmentCleanup(ctx context.Context, cleanup AttachmentCleanup) error {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return err
	}
	attachmentsJSON, err := json.Marshal(cleanup.Attachments)
	if err != nil {
		return fmt.Errorf("marshal attachment cleanup: %w", err)
	}
	if cleanup.CreatedAt.IsZero() {
		cleanup.CreatedAt = time.Now().UTC()
	}
	if cleanup.CurrentSessionID == "" {
		cleanup.CurrentSessionID = cleanup.SessionID
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment cleanup upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	cleanup.CurrentSessionID, err = r.resolveAttachmentCleanupSessionTx(ctx, tx, cleanup)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO queue_attachment_cleanups
			(session_id, current_session_id, entry_id, operation_id, task_id, owner_id, lease_id,
			 remove_entry, claim_pending, entry_fingerprint, attachments_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, entry_id, operation_id) DO UPDATE SET
			task_id = excluded.task_id,
			owner_id = excluded.owner_id,
			lease_id = excluded.lease_id,
			remove_entry = excluded.remove_entry,
			claim_pending = excluded.claim_pending,
			entry_fingerprint = excluded.entry_fingerprint,
			attachments_json = excluded.attachments_json
	`), cleanup.SessionID, cleanup.CurrentSessionID, cleanup.EntryID, cleanup.OperationID, cleanup.TaskID,
		cleanup.OwnerID, cleanup.LeaseID, boolToInt(cleanup.RemoveEntry),
		boolToInt(cleanup.ClaimPending), cleanup.EntryFingerprint,
		string(attachmentsJSON), cleanup.CreatedAt); err != nil {
		return fmt.Errorf("upsert attachment cleanup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment cleanup upsert: %w", err)
	}
	return nil
}

func (r *sqliteRepository) resolveAttachmentCleanupSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	cleanup AttachmentCleanup,
) (string, error) {
	entrySessionID := cleanup.CurrentSessionID
	queuedSessionID, err := r.queuedAttachmentCleanupSessionTx(ctx, tx, cleanup)
	if err != nil {
		return "", err
	}
	if queuedSessionID != "" {
		entrySessionID = queuedSessionID
	}
	sessionIDs := make([]string, 0, 2)
	seenSessionIDs := make(map[string]struct{}, 2)
	for _, sessionID := range []string{cleanup.CurrentSessionID, entrySessionID} {
		if sessionID == "" {
			continue
		}
		if _, seen := seenSessionIDs[sessionID]; seen {
			continue
		}
		seenSessionIDs[sessionID] = struct{}{}
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	for _, sessionID := range sessionIDs {
		if err := r.lockSessionTxUnfenced(ctx, tx, sessionID); err != nil {
			return "", err
		}
	}
	queuedSessionID, err = r.queuedAttachmentCleanupSessionTx(ctx, tx, cleanup)
	if err != nil {
		return "", err
	}
	if queuedSessionID != "" {
		entrySessionID = queuedSessionID
	}
	return ResolveSessionTransferInTransaction(ctx, tx, r.db, entrySessionID)
}

func (r *sqliteRepository) queuedAttachmentCleanupSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	cleanup AttachmentCleanup,
) (string, error) {
	var queuedSessionID string
	err := tx.GetContext(ctx, &queuedSessionID, tx.Rebind(`
		SELECT session_id
		FROM queued_messages
		WHERE id = ? AND task_id = ?
	`), cleanup.EntryID, cleanup.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve queued attachment cleanup entry: %w", err)
	}
	return queuedSessionID, nil
}
func (r *sqliteRepository) DeleteAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) error {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attachment cleanup delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var currentSessionID string
	err = tx.GetContext(ctx, &currentSessionID, tx.Rebind(`
		SELECT current_session_id FROM queue_attachment_cleanups
		WHERE session_id = ? AND entry_id = ? AND operation_id = ?
	`), sessionID, entryID, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("locate attachment cleanup for delete: %w", err)
	}
	if err := r.lockSessionTx(ctx, tx, currentSessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_attachment_cleanups
		WHERE session_id = ? AND entry_id = ? AND operation_id = ?
	`), sessionID, entryID, operationID); err != nil {
		return fmt.Errorf("delete attachment cleanup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attachment cleanup delete: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListAttachmentCleanups(ctx context.Context) ([]AttachmentCleanup, error) {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT session_id, current_session_id, entry_id, operation_id, task_id, owner_id, lease_id,
		       remove_entry, claim_pending, entry_fingerprint, attachments_json, created_at
		FROM queue_attachment_cleanups
		ORDER BY created_at, session_id, entry_id, operation_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list attachment cleanups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cleanups []AttachmentCleanup
	for rows.Next() {
		var cleanup AttachmentCleanup
		var attachmentsJSON string
		if err := rows.Scan(
			&cleanup.SessionID,
			&cleanup.CurrentSessionID,
			&cleanup.EntryID,
			&cleanup.OperationID,
			&cleanup.TaskID,
			&cleanup.OwnerID,
			&cleanup.LeaseID,
			&cleanup.RemoveEntry,
			&cleanup.ClaimPending,
			&cleanup.EntryFingerprint,
			&attachmentsJSON,
			&cleanup.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attachment cleanup: %w", err)
		}
		if err := json.Unmarshal([]byte(attachmentsJSON), &cleanup.Attachments); err != nil {
			return nil, fmt.Errorf("unmarshal attachment cleanup: %w", err)
		}
		cleanups = append(cleanups, cleanup)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachment cleanups: %w", err)
	}
	return cleanups, nil
}

func (r *sqliteRepository) GetAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) (*AttachmentCleanup, error) {
	if err := r.ensureAttachmentCleanupSchema(ctx); err != nil {
		return nil, err
	}
	var cleanup AttachmentCleanup
	var attachmentsJSON string
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, current_session_id, entry_id, operation_id, task_id, owner_id, lease_id,
		       remove_entry, claim_pending, entry_fingerprint, attachments_json, created_at
		FROM queue_attachment_cleanups
		WHERE session_id = ? AND entry_id = ? AND operation_id = ?
	`), sessionID, entryID, operationID).Scan(
		&cleanup.SessionID,
		&cleanup.CurrentSessionID,
		&cleanup.EntryID,
		&cleanup.OperationID,
		&cleanup.TaskID,
		&cleanup.OwnerID,
		&cleanup.LeaseID,
		&cleanup.RemoveEntry,
		&cleanup.ClaimPending,
		&cleanup.EntryFingerprint,
		&attachmentsJSON,
		&cleanup.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get attachment cleanup: %w", err)
	}
	if err := json.Unmarshal([]byte(attachmentsJSON), &cleanup.Attachments); err != nil {
		return nil, fmt.Errorf("unmarshal attachment cleanup: %w", err)
	}
	return &cleanup, nil
}

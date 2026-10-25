package messagequeue

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const editLeaseSchema = `
	CREATE TABLE IF NOT EXISTS queue_edit_leases (
		session_id TEXT NOT NULL,
		entry_id TEXT NOT NULL,
		lease_id TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		PRIMARY KEY (session_id, entry_id)
	)
`

type durableEditLeaseMutationRepository interface {
	updateContentAndMetadataWithLease(
		ctx context.Context,
		sessionID, entryID, leaseID, content string,
		attachments []MessageAttachment,
		metadataUpdates map[string]interface{},
		queuedBy string,
	) error
}

func (r *sqliteRepository) ensureEditLeaseSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, editLeaseSchema); err != nil {
		return fmt.Errorf("ensure queue edit lease schema: %w", err)
	}
	return nil
}

func (r *sqliteRepository) acquireEditLease(ctx context.Context, lease *QueueEditLease) error {
	tx, err := r.beginSessionMutationTx(ctx, lease.SessionID, "acquire edit lease")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_edit_leases
		WHERE session_id = ? AND entry_id = ? AND expires_at <= ?
	`), lease.SessionID, lease.EntryID, now); err != nil {
		return fmt.Errorf("expire queue edit lease: %w", err)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO queue_edit_leases (session_id, entry_id, lease_id, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id, entry_id) DO NOTHING
	`), lease.SessionID, lease.EntryID, lease.LeaseID, lease.ExpiresAt)
	if err != nil {
		return fmt.Errorf("acquire queue edit lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count acquired queue edit leases: %w", err)
	}
	if affected != 1 {
		return ErrEditConflict
	}
	return tx.Commit()
}

func (r *sqliteRepository) renewEditLease(ctx context.Context, lease *QueueEditLease) error {
	tx, err := r.beginSessionMutationTx(ctx, lease.SessionID, "renew edit lease")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE queue_edit_leases SET expires_at = ?
		WHERE session_id = ? AND entry_id = ? AND lease_id = ? AND expires_at > ?
	`), lease.ExpiresAt, lease.SessionID, lease.EntryID, lease.LeaseID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("renew queue edit lease: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count renewed queue edit leases: %w", err)
	}
	if affected != 1 {
		return ErrEditLeaseNotFound
	}
	return tx.Commit()
}

func (r *sqliteRepository) releaseEditLease(ctx context.Context, sessionID, entryID, leaseID string) error {
	tx, err := r.beginSessionMutationTx(ctx, sessionID, "release edit lease")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_edit_leases WHERE session_id = ? AND entry_id = ? AND lease_id = ?
	`), sessionID, entryID, leaseID); err != nil {
		return fmt.Errorf("release queue edit lease: %w", err)
	}
	return tx.Commit()
}

func (r *sqliteRepository) editLeaseBlocksEntryTx(ctx context.Context, tx *sqlx.Tx, sessionID, entryID string) (bool, error) {
	var blocked bool
	if err := tx.GetContext(ctx, &blocked, tx.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM queue_edit_leases
			WHERE session_id = ? AND entry_id = ? AND expires_at > ?
		)
	`), sessionID, entryID, time.Now().UTC()); err != nil {
		return false, fmt.Errorf("check queue edit lease: %w", err)
	}
	return blocked, nil
}

func (r *sqliteRepository) deleteEditLeaseForEntryTx(ctx context.Context, tx *sqlx.Tx, sessionID, entryID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_edit_leases WHERE session_id = ? AND entry_id = ?
	`), sessionID, entryID); err != nil {
		return fmt.Errorf("invalidate replaced queue edit lease: %w", err)
	}
	return nil
}

func deleteEditLeasesForSessionIDsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	sessionIDs []string,
) error {
	for _, sessionID := range sessionIDs {
		if _, err := tx.ExecContext(ctx, db.Rebind(`
			DELETE FROM queue_edit_leases WHERE session_id = ?
		`), sessionID); err != nil {
			return fmt.Errorf("invalidate queue edit leases for session %s: %w", sessionID, err)
		}
	}
	return nil
}

func (r *sqliteRepository) deleteEditLeasesForSessionTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	return deleteEditLeasesForSessionIDsTx(ctx, tx, r.db, []string{sessionID})
}

func (r *sqliteRepository) deleteEditLeasesForTransferTx(
	ctx context.Context,
	tx *sqlx.Tx,
	oldSessionID, newSessionID string,
) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_edit_leases WHERE session_id IN (?, ?)
	`), oldSessionID, newSessionID); err != nil {
		return fmt.Errorf("invalidate transferred queue edit leases: %w", err)
	}
	return nil
}

package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

// PendingQueueDispatch records an ordinary queue row removed for at-least-once
// delivery. Accepted selects acknowledgement rather than restoration during
// startup. A crash after agent acceptance but before Accepted commits can
// deliver the same prompt again; the agent protocol has no idempotency key.
type PendingQueueDispatch struct {
	Message  QueuedMessage
	Accepted bool
}

const queueDispatchRecoverySchema = `
	CREATE TABLE IF NOT EXISTS queue_dispatch_claims (
		entry_id     TEXT PRIMARY KEY,
		session_id   TEXT NOT NULL,
		attempt_id   TEXT NOT NULL,
		message_json TEXT NOT NULL,
		accepted     INTEGER NOT NULL DEFAULT 0,
		created_at   TIMESTAMP NOT NULL
	)
`

func (r *sqliteRepository) ensureQueueDispatchRecoverySchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, queueDispatchRecoverySchema); err != nil {
		return fmt.Errorf("ensure queue dispatch recovery schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		ALTER TABLE queue_dispatch_claims ADD COLUMN attempt_id TEXT NOT NULL DEFAULT ''
	`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add queue dispatch attempt id: %w", err)
	}
	rows, err := r.db.QueryxContext(ctx, `SELECT entry_id FROM queue_dispatch_claims WHERE attempt_id = ''`)
	if err != nil {
		return fmt.Errorf("list queue dispatches without attempt ids: %w", err)
	}
	var entryIDs []string
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan queue dispatch without attempt id: %w", err)
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate queue dispatches without attempt ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close queue dispatch attempt migration rows: %w", err)
	}
	for _, entryID := range entryIDs {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE queue_dispatch_claims SET attempt_id = ?
			WHERE entry_id = ? AND attempt_id = ''
		`), uuid.NewString(), entryID); err != nil {
			return fmt.Errorf("backfill queue dispatch attempt id: %w", err)
		}
	}
	return nil
}

func (r *sqliteRepository) persistQueueDispatchClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	msg *QueuedMessage,
) error {
	msg.dispatchAttemptID = uuid.NewString()
	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal queue dispatch claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_dispatch_claims (entry_id, session_id, attempt_id, message_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`), msg.ID, msg.SessionID, msg.dispatchAttemptID, string(messageJSON), time.Now().UTC()); err != nil {
		return fmt.Errorf("persist queue dispatch claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListPendingQueueDispatches(ctx context.Context) ([]PendingQueueDispatch, error) {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT attempt_id, message_json, accepted
		FROM queue_dispatch_claims ORDER BY created_at, entry_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list pending queue dispatch: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pending []PendingQueueDispatch
	for rows.Next() {
		var attemptID, messageJSON string
		var accepted int
		if err := rows.Scan(&attemptID, &messageJSON, &accepted); err != nil {
			return nil, fmt.Errorf("scan pending queue dispatch: %w", err)
		}
		var msg QueuedMessage
		if err := json.Unmarshal([]byte(messageJSON), &msg); err != nil {
			return nil, fmt.Errorf("unmarshal pending queue dispatch: %w", err)
		}
		msg.dispatchAttemptID = attemptID
		msg.reservationGenerationsCaptured = true
		pending = append(pending, PendingQueueDispatch{Message: msg, Accepted: accepted != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending queue dispatch: %w", err)
	}
	return pending, nil
}

func (r *sqliteRepository) transferPendingQueueDispatchesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	oldSessionID, newSessionID string,
) error {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT entry_id, message_json FROM queue_dispatch_claims WHERE session_id = ?
	`), oldSessionID)
	if err != nil {
		return fmt.Errorf("list transferred queue dispatch claims: %w", err)
	}
	type claimUpdate struct {
		entryID     string
		attemptID   string
		messageJSON string
	}
	var updates []claimUpdate
	for rows.Next() {
		var entryID, messageJSON string
		if err := rows.Scan(&entryID, &messageJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan transferred queue dispatch claim: %w", err)
		}
		var msg QueuedMessage
		if err := json.Unmarshal([]byte(messageJSON), &msg); err != nil {
			_ = rows.Close()
			return fmt.Errorf("unmarshal transferred queue dispatch claim: %w", err)
		}
		msg.SessionID = newSessionID
		attemptID := uuid.NewString()
		updatedJSON, err := json.Marshal(msg)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("marshal transferred queue dispatch claim: %w", err)
		}
		updates = append(updates, claimUpdate{
			entryID: entryID, attemptID: attemptID, messageJSON: string(updatedJSON),
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate transferred queue dispatch claims: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close transferred queue dispatch claims: %w", err)
	}
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE queue_dispatch_claims SET session_id = ?, attempt_id = ?, message_json = ?
			WHERE entry_id = ? AND session_id = ?
		`), newSessionID, update.attemptID, update.messageJSON, update.entryID, oldSessionID); err != nil {
			return fmt.Errorf("transfer queue dispatch claim: %w", err)
		}
	}
	return nil
}
func pendingQueueDispatchesForTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]string, []string, error) {
	rows, err := tx.QueryxContext(ctx, `SELECT entry_id, session_id, message_json FROM queue_dispatch_claims`)
	if err != nil {
		return nil, nil, fmt.Errorf("list task queue dispatch claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entryIDs []string
	var sessions []string
	for rows.Next() {
		var entryID, sessionID, messageJSON string
		if err := rows.Scan(&entryID, &sessionID, &messageJSON); err != nil {
			return nil, nil, fmt.Errorf("scan task queue dispatch claim: %w", err)
		}
		var msg QueuedMessage
		if err := json.Unmarshal([]byte(messageJSON), &msg); err != nil {
			return nil, nil, fmt.Errorf("unmarshal task queue dispatch claim: %w", err)
		}
		if msg.TaskID == taskID {
			entryIDs = append(entryIDs, entryID)
			sessions = append(sessions, sessionID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate task queue dispatch claims: %w", err)
	}
	return entryIDs, sessions, nil
}

func (r *sqliteRepository) validatePendingQueueDispatchTx(
	ctx context.Context,
	tx *sqlx.Tx,
	msg *QueuedMessage,
) error {
	var storedSessionID, storedAttemptID string
	var accepted int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, attempt_id, accepted FROM queue_dispatch_claims WHERE entry_id = ?
	`), msg.ID).Scan(&storedSessionID, &storedAttemptID, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrQueueDispatchClaimChanged
	}
	if err != nil {
		return fmt.Errorf("read queue dispatch claim: %w", err)
	}
	if storedSessionID != msg.SessionID || storedAttemptID != msg.dispatchAttemptID || accepted != 0 {
		return ErrQueueDispatchClaimChanged
	}
	return nil
}

func (r *sqliteRepository) deletePendingQueueDispatchTx(
	ctx context.Context,
	tx *sqlx.Tx,
	msg *QueuedMessage,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_dispatch_claims
		WHERE session_id = ? AND entry_id = ? AND attempt_id = ?
	`), msg.SessionID, msg.ID, msg.dispatchAttemptID)
	if err != nil {
		return fmt.Errorf("delete queue dispatch claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete queue dispatch claim rows affected: %w", err)
	}
	if affected != 1 {
		return ErrQueueDispatchClaimChanged
	}
	return nil
}

func (r *sqliteRepository) deletePendingQueueDispatchesBySessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_dispatch_claims WHERE session_id = ?
	`), sessionID); err != nil {
		return fmt.Errorf("delete session queue dispatch claims: %w", err)
	}
	return nil
}

func (r *sqliteRepository) MarkPendingQueueDispatchAccepted(
	ctx context.Context,
	msg *QueuedMessage,
) error {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return err
	}
	tx, err := r.beginSessionMutationTx(ctx, msg.SessionID, "mark queue dispatch accepted")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_dispatch_claims SET accepted = 1
		WHERE entry_id = ? AND session_id = ? AND attempt_id = ?
	`), msg.ID, msg.SessionID, msg.dispatchAttemptID)
	if err != nil {
		return fmt.Errorf("mark queue dispatch accepted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark queue dispatch accepted rows affected: %w", err)
	}
	if affected != 1 {
		return ErrQueueDispatchClaimChanged
	}
	return tx.Commit()
}

func (r *sqliteRepository) DeletePendingQueueDispatch(
	ctx context.Context,
	msg *QueuedMessage,
) error {
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return err
	}
	tx, err := r.beginSessionMutationTx(ctx, msg.SessionID, "delete pending queue dispatch")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.deletePendingQueueDispatchTx(ctx, tx, msg); err != nil {
		return err
	}
	return tx.Commit()
}

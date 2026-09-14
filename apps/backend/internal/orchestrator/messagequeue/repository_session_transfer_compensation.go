package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"time"
)

const sessionTransferCompensationLeaseDuration = time.Minute
const postgresDriverName = "pgx"
const postgresForUpdateSuffix = " FOR UPDATE"

const sessionTransferCompensationSchema = `
	CREATE TABLE IF NOT EXISTS queue_session_transfer_compensations (
		task_id                     TEXT NOT NULL,
		from_session_id             TEXT NOT NULL,
		to_session_id               TEXT NOT NULL,
		operation_id                TEXT NOT NULL DEFAULT '',
		recovery_owner              TEXT NOT NULL DEFAULT '',
		recovery_lease_expires_at   TIMESTAMP,
		entry_ids_json              TEXT NOT NULL DEFAULT '[]',
		attachment_ids_json         TEXT NOT NULL DEFAULT '[]',
		cleanup_locators_json       TEXT NOT NULL DEFAULT '[]',
		created_at                  TIMESTAMP NOT NULL,
		PRIMARY KEY (task_id, from_session_id, to_session_id)
	)
`

func (r *sqliteRepository) ensureSessionTransferCompensationSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, sessionTransferCompensationSchema); err != nil {
		return fmt.Errorf("ensure session transfer compensation schema: %w", err)
	}
	for _, migration := range []struct {
		statement string
		name      string
	}{
		{`ALTER TABLE queue_session_transfer_compensations ADD COLUMN operation_id TEXT NOT NULL DEFAULT ''`, "operation id"},
		{`ALTER TABLE queue_session_transfer_compensations ADD COLUMN attachment_ids_json TEXT NOT NULL DEFAULT '[]'`, "attachment ids"},
		{`ALTER TABLE queue_session_transfer_compensations ADD COLUMN cleanup_locators_json TEXT NOT NULL DEFAULT '[]'`, "cleanup locators"},
		{`ALTER TABLE queue_session_transfer_compensations ADD COLUMN recovery_owner TEXT NOT NULL DEFAULT ''`, "recovery owner"},
		{`ALTER TABLE queue_session_transfer_compensations ADD COLUMN recovery_lease_expires_at TIMESTAMP`, "recovery lease"},
	} {
		if _, err := r.db.ExecContext(ctx, migration.statement); err != nil && !internaldb.IsDuplicateColumnError(err) {
			return fmt.Errorf("add session transfer %s: %w", migration.name, err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE queue_session_transfer_compensations
		SET recovery_owner = operation_id
		WHERE recovery_owner = ''
	`); err != nil {
		return fmt.Errorf("backfill session transfer recovery owner: %w", err)
	}
	return nil
}

func marshalSessionTransferCompensation(
	compensation SessionTransferCompensation,
) (string, string, string, error) {
	entryIDsJSON, err := json.Marshal(compensation.EntryIDs)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal session transfer compensation: %w", err)
	}
	attachmentIDsJSON, err := json.Marshal(compensation.AttachmentIDs)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal session transfer attachment ids: %w", err)
	}
	cleanupLocatorsJSON, err := json.Marshal(compensation.CleanupLocators)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal session transfer cleanup locators: %w", err)
	}
	return string(entryIDsJSON), string(attachmentIDsJSON), string(cleanupLocatorsJSON), nil
}

func (r *sqliteRepository) lockSessionTransferPairTx(
	ctx context.Context,
	tx *sqlx.Tx,
	fromSessionID, toSessionID string,
) (string, string, error) {
	first, second := fromSessionID, toSessionID
	if first > second {
		first, second = second, first
	}
	if err := r.lockSessionTxUnfenced(ctx, tx, first); err != nil {
		return "", "", err
	}
	if first != second {
		if err := r.lockSessionTxUnfenced(ctx, tx, second); err != nil {
			return "", "", err
		}
	}
	return first, second, nil
}

func guardSessionTransferCompensationOwnerTx(
	ctx context.Context,
	tx *sqlx.Tx,
	compensation SessionTransferCompensation,
	first, second string,
) error {
	var conflicts int
	if err := tx.GetContext(ctx, &conflicts, tx.Rebind(`
		SELECT COUNT(*) FROM queue_session_transfer_compensations
		WHERE (from_session_id IN (?, ?) OR to_session_id IN (?, ?))
		  AND NOT (task_id = ? AND from_session_id = ? AND to_session_id = ?)
	`), first, second, first, second, compensation.TaskID,
		compensation.FromSessionID, compensation.ToSessionID); err != nil {
		return fmt.Errorf("check active session transfer: %w", err)
	}
	if conflicts > 0 {
		return ErrSessionTransferInProgress
	}
	var activeOperationID string
	err := tx.GetContext(ctx, &activeOperationID, tx.Rebind(`
		SELECT operation_id FROM queue_session_transfer_compensations
		WHERE task_id = ? AND from_session_id = ? AND to_session_id = ?
	`), compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read active session transfer owner: %w", err)
	}
	if err == nil && activeOperationID != compensation.OperationID {
		return ErrSessionTransferInProgress
	}
	return nil
}

func (r *sqliteRepository) lockTaskForSessionTransferTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	fromSessionID string,
	toSessionID string,
) error {
	if !r.tasksTablePresent {
		return nil
	}
	query := `SELECT id FROM tasks WHERE id = ?`
	if r.db.DriverName() == postgresDriverName {
		query += postgresForUpdateSuffix
	}
	var lockedTaskID string
	if err := tx.GetContext(ctx, &lockedTaskID, r.db.Rebind(query), taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session transfer task %q no longer exists", taskID)
		}
		return fmt.Errorf("lock session transfer task %q: %w", taskID, err)
	}
	if !r.taskSessionsTablePresent {
		return nil
	}
	for _, sessionID := range []string{fromSessionID, toSessionID} {
		if sessionID == "" {
			continue
		}
		var sessionTaskID string
		if err := tx.GetContext(ctx, &sessionTaskID, r.db.Rebind(`
			SELECT task_id FROM task_sessions WHERE id = ?
		`), sessionID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("session transfer session %q no longer exists", sessionID)
			}
			return fmt.Errorf("load session transfer session %q: %w", sessionID, err)
		}
		if sessionTaskID != taskID {
			return fmt.Errorf("session transfer session %q belongs to task %q, not %q", sessionID, sessionTaskID, taskID)
		}
	}
	return nil
}

func (r *sqliteRepository) UpsertSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
) error {
	if compensation.OperationID == "" {
		return errors.New("session transfer operation id is required")
	}
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return err
	}
	entryIDsJSON, attachmentIDsJSON, cleanupLocatorsJSON, err := marshalSessionTransferCompensation(compensation)
	if err != nil {
		return err
	}
	if compensation.CreatedAt.IsZero() {
		compensation.CreatedAt = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session transfer compensation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskForSessionTransferTx(
		ctx, tx, compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID,
	); err != nil {
		return err
	}
	first, second, err := r.lockSessionTransferPairTx(
		ctx, tx, compensation.FromSessionID, compensation.ToSessionID,
	)
	if err != nil {
		return err
	}
	if err := guardSessionTransferCompensationOwnerTx(
		ctx, tx, compensation, first, second,
	); err != nil {
		return err
	}
	leaseExpiresAt := time.Now().UTC().Add(sessionTransferCompensationLeaseDuration)
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO queue_session_transfer_compensations
			(operation_id, recovery_owner, recovery_lease_expires_at, task_id,
			 from_session_id, to_session_id, entry_ids_json, attachment_ids_json,
			 cleanup_locators_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, from_session_id, to_session_id) DO UPDATE SET
			entry_ids_json = excluded.entry_ids_json,
			attachment_ids_json = excluded.attachment_ids_json,
			cleanup_locators_json = excluded.cleanup_locators_json,
			recovery_lease_expires_at = excluded.recovery_lease_expires_at
		WHERE queue_session_transfer_compensations.operation_id = excluded.operation_id
		  AND queue_session_transfer_compensations.recovery_owner = excluded.recovery_owner
	`), compensation.OperationID, compensation.OperationID, leaseExpiresAt,
		compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID,
		entryIDsJSON, attachmentIDsJSON, cleanupLocatorsJSON, compensation.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert session transfer compensation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("session transfer compensation rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSessionTransferOwnershipLost
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session transfer compensation: %w", err)
	}
	return nil
}

func (r *sqliteRepository) DeleteSessionTransferCompensation(
	ctx context.Context,
	operationID, taskID, fromSessionID, toSessionID string,
) error {
	return r.deleteSessionTransferCompensationWithOwner(
		ctx, operationID, operationID, taskID, fromSessionID, toSessionID,
	)
}

func (r *sqliteRepository) deleteSessionTransferCompensationWithOwner(
	ctx context.Context,
	operationID, ownerID, taskID, fromSessionID, toSessionID string,
) error {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete session transfer compensation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := r.lockSessionTransferPairTx(
		ctx, tx, fromSessionID, toSessionID,
	); err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		DELETE FROM queue_session_transfer_compensations
		WHERE operation_id = ? AND recovery_owner = ?
		  AND task_id = ? AND from_session_id = ? AND to_session_id = ?
		  AND recovery_lease_expires_at > ?
	`), operationID, ownerID, taskID, fromSessionID, toSessionID, now)
	if err != nil {
		return fmt.Errorf("delete session transfer compensation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete session transfer compensation rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSessionTransferOwnershipLost
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete session transfer compensation: %w", err)
	}
	return nil
}

func (r *sqliteRepository) claimSessionTransferCompensationRecovery(
	ctx context.Context,
	compensation SessionTransferCompensation,
) (string, error) {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return "", err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin claim session transfer compensation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := r.lockSessionTransferPairTx(
		ctx, tx, compensation.FromSessionID, compensation.ToSessionID,
	); err != nil {
		return "", err
	}
	ownerID := uuid.NewString()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE queue_session_transfer_compensations
		SET recovery_owner = ?, recovery_lease_expires_at = ?
		WHERE operation_id = ? AND task_id = ? AND from_session_id = ? AND to_session_id = ?
		  AND (recovery_lease_expires_at IS NULL OR recovery_lease_expires_at <= ?)
	`), ownerID, now.Add(sessionTransferCompensationLeaseDuration),
		compensation.OperationID, compensation.TaskID, compensation.FromSessionID,
		compensation.ToSessionID, now)
	if err != nil {
		return "", fmt.Errorf("claim session transfer compensation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("claim session transfer compensation rows affected: %w", err)
	}
	if affected != 1 {
		return "", ErrSessionTransferInProgress
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit claim session transfer compensation: %w", err)
	}
	return ownerID, nil
}

func (r *sqliteRepository) renewSessionTransferCompensationLease(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin renew session transfer compensation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := r.lockSessionTransferPairTx(
		ctx, tx, compensation.FromSessionID, compensation.ToSessionID,
	); err != nil {
		return err
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE queue_session_transfer_compensations
		SET recovery_lease_expires_at = ?
		WHERE operation_id = ? AND recovery_owner = ?
		  AND task_id = ? AND from_session_id = ? AND to_session_id = ?
		  AND recovery_lease_expires_at > ?
	`), now.Add(sessionTransferCompensationLeaseDuration),
		compensation.OperationID, ownerID, compensation.TaskID,
		compensation.FromSessionID, compensation.ToSessionID, now)
	if err != nil {
		return fmt.Errorf("renew session transfer compensation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("renew session transfer compensation rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSessionTransferOwnershipLost
	}
	return tx.Commit()
}

func (r *sqliteRepository) ListSessionTransferCompensations(
	ctx context.Context,
) ([]SessionTransferCompensation, error) {
	if err := r.ensureSessionTransferCompensationSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, `
		SELECT operation_id, task_id, from_session_id, to_session_id, entry_ids_json,
		       attachment_ids_json, cleanup_locators_json, created_at
		FROM queue_session_transfer_compensations
		ORDER BY created_at, task_id, from_session_id, to_session_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list session transfer compensations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var compensations []SessionTransferCompensation
	for rows.Next() {
		var compensation SessionTransferCompensation
		var entryIDsJSON, attachmentIDsJSON, cleanupLocatorsJSON string
		if err := rows.Scan(
			&compensation.OperationID,
			&compensation.TaskID,
			&compensation.FromSessionID,
			&compensation.ToSessionID,
			&entryIDsJSON,
			&attachmentIDsJSON,
			&cleanupLocatorsJSON,
			&compensation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session transfer compensation: %w", err)
		}
		if err := json.Unmarshal([]byte(entryIDsJSON), &compensation.EntryIDs); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer compensation: %w", err)
		}
		if err := json.Unmarshal([]byte(attachmentIDsJSON), &compensation.AttachmentIDs); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer attachment ids: %w", err)
		}
		if err := json.Unmarshal([]byte(cleanupLocatorsJSON), &compensation.CleanupLocators); err != nil {
			return nil, fmt.Errorf("unmarshal session transfer cleanup locators: %w", err)
		}
		compensations = append(compensations, compensation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session transfer compensations: %w", err)
	}
	return compensations, nil
}

func authorizeSessionTransferTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	fromSessionID, toSessionID, operationID string,
) error {
	var activeOperationID, activeOwnerID, activeFromSessionID, activeToSessionID string
	var leaseExpiresAt sql.NullTime
	rows, err := tx.QueryxContext(ctx, db.Rebind(`
		SELECT operation_id, recovery_owner, recovery_lease_expires_at,
		       from_session_id, to_session_id
		FROM queue_session_transfer_compensations
		WHERE from_session_id IN (?, ?) OR to_session_id IN (?, ?)
	`), fromSessionID, toSessionID, fromSessionID, toSessionID)
	if err != nil {
		return fmt.Errorf("authorize session transfer: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate session transfer authorization: %w", err)
		}
		if operationID != "" {
			return ErrSessionTransferOwnershipLost
		}
		return nil
	}
	if err := rows.Scan(
		&activeOperationID, &activeOwnerID, &leaseExpiresAt,
		&activeFromSessionID, &activeToSessionID,
	); err != nil {
		return fmt.Errorf("scan session transfer authorization: %w", err)
	}
	if rows.Next() {
		return ErrSessionTransferInProgress
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session transfer authorization: %w", err)
	}
	if operationID == "" ||
		operationID != activeOperationID ||
		fromSessionID != activeFromSessionID ||
		toSessionID != activeToSessionID {
		return ErrSessionTransferInProgress
	}
	if activeOwnerID != operationID || !leaseExpiresAt.Valid || !leaseExpiresAt.Time.After(time.Now().UTC()) {
		return ErrSessionTransferOwnershipLost
	}
	return nil
}

func commitAuthorizedSessionTransferTx(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	fromSessionID, toSessionID, operationID string,
) error {
	if err := authorizeSessionTransferTx(
		ctx, tx, db, fromSessionID, toSessionID, operationID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// GuardSessionTransferInTransaction rejects mutations that race an active
// durable session transfer. Callers must first hold the queue_session_locks
// row for sessionID. A missing compensation table means no active transfer.
func GuardSessionTransferInTransaction(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID string) error {
	if err := guardSessionTransferTx(ctx, tx, db, sessionID); err != nil {
		if internaldb.IsMissingTableError(err) {
			return nil
		}
		return err
	}
	return nil
}

func guardSessionTransferTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID string) error {
	var active bool
	if err := tx.GetContext(ctx, &active, db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM queue_session_transfer_compensations
			WHERE from_session_id = ? OR to_session_id = ?
		)
	`), sessionID, sessionID); err != nil {
		return fmt.Errorf("guard active session transfer: %w", err)
	}
	if active {
		return ErrSessionTransferInProgress
	}
	return nil
}

// ResolveSessionTransferInTransaction maps a source session to the destination
// selected by the active durable transfer. Callers must first hold the
// queue_session_locks row for sessionID.
func ResolveSessionTransferInTransaction(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	sessionID string,
) (string, error) {
	rows, err := tx.QueryxContext(ctx, db.Rebind(`
		SELECT from_session_id, to_session_id
		FROM queue_session_transfer_compensations
		WHERE from_session_id = ? OR to_session_id = ?
	`), sessionID, sessionID)
	if err != nil {
		return "", fmt.Errorf("resolve active session transfer: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("iterate active session transfer: %w", err)
		}
		return sessionID, nil
	}
	var fromSessionID, toSessionID string
	if err := rows.Scan(&fromSessionID, &toSessionID); err != nil {
		return "", fmt.Errorf("scan active session transfer: %w", err)
	}
	if rows.Next() {
		return "", ErrSessionTransferInProgress
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate active session transfer: %w", err)
	}
	if sessionID == fromSessionID {
		return toSessionID, nil
	}
	return sessionID, nil
}

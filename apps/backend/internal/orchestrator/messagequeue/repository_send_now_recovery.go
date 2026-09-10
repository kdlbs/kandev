package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

const sendNowClaimRecoverySchema = `
	CREATE TABLE IF NOT EXISTS queue_send_now_claims (
		session_id  TEXT PRIMARY KEY,
		claim_id    TEXT NOT NULL,
		claim_json  TEXT NOT NULL,
		accepted    INTEGER NOT NULL DEFAULT 0,
		created_at  TIMESTAMP NOT NULL
	)
`

func (r *sqliteRepository) ensureSendNowClaimRecoverySchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, sendNowClaimRecoverySchema); err != nil {
		return fmt.Errorf("ensure Send Now claim recovery schema: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_send_now_claims ADD COLUMN accepted INTEGER NOT NULL DEFAULT 0`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add Send Now claim acceptance: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `ALTER TABLE queue_send_now_claims ADD COLUMN claim_id TEXT NOT NULL DEFAULT ''`); err != nil && !internaldb.IsDuplicateColumnError(err) {
		return fmt.Errorf("add Send Now claim identity: %w", err)
	}
	rows, err := r.db.QueryxContext(ctx, `SELECT session_id FROM queue_send_now_claims WHERE claim_id = ''`)
	if err != nil {
		return fmt.Errorf("list Send Now claims without identities: %w", err)
	}
	var sessionIDs []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Send Now claim without identity: %w", err)
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate Send Now claims without identities: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Send Now claim identity migration rows: %w", err)
	}
	for _, sessionID := range sessionIDs {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE queue_send_now_claims SET claim_id = ?
			WHERE session_id = ? AND claim_id = ''
		`), uuid.NewString(), sessionID); err != nil {
			return fmt.Errorf("backfill Send Now claim identity: %w", err)
		}
	}
	return nil
}

func (r *sqliteRepository) persistSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	claim *SendNowClaim,
) error {
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	if claim.ClaimID == "" {
		claim.ClaimID = uuid.NewString()
	}
	claimJSON, err := json.Marshal(claim)
	if err != nil {
		return fmt.Errorf("marshal Send Now claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_send_now_claims (session_id, claim_id, claim_json, created_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, claim.ClaimID, string(claimJSON), time.Now().UTC()); err != nil {
		if isDuplicateSendNowClaimError(err) {
			return ErrSendNowClaimChanged
		}
		return fmt.Errorf("persist Send Now claim: %w", err)
	}
	return nil
}

func isDuplicateSendNowClaimError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique constraint failed: queue_send_now_claims.session_id")
}

func (r *sqliteRepository) deleteSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_send_now_claims WHERE session_id = ?
	`), sessionID); err != nil {
		return fmt.Errorf("delete Send Now claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) deleteExactSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, claimID string,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_send_now_claims WHERE session_id = ? AND claim_id = ?
	`), sessionID, claimID)
	if err != nil {
		return fmt.Errorf("delete exact Send Now claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete exact Send Now claim rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSendNowClaimChanged
	}
	return nil
}

func (r *sqliteRepository) transferPendingSendNowClaimTx(
	ctx context.Context,
	tx *sqlx.Tx,
	oldSessionID, newSessionID string,
) error {
	var claimID, claimJSON string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT claim_id, claim_json
		FROM queue_send_now_claims
		WHERE session_id = ?
	`), oldSessionID).Scan(&claimID, &claimJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read source Send Now claim: %w", err)
	}
	var destinationClaimID string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT claim_id
		FROM queue_send_now_claims
		WHERE session_id = ?
	`), newSessionID).Scan(&destinationClaimID)
	if err == nil {
		return ErrSendNowClaimChanged
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read destination Send Now claim: %w", err)
	}
	var claim SendNowClaim
	if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
		return fmt.Errorf("unmarshal source Send Now claim: %w", err)
	}
	claim.ClaimID = claimID
	sessionID, err := sendNowClaimSessionID(&claim)
	if err != nil || sessionID != oldSessionID || claim.Dispatch.SessionID != oldSessionID {
		return ErrSendNowClaimChanged
	}
	for sourceIndex := range claim.Sources {
		claim.Sources[sourceIndex].SessionID = newSessionID
	}
	claim.Dispatch.SessionID = newSessionID
	claim.SessionGeneration, err = r.getSendNowGenerationTx(ctx, tx, newSessionID)
	if err != nil {
		return err
	}
	claimJSONBytes, err := json.Marshal(&claim)
	if err != nil {
		return fmt.Errorf("marshal transferred Send Now claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_send_now_claims
		SET session_id = ?, claim_json = ?
		WHERE session_id = ? AND claim_id = ?
	`), newSessionID, string(claimJSONBytes), oldSessionID, claimID); err != nil {
		return fmt.Errorf("transfer Send Now claim: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ListPendingSendNowClaims(ctx context.Context) ([]PendingSendNowClaim, error) {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(
		ctx,
		`SELECT claim_id, claim_json, accepted FROM queue_send_now_claims ORDER BY created_at, session_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list Send Now claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pending []PendingSendNowClaim
	for rows.Next() {
		var claimID, claimJSON string
		var accepted int
		if err := rows.Scan(&claimID, &claimJSON, &accepted); err != nil {
			return nil, fmt.Errorf("scan Send Now claim: %w", err)
		}
		var claim SendNowClaim
		if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
			return nil, fmt.Errorf("unmarshal Send Now claim: %w", err)
		}
		claim.ClaimID = claimID
		pending = append(pending, PendingSendNowClaim{Claim: claim, Accepted: accepted != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Send Now claims: %w", err)
	}
	return pending, nil
}
func (r *sqliteRepository) MarkPendingSendNowClaimAccepted(ctx context.Context, claim *SendNowClaim) error {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return err
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	if claim.ClaimID == "" {
		return ErrSendNowClaimChanged
	}
	tx, err := r.beginSessionMutationTx(ctx, sessionID, "mark Send Now claim accepted")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_send_now_claims SET accepted = 1
		WHERE session_id = ? AND claim_id = ?
	`), sessionID, claim.ClaimID)
	if err != nil {
		return fmt.Errorf("mark Send Now claim accepted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark Send Now claim accepted rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSendNowClaimChanged
	}
	return tx.Commit()
}

func (r *sqliteRepository) DeletePendingSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	if err := r.ensureSendNowClaimRecoverySchema(ctx); err != nil {
		return err
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	if claim.ClaimID == "" {
		return ErrSendNowClaimChanged
	}
	tx, err := r.beginSessionMutationTx(ctx, sessionID, "discard pending Send Now claim")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queue_send_now_claims WHERE session_id = ? AND claim_id = ?
	`), sessionID, claim.ClaimID)
	if err != nil {
		return fmt.Errorf("discard pending Send Now claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("discard pending Send Now claim rows affected: %w", err)
	}
	if affected != 1 {
		return ErrSendNowClaimChanged
	}
	return tx.Commit()
}

func pendingSendNowSessionsForTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) ([]string, error) {
	rows, err := tx.QueryxContext(ctx, `SELECT session_id, claim_json FROM queue_send_now_claims`)
	if err != nil {
		return nil, fmt.Errorf("list task Send Now claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessions []string
	for rows.Next() {
		var sessionID, claimJSON string
		if err := rows.Scan(&sessionID, &claimJSON); err != nil {
			return nil, fmt.Errorf("scan task Send Now claim: %w", err)
		}
		var claim SendNowClaim
		if err := json.Unmarshal([]byte(claimJSON), &claim); err != nil {
			return nil, fmt.Errorf("unmarshal task Send Now claim: %w", err)
		}
		matches := claim.Dispatch.TaskID == taskID
		for i := range claim.Sources {
			matches = matches || claim.Sources[i].TaskID == taskID
		}
		if matches {
			sessions = append(sessions, sessionID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task Send Now claims: %w", err)
	}
	return sessions, nil
}

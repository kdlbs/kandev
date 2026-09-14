package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

const queueAdmissionReceiptSchema = `
CREATE TABLE IF NOT EXISTS queue_admission_receipts (
	 task_id                TEXT NOT NULL,
	 session_id             TEXT NOT NULL,
	 session_incarnation_id TEXT NOT NULL,
	 client_queue_id        TEXT NOT NULL,
	 request_fingerprint    TEXT NOT NULL,
	 accepted_queue_id      TEXT NOT NULL,
	 response_json          TEXT NOT NULL,
	 admitted_at            TIMESTAMP NOT NULL,
	 PRIMARY KEY (task_id, session_id, session_incarnation_id, client_queue_id)
)`

// LookupQueueAdmission checks the durable receipt for one identified request
// without evaluating queue capacity or staged attachment ownership.
func (r *sqliteRepository) LookupQueueAdmission(
	ctx context.Context,
	identity QueueSessionIdentity,
	clientQueueID string,
	candidate *QueuedMessage,
) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, clientQueueID, candidate); err != nil {
		return nil, false, err
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate)
	if err != nil {
		return nil, false, err
	}
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin queue admission lookup tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardQueueAdmissionTx(ctx, tx, identity, nil); err != nil {
		return nil, false, err
	}
	receipt, err := r.readQueueAdmissionReceiptTx(ctx, tx, identity, clientQueueID)
	if err != nil {
		return nil, false, err
	}
	if receipt == nil {
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit queue admission lookup: %w", err)
		}
		return nil, false, nil
	}
	if receipt.Fingerprint != fingerprint {
		return nil, false, ErrQueueIDConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit queue admission replay lookup: %w", err)
	}
	return receipt.Message, true, nil
}

// AdmitQueueMessage commits one ordinary caller-identified admission. The
// receipt is written with the queue insertion or automatic fold, so a replay
// can never repeat a queue mutation after the visible row has changed.
func (r *sqliteRepository) AdmitQueueMessage(
	ctx context.Context,
	identity QueueSessionIdentity,
	clientQueueID string,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, clientQueueID, candidate); err != nil {
		return nil, false, err
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate)
	if err != nil {
		return nil, false, err
	}
	admitted := cloneQueuedMessage(candidate)
	admitted.ID = clientQueueID
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin queue admission tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardQueueAdmissionTx(ctx, tx, identity, policy); err != nil {
		return nil, false, err
	}
	receipt, err := r.readQueueAdmissionReceiptTx(ctx, tx, identity, clientQueueID)
	if err != nil {
		return nil, false, err
	}
	if receipt != nil {
		if receipt.Fingerprint != fingerprint {
			return nil, false, ErrQueueIDConflict
		}
		if err := tx.Commit(); err != nil {
			return nil, false, fmt.Errorf("commit queue admission replay: %w", err)
		}
		return receipt.Message, true, nil
	}
	if err := r.rejectExistingQueueAdmissionIDTx(ctx, tx, admitted); err != nil {
		return nil, false, err
	}
	accepted, err := r.admitQueueCandidateTx(ctx, tx, admitted, claim, maxPerSession, policy)
	if err != nil {
		return nil, false, err
	}
	if err := r.insertQueueAdmissionReceiptTx(ctx, tx, identity, clientQueueID, fingerprint, accepted); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit queue admission: %w", err)
	}
	return accepted, false, nil
}

func validateQueueAdmissionInput(
	identity QueueSessionIdentity,
	clientQueueID string,
	candidate *QueuedMessage,
) error {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return ErrSessionIdentityMismatch
	}
	if clientQueueID == "" || len(clientQueueID) > MaxQueueAdmissionIDLength {
		return errors.New("client queue id is invalid")
	}
	if candidate == nil || candidate.TaskID != identity.TaskID || candidate.SessionID != identity.SessionID {
		return ErrSessionIdentityMismatch
	}
	return nil
}

func (r *sqliteRepository) guardQueueAdmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	policy *AutoMergePolicy,
) error {
	if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
		return err
	}
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		return err
	}
	if err := r.guardSessionTx(ctx, tx, identity.SessionID, identity.TaskID); err != nil {
		return err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return err
	}
	return r.validateAutoMergePolicyTx(ctx, tx, &identity, policy)
}

func (r *sqliteRepository) readQueueAdmissionReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	clientQueueID string,
) (*queueAdmissionReceipt, error) {
	query := `
		SELECT request_fingerprint, response_json
		FROM queue_admission_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ? AND client_queue_id = ?`
	if r.db.DriverName() == postgresDriverName {
		query += postgresForUpdateSuffix
	}
	var fingerprint, responseJSON string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), identity.TaskID, identity.SessionID,
		identity.SessionIncarnationID, clientQueueID).Scan(&fingerprint, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read queue admission receipt: %w", err)
	}
	var message QueuedMessage
	if err := json.Unmarshal([]byte(responseJSON), &message); err != nil {
		return nil, fmt.Errorf("decode queue admission receipt: %w", err)
	}
	return &queueAdmissionReceipt{Fingerprint: fingerprint, Message: &message}, nil
}

func (r *sqliteRepository) rejectExistingQueueAdmissionIDTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
) error {
	var present bool
	err := tx.GetContext(ctx, &present, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM queued_messages WHERE id = ?)
	`), candidate.ID)
	if err != nil {
		return fmt.Errorf("check queue admission id: %w", err)
	}
	if present {
		return ErrQueueIDConflict
	}
	return nil
}

func (r *sqliteRepository) admitQueueCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, error) {
	if candidate.QueuedAt.IsZero() {
		candidate.QueuedAt = time.Now().UTC()
	}
	if err := r.ensureQueueCapacityTx(ctx, tx, candidate.SessionID, maxPerSession); err == nil {
		return r.insertQueueCandidateTx(ctx, tx, candidate, claim, maxPerSession, policy)
	} else if !errors.Is(err, ErrQueueFull) {
		return nil, err
	}
	if claim != nil && len(claim.IDs) > 0 {
		return nil, ErrQueueFull
	}
	if policy == nil || !policy.Enabled {
		return nil, ErrQueueFull
	}
	return r.admitFullQueueCandidateTx(ctx, tx, candidate, claim)
}

func (r *sqliteRepository) insertQueueCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, error) {
	if err := claimOptionalMessageAttachmentsTx(
		ctx, tx, &QueueSessionIdentity{TaskID: candidate.TaskID, SessionID: candidate.SessionID},
		claim, candidate.TaskID, candidate.SessionID,
	); err != nil {
		return nil, err
	}
	if candidate.QueuedAt.IsZero() {
		candidate.QueuedAt = time.Now().UTC()
	}
	if err := insertQueuedMessageInTransaction(ctx, tx, r.db, candidate, maxPerSession); err != nil {
		if isQueuedMessageIDViolation(err) {
			return nil, ErrQueueIDConflict
		}
		return nil, err
	}
	if policy == nil || !policy.Enabled {
		return candidate, nil
	}
	return r.mergeInsertedAdmissionCandidateTx(ctx, tx, candidate)
}

func (r *sqliteRepository) mergeInsertedAdmissionCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
) (*QueuedMessage, error) {
	target, err := readMergeTarget(ctx, r, tx, candidate.SessionID, candidate.Position)
	if errors.Is(err, ErrNoMergeTarget) {
		return candidate, nil
	}
	if err != nil {
		return nil, err
	}
	values, compatible := buildAutoMergedEntry(target, candidate)
	if !compatible {
		return candidate, nil
	}
	blocked, err := r.editLeaseBlocksEntryTx(ctx, tx, candidate.SessionID, target.ID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return candidate, nil
	}
	if err := applyMergeWrites(ctx, r, tx, target, candidate, values.content, values.attachments, values.metadata, candidate.SessionID); err != nil {
		return nil, err
	}
	target.Content = values.content
	target.Attachments = values.attachments
	target.Metadata = values.metadata
	target.QueuedAt = values.queuedAt
	return target, nil
}

func (r *sqliteRepository) admitFullQueueCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
) (*QueuedMessage, error) {
	target, storedContent, storedAttachmentsJSON, storedMetadataJSON, err := r.scanTailWithRawJSON(ctx, tx, candidate.SessionID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrQueueFull
	}
	values, compatible := buildAutoMergedEntry(target, candidate)
	if !compatible {
		return nil, ErrQueueFull
	}
	blocked, err := r.editLeaseBlocksEntryTx(ctx, tx, candidate.SessionID, target.ID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrQueueFull
	}
	if err := claimOptionalMessageAttachmentsTx(
		ctx, tx, &QueueSessionIdentity{TaskID: candidate.TaskID, SessionID: candidate.SessionID},
		claim, candidate.TaskID, candidate.SessionID,
	); err != nil {
		return nil, err
	}
	if err := updateAdmissionMergeTargetTx(
		ctx, r, tx, target, storedContent, storedAttachmentsJSON, storedMetadataJSON, values,
	); err != nil {
		return nil, err
	}
	return target, nil
}

func updateAdmissionMergeTargetTx(
	ctx context.Context,
	r *sqliteRepository,
	tx *sqlx.Tx,
	target *QueuedMessage,
	storedContent, storedAttachmentsJSON, storedMetadataJSON string,
	values *autoMergedValues,
) error {
	attachmentsJSON, err := marshalAttachments(values.attachments)
	if err != nil {
		return err
	}
	metadataJSON, err := marshalMetadata(values.metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages
		SET content = ?, attachments_json = ?, metadata_json = ?, queued_at = ?
		WHERE id = ? AND session_id = ? AND position = ? AND content = ?
		  AND attachments_json = ? AND metadata_json = ?
	`), values.content, attachmentsJSON, metadataJSON, values.queuedAt, target.ID,
		target.SessionID, target.Position, storedContent, storedAttachmentsJSON, storedMetadataJSON)
	if err != nil {
		return fmt.Errorf("update full queue admission target: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrQueueChanged
	}
	target.Content = values.content
	target.Attachments = values.attachments
	target.Metadata = values.metadata
	target.QueuedAt = values.queuedAt
	return nil
}

func (r *sqliteRepository) insertQueueAdmissionReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	clientQueueID, fingerprint string,
	message *QueuedMessage,
) error {
	if message == nil {
		return errors.New("queue admission message is nil")
	}
	snapshot := queueAdmissionResponseSnapshot(message)
	responseJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode queue admission receipt: %w", err)
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queue_admission_receipts
			(task_id, session_id, session_incarnation_id, client_queue_id,
			 request_fingerprint, accepted_queue_id, response_json, admitted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), identity.TaskID, identity.SessionID, identity.SessionIncarnationID, clientQueueID,
		fingerprint, message.ID, string(responseJSON), time.Now().UTC())
	if err != nil {
		if isQueueAdmissionReceiptViolation(err) {
			return ErrQueueIDConflict
		}
		return fmt.Errorf("insert queue admission receipt: %w", err)
	}
	return nil
}

func isQueueAdmissionReceiptViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "queue_admission_receipts")
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: queue_admission_receipts")
}

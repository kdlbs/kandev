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
	fingerprint, err := queueAdmissionFingerprint(identity, candidate, nil)
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

// RetireAsyncQuestionQueueAdmission removes the exact unsent row accepted for
// one async-question outbox id. The immutable admission receipt supplies the
// actual task, session incarnation, and accepted queue id, including parent
// deliveries that were rebound before admission.
func (r *sqliteRepository) RetireAsyncQuestionQueueAdmission(
	ctx context.Context,
	clientQueueID, questionID string,
) (bool, error) {
	identity, acceptedID, found, err := r.asyncQuestionRetirementIdentity(ctx, clientQueueID, questionID)
	if err != nil || !found {
		return false, err
	}
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin async question queue retirement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		return false, err
	}
	removed, err := r.retireAsyncQuestionQueueEntryTx(ctx, tx, identity, acceptedID, clientQueueID, questionID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return removed, nil
}

func (r *sqliteRepository) asyncQuestionRetirementIdentity(
	ctx context.Context,
	clientQueueID, questionID string,
) (QueueSessionIdentity, string, bool, error) {
	if clientQueueID == "" || questionID == "" {
		return QueueSessionIdentity{}, "", false, errors.New("async question queue admission identity is incomplete")
	}
	identity, acceptedID, response, _, found, err := r.readAsyncQuestionAdmissionReceipt(ctx, clientQueueID, questionID)
	if err != nil || !found {
		return QueueSessionIdentity{}, "", false, err
	}
	if !asyncQuestionQueueMessageMatches(response, identity, acceptedID, clientQueueID, questionID) {
		return QueueSessionIdentity{}, "", false, nil
	}
	return identity, acceptedID, true, nil
}

func (r *sqliteRepository) retireAsyncQuestionQueueEntryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	acceptedID, clientQueueID, questionID string,
) (bool, error) {
	currentAcceptedID, currentResponse, found, err := r.readAsyncQuestionAdmissionReceiptTx(
		ctx, tx, identity, clientQueueID,
	)
	if err != nil || !found {
		return false, err
	}
	if currentAcceptedID != acceptedID || !asyncQuestionQueueMessageMatches(
		currentResponse, identity, currentAcceptedID, clientQueueID, questionID,
	) {
		return false, nil
	}
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		  FROM queued_messages
		 WHERE id = ? AND task_id = ? AND session_id = ?
	`), acceptedID, identity.TaskID, identity.SessionID)
	queued, metadataJSON, err := scanQueuedRowWithMetadataJSON(row)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read async question queue retirement candidate: %w", err)
	}
	if !asyncQuestionQueueMessageMatches(queued, identity, acceptedID, clientQueueID, questionID) {
		return false, nil
	}
	reserved, err := isReservedMetadataJSON(metadataJSON)
	if err != nil {
		return false, err
	}
	if reserved {
		return false, r.markReservedAsyncQuestionEntryRetiredTx(ctx, tx, identity, acceptedID, queued, metadataJSON)
	}
	blocked, err := r.editLeaseBlocksEntryTx(ctx, tx, identity.SessionID, acceptedID)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, ErrEditConflict
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages
		 WHERE id = ? AND task_id = ? AND session_id = ? AND queued_by = ? AND metadata_json = ?
	`), acceptedID, identity.TaskID, identity.SessionID, queued.QueuedBy, metadataJSON)
	if err != nil {
		return false, fmt.Errorf("delete retired async question queue entry: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return deleted == 1, nil
}

func (r *sqliteRepository) markReservedAsyncQuestionEntryRetiredTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	acceptedID string,
	queued *QueuedMessage,
	metadataJSON string,
) error {
	metadata := copyMessageMetadata(queued.Metadata, 1)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata[MetadataAsyncQuestionRetired] = true
	retiredJSON, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages SET metadata_json = ?
		 WHERE id = ? AND task_id = ? AND session_id = ? AND queued_by = ? AND metadata_json = ?
	`), retiredJSON, acceptedID, identity.TaskID, identity.SessionID, queued.QueuedBy, metadataJSON)
	if err != nil {
		return fmt.Errorf("mark reserved async question queue entry retired: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrQueueChanged
	}
	return nil
}

func (r *sqliteRepository) readAsyncQuestionAdmissionReceipt(
	ctx context.Context,
	clientQueueID string,
	questionID string,
) (QueueSessionIdentity, string, *QueuedMessage, time.Time, bool, error) {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT task_id, session_id, session_incarnation_id, accepted_queue_id, response_json, admitted_at
		  FROM queue_admission_receipts
		 WHERE client_queue_id = ?
		 ORDER BY task_id, session_id, session_incarnation_id
	`), clientQueueID)
	if err != nil {
		return QueueSessionIdentity{}, "", nil, time.Time{}, false, err
	}
	defer func() { _ = rows.Close() }()
	var (
		matchIdentity QueueSessionIdentity
		matchID       string
		matchMessage  *QueuedMessage
		matchTime     time.Time
	)
	for rows.Next() {
		var identity QueueSessionIdentity
		var acceptedID, responseJSON string
		var admittedAt time.Time
		if err := rows.Scan(&identity.TaskID, &identity.SessionID, &identity.SessionIncarnationID,
			&acceptedID, &responseJSON, &admittedAt); err != nil {
			return QueueSessionIdentity{}, "", nil, time.Time{}, false, err
		}
		var message QueuedMessage
		if err := json.Unmarshal([]byte(responseJSON), &message); err != nil {
			return QueueSessionIdentity{}, "", nil, time.Time{}, false, fmt.Errorf("decode async question queue receipt: %w", err)
		}
		if !asyncQuestionQueueMessageMatches(&message, identity, acceptedID, clientQueueID, questionID) {
			continue
		}
		if matchMessage != nil {
			return QueueSessionIdentity{}, "", nil, time.Time{}, false, errors.New("async question admission receipt is ambiguous")
		}
		matchIdentity = identity
		matchID = acceptedID
		matchMessage = &message
		matchTime = admittedAt
	}
	if err := rows.Err(); err != nil {
		return QueueSessionIdentity{}, "", nil, time.Time{}, false, err
	}
	if matchMessage == nil {
		return QueueSessionIdentity{}, "", nil, time.Time{}, false, nil
	}
	return matchIdentity, matchID, matchMessage, matchTime, true, nil
}

func (r *sqliteRepository) readAsyncQuestionAdmissionReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	clientQueueID string,
) (string, *QueuedMessage, bool, error) {
	query := `
		SELECT accepted_queue_id, response_json
		  FROM queue_admission_receipts
		 WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ? AND client_queue_id = ?`
	if r.db.DriverName() == postgresDriverName {
		query += postgresForUpdateSuffix
	}
	var acceptedID, responseJSON string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), identity.TaskID, identity.SessionID,
		identity.SessionIncarnationID, clientQueueID).Scan(&acceptedID, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	var message QueuedMessage
	if err := json.Unmarshal([]byte(responseJSON), &message); err != nil {
		return "", nil, false, fmt.Errorf("decode async question queue receipt: %w", err)
	}
	return acceptedID, &message, true, nil
}

func asyncQuestionQueueMessageMatches(
	message *QueuedMessage,
	identity QueueSessionIdentity,
	acceptedID, clientQueueID, questionID string,
) bool {
	if message == nil || message.ID != acceptedID || message.TaskID != identity.TaskID ||
		message.SessionID != identity.SessionID || message.QueuedBy != QueuedByAsyncQuestion {
		return false
	}
	if message.Metadata[MetadataAsyncQuestion] != questionID {
		return false
	}
	return true
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
	policy *AdmissionPolicy,
	workflowEntry *WorkflowEntryIdentity,
) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, clientQueueID, candidate); err != nil {
		return nil, false, err
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate, workflowEntry)
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
	if err := r.validateWorkflowEntryTx(ctx, tx, candidate.TaskID, candidate.SessionID, workflowEntry); err != nil {
		return nil, false, err
	}
	if workflowEntry != nil && workflowEntry.EnforceTaskManagementClaim {
		if err := r.ensureQueueCapacityTx(ctx, tx, candidate.SessionID, maxPerSession); err != nil {
			return nil, false, err
		}
	}
	if err := r.rejectExistingQueueAdmissionIDTx(ctx, tx, admitted); err != nil {
		return nil, false, err
	}
	accepted, err := r.admitQueueCandidateTx(ctx, tx, admitted, claim, maxPerSession, policy)
	if err != nil {
		return nil, false, err
	}
	if workflowEntry != nil {
		if err := persistTaskManagementFenceTx(ctx, tx, r.db, accepted.ID, accepted.TaskID, *workflowEntry); err != nil {
			return nil, false, err
		}
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
	policy *AdmissionPolicy,
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
	policy *AdmissionPolicy,
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
	if policy == nil || !policy.AutoMerge.Enabled {
		return nil, ErrQueueFull
	}
	return r.admitFullQueueCandidateTx(ctx, tx, candidate, claim, policy)
}

func (r *sqliteRepository) insertQueueCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AdmissionPolicy,
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
	if policy == nil || !policy.AutoMerge.Enabled {
		return candidate, nil
	}
	return r.mergeInsertedAdmissionCandidateTx(ctx, tx, candidate, policy)
}

func (r *sqliteRepository) mergeInsertedAdmissionCandidateTx(
	ctx context.Context,
	tx *sqlx.Tx,
	candidate *QueuedMessage,
	policy *AdmissionPolicy,
) (*QueuedMessage, error) {
	target, values, err := r.readAutoFoldTargetTx(ctx, tx, candidate, policy)
	if err != nil {
		return nil, err
	}
	if values == nil {
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
	policy *AdmissionPolicy,
) (*QueuedMessage, error) {
	fold, err := r.readCandidateFoldTx(ctx, tx, candidate, policy)
	if err != nil {
		return nil, err
	}
	if fold == nil {
		return nil, ErrQueueFull
	}
	target, values := fold.target, fold.values
	storedContent, storedAttachmentsJSON, storedMetadataJSON := fold.rawContent, fold.rawAttachments, fold.rawMetadata
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
		  AND attachments_json = ? AND metadata_json = ? AND queued_at = ?
	`), values.content, attachmentsJSON, metadataJSON, values.queuedAt, target.ID,
		target.SessionID, target.Position, storedContent, storedAttachmentsJSON, storedMetadataJSON, target.QueuedAt)
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

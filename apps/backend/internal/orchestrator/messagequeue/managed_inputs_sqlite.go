package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const managedInputReceiptSchema = `
CREATE TABLE IF NOT EXISTS managed_input_receipts (
	input_id                 TEXT PRIMARY KEY,
	task_id                  TEXT NOT NULL,
	session_id               TEXT NOT NULL,
	session_incarnation_id   TEXT NOT NULL,
	occurrence_key           TEXT NOT NULL,
	fingerprint              TEXT NOT NULL,
	payload_digest           TEXT NOT NULL,
	payload                  TEXT NOT NULL,
	origin                   TEXT NOT NULL,
	coalesce_key             TEXT NOT NULL DEFAULT '',
	sequence                 BIGINT NOT NULL,
	conversation_revision    BIGINT NOT NULL,
	state                    TEXT NOT NULL,
	created_at               TIMESTAMP NOT NULL,
	updated_at               TIMESTAMP NOT NULL,
	turn_id                  TEXT NOT NULL DEFAULT '',
	execution_id             TEXT NOT NULL DEFAULT '',
	superseded_by            TEXT NOT NULL DEFAULT '',
	outcome                  TEXT NOT NULL DEFAULT '',
	UNIQUE (task_id, session_id, session_incarnation_id, occurrence_key)
);
CREATE INDEX IF NOT EXISTS idx_managed_input_receipts_fifo
	ON managed_input_receipts(task_id, session_id, session_incarnation_id, sequence, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_managed_input_receipts_execution
	ON managed_input_receipts(task_id, session_id, session_incarnation_id, turn_id, execution_id)
	WHERE turn_id <> '' AND execution_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_managed_input_receipts_running
	ON managed_input_receipts(task_id, session_id, session_incarnation_id)
	WHERE state = 'running';
`

const managedInputReceiptColumns = `input_id, occurrence_key, task_id, session_id,
	session_incarnation_id, payload_digest, payload, origin, coalesce_key, sequence,
	conversation_revision, state, created_at, updated_at, turn_id, execution_id,
	superseded_by, outcome, fingerprint`

func (r *sqliteRepository) AdmitManagedInput(
	ctx context.Context,
	identity QueueSessionIdentity,
	request ManagedInputRequest,
	maxPerSession int,
) (ManagedInputReceipt, bool, error) {
	if err := validateManagedInputRequest(identity, request); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	fingerprint, err := managedInputFingerprint(identity, request)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	message := managedInputQueueMessage(identity, request)
	queueFingerprint, err := queueAdmissionFingerprint(identity, message)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input admission", true)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	receipt, replayed, err := r.admitManagedInputTx(
		ctx, tx, identity, request, maxPerSession, fingerprint, queueFingerprint, message,
	)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input admission: %w", err)
	}
	return receipt, replayed, nil
}

//nolint:cyclop // The transaction checks input identity, sequence, and coalescing before insertion.
func (r *sqliteRepository) admitManagedInputTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	request ManagedInputRequest,
	maxPerSession int,
	fingerprint, queueFingerprint string,
	message *QueuedMessage,
) (ManagedInputReceipt, bool, error) {
	if receipt, err := r.getManagedInputByOccurrenceTx(ctx, tx, identity, request.OccurrenceKey); err == nil {
		if receipt.fingerprint != fingerprint {
			return ManagedInputReceipt{}, false, ErrQueueIDConflict
		}
		return receipt, true, nil
	} else if !errors.Is(err, ErrEntryNotFound) {
		return ManagedInputReceipt{}, false, err
	}
	queueReceipt, err := r.readQueueAdmissionReceiptTx(ctx, tx, identity, request.OccurrenceKey)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if queueReceipt != nil {
		return ManagedInputReceipt{}, false, ErrQueueIDConflict
	}
	if err := r.rejectExistingManagedInputIDTx(ctx, tx, request.ID); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if err := r.rejectExistingQueueAdmissionIDTx(ctx, tx, message); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	var replacement *ManagedInputReceipt
	if managedInputCanCoalesce(request) {
		replacement, _, err = r.findPendingPeriodicManagedInputTx(
			ctx, tx, identity, request.CoalesceKey,
		)
		if err != nil {
			return ManagedInputReceipt{}, false, err
		}
	}
	now := time.Now().UTC()
	message.QueuedAt = now
	if replacement == nil {
		if err := insertQueuedMessageInTransaction(ctx, tx, r.db, message, maxPerSession); err != nil {
			return ManagedInputReceipt{}, false, err
		}
	} else {
		if err := r.replacePendingManagedInputTx(ctx, tx, identity, replacement.ID, message, now, maxPerSession); err != nil {
			return ManagedInputReceipt{}, false, err
		}
		if err := r.markManagedInputSupersededTx(ctx, tx, identity, replacement.ID, request.ID, now); err != nil {
			return ManagedInputReceipt{}, false, err
		}
	}
	sequence, err := r.nextManagedInputSequenceTx(ctx, tx, identity)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	receipt := managedInputReceiptFromRequest(identity, request, sequence, now)
	receipt.fingerprint = fingerprint
	if err := r.insertManagedInputReceiptTx(ctx, tx, receipt); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if err := r.insertQueueAdmissionReceiptTx(
		ctx, tx, identity, request.OccurrenceKey, queueFingerprint, message,
	); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	return receipt, false, nil
}

func (r *sqliteRepository) nextManagedInputSequenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
) (int64, error) {
	var sequence int64
	err := tx.GetContext(ctx, &sequence, r.db.Rebind(`
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM managed_input_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?
	`), identity.TaskID, identity.SessionID, identity.SessionIncarnationID)
	if err != nil {
		return 0, fmt.Errorf("allocate managed input sequence: %w", err)
	}
	return sequence, nil
}

func (r *sqliteRepository) GetManagedInput(
	ctx context.Context,
	identity QueueSessionIdentity,
	inputID string,
) (ManagedInputReceipt, error) {
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input read", false)
	if err != nil {
		return ManagedInputReceipt{}, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	receipt, err := r.getManagedInputByIDTx(ctx, tx, identity, inputID)
	if err != nil {
		return ManagedInputReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, fmt.Errorf("commit managed input read: %w", err)
	}
	return receipt, nil
}

func (r *sqliteRepository) GetManagedInputByExecution(
	ctx context.Context,
	identity QueueSessionIdentity,
	turnID, executionID string,
) (ManagedInputReceipt, error) {
	if turnID == "" || executionID == "" {
		return ManagedInputReceipt{}, ErrEntryNotFound
	}
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input execution lookup", false)
	if err != nil {
		return ManagedInputReceipt{}, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	query := `SELECT ` + managedInputReceiptColumns + ` FROM managed_input_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?
		AND turn_id = ? AND execution_id = ?`
	var receipt ManagedInputReceipt
	if err := scanManagedInput(tx.QueryRowxContext(ctx, r.db.Rebind(query),
		identity.TaskID, identity.SessionID, identity.SessionIncarnationID, turnID, executionID), &receipt); err != nil {
		return ManagedInputReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, fmt.Errorf("commit managed input execution lookup: %w", err)
	}
	return receipt, nil
}

func (r *sqliteRepository) ListManagedInputs(
	ctx context.Context,
	identity QueueSessionIdentity,
) ([]ManagedInputReceipt, error) {
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input list", false)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	query := `SELECT ` + managedInputReceiptColumns + ` FROM managed_input_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?
		ORDER BY sequence ASC, created_at ASC, input_id ASC`
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query),
		identity.TaskID, identity.SessionID, identity.SessionIncarnationID)
	if err != nil {
		return nil, fmt.Errorf("list managed input receipts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	receipts := make([]ManagedInputReceipt, 0)
	for rows.Next() {
		var receipt ManagedInputReceipt
		if err := scanManagedInput(rows, &receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed input receipts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close managed input receipts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit managed input list: %w", err)
	}
	return receipts, nil
}

func (r *sqliteRepository) CancelManagedInput(
	ctx context.Context,
	identity QueueSessionIdentity,
	inputID string,
) (ManagedInputReceipt, bool, error) {
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input cancellation", false)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	receipt, err := r.getManagedInputByIDTx(ctx, tx, identity, inputID)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if receipt.State != ManagedInputStateAccepted {
		if err := tx.Commit(); err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input cancellation check: %w", err)
		}
		return receipt, false, nil
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND task_id = ? AND session_id = ?
	`), inputID, identity.TaskID, identity.SessionID); err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("remove cancelled managed input from FIFO: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE managed_input_receipts SET state = ?, updated_at = ?
		WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ? AND state = ?
	`), ManagedInputStateCancelled, now, inputID, identity.TaskID, identity.SessionID,
		identity.SessionIncarnationID, ManagedInputStateAccepted)
	if err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("cancel managed input receipt: %w", err)
	}
	if err := requireOneManagedInputRow(result); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	receipt.State = ManagedInputStateCancelled
	receipt.UpdatedAt = now
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input cancellation: %w", err)
	}
	return receipt, true, nil
}

//nolint:cyclop // The transaction fences the queued-to-running transition on durable identity.
func (r *sqliteRepository) MarkManagedInputRunning(
	ctx context.Context,
	identity QueueSessionIdentity,
	inputID, turnID, executionID string,
) (ManagedInputReceipt, bool, error) {
	if turnID == "" || executionID == "" {
		return ManagedInputReceipt{}, false, errors.New("managed input execution identity is required")
	}
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input start", true)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	receipt, err := r.getManagedInputByIDTx(ctx, tx, identity, inputID)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if receipt.State == ManagedInputStateRunning && receipt.TurnID == turnID && receipt.ExecutionID == executionID {
		if err := tx.Commit(); err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input start replay: %w", err)
		}
		return receipt, false, nil
	}
	if receipt.State != ManagedInputStateAccepted {
		return ManagedInputReceipt{}, false, ErrManagedInputTransition
	}
	if err := r.ensureManagedInputIdleTx(ctx, tx, identity, inputID); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if err := r.ensureManagedExecutionFreeTx(ctx, tx, identity, inputID, turnID, executionID); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	var headID string
	err = tx.GetContext(ctx, &headID, r.db.Rebind(`
		SELECT id FROM queued_messages WHERE session_id = ? ORDER BY position ASC LIMIT 1
	`), identity.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedInputReceipt{}, false, ErrManagedInputNotPending
	}
	if err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("read managed input FIFO head: %w", err)
	}
	if headID != inputID {
		return ManagedInputReceipt{}, false, ErrManagedInputNotHead
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND task_id = ? AND session_id = ?
	`), inputID, identity.TaskID, identity.SessionID)
	if err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("claim managed input from FIFO: %w", err)
	}
	if err := requireOneManagedInputRow(result); err != nil {
		return ManagedInputReceipt{}, false, ErrManagedInputNotPending
	}
	now := time.Now().UTC()
	result, err = tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE managed_input_receipts
		SET state = ?, turn_id = ?, execution_id = ?, updated_at = ?
		WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ? AND state = ?
	`), ManagedInputStateRunning, turnID, executionID, now, inputID, identity.TaskID,
		identity.SessionID, identity.SessionIncarnationID, ManagedInputStateAccepted)
	if err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("mark managed input running: %w", err)
	}
	if err := requireOneManagedInputRow(result); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	receipt.State = ManagedInputStateRunning
	receipt.TurnID = turnID
	receipt.ExecutionID = executionID
	receipt.UpdatedAt = now
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input start: %w", err)
	}
	return receipt, true, nil
}

//nolint:cyclop // The transaction applies each permitted terminal transition atomically.
func (r *sqliteRepository) SettleManagedInput(
	ctx context.Context,
	identity QueueSessionIdentity,
	inputID, turnID, executionID string,
	state ManagedInputState,
	outcome string,
) (ManagedInputReceipt, bool, error) {
	if err := validateManagedInputSettlement(turnID, executionID, state, outcome); err != nil {
		return ManagedInputReceipt{}, false, err
	}
	tx, unlock, err := r.beginManagedInputTx(ctx, identity, "managed input settlement", false)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
		unlock()
	}()
	receipt, err := r.getManagedInputByIDTx(ctx, tx, identity, inputID)
	if err != nil {
		return ManagedInputReceipt{}, false, err
	}
	if receipt.State == state && receipt.TurnID == turnID && receipt.ExecutionID == executionID && receipt.Outcome == outcome {
		if err := tx.Commit(); err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input settlement replay: %w", err)
		}
		return receipt, false, nil
	}
	if receipt.State == ManagedInputStateAccepted && state == ManagedInputStateUncertain &&
		turnID == "" && executionID == "" {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			DELETE FROM queued_messages WHERE id = ? AND task_id = ? AND session_id = ?
		`), inputID, identity.TaskID, identity.SessionID); err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("remove uncertain managed input from FIFO: %w", err)
		}
		now := time.Now().UTC()
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE managed_input_receipts SET state = ?, outcome = ?, updated_at = ?
			WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ?
			AND state = ? AND turn_id = '' AND execution_id = ''
		`), ManagedInputStateUncertain, outcome, now, inputID, identity.TaskID,
			identity.SessionID, identity.SessionIncarnationID, ManagedInputStateAccepted)
		if err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("settle unlinked managed input uncertain: %w", err)
		}
		if err := requireOneManagedInputRow(result); err != nil {
			return ManagedInputReceipt{}, false, ErrManagedInputTransition
		}
		receipt.State = ManagedInputStateUncertain
		receipt.Outcome = outcome
		receipt.UpdatedAt = now
		if err := tx.Commit(); err != nil {
			return ManagedInputReceipt{}, false, fmt.Errorf("commit unlinked managed input uncertainty: %w", err)
		}
		return receipt, true, nil
	}
	if receipt.State != ManagedInputStateRunning || receipt.TurnID != turnID || receipt.ExecutionID != executionID {
		return ManagedInputReceipt{}, false, ErrManagedInputTransition
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE managed_input_receipts SET state = ?, outcome = ?, updated_at = ?
		WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ?
		AND state = ? AND turn_id = ? AND execution_id = ?
	`), state, outcome, now, inputID, identity.TaskID, identity.SessionID,
		identity.SessionIncarnationID, ManagedInputStateRunning, turnID, executionID)
	if err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("settle managed input: %w", err)
	}
	if err := requireOneManagedInputRow(result); err != nil {
		return ManagedInputReceipt{}, false, ErrManagedInputTransition
	}
	receipt.State = state
	receipt.Outcome = outcome
	receipt.UpdatedAt = now
	if err := tx.Commit(); err != nil {
		return ManagedInputReceipt{}, false, fmt.Errorf("commit managed input settlement: %w", err)
	}
	return receipt, true, nil
}

func (r *sqliteRepository) beginManagedInputTx(
	ctx context.Context,
	identity QueueSessionIdentity,
	operation string,
	requireActiveTask bool,
) (*sqlx.Tx, func(), error) {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return nil, nil, ErrSessionIdentityMismatch
	}
	unlock := r.withSessionLock(identity.SessionID)
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		unlock()
		return nil, nil, fmt.Errorf("begin %s: %w", operation, err)
	}
	if err := r.lockManagedInputTaskTx(ctx, tx, identity.TaskID); err != nil {
		_ = tx.Rollback()
		unlock()
		return nil, nil, err
	}
	if requireActiveTask {
		if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
			_ = tx.Rollback()
			unlock()
			return nil, nil, err
		}
	}
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		_ = tx.Rollback()
		unlock()
		return nil, nil, err
	}
	if err := r.guardSessionTx(ctx, tx, identity.SessionID, identity.TaskID); err != nil {
		_ = tx.Rollback()
		unlock()
		return nil, nil, err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		_ = tx.Rollback()
		unlock()
		return nil, nil, err
	}
	return tx, unlock, nil
}

func (r *sqliteRepository) lockManagedInputTaskTx(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if !r.tasksTablePresent {
		return nil
	}
	query := `SELECT id FROM tasks WHERE id = ?`
	if r.db.DriverName() == postgresDriverName {
		query += postgresForUpdateSuffix
	}
	var lockedID string
	if err := tx.GetContext(ctx, &lockedID, r.db.Rebind(query), taskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskInactive
		}
		return fmt.Errorf("lock task for managed input: %w", err)
	}
	return nil
}

func (r *sqliteRepository) getManagedInputByIDTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	inputID string,
) (ManagedInputReceipt, error) {
	query := `SELECT ` + managedInputReceiptColumns + ` FROM managed_input_receipts
		WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ?`
	var receipt ManagedInputReceipt
	err := scanManagedInput(tx.QueryRowxContext(ctx, r.db.Rebind(query), inputID,
		identity.TaskID, identity.SessionID, identity.SessionIncarnationID), &receipt)
	return receipt, err
}

func (r *sqliteRepository) getManagedInputByOccurrenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	occurrenceKey string,
) (ManagedInputReceipt, error) {
	query := `SELECT ` + managedInputReceiptColumns + ` FROM managed_input_receipts
		WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ? AND occurrence_key = ?`
	var receipt ManagedInputReceipt
	err := scanManagedInput(tx.QueryRowxContext(ctx, r.db.Rebind(query), identity.TaskID,
		identity.SessionID, identity.SessionIncarnationID, occurrenceKey), &receipt)
	return receipt, err
}

func (r *sqliteRepository) findPendingPeriodicManagedInputTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	coalesceKey string,
) (*ManagedInputReceipt, int64, error) {
	query := `SELECT ` + prefixManagedInputColumns("m") + `, q.position, q.metadata_json
		FROM managed_input_receipts m
		JOIN queued_messages q ON q.id = m.input_id AND q.task_id = m.task_id AND q.session_id = m.session_id
		WHERE m.task_id = ? AND m.session_id = ? AND m.session_incarnation_id = ?
		AND m.state = ? AND m.origin = ? AND m.coalesce_key = ?
		ORDER BY q.position DESC`
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), identity.TaskID,
		identity.SessionID, identity.SessionIncarnationID, ManagedInputStateAccepted,
		ManagedInputOriginPeriodic, coalesceKey)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var receipt ManagedInputReceipt
		var position int64
		var metadataJSON string
		if err := scanManagedInputWithExtra(rows, &receipt, &position, &metadataJSON); err != nil {
			return nil, 0, err
		}
		metadata := make(map[string]interface{})
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				return nil, 0, fmt.Errorf("unmarshal pending periodic managed input metadata: %w", err)
			}
		}
		queueRow := &QueuedMessage{Metadata: metadata}
		if queueRow.IsReservedInFlight() || queueRow.IsDeliveryAttempted() {
			continue
		}
		return &receipt, position, nil
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate pending periodic managed inputs: %w", err)
	}
	return nil, 0, nil
}

func (r *sqliteRepository) rejectExistingManagedInputIDTx(
	ctx context.Context,
	tx *sqlx.Tx,
	inputID string,
) error {
	var present bool
	if err := tx.GetContext(ctx, &present, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM managed_input_receipts WHERE input_id = ?)
	`), inputID); err != nil {
		return fmt.Errorf("check managed input id: %w", err)
	}
	if present {
		return ErrQueueIDConflict
	}
	return nil
}

func (r *sqliteRepository) insertManagedInputReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	receipt ManagedInputReceipt,
) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO managed_input_receipts
			(input_id, task_id, session_id, session_incarnation_id, occurrence_key, fingerprint,
			 payload_digest, payload, origin, coalesce_key, sequence, conversation_revision,
			 state, created_at, updated_at, turn_id, execution_id, superseded_by, outcome)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), receipt.ID, receipt.TaskID, receipt.SessionID, receipt.SessionIncarnationID,
		receipt.OccurrenceKey, receipt.fingerprint, receipt.PayloadDigest, receipt.Payload,
		receipt.Origin, receipt.CoalesceKey, receipt.Sequence, receipt.ConversationRevision,
		receipt.State, receipt.CreatedAt, receipt.UpdatedAt, receipt.TurnID,
		receipt.ExecutionID, receipt.SupersededBy, receipt.Outcome)
	if err != nil {
		return fmt.Errorf("insert managed input receipt: %w", err)
	}
	return nil
}

func (r *sqliteRepository) replacePendingManagedInputTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	oldID string,
	message *QueuedMessage,
	now time.Time,
	maxPerSession int,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND task_id = ? AND session_id = ?
	`), oldID, identity.TaskID, identity.SessionID)
	if err != nil {
		return fmt.Errorf("remove superseded periodic managed input from FIFO: %w", err)
	}
	if err := requireOneManagedInputRow(result); err != nil {
		return err
	}
	message.QueuedAt = now
	return insertQueuedMessageInTransaction(ctx, tx, r.db, message, maxPerSession)
}

func (r *sqliteRepository) markManagedInputSupersededTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	oldID, newID string,
	now time.Time,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE managed_input_receipts SET state = ?, superseded_by = ?, updated_at = ?
		WHERE input_id = ? AND task_id = ? AND session_id = ? AND session_incarnation_id = ?
		AND state = ? AND origin = ?
	`), ManagedInputStateSuperseded, newID, now, oldID, identity.TaskID,
		identity.SessionID, identity.SessionIncarnationID, ManagedInputStateAccepted,
		ManagedInputOriginPeriodic)
	if err != nil {
		return fmt.Errorf("mark managed input superseded: %w", err)
	}
	return requireOneManagedInputRow(result)
}

func (r *sqliteRepository) ensureManagedExecutionFreeTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	inputID, turnID, executionID string,
) error {
	var present bool
	if err := tx.GetContext(ctx, &present, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM managed_input_receipts
			WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?
			AND input_id <> ? AND turn_id = ? AND execution_id = ?
		)
	`), identity.TaskID, identity.SessionID, identity.SessionIncarnationID,
		inputID, turnID, executionID); err != nil {
		return fmt.Errorf("check managed execution receipt: %w", err)
	}
	if present {
		return ErrManagedInputTransition
	}
	return nil
}

func (r *sqliteRepository) ensureManagedInputIdleTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	inputID string,
) error {
	var present bool
	if err := tx.GetContext(ctx, &present, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM managed_input_receipts
			WHERE task_id = ? AND session_id = ? AND session_incarnation_id = ?
			AND input_id <> ? AND state = ?
		)
	`), identity.TaskID, identity.SessionID, identity.SessionIncarnationID,
		inputID, ManagedInputStateRunning); err != nil {
		return fmt.Errorf("check managed conversation running input: %w", err)
	}
	if present {
		return ErrManagedInputBusy
	}
	return nil
}

func scanManagedInput(scanner interface{ Scan(...interface{}) error }, receipt *ManagedInputReceipt) error {
	err := scanner.Scan(
		&receipt.ID, &receipt.OccurrenceKey, &receipt.TaskID, &receipt.SessionID,
		&receipt.SessionIncarnationID, &receipt.PayloadDigest, &receipt.Payload,
		&receipt.Origin, &receipt.CoalesceKey, &receipt.Sequence,
		&receipt.ConversationRevision, &receipt.State, &receipt.CreatedAt,
		&receipt.UpdatedAt, &receipt.TurnID, &receipt.ExecutionID,
		&receipt.SupersededBy, &receipt.Outcome, &receipt.fingerprint,
	)
	return managedInputScanError(err)
}

func scanManagedInputWithExtra(
	scanner interface{ Scan(...interface{}) error },
	receipt *ManagedInputReceipt,
	extra ...interface{},
) error {
	destinations := []interface{}{
		&receipt.ID, &receipt.OccurrenceKey, &receipt.TaskID, &receipt.SessionID,
		&receipt.SessionIncarnationID, &receipt.PayloadDigest, &receipt.Payload,
		&receipt.Origin, &receipt.CoalesceKey, &receipt.Sequence,
		&receipt.ConversationRevision, &receipt.State, &receipt.CreatedAt,
		&receipt.UpdatedAt, &receipt.TurnID, &receipt.ExecutionID,
		&receipt.SupersededBy, &receipt.Outcome, &receipt.fingerprint,
	}
	destinations = append(destinations, extra...)
	return managedInputScanError(scanner.Scan(destinations...))
}

func managedInputScanError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("scan managed input receipt: %w", err)
	}
	return nil
}

func prefixManagedInputColumns(prefix string) string {
	columns := strings.Split(managedInputReceiptColumns, ",")
	for index := range columns {
		columns[index] = prefix + "." + strings.TrimSpace(columns[index])
	}
	return strings.Join(columns, ", ")
}

func requireOneManagedInputRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read managed input mutation count: %w", err)
	}
	if affected != 1 {
		return ErrManagedInputTransition
	}
	return nil
}

func initManagedInputSchema(db *sqlx.DB) error {
	if _, err := db.Exec(managedInputReceiptSchema); err != nil {
		return fmt.Errorf("create managed input receipts: %w", err)
	}
	return nil
}

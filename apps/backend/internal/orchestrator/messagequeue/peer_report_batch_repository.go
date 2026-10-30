package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// FoldPeerReportBatch commits one recovery owner before dispatch. Folded claims
// are absorbed into the still-pending leader, not acknowledged by an executor.
// Startup therefore parks the combined leader under its existing unknown
// delivery policy; this operation grants no authority to replay it.
func (r *sqliteRepository) FoldPeerReportBatch(
	ctx context.Context, identity QueueSessionIdentity, composed *QueuedMessage, batch []*QueuedMessage,
) error {
	messageJSON, err := validatePeerReportBatch(identity, composed, batch)
	if err != nil {
		return err
	}
	if err := r.ensureQueueDispatchRecoverySchema(ctx); err != nil {
		return err
	}
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.beginSessionMutationTx(ctx, identity.SessionID, "fold peer reports")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
		return err
	}
	if err := r.guardSessionTx(ctx, tx, identity.SessionID, identity.TaskID); err != nil {
		return err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return err
	}
	for _, entry := range batch {
		if err := r.validateFreshPeerReportClaimTx(ctx, tx, entry); err != nil {
			return err
		}
	}
	if err := r.writePeerReportFoldTx(ctx, tx, composed, batch, messageJSON); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		// A commit error does not establish whether ownership transferred. Keep
		// the stored claims for reconciliation instead of replaying originals.
		return fmt.Errorf("%w: commit peer report fold: %v", ErrQueueDispatchClaimChanged, err)
	}
	return nil
}

func validatePeerReportBatch(
	identity QueueSessionIdentity, composed *QueuedMessage, batch []*QueuedMessage,
) ([]byte, error) {
	if len(batch) < 2 || len(batch) > MaxPeerReportBatchEntries || composed == nil {
		return nil, ErrQueueDispatchClaimChanged
	}
	seen := make(map[string]bool, len(batch))
	batchBytes := 0
	for _, entry := range batch {
		if !freshPeerReportBatchEntry(identity, entry) || seen[entry.ID] {
			return nil, ErrQueueDispatchClaimChanged
		}
		if !peerReportFitsBatch(batch[0], entry, batchBytes) {
			return nil, ErrQueueDispatchClaimChanged
		}
		seen[entry.ID] = true
		batchBytes += len(entry.Content)
	}
	expected, err := json.Marshal(ComposePeerReportBatch(batch))
	if err != nil {
		return nil, fmt.Errorf("marshal peer report batch: %w", err)
	}
	actual, err := json.Marshal(composed)
	if err != nil {
		return nil, fmt.Errorf("marshal composed peer report: %w", err)
	}
	if string(actual) != string(expected) {
		return nil, ErrQueueDispatchClaimChanged
	}
	return actual, nil
}

func freshPeerReportBatchEntry(identity QueueSessionIdentity, entry *QueuedMessage) bool {
	return IsOrdinaryPeerReport(entry) && entry.TaskID == identity.TaskID && entry.SessionID == identity.SessionID &&
		entry.dispatchAttemptID != "" && entry.reservationGenerationsCaptured
}

func (r *sqliteRepository) validateFreshPeerReportClaimTx(
	ctx context.Context, tx *sqlx.Tx, entry *QueuedMessage,
) error {
	current := *entry
	if err := r.captureReservationGenerationsTx(ctx, tx, &current); err != nil {
		return err
	}
	if current.reservationSessionGeneration != entry.reservationSessionGeneration ||
		current.reservationLifecycleGeneration != entry.reservationLifecycleGeneration {
		return ErrQueueDispatchClaimChanged
	}
	var sessionID, attemptID, messageJSON, protocol, submissionID, payloadHash string
	var accepted int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, attempt_id, message_json, accepted, protocol_mode, submission_id, payload_hash
		FROM queue_dispatch_claims WHERE entry_id = ?
	`), entry.ID).Scan(&sessionID, &attemptID, &messageJSON, &accepted, &protocol, &submissionID, &payloadHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrQueueDispatchClaimChanged
	}
	if err != nil {
		return fmt.Errorf("read peer report dispatch claim: %w", err)
	}
	expected, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal peer report dispatch claim: %w", err)
	}
	if sessionID != entry.SessionID || attemptID != entry.dispatchAttemptID || accepted != 0 ||
		protocol != DeliveryProtocolPending || submissionID != "queue:"+entry.dispatchAttemptID || payloadHash != "" ||
		messageJSON != string(expected) {
		return ErrQueueDispatchClaimChanged
	}
	return nil
}

func (r *sqliteRepository) writePeerReportFoldTx(
	ctx context.Context, tx *sqlx.Tx, composed *QueuedMessage, batch []*QueuedMessage, messageJSON []byte,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_dispatch_claims SET message_json = ?
		WHERE entry_id = ? AND session_id = ? AND attempt_id = ? AND accepted = 0
	`), string(messageJSON), composed.ID, composed.SessionID, composed.dispatchAttemptID)
	if err := peerReportFoldRowsAffected(result, err); err != nil {
		return err
	}
	for _, entry := range batch[1:] {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE queue_dispatch_claims SET accepted = 1
			WHERE entry_id = ? AND session_id = ? AND attempt_id = ? AND accepted = 0
		`), entry.ID, entry.SessionID, entry.dispatchAttemptID)
		if err := peerReportFoldRowsAffected(result, err); err != nil {
			return err
		}
	}
	return nil
}

func peerReportFoldRowsAffected(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("write peer report fold: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("peer report fold rows affected: %w", err)
	}
	if affected != 1 {
		return ErrQueueDispatchClaimChanged
	}
	return nil
}

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrStepCompletionStale means the calling turn no longer owns the current entry.
var ErrStepCompletionStale = errors.New("workflow entry changed before signal was recorded")

// ClaimStepCompletionSignal serializes entry validation and source-specific
// deduplication against task moves. A nil entry is reserved for the no-turn
// compatibility path.
//
// The idempotency key is (session, source turn, workflow entry). The first
// accepted payload wins within one source turn; a later current turn in the
// same entry replaces an unconsumed earlier source. A receipt is written with
// the signal so a retry can distinguish acceptance from a committed move.
func (r *Repository) ClaimStepCompletionSignal(ctx context.Context, taskID, sessionID string, signal models.PendingStepCompletionSignal) (models.PendingStepCompletionSignal, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return signal, false, err
	}
	defer func() { _ = tx.Rollback() }()
	entry, err := r.lockStepCompletionEntryTx(ctx, tx, taskID, signal)
	if err != nil {
		return signal, false, err
	}
	signal.WorkflowStepTransitionID = &entry
	metadata, err := r.lockStepCompletionSessionTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return signal, false, err
	}
	bag, err := decodeStepCompletionMetadata(metadata)
	if err != nil {
		return signal, false, err
	}
	pending, hasPending := models.LoadPendingStepSignal(bag)
	receipt, hasReceipt := models.LoadStepCompletionReceipt(bag)

	if hasPending && pending.MatchesWorkflowEntry(signal.StepID, entry) {
		if pending.SourceTurnID == signal.SourceTurnID {
			return pending, false, tx.Commit()
		}
		// A no-turn compatibility claim is entry-wide first-wins and can never
		// replace a signal that already has source identity.
		if signal.SourceTurnID == "" {
			return pending, false, tx.Commit()
		}
	}
	if hasReceipt && receipt.MatchesSource(signal) {
		// A different bag entry means this receipt was already consumed or the
		// bag was replaced; never let the receipt mask a fresh pending source.
		if !hasPending || pending.SignalID == receipt.SignalID {
			return models.SignalFromReceipt(receipt), false, tx.Commit()
		}
	}
	if signal.SourceTurnID != "" {
		current, found, err := r.latestTurnIDForClaimTx(ctx, tx, sessionID)
		if err != nil {
			return signal, false, err
		}
		if !found || current != signal.SourceTurnID {
			return signal, false, ErrStepCompletionStale
		}
	}

	signal.SignalID = models.NewStepCompletionSignalID()
	acceptedReceipt := models.NewStepCompletionReceipt(taskID, sessionID, signal, signal.SignaledAt)
	if err := r.writeStepCompletionClaimTx(ctx, tx, sessionID, signal, acceptedReceipt); err != nil {
		return signal, false, err
	}
	return signal, true, tx.Commit()
}

// lockStepCompletionSessionTx validates ownership and returns the current
// metadata snapshot under the task lock already held by the claim.
func (r *Repository) lockStepCompletionSessionTx(ctx context.Context, tx *sql.Tx, taskID, sessionID string) (string, error) {
	var owner, metadata string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id,COALESCE(NULLIF(metadata,''),'{}') FROM task_sessions WHERE id=?`), sessionID).Scan(&owner, &metadata)
	if err != nil {
		return "", err
	}
	if owner != taskID {
		return "", ErrStepCompletionStale
	}
	return metadata, nil
}

func decodeStepCompletionMetadata(metadata string) (map[string]interface{}, error) {
	var bag map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &bag); err != nil {
		return nil, fmt.Errorf("decode completion metadata: %w", err)
	}
	if bag == nil {
		bag = map[string]interface{}{}
	}
	return bag, nil
}

// latestTurnIDForClaimTx rechecks the captured source against the session's
// current persisted turn. It reads under the already-held task lock and never
// takes the session-turn advisory lock, preserving the task-first order used by
// stamped turn creation.
func (r *Repository) latestTurnIDForClaimTx(ctx context.Context, tx *sql.Tx, sessionID string) (string, bool, error) {
	query := fmt.Sprintf(`
		SELECT id FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = ? AND %s
		ORDER BY turn_row.started_at DESC, turn_row.created_at DESC, turn_row.id DESC
		LIMIT 1
	`, turnHistoryPredicate(r.db.DriverName(), "turn_row"))
	var turnID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), sessionID).Scan(&turnID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return turnID, true, nil
}

// writeStepCompletionClaimTx writes the winning signal and its pending receipt
// together. Both writes share the claim transaction.
func (r *Repository) writeStepCompletionClaimTx(ctx context.Context, tx *sql.Tx, sessionID string, signal models.PendingStepCompletionSignal, receipt models.StepCompletionReceipt) error {
	if err := r.updateStepCompletionMetadataKeyTx(ctx, tx, sessionID, models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		return err
	}
	return r.updateStepCompletionMetadataKeyTx(ctx, tx, sessionID, models.SessionMetaKeyLastStepCompletionReceipt, receipt)
}

func (r *Repository) updateStepCompletionMetadataKeyTx(ctx context.Context, tx *sql.Tx, sessionID, key string, value interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), key, string(raw), r.nowUTC(), sessionID)...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// SetStepCompletionReceiptOutcome updates a receipt's outcome only while the
// exact accepted signal still owns the bounded slot. A delayed writer therefore
// cannot mark a replacement as blocked or unconfigured.
func (r *Repository) SetStepCompletionReceiptOutcome(ctx context.Context, sessionID, signalID, outcome, reason string) (bool, error) {
	if sessionID == "" || signalID == "" || outcome == "" {
		return false, nil
	}
	session, err := r.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	if !ok || receipt.SignalID != signalID {
		return false, nil
	}
	if receipt.IsCommitted() {
		// A delayed not_configured or blocked writer must never downgrade
		// committed transition proof.
		return false, nil
	}
	receipt.Outcome = outcome
	receipt.Reason = reason
	expected, present := session.Metadata[models.SessionMetaKeyLastStepCompletionReceipt]
	if !present {
		return false, nil
	}
	return r.SetSessionMetadataKeyIfJSONValue(ctx, sessionID, models.SessionMetaKeyLastStepCompletionReceipt, expected, receipt)
}

// GetStepCompletionReceipt returns the retained receipt when it matches the
// requested signal identity.
func (r *Repository) GetStepCompletionReceipt(ctx context.Context, sessionID, signalID string) (models.StepCompletionReceipt, bool, error) {
	session, err := r.GetTaskSession(ctx, sessionID)
	if err != nil {
		return models.StepCompletionReceipt{}, false, err
	}
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	if !ok || (signalID != "" && receipt.SignalID != signalID) {
		return models.StepCompletionReceipt{}, false, nil
	}
	return receipt, true, nil
}

func (r *Repository) lockStepCompletionEntryTx(ctx context.Context, tx *sql.Tx, taskID string, signal models.PendingStepCompletionSignal) (int64, error) {
	_, stepID, found, err := r.readTaskStepInTx(ctx, tx, taskID)
	if err != nil {
		return 0, err
	}
	if !found || stepID != signal.StepID {
		return 0, ErrStepCompletionStale
	}
	entry, err := r.getLatestTaskStepTransitionIDTx(ctx, tx, taskID)
	if err != nil {
		return 0, err
	}
	if signal.WorkflowStepTransitionID != nil && *signal.WorkflowStepTransitionID != entry {
		return 0, ErrStepCompletionStale
	}
	return entry, nil
}

// ErrStepCompletionReceiptMissing means a guarded completion committed without
// its matching accepted receipt. The move must roll back so the signal remains
// retryable.
var ErrStepCompletionReceiptMissing = errors.New("step completion receipt is missing")

// stepCompletionSignalMatchesGuardTx re-reads the persisted pending signal
// under the move transaction. A removed or replaced source cannot authorize a
// prepared transition. A legacy guard without signal identity keeps its
// entry-only fence.
func (r *Repository) stepCompletionSignalMatchesGuardTx(ctx context.Context, tx *sql.Tx, taskID, stepID string) (bool, error) {
	guard, ok := models.WorkflowStepCompletionGuardFromContext(ctx, taskID, stepID)
	if !ok || guard.SignalID == "" || guard.SessionID == "" {
		return true, nil
	}
	metadata, err := r.readStepCompletionSessionMetadataTx(ctx, tx, taskID, guard.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	bag, err := decodeStepCompletionMetadata(metadata)
	if err != nil {
		return false, err
	}
	pending, ok := models.LoadPendingStepSignal(bag)
	if !ok {
		return false, nil
	}
	return pending.SignalID == guard.SignalID, nil
}

// recordStepCompletionProofTx persists the committed ledger identity on the
// matching receipt in the same transaction as the move.
func (r *Repository) recordStepCompletionProofTx(ctx context.Context, tx *sql.Tx, taskID, stepID string, task *models.Task, admitted bool) error {
	guard, ok := models.WorkflowStepCompletionGuardFromContext(ctx, taskID, stepID)
	if !ok || guard.SignalID == "" || guard.SessionID == "" {
		return nil
	}
	metadata, err := r.readStepCompletionSessionMetadataTx(ctx, tx, taskID, guard.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrStepCompletionReceiptMissing, guard.SignalID)
		}
		return err
	}
	bag, err := decodeStepCompletionMetadata(metadata)
	if err != nil {
		return err
	}
	receipt, ok := models.LoadStepCompletionReceipt(bag)
	if !ok || receipt.SignalID != guard.SignalID {
		return fmt.Errorf("%w: %s", ErrStepCompletionReceiptMissing, guard.SignalID)
	}
	committed := receipt.WithTransition(models.StepCompletionTransition{
		LedgerID:   task.WorkflowStepTransitionID,
		WorkflowID: task.WorkflowID,
		StepID:     task.WorkflowStepID,
		Queued:     !admitted,
	})
	return r.updateStepCompletionMetadataKeyTx(ctx, tx, guard.SessionID, models.SessionMetaKeyLastStepCompletionReceipt, committed)
}

func (r *Repository) readStepCompletionSessionMetadataTx(ctx context.Context, tx *sql.Tx, taskID, sessionID string) (string, error) {
	var owner, metadata string
	err := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT task_id,COALESCE(NULLIF(metadata,''),'{}') FROM task_sessions WHERE id=?`,
	), sessionID).Scan(&owner, &metadata)
	if err != nil {
		return "", err
	}
	if owner != taskID {
		return "", sql.ErrNoRows
	}
	return metadata, nil
}

// rebaseCompletionHandoff overlays only the carry slot owned by this completion.
// All unrelated task metadata comes from the locked current row.
func rebaseCompletionHandoff(ctx context.Context, requested, current *models.Task, stepID string) {
	if !models.WorkflowStepCompletionOwnsHandoff(ctx, requested.ID, stepID) {
		return
	}
	if current.Metadata == nil {
		current.Metadata = make(map[string]interface{})
	}
	if carry, present := requested.Metadata[models.MetaKeyStepHandoffCarry]; present {
		current.Metadata[models.MetaKeyStepHandoffCarry] = carry
	} else {
		delete(current.Metadata, models.MetaKeyStepHandoffCarry)
	}
}

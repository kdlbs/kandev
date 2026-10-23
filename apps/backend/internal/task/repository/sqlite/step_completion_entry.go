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

// ClaimStepCompletionSignal serializes entry validation and first-signal-wins
// against task moves. A nil entry is reserved for the no-turn compatibility path.
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
	stepID := signal.StepID
	signal.WorkflowStepTransitionID = &entry
	var owner, metadata string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id,COALESCE(NULLIF(metadata,''),'{}') FROM task_sessions WHERE id=?`), sessionID).Scan(&owner, &metadata)
	if err != nil {
		return signal, false, err
	}
	if owner != taskID {
		return signal, false, ErrStepCompletionStale
	}
	var bag map[string]interface{}
	if err = json.Unmarshal([]byte(metadata), &bag); err != nil {
		return signal, false, fmt.Errorf("decode completion metadata: %w", err)
	}
	if stored, ok := models.LoadPendingStepSignal(bag); ok && stored.WorkflowStepTransitionID != nil && stored.MatchesWorkflowEntry(stepID, entry) {
		return stored, false, tx.Commit()
	}
	raw, err := json.Marshal(signal)
	if err != nil {
		return signal, false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), models.SessionMetaKeyPendingStepCompletion, string(raw), r.nowUTC(), sessionID)...)
	if err != nil {
		return signal, false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return signal, false, err
	}
	if count != 1 {
		return signal, false, sql.ErrNoRows
	}
	return signal, true, tx.Commit()
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

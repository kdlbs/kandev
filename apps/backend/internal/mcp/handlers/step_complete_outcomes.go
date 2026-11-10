package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// stepCompletionStepEntryReader exposes the task's current workflow entry for a
// fallback projection when a repository has no single-snapshot reader.
type stepCompletionStepEntryReader interface {
	GetTaskWorkflowStepEntry(ctx context.Context, taskID string) (workflowID, stepID string, entryID int64, err error)
}

// stepCompletionSnapshotReader returns the route, entry, and primary session
// from one statement-consistent read. Production SQLite implements it.
type stepCompletionSnapshotReader interface {
	GetTaskCompletionSnapshot(ctx context.Context, taskID string) (models.TaskCompletionSnapshot, error)
}

func stepCompletionSignalSource(sessionID string, signal models.PendingStepCompletionSignal) map[string]interface{} {
	source := map[string]interface{}{
		"session_id": sessionID,
		"step_id":    signal.StepID,
		"turn_known": signal.SourceTurnID != "",
	}
	if signal.SourceTurnID != "" {
		source["turn_id"] = signal.SourceTurnID
	}
	if signal.WorkflowStepTransitionID != nil {
		source["entry_id"] = *signal.WorkflowStepTransitionID
	}
	return source
}

func stepCompletionOutcome(status, reason string) map[string]interface{} {
	outcome := map[string]interface{}{"status": status}
	if reason != "" {
		outcome["reason"] = reason
	}
	return outcome
}

// stepCompletionReceiptOutcome projects a retained receipt. A pending receipt
// whose bag entry is gone was cancelled or superseded, never silently applied.
func stepCompletionReceiptOutcome(receipt *models.StepCompletionReceipt, pending bool) map[string]interface{} {
	if receipt == nil {
		return stepCompletionOutcome(models.StepCompletionOutcomeUnproven, "")
	}
	if receipt.Outcome == models.StepCompletionOutcomePending && !pending {
		return stepCompletionOutcome(models.StepCompletionOutcomeSuperseded, models.StepCompletionReasonSignalCancelled)
	}
	return stepCompletionOutcome(receipt.Outcome, receipt.Reason)
}

func stepCompletionTransitionProof(receipt *models.StepCompletionReceipt) map[string]interface{} {
	if receipt == nil || !receipt.HasProof() {
		return nil
	}
	transition := map[string]interface{}{
		"ledger_id": receipt.Transition.LedgerID,
		"step_id":   receipt.Transition.StepID,
	}
	if receipt.Transition.WorkflowID != "" {
		transition["workflow_id"] = receipt.Transition.WorkflowID
	}
	if receipt.Transition.Queued {
		transition["queued"] = true
	}
	return transition
}

func (h *Handlers) stepCompletionCurrent(ctx context.Context, taskID string, fallback *models.Task) map[string]interface{} {
	if reader, ok := h.sessionRepo.(stepCompletionSnapshotReader); ok {
		snapshot, err := reader.GetTaskCompletionSnapshot(ctx, taskID)
		if err == nil {
			return map[string]interface{}{
				"state":              string(snapshot.State),
				"workflow_id":        snapshot.WorkflowID,
				"step_id":            snapshot.StepID,
				"queued_for_step_id": snapshot.QueuedForStepID,
				"entry_id":           snapshot.EntryID,
				"primary_session_id": snapshot.PrimarySessionID,
			}
		}
	}
	if fallback == nil {
		return nil
	}
	// Fallback repositories expose only the task row; omit entry and primary
	// session rather than splicing separately read values into this snapshot.
	current := map[string]interface{}{
		"state":       string(fallback.State),
		"workflow_id": fallback.WorkflowID,
		"step_id":     fallback.WorkflowStepID,
	}
	if fallback.QueuedForStepID != "" {
		current["queued_for_step_id"] = fallback.QueuedForStepID
	}
	return current
}

// stepCompletionReceiptWriter persists a read-model outcome for one exact
// accepted signal. Repositories that do not implement it keep legacy results.
type stepCompletionReceiptWriter interface {
	SetStepCompletionReceiptOutcome(ctx context.Context, sessionID, signalID, outcome, reason string) (bool, error)
}

func (h *Handlers) persistStepCompletionReceiptOutcome(ctx context.Context, sessionID, signalID, outcome, reason string) {
	writer, ok := h.sessionRepo.(stepCompletionReceiptWriter)
	if !ok || signalID == "" {
		return
	}
	if _, err := writer.SetStepCompletionReceiptOutcome(ctx, sessionID, signalID, outcome, reason); err != nil {
		h.logger.Warn("failed to persist step-completion receipt outcome",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// exactStepCompletionReceipt returns the retained receipt when the source
// captured at admission matches it and the receipt still describes the current
// entry or a committed destination. It grants no new authority; callers use it
// only for a read-only duplicate response.
func (h *Handlers) exactStepCompletionReceipt(ctx context.Context, task *models.Task, session *models.TaskSession, launch stepCompletionLaunch) (*models.StepCompletionReceipt, bool) {
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	if !ok {
		return nil, false
	}
	key := models.PendingStepCompletionSignal{
		SourceTurnID:             launch.TurnID,
		StepID:                   launch.StepID,
		WorkflowStepTransitionID: launch.Entry,
	}
	if !receipt.MatchesSource(key) {
		return nil, false
	}
	if reader, ok := h.sessionRepo.(stepCompletionSnapshotReader); ok {
		if snapshot, err := reader.GetTaskCompletionSnapshot(ctx, task.ID); err == nil {
			if snapshot.StepID == receipt.StepID {
				// A-to-B-to-A creates a new entry at the same step. A retry from
				// the prior entry stays stale even though the step ID matches.
				return &receipt, receipt.SameEntry(&snapshot.EntryID)
			}
			return &receipt, receipt.IsCommitted()
		}
	}
	if task.WorkflowStepID == receipt.StepID {
		// A-to-B-to-A creates a new entry at the same step. A retry from the
		// prior entry stays stale even though the step ID matches the receipt.
		reader, ok := h.sessionRepo.(stepCompletionStepEntryReader)
		if !ok {
			return &receipt, true
		}
		if _, _, currentEntry, err := reader.GetTaskWorkflowStepEntry(ctx, task.ID); err == nil {
			return &receipt, receipt.SameEntry(&currentEntry)
		}
		return &receipt, true
	}
	// The source step is no longer current. Only committed proof may answer a
	// retry after the destination began entry; a pending receipt stays subject
	// to the normal stale-step rejection.
	return &receipt, receipt.IsCommitted()
}

// stepCompletionReceiptForClaim reloads the bounded receipt for the exact
// claimed signal. A concurrent replacement leaves the older claim unproven and
// the duplicate response says so instead of borrowing newer proof.
func (h *Handlers) stepCompletionReceiptForClaim(ctx context.Context, sessionID string, claimed models.PendingStepCompletionSignal) (*models.StepCompletionReceipt, bool) {
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false
	}
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	if !ok || (claimed.SignalID != "" && receipt.SignalID != claimed.SignalID) {
		return nil, false
	}
	pending, hasPending := models.LoadPendingStepSignal(session.Metadata)
	return &receipt, hasPending && pending.SignalID == receipt.SignalID
}

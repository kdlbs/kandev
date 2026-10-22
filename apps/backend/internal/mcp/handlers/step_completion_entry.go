package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *Handlers) stepCompletionLaunchStep(ctx context.Context, sessionID, fallback string) (string, *int64, error) {
	reader, ok := h.sessionRepo.(stepCompletionTurnReader)
	if !ok {
		return fallback, nil, nil
	}
	turns, err := reader.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return "", nil, err
	}
	if len(turns) == 0 {
		return fallback, nil, nil
	}
	latest := turns[len(turns)-1]
	if latest == nil {
		return "", nil, errors.New("latest turn is missing")
	}
	stepID := models.StringFromAny(latest.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
	if stepID == "" {
		return "", nil, errors.New("latest turn has no workflow-step stamp")
	}
	entry := int64(0)
	if raw, present := latest.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart]; present {
		var valid bool
		entry, valid = models.WorkflowEntryIDFromAny(raw)
		if !valid {
			return "", nil, errors.New("latest turn has an invalid workflow-entry stamp")
		}
	}
	return stepID, &entry, nil
}

type stepCompletionEntryClaimer interface {
	ClaimStepCompletionSignal(context.Context, string, string, models.PendingStepCompletionSignal) (models.PendingStepCompletionSignal, bool, error)
}

func (h *Handlers) claimStepCompletionSignal(ctx context.Context, taskID, sessionID, stepID string, signal models.PendingStepCompletionSignal) (models.PendingStepCompletionSignal, bool, error) {
	if claimer, ok := h.sessionRepo.(stepCompletionEntryClaimer); ok {
		return claimer.ClaimStepCompletionSignal(ctx, taskID, sessionID, signal)
	}
	if signal.WorkflowStepTransitionID != nil {
		return signal, false, errors.New("entry-aware completion claim is unavailable")
	}
	if claimer, ok := h.sessionRepo.(stepCompletionSignalClaimer); ok {
		stored, err := claimer.SetSessionMetadataKeyIfAbsentOrDifferentStepIfTaskAtStep(ctx, taskID, sessionID, models.SessionMetaKeyPendingStepCompletion, stepID, signal)
		return signal, stored, err
	}
	stored, err := h.sessionRepo.SetSessionMetadataKeyIfAbsentOrDifferentStep(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, stepID, signal)
	return signal, stored, err
}

func (h *Handlers) handleDuplicateStepComplete(
	ctx context.Context,
	msg *ws.Message,
	taskID, sessionID, stepID string,
	session *models.TaskSession,
	claimed models.PendingStepCompletionSignal,
) (*ws.Message, error) {
	// If the initial snapshot already contained this step's signal, this is a
	// retry after a possible publish failure and the event must be re-published
	// while the session waits. If the initial snapshot was empty, another
	// concurrent request won the claim and will publish the event itself.
	existing, ok := models.LoadPendingStepSignal(session.Metadata)
	if ok && existing.StepID == stepID && sameCompletionEntry(existing, claimed) && session.State == models.TaskSessionStateWaitingForInput {
		if errMsg, err := h.publishStepCompletionEvent(ctx, msg, taskID, sessionID, stepID, existing); errMsg != nil {
			return errMsg, err
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"accepted":   false,
		mcpReasonKey: "already_signaled",
	})
}

func sameCompletionEntry(a, b models.PendingStepCompletionSignal) bool {
	if a.WorkflowStepTransitionID == nil || b.WorkflowStepTransitionID == nil {
		return a.WorkflowStepTransitionID == nil && b.WorkflowStepTransitionID == nil
	}
	return *a.WorkflowStepTransitionID == *b.WorkflowStepTransitionID
}

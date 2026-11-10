package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

func (s *Service) matchingStepCompletionSignal(ctx context.Context, taskID, stepID string, session *models.TaskSession) (*models.PendingStepCompletionSignal, bool) {
	return matchingWorkflowSignal(ctx, s.repo, taskID, stepID, session)
}

func matchingWorkflowSignal(ctx context.Context, repo sessionExecutorStore, taskID, stepID string, session *models.TaskSession) (*models.PendingStepCompletionSignal, bool) {
	signal, has := models.LoadPendingStepSignal(session.Metadata)
	if !has || signal.StepID != stepID {
		return nil, false
	}
	if expected, guarded := models.WorkflowStepTransitionGuardFromContext(ctx, taskID, stepID); guarded {
		return &signal, signal.MatchesWorkflowEntry(stepID, expected)
	}
	reader, ok := repo.(workflowTaskStepEntryReader)
	if !ok {
		return &signal, signal.WorkflowStepTransitionID == nil
	}
	_, currentStep, entry, err := reader.GetTaskWorkflowStepEntry(ctx, taskID)
	if err != nil {
		return nil, false
	}
	return &signal, currentStep == stepID && signal.MatchesWorkflowEntry(stepID, entry)
}

func (s *Service) withStepCompletionEntryGuard(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep, cause turnCompletionCause) context.Context {
	if cause == turnCompletionCauseUserCancellation || !step.AutoAdvanceRequiresSignal {
		return ctx
	}
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	if !ok {
		return ctx
	}
	entry := int64(0)
	if signal.WorkflowStepTransitionID != nil {
		entry = *signal.WorkflowStepTransitionID
	}
	return models.WithWorkflowStepCompletionGuard(ctx, taskID, step.ID, entry)
}

// clearConsumedStepSignal only removes the observed bag value. A delayed source
// transition must leave a successor entry's completion signal untouched.
func (s *Service) clearConsumedStepSignal(ctx context.Context, session *models.TaskSession) {
	expected, present := session.Metadata[models.SessionMetaKeyPendingStepCompletion]
	if !present || expected == nil {
		return
	}
	setter, ok := s.repo.(interface {
		SetSessionMetadataKeyIfJSONValue(context.Context, string, string, interface{}, interface{}) (bool, error)
	})
	if !ok {
		return
	}
	removed, err := setter.SetSessionMetadataKeyIfJSONValue(ctx, session.ID, models.SessionMetaKeyPendingStepCompletion, expected, nil)
	if err != nil {
		s.logger.Warn("failed to clear consumed completion signal", zap.String("session_id", session.ID), zap.Error(err))
		return
	}
	if removed {
		delete(session.Metadata, models.SessionMetaKeyPendingStepCompletion)
	}
}

func guardedStepCompletionMove(ctx context.Context, repo sessionExecutorStore, task *models.Task, fromStepID string, target *wfmodels.WorkflowStep) (bool, error) {
	if target == nil {
		return false, fmt.Errorf("completion target step is unavailable")
	}
	if effect := workflowEffectFromContext(ctx); effect != nil {
		writer, ok := repo.(workflowMoveAdmissionCASEffectRepository)
		if !ok {
			return false, fmt.Errorf("entry-aware delivery transition is unavailable")
		}
		return writer.UpdateTaskWithWorkflowStepAdmissionIfAtStepAndEffect(ctx, task, fromStepID, target.ID, target.WIPLimit, effect)
	}
	writer, ok := repo.(workflowMoveAdmissionCASRepository)
	if !ok {
		return false, fmt.Errorf("entry-aware workflow transition is unavailable")
	}
	return writer.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, fromStepID, target.ID, target.WIPLimit)
}

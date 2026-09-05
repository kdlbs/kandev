package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type workflowTransitionIdentityReader interface {
	GetLatestTaskStepTransition(ctx context.Context, taskID string) (*models.TaskStepTransition, error)
}

// currentWorkflowTransitionID returns the authoritative transition identity
// only while taskID still names stepID. A repository that predates the ledger
// read keeps the existing queue behavior for compatibility with test adapters.
func (s *Service) currentWorkflowTransitionID(ctx context.Context, taskID, stepID string) (int64, bool, error) {
	reader, ok := s.repo.(workflowTransitionIdentityReader)
	if !ok || stepID == "" {
		return 0, false, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return 0, true, fmt.Errorf("load workflow control task: %w", err)
	}
	if task == nil || task.WorkflowStepID != stepID {
		return 0, true, nil
	}
	transition, err := reader.GetLatestTaskStepTransition(ctx, taskID)
	if err != nil {
		return 0, true, fmt.Errorf("load workflow transition identity: %w", err)
	}
	if transition == nil || transition.ToWorkflowStepID != stepID {
		return 0, true, nil
	}
	return transition.ID, true, nil
}

// workflowControlDeliveryIsCurrent revalidates a queued control prompt at the
// final dispatch boundary. A later task move wins over an older transition and
// therefore prevents its prompt from reaching the agent.
func (s *Service) workflowControlDeliveryIsCurrent(ctx context.Context, queuedMsg *messagequeue.QueuedMessage, task *models.Task) bool {
	if queuedMsg == nil || task == nil || !queuedMsg.IsWorkflowControl() {
		return false
	}
	transitionID, ok := workflowTransitionIDFromMetadata(queuedMsg.Metadata)
	if !ok {
		return false
	}
	stepID, _ := queuedMsg.Metadata["workflow_step_id"].(string)
	if stepID == "" || task.WorkflowStepID != stepID {
		return false
	}
	reader, ok := s.repo.(workflowTransitionIdentityReader)
	if !ok {
		return false
	}
	transition, err := reader.GetLatestTaskStepTransition(ctx, queuedMsg.TaskID)
	return err == nil && transition != nil && transition.ID == transitionID && transition.ToWorkflowStepID == stepID
}

func workflowTransitionIDFromMetadata(metadata map[string]interface{}) (int64, bool) {
	switch value := metadata[messagequeue.MetadataWorkflowTransitionID].(type) {
	case int64:
		return value, value > 0
	case int:
		return int64(value), value > 0
	case float64:
		return int64(value), value > 0 && value == float64(int64(value))
	default:
		return 0, false
	}
}

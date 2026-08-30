package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/routing"
)

type workflowRouteOperationRecorder interface {
	RecordWorkflowRouteOperation(context.Context, routing.Operation) error
}

type workflowRouteOperationReader interface {
	GetWorkflowRouteOperation(context.Context, string) (routing.Operation, bool, error)
}

type currentCoordinatorGrantReader interface {
	IsCurrentCoordinatorGrant(context.Context, string, string, string, string) (bool, error)
}

// RecordWorkflowRouteOperation persists a non-transitioning routing outcome.
// Committed/already-satisfied moves are recorded by the task repository in the
// same transaction as the task-step ledger row.
func (s *Service) RecordWorkflowRouteOperation(ctx context.Context, operation routing.Operation) error {
	recorder, ok := s.tasks.(workflowRouteOperationRecorder)
	if !ok {
		return fmt.Errorf("workflow route operation repository unavailable")
	}
	return recorder.RecordWorkflowRouteOperation(ctx, operation)
}

// GetWorkflowRouteOperation reads the stable outcome for an idempotency key.
func (s *Service) GetWorkflowRouteOperation(
	ctx context.Context,
	operationID string,
) (routing.Operation, bool, error) {
	reader, ok := s.tasks.(workflowRouteOperationReader)
	if !ok {
		return routing.Operation{}, false, fmt.Errorf("workflow route operation repository unavailable")
	}
	return reader.GetWorkflowRouteOperation(ctx, operationID)
}

// IsCurrentCoordinatorGrant fails closed unless the durable grant and the
// principal's live execution still designate this task as the workspace
// Coordinator. The storage contract is shared with exact pending-move
// cancellation; routing owns no second grant representation.
func (s *Service) IsCurrentCoordinatorGrant(
	ctx context.Context,
	workspaceID, taskID, sessionID, executionID string,
) (bool, error) {
	reader, ok := s.tasks.(currentCoordinatorGrantReader)
	if !ok {
		return false, fmt.Errorf("workspace coordinator grant repository unavailable")
	}
	return reader.IsCurrentCoordinatorGrant(ctx, workspaceID, taskID, sessionID, executionID)
}

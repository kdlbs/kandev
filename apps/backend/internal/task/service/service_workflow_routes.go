package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/routing"
)

type workflowRouteOperationRecorder interface {
	RecordWorkflowRouteOperation(context.Context, routing.Operation) error
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

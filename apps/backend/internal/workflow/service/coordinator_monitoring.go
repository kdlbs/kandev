package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/workflow/models"
)

// GetCoordinatorMonitoring returns the saved Coordinator monitoring rows for
// a workflow (checked steps and their custom prompts). Steps with no saved
// row are simply absent from the result.
func (s *Service) GetCoordinatorMonitoring(ctx context.Context, workflowID string) ([]models.CoordinatorStepMonitor, error) {
	entries, err := s.repo.GetCoordinatorMonitoring(ctx, workflowID)
	if err != nil {
		s.logger.Error("failed to get coordinator monitoring", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	return entries, nil
}

// SetCoordinatorMonitoring atomically replaces the Coordinator monitoring
// configuration for a workflow. Callers are expected to have already checked
// EnsureWorkflowMutable, mirroring the step-mutation pattern where the
// controller enforces the read-only guard before calling the service.
func (s *Service) SetCoordinatorMonitoring(ctx context.Context, workspaceID, workflowID string, entries []models.CoordinatorStepMonitor) ([]models.CoordinatorStepMonitor, error) {
	if err := s.repo.ReplaceCoordinatorMonitoring(ctx, workspaceID, workflowID, entries); err != nil {
		s.logger.Error("failed to set coordinator monitoring", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	s.logger.Info("saved coordinator monitoring", zap.String("workflow_id", workflowID), zap.Int("count", len(entries)))
	return s.GetCoordinatorMonitoring(ctx, workflowID)
}

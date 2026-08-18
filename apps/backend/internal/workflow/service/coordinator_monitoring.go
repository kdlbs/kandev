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
//
// The stored workspace is resolved from the workflow itself, not from
// workspaceID: that value reaches us from the request body, and a row whose
// workspace does not match its workflow is unusable provenance for the
// workspace-scoped readers this policy exists to serve.
func (s *Service) SetCoordinatorMonitoring(ctx context.Context, workspaceID, workflowID string, entries []models.CoordinatorStepMonitor) ([]models.CoordinatorStepMonitor, error) {
	workspaceID = s.resolveCoordinatorWorkspaceID(ctx, workflowID, workspaceID)
	if err := s.repo.ReplaceCoordinatorMonitoring(ctx, workspaceID, workflowID, entries); err != nil {
		s.logger.Error("failed to set coordinator monitoring", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	s.logger.Info("saved coordinator monitoring", zap.String("workflow_id", workflowID), zap.Int("count", len(entries)))
	return s.GetCoordinatorMonitoring(ctx, workflowID)
}

// resolveCoordinatorWorkspaceID returns the workflow's own workspace id so
// server-owned provenance never depends on what the caller claimed. It falls
// back to the supplied value when no workflow provider is wired (a bare test
// service) or the workflow cannot be read, matching the "fail open only when
// the caller genuinely has no way to validate" convention used elsewhere for
// late-wired dependencies.
func (s *Service) resolveCoordinatorWorkspaceID(ctx context.Context, workflowID, supplied string) string {
	if s.workflowProvider == nil {
		return supplied
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil || wf == nil {
		s.logger.Warn("coordinator monitoring: could not resolve workflow workspace, using supplied value",
			zap.String("workflow_id", workflowID), zap.Error(err))
		return supplied
	}
	if wf.WorkspaceID == "" {
		return supplied
	}
	return wf.WorkspaceID
}

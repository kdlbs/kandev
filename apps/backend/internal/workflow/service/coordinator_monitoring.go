package service

import (
	"context"
	"fmt"

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
	resolvedWorkspaceID, err := s.resolveCoordinatorWorkspaceID(ctx, workflowID, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceCoordinatorMonitoring(ctx, resolvedWorkspaceID, workflowID, entries); err != nil {
		s.logger.Error("failed to set coordinator monitoring", zap.String("workflow_id", workflowID), zap.Error(err))
		return nil, err
	}
	s.logger.Info("saved coordinator monitoring", zap.String("workflow_id", workflowID), zap.Int("count", len(entries)))
	return s.GetCoordinatorMonitoring(ctx, workflowID)
}

// resolveCoordinatorWorkspaceID returns the workflow's own workspace id so
// server-owned provenance never depends on what the caller claimed. A bare
// test service without a provider retains its explicit fallback; a wired
// provider must resolve successfully or the mutation fails before any write.
func (s *Service) resolveCoordinatorWorkspaceID(ctx context.Context, workflowID, supplied string) (string, error) {
	if s.workflowProvider == nil {
		return supplied, nil
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return "", fmt.Errorf("resolve coordinator monitoring workflow: %w", err)
	}
	if wf == nil {
		return "", fmt.Errorf("resolve coordinator monitoring workflow: workflow %q not found", workflowID)
	}
	if wf.WorkspaceID == "" {
		return "", fmt.Errorf("resolve coordinator monitoring workflow: workflow %q has no workspace", workflowID)
	}
	return wf.WorkspaceID, nil
}

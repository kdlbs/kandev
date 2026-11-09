package service

import (
	"context"
	"errors"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// ErrWorkflowReadOnly rejects UI mutations of workflows managed by workflow
// sync. The sync applier itself writes through the repo/provider directly and
// never hits the guarded controller/handler paths.
var ErrWorkflowReadOnly = errors.New("workflow is managed by GitHub sync and is read-only; edit its definition in the synced repository")

// ErrWorkflowWorkspaceReadOnly rejects UI mutations of workflows that live in
// the dedicated Improve Kandev workspace, whose workflows are read-only. The
// message is the canonical user-facing string shared with the handler layer
// (task/handlers `workspaceReadOnlyMsg`), so both layers surface identical
// wording for the same restriction.
var ErrWorkflowWorkspaceReadOnly = errors.New("this workspace is managed by Improve Kandev and is read-only")

// EnsureWorkflowMutable applies task-domain authorization and returns
// ErrWorkflowReadOnly or ErrWorkflowWorkspaceReadOnly when policy blocks a write.
// Standalone services use the provider fallback when no task-domain checker is wired.
func (s *Service) EnsureWorkflowMutable(ctx context.Context, workflowID string) error {
	if s.workflowMutationPolicyChecker != nil {
		synced, workspaceReadOnly, err := s.workflowMutationPolicyChecker(ctx, workflowID)
		if err != nil {
			return err
		}
		if synced {
			return ErrWorkflowReadOnly
		}
		if workspaceReadOnly {
			return ErrWorkflowWorkspaceReadOnly
		}
		return nil
	}
	if s.workflowProvider == nil {
		return nil
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil //nolint:nilerr // fail open: guard only blocks resolved workflows
	}
	if wf.Source == taskmodels.WorkflowSourceGitHub {
		return ErrWorkflowReadOnly
	}
	if s.workspaceProvider != nil && wf.WorkspaceID != "" {
		workspace, wsErr := s.workspaceProvider.GetWorkspace(ctx, wf.WorkspaceID)
		if wsErr == nil && workspace != nil && workspace.IsImproveKandev() {
			return ErrWorkflowWorkspaceReadOnly
		}
	}
	return nil
}

// EnsureWorkspaceMutable applies task-domain policy before an import writes
// workflows and steps into a workspace.
func (s *Service) EnsureWorkspaceMutable(ctx context.Context, workspaceID string) error {
	if s.workspaceMutationPolicyChecker != nil {
		readOnly, err := s.workspaceMutationPolicyChecker(ctx, workspaceID)
		if err != nil {
			return err
		}
		if readOnly {
			return ErrWorkflowWorkspaceReadOnly
		}
		return nil
	}
	if s.workspaceProvider == nil {
		return nil
	}
	workspace, err := s.workspaceProvider.GetWorkspace(ctx, workspaceID)
	if err == nil && workspace != nil && workspace.IsImproveKandev() {
		return ErrWorkflowWorkspaceReadOnly
	}
	return nil
}

// SetWorkflowMutationReadOnlyChecker wires mutation policy from the task
// domain, which owns workflow workspace permissions.
func (s *Service) SetWorkflowMutationReadOnlyChecker(checker func(context.Context, string) (bool, bool, error)) {
	s.workflowMutationPolicyChecker = checker
}

// SetWorkspaceMutationReadOnlyChecker wires workspace policy for imports.
func (s *Service) SetWorkspaceMutationReadOnlyChecker(checker func(context.Context, string) (bool, error)) {
	s.workspaceMutationPolicyChecker = checker
}

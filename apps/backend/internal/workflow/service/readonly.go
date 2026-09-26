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

// EnsureWorkflowMutable returns ErrWorkflowReadOnly when the workflow's
// definition is owned by workflow sync (source == "github"), and
// ErrWorkflowWorkspaceReadOnly when the workflow lives in the read-only
// Improve Kandev workspace. UI-facing mutation paths (step CRUD, template
// application) call this before writing.
//
// The guard fails open when the workflow or workspace can't be resolved: its
// only job is to block GitHub-managed workflows and improve-workspace
// workflows, the underlying mutation surfaces real not-found errors itself,
// and callers like the MCP handlers operate on workflows the wired provider
// may not track.
func (s *Service) EnsureWorkflowMutable(ctx context.Context, workflowID string) error {
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

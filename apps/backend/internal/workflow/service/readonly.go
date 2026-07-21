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

// EnsureWorkflowMutable returns ErrWorkflowReadOnly when the workflow's
// definition is owned by workflow sync (source == "github"). UI-facing
// mutation paths (step CRUD, template application) call this before writing.
//
// The guard fails open when the workflow can't be resolved: its only job is
// to block GitHub-managed workflows, the underlying mutation surfaces real
// not-found errors itself, and callers like the MCP handlers operate on
// workflows the wired provider may not track.
func (s *Service) EnsureWorkflowMutable(ctx context.Context, workflowID string) error {
	if s.workflowProvider == nil {
		return nil
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil //nolint:nilerr // fail open: guard only blocks resolved github-sourced workflows
	}
	if wf.Source == taskmodels.WorkflowSourceGitHub {
		return ErrWorkflowReadOnly
	}
	return nil
}

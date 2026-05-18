package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ChildTaskSpec captures the typed inputs for CreateChildTask.
//
// Mirrors engine.ChildTaskSpec / engine_adapters.ChildTaskCreateSpec
// without importing either package — the task service stays free of
// engine and office dependencies.
type ChildTaskSpec struct {
	Title          string
	Description    string
	WorkflowID     string
	StepID         string
	AgentProfileID string
}

// CreateChildTask creates a new task whose parent_id is parent.ID. The
// new task inherits the parent's workspace, repositories, and agent
// assignee unless overridden in spec. The workflow defaults to the
// parent's workflow when WorkflowID is blank; the workflow step defaults
// to the workflow's first runnable step when StepID is blank.
//
// Returns the new task id (non-empty) on success or a non-nil error.
//
// This is the canonical entry point for cross-strategy delegation —
// engine.CreateChildTaskCallback resolves to this method via the
// engine_adapters.TaskCreatorAdapter.
func (s *Service) CreateChildTask(
	ctx context.Context, parent *models.Task, spec ChildTaskSpec,
) (string, error) {
	if parent == nil {
		return "", fmt.Errorf("parent task is required")
	}
	if spec.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	workflowID := spec.WorkflowID
	if workflowID == "" {
		workflowID = parent.WorkflowID
	}
	assignee := spec.AgentProfileID
	if assignee == "" {
		assignee = parent.AssigneeAgentProfileID
	}

	req := &CreateTaskRequest{ //nolint:exhaustruct
		WorkspaceID:            parent.WorkspaceID,
		WorkflowID:             workflowID,
		WorkflowStepID:         spec.StepID,
		Title:                  spec.Title,
		Description:            spec.Description,
		ParentID:               parent.ID,
		AssigneeAgentProfileID: assignee,
		Origin:                 models.TaskOriginAgentCreated,
		ProjectID:              parent.ProjectID,
	}
	task, err := s.CreateTask(ctx, req)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

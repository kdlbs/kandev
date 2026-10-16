package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// SetCoordinatorStandingInstructionsReader installs the coordinator Standing
// Instructions content builder
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
// Wired in internal/backendapp/main.go with a closure over
// coordinator.Service.CoordinatorStandingInstructionsData and
// coordinator.StandingInstructions: this package cannot import
// internal/coordinator directly, since internal/coordinator already imports
// internal/orchestrator.
func (s *Service) SetCoordinatorStandingInstructionsReader(
	reader func(ctx context.Context, coordinatorID, workspaceName, workspaceID string) (string, error),
) {
	s.coordinatorStandingInstructions = reader
}

// coordinatorMetadataID reads the coordinator id a coordinator-origin task's
// conversation was opened for, mirroring
// coordinator.conversationTaskCoordinatorID's defensive metadata read.
func coordinatorMetadataID(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	if purpose, _ := task.Metadata[models.MetaKeyCoordinatorPurpose].(string); purpose == models.CoordinatorPurposeDream {
		return ""
	}
	id, _ := task.Metadata[models.MetaKeyCoordinatorID].(string)
	return id
}

// wrapCoordinatorStandingInstructions attaches the Standing Instructions
// system block ahead of a coordinator conversation's first prompt
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
// Falls back to the bare prompt whenever the block cannot be built (reader
// unwired, no coordinator id on the task, or a lookup failure): the
// coordinator's tool surface and attended-only restriction are enforced
// server-side regardless, so a missing informational block is a degraded
// prompt, not a security gap.
func (s *Service) wrapCoordinatorStandingInstructions(ctx context.Context, prompt string, dbTask *models.Task) string {
	if s.coordinatorStandingInstructions == nil {
		return prompt
	}
	coordinatorID := coordinatorMetadataID(dbTask)
	if coordinatorID == "" {
		return prompt
	}
	workspaceName := ""
	if s.repo != nil {
		if workspace, err := s.repo.GetWorkspace(ctx, dbTask.WorkspaceID); err == nil && workspace != nil {
			workspaceName = workspace.Name
		}
	}
	content, err := s.coordinatorStandingInstructions(ctx, coordinatorID, workspaceName, dbTask.WorkspaceID)
	if err != nil || content == "" {
		return prompt
	}
	return sysprompt.Wrap(content) + "\n\n" + prompt
}

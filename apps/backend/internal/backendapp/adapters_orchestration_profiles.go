package backendapp

import (
	"context"
	"fmt"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"maps"
)

func (a *taskCreatorAdapter) directWorkerProfile(ctx context.Context, spec shared.WorkspaceTaskSpec, metadata map[string]interface{}) (string, error) {
	if spec.AssigneeID == "" {
		return "", nil
	}
	if a.profiles == nil {
		return "", fmt.Errorf("profile store unavailable")
	}
	profile, err := a.profiles.GetAgentProfile(ctx, spec.AssigneeID)
	if err != nil || profile == nil || !profile.Enabled || profile.Role != "" || (profile.WorkspaceID != "" && profile.WorkspaceID != spec.WorkspaceID) {
		return "", fmt.Errorf("select an enabled execution profile available to this workspace")
	}
	metadata[models.MetaKeyAgentProfileID] = profile.ID
	metadata["orchestration_managed"] = true
	delete(metadata, "orchestration_execution_profile_id")
	delete(metadata, "orchestration_persona_id")
	return profile.ID, nil
}
func (a *taskCreatorAdapter) assignDirectWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if err := a.requireIdleWorkspaceTask(ctx, task.ID); err != nil {
		return err
	}
	metadata := maps.Clone(task.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	_, err := a.directWorkerProfile(ctx, shared.WorkspaceTaskSpec{WorkspaceID: task.WorkspaceID, AssigneeID: command.AssigneeID}, metadata)
	if err != nil {
		return err
	}
	metadata["orchestration_chief_id"] = command.ChiefID
	description, err := attachAssistantTaskContext(task, metadata, command.AssigneeID, command.DelegationReference)
	if err != nil {
		return err
	}
	if err = shared.CheckWorkspaceEffect(ctx); err != nil {
		return err
	}
	_, err = a.taskSvc.UpdateTask(ctx, task.ID, &taskservice.UpdateTaskRequest{Metadata: metadata, Description: description})
	return err
}
func (a *taskCreatorAdapter) startAssignedWorkspaceTask(ctx context.Context, task *models.Task) error {
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	id, _ := task.Metadata[models.MetaKeyAgentProfileID].(string)
	if id == "" {
		id = task.AssigneeAgentProfileID
	}
	if id == "" {
		return fmt.Errorf("assign an execution profile before starting the task")
	}
	if _, err := a.directWorkerProfile(ctx, shared.WorkspaceTaskSpec{WorkspaceID: task.WorkspaceID, AssigneeID: id}, map[string]interface{}{}); err != nil {
		return err
	}
	executor, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	_, err := a.orch.StartTask(ctx, task.ID, id, "", executor, "", task.Description, task.WorkflowStepID, false, false, nil)
	return err
}

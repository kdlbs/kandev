package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/office/routing"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// CreateWorkspaceTask keeps delivery on the workspace's existing workflow engine.
func (a *taskCreatorAdapter) CreateWorkspaceTask(ctx context.Context, spec shared.WorkspaceTaskSpec) (string, error) {
	workflowID, err := a.workspaceDeliveryWorkflow(ctx, spec.WorkspaceID, spec.WorkflowID)
	if err != nil {
		return "", err
	}
	if err := a.validateAssistantEntry(ctx, workflowID, spec); err != nil {
		return "", err
	}
	metadata := map[string]interface{}{"orchestration_chief_id": spec.ChiefID}
	if spec.MaintenanceCandidateID != "" {
		if _, err := a.ValidateMaintenanceScope(ctx, spec.WorkspaceID, shared.MaintenanceScope{RepositoryID: spec.RepositoryID, WorkflowID: workflowID, WorkflowStepID: spec.WorkflowStepID}); err != nil {
			return "", err
		}
		metadata["orchestration_maintenance_candidate"] = spec.MaintenanceCandidateID
	}
	if spec.ObjectiveID != "" {
		metadata["orchestration_objective_id"], metadata["orchestration_context_ref"] = spec.ObjectiveID, spec.ContextRef
		metadata["orchestration_acceptance_revision"], metadata["orchestration_source_comment_id"] = spec.AcceptanceRevision, spec.SourceCommentID
		metadata["orchestration_operation_id"] = spec.DispatchOperationID
		if spec.Packet != nil {
			metadata["orchestration_binding_id"] = spec.Packet.BindingID
		}
	}
	if spec.DirectProfile {
		metadata["orchestration_managed"] = true
	}
	profileID, err := a.workspaceWorkerProfile(ctx, spec, metadata)
	if err != nil {
		return "", err
	}
	if _, err := attachAssistantTaskContext(&models.Task{WorkspaceID: spec.WorkspaceID, Description: spec.Description}, metadata, profileID, spec.DelegationReference); err != nil {
		return "", err
	}
	req := &taskservice.CreateTaskRequest{
		LocalPreparationOnly: spec.MaintenanceCandidateID != "",
		PlanMode:             spec.ExecutionMode == "design",
		WorkspaceID:          spec.WorkspaceID, WorkflowID: workflowID, WorkflowStepID: spec.WorkflowStepID,
		Title: spec.Title, Description: assistantDelegationPrompt(spec.Description, spec.DelegationReference), ParentID: spec.ParentID,
		AssigneeAgentProfileID: profileID, Origin: models.TaskOriginAgentCreated, Metadata: metadata, ExternalID: spec.ExternalID,
	}
	if spec.RepositoryID != "" {
		repository, err := a.taskSvc.GetRepository(ctx, spec.RepositoryID)
		if err != nil || repository.WorkspaceID != spec.WorkspaceID {
			return "", fmt.Errorf("repository must belong to the workspace")
		}
		req.Repositories = []taskservice.TaskRepositoryInput{{RepositoryID: spec.RepositoryID, BaseBranch: repository.DefaultBranch}}
	}
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return "", err
	}
	result, err := a.taskSvc.CreateTask(ctx, req)
	if err != nil {
		return "", err
	}
	return result.Task.ID, nil
}

func (a *taskCreatorAdapter) workspaceDeliveryWorkflow(ctx context.Context, workspaceID, selected string) (string, error) {
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return "", err
	}
	var eligible []*models.Workflow
	for _, workflow := range workflows {
		if workflow.Hidden {
			continue
		}
		if selected == workflow.ID {
			return selected, nil
		}
		eligible = append(eligible, workflow)
	}
	if selected != "" {
		return "", fmt.Errorf("select a delivery workflow in this workspace")
	}
	if len(eligible) != 1 {
		return "", fmt.Errorf("select workflow_id explicitly: workspace has %d delivery workflows", len(eligible))
	}
	return eligible[0].ID, nil
}

func (a *taskCreatorAdapter) workspaceWorkerProfile(ctx context.Context, spec shared.WorkspaceTaskSpec, metadata map[string]interface{}) (string, error) {
	if spec.DirectProfile {
		return a.directWorkerProfile(ctx, spec, metadata)
	}
	return a.legacyWorkerProfile(ctx, spec, metadata)
}

func (a *taskCreatorAdapter) legacyWorkerProfile(ctx context.Context, spec shared.WorkspaceTaskSpec, metadata map[string]interface{}) (string, error) {
	if spec.AssigneeID == "" {
		return "", nil
	}
	if a.profiles == nil {
		return "", fmt.Errorf("profile store unavailable")
	}
	persona, err := a.profiles.GetAgentProfile(ctx, spec.AssigneeID)
	if err != nil || persona == nil || persona.WorkspaceID != spec.WorkspaceID || persona.Role == "" {
		return "", fmt.Errorf("worker must be a persona in this workspace")
	}
	override, err := routing.ReadAgentOverrides(persona.Settings)
	if err != nil {
		return "", err
	}
	profile, err := a.profiles.GetAgentProfile(ctx, override.ExecutionProfileID)
	if err != nil || profile == nil || !profile.Enabled || profile.Role != "" || (profile.WorkspaceID != "" && profile.WorkspaceID != spec.WorkspaceID) {
		return "", fmt.Errorf("worker requires an enabled execution profile")
	}
	metadata[models.MetaKeyAgentProfileID] = profile.ID
	metadata["orchestration_execution_profile_id"] = profile.ID
	metadata["orchestration_persona_id"] = persona.ID
	var executor struct {
		ProfileID string `json:"executor_profile_id"`
	}
	_ = json.Unmarshal([]byte(persona.ExecutorPreference), &executor)
	if executor.ProfileID != "" {
		metadata[models.MetaKeyExecutorProfileID] = executor.ProfileID
	}
	return profile.ID, nil
}

func (a *taskCreatorAdapter) WorkspaceCatalog(ctx context.Context, workspaceID string) (any, error) {
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	repositories, err := a.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{workspaceWorkflowsKey: workflows, workspaceRepositoriesKey: repositories}
	if a.workflow != nil {
		steps, err := a.workflow.ListStepsByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		result["workflow_steps"] = steps
	}
	return result, nil
}

func (a *taskCreatorAdapter) ManageWorkspaceTask(ctx context.Context, command shared.WorkspaceTaskCommand) error {
	task, err := a.taskSvc.GetTask(ctx, command.TaskID)
	if err != nil || task.WorkspaceID != command.WorkspaceID {
		return fmt.Errorf("task must belong to this workspace")
	}
	if task.IsFromOffice || task.IsEphemeral {
		return fmt.Errorf("select a Kanban delivery task")
	}
	if candidate, _ := task.Metadata["orchestration_maintenance_candidate"].(string); candidate != "" {
		return fmt.Errorf("maintenance tasks require the closed Assistant repair controls")
	}
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return err
	}
	return a.dispatchWorkspaceTask(ctx, task, command)
}

func (a *taskCreatorAdapter) dispatchWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	switch command.Action {
	case "message":
		return a.messageWorkspaceTask(ctx, task, command)
	case "adopt", "assign":
		return a.adoptWorkspaceTask(ctx, task, command)
	case "start":
		return a.startWorkspaceTask(ctx, task, command.DirectProfile)
	case "edit":
		return a.editWorkspaceTask(ctx, task, command)
	case "move":
		return a.moveWorkspaceTask(ctx, task, command)
	case "archive":
		return a.taskSvc.ArchiveTask(ctx, task.ID)
	case "delete":
		return a.taskSvc.DeleteTask(ctx, task.ID)
	case "stop":
		if a.orch == nil {
			return fmt.Errorf("orchestrator unavailable")
		}
		return a.orch.StopTask(ctx, task.ID, "workspace chief requested stop", false)
	default:
		return fmt.Errorf("unsupported task action")
	}
}

func (a *taskCreatorAdapter) startWorkspaceTask(ctx context.Context, task *models.Task, direct bool) error {
	if direct {
		return a.startAssignedWorkspaceTask(ctx, task)
	}
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	profile, _ := task.Metadata["orchestration_execution_profile_id"].(string)
	if profile == "" {
		return fmt.Errorf("assign a worker before starting delegated work")
	}
	executor, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	_, err := a.orch.StartTask(ctx, task.ID, profile, "", executor, "", task.Description, task.WorkflowStepID, false, false, nil)
	return err
}

func (a *taskCreatorAdapter) requireIdleWorkspaceTask(ctx context.Context, taskID string) error {
	if a.taskRepo == nil {
		return nil
	}
	session, err := a.taskRepo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return err
	}
	if session != nil {
		return fmt.Errorf("stop active execution before changing task ownership")
	}
	return nil
}

func (a *taskCreatorAdapter) adoptWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if command.DirectProfile && command.AssigneeID != "" {
		return a.assignDirectWorkspaceTask(ctx, task, command)
	}
	if command.Action == "adopt" && command.AssigneeID == "" && a.taskRepo != nil {
		if command.DirectProfile {
			if _, err := a.taskRepo.SetTaskMetadataKeyIfNotArchived(ctx, task.ID, "orchestration_managed", true); err != nil {
				return err
			}
		}
		return a.observeWorkspaceTask(ctx, task.ID, command.ChiefID)
	}

	if err := a.requireIdleWorkspaceTask(ctx, task.ID); err != nil {
		return err
	}
	metadata := maps.Clone(task.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if command.AssigneeID != "" {
		selected := maps.Clone(metadata)
		_, err := a.workspaceWorkerProfile(ctx, shared.WorkspaceTaskSpec{WorkspaceID: command.WorkspaceID, AssigneeID: command.AssigneeID}, selected)
		if err != nil {
			return err
		}
		previous, _ := metadata["orchestration_execution_profile_id"].(string)
		next, _ := selected["orchestration_execution_profile_id"].(string)
		if previous != "" && previous != next {
			return fmt.Errorf("reassignment cannot change the task's pinned account")
		}
		metadata = selected
	}
	metadata["orchestration_chief_id"] = command.ChiefID
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return err
	}
	_, err := a.taskSvc.UpdateTask(ctx, task.ID, &taskservice.UpdateTaskRequest{Metadata: metadata})
	return err
}

func (a *taskCreatorAdapter) observeWorkspaceTask(ctx context.Context, taskID, chiefID string) error {
	if err := shared.CheckWorkspaceEffect(ctx); err != nil {
		return err
	}
	changed, err := a.taskRepo.SetTaskMetadataKeyIfNotArchived(ctx, taskID, "orchestration_chief_id", chiefID)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("task is archived or unavailable")
	}
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	a.taskSvc.PublishTaskUpdated(ctx, task)
	return nil
}

package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type sandboxCreationAdmission struct {
	policies *sandbox.Service
	targets  sandboxTargetResolver
}

func (a sandboxCreationAdmission) admit(ctx context.Context, task *models.Task, req *taskservice.CreateTaskRequest) error {
	workspace, err := a.targets.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace == nil {
		return sandbox.ErrIneligible
	}
	hasBindings, err := a.policies.HasBindings(ctx, workspace.OrgID)
	if err != nil {
		return err
	}
	if !hasBindings {
		return nil
	}
	selection := sandbox.SelectionContext{OrgID: workspace.OrgID, WorkspaceID: workspace.ID, TaskID: task.ID, ParentTaskID: task.ParentID, WorkflowStepID: task.WorkflowStepID}
	caller, ok := scope.PrincipalFromContext(ctx)
	callerID := ""
	if ok {
		if caller.WorkspaceID != workspace.ID {
			return sandbox.ErrIneligible
		}
		callerID = caller.CallerTaskID
	}
	if err = a.policies.InheritTaskGuard(ctx, selection, task.ParentID, callerID); err != nil {
		return err
	}
	selection.ExecutorID, err = a.targets.tasks.ResolveTaskCreationExecutor(ctx, req)
	if err != nil {
		return err
	}
	if selection.ExecutorID == "" {
		return sandbox.ErrIneligible
	}
	selection.AgentProfileID, err = a.agentProfile(ctx, task, req, workspace)
	if err != nil {
		return err
	}
	_, err = a.policies.AdmitTask(ctx, selection, task.ParentID)
	return err
}

func (a sandboxCreationAdmission) agentProfile(ctx context.Context, task *models.Task, req *taskservice.CreateTaskRequest, workspace *models.Workspace) (string, error) {
	if id, ok := req.DeferredLaunch["agent_profile_id"].(string); ok && id != "" {
		return id, nil
	}
	if replacement, ok := task.WorkflowAgentOverrides.ReplacementFor(task.WorkflowID, task.WorkflowStepID); ok {
		return replacement, nil
	}
	if task.WorkflowStepID != "" {
		step, err := a.targets.workflows.GetStep(ctx, task.WorkflowStepID)
		if err != nil {
			return "", err
		}
		if step == nil || step.WorkflowID != task.WorkflowID {
			return "", sandbox.ErrIneligible
		}
		if step.AgentProfileID != "" {
			return step.AgentProfileID, nil
		}
	}
	if workspace.DefaultAgentProfileID != nil {
		return *workspace.DefaultAgentProfileID, nil
	}
	return "", nil
}

package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/sandbox"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type sandboxTargetResolver struct {
	tasks     *taskservice.Service
	profiles  settingsstore.Repository
	workflows *workflowservice.Service
}

func (r sandboxTargetResolver) resolve(ctx context.Context, scope, id string) (sandbox.TargetScope, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.Instance || identity.UserID == "" {
		return sandbox.TargetScope{}, sandbox.ErrNotFound
	}
	workspaceID, err := r.workspace(ctx, scope, id)
	if err != nil {
		return sandbox.TargetScope{}, sandbox.ErrNotFound
	}
	if workspaceID == "" {
		return sandbox.TargetScope{OrgID: identity.OrgID}, nil
	}
	workspace, err := r.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return sandbox.TargetScope{}, sandbox.ErrNotFound
	}
	if workspace.OrgID != identity.OrgID {
		return sandbox.TargetScope{}, sandbox.ErrNotFound
	}
	return sandbox.TargetScope{OrgID: workspace.OrgID, WorkspaceID: workspace.ID}, nil
}

func (r sandboxTargetResolver) workspace(ctx context.Context, scope, id string) (string, error) {
	switch scope {
	case sandbox.ScopeExecutor:
		executor, err := r.tasks.GetExecutor(ctx, id)
		if err != nil {
			return "", err
		}
		if executor == nil || executor.DeletedAt != nil || (executor.Type != models.ExecutorTypeLocal && executor.Type != models.ExecutorTypeWorktree) {
			return "", sandbox.ErrIneligible
		}
		return "", nil
	case sandbox.ScopeTask:
		task, err := r.tasks.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if task == nil {
			return "", sandbox.ErrNotFound
		}
		return task.WorkspaceID, nil
	case sandbox.ScopeWorkflowStep:
		return r.workflowWorkspace(ctx, id)
	case sandbox.ScopeAgent:
		return r.agentWorkspace(ctx, id)
	default:
		return "", sandbox.ErrNotFound
	}
}

func (r sandboxTargetResolver) agentWorkspace(ctx context.Context, id string) (string, error) {
	profile, err := r.profiles.GetAgentProfile(ctx, id)
	if err != nil {
		return "", err
	}
	if profile == nil {
		return "", sandbox.ErrNotFound
	}
	if profile.WorkspaceID != "" {
		return profile.WorkspaceID, nil
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return "", err
	}
	if agent == nil {
		return "", sandbox.ErrNotFound
	}
	if agent.WorkspaceID != nil {
		return *agent.WorkspaceID, nil
	}
	return "", nil
}

func (r sandboxTargetResolver) workflowWorkspace(ctx context.Context, id string) (string, error) {
	step, err := r.workflows.GetStep(ctx, id)
	if err != nil {
		return "", err
	}
	if step == nil {
		return "", sandbox.ErrNotFound
	}
	workflow, err := r.tasks.GetWorkflow(ctx, step.WorkflowID)
	if err != nil {
		return "", err
	}
	if workflow == nil {
		return "", sandbox.ErrNotFound
	}
	return workflow.WorkspaceID, nil
}

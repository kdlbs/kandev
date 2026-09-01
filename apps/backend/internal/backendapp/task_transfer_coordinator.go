package backendapp

import (
	"context"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

type taskTransferTaskReader interface {
	GetTask(context.Context, string) (*models.Task, error)
}

type taskTransferSessionReader interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

type taskTransferAgentReader interface {
	GetAgentInstance(context.Context, string) (*settingsmodels.AgentProfile, error)
}

type taskTransferCoordinatorAttestor struct {
	tasks    taskTransferTaskReader
	sessions taskTransferSessionReader
	agents   taskTransferAgentReader
}

func (a taskTransferCoordinatorAttestor) AttestTaskTransferCoordinator(
	ctx context.Context,
	principal mcpscope.Principal,
) (models.TaskTransferActor, bool) {
	if a.tasks == nil || a.sessions == nil || a.agents == nil {
		return models.TaskTransferActor{}, false
	}
	agent, ok := a.resolveTaskTransferCoordinator(ctx, principal)
	if !ok {
		return models.TaskTransferActor{}, false
	}
	return models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: agent.ID, SessionID: principal.CallerSessionID,
		CallerTaskID: principal.CallerTaskID,
	}, true
}

func (a taskTransferCoordinatorAttestor) AttestTaskTransferCoordinatorReplay(
	ctx context.Context,
	principal mcpscope.Principal,
	command models.TaskTransferCommand,
	persisted models.TaskTransferActor,
) (models.TaskTransferActor, bool) {
	if !a.taskTransferReplayRequestMatches(principal, command, persisted) {
		return models.TaskTransferActor{}, false
	}
	task, ok := a.resolveTaskTransferReplayPlacement(ctx, principal, command, persisted)
	if !ok || !a.taskTransferReplayProfilesMatch(ctx, task, command, persisted) {
		return models.TaskTransferActor{}, false
	}
	return models.TaskTransferActor{
		Kind: persisted.Kind, ID: persisted.ID, SessionID: persisted.SessionID, CallerTaskID: principal.CallerTaskID,
	}, true
}

func (a taskTransferCoordinatorAttestor) taskTransferReplayRequestMatches(
	principal mcpscope.Principal,
	command models.TaskTransferCommand,
	persisted models.TaskTransferActor,
) bool {
	return a.tasks != nil && a.sessions != nil && a.agents != nil && principal.CallerTaskID == command.TaskID &&
		persisted.Kind == models.TaskTransferActorCoordinator && persisted.SessionID == principal.CallerSessionID
}

func (a taskTransferCoordinatorAttestor) resolveTaskTransferReplayPlacement(
	ctx context.Context,
	principal mcpscope.Principal,
	command models.TaskTransferCommand,
	persisted models.TaskTransferActor,
) (*models.Task, bool) {
	task, err := a.tasks.GetTask(ctx, principal.CallerTaskID)
	if err != nil || task == nil || task.ID != command.TaskID || task.WorkspaceID != command.DestinationWorkspaceID ||
		task.WorkflowID != command.DestinationWorkflowID || task.WorkspaceID != principal.WorkspaceID ||
		task.AssigneeAgentProfileID == "" {
		return nil, false
	}
	session, err := a.sessions.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil || !taskTransferSessionMatches(session, principal) || session.AgentProfileID != persisted.ID {
		return nil, false
	}
	return task, true
}

func (a taskTransferCoordinatorAttestor) taskTransferReplayProfilesMatch(
	ctx context.Context,
	task *models.Task,
	command models.TaskTransferCommand,
	persisted models.TaskTransferActor,
) bool {
	sourceCEO, err := a.agents.GetAgentInstance(ctx, persisted.ID)
	if err != nil || !taskTransferActiveCEO(sourceCEO, command.ExpectedSourceWorkspaceID) {
		return false
	}
	destinationCEO, err := a.agents.GetAgentInstance(ctx, task.AssigneeAgentProfileID)
	return err == nil && taskTransferActiveCEO(destinationCEO, command.DestinationWorkspaceID)
}

func (a taskTransferCoordinatorAttestor) resolveTaskTransferCoordinator(
	ctx context.Context,
	principal mcpscope.Principal,
) (*settingsmodels.AgentProfile, bool) {
	task, err := a.tasks.GetTask(ctx, principal.CallerTaskID)
	if err != nil || task == nil || task.ID != principal.CallerTaskID || task.WorkspaceID != principal.WorkspaceID ||
		!task.IsOfficeOwnedAndAssigned() {
		return nil, false
	}
	session, err := a.sessions.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil || !taskTransferSessionMatches(session, principal) {
		return nil, false
	}
	agent, err := a.agents.GetAgentInstance(ctx, session.AgentProfileID)
	if err != nil || agent == nil || agent.ID != session.AgentProfileID ||
		agent.ID != task.AssigneeAgentProfileID || agent.WorkspaceID != principal.WorkspaceID ||
		!taskTransferActiveCEO(agent, principal.WorkspaceID) {
		return nil, false
	}
	return agent, true
}

func taskTransferActiveCEO(agent *settingsmodels.AgentProfile, workspaceID string) bool {
	return agent != nil && agent.WorkspaceID == workspaceID && agent.Role == settingsmodels.AgentRoleCEO &&
		agent.DeletedAt == nil && agent.Status != settingsmodels.AgentStatusStopped &&
		agent.Status != settingsmodels.AgentStatusPendingApproval
}

func taskTransferSessionMatches(session *models.TaskSession, principal mcpscope.Principal) bool {
	return session != nil && session.ID == principal.CallerSessionID &&
		session.TaskID == principal.CallerTaskID && session.AgentProfileID != "" &&
		session.State == models.TaskSessionStateRunning
}

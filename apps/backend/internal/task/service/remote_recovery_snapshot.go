package service

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type remoteRecoverySnapshotInputs struct {
	session     *models.TaskSession
	task        *models.Task
	workspace   *models.Workspace
	running     *models.ExecutorRunning
	environment *models.TaskEnvironment
}

type remoteRecoveryDeliverySnapshot struct {
	sourcePresent bool
	pending       bool
	recovery      models.AgentDeliveryRecovery
}

// GetRemoteRecoverySnapshot returns the current durable owner and eligibility
// projection for a session without materializing or repairing its environment.
func (s *Service) GetRemoteRecoverySnapshot(ctx context.Context, sessionID string) (*agentruntime.RemoteRecoverySnapshot, error) {
	if err := s.validateRemoteRecoverySnapshotRequest(sessionID); err != nil {
		return nil, err
	}
	inputs, err := s.loadRemoteRecoverySnapshotInputs(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	ownerArchived, ownerCleanupRunning, err := s.remoteRecoveryEnvironmentOwnerStatus(ctx, inputs.task, inputs.environment)
	if err != nil {
		return nil, err
	}
	cleanupRunning, err := s.resourceCleanups.HasActiveTaskResourceCleanupJob(ctx, inputs.task.ID)
	if err != nil {
		return nil, fmt.Errorf("read remote recovery cleanup state for task %q: %w", inputs.task.ID, err)
	}
	delivery := remoteRecoveryDeliveryStatus(inputs.session, inputs.running)
	return assembleRemoteRecoverySnapshot(inputs, ownerArchived, ownerCleanupRunning, cleanupRunning, delivery), nil
}

func (s *Service) validateRemoteRecoverySnapshotRequest(sessionID string) error {
	if sessionID == "" || s.sessions == nil || s.tasks == nil || s.workspaces == nil ||
		s.taskEnvironments == nil || s.resourceCleanups == nil || s.executors == nil {
		return fmt.Errorf("remote recovery snapshot dependencies are unavailable")
	}
	return nil
}

func (s *Service) loadRemoteRecoverySnapshotInputs(ctx context.Context, sessionID string) (*remoteRecoverySnapshotInputs, error) {
	session, err := s.loadRemoteRecoverySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	task, err := s.loadRemoteRecoveryTask(ctx, session)
	if err != nil {
		return nil, err
	}
	workspace, err := s.loadRemoteRecoveryWorkspace(ctx, task)
	if err != nil {
		return nil, err
	}
	running, err := s.loadRemoteRecoveryExecution(ctx, session, task)
	if err != nil {
		return nil, err
	}
	environment, err := s.loadRemoteRecoveryEnvironment(ctx, session.TaskEnvironmentID)
	if err != nil {
		return nil, err
	}
	return &remoteRecoverySnapshotInputs{
		session: session, task: task, workspace: workspace, running: running, environment: environment,
	}, nil
}

func (s *Service) loadRemoteRecoverySession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load remote recovery session %q: %w", sessionID, err)
	}
	if session == nil || session.ID != sessionID || session.TaskID == "" {
		return nil, fmt.Errorf("remote recovery session identity is incomplete")
	}
	return session, nil
}

func (s *Service) loadRemoteRecoveryTask(ctx context.Context, session *models.TaskSession) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("load remote recovery task %q: %w", session.TaskID, err)
	}
	if task == nil || task.ID != session.TaskID || task.WorkspaceID == "" {
		return nil, fmt.Errorf("remote recovery task identity is incomplete")
	}
	return task, nil
}

func (s *Service) loadRemoteRecoveryWorkspace(ctx context.Context, task *models.Task) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load remote recovery workspace %q: %w", task.WorkspaceID, err)
	}
	if workspace == nil || workspace.ID != task.WorkspaceID {
		return nil, fmt.Errorf("remote recovery workspace identity is incomplete")
	}
	return workspace, nil
}

func (s *Service) loadRemoteRecoveryExecution(
	ctx context.Context,
	session *models.TaskSession,
	task *models.Task,
) (*models.ExecutorRunning, error) {
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load remote recovery execution %q: %w", session.ID, err)
	}
	if running == nil || running.SessionID != session.ID || running.TaskID != task.ID ||
		running.AgentExecutionID == "" || running.AgentExecutionID != session.AgentExecutionID || running.Runtime == "" {
		return nil, fmt.Errorf("remote recovery execution identity is incomplete")
	}
	return running, nil
}

func (s *Service) loadRemoteRecoveryEnvironment(ctx context.Context, environmentID string) (*models.TaskEnvironment, error) {
	environment, err := s.taskEnvironments.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("load remote recovery task environment %q: %w", environmentID, err)
	}
	if environment == nil || environment.ID != environmentID || environment.TaskID == "" ||
		environment.OwnershipGeneration <= 0 || environment.ExecutorType == "" || environment.ExecutorID == "" {
		return nil, fmt.Errorf("remote recovery task environment identity is incomplete")
	}
	return environment, nil
}

func remoteRecoveryDeliveryStatus(session *models.TaskSession, running *models.ExecutorRunning) remoteRecoveryDeliverySnapshot {
	recovery, hasRecovery := models.LoadAgentDeliveryRecovery(session.Metadata)
	sourcePresent := hasRecovery && recovery.SessionID == session.ID &&
		recovery.AgentExecutionID == running.AgentExecutionID && recovery.SubmissionID != "" &&
		recovery.StreamID != "" && recovery.IncarnationID != "" && recovery.HarnessGeneration > 0 && recovery.Revision > 0
	pending := sourcePresent && (recovery.Phase == models.AgentDeliveryRecoveryUncertain ||
		recovery.Phase == models.AgentDeliveryRecoveryReconnecting)
	return remoteRecoveryDeliverySnapshot{sourcePresent: sourcePresent, pending: pending, recovery: recovery}
}

func assembleRemoteRecoverySnapshot(
	inputs *remoteRecoverySnapshotInputs,
	ownerArchived bool,
	ownerCleanupRunning bool,
	cleanupRunning bool,
	delivery remoteRecoveryDeliverySnapshot,
) *agentruntime.RemoteRecoverySnapshot {
	session, task, workspace := inputs.session, inputs.task, inputs.workspace
	environment := inputs.environment
	return &agentruntime.RemoteRecoverySnapshot{
		TaskID:                         task.ID,
		SessionID:                      session.ID,
		SessionState:                   session.State,
		AgentExecutionID:               inputs.running.AgentExecutionID,
		Runtime:                        string(inputs.running.Runtime),
		IsPassthrough:                  session.IsPassthrough,
		RouteState:                     session.RouteState,
		RouteGeneration:                session.RouteGeneration,
		TaskWorkspaceID:                task.WorkspaceID,
		TaskArchived:                   task.ArchivedAt != nil,
		TaskOrigin:                     task.Origin,
		WorkspaceOwnerID:               workspace.OwnerID,
		WorkspaceOrgID:                 workspace.OrgID,
		TaskEnvironmentID:              environment.ID,
		EnvironmentOwnerTaskID:         environment.TaskID,
		EnvironmentOwnershipGen:        environment.OwnershipGeneration,
		EnvironmentExecutorType:        environment.ExecutorType,
		EnvironmentExecutorID:          environment.ExecutorID,
		EnvironmentStatus:              string(environment.Status),
		EnvironmentOwnerArchived:       ownerArchived,
		EnvironmentOwnerCleanupRunning: ownerCleanupRunning,
		TaskResourceCleanupRunning:     cleanupRunning,
		RecoverySourcePresent:          delivery.sourcePresent,
		RecoverableDeliveryPending:     delivery.pending,
		RecoveryExecutionID:            delivery.recovery.AgentExecutionID,
		RecoverySubmissionID:           delivery.recovery.SubmissionID,
		RecoveryStreamID:               delivery.recovery.StreamID,
		RecoveryIncarnationID:          delivery.recovery.IncarnationID,
		RecoveryHarnessGeneration:      delivery.recovery.HarnessGeneration,
		RecoveryRevision:               delivery.recovery.Revision,
	}
}

func (s *Service) remoteRecoveryEnvironmentOwnerStatus(
	ctx context.Context,
	task *models.Task,
	environment *models.TaskEnvironment,
) (bool, bool, error) {
	if environment.TaskID == task.ID {
		return false, false, nil
	}
	ownerTask, err := s.tasks.GetTask(ctx, environment.TaskID)
	if err != nil {
		return false, false, fmt.Errorf("load remote recovery environment owner task %q: %w", environment.TaskID, err)
	}
	if ownerTask == nil || ownerTask.ID != environment.TaskID {
		return false, false, fmt.Errorf("remote recovery environment owner task identity is incomplete")
	}
	if ownerTask.WorkspaceID != task.WorkspaceID {
		return false, false, fmt.Errorf("remote recovery environment owner belongs to a different workspace")
	}
	cleanupRunning, err := s.resourceCleanups.HasActiveTaskResourceCleanupJob(ctx, ownerTask.ID)
	if err != nil {
		return false, false, fmt.Errorf("read remote recovery cleanup state for environment owner %q: %w", ownerTask.ID, err)
	}
	return ownerTask.ArchivedAt != nil, cleanupRunning, nil
}

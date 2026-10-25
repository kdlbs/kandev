package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const bootstrapAttemptRecoveryReason = "bootstrap_failed"

type bootstrapRecoveryContextKey struct{}

type bootstrapRecoveryAuthority struct {
	taskID, sessionID, incarnationID   string
	errorStamp, executionID, attemptID string
	routeGeneration                    int64
}

func (s *Service) prepareBootstrapRecoveryContext(ctx context.Context, session *models.TaskSession, stamp string) (context.Context, error) {
	if !hasCurrentBootstrapFailure(session) {
		return ctx, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, fmt.Errorf("load bootstrap executor inventory: %w", err)
	}
	if running != nil {
		return ctx, nil
	}
	failure, _ := models.LoadLastAgentError(session.Metadata)
	if stamp == "" || stamp != failure.Stamp() || failure.ExecutionID == "" || failure.AttemptID == "" {
		return nil, bootstrapRecoveryRequired(session.ID)
	}
	return context.WithValue(ctx, bootstrapRecoveryContextKey{}, bootstrapRecoveryAuthority{
		taskID: session.TaskID, sessionID: session.ID, incarnationID: session.QueueIncarnationID,
		errorStamp: stamp, executionID: failure.ExecutionID, attemptID: failure.AttemptID,
		routeGeneration: session.RouteGeneration,
	}), nil
}

// Recheck the stamped observation under the session lifecycle lock before
// reconstructing inventory. A newer execution never inherits this authority.
func (s *Service) admitBootstrapRecovery(ctx context.Context, session *models.TaskSession, running *models.ExecutorRunning) (bool, error) {
	authority, ok := ctx.Value(bootstrapRecoveryContextKey{}).(bootstrapRecoveryAuthority)
	if !ok {
		if running == nil && hasCurrentBootstrapFailure(session) {
			return false, bootstrapRecoveryRequired(session.ID)
		}
		return false, nil
	}
	if !authority.matches(session, running) {
		return false, bootstrapRecoveryRequired(session.ID)
	}
	if s.agentManager != nil && s.agentManager.IsAgentRunningForSession(ctx, session.ID) {
		return false, bootstrapRecoveryRequired(session.ID)
	}
	if running == nil {
		if err := s.validateBootstrapRecoverySnapshots(ctx, session); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (a bootstrapRecoveryAuthority) matches(session *models.TaskSession, running *models.ExecutorRunning) bool {
	failure, found := models.LoadLastAgentError(session.Metadata)
	return found && hasCurrentBootstrapFailure(session) && a.taskID == session.TaskID &&
		a.sessionID == session.ID && a.incarnationID == session.QueueIncarnationID &&
		a.routeGeneration == session.RouteGeneration && a.errorStamp == failure.Stamp() &&
		a.executionID == failure.ExecutionID && a.attemptID == failure.AttemptID &&
		(running == nil || running.AgentExecutionID == failure.ExecutionID)
}

func (s *Service) validateBootstrapRecoverySnapshots(ctx context.Context, session *models.TaskSession) error {
	if session.AgentProfileID == "" || session.AgentProfileSnapshot["id"] != session.AgentProfileID ||
		session.ExecutorID == "" || session.ExecutorSnapshot["id"] != session.ExecutorID ||
		(session.EnvironmentID != "" && session.EnvironmentSnapshot["id"] != session.EnvironmentID) ||
		(session.RepositoryID != "" && session.RepositorySnapshot["id"] != session.RepositoryID) {
		return bootstrapRecoveryRequired(session.ID)
	}
	selected, err := s.repo.GetExecutor(ctx, session.ExecutorID)
	if errors.Is(err, models.ErrExecutorNotFound) {
		return bootstrapRecoveryRequired(session.ID)
	}
	if err != nil {
		return fmt.Errorf("load bootstrap executor binding: %w", err)
	}
	if selected == nil || selected.DeletedAt != nil || session.ExecutorSnapshot["executor_type"] != string(selected.Type) {
		return bootstrapRecoveryRequired(session.ID)
	}
	return nil
}

func bootstrapRecoveryRequired(sessionID string) error {
	return &sessionRecoveryRequiredError{Block: &models.SessionRecoveryBlock{
		SessionID: sessionID, Reason: bootstrapAttemptRecoveryReason, State: models.RecoveryBlockOpen,
	}}
}

// Bootstrap failure remains an admission block after runtime inventory cleanup.
// A later execution owns its own outcome; historical failures cannot block it.
func (s *Service) checkBootstrapAttemptRecoveryBlock(ctx context.Context, sessionID string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load bootstrap recovery state: %w", err)
	}
	if !hasCurrentBootstrapFailure(session) {
		return nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return fmt.Errorf("load bootstrap executor inventory: %w", err)
	}
	if running != nil {
		return nil
	}
	return bootstrapRecoveryRequired(sessionID)
}

func hasCurrentBootstrapFailure(session *models.TaskSession) bool {
	if session == nil || (session.State != models.TaskSessionStateFailed &&
		session.State != models.TaskSessionStateWaitingForInput) {
		return false
	}
	failure, found := models.LoadLastAgentError(session.Metadata)
	if !found || failure.Phase != models.LaunchErrorPhaseBootstrap {
		return false
	}
	return session.AgentExecutionID == "" || failure.ExecutionID == "" ||
		session.AgentExecutionID == failure.ExecutionID
}

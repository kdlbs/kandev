package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type settledDeliverySessionStateGuardContextKey struct{}

func withSettledDeliverySessionStateGuardHeld(ctx context.Context) context.Context {
	return context.WithValue(ctx, settledDeliverySessionStateGuardContextKey{}, true)
}

func settledDeliverySessionStateGuardHeld(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	held, _ := ctx.Value(settledDeliverySessionStateGuardContextKey{}).(bool)
	return held
}

// convergeSettledDeliverySessionState repairs only a stale STARTING or RUNNING
// session row from one exact settled terminal and current idle runtime proof.
// It never creates a terminal, repeats a completion effect, or dispatches work.
func (s *Service) convergeSettledDeliverySessionState(
	ctx context.Context,
	sessionID string,
	terminal *models.AgentDeliverySessionStateRequest,
) error {
	settlements, terminal, err := s.settledDeliverySessionStateTerminal(ctx, sessionID, terminal)
	if err != nil || terminal == nil || terminal.SessionID != sessionID {
		return err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if isTaskSessionNotFound(err) {
			return nil
		}
		return err
	}
	if session == nil || !deliverySessionStateCandidate(session, terminal) {
		return nil
	}
	if !s.deliverySessionStateRuntimeIdle(ctx, session) {
		return nil
	}
	result, err := s.correctSettledDeliverySessionState(ctx, sessionID, terminal, settlements)
	if err != nil || result == nil || result.Outcome != models.AgentDeliverySessionStateChanged {
		return err
	}
	s.publishSettledDeliverySessionState(ctx, sessionID, result)
	return nil
}

func (s *Service) settledDeliverySessionStateTerminal(
	ctx context.Context,
	sessionID string,
	terminal *models.AgentDeliverySessionStateRequest,
) (repository.AgentDeliverySettlementRepository, *models.AgentDeliverySessionStateRequest, error) {
	settlements, ok := s.repo.(repository.AgentDeliverySettlementRepository)
	if !ok || sessionID == "" {
		return nil, nil, nil
	}
	if terminal != nil {
		return settlements, terminal, nil
	}
	loaded, found, err := settlements.LoadCurrentSettledAgentDeliveryTerminal(ctx, sessionID)
	if err != nil || !found {
		return settlements, nil, err
	}
	return settlements, loaded, nil
}

func (s *Service) correctSettledDeliverySessionState(
	ctx context.Context,
	sessionID string,
	terminal *models.AgentDeliverySessionStateRequest,
	settlements repository.AgentDeliverySettlementRepository,
) (*models.AgentDeliverySessionStateResult, error) {
	if s.deliverySessionStateTestHook != nil {
		s.deliverySessionStateTestHook()
	}
	if !settledDeliverySessionStateGuardHeld(ctx) {
		guard, err := s.lockCancelInFlightGuardWithContext(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		defer guard.release()
	}
	if s.isCancelInFlight(sessionID) || s.isSessionResetInProgress(sessionID) ||
		s.isRouteActionInFlight(sessionID) {
		return nil, nil
	}
	current, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if isTaskSessionNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if current == nil || !deliverySessionStateCandidate(current, terminal) {
		return nil, nil
	}
	if !s.deliverySessionStateRuntimeIdle(ctx, current) {
		return nil, nil
	}
	return settlements.CorrectSettledAgentDeliverySessionState(ctx, *terminal)
}

func deliverySessionStateCandidate(
	session *models.TaskSession,
	terminal *models.AgentDeliverySessionStateRequest,
) bool {
	if session.State != models.TaskSessionStateStarting && session.State != models.TaskSessionStateRunning {
		return false
	}
	if terminal.ExecutionID != "" && session.AgentExecutionID != "" &&
		terminal.ExecutionID != session.AgentExecutionID {
		return false
	}
	return true
}

// deliverySessionStateRuntimeIdle requires current lifecycle ownership plus an
// idle foreground boundary: the same live execution is ready for a prompt, no
// turn is open, and no first-prompt dispatch is pending.
func (s *Service) deliverySessionStateRuntimeIdle(ctx context.Context, session *models.TaskSession) bool {
	if s.agentManager == nil || session.AgentExecutionID == "" || s.isInitialTaskBriefDispatchPending(session.ID) {
		return false
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if err != nil || executionID == "" || executionID != session.AgentExecutionID {
		return false
	}
	if !s.agentManager.IsAgentReadyForPrompt(ctx, session.ID) {
		return false
	}
	if turnID, err := s.peekActiveTurnID(ctx, session.ID); err != nil || turnID != "" {
		return false
	}
	return true
}

func (s *Service) publishSettledDeliverySessionState(
	ctx context.Context,
	sessionID string,
	result *models.AgentDeliverySessionStateResult,
) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		s.logger.Warn("failed to reload session after settled delivery state correction",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	updatedAt := result.UpdatedAt
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID,
		result.OldState, result.NewState, session.ErrorMessage, &updatedAt, session)
	s.republishTaskActivityOnSettle(ctx, session.TaskID, result.OldState, result.NewState)
	s.onSessionStateChangedForParkedProjection(ctx, session.TaskID, session.ID,
		result.OldState, result.NewState)
}

func deliverySessionStateRequestFromEffect(effect *models.AgentDeliveryEffect) *models.AgentDeliverySessionStateRequest {
	if effect == nil {
		return nil
	}
	return &models.AgentDeliverySessionStateRequest{
		SessionID:         effect.SessionID,
		IncarnationID:     effect.IncarnationID,
		HarnessGeneration: effect.HarnessGeneration,
		StreamID:          effect.StreamID,
		Sequence:          effect.Sequence,
		SubmissionID:      effect.SubmissionID,
		TurnID:            effect.TurnID,
		Outcome:           effect.Outcome,
	}
}

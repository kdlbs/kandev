package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) handleAgentctlDeliveryRecovery(ctx context.Context, payload agentruntime.AgentctlEventPayload) {
	if !validAgentctlDeliveryRecoveryPayload(payload) || s.repo == nil {
		s.logger.Debug("ignoring incomplete agent delivery recovery event",
			zap.String("session_id", payload.SessionID),
			zap.String("submission_id", payload.DeliverySubmissionID),
			zap.String("phase", payload.DeliveryRecoveryPhase),
			zap.Uint64("prompt_generation", payload.PromptGeneration))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	owner, ok := s.loadAgentDeliveryRecoveryEventOwner(ctx, payload)
	if !ok {
		return
	}
	if terminalDeliverySubmission(owner.submission.State) {
		s.logger.Debug("delivery recovery event refers to an already-settled submission",
			zap.String("session_id", payload.SessionID),
			zap.String("submission_id", payload.DeliverySubmissionID),
			zap.String("submission_state", string(owner.submission.State)))
		_ = s.clearAgentDeliveryRecoveryNotice(ctx, payload.SessionID, payload.DeliveryIncarnationID,
			payload.DeliverySubmissionID, int64(payload.DeliveryHarnessGeneration))
		return
	}
	if !s.agentDeliveryRecoveryCursorMatches(ctx, payload, owner.submissions) {
		return
	}
	recovery := agentDeliveryRecoveryFromEvent(payload)
	if recovery.Phase == models.AgentDeliveryRecoveryRecovered {
		s.resolveReattachedDeliveryRecovery(ctx, payload, owner.recoveryStore, recovery)
		return
	}
	s.persistAgentDeliveryRecoveryNotice(ctx, payload, owner, recovery)
}

type agentDeliveryRecoveryEventOwner struct {
	submissions   repository.AgentDeliveryRepository
	recoveryStore repository.AgentDeliveryRecoveryRepository
	submission    *models.AgentDeliverySubmission
}

func (s *Service) loadAgentDeliveryRecoveryEventOwner(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
) (*agentDeliveryRecoveryEventOwner, bool) {
	if _, ok := s.loadAgentDeliveryRecoverySession(ctx, payload); !ok {
		return nil, false
	}
	submissions, submission, ok := s.loadAgentDeliveryRecoverySubmission(ctx, payload)
	if !ok {
		return nil, false
	}
	recoveryStore, ok := s.repo.(repository.AgentDeliveryRecoveryRepository)
	if !ok {
		return nil, false
	}
	return &agentDeliveryRecoveryEventOwner{
		submissions: submissions, recoveryStore: recoveryStore, submission: submission,
	}, true
}

func (s *Service) loadAgentDeliveryRecoverySession(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
) (*models.TaskSession, bool) {
	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil || session == nil || session.TaskID != payload.TaskID ||
		session.AgentExecutionID != payload.AgentExecutionID || session.QueueIncarnationID != payload.DeliveryIncarnationID {
		s.logger.Debug("ignoring agent delivery recovery for a replaced session owner",
			zap.String("session_id", payload.SessionID),
			zap.String("submission_id", payload.DeliverySubmissionID),
			zap.Error(err))
		return nil, false
	}
	continuity, ok := s.repo.(sessionContinuityStore)
	if !ok {
		return nil, false
	}
	current, err := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, payload.DeliveryIncarnationID)
	if err != nil || current == nil || current.Generation != int64(payload.DeliveryHarnessGeneration) {
		s.logger.Debug("ignoring agent delivery recovery for a replaced harness generation",
			zap.String("session_id", payload.SessionID),
			zap.String("submission_id", payload.DeliverySubmissionID),
			zap.Error(err))
		return nil, false
	}
	return session, true
}

func (s *Service) loadAgentDeliveryRecoverySubmission(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
) (repository.AgentDeliveryRepository, *models.AgentDeliverySubmission, bool) {
	submissions, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok {
		return nil, nil, false
	}
	submission, err := submissions.GetAgentDeliverySubmission(ctx, payload.DeliverySubmissionID)
	if err != nil || submission == nil || submission.SessionID != payload.SessionID ||
		submission.IncarnationID != payload.DeliveryIncarnationID ||
		submission.HarnessGeneration != int64(payload.DeliveryHarnessGeneration) {
		s.logger.Debug("ignoring agent delivery recovery without matching canonical submission",
			zap.String("session_id", payload.SessionID),
			zap.String("submission_id", payload.DeliverySubmissionID),
			zap.Error(err))
		return nil, nil, false
	}
	return submissions, submission, true
}

func (s *Service) agentDeliveryRecoveryCursorMatches(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
	submissions repository.AgentDeliveryRepository,
) bool {
	cursor, err := submissions.GetAgentDeliveryCursor(ctx, payload.DeliveryStreamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		s.logger.Warn("failed to validate durable delivery recovery stream",
			zap.String("session_id", payload.SessionID), zap.Error(err))
		return false
	}
	return cursor == nil || cursor.IncarnationID == payload.DeliveryIncarnationID &&
		cursor.HarnessGeneration == int64(payload.DeliveryHarnessGeneration) && cursor.StreamID == payload.DeliveryStreamID
}

func agentDeliveryRecoveryFromEvent(payload agentruntime.AgentctlEventPayload) *models.AgentDeliveryRecovery {
	return &models.AgentDeliveryRecovery{
		Phase: payload.DeliveryRecoveryPhase, SessionID: payload.SessionID,
		AgentExecutionID: payload.AgentExecutionID, SubmissionID: payload.DeliverySubmissionID,
		StreamID: payload.DeliveryStreamID, IncarnationID: payload.DeliveryIncarnationID,
		HarnessGeneration: int64(payload.DeliveryHarnessGeneration), PromptGeneration: payload.PromptGeneration,
		Message: payload.ErrorMessage,
	}
}

func (s *Service) resolveReattachedDeliveryRecovery(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
	recoveryStore repository.AgentDeliveryRecoveryRepository,
	recovery *models.AgentDeliveryRecovery,
) {
	recovered, err := recoveryStore.ResolveAgentDeliveryRecovery(ctx, recovery, "durable_delivery_reconnected")
	if err != nil {
		s.logger.Warn("failed to resolve reattached durable delivery recovery",
			zap.String("session_id", payload.SessionID), zap.String("submission_id", payload.DeliverySubmissionID), zap.Error(err))
		return
	}
	if recovered {
		s.publishAgentDeliveryRecoveryState(ctx, payload.SessionID)
	}
}

func (s *Service) persistAgentDeliveryRecoveryNotice(
	ctx context.Context,
	payload agentruntime.AgentctlEventPayload,
	owner *agentDeliveryRecoveryEventOwner,
	recovery *models.AgentDeliveryRecovery,
) {
	block := &models.SessionRecoveryBlock{
		SessionID: payload.SessionID, IncarnationID: payload.DeliveryIncarnationID,
		ExpectedGeneration: int64(payload.DeliveryHarnessGeneration), Reason: "unknown_prompt_outcome",
		State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
		DeliverySubmissionID: payload.DeliverySubmissionID, DeliveryStreamID: payload.DeliveryStreamID,
	}
	stored, err := owner.recoveryStore.UpsertAgentDeliveryRecovery(ctx, recovery, block)
	if err != nil {
		s.logger.Warn("failed to persist durable delivery recovery state",
			zap.String("session_id", payload.SessionID), zap.String("submission_id", payload.DeliverySubmissionID), zap.Error(err))
		return
	}
	if stored {
		agentruntime.RecordRecoveryRequired("agent_delivery", "unknown_prompt_outcome")
		s.publishAgentDeliveryRecoveryState(ctx, payload.SessionID)
	}
	// A terminal may have committed between the initial read and notice write.
	// Clearing remains submission-specific, so a racing successor is preserved.
	submission, err := owner.submissions.GetAgentDeliverySubmission(ctx, payload.DeliverySubmissionID)
	if err == nil && terminalDeliverySubmission(submission.State) {
		_ = s.clearAgentDeliveryRecoveryNotice(ctx, payload.SessionID, payload.DeliveryIncarnationID,
			payload.DeliverySubmissionID, int64(payload.DeliveryHarnessGeneration))
	}
}

func validAgentctlDeliveryRecoveryPayload(payload agentruntime.AgentctlEventPayload) bool {
	if payload.DeliveryRecoveryPhase != models.AgentDeliveryRecoveryReconnecting &&
		payload.DeliveryRecoveryPhase != models.AgentDeliveryRecoveryUncertain &&
		payload.DeliveryRecoveryPhase != models.AgentDeliveryRecoveryRecovered {
		return false
	}
	return payload.TaskID != "" && payload.SessionID != "" && payload.AgentExecutionID != "" &&
		payload.DeliverySubmissionID != "" && payload.DeliveryStreamID != "" &&
		payload.DeliveryIncarnationID != "" && payload.DeliveryHarnessGeneration > 0 && payload.PromptGeneration > 0
}

func terminalDeliverySubmission(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionCompleted || state == models.DeliverySubmissionFailed ||
		state == models.DeliverySubmissionCancelled
}

func (s *Service) clearAgentDeliveryRecoveryNotice(
	ctx context.Context,
	sessionID, incarnationID, submissionID string,
	harnessGeneration int64,
) error {
	recoveryStore, ok := s.repo.(repository.AgentDeliveryRecoveryRepository)
	if !ok {
		return nil
	}
	cleared, err := recoveryStore.ClearAgentDeliveryRecovery(
		ctx, sessionID, incarnationID, submissionID, harnessGeneration,
	)
	if err != nil {
		s.logger.Warn("failed to clear settled durable delivery recovery notice",
			zap.String("session_id", sessionID), zap.String("submission_id", submissionID), zap.Error(err))
		return err
	}
	if cleared {
		s.publishAgentDeliveryRecoveryState(ctx, sessionID)
	}
	return nil
}

func (s *Service) publishAgentDeliveryRecoveryState(ctx context.Context, sessionID string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		if err != nil {
			s.logger.Warn("failed to reload session after durable delivery recovery change",
				zap.String("session_id", sessionID), zap.Error(err))
		}
		return
	}
	updatedAt := session.UpdatedAt
	s.publishTaskSessionStateChanged(ctx, session.TaskID, session.ID,
		session.State, session.State, session.ErrorMessage, &updatedAt, session)
}

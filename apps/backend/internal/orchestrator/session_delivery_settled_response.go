package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) retryConcurrentlySettledDelivery(ctx context.Context, session *models.TaskSession, expected models.AgentDeliveryRecovery) (*SessionDeliveryRecoveryResponse, error) {
	current, err := s.loadDeliveryRetrySession(ctx, session.TaskID, session.ID)
	if err != nil {
		return nil, err
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(current.Metadata)
	if !ok || !sameDeliveryRecoveryBinding(recovery, expected) {
		return nil, nil
	}
	generation, err := s.currentDeliveryGeneration(ctx, current, recovery.IncarnationID)
	if err != nil {
		return nil, err
	}
	if !recoverySessionMatches(current, recovery, generation) {
		return nil, nil
	}
	submissions := s.repo.(repository.AgentDeliveryRepository)
	submission, err := submissions.GetAgentDeliverySubmission(ctx, expected.SubmissionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if submission == nil || !terminalDeliverySubmission(submission.State) || !canonicalRecoverySubmissionMatches(current, recovery, submission, generation) {
		return nil, nil
	}
	blocks := s.repo.(sessionRecoveryBlockStore)
	block, err := blocks.GetOpenSessionRecoveryBlock(ctx, current.ID, recovery.IncarnationID, generation)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	response, err := s.retryKnownSessionDelivery(ctx, current, block, recovery, generation)
	if err != nil {
		return nil, err
	}
	return s.refreshSettledRetryRevision(ctx, current, expected, generation, response)
}

func (s *Service) refreshSettledRetryRevision(ctx context.Context, current *models.TaskSession, expected models.AgentDeliveryRecovery, generation int64, response *SessionDeliveryRecoveryResponse) (*SessionDeliveryRecoveryResponse, error) {
	if response.Outcome != SessionDeliveryRecoverySettled {
		return response, nil
	}
	latest, err := s.loadDeliveryRetrySession(ctx, current.TaskID, current.ID)
	if err != nil {
		return nil, err
	}
	refreshed, ok := models.LoadAgentDeliveryRecovery(latest.Metadata)
	if !ok || !sameDeliveryRecoveryBinding(refreshed, expected) || !recoverySessionMatches(latest, refreshed, generation) {
		return nil, nil
	}
	response.RecoveryRevision = refreshed.Revision
	return response, nil
}

func sameDeliveryRecoveryBinding(current, expected models.AgentDeliveryRecovery) bool {
	return current.SessionID == expected.SessionID && current.AgentExecutionID == expected.AgentExecutionID &&
		current.IncarnationID == expected.IncarnationID && current.HarnessGeneration == expected.HarnessGeneration &&
		current.StreamID == expected.StreamID && current.SubmissionID == expected.SubmissionID &&
		current.PromptGeneration == expected.PromptGeneration
}

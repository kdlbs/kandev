package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) reconstructionMessageAuthority(ctx context.Context, session *models.TaskSession, candidate journal.SubmissionSummary, messageID string) (*models.AgentDeliverySubmission, string, error) {
	store, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok {
		return nil, recoveryStateUnavailableReason, nil
	}
	canonical, err := store.GetAgentDeliverySubmission(ctx, candidate.ID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, repository.ErrAgentDeliverySubmissionNotFound) {
		reason, messageErr := s.validateReconstructionMessage(ctx, session, messageID)
		return nil, reason, messageErr
	}
	if err != nil {
		return nil, "", err
	}
	if canonical == nil || canonical.ID != candidate.ID || canonical.SessionID != session.ID || canonical.IncarnationID != candidate.IncarnationID || canonical.HarnessGeneration != int64(candidate.HarnessGeneration) || canonical.DispatchAttemptID == "" || canonical.PayloadHash != candidate.Hash {
		return nil, deliveryIdentityMismatchReason, nil
	}
	return canonical, "", nil
}

package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) reconstructionMessageAuthority(ctx context.Context, session *models.TaskSession, candidate journal.SubmissionSummary, messageID string) (bool, string, error) {
	store, ok := s.repo.(repository.AgentDeliveryRepository)
	if !ok {
		return false, recoveryStateUnavailableReason, nil
	}
	canonical, err := store.GetAgentDeliverySubmission(ctx, candidate.ID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, repository.ErrAgentDeliverySubmissionNotFound) {
		reason, messageErr := s.validateReconstructionMessage(ctx, session, messageID)
		return false, reason, messageErr
	}
	if err != nil {
		return false, "", err
	}
	if canonical == nil || canonical.SessionID != session.ID || canonical.IncarnationID != candidate.IncarnationID || canonical.HarnessGeneration != int64(candidate.HarnessGeneration) || canonical.DispatchAttemptID != messageID || canonical.PayloadHash != candidate.Hash {
		return false, deliveryIdentityMismatchReason, nil
	}
	return true, "", nil
}

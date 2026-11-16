package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type unsentDeliveryStore interface {
	FailUnsentAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission, time.Time) (bool, error)
}

func (s *Service) settlePromptDispatchError(ctx context.Context, sessionID string, delivery *agentDeliverySubmissionRuntime, accepted bool, cause error) error {
	if delivery == nil {
		return cause
	}
	var notSent *agentruntime.RequestNotSentError
	if !accepted && errors.As(cause, &notSent) {
		if err := delivery.failUnsent(ctx); err == nil {
			return cause
		} else {
			cause = errors.Join(cause, err)
		}
	}
	return errors.Join(cause, delivery.markInterrupted(ctx, "prompt_dispatch_failed"), s.deliveryRecoveryError(ctx, sessionID, unknownPromptOutcome, delivery))
}

func (r *agentDeliverySubmissionRuntime) failUnsent(ctx context.Context) error {
	writer, ok := r.store.(unsentDeliveryStore)
	if !ok {
		return fmt.Errorf("unsent delivery settlement is unavailable")
	}
	row := r.expected
	if row == nil || row.SessionID != r.sessionID {
		return fmt.Errorf("unsent delivery identity is unavailable")
	}
	changed, err := writer.FailUnsentAgentDeliverySubmission(ctx, row, time.Now().UTC())
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("unsent delivery reservation changed")
	}
	r.completed = true
	return nil
}

// Unknown outcomes cannot be displaced by a later admission's recovery block.
func (s *Service) checkUnknownPromptAdmission(ctx context.Context, sessionID string) error {
	if list, ok := s.repo.(repository.SessionRecoveryBlockListRepository); ok {
		incarnation, generation, err := s.recoverySessionIdentity(ctx, sessionID)
		if err != nil {
			return err
		}
		blocks, err := list.ListOpenSessionRecoveryBlocks(ctx, sessionID, incarnation, generation)
		if err != nil {
			return err
		}
		for _, block := range blocks {
			if block.Reason == unknownPromptOutcome {
				return &sessionRecoveryRequiredError{Block: block}
			}
		}
		return nil
	}
	block, err := s.GetOpenSessionRecoveryBlock(ctx, sessionID)
	if err != nil {
		return err
	}
	if block != nil && block.Reason == unknownPromptOutcome {
		return &sessionRecoveryRequiredError{Block: block}
	}
	return nil
}

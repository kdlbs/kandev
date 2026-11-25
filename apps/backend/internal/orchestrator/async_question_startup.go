package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type asyncQuestionStartupStore interface {
	SettleAsyncQuestionWaitsForStartup(context.Context, bool) error
	ListPendingAsyncQuestionDeliveryIdentities(context.Context) ([]models.AsyncQuestionDeliveryIdentity, error)
	MarkAsyncQuestionDeliveryRetired(context.Context, models.AsyncQuestionDeliveryIdentity, *time.Time) error
	MarkAsyncQuestionDeliveryAdmitted(context.Context, string, time.Time) error
	IsAsyncQuestionDeliveryRetired(context.Context, string, string) (bool, error)
}

func (s *Service) settleAsyncQuestionStateBeforeQueueRecovery(ctx context.Context) error {
	store, ok := s.repo.(asyncQuestionStartupStore)
	if !ok {
		return nil
	}
	if err := store.SettleAsyncQuestionWaitsForStartup(ctx, s.asyncQuestionsEnabled); err != nil {
		return fmt.Errorf("settle async question waits: %w", err)
	}
	if !s.asyncQuestionsEnabled {
		if err := s.retirePendingAsyncQuestionDeliveries(ctx, store); err != nil {
			return err
		}
		if err := s.retireQueuedAsyncQuestionCandidates(ctx, store); err != nil {
			return err
		}
	}
	return s.finishAsyncQuestionDeliveryRetirements(ctx, store)
}

func (s *Service) retirePendingAsyncQuestionDeliveries(
	ctx context.Context,
	store asyncQuestionStartupStore,
) error {
	identities, err := store.ListPendingAsyncQuestionDeliveryIdentities(ctx)
	if err != nil {
		return fmt.Errorf("list pending async question deliveries: %w", err)
	}
	if len(identities) > 0 && s.messageQueue == nil {
		return fmt.Errorf("retire pending async question deliveries: message queue is unavailable")
	}
	for _, identity := range identities {
		state, admittedAt, inspectErr := s.messageQueue.AsyncQuestionAdmissionDisposition(ctx, identity.AdmissionID, identity.QuestionID)
		if inspectErr != nil {
			return fmt.Errorf("inspect async question admission %s: %w", identity.AdmissionID, inspectErr)
		}
		switch state {
		case messagequeue.AsyncQuestionAdmissionMissing:
			if err := store.MarkAsyncQuestionDeliveryRetired(ctx, identity, nil); err != nil {
				return fmt.Errorf("retire unadmitted async question delivery %s: %w", identity.AdmissionID, err)
			}
		case messagequeue.AsyncQuestionAdmissionQueued, messagequeue.AsyncQuestionAdmissionUncertain:
			retireAt := admittedAt
			if err := store.MarkAsyncQuestionDeliveryRetired(ctx, identity, &retireAt); err != nil {
				return fmt.Errorf("retire queued async question delivery %s: %w", identity.AdmissionID, err)
			}
		case messagequeue.AsyncQuestionAdmissionAccepted, messagequeue.AsyncQuestionAdmissionSettled:
			if err := store.MarkAsyncQuestionDeliveryAdmitted(ctx, identity.AdmissionID, admittedAt); err != nil {
				return fmt.Errorf("settle async question admission %s: %w", identity.AdmissionID, err)
			}
		default:
			return fmt.Errorf("inspect async question admission %s: unsupported disposition %q", identity.AdmissionID, state)
		}
	}
	return nil
}

func (s *Service) retireQueuedAsyncQuestionCandidates(
	ctx context.Context,
	store asyncQuestionStartupStore,
) error {
	if s.messageQueue == nil {
		return nil
	}
	candidates, err := s.messageQueue.ListAsyncQuestionAdmissionCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list queued async question admissions: %w", err)
	}
	for _, candidate := range candidates {
		identity := models.AsyncQuestionDeliveryIdentity{
			AdmissionID: candidate.ClientQueueID,
			QuestionID:  candidate.QuestionID,
		}
		retireAt := candidate.AdmittedAt
		if err := store.MarkAsyncQuestionDeliveryRetired(ctx, identity, &retireAt); err != nil {
			return fmt.Errorf("retire queued async question delivery %s: %w", identity.AdmissionID, err)
		}
	}
	return nil
}

func (s *Service) finishAsyncQuestionDeliveryRetirements(
	ctx context.Context,
	store asyncQuestionStartupStore,
) error {
	if s.messageQueue == nil {
		return nil
	}
	candidates, err := s.messageQueue.ListAsyncQuestionAdmissionCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list async question admissions to check for retirement: %w", err)
	}
	for _, candidate := range candidates {
		retired, err := store.IsAsyncQuestionDeliveryRetired(ctx, candidate.ClientQueueID, candidate.QuestionID)
		if err != nil {
			return fmt.Errorf("check async question retirement %s: %w", candidate.ClientQueueID, err)
		}
		if !retired {
			continue
		}
		if _, err := s.messageQueue.RetireAsyncQuestionQueueAdmission(ctx, candidate.ClientQueueID, candidate.QuestionID); err != nil {
			return fmt.Errorf("remove retired async question admission %s: %w", candidate.ClientQueueID, err)
		}
	}
	return nil
}

func (s *Service) asyncQuestionDispatchRetirement(
	ctx context.Context,
	message *messagequeue.QueuedMessage,
) (models.AsyncQuestionDeliveryIdentity, bool, error) {
	if message == nil || message.QueuedBy != messagequeue.QueuedByAsyncQuestion || message.Metadata == nil {
		return models.AsyncQuestionDeliveryIdentity{}, false, nil
	}
	questionID, _ := message.Metadata[messagequeue.MetadataAsyncQuestion].(string)
	if questionID == "" {
		return models.AsyncQuestionDeliveryIdentity{}, false, nil
	}
	admissionID, found, err := s.messageQueue.AsyncQuestionAdmissionIDForQueueMessage(ctx, message)
	if err != nil || !found {
		return models.AsyncQuestionDeliveryIdentity{}, false, err
	}
	store, ok := s.repo.(asyncQuestionStartupStore)
	if !ok {
		return models.AsyncQuestionDeliveryIdentity{}, false, nil
	}
	retired, err := store.IsAsyncQuestionDeliveryRetired(ctx, admissionID, questionID)
	return models.AsyncQuestionDeliveryIdentity{AdmissionID: admissionID, QuestionID: questionID}, retired, err
}

package messagequeue

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type asyncQuestionAdmissionRetirer interface {
	RetireAsyncQuestionQueueAdmission(context.Context, string, string) (bool, error)
}

type asyncQuestionAdmissionInspector interface {
	AsyncQuestionAdmissionDisposition(context.Context, string, string) (AsyncQuestionAdmissionState, time.Time, error)
	ListAsyncQuestionAdmissionCandidates(context.Context) ([]AsyncQuestionAdmissionCandidate, error)
	AsyncQuestionAdmissionIDForQueueMessage(context.Context, *QueuedMessage) (string, bool, error)
}

// AsyncQuestionAdmissionDisposition reports whether an outbox receipt is
// pending in the queue, has an ambiguous dispatch claim, or is already settled.
func (s *Service) AsyncQuestionAdmissionDisposition(
	ctx context.Context,
	clientQueueID, questionID string,
) (AsyncQuestionAdmissionState, time.Time, error) {
	if s == nil || s.repo == nil {
		return AsyncQuestionAdmissionMissing, time.Time{}, nil
	}
	inspector, ok := s.repo.(asyncQuestionAdmissionInspector)
	if !ok {
		return AsyncQuestionAdmissionMissing, time.Time{}, nil
	}
	return inspector.AsyncQuestionAdmissionDisposition(ctx, clientQueueID, questionID)
}

// ListAsyncQuestionAdmissionCandidates returns only live generated rows and
// unsettled claims, not historical admission receipts.
func (s *Service) ListAsyncQuestionAdmissionCandidates(
	ctx context.Context,
) ([]AsyncQuestionAdmissionCandidate, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	inspector, ok := s.repo.(asyncQuestionAdmissionInspector)
	if !ok {
		return nil, nil
	}
	return inspector.ListAsyncQuestionAdmissionCandidates(ctx)
}

// AsyncQuestionAdmissionIDForQueueMessage resolves legacy dispatch claims from
// their accepted queue id and receipt when they predate admission metadata.
func (s *Service) AsyncQuestionAdmissionIDForQueueMessage(
	ctx context.Context,
	message *QueuedMessage,
) (string, bool, error) {
	if s == nil || s.repo == nil {
		return "", false, nil
	}
	inspector, ok := s.repo.(asyncQuestionAdmissionInspector)
	if !ok {
		return "", false, nil
	}
	return inspector.AsyncQuestionAdmissionIDForQueueMessage(ctx, message)
}

// RetireAsyncQuestionQueueAdmission removes a generated prompt only when its
// durable client admission receipt still identifies the exact unsent queue row.
func (s *Service) RetireAsyncQuestionQueueAdmission(
	ctx context.Context,
	clientQueueID, questionID string,
) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("retire async question queue admission: %w", ErrQueueAdmissionUnavailable)
	}
	retirer, ok := s.repo.(asyncQuestionAdmissionRetirer)
	if !ok {
		return false, nil
	}
	removed, err := retirer.RetireAsyncQuestionQueueAdmission(ctx, clientQueueID, questionID)
	if err != nil {
		return false, err
	}
	if removed {
		s.logger.Info("retired unsent async question queue entry",
			zap.String("client_queue_id", clientQueueID),
			zap.String("question_id", questionID))
	}
	return removed, nil
}

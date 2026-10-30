package questions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// MetadataAsyncQuestionID identifies a correlated async question
	// continuation on a queued message. It is not an editable user draft.
	MetadataAsyncQuestionID = "async_question_id"
	// MetadataAsyncQuestionKind names the continuation kind.
	MetadataAsyncQuestionKind = "async_question_kind"
	// MetadataAsyncQuestionOutcomeVersion pins the winning outcome.
	MetadataAsyncQuestionOutcomeVersion = "async_question_outcome_version"
	// MetadataAsyncQuestionNotification marks the entry as a generated
	// notification so queue UI and auto-merge cannot treat it as a draft.
	MetadataAsyncQuestionNotification = "async_question_notification"

	deliveryBackoffBase = 2 * time.Second
	deliveryBackoffMax  = 2 * time.Minute
	deliveryPageSize    = 50
)

// admissionID derives the immutable queue admission identity. A restart or
// retry repeats this exact id, so queue admission produces no second row.
func admissionID(questionID string, outcomeVersion int64, kind models.AsyncQuestionDeliveryKind, destination string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%s", questionID, outcomeVersion, kind, destination)))
	return "aq-" + hex.EncodeToString(sum[:16])
}

func (s *Service) newDelivery(
	question *models.AsyncQuestion,
	kind models.AsyncQuestionDeliveryKind,
	destinationTaskID, destinationSessionID, content string,
	metadata map[string]interface{},
) *models.AsyncQuestionDelivery {
	incarnationID := question.SessionIncarnationID
	if kind == models.AsyncQuestionDeliveryParentQuestion || kind == models.AsyncQuestionDeliveryParentOutcome {
		incarnationID = question.ParentSessionIncarnationID
	}
	now := s.now()
	return &models.AsyncQuestionDelivery{
		AdmissionID:              admissionID(question.ID, question.Version, kind, destinationSessionID),
		QuestionID:               question.ID,
		OutcomeVersion:           question.Version,
		Kind:                     kind,
		DestinationTaskID:        destinationTaskID,
		DestinationSession:       destinationSessionID,
		DestinationIncarnationID: incarnationID,
		Content:                  content,
		Metadata:                 metadata,
		State:                    models.AsyncQuestionDeliveryPending,
		NextAttemptAt:            now,
		CreatedAt:                now,
	}
}

func correlatedMetadata(question *models.AsyncQuestion, kind models.AsyncQuestionDeliveryKind) map[string]interface{} {
	return map[string]interface{}{
		MetadataAsyncQuestionID:             question.ID,
		MetadataAsyncQuestionKind:           string(kind),
		MetadataAsyncQuestionOutcomeVersion: question.Version,
		MetadataAsyncQuestionNotification:   true,
		"sender_task_id":                    question.TaskID,
		"sender_session_id":                 question.SessionID,
	}
}

func (s *Service) parentNotificationDelivery(question *models.AsyncQuestion) *models.AsyncQuestionDelivery {
	content := formatParentNotification(question)
	metadata := correlatedMetadata(question, models.AsyncQuestionDeliveryParentQuestion)
	metadata[models.MetaKeyParentQuestionID] = question.ID
	metadata[models.MetaKeyParentQuestion] = true
	metadata[models.MetaKeyParentQuestionStatus] = "pending"
	return s.newDelivery(question, models.AsyncQuestionDeliveryParentQuestion,
		question.ParentTaskID, question.ParentSessionID, content, metadata)
}

func (s *Service) ownerOutcomeDelivery(question *models.AsyncQuestion, outcome models.AsyncQuestionOutcome) *models.AsyncQuestionDelivery {
	content := formatOutcomeForOwner(question, outcome)
	metadata := correlatedMetadata(question, models.AsyncQuestionDeliveryOwner)
	metadata["async_question_responder"] = string(outcome.ResponderKind)
	return s.newDelivery(question, models.AsyncQuestionDeliveryOwner,
		question.TaskID, question.SessionID, content, metadata)
}

func (s *Service) parentOutcomeDelivery(question *models.AsyncQuestion, outcome models.AsyncQuestionOutcome) *models.AsyncQuestionDelivery {
	content := formatOutcomeForParent(question, outcome)
	metadata := correlatedMetadata(question, models.AsyncQuestionDeliveryParentOutcome)
	metadata[models.MetaKeyParentQuestionID] = question.ID
	metadata[models.MetaKeyParentQuestion] = true
	metadata[models.MetaKeyParentQuestionStatus] = "answered"
	return s.newDelivery(question, models.AsyncQuestionDeliveryParentOutcome,
		question.ParentTaskID, question.ParentSessionID, content, metadata)
}

func formatParentNotification(question *models.AsyncQuestion) string {
	data, _ := json.MarshalIndent(question.Questions, "", "  ")
	contextLine := strings.TrimSpace(question.Context)
	if contextLine == "" {
		contextLine = "(no additional context)"
	}
	return fmt.Sprintf("ASYNC CHILD QUESTION\n\nChild task: %s (%s)\nQuestion ID: %s\nContext: %s\n\nQuestions:\n%s\n\nThis child continues working while you decide. Reply with message_task_kandev using task_id=%s, reply_to_question_id=%s, and your answer as the prompt. Do not wait for the child to pause.",
		question.TaskID, question.TaskID, question.ID, contextLine, data, question.TaskID, question.ID)
}

func formatOutcomeForOwner(question *models.AsyncQuestion, outcome models.AsyncQuestionOutcome) string {
	data, _ := json.MarshalIndent(outcome, "", "  ")
	return fmt.Sprintf("ASYNC QUESTION ANSWERED\n\nQuestion ID: %s\nOriginal context: %s\nOutcome:\n%s\n\nThis is the correlated decision for a question you asked earlier. Read it at your next checkpoint. It does not cancel or steer your current turn; continue your work and use the answer where it applies.",
		question.ID, strings.TrimSpace(question.Context), data)
}

func formatOutcomeForParent(question *models.AsyncQuestion, outcome models.AsyncQuestionOutcome) string {
	data, _ := json.MarshalIndent(outcome, "", "  ")
	return fmt.Sprintf("ASYNC CHILD QUESTION RESOLVED\n\nChild task: %s\nQuestion ID: %s\nOutcome:\n%s\n\nThe canonical child request is now closed. No manual relay is required.",
		question.TaskID, question.ID, data)
}

// ReconcileOnce admits every due durable intent. It returns the number of
// intents that reached an admission receipt.
func (s *Service) ReconcileOnce(ctx context.Context) (int, error) {
	if s.queue == nil {
		return 0, ErrQueueUnavailable
	}
	admitted := 0
	for {
		deliveries, err := s.store.ListAsyncQuestionDeliveries(ctx, s.now(), deliveryPageSize)
		if err != nil {
			return admitted, err
		}
		if len(deliveries) == 0 {
			return admitted, nil
		}
		for _, delivery := range deliveries {
			if err := ctx.Err(); err != nil {
				return admitted, err
			}
			ok, err := s.admitDelivery(ctx, delivery)
			if err != nil {
				return admitted, err
			}
			if ok {
				admitted++
			}
		}
	}
}

func (s *Service) admitDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) (bool, error) {
	if err := s.queue.AdmitAsyncQuestionDelivery(ctx, delivery); err != nil {
		if errors.Is(err, ErrDestinationTerminal) {
			return false, s.store.MarkAsyncQuestionDeliveryAbandoned(ctx, delivery.AdmissionID, err.Error())
		}
		s.recordDeliveryFailure(ctx, delivery, err)
		return false, nil
	}
	if err := s.store.MarkAsyncQuestionDeliveryAdmitted(ctx, delivery.AdmissionID, s.now()); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) recordDeliveryFailure(ctx context.Context, delivery *models.AsyncQuestionDelivery, admitErr error) {
	attempts := delivery.Attempts + 1
	backoff := deliveryBackoffBase * time.Duration(1<<min(attempts, 6))
	if backoff > deliveryBackoffMax {
		backoff = deliveryBackoffMax
	}
	if err := s.store.MarkAsyncQuestionDeliveryFailed(ctx, delivery.AdmissionID, attempts, s.now().Add(backoff), admitErr.Error()); err != nil {
		s.logger.Warn("failed to record async question delivery failure",
			zap.String("admission_id", delivery.AdmissionID), zap.Error(err))
	}
	s.logger.Warn("async question delivery admission failed",
		zap.String("admission_id", delivery.AdmissionID),
		zap.String("question_id", delivery.QuestionID), zap.Error(admitErr))
}

// Start runs the cancellable reconciliation loop with a startup drain.
func (s *Service) Start(parent context.Context) {
	if err := s.ReconcileWaitGates(parent); err != nil {
		s.logger.Warn("failed to reconcile async question waits on startup", zap.Error(err))
	}
	if s.queue == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if _, err := s.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("async question outbox reconciliation failed", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-s.wake:
			case <-ticker.C:
			}
		}
	}()
}

// Stop cancels and joins the reconciliation worker.
func (s *Service) Stop() {
	if s.cancel == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	s.cancel()
	<-s.done
	s.cancel = nil
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// buildWaitWakeDelivery creates the wake intent atomically with withdrawal and
// gate release. A requested gate stays fenced until its pause acknowledges.
func (s *Service) buildWaitWakeDelivery(
	question *models.AsyncQuestion,
	wait *models.AsyncQuestionWait,
	version int64,
) *models.AsyncQuestionDelivery {
	if question == nil || wait == nil {
		return nil
	}
	content := fmt.Sprintf("ASYNC QUESTION WITHDRAWN\n\nQuestion ID: %s\nThe explicit wait for this request is released because the question was withdrawn. Continue with your remaining work.", question.ID)
	delivery := s.newDelivery(question, models.AsyncQuestionDeliveryWake,
		wait.TaskID, wait.SessionID, content,
		correlatedMetadata(question, models.AsyncQuestionDeliveryWake))
	delivery.OutcomeVersion = version
	delivery.AdmissionID = admissionID(question.ID, version, models.AsyncQuestionDeliveryWake, wait.SessionID)
	return delivery
}

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
	authoritySweepLimit = 100
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

func (s *Service) transcriptDelivery(question *models.AsyncQuestion) *models.AsyncQuestionDelivery {
	data, _ := json.MarshalIndent(question.Questions, "", "  ")
	content := fmt.Sprintf("ASYNC QUESTION REQUEST\n\nQuestion ID: %s\nContext: %s\n\nQuestions:\n%s",
		question.ID, strings.TrimSpace(question.Context), data)
	metadata := map[string]interface{}{
		MetadataAsyncQuestionID:   question.ID,
		MetadataAsyncQuestionKind: string(models.AsyncQuestionDeliveryTranscript),
	}
	return s.newDelivery(question, models.AsyncQuestionDeliveryTranscript,
		question.TaskID, question.SessionID, content, metadata)
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
	questions, _ := json.MarshalIndent(question.Questions, "", "  ")
	return fmt.Sprintf("ASYNC QUESTION ANSWERED\n\nQuestion ID: %s\nOriginal context: %s\nQuestions and options:\n%s\nOutcome:\n%s\n\nThis is the correlated decision for a question you asked earlier. Read it at your next checkpoint. It does not cancel or steer your current turn; continue your work and use the answer where it applies.",
		question.ID, strings.TrimSpace(question.Context), questions, data)
}

func formatOutcomeForParent(question *models.AsyncQuestion, outcome models.AsyncQuestionOutcome) string {
	data, _ := json.MarshalIndent(outcome, "", "  ")
	questions, _ := json.MarshalIndent(question.Questions, "", "  ")
	return fmt.Sprintf("ASYNC CHILD QUESTION RESOLVED\n\nChild task: %s\nQuestion ID: %s\nQuestions and options:\n%s\nOutcome:\n%s\n\nThe canonical child request is now closed. No manual relay is required.",
		question.TaskID, question.ID, questions, data)
}

// ReconcileOnce admits every due durable intent. It returns the number of
// intents that reached an admission receipt.
func (s *Service) ReconcileOnce(ctx context.Context) (int, error) {
	stale, staleErr := s.store.ReconcileAsyncQuestionAuthority(ctx, authoritySweepLimit, s.buildWaitWakeDelivery)
	for _, question := range stale {
		s.notify(ctx, question, "updated")
		s.retireParentQuestionPrompt(ctx, question)
	}
	if len(stale) > 0 {
		s.signal()
	}
	if staleErr != nil {
		return 0, staleErr
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
	if delivery.Kind == models.AsyncQuestionDeliveryTranscript {
		return s.admitTranscriptDelivery(ctx, delivery)
	}
	return s.admitQueuedDelivery(ctx, delivery)
}

func (s *Service) admitTranscriptDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) (bool, error) {
	if s.transcript == nil {
		return false, ErrTranscriptWriterUnavailable
	}
	question, err := s.store.GetAsyncQuestion(ctx, delivery.QuestionID)
	if err != nil {
		s.recordDeliveryFailure(ctx, delivery, err)
		return false, nil
	}
	if question == nil {
		return false, s.store.MarkAsyncQuestionDeliveryAbandoned(ctx, delivery.AdmissionID, "question no longer exists")
	}
	if err := s.transcript.WriteAsyncQuestionTranscriptEntry(ctx, delivery, question); err != nil {
		s.recordDeliveryFailure(ctx, delivery, err)
		return false, nil
	}
	if err := s.store.MarkAsyncQuestionDeliveryAdmitted(ctx, delivery.AdmissionID, s.now()); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) admitQueuedDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) (bool, error) {
	if s.queue == nil {
		return false, ErrQueueUnavailable
	}
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
	s.lifecycleMu.Lock()
	if s.running || (s.queue == nil && s.transcript == nil) {
		s.lifecycleMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	wake := make(chan struct{}, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	s.running = true
	s.cancel = cancel
	s.wake = wake
	s.stop = stop
	s.done = done
	s.lifecycleMu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if _, err := s.ReconcileOnce(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("async question outbox reconciliation failed", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-wake:
			case <-ticker.C:
			}
		}
	}()
}

// Stop cancels and joins the reconciliation worker. Repeated calls are safe,
// and a later Start creates fresh lifecycle channels.
func (s *Service) Stop() {
	s.lifecycleMu.Lock()
	if !s.running {
		s.lifecycleMu.Unlock()
		return
	}
	s.running = false
	stop := s.stop
	cancel := s.cancel
	done := s.done
	s.lifecycleMu.Unlock()
	close(stop)
	cancel()
	<-done
}

func (s *Service) signal() {
	s.lifecycleMu.Lock()
	wake := s.wake
	s.lifecycleMu.Unlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

// buildWaitWakeDelivery creates the wake intent atomically with withdrawal and
// gate release. A requested gate stays fenced until its pause acknowledges.
func (s *Service) buildWaitWakeDelivery(
	question *models.AsyncQuestion,
	wait *models.AsyncQuestionWait,
	version int64,
	reason string,
) *models.AsyncQuestionDelivery {
	if question == nil || wait == nil {
		return nil
	}
	content := fmt.Sprintf("ASYNC QUESTION WITHDRAWN\n\nQuestion ID: %s\nThe explicit wait for this request is released because the question was withdrawn. Continue with your remaining work.", question.ID)
	if reason != "withdrawn" {
		content = fmt.Sprintf("ASYNC QUESTION STALE\n\nQuestion ID: %s\nReason: %s\nThe explicit wait for this request is released because the request can no longer receive an answer. Continue with your remaining work.", question.ID, reason)
	}
	delivery := s.newDelivery(question, models.AsyncQuestionDeliveryWake,
		wait.TaskID, wait.SessionID, content,
		correlatedMetadata(question, models.AsyncQuestionDeliveryWake))
	delivery.OutcomeVersion = version
	delivery.AdmissionID = admissionID(question.ID, version, models.AsyncQuestionDeliveryWake, wait.SessionID)
	return delivery
}

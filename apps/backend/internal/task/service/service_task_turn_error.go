package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrTurnErrorStampRequired reports a dismissal request without the observed
// attention stamp.
var ErrTurnErrorStampRequired = errors.New("turn error stamp is required")

// ErrTurnErrorStale reports a dismissal whose observed stamp is no longer the
// current attention record. The newer record is preserved.
var ErrTurnErrorStale = errors.New("turn error stamp is stale")

// ErrTurnErrorNotFound reports a dismissal for a task with no current
// turn-error attention.
var ErrTurnErrorNotFound = errors.New("turn error not found")

// turnErrorStore is the conditional task-metadata capability required by
// turn-error attention. The production repository implements it; focused test
// repositories may omit it.
type turnErrorStore interface {
	RecordTaskTurnError(ctx context.Context, taskID string, value models.TaskTurnError) (bool, error)
	AcknowledgeTaskTurnErrorIfStamp(
		ctx context.Context,
		taskID, stamp string,
		acknowledgedAt time.Time,
		reason string,
	) (models.TurnErrorAcknowledgementResult, error)
}

func (s *Service) turnErrorStore() (turnErrorStore, error) {
	store, ok := s.tasks.(turnErrorStore)
	if !ok {
		return nil, errors.New("task turn-error store is unavailable")
	}
	return store, nil
}

// RecordTaskTurnErrorAttention persists the latest failed-interactive-turn
// pointer. It is called only by the trusted retained-turn settlement owner,
// after the failed-turn message is durable and before successor admission is
// released.
func (s *Service) RecordTaskTurnErrorAttention(
	ctx context.Context,
	taskID string,
	value models.TaskTurnError,
) (bool, error) {
	value = models.NormalizeTaskTurnError(value)
	if taskID == "" || !value.Valid() {
		return false, nil
	}
	store, err := s.turnErrorStore()
	if err != nil {
		return false, err
	}
	stored, err := store.RecordTaskTurnError(ctx, taskID, value)
	if err != nil {
		return false, err
	}
	if stored {
		s.publishTurnErrorTaskUpdated(ctx, taskID)
	}
	return stored, nil
}

// DismissTaskTurnError acknowledges exactly the observed task attention. It is
// shared by the inline alert and the task context/overflow menu.
func (s *Service) DismissTaskTurnError(ctx context.Context, taskID, stamp string) error {
	if taskID == "" || stamp == "" {
		return ErrTurnErrorStampRequired
	}
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return err
	}
	store, err := s.turnErrorStore()
	if err != nil {
		return err
	}
	result, err := store.AcknowledgeTaskTurnErrorIfStamp(
		context.WithoutCancel(ctx), taskID, stamp, time.Now().UTC(), models.TurnErrorAcknowledgementDismiss,
	)
	if err != nil {
		return err
	}
	switch result {
	case models.TurnErrorAcknowledgementStored, models.TurnErrorAcknowledgementNoOp:
		s.publishTurnErrorTaskUpdated(ctx, taskID)
		return nil
	case models.TurnErrorAcknowledgementStale:
		return ErrTurnErrorStale
	default:
		return ErrTurnErrorNotFound
	}
}

// AcknowledgeTaskTurnError is the accepted-manual-prompt acknowledgement. It
// only retires an attention record whose source session matches the prompt
// target. A stale, missing, or foreign stamp leaves attention unchanged and
// must never reject otherwise valid input.
func (s *Service) AcknowledgeTaskTurnError(ctx context.Context, taskID, sessionID, stamp string) error {
	if taskID == "" || sessionID == "" || stamp == "" {
		return nil
	}
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return err
	}
	current, ok := models.LoadTaskTurnError(task.Metadata)
	if !ok || current.SessionID != sessionID || current.StampValue != stamp {
		return nil
	}
	if current.Acknowledged() {
		return nil
	}
	store, err := s.turnErrorStore()
	if err != nil {
		return err
	}
	result, err := store.AcknowledgeTaskTurnErrorIfStamp(
		context.WithoutCancel(ctx), taskID, stamp, time.Now().UTC(), models.TurnErrorAcknowledgementPrompt,
	)
	if err != nil {
		return err
	}
	if result == models.TurnErrorAcknowledgementStored {
		s.publishTurnErrorTaskUpdated(ctx, taskID)
	}
	return nil
}

// publishTurnErrorTaskUpdated publishes the ordinary task mutation so the
// status-summary projector and task-list clients refresh their projection.
func (s *Service) publishTurnErrorTaskUpdated(ctx context.Context, taskID string) {
	if s.eventBus == nil || taskID == "" {
		return
	}
	eventCtx := context.WithoutCancel(ctx)
	task, err := s.tasks.GetTask(eventCtx, taskID)
	if err != nil {
		s.logger.Warn("failed to load task after turn-error mutation",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if task == nil {
		return
	}
	s.publishTaskEvent(eventCtx, events.TaskUpdated, task, nil)
}

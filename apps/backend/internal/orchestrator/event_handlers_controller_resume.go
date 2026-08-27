package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const controllerResumeCoalesceKey = "controller_resume"

func controllerResumeReviewableState(state v1.TaskState) bool {
	return state == v1.TaskStateReview || models.IsTerminalTaskState(state)
}

func (s *Service) maybeResumeParentController(ctx context.Context, data watcher.TaskEventData) {
	if data.NewState == nil || !controllerResumeReviewableState(*data.NewState) {
		return
	}
	if data.OldState != nil && *data.OldState == *data.NewState {
		return
	}
	child, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil {
		s.logger.Warn("controller resume: failed to load changed child task", zap.String("task_id", data.TaskID), zap.Error(err))
		return
	}
	if child.ParentID == "" {
		return
	}
	s.resumeParentControllerForChild(ctx, child, *data.NewState)
}

func (s *Service) resumeParentControllerForChild(ctx context.Context, child *models.Task, newState v1.TaskState) {
	parent, err := s.repo.GetTask(ctx, child.ParentID)
	if err != nil {
		s.logger.Warn("controller resume: failed to load parent task", zap.String("parent_task_id", child.ParentID), zap.Error(err))
		return
	}
	if parent.ID == child.ID || parent.IsEphemeral || parent.ArchivedAt != nil || !models.IsControllerResumeEnabled(parent.Metadata) {
		return
	}
	operationID := controllerResumeOperationID(parent.ID, child.ID, newState, child.UpdatedAt)
	if _, loaded := s.controllerResumeApplied.LoadOrStore(operationID, struct{}{}); loaded {
		return
	}
	if err := s.dispatchControllerResumeWake(ctx, parent.ID, child, newState); err != nil {
		s.controllerResumeApplied.Delete(operationID)
		s.logger.Warn("controller resume: wake dispatch failed", zap.String("parent_task_id", parent.ID), zap.String("child_task_id", child.ID), zap.Error(err))
	}
}

func (s *Service) dispatchControllerResumeWake(ctx context.Context, parentID string, child *models.Task, newState v1.TaskState) error {
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, parentID)
	if err != nil || session == nil {
		return fmt.Errorf("parent task %s has no active session", parentID)
	}
	prompt := fmt.Sprintf("[controller-resume] Direct child task %q (%s) entered %s. Review its result and continue orchestration.", child.Title, child.ID, newState)
	metadata := map[string]interface{}{
		"source":        "controller_resume",
		"child_task_id": child.ID,
		"child_state":   string(newState),
	}
	return s.deliverControllerResumePrompt(ctx, session, prompt, metadata)
}

func (s *Service) deliverControllerResumePrompt(ctx context.Context, session *models.TaskSession, prompt string, metadata map[string]interface{}) error {
	switch session.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateRunning, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
	default:
		return fmt.Errorf("session %s is not promptable: %s", session.ID, session.State)
	}
	if s.messageQueue == nil {
		return fmt.Errorf("message queue is not configured")
	}
	if _, _, err := s.messageQueue.QueueMessageWithCoalesceKey(ctx, session.ID, session.TaskID, prompt, "", messagequeue.QueuedByWorkflow, false, nil, metadata, controllerResumeCoalesceKey, true); err != nil {
		return err
	}
	s.publishQueueStatusEvent(ctx, session.ID)
	if session.State == models.TaskSessionStateWaitingForInput || session.State == models.TaskSessionStateIdle {
		s.drainQueuedMessageForPromptableSession(ctx, session.ID)
	}
	return nil
}

func controllerResumeOperationID(parentID, childID string, state v1.TaskState, childUpdatedAt time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", parentID, childID, state, childUpdatedAt.UTC().Format(time.RFC3339Nano))))
	return fmt.Sprintf("controller_resume:%s:%s", parentID, hex.EncodeToString(sum[:]))
}

package orchestrator

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// handleAgentRunning handles agent running events (user sent input in passthrough mode)
// This is called when the user sends input to the agent, indicating a new turn started.
func (s *Service) handleAgentRunning(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent running event",
			zap.String("task_id", data.TaskID))
		return
	}

	// Process on_turn_start workflow events (step transitions).
	// For ACP sessions this happens in the message handler before PromptTask;
	// for passthrough it happens here when the PTY detects user input.
	s.processOnTurnStartViaEngine(ctx, data.TaskID, data.SessionID)

	// Move session to running and task to in progress
	s.setSessionRunning(ctx, data.TaskID, data.SessionID)
}

// publishQueueStatusEvent publishes a queue status changed event for the given session
func (s *Service) publishQueueStatusEvent(ctx context.Context, sessionID string) {
	if s.eventBus == nil {
		return
	}

	queueStatus := s.messageQueue.GetStatus(ctx, sessionID)
	eventData := map[string]interface{}{
		"session_id": sessionID,
		"is_queued":  queueStatus.IsQueued,
		"message":    queueStatus.Message,
	}

	s.logger.Debug("publishing queue status changed event",
		zap.String("session_id", sessionID),
		zap.Bool("is_queued", queueStatus.IsQueued))

	_ = s.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"orchestrator",
		eventData,
	))
}

// requeueMessage re-enqueues a message that could not be delivered, publishing a queue status event on success.
func (s *Service) requeueMessage(ctx context.Context, queuedMsg *messagequeue.QueuedMessage, queuedBy string) {
	requeuedMsg, queueErr := s.messageQueue.QueueMessage(
		ctx,
		queuedMsg.SessionID,
		queuedMsg.TaskID,
		queuedMsg.Content,
		queuedMsg.Model,
		queuedBy,
		queuedMsg.PlanMode,
		queuedMsg.Attachments,
	)
	if queueErr != nil {
		s.logger.Error("failed to requeue message",
			zap.String("session_id", queuedMsg.SessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID),
			zap.String("queued_by", queuedBy),
			zap.Error(queueErr))
		return
	}
	s.logger.Info("message requeued",
		zap.String("session_id", queuedMsg.SessionID),
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("old_queue_id", queuedMsg.ID),
		zap.String("new_queue_id", requeuedMsg.ID),
		zap.String("queued_by", queuedBy))
	s.publishQueueStatusEvent(ctx, queuedMsg.SessionID)
}

// handleAgentReady handles agent ready events (turn complete in passthrough mode)
// This is called when the agent finishes processing and is waiting for input.
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {

	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent ready event",
			zap.String("task_id", data.TaskID))
		return
	}

	if s.isSessionResetInProgress(data.SessionID) {
		s.logger.Debug("ignoring agent.ready while session reset is in progress",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("agent_execution_id", data.AgentExecutionID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent.ready",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	if data.AgentExecutionID != "" && session.AgentExecutionID != "" && session.AgentExecutionID != data.AgentExecutionID {
		s.logger.Debug("ignoring stale agent.ready for non-active execution",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("event_execution_id", data.AgentExecutionID),
			zap.String("active_execution_id", session.AgentExecutionID))
		return
	}

	if session.State == models.TaskSessionStateStarting {
		s.logger.Info("agent.ready received while session is STARTING; marking session idle",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		s.completeTurnForSession(ctx, data.SessionID)
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID)
		return
	}

	if session.State != models.TaskSessionStateRunning {
		s.logger.Debug("ignoring agent.ready while session is not running",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("session_state", string(session.State)))
		return
	}

	// Complete the current turn
	s.completeTurnForSession(ctx, data.SessionID)

	// Check for workflow transition based on session's current step.
	// Uses the engine when available; falls back to legacy evaluation.
	// The ViaEngine method handles setSessionWaitingForInput internally when no transition occurs.
	s.processOnTurnCompleteViaEngine(ctx, data.TaskID, data.SessionID)

	// ALWAYS check for queued messages after agent becomes ready, regardless of workflow transition
	queueStatus := s.messageQueue.GetStatus(ctx, data.SessionID)
	s.logger.Info("checking for queued messages",
		zap.String("session_id", data.SessionID),
		zap.Bool("is_queued", queueStatus.IsQueued),
		zap.Any("message", queueStatus.Message))

	// Skip queued messages for passthrough sessions — PromptTask uses ACP which
	// doesn't work for passthrough, and writing to PTY stdin is unreliable.
	// Check before TakeQueued to avoid destructively removing the message.
	if s.agentManager.IsPassthroughSession(ctx, data.SessionID) {
		s.logger.Info("skipping queued message for passthrough session",
			zap.String("session_id", data.SessionID))
		return
	}

	queuedMsg, exists := s.messageQueue.TakeQueued(ctx, data.SessionID)
	if !exists {
		s.logger.Debug("no queued message to execute",
			zap.String("session_id", data.SessionID))
		return
	}

	// Skip if the queued message has empty content (might have been cleared accidentally)
	if queuedMsg.Content == "" && len(queuedMsg.Attachments) == 0 {
		s.logger.Warn("skipping empty queued message",
			zap.String("session_id", data.SessionID),
			zap.String("queue_id", queuedMsg.ID))

		// Still publish status change to clear frontend state
		s.publishQueueStatusEvent(ctx, data.SessionID)
		return
	}

	s.logger.Info("auto-executing queued message",
		zap.String("session_id", data.SessionID),
		zap.String("task_id", queuedMsg.TaskID),
		zap.String("queue_id", queuedMsg.ID))

	// PromptTask rejects while session is RUNNING; queued follow-ups should be
	// treated like fresh user input after turn completion.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	// Publish queue status changed event to notify frontend
	s.publishQueueStatusEvent(ctx, data.SessionID)

	// Execute the queued message asynchronously
	go s.executeQueuedMessage(data.SessionID, queuedMsg)
}

func (s *Service) executeQueuedMessage(callerSessionID string, queuedMsg *messagequeue.QueuedMessage) {
	promptCtx := context.Background() // Use a fresh context for async execution

	if s.isSessionResetInProgress(queuedMsg.SessionID) {
		s.logger.Warn("queued message execution deferred due to context reset in progress",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID))
		s.requeueMessage(promptCtx, queuedMsg, "workflow-auto-start-reset-retry")
		return
	}

	// Create user message for the queued message (so it appears in chat history)
	if s.messageCreator != nil {
		turnID := s.getActiveTurnID(queuedMsg.SessionID)
		if turnID == "" {
			// Start a new turn if needed
			s.startTurnForSession(promptCtx, queuedMsg.SessionID)
			turnID = s.getActiveTurnID(queuedMsg.SessionID)
		}

		// Note: Attachments will be sent to the agent via PromptTask but not stored in the message
		// This matches the behavior of direct prompts
		meta := NewUserMessageMeta().WithPlanMode(queuedMsg.PlanMode)
		err := s.messageCreator.CreateUserMessage(promptCtx, queuedMsg.TaskID, queuedMsg.Content, queuedMsg.SessionID, turnID, meta.ToMap())
		if err != nil {
			s.logger.Error("failed to create user message for queued message",
				zap.String("session_id", queuedMsg.SessionID),
				zap.Error(err))
			// Continue anyway - the prompt should still be sent
		}
	}

	// Convert queue attachments to v1 attachments
	attachments := make([]v1.MessageAttachment, len(queuedMsg.Attachments))
	for i, att := range queuedMsg.Attachments {
		attachments[i] = v1.MessageAttachment{
			Type:     att.Type,
			Data:     att.Data,
			MimeType: att.MimeType,
		}
	}

	_, err := s.PromptTask(promptCtx, queuedMsg.TaskID, queuedMsg.SessionID,
		queuedMsg.Content, queuedMsg.Model, queuedMsg.PlanMode, attachments)
	if err != nil {
		s.logger.Error("failed to execute queued message",
			zap.String("session_id", callerSessionID),
			zap.String("task_id", queuedMsg.TaskID),
			zap.String("queue_id", queuedMsg.ID),
			zap.Error(err))

		if isAgentPromptInProgressError(err) || isTransientPromptError(err) || isSessionResetInProgressError(err) {
			s.logger.Warn("queued message execution failed transiently; requeueing",
				zap.String("session_id", callerSessionID),
				zap.String("task_id", queuedMsg.TaskID),
				zap.String("queue_id", queuedMsg.ID))
			s.requeueMessage(promptCtx, queuedMsg, "workflow-auto-start-retry")
			return
		}

		// TODO: Implement dead letter queue for failed queued messages
		// Currently, failed messages are lost. Consider:
		// 1. Retry mechanism with exponential backoff
		// 2. Persist failed messages to database for manual intervention
		// 3. Notification to user about failed queue execution
		s.logger.Warn("queued message execution failed - message is lost (no retry/dead letter queue)",
			zap.String("session_id", callerSessionID),
			zap.String("queue_id", queuedMsg.ID),
			zap.String("content_preview", queuedMsg.Content[:min(50, len(queuedMsg.Content))]))
	}
}

// handleAgentCompleted handles agent completion events
func (s *Service) handleAgentCompleted(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent completed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Update scheduler and remove from queue
	s.scheduler.HandleTaskCompleted(data.TaskID, true)
	s.scheduler.RemoveTask(data.TaskID)

	// Check for workflow transition based on session's current step.
	transitioned := s.processOnTurnCompleteViaEngine(ctx, data.TaskID, data.SessionID)

	// If no workflow transition occurred, move task to REVIEW state for user review
	if !transitioned {
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state after agent completion",
				zap.String("task_id", data.TaskID))
		}
	}
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Check if the agent was started with a resume token (--resume). If so, any failure
	// means the previous session couldn't be restored. Clear the token and let the user
	// start a fresh session — don't treat this as a hard failure.
	if data.SessionID != "" && s.wasResumeAttempt(ctx, data.SessionID) {
		if s.handleResumeFailure(ctx, data) {
			return // Resume token cleared, session set to WAITING_FOR_INPUT
		}
		// Fall through to normal failure handling if cleanup failed
	}

	// Update session state to FAILED
	if data.SessionID != "" {
		s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateFailed, data.ErrorMessage, false)
	}

	// Trigger retry logic
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	if !s.scheduler.RetryTask(data.TaskID) {
		s.logger.Error("task failed and retry limit exceeded",
			zap.String("task_id", data.TaskID))

		// Move task to REVIEW state even on failure - user can decide to retry or close
		// This maintains the review cycle: user reviews the failure and decides next steps
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW after failure",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		} else {
			s.logger.Info("task moved to REVIEW state after failure (for user review)",
				zap.String("task_id", data.TaskID))
		}
	} else {
		// If retry is triggered, also move task to REVIEW state
		// The retry will start a new agent when the task is re-launched
		if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
			s.logger.Error("failed to update task state to REVIEW after failure (with retry pending)",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		}
	}
}

// wasResumeAttempt checks whether the session's last execution used a resume token.
// If the token is still present in the DB, the agent was started with --resume.
func (s *Service) wasResumeAttempt(ctx context.Context, sessionID string) bool {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return false
	}
	return running.ResumeToken != ""
}

// handleResumeFailure handles the case where an agent failed while using a resume token.
// It clears the token so the next attempt starts fresh, and notifies the user.
//
// The session is set to WAITING_FOR_INPUT so the user can send a new message
// (which triggers a fresh agent start without --resume).
//
// Returns true to signal that the caller should skip normal failure handling
// (scheduler retry, FAILED state) since we've handled the state transition ourselves.
func (s *Service) handleResumeFailure(ctx context.Context, data watcher.AgentEventData) bool {
	s.logger.Warn("detected resume failure, clearing token for fresh start on next user action",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("error", data.ErrorMessage))

	// 1. Clear the resume token so the next attempt won't use --resume.
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, data.SessionID)
	if err == nil && running != nil && running.ResumeToken != "" {
		running.ResumeToken = ""
		if upsertErr := s.repo.UpsertExecutorRunning(ctx, running); upsertErr != nil {
			s.logger.Error("failed to clear resume token",
				zap.String("session_id", data.SessionID),
				zap.Error(upsertErr))
		}
	}

	// 2. Send a status message about the failed resume.
	if s.messageCreator != nil {
		statusMsg := fmt.Sprintf("Previous agent session could not be restored (%s). Send a new message to start a fresh session.", data.ErrorMessage)
		if err := s.messageCreator.CreateSessionMessage(
			ctx,
			data.TaskID,
			statusMsg,
			data.SessionID,
			string(v1.MessageTypeStatus),
			s.getActiveTurnID(data.SessionID),
			map[string]interface{}{
				"variant":       "warning",
				"resume_failed": true,
			},
			false,
		); err != nil {
			s.logger.Warn("failed to create resume failure status message",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		}
	}

	// 3. Set session to WAITING_FOR_INPUT (not FAILED) so the user can interact.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false)

	// 4. Ensure task is in REVIEW state.
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Warn("failed to set task to REVIEW after resume failure",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}

	return true
}

// handleAgentStopped handles agent stopped events (manual stop or cancellation)
func (s *Service) handleAgentStopped(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent stopped",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Complete the current turn if there is one
	s.completeTurnForSession(ctx, data.SessionID)

	// Update session state to cancelled (already done by executor, but ensure consistency)
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateCancelled, "", false)

	// NOTE: We do NOT update task state here because:
	// 1. If this is from CompleteTask(), the task state will be set to COMPLETED by the caller
	// 2. If this is from StopTask(), the task state should be set to REVIEW by the caller
	// 3. Updating here would create a race condition with the caller's state update
	//
	// The task state management is the responsibility of the operation that triggered the stop,
	// not the event handler. This handler only manages session-level cleanup.
}

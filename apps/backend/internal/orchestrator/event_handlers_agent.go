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
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent running",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	// on_turn_start is only needed for passthrough sessions where there's
	// no PromptTask call to handle it. For ACP sessions, PromptTask or
	// dispatchPromptAsync already processes on_turn_start.
	if s.agentManager.IsPassthroughSession(ctx, data.SessionID) {
		s.processOnTurnStartViaEngine(ctx, data.TaskID, session)
	}

	// Move session to running and task to in progress
	s.setSessionRunning(ctx, data.TaskID, data.SessionID, session)
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

	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		s.logger.Debug("ignoring agent.ready while session is not running or starting",
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
	s.processOnTurnCompleteViaEngine(ctx, data.TaskID, session)

	// ALWAYS check for queued messages after agent becomes ready, regardless of workflow transition
	queueStatus := s.messageQueue.GetStatus(ctx, data.SessionID)
	s.logger.Info("checking for queued messages",
		zap.String("session_id", data.SessionID),
		zap.Bool("is_queued", queueStatus.IsQueued),
		zap.Any("message", queueStatus.Message))

	// Passthrough sessions: deliver queued messages via PTY stdin instead of ACP.
	if s.agentManager.IsPassthroughSession(ctx, data.SessionID) {
		queuedMsg, exists := s.messageQueue.TakeQueued(ctx, data.SessionID)
		if !exists {
			return
		}
		if queuedMsg.Content != "" {
			if err := s.deliverPassthroughPrompt(ctx, data.SessionID, queuedMsg.Content); err != nil {
				s.logger.Warn("failed to deliver queued message to passthrough",
					zap.String("session_id", data.SessionID),
					zap.Error(err))
			}
		}
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
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, "", false, session)

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

	attachments := make([]v1.MessageAttachment, len(queuedMsg.Attachments))
	for i, att := range queuedMsg.Attachments {
		attachments[i] = v1.MessageAttachment{
			Type:     att.Type,
			Data:     att.Data,
			MimeType: att.MimeType,
		}
	}

	// Create user message for the queued message (so it appears in chat history)
	if s.messageCreator != nil {
		turnID := s.getActiveTurnID(queuedMsg.SessionID)
		if turnID == "" {
			// Start a new turn if needed
			s.startTurnForSession(promptCtx, queuedMsg.SessionID)
			turnID = s.getActiveTurnID(queuedMsg.SessionID)
		}

		meta := NewUserMessageMeta().
			WithPlanMode(queuedMsg.PlanMode).
			WithAttachments(attachments)
		err := s.messageCreator.CreateUserMessage(promptCtx, queuedMsg.TaskID, queuedMsg.Content, queuedMsg.SessionID, turnID, meta.ToMap())
		if err != nil {
			s.logger.Error("failed to create user message for queued message",
				zap.String("session_id", queuedMsg.SessionID),
				zap.Error(err))
			// Continue anyway - the prompt should still be sent
		}
	}

	// Process on_turn_start before sending the queued prompt, just like
	// dispatchPromptAsync does for user-initiated messages. This allows
	// workflow transitions (e.g. move_to_next) to fire on auto-started prompts.
	if session, sErr := s.repo.GetTaskSession(promptCtx, queuedMsg.SessionID); sErr == nil {
		s.processOnTurnStartViaEngine(promptCtx, queuedMsg.TaskID, session)
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
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("failed to load session for agent completed",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	transitioned := s.processOnTurnCompleteViaEngine(ctx, data.TaskID, session)

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

	// Capture a git status snapshot before cleanup so it can be served
	// when clients subscribe to this session later (sidebar diff stats, etc.).
	s.captureGitStatusSnapshot(ctx, data.SessionID)

	// Clean up the agent execution (stop agentctl, release port)
	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Check if the agent was started with a resume token AND session init hadn't completed.
	// If init completed, this is a normal prompt failure (e.g. agent internal timeout),
	// not a resume failure — skip the resume cleanup path.
	if data.SessionID != "" && s.wasResumeAttempt(ctx, data.SessionID) &&
		!s.agentManager.WasSessionInitialized(data.AgentExecutionID) {
		if s.handleResumeFailure(ctx, data) {
			return // Resume token cleared, session set to WAITING_FOR_INPUT
		}
		// Fall through to normal failure handling if cleanup failed
	}

	// Make all agent CLI failures recoverable — let the user choose to resume or start fresh.
	if data.SessionID != "" {
		s.handleRecoverableFailure(ctx, data)
		return
	}

	// No session — fall back to scheduler retry + task to REVIEW.
	s.scheduler.HandleTaskCompleted(data.TaskID, false)
	s.scheduler.RetryTask(data.TaskID)

	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW after failure",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}

	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
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

// clearResumeToken removes the resume token from the executor running record so
// the next agent start won't use --resume. This is used by both automatic resume
// failure handling and user-initiated fresh start recovery.
func (s *Service) clearResumeToken(ctx context.Context, sessionID string) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.ResumeToken == "" {
		return
	}
	running.ResumeToken = ""
	if upsertErr := s.repo.UpsertExecutorRunning(ctx, running); upsertErr != nil {
		s.logger.Error("failed to clear resume token",
			zap.String("session_id", sessionID),
			zap.Error(upsertErr))
	}
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
	s.clearResumeToken(ctx, data.SessionID)

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

// handleRecoverableFailure handles agent failures by keeping the session recoverable.
// Instead of marking the session FAILED (terminal), it sets WAITING_FOR_INPUT and
// creates an error message with recovery action buttons so the user can choose to
// resume the agent session or start fresh.
func (s *Service) handleRecoverableFailure(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling recoverable agent failure",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("error", data.ErrorMessage))

	// Complete the current turn.
	s.completeTurnForSession(ctx, data.SessionID)

	// Create a status message with recovery action metadata.
	if s.messageCreator != nil {
		authErr := isAuthError(data.ErrorMessage)
		displayMsg := data.ErrorMessage
		if authErr {
			if readable := extractReadableAuthError(data.ErrorMessage); readable != "" {
				displayMsg = readable
			}
		}

		statusMsg := fmt.Sprintf("Agent encountered an error: %s", displayMsg)

		hasResumeToken := s.wasResumeAttempt(ctx, data.SessionID)
		meta := map[string]interface{}{
			"variant":          "error",
			"recovery_actions": true,
			"session_id":       data.SessionID,
			"task_id":          data.TaskID,
			"has_resume_token": hasResumeToken,
			"is_auth_error":    authErr,
		}

		// Include cached auth methods so the frontend can show login options.
		if authErr {
			if methods := s.agentManager.GetSessionAuthMethods(data.SessionID); len(methods) > 0 {
				meta["auth_methods"] = methods
			}
		}

		// Build generic actions array for the frontend ActionMessage component.
		meta["actions"] = buildRecoveryActions(data.TaskID, data.SessionID, hasResumeToken, authErr)

		if err := s.messageCreator.CreateSessionMessage(
			ctx,
			data.TaskID,
			statusMsg,
			data.SessionID,
			string(v1.MessageTypeStatus),
			s.getActiveTurnID(data.SessionID),
			meta,
			false,
		); err != nil {
			s.logger.Warn("failed to create recovery status message",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		}
	}

	// Set session to WAITING_FOR_INPUT so the user can interact via recovery buttons.
	s.updateTaskSessionState(ctx, data.TaskID, data.SessionID, models.TaskSessionStateWaitingForInput, data.ErrorMessage, false)

	// Ensure task is in REVIEW state.
	if err := s.taskRepo.UpdateTaskState(ctx, data.TaskID, v1.TaskStateReview); err != nil {
		s.logger.Warn("failed to set task to REVIEW after recoverable failure",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}

	// Clean up the agent execution.
	go s.cleanupAgentExecution(data.AgentExecutionID, data.TaskID, data.SessionID)
}

// handleAgentStartFailed is called by the executor when StartAgentProcess fails.
// It detects auth errors and routes them through the recoverable failure path so
// the frontend shows login guidance instead of a terminal failure.
// Returns true if the failure was handled (caller should skip default FAILED logic).
func (s *Service) handleAgentStartFailed(ctx context.Context, taskID, sessionID, agentExecutionID string, err error) bool {
	if !isAuthError(err.Error()) {
		return false
	}
	s.logger.Info("agent start failure is auth error, treating as recoverable",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	s.handleRecoverableFailure(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: agentExecutionID,
		ErrorMessage:     err.Error(),
	})
	return true
}

// buildRecoveryActions creates the generic actions array for agent error recovery.
func buildRecoveryActions(taskID, sessionID string, hasResumeToken, isAuthError bool) []map[string]interface{} {
	recoverPayload := func(action string) map[string]interface{} {
		return map[string]interface{}{
			"method":  "session.recover",
			"payload": map[string]interface{}{"task_id": taskID, "session_id": sessionID, "action": action},
		}
	}
	actions := []map[string]interface{}{}
	if hasResumeToken {
		actions = append(actions, map[string]interface{}{
			"type": "ws_request", "label": "Resume session", "icon": "refresh",
			"tooltip": "Re-launch with resume flag — keeps all previous messages and context",
			"test_id": "recovery-resume-button", "params": recoverPayload("resume"),
		})
	}
	label := "Start fresh session"
	if isAuthError {
		label = "Restart session"
	}
	testID := "recovery-fresh-button"
	if isAuthError {
		testID = "recovery-restart-button"
	}
	actions = append(actions, map[string]interface{}{
		"type": "ws_request", "label": label, "icon": "player-play",
		"tooltip": "New agent process on the same workspace — no previous conversation context",
		"test_id": testID, "params": recoverPayload("fresh_start"),
	})
	return actions
}

// handleAgentStopped handles agent stopped events (manual stop or cancellation)
func (s *Service) handleAgentStopped(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Info("handling agent stopped",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID))

	// Complete the current turn if there is one
	s.completeTurnForSession(ctx, data.SessionID)

	// Don't override WAITING_FOR_INPUT — the recovery path sets this so the user
	// can choose to resume or start fresh. The stopped event fires as a side-effect
	// of cleanupAgentExecution and should not clobber the recovery state.
	if session, err := s.repo.GetTaskSession(ctx, data.SessionID); err == nil &&
		session.State == models.TaskSessionStateWaitingForInput {
		s.logger.Info("skipping CANCELLED transition, session is in recovery (WAITING_FOR_INPUT)",
			zap.String("session_id", data.SessionID))
		return
	}

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

// cleanupAgentExecution stops the agentctl instance and releases its port after
// the agent reaches a terminal state (completed/failed). This runs in a goroutine
// so it doesn't block the event handler.
func (s *Service) cleanupAgentExecution(executionID, taskID, sessionID string) {
	if executionID == "" {
		return
	}
	ctx := context.Background()
	if err := s.executor.StopExecution(ctx, executionID, "agent completed", true); err != nil {
		s.logger.Debug("agent execution cleanup after terminal state",
			zap.String("execution_id", executionID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

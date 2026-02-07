// Package orchestrator provides event handler methods for the orchestrator service.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// toolKindToMessageType maps the normalized tool kind to a frontend message type.
func toolKindToMessageType(normalized *streams.NormalizedPayload) string {
	if normalized == nil {
		return "tool_call"
	}
	return normalized.Kind().ToMessageType()
}

// Event handlers

func (s *Service) handleACPSessionCreated(ctx context.Context, data watcher.ACPSessionEventData) {
	if data.SessionID == "" || data.ACPSessionID == "" {
		return
	}
	s.storeResumeToken(ctx, data.TaskID, data.SessionID, data.ACPSessionID)
}

// storeResumeToken stores an agent's session ID as the resume token for session recovery.
// This is called both from handleACPSessionCreated (for ACP-based agents) and from
// handleAgentStreamEvent (for stream-based agents like Claude Code).
func (s *Service) storeResumeToken(ctx context.Context, taskID, sessionID, acpSessionID string) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load task session for resume token storage",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	resumable := true
	if session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil {
			resumable = executor.Resumable
		}
	}

	running := &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           session.TaskID,
		ExecutorID:       session.ExecutorID,
		Status:           "ready",
		Resumable:        resumable,
		ResumeToken:      acpSessionID,
		AgentExecutionID: session.AgentExecutionID,
		ContainerID:      session.ContainerID,
	}
	if len(session.Worktrees) > 0 {
		running.WorktreeID = session.Worktrees[0].WorktreeID
		running.WorktreePath = session.Worktrees[0].WorktreePath
		running.WorktreeBranch = session.Worktrees[0].WorktreeBranch
	}

	if err := s.repo.UpsertExecutorRunning(ctx, running); err != nil {
		s.logger.Warn("failed to persist resume token for session",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	s.logger.Debug("stored resume token for session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("resume_token", acpSessionID))
}

// handleAgentRunning handles agent running events (user sent input in passthrough mode)
// This is called when the user sends input to the agent, indicating a new turn started.
func (s *Service) handleAgentRunning(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent running event",
			zap.String("task_id", data.TaskID))
		return
	}

	// Move session to running and task to in progress
	s.setSessionRunning(ctx, data.TaskID, data.SessionID)
}

// handleAgentReady handles agent ready events (turn complete in passthrough mode)
// This is called when the agent finishes processing and is waiting for input.
func (s *Service) handleAgentReady(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" {
		s.logger.Warn("missing session_id for agent ready event",
			zap.String("task_id", data.TaskID))
		return
	}

	// Complete the current turn
	s.completeTurnForSession(ctx, data.SessionID)

	// Check for workflow transition based on session's current step
	// This handles the case where the agent finishes a step and should move to the next
	transitioned := s.handleWorkflowTransition(ctx, data.TaskID, data.SessionID)

	// If no workflow transition occurred, move session to waiting for input and task to review
	if !transitioned {
		s.setSessionWaitingForInput(ctx, data.TaskID, data.SessionID)
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

	// Check for workflow transition based on session's current step
	transitioned := s.handleWorkflowTransition(ctx, data.TaskID, data.SessionID)

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

// handleWorkflowTransition checks if the session's current workflow step has an on_complete_step_id
// and moves the task to that step if so. Returns true if a transition occurred.
func (s *Service) handleWorkflowTransition(ctx context.Context, taskID, sessionID string) bool {
	if sessionID == "" || s.workflowStepGetter == nil {
		return false
	}

	// Get the session to find its current workflow step
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for workflow transition",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	if session.WorkflowStepID == nil || *session.WorkflowStepID == "" {
		s.logger.Debug("session has no workflow step, skipping transition",
			zap.String("session_id", sessionID))
		return false
	}

	workflowStepID := *session.WorkflowStepID

	// Get the current workflow step
	currentStep, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		s.logger.Warn("failed to get workflow step for transition",
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
		return false
	}

	// If the CURRENT step requires approval, don't transition - stay here and wait for approval
	// This handles the case where a step has both AutoStartAgent and RequireApproval enabled
	if currentStep.RequireApproval {
		s.logger.Info("current step requires approval, staying on step",
			zap.String("session_id", sessionID),
			zap.String("step_id", currentStep.ID),
			zap.String("step_name", currentStep.Name))

		// Set review status to pending
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, "pending"); err != nil {
			s.logger.Warn("failed to set session review status to pending",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Info("session review status set to pending",
				zap.String("session_id", sessionID),
				zap.String("current_step", currentStep.Name))
		}

		// Update session state to WAITING_FOR_INPUT and task state to REVIEW
		s.setSessionWaitingForInput(ctx, taskID, sessionID)

		// Publish session updated event with review status
		if s.eventBus != nil {
			eventData := map[string]interface{}{
				"task_id":          taskID,
				"session_id":       sessionID,
				"workflow_step_id": currentStep.ID,
				"new_state":        string(models.TaskSessionStateWaitingForInput),
				"review_status":    "pending",
			}
			_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
				events.TaskSessionStateChanged,
				"orchestrator",
				eventData,
			))
		}

		// Return true to indicate we handled the completion (no further transition needed)
		return true
	}

	// Check if there's an on_complete transition
	if currentStep.OnCompleteStepID == "" {
		s.logger.Debug("workflow step has no on_complete transition",
			zap.String("step_id", currentStep.ID),
			zap.String("step_name", currentStep.Name))
		return false
	}

	// Get the target step to log its name
	targetStep, err := s.workflowStepGetter.GetStep(ctx, currentStep.OnCompleteStepID)
	if err != nil {
		s.logger.Warn("failed to get target workflow step",
			zap.String("target_step_id", currentStep.OnCompleteStepID),
			zap.Error(err))
		return false
	}

	// Get the task to update its workflow step
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to get task for workflow transition",
			zap.String("task_id", taskID),
			zap.Error(err))
		return false
	}

	// Update the task's workflow step
	task.WorkflowStepID = currentStep.OnCompleteStepID
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task to next workflow step",
			zap.String("task_id", taskID),
			zap.String("from_step", currentStep.Name),
			zap.String("to_step", targetStep.Name),
			zap.Error(err))
		return false
	}

	// Publish task updated event so frontend updates the kanban board
	if s.eventBus != nil {
		taskEventData := map[string]interface{}{
			"task_id":          task.ID,
			"board_id":         task.BoardID,
			"workflow_step_id": task.WorkflowStepID,
			"title":            task.Title,
			"description":      task.Description,
			"state":            string(task.State),
			"priority":         task.Priority,
			"position":         task.Position,
		}
		_ = s.eventBus.Publish(ctx, events.TaskUpdated, bus.NewEvent(
			events.TaskUpdated,
			"orchestrator",
			taskEventData,
		))
	}

	// Also update the session's workflow step
	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, currentStep.OnCompleteStepID); err != nil {
		s.logger.Warn("failed to update session workflow step",
			zap.String("session_id", sessionID),
			zap.String("step_id", currentStep.OnCompleteStepID),
			zap.Error(err))
		// Don't return false - task was already moved
	}

	// If the target step requires approval, set the session's review_status to "pending"
	var reviewStatus string
	if targetStep.RequireApproval {
		reviewStatus = "pending"
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, reviewStatus); err != nil {
			s.logger.Warn("failed to set session review status to pending",
				zap.String("session_id", sessionID),
				zap.Error(err))
			reviewStatus = "" // Clear on error
		} else {
			s.logger.Info("session review status set to pending",
				zap.String("session_id", sessionID),
				zap.String("target_step", targetStep.Name))
		}
	} else {
		// Target step does not require approval - clear any existing review status
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Debug("session review status cleared for non-review step",
				zap.String("session_id", sessionID),
				zap.String("target_step", targetStep.Name))
		}
	}

	// Update session state to WAITING_FOR_INPUT and task state to REVIEW since the agent completed this step
	// This must happen before publishing the event so the frontend gets the correct state
	s.setSessionWaitingForInput(ctx, taskID, sessionID)

	// Publish session updated event with workflow step and review status changes
	// This allows the frontend to update the session state without a full refresh
	if s.eventBus != nil {
		eventData := map[string]interface{}{
			"task_id":          taskID,
			"session_id":       sessionID,
			"workflow_step_id": targetStep.ID,
			"new_state":        string(models.TaskSessionStateWaitingForInput),
		}
		if reviewStatus != "" {
			eventData["review_status"] = reviewStatus
		}
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			eventData,
		))
	}

	s.logger.Info("workflow transition completed",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", currentStep.Name),
		zap.String("to_step", targetStep.Name))

	return true
}

// handleReviewStepRollback checks if the session is in a review-type step.
// If so, it moves the session back to the previous step (the step that led to the review step)
// and clears the review status. This handles the case where the user sends a message to
// iterate on the work instead of approving.
// Note: This works for both "simple" boards (where review steps don't require approval) and
// "advanced" boards (where review steps have RequireApproval=true).
func (s *Service) handleReviewStepRollback(ctx context.Context, taskID, sessionID string) {
	if sessionID == "" || s.workflowStepGetter == nil {
		return
	}

	// Get the session to check its current workflow step
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for review rollback check",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	// Only proceed if session has a workflow step
	if session.WorkflowStepID == nil || *session.WorkflowStepID == "" {
		return
	}

	currentStepID := *session.WorkflowStepID

	// Get the current workflow step to check if it's a review step
	currentStep, err := s.workflowStepGetter.GetStep(ctx, currentStepID)
	if err != nil {
		s.logger.Warn("failed to get workflow step for review rollback",
			zap.String("workflow_step_id", currentStepID),
			zap.Error(err))
		return
	}

	// Only proceed if the current step is a pure review step (StepType == "review")
	// Work steps like Planning or Implementation with RequireApproval should NOT rollback
	// because the agent is actively working on them, not just waiting for approval
	if currentStep.StepType != "review" {
		return
	}

	// Get the task to find the board ID
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to get task for review rollback",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}

	// Find the source step (the step that has on_complete_step_id pointing to current step)
	sourceStep, err := s.workflowStepGetter.GetSourceStep(ctx, task.BoardID, currentStepID)
	if err != nil {
		s.logger.Warn("failed to get source step for review rollback",
			zap.String("current_step_id", currentStepID),
			zap.Error(err))
		return
	}
	if sourceStep == nil {
		s.logger.Debug("no source step found for review rollback, staying in current step",
			zap.String("current_step_id", currentStepID))
		return
	}

	// Move session back to the source step
	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, sourceStep.ID); err != nil {
		s.logger.Error("failed to move session back to source step",
			zap.String("session_id", sessionID),
			zap.String("source_step_id", sourceStep.ID),
			zap.Error(err))
		return
	}

	// Clear the review status
	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
		s.logger.Warn("failed to clear session review status",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Also move the task to the source step
	task.WorkflowStepID = sourceStep.ID
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task back to source step",
			zap.String("task_id", taskID),
			zap.String("source_step_id", sourceStep.ID),
			zap.Error(err))
	}

	// Publish session state change event so frontend updates
	if s.eventBus != nil {
		eventData := map[string]interface{}{
			"task_id":          taskID,
			"session_id":       sessionID,
			"workflow_step_id": sourceStep.ID,
			"new_state":        string(session.State),
			"review_status":    "", // Cleared
		}
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			eventData,
		))
	}

	s.logger.Info("session moved back from review step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", currentStep.Name),
		zap.String("to_step", sourceStep.Name))
}

// handleAgentFailed handles agent failure events
func (s *Service) handleAgentFailed(ctx context.Context, data watcher.AgentEventData) {
	s.logger.Warn("handling agent failed",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("agent_execution_id", data.AgentExecutionID),
		zap.String("error_message", data.ErrorMessage))

	// Check if this was a resume failure (agent session no longer exists on the CLI side).
	// If so, clear the resume token and re-launch a fresh session instead of failing.
	// We do this BEFORE setting the session state to FAILED so the re-launch can proceed cleanly.
	if data.SessionID != "" && isResumeFailure(data.ErrorMessage) {
		if s.handleResumeFailure(ctx, data) {
			return // Successfully re-launched without resume
		}
		// Fall through to normal failure handling if re-launch failed
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

// isResumeFailure checks if the error message indicates a failed --resume attempt
// (e.g., the Claude Code conversation was deleted or expired).
func isResumeFailure(errorMsg string) bool {
	lower := strings.ToLower(errorMsg)
	return strings.Contains(lower, "no conversation found")
}

// handleResumeFailure handles the case where an agent failed because --resume pointed
// to a non-existent conversation. It clears the resume token, sends a status message,
// and schedules an asynchronous re-launch of the session without resume.
//
// The re-launch must be async because this handler is called synchronously from
// MarkCompleted → PublishAgentEvent, BEFORE RemoveExecution cleans up the failed
// execution from the manager's store. Calling ResumeSession synchronously would
// fail with ErrExecutionAlreadyRunning.
//
// Returns true to signal that the caller should skip normal failure handling
// (scheduler retry, FAILED state) since we're handling the retry ourselves.
func (s *Service) handleResumeFailure(ctx context.Context, data watcher.AgentEventData) bool {
	s.logger.Warn("detected resume failure, retrying without resume",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("error", data.ErrorMessage))

	// 1. Clear the resume token so the next attempt won't use --resume.
	// Note: executor.ResumeSession already upserts a new ExecutorRunning without ResumeToken,
	// but clear it explicitly to be safe against race conditions.
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, data.SessionID)
	if err == nil && running != nil && running.ResumeToken != "" {
		running.ResumeToken = ""
		if upsertErr := s.repo.UpsertExecutorRunning(ctx, running); upsertErr != nil {
			s.logger.Error("failed to clear resume token",
				zap.String("session_id", data.SessionID),
				zap.Error(upsertErr))
		}
	}

	// 2. Send a status message about the failed resume
	if s.messageCreator != nil {
		statusMsg := fmt.Sprintf("Session resume failed: %s. Starting a fresh session...", data.ErrorMessage)
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

	// 3. Schedule async re-launch. We use a goroutine because this handler runs
	// synchronously inside the MarkCompleted → PublishAgentEvent call chain,
	// before RemoveExecution clears the old execution from the manager's store.
	// A synchronous ResumeSession call would fail with ErrExecutionAlreadyRunning.
	taskID := data.TaskID
	sessionID := data.SessionID
	go func() {
		// Brief delay to let RemoveExecution complete after event handler returns
		time.Sleep(500 * time.Millisecond)
		bgCtx := context.Background()

		session, err := s.repo.GetTaskSession(bgCtx, sessionID)
		if err != nil {
			s.logger.Error("failed to get session for resume retry",
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = s.repo.UpdateTaskSessionState(bgCtx, sessionID, models.TaskSessionStateFailed, "re-launch failed: "+err.Error())
			return
		}

		_, err = s.executor.ResumeSession(bgCtx, session, true)
		if err != nil {
			s.logger.Error("failed to re-launch session after resume failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			_ = s.repo.UpdateTaskSessionState(bgCtx, sessionID, models.TaskSessionStateFailed, "re-launch failed: "+err.Error())
			return
		}

		s.logger.Info("session re-launched after resume failure (fresh session without --resume)",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
	}()

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

// handleAgentStreamEvent handles agent stream events (tool calls, message chunks, etc.)
func (s *Service) handleAgentStreamEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil {
		return
	}

	taskID := payload.TaskID
	sessionID := payload.SessionID
	eventType := payload.Data.Type

	s.logger.Debug("handling agent stream event",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("event_type", eventType))

	// Handle different event types
	switch eventType {
	case "message_streaming":
		s.handleMessageStreamingEvent(ctx, payload)

	case "thinking_streaming":
		s.handleThinkingStreamingEvent(ctx, payload)

	case "tool_call":
		s.saveAgentTextIfPresent(ctx, payload)
		s.handleToolCallEvent(ctx, payload)

	case "tool_update":
		s.handleToolUpdateEvent(ctx, payload)

	case "complete":
		s.logger.Debug("orchestrator received complete event",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Int("text_length", len(payload.Data.Text)),
			zap.Bool("has_text", payload.Data.Text != ""))
		s.saveAgentTextIfPresent(ctx, payload)
		s.completeTurnForSession(ctx, sessionID)
		s.setSessionWaitingForInput(ctx, taskID, sessionID)

	case "error":
		// Handle error events
		// Note: Agent-specific error parsing is done in the adapter layer.
		// The adapter provides a user-friendly message in the Text field,
		// with raw details in the Data field for debugging.
		if sessionID != "" && s.messageCreator != nil {
			// Build error message - adapters should provide parsed message in Text field
			errorMsg := payload.Data.Error
			if errorMsg == "" {
				errorMsg = payload.Data.Text
			}
			if errorMsg == "" {
				errorMsg = "An error occurred while processing your request"
			}

			// Build metadata with all available error context
			metadata := map[string]interface{}{
				"provider":       "agent",
				"provider_agent": payload.AgentID,
			}
			if payload.Data.Data != nil {
				metadata["error_data"] = payload.Data.Data
			}

			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				errorMsg,
				sessionID,
				string(v1.MessageTypeError),
				s.getActiveTurnID(sessionID),
				metadata,
				false,
			); err != nil {
				s.logger.Error("failed to create error message",
					zap.String("task_id", taskID),
					zap.Error(err))
			}
		}
		// Complete the turn since the agent errored
		s.completeTurnForSession(ctx, sessionID)

	case "session_status":
		// Handle session status events (resumed vs new session)
		// Also store the agent's session ID as resume token for session recovery
		if sessionID != "" && payload.Data.ACPSessionID != "" {
			s.storeResumeToken(ctx, taskID, sessionID, payload.Data.ACPSessionID)
		}

		if sessionID != "" && s.messageCreator != nil {
			var statusMsg string
			if payload.Data.SessionStatus == "resumed" {
				statusMsg = "Session resumed"
			} else {
				statusMsg = "New session started"
			}
			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				statusMsg,
				sessionID,
				string(v1.MessageTypeStatus),
				s.getActiveTurnID(sessionID),
				nil,
				false,
			); err != nil {
				s.logger.Error("failed to create session status message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(err))
			}
		}

	case "available_commands":
		// Handle available commands events - broadcast to WebSocket for frontend
		if sessionID != "" && s.eventBus != nil && len(payload.Data.AvailableCommands) > 0 {
			eventPayload := lifecycle.AvailableCommandsEventPayload{
				TaskID:            taskID,
				SessionID:         sessionID,
				AgentID:           payload.AgentID,
				AvailableCommands: payload.Data.AvailableCommands,
			}
			subject := events.BuildAvailableCommandsSubject(sessionID)
			_ = s.eventBus.Publish(ctx, subject, bus.NewEvent(events.AvailableCommandsUpdated, "orchestrator", eventPayload))
		}

	case "log":
		// Handle log events - store agent log messages to the database
		if sessionID != "" && s.messageCreator != nil {
			// Try to extract data as map
			dataMap, _ := payload.Data.Data.(map[string]interface{})

			// Extract log message content
			logMsg := payload.Data.Text
			if logMsg == "" && dataMap != nil {
				if msg, ok := dataMap["message"].(string); ok {
					logMsg = msg
				}
			}
			if logMsg == "" {
				return // No message content to store
			}

			// Build metadata with log level and other context
			metadata := map[string]interface{}{
				"provider":       "agent",
				"provider_agent": payload.AgentID,
			}
			if dataMap != nil {
				if level, ok := dataMap["level"].(string); ok {
					metadata["level"] = level
				}
				// Include any additional data from the log event
				for k, v := range dataMap {
					if k != "message" && k != "level" {
						metadata[k] = v
					}
				}
			}

			if err := s.messageCreator.CreateSessionMessage(
				ctx,
				taskID,
				logMsg,
				sessionID,
				string(v1.MessageTypeLog),
				s.getActiveTurnID(sessionID),
				metadata,
				false,
			); err != nil {
				s.logger.Error("failed to create log message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(err))
			} else {
				level := "unknown"
				if l, ok := metadata["level"].(string); ok {
					level = l
				}
				s.logger.Debug("created log message",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.String("level", level))
			}
		}
	}
}

// handleToolCallEvent handles tool_call events and creates messages
func (s *Service) handleToolCallEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_call",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ParentToolCallID, // Pass parent for subagent nesting
			payload.Data.ToolTitle,
			payload.Data.ToolStatus,
			payload.SessionID,
			s.getActiveTurnID(payload.SessionID),
			payload.Data.Normalized, // Pass normalized tool data for message metadata
		); err != nil {
			s.logger.Error("failed to create tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		} else {
			s.logger.Debug("created tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID))
		}

		s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
	}
}

// saveAgentTextIfPresent saves any accumulated agent text as an agent message
func (s *Service) saveAgentTextIfPresent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator != nil {
		if err := s.messageCreator.CreateAgentMessage(ctx, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create agent message",
				zap.String("task_id", payload.TaskID),
				zap.Error(err))
		} else {
			s.logger.Debug("created agent message",
				zap.String("task_id", payload.TaskID),
				zap.Int("message_length", len(payload.Data.Text)))
		}
	}
}

// handleMessageStreamingEvent handles streaming message events for real-time text updates.
// It creates a new message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleMessageStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator == nil {
		return
	}

	// MessageID is pre-generated by the manager to avoid race conditions
	messageID := payload.Data.MessageID
	if messageID == "" {
		s.logger.Warn("streaming message event missing message ID",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	if payload.Data.IsAppend {
		// Append to existing message
		if err := s.messageCreator.AppendAgentMessage(ctx, messageID, payload.Data.Text); err != nil {
			s.logger.Error("failed to append to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("appended to streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	} else {
		// Create new streaming message with the pre-generated ID
		if err := s.messageCreator.CreateAgentMessageStreaming(ctx, messageID, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("created streaming message",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	}
}

// handleThinkingStreamingEvent handles streaming thinking events for real-time reasoning updates.
// It creates a new thinking message on first chunk (IsAppend=false) or appends to existing (IsAppend=true).
func (s *Service) handleThinkingStreamingEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.Data.Text == "" || payload.SessionID == "" {
		return
	}

	if s.messageCreator == nil {
		return
	}

	// MessageID is pre-generated by the manager to avoid race conditions
	messageID := payload.Data.MessageID
	if messageID == "" {
		s.logger.Warn("streaming thinking event missing message ID",
			zap.String("task_id", payload.TaskID),
			zap.String("session_id", payload.SessionID))
		return
	}

	if payload.Data.IsAppend {
		// Append to existing thinking message
		if err := s.messageCreator.AppendThinkingMessage(ctx, messageID, payload.Data.Text); err != nil {
			s.logger.Error("failed to append to streaming thinking message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("appended to streaming thinking message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	} else {
		// Create new streaming thinking message with the pre-generated ID
		if err := s.messageCreator.CreateThinkingMessageStreaming(ctx, messageID, payload.TaskID, payload.Data.Text, payload.SessionID, s.getActiveTurnID(payload.SessionID)); err != nil {
			s.logger.Error("failed to create streaming thinking message",
				zap.String("task_id", payload.TaskID),
				zap.String("message_id", messageID),
				zap.Error(err))
		} else {
			s.logger.Debug("created streaming thinking message",
				zap.String("task_id", payload.TaskID),
				zap.String("session_id", payload.SessionID),
				zap.String("message_id", messageID),
				zap.Int("content_length", len(payload.Data.Text)))
		}
	}
}

// handleToolUpdateEvent handles tool_update events and updates messages
func (s *Service) handleToolUpdateEvent(ctx context.Context, payload *lifecycle.AgentStreamEventPayload) {
	if payload.SessionID == "" {
		s.logger.Warn("missing session_id for tool_update",
			zap.String("task_id", payload.TaskID),
			zap.String("tool_call_id", payload.Data.ToolCallID))
		return
	}

	if s.messageCreator == nil {
		return
	}

	// Determine message type from normalized payload for fallback creation
	msgType := toolKindToMessageType(payload.Data.Normalized)

	// Handle all status updates (running, complete, error)
	switch payload.Data.ToolStatus {
	case "running", "complete", "completed", "success", "error", "failed":
		if err := s.messageCreator.UpdateToolCallMessage(
			ctx,
			payload.TaskID,
			payload.Data.ToolCallID,
			payload.Data.ParentToolCallID, // Pass parent for subagent nesting
			payload.Data.ToolStatus,
			"", // result - no longer used, tool results in NormalizedPayload
			payload.SessionID,
			payload.Data.ToolTitle,                // Include title from update event
			s.getActiveTurnID(payload.SessionID), // Turn ID for fallback creation
			msgType,                               // Message type for fallback creation
			payload.Data.Normalized,               // Pass normalized tool data for message metadata
		); err != nil {
			s.logger.Warn("failed to update tool call message",
				zap.String("task_id", payload.TaskID),
				zap.String("tool_call_id", payload.Data.ToolCallID),
				zap.Error(err))
		}

		// Update session state for completion events
		if payload.Data.ToolStatus == "complete" || payload.Data.ToolStatus == "completed" ||
			payload.Data.ToolStatus == "success" || payload.Data.ToolStatus == "error" || payload.Data.ToolStatus == "failed" {
			s.updateTaskSessionState(ctx, payload.TaskID, payload.SessionID, models.TaskSessionStateRunning, "", false)
		}
	}
}

func (s *Service) updateTaskSessionState(ctx context.Context, taskID, sessionID string, nextState models.TaskSessionState, errorMessage string, allowWakeFromWaiting bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return
	}
	if session.State == models.TaskSessionStateWaitingForInput && nextState == models.TaskSessionStateRunning && !allowWakeFromWaiting {
		return
	}
	oldState := session.State
	switch session.State {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return
	}
	if session.State == nextState {
		return
	}
	if err := s.repo.UpdateTaskSessionState(ctx, sessionID, nextState, errorMessage); err != nil {
		s.logger.Error("failed to update task session state",
			zap.String("session_id", sessionID),
			zap.String("state", string(nextState)),
			zap.Error(err))
	}
	s.logger.Debug("task session state updated",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(nextState)))
	if s.eventBus != nil {
		eventData := map[string]interface{}{
			"task_id":                taskID,
			"session_id":             sessionID,
			"old_state":              string(oldState),
			"new_state":              string(nextState),
			"agent_profile_id":       session.AgentProfileID,
			"agent_profile_snapshot": session.AgentProfileSnapshot,
		}
		// Include review_status and workflow_step_id if present to ensure frontend state consistency
		if session.ReviewStatus != nil {
			eventData["review_status"] = *session.ReviewStatus
		}
		if session.WorkflowStepID != nil {
			eventData["workflow_step_id"] = *session.WorkflowStepID
		}
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(events.TaskSessionStateChanged, "task-session", eventData))
	}
}

func (s *Service) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		s.logger.Error("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to REVIEW state",
			zap.String("task_id", taskID))
	}
}

func (s *Service) setSessionRunning(ctx context.Context, taskID, sessionID string) {
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateRunning, "", true)

	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		s.logger.Error("failed to update task state to IN_PROGRESS",
			zap.String("task_id", taskID),
			zap.Error(err))
	} else {
		s.logger.Info("task moved to IN_PROGRESS state",
			zap.String("task_id", taskID))
	}
}

// handleGitEvent handles unified git events and dispatches to appropriate handler
func (s *Service) handleGitEvent(ctx context.Context, data watcher.GitEventData) {
	s.logger.Debug("handling git event",
		zap.String("type", string(data.Type)),
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))

	if data.SessionID == "" {
		s.logger.Debug("missing session_id for git event",
			zap.String("task_id", data.TaskID),
			zap.String("type", string(data.Type)))
		return
	}

	switch data.Type {
	case lifecycle.GitEventTypeStatusUpdate:
		s.handleGitStatusUpdate(ctx, data)
	case lifecycle.GitEventTypeCommitCreated:
		s.handleGitCommitCreated(ctx, data)
	case lifecycle.GitEventTypeCommitsReset:
		s.handleGitCommitsReset(ctx, data)
	case lifecycle.GitEventTypeSnapshotCreated:
		// Snapshot events are published from orchestrator, no need to handle here
		s.logger.Debug("received snapshot_created event, no action needed",
			zap.String("session_id", data.SessionID))
	default:
		s.logger.Warn("unknown git event type",
			zap.String("type", string(data.Type)),
			zap.String("session_id", data.SessionID))
	}
}

// handleGitStatusUpdate handles git status updates by creating git snapshots
func (s *Service) handleGitStatusUpdate(ctx context.Context, data watcher.GitEventData) {
	if data.Status == nil {
		s.logger.Debug("missing status data for git status update",
			zap.String("task_id", data.TaskID))
		return
	}

	// Forward status_update event to WebSocket subject for frontend
	// Since data is already lifecycle.GitEventPayload, we can forward it directly
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitWSEvent, "orchestrator", &data)
		_ = s.eventBus.Publish(ctx, events.BuildGitWSEventSubject(data.SessionID), event)
	}

	// Convert Files from interface{} to map[string]interface{}
	var files map[string]interface{}
	if data.Status.Files != nil {
		if f, ok := data.Status.Files.(map[string]interface{}); ok {
			files = f
		}
	}

	// Create git snapshot instead of storing in session metadata
	snapshot := &models.GitSnapshot{
		SessionID:    data.SessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Branch:       data.Status.Branch,
		RemoteBranch: data.Status.RemoteBranch,
		HeadCommit:   data.Status.HeadCommit,
		BaseCommit:   data.Status.BaseCommit,
		Ahead:        data.Status.Ahead,
		Behind:       data.Status.Behind,
		Files:        files,
		TriggeredBy:  "git_status_event",
		Metadata: map[string]interface{}{
			"modified":  data.Status.Modified,
			"added":     data.Status.Added,
			"deleted":   data.Status.Deleted,
			"untracked": data.Status.Untracked,
			"renamed":   data.Status.Renamed,
			"timestamp": data.Timestamp,
		},
	}

	sessionID := data.SessionID
	taskID := data.TaskID

	// Persist snapshot to database asynchronously, but skip if duplicate of latest
	go func() {
		bgCtx := context.Background()

		// Check if this is a duplicate of the latest snapshot
		latest, err := s.repo.GetLatestGitSnapshot(bgCtx, sessionID)
		if err == nil && latest != nil {
			// Compare key fields to detect duplicates
			if s.isSnapshotDuplicate(latest, snapshot) {
				s.logger.Debug("skipping duplicate git snapshot",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID))
				return
			}
		}

		if err := s.repo.CreateGitSnapshot(bgCtx, snapshot); err != nil {
			s.logger.Error("failed to create git snapshot",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			s.logger.Debug("created git snapshot",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("snapshot_id", snapshot.ID))

			// Publish event to notify frontend
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeSnapshotCreated,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: snapshot.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					Snapshot: &lifecycle.GitSnapshotData{
						ID:           snapshot.ID,
						SessionID:    snapshot.SessionID,
						SnapshotType: string(snapshot.SnapshotType),
						Branch:       snapshot.Branch,
						RemoteBranch: snapshot.RemoteBranch,
						HeadCommit:   snapshot.HeadCommit,
						BaseCommit:   snapshot.BaseCommit,
						Ahead:        snapshot.Ahead,
						Behind:       snapshot.Behind,
						Files:        snapshot.Files,
						TriggeredBy:  snapshot.TriggeredBy,
						CreatedAt:    snapshot.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}

// isSnapshotDuplicate checks if two snapshots have the same content
func (s *Service) isSnapshotDuplicate(existing, new *models.GitSnapshot) bool {
	// Different snapshot types are never duplicates
	if existing.SnapshotType != new.SnapshotType {
		return false
	}

	// Compare branch and commit info
	if existing.Branch != new.Branch ||
		existing.HeadCommit != new.HeadCommit ||
		existing.Ahead != new.Ahead ||
		existing.Behind != new.Behind {
		return false
	}

	// Compare file counts first (quick check)
	existingFileCount := len(existing.Files)
	newFileCount := len(new.Files)
	if existingFileCount != newFileCount {
		return false
	}

	// Compare file paths, staged status, line counts, and diff content
	for path, newFileData := range new.Files {
		existingFileData, exists := existing.Files[path]
		if !exists {
			return false
		}

		// Compare file details - extract from interface{}
		newInfo := extractFileInfo(newFileData)
		existingInfo := extractFileInfo(existingFileData)

		if newInfo.staged != existingInfo.staged ||
			newInfo.additions != existingInfo.additions ||
			newInfo.deletions != existingInfo.deletions ||
			newInfo.diff != existingInfo.diff {
			return false
		}
	}

	return true
}

// fileInfoCompare holds extracted file info fields for comparison
type fileInfoCompare struct {
	staged    bool
	additions int
	deletions int
	diff      string
}

// extractFileInfo extracts comparable fields from a file info interface
func extractFileInfo(fileData interface{}) fileInfoCompare {
	info := fileInfoCompare{}
	if fileData == nil {
		return info
	}
	if fileMap, ok := fileData.(map[string]interface{}); ok {
		if staged, ok := fileMap["staged"].(bool); ok {
			info.staged = staged
		}
		// Handle both int and float64 (JSON numbers are float64)
		if additions, ok := fileMap["additions"].(float64); ok {
			info.additions = int(additions)
		} else if additions, ok := fileMap["additions"].(int); ok {
			info.additions = additions
		}
		if deletions, ok := fileMap["deletions"].(float64); ok {
			info.deletions = int(deletions)
		} else if deletions, ok := fileMap["deletions"].(int); ok {
			info.deletions = deletions
		}
		// Extract diff content for comparison
		if diff, ok := fileMap["diff"].(string); ok {
			info.diff = diff
		}
	}
	return info
}

// handleContextWindowUpdated handles context window updates and persists them to session metadata
func (s *Service) handleContextWindowUpdated(ctx context.Context, data watcher.ContextWindowData) {
	s.logger.Debug("handling context window update",
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.TaskSessionID),
		zap.Int64("size", data.ContextWindowSize),
		zap.Int64("used", data.ContextWindowUsed))

	if data.TaskSessionID == "" {
		s.logger.Debug("missing session_id for context window update",
			zap.String("task_id", data.TaskID))
		return
	}

	session, err := s.repo.GetTaskSession(ctx, data.TaskSessionID)
	if err != nil {
		s.logger.Debug("no task session for context window update",
			zap.String("session_id", data.TaskSessionID),
			zap.Error(err))
		return
	}

	// Update session metadata with context window info
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	contextWindowData := map[string]interface{}{
		"size":       data.ContextWindowSize,
		"used":       data.ContextWindowUsed,
		"remaining":  data.ContextWindowRemaining,
		"efficiency": data.ContextEfficiency,
	}
	session.Metadata["context_window"] = contextWindowData

	// Persist to database asynchronously
	go func() {
		if err := s.repo.UpdateTaskSession(context.Background(), session); err != nil {
			s.logger.Error("failed to update session with context window",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("persisted context window to session",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", session.ID))
		}
	}()

	// Broadcast session state change with metadata so frontend can update
	// This uses the existing session.state_changed event with metadata included
	if s.eventBus != nil {
		_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"orchestrator",
			map[string]interface{}{
				"task_id":                data.TaskID,
				"session_id":             session.ID,
				"new_state":              string(session.State),
				"agent_profile_id":       session.AgentProfileID,
				"agent_profile_snapshot": session.AgentProfileSnapshot,
				"metadata": map[string]interface{}{
					"context_window": contextWindowData,
				},
			},
		))
	}
}

// handlePermissionRequest handles permission request events and saves as message
func (s *Service) handlePermissionRequest(ctx context.Context, data watcher.PermissionRequestData) {
	s.logger.Debug("handling permission request",
		zap.String("task_id", data.TaskID),
		zap.String("pending_id", data.PendingID),
		zap.String("title", data.Title))

	if data.TaskSessionID == "" {
		s.logger.Warn("missing session_id for permission_request",
			zap.String("task_id", data.TaskID),
			zap.String("pending_id", data.PendingID))
		return
	}

	s.setSessionWaitingForInput(ctx, data.TaskID, data.TaskSessionID)

	if s.messageCreator != nil {
		_, err := s.messageCreator.CreatePermissionRequestMessage(
			ctx,
			data.TaskID,
			data.TaskSessionID,
			data.PendingID,
			data.ToolCallID,
			data.Title,
			s.getActiveTurnID(data.TaskSessionID),
			data.Options,
			data.ActionType,
			data.ActionDetails,
		)
		if err != nil {
			s.logger.Error("failed to create permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID),
				zap.Error(err))
		} else {
			s.logger.Debug("created permission request message",
				zap.String("task_id", data.TaskID),
				zap.String("pending_id", data.PendingID))
		}
	}
}

// handleGitCommitCreated handles git commit events by creating session commit records
func (s *Service) handleGitCommitCreated(ctx context.Context, data watcher.GitEventData) {
	if data.Commit == nil {
		s.logger.Debug("missing commit data for git commit event",
			zap.String("task_id", data.TaskID))
		return
	}

	s.logger.Debug("handling git commit created",
		zap.String("task_id", data.TaskID),
		zap.String("commit_sha", data.Commit.CommitSHA))

	// Parse committed_at timestamp
	var committedAt time.Time
	if data.Commit.CommittedAt != "" {
		if t, err := time.Parse(time.RFC3339, data.Commit.CommittedAt); err == nil {
			committedAt = t
		} else {
			committedAt = time.Now().UTC()
		}
	} else {
		committedAt = time.Now().UTC()
	}

	sessionID := data.SessionID
	taskID := data.TaskID
	commitSHA := data.Commit.CommitSHA

	// Create session commit record
	commit := &models.SessionCommit{
		SessionID:     sessionID,
		CommitSHA:     data.Commit.CommitSHA,
		ParentSHA:     data.Commit.ParentSHA,
		AuthorName:    data.Commit.AuthorName,
		AuthorEmail:   data.Commit.AuthorEmail,
		CommitMessage: data.Commit.Message,
		CommittedAt:   committedAt,
		FilesChanged:  data.Commit.FilesChanged,
		Insertions:    data.Commit.Insertions,
		Deletions:     data.Commit.Deletions,
	}

	// Persist commit record to database asynchronously
	go func() {
		bgCtx := context.Background()
		if err := s.repo.CreateSessionCommit(bgCtx, commit); err != nil {
			s.logger.Error("failed to create session commit record",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("commit_sha", commitSHA),
				zap.Error(err))
		} else {
			s.logger.Debug("created session commit record",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("commit_sha", commitSHA))

			// Publish event to notify frontend using unified format
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeCommitCreated,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
					Commit: &lifecycle.GitCommitData{
						ID:           commit.ID,
						CommitSHA:    commit.CommitSHA,
						ParentSHA:    commit.ParentSHA,
						Message:      commit.CommitMessage,
						AuthorName:   commit.AuthorName,
						AuthorEmail:  commit.AuthorEmail,
						FilesChanged: commit.FilesChanged,
						Insertions:   commit.Insertions,
						Deletions:    commit.Deletions,
						CommittedAt:  commit.CommittedAt.Format(time.RFC3339),
						CreatedAt:    commit.CreatedAt.Format("2006-01-02T15:04:05.000000000Z07:00"),
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}

// handleGitCommitsReset handles git reset events by removing orphaned commits
func (s *Service) handleGitCommitsReset(ctx context.Context, data watcher.GitEventData) {
	if data.Reset == nil {
		s.logger.Debug("missing reset data for git reset event",
			zap.String("task_id", data.TaskID))
		return
	}

	sessionID := data.SessionID
	taskID := data.TaskID
	previousHead := data.Reset.PreviousHead
	currentHead := data.Reset.CurrentHead

	s.logger.Info("handling git commits reset",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("previous_head", previousHead),
		zap.String("current_head", currentHead))

	// Remove orphaned commits asynchronously
	go func() {
		bgCtx := context.Background()

		// Get all stored commits for this session
		commits, err := s.repo.GetSessionCommits(bgCtx, sessionID)
		if err != nil {
			s.logger.Error("failed to get session commits for reset handling",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}

		if len(commits) == 0 {
			return
		}

		// Build a map for quick lookup
		commitBySHA := make(map[string]*models.SessionCommit)
		for _, c := range commits {
			commitBySHA[c.CommitSHA] = c
		}

		// Walk the parent chain from currentHead to find reachable commits
		reachable := make(map[string]bool)
		current := currentHead
		for current != "" {
			reachable[current] = true
			if c, exists := commitBySHA[current]; exists {
				current = c.ParentSHA
			} else {
				// Parent not in our stored commits, stop walking
				break
			}
		}

		// Delete commits that are not reachable from currentHead
		var deleted int
		for _, c := range commits {
			if !reachable[c.CommitSHA] {
				if err := s.repo.DeleteSessionCommit(bgCtx, c.ID); err != nil {
					s.logger.Error("failed to delete orphaned commit",
						zap.String("session_id", sessionID),
						zap.String("commit_sha", c.CommitSHA),
						zap.Error(err))
				} else {
					deleted++
					s.logger.Debug("deleted orphaned commit after reset",
						zap.String("session_id", sessionID),
						zap.String("commit_sha", c.CommitSHA))
				}
			}
		}

		if deleted > 0 {
			s.logger.Info("removed orphaned commits after git reset",
				zap.String("session_id", sessionID),
				zap.Int("deleted_count", deleted),
				zap.String("new_head", currentHead))

			// Publish event to notify frontend that commits changed
			if s.eventBus != nil {
				event := bus.NewEvent(events.GitEvent, "orchestrator", &lifecycle.GitEventPayload{
					Type:      lifecycle.GitEventTypeCommitsReset,
					SessionID: sessionID,
					TaskID:    taskID,
					Timestamp: time.Now().Format("2006-01-02T15:04:05.000000000Z07:00"),
					Reset: &lifecycle.GitResetData{
						PreviousHead: previousHead,
						CurrentHead:  currentHead,
						DeletedCount: deleted,
					},
				})
				_ = s.eventBus.Publish(bgCtx, events.BuildGitWSEventSubject(sessionID), event)
			}
		}
	}()
}

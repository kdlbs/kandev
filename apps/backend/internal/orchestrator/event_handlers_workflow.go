package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// processOnTurnComplete processes the on_turn_complete events for the current step.
// Returns true if a transition occurred (step change happened).
func (s *Service) processOnTurnComplete(ctx context.Context, taskID, sessionID string) bool {
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
	// If no on_turn_complete actions, do nothing (manual step)
	if len(currentStep.Events.OnTurnComplete) == 0 {
		s.logger.Debug("step has no on_turn_complete actions, waiting for user",
			zap.String("step_id", currentStep.ID),
			zap.String("step_name", currentStep.Name))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return false
	}

	// Process side-effect actions first, then find the first transition action
	transitionAction := s.processTurnCompleteActions(ctx, sessionID, currentStep)

	// If no transition action found, just apply side effects and wait
	if transitionAction == nil {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return false
	}
	targetStepID, ok := s.resolveTransitionTargetStep(ctx, taskID, sessionID, currentStep, transitionAction)
	if !ok {
		return false
	}
	s.executeStepTransition(ctx, taskID, sessionID, currentStep, targetStepID, true)
	return true
}

func (s *Service) resolveTransitionTargetStep(ctx context.Context, taskID, sessionID string, currentStep *wfmodels.WorkflowStep, action *wfmodels.OnTurnCompleteAction) (string, bool) {
	switch action.Type {
	case wfmodels.OnTurnCompleteMoveToNext:
		nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, currentStep.WorkflowID, currentStep.Position)
		if err != nil {
			s.logger.Warn("failed to get next step by position",
				zap.String("workflow_id", currentStep.WorkflowID),
				zap.Int("current_position", currentStep.Position),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return "", false
		}
		if nextStep == nil {
			s.logger.Debug("no next step found (last step), staying", zap.String("step_name", currentStep.Name))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return "", false
		}
		return nextStep.ID, true
	case wfmodels.OnTurnCompleteMoveToPrevious:
		prevStep, err := s.workflowStepGetter.GetPreviousStepByPosition(ctx, currentStep.WorkflowID, currentStep.Position)
		if err != nil {
			s.logger.Warn("failed to get previous step by position",
				zap.String("workflow_id", currentStep.WorkflowID),
				zap.Int("current_position", currentStep.Position),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return "", false
		}
		if prevStep == nil {
			s.logger.Debug("no previous step found (first step), staying", zap.String("step_name", currentStep.Name))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return "", false
		}
		return prevStep.ID, true
	case wfmodels.OnTurnCompleteMoveToStep:
		var targetStepID string
		if action.Config != nil {
			if sid, ok := action.Config["step_id"].(string); ok {
				targetStepID = sid
			}
		}
		if targetStepID == "" {
			s.logger.Warn("move_to_step action missing step_id config", zap.String("step_id", currentStep.ID))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return "", false
		}
		return targetStepID, true
	}
	return "", false
}

// processOnTurnStart processes the on_turn_start events for the current step.
// This is called when a user sends a message. Returns true if a transition occurred.
func (s *Service) processOnTurnStart(ctx context.Context, taskID, sessionID string) bool {
	if sessionID == "" || s.workflowStepGetter == nil {
		return false
	}

	// Get the session to find its current workflow step
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for on_turn_start",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	if session.WorkflowStepID == nil || *session.WorkflowStepID == "" {
		return false
	}

	workflowStepID := *session.WorkflowStepID

	// Get the current workflow step
	currentStep, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		s.logger.Warn("failed to get workflow step for on_turn_start",
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
		return false
	}

	// If no on_turn_start actions, do nothing
	if len(currentStep.Events.OnTurnStart) == 0 {
		return false
	}

	// Find the first transition action
	var transitionAction *wfmodels.OnTurnStartAction
	for i := range currentStep.Events.OnTurnStart {
		action := &currentStep.Events.OnTurnStart[i]
		switch action.Type {
		case wfmodels.OnTurnStartMoveToNext, wfmodels.OnTurnStartMoveToPrevious, wfmodels.OnTurnStartMoveToStep:
			if transitionAction == nil {
				transitionAction = action
			}
		}
	}

	if transitionAction == nil {
		return false
	}

	// Resolve the target step ID
	targetStepID, ok := s.resolveTurnStartTargetStep(ctx, currentStep, transitionAction)
	if !ok {
		return false
	}

	s.logger.Info("on_turn_start triggered step transition",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", currentStep.Name),
		zap.String("action", string(transitionAction.Type)))

	// Execute the step transition WITHOUT triggering on_enter auto-start
	// (user is about to send a message, the prompt will come from them)
	s.executeStepTransition(ctx, taskID, sessionID, currentStep, targetStepID, false)
	return true
}

// ProcessOnTurnStart is the public API for triggering on_turn_start events.
// Called by message handlers before sending a prompt to the agent.
func (s *Service) ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) error {
	s.processOnTurnStartViaEngine(ctx, taskID, sessionID)
	return nil
}

// executeStepTransition moves a task/session from one step to another.
// If triggerOnEnter is true, on_enter actions (like auto_start_agent) are processed.
// If false, only the step change is applied (used for on_turn_start where the user is about to send a message).
func (s *Service) executeStepTransition(ctx context.Context, taskID, sessionID string, fromStep *wfmodels.WorkflowStep, toStepID string, triggerOnEnter bool) {
	// Process on_exit actions for the step we're leaving (before the step change)
	s.processOnExit(ctx, taskID, sessionID, fromStep)

	// Get the target step
	targetStep, err := s.workflowStepGetter.GetStep(ctx, toStepID)
	if err != nil {
		s.logger.Warn("failed to get target workflow step",
			zap.String("target_step_id", toStepID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	// Get the task to update its workflow step
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to get task for workflow transition",
			zap.String("task_id", taskID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	// Update the task's workflow step
	task.WorkflowStepID = toStepID
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to move task to next workflow step",
			zap.String("task_id", taskID),
			zap.String("from_step", fromStep.Name),
			zap.String("to_step", targetStep.Name),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	// Publish task updated event
	if s.eventBus != nil {
		taskEventData := map[string]interface{}{
			"task_id":          task.ID,
			"workflow_id":      task.WorkflowID,
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

	// Update session's workflow step
	if err := s.repo.UpdateSessionWorkflowStep(ctx, sessionID, toStepID); err != nil {
		s.logger.Warn("failed to update session workflow step",
			zap.String("session_id", sessionID),
			zap.String("step_id", toStepID),
			zap.Error(err))
	}

	// Clear review status when transitioning
	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
		s.logger.Warn("failed to clear session review status",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	s.logger.Info("workflow transition completed",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", fromStep.Name),
		zap.String("to_step", targetStep.Name),
		zap.Bool("trigger_on_enter", triggerOnEnter))
	// Process on_enter for the target step (skip if triggerOnEnter is false,
	// e.g. on_turn_start transitions where the user is about to send a message)
	if triggerOnEnter {
		s.processOnEnter(ctx, taskID, sessionID, targetStep, task.Description)
	} else {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
	}
}

// processOnEnter processes the on_enter events for a step after transitioning to it.
func (s *Service) processOnEnter(ctx context.Context, taskID, sessionID string, step *wfmodels.WorkflowStep, taskDescription string) {
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, sessionID)

	// Check if this step enables plan mode
	hasPlanMode := false
	for _, action := range step.Events.OnEnter {
		if action.Type == wfmodels.OnEnterEnablePlanMode {
			hasPlanMode = true
			break
		}
	}

	// Skip plan mode management for passthrough sessions — the CLI manages its own state.
	// For ACP sessions, auto-disable plan mode when entering a step that doesn't explicitly enable it.
	if !isPassthrough {
		if !hasPlanMode {
			s.clearSessionPlanMode(ctx, sessionID)
		}
	}

	if len(step.Events.OnEnter) == 0 {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID)
		return
	}

	// Process reset_agent_context FIRST — must complete before auto_start_agent.
	// Context reset works for both ACP and passthrough sessions.
	if step.HasOnEnterAction(wfmodels.OnEnterResetAgentContext) {
		if !s.resetAgentContext(ctx, taskID, sessionID, step.Name) {
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID)
			return
		}
	}

	hasAutoStart := false
	for _, action := range step.Events.OnEnter {
		switch action.Type {
		case wfmodels.OnEnterEnablePlanMode:
			// Skip plan mode for passthrough — CLI manages its own state
			if !isPassthrough {
				s.setSessionPlanMode(ctx, sessionID, true)
			}
		case wfmodels.OnEnterAutoStartAgent:
			hasAutoStart = true
		}
	}

	// Skip auto-start for passthrough sessions — stdin is unreliable and the
	// process may not be expecting input. Let the user interact directly.
	if hasAutoStart && !isPassthrough {
		// Build prompt from step configuration
		effectivePrompt := s.buildWorkflowPrompt(taskDescription, step, taskID)
		planMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)

		// Run auto-start inline so queue state is visible before handleAgentReady
		// checks for queued messages.
		err := s.autoStartStepPrompt(ctx, taskID, sessionID, step.Name, effectivePrompt, planMode, true)
		if err != nil {
			s.logger.Error("failed to auto-start agent for step",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
		}
		return
	} else {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID)
	}
}

func (s *Service) autoStartStepPrompt(
	ctx context.Context,
	taskID, sessionID, stepName, prompt string,
	planMode bool,
	shouldQueueIfBusy bool,
) error {
	if shouldQueueIfBusy {
		queued, err := s.queueAutoStartPromptIfRunning(ctx, taskID, sessionID, prompt, planMode)
		if err != nil {
			return err
		}
		if queued {
			return nil
		}
	}

	const maxRetryAttempts = 5
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		_, err := s.PromptTask(ctx, taskID, sessionID, prompt, "", planMode, nil)
		if err == nil {
			return nil
		}

		if !isAgentPromptInProgressError(err) && !isTransientPromptError(err) && !isSessionResetInProgressError(err) {
			return err
		}

		if shouldQueueIfBusy {
			if queueErr := s.queueAutoStartPrompt(ctx, taskID, sessionID, prompt, planMode); queueErr != nil {
				return queueErr
			}
			return nil
		}

		if attempt == maxRetryAttempts {
			return err
		}

		delay := time.Duration(50*(1<<(attempt-1))) * time.Millisecond
		select {
		case <-ctx.Done():
			return fmt.Errorf("auto-start context canceled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return nil
}

func (s *Service) queueAutoStartPromptIfRunning(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
) (bool, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to load session for auto-start queue check: %w", err)
	}
	if session.State != models.TaskSessionStateRunning {
		return false, nil
	}
	if err := s.queueAutoStartPrompt(ctx, taskID, sessionID, prompt, planMode); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) queueAutoStartPrompt(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
) error {
	if s.messageQueue == nil {
		return fmt.Errorf("message queue is not configured")
	}
	_, err := s.messageQueue.QueueMessage(ctx, sessionID, taskID, prompt, "", "workflow-auto-start", planMode, []messagequeue.MessageAttachment{})
	if err != nil {
		return fmt.Errorf("failed to queue workflow auto-start prompt: %w", err)
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	return nil
}

// resetAgentContext restarts the agent subprocess with a fresh ACP session, clearing
// the agent's conversation context. The workspace environment is preserved.
func (s *Service) resetAgentContext(ctx context.Context, taskID, sessionID, stepName string) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for context reset",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	if session.AgentExecutionID == "" {
		s.logger.Debug("no agent execution for context reset, skipping",
			zap.String("session_id", sessionID))
		return true
	}

	s.logger.Info("resetting agent context for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("step_name", stepName),
		zap.String("agent_execution_id", session.AgentExecutionID))

	s.setSessionResetInProgress(sessionID, true)
	defer s.setSessionResetInProgress(sessionID, false)

	if err := s.agentManager.RestartAgentProcess(ctx, session.AgentExecutionID); err != nil {
		s.logger.Error("failed to reset agent context",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName),
			zap.Error(err))
		return false
	}

	// Clear the stored ACP session ID in the database so future resumes use session/new
	if session.Metadata != nil {
		delete(session.Metadata, "acp_session_id")
	}
	if updateErr := s.repo.UpdateTaskSession(ctx, session); updateErr != nil {
		s.logger.Warn("failed to clear ACP session ID from session metadata",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	return true
}

// processOnExit processes the on_exit events for a step when leaving it.
// This is called before transitioning to the next step. Only side-effect actions
// are supported (no transitions — those are decided by on_turn_complete).
func (s *Service) processOnExit(ctx context.Context, taskID, sessionID string, step *wfmodels.WorkflowStep) {
	if len(step.Events.OnExit) == 0 {
		return
	}

	// Skip plan mode management for passthrough sessions — the CLI manages its own state.
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, sessionID)

	for _, action := range step.Events.OnExit {
		if action.Type == wfmodels.OnExitDisablePlanMode && !isPassthrough {
			s.clearSessionPlanMode(ctx, sessionID)
			s.logger.Debug("on_exit: disabled plan mode",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name))
		}
	}
}

// clearSessionPlanMode clears plan mode from session metadata.
func (s *Service) clearSessionPlanMode(ctx context.Context, sessionID string) {
	s.setSessionPlanMode(ctx, sessionID, false)
}

// setSessionPlanMode sets or clears plan mode in session metadata.
func (s *Service) setSessionPlanMode(ctx context.Context, sessionID string, enabled bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to get session for plan mode update",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}

	if enabled {
		session.Metadata["plan_mode"] = true
	} else {
		delete(session.Metadata, "plan_mode")
	}

	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to update session plan mode",
			zap.String("session_id", sessionID),
			zap.Bool("enabled", enabled),
			zap.Error(err))
	}
}

// processTurnCompleteActions processes on_turn_complete actions for a step:
// it executes side-effect actions and returns the first eligible transition action.
func (s *Service) processTurnCompleteActions(ctx context.Context, sessionID string, step *wfmodels.WorkflowStep) *wfmodels.OnTurnCompleteAction {
	var transitionAction *wfmodels.OnTurnCompleteAction
	for i := range step.Events.OnTurnComplete {
		action := &step.Events.OnTurnComplete[i]
		switch action.Type {
		case wfmodels.OnTurnCompleteDisablePlanMode:
			s.clearSessionPlanMode(ctx, sessionID)
		case wfmodels.OnTurnCompleteMoveToNext, wfmodels.OnTurnCompleteMoveToPrevious, wfmodels.OnTurnCompleteMoveToStep:
			if engine.ConfigRequiresApproval(action.Config) {
				continue
			}
			if transitionAction == nil {
				transitionAction = action
			}
		}
	}
	return transitionAction
}

// publishSessionWaitingEvent publishes a session state change event for WAITING_FOR_INPUT.
func (s *Service) publishSessionWaitingEvent(ctx context.Context, taskID, sessionID, stepID string) {
	if s.eventBus == nil {
		return
	}
	eventData := map[string]interface{}{
		"task_id":          taskID,
		"session_id":       sessionID,
		"workflow_step_id": stepID,
		"new_state":        string(models.TaskSessionStateWaitingForInput),
	}
	_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
		events.TaskSessionStateChanged,
		"orchestrator",
		eventData,
	))
}

// resolveTurnStartTargetStep resolves the target step ID for an on_turn_start transition action.
// Returns the step ID and true if resolved; empty string and false if not resolvable.
func (s *Service) resolveTurnStartTargetStep(ctx context.Context, currentStep *wfmodels.WorkflowStep, action *wfmodels.OnTurnStartAction) (string, bool) {
	switch action.Type {
	case wfmodels.OnTurnStartMoveToNext:
		next, err := s.workflowStepGetter.GetNextStepByPosition(ctx, currentStep.WorkflowID, currentStep.Position)
		if err != nil || next == nil {
			return "", false
		}
		return next.ID, true
	case wfmodels.OnTurnStartMoveToPrevious:
		prev, err := s.workflowStepGetter.GetPreviousStepByPosition(ctx, currentStep.WorkflowID, currentStep.Position)
		if err != nil || prev == nil {
			return "", false
		}
		return prev.ID, true
	case wfmodels.OnTurnStartMoveToStep:
		if action.Config != nil {
			if sid, ok := action.Config["step_id"].(string); ok && sid != "" {
				return sid, true
			}
		}
		return "", false
	}
	return "", false
}

// ============================================================================
// Engine-driven workflow methods
// ============================================================================

// processOnTurnCompleteViaEngine uses the workflow engine to evaluate on_turn_complete
// actions and drive step transitions. Falls back to the legacy method when the engine
// is not initialized. Returns true if a step transition occurred.
func (s *Service) processOnTurnCompleteViaEngine(ctx context.Context, taskID, sessionID string) bool {
	if s.workflowEngine == nil {
		return s.processOnTurnComplete(ctx, taskID, sessionID)
	}

	if sessionID == "" || s.workflowStepGetter == nil {
		return false
	}

	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:       taskID,
		SessionID:    sessionID,
		Trigger:      engine.TriggerOnTurnComplete,
		EvaluateOnly: true,
	})
	if err != nil {
		s.logger.Error("workflow engine error on_turn_complete",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return false
	}

	if !result.Transitioned {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return false
	}

	s.logger.Info("engine: on_turn_complete transition",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", result.FromStepID),
		zap.String("to_step_id", result.ToStepID))

	// Retrieve the from-step for on_exit processing.
	fromStep, err := s.workflowStepGetter.GetStep(ctx, result.FromStepID)
	if err != nil {
		s.logger.Warn("failed to load from-step for on_exit",
			zap.String("step_id", result.FromStepID),
			zap.Error(err))
	} else {
		s.processOnExit(ctx, taskID, sessionID, fromStep)
	}

	// Apply the transition in the DB.
	if err := s.workflowStore.ApplyTransition(ctx, taskID, sessionID, result.FromStepID, result.ToStepID, engine.TriggerOnTurnComplete); err != nil {
		s.logger.Error("failed to apply engine transition",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return false
	}

	// Persist data patches from callbacks.
	if len(result.DataPatch) > 0 {
		if err := s.workflowStore.PersistData(ctx, sessionID, result.DataPatch); err != nil {
			s.logger.Warn("failed to persist workflow data patch",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	// Process on_enter for the target step (auto-start, plan mode, context reset).
	targetStep, err := s.workflowStepGetter.GetStep(ctx, result.ToStepID)
	if err != nil {
		s.logger.Warn("failed to load target step for on_enter",
			zap.String("step_id", result.ToStepID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return true
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to load task for on_enter prompt",
			zap.String("task_id", taskID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return true
	}

	s.processOnEnter(ctx, taskID, sessionID, targetStep, task.Description)
	return true
}

// processOnTurnStartViaEngine uses the workflow engine to evaluate on_turn_start
// actions. Falls back to the legacy method when the engine is not initialized.
// Returns true if a step transition occurred.
func (s *Service) processOnTurnStartViaEngine(ctx context.Context, taskID, sessionID string) bool {
	if s.workflowEngine == nil {
		return s.processOnTurnStart(ctx, taskID, sessionID)
	}

	if sessionID == "" || s.workflowStepGetter == nil {
		return false
	}

	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:       taskID,
		SessionID:    sessionID,
		Trigger:      engine.TriggerOnTurnStart,
		EvaluateOnly: true,
	})
	if err != nil {
		s.logger.Error("workflow engine error on_turn_start",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	if !result.Transitioned {
		return false
	}

	s.logger.Info("engine: on_turn_start transition",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", result.FromStepID),
		zap.String("to_step_id", result.ToStepID))

	// Retrieve the from-step for on_exit processing.
	fromStep, err := s.workflowStepGetter.GetStep(ctx, result.FromStepID)
	if err != nil {
		s.logger.Warn("failed to load from-step for on_exit",
			zap.String("step_id", result.FromStepID),
			zap.Error(err))
	} else {
		s.processOnExit(ctx, taskID, sessionID, fromStep)
	}

	// Apply the transition in the DB.
	if err := s.workflowStore.ApplyTransition(ctx, taskID, sessionID, result.FromStepID, result.ToStepID, engine.TriggerOnTurnStart); err != nil {
		s.logger.Error("failed to apply engine transition",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	// on_turn_start does NOT trigger on_enter (user's message is the next prompt).
	s.setSessionWaitingForInput(ctx, taskID, sessionID)
	return true
}

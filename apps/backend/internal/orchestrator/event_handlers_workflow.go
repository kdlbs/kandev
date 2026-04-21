package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// processOnTurnComplete processes the on_turn_complete events for the current step.
// Returns true if a transition occurred (step change happened).
func (s *Service) processOnTurnComplete(ctx context.Context, task *models.Task, session *models.TaskSession) bool {
	if session.ID == "" || s.workflowStepGetter == nil {
		return false
	}

	taskID := task.ID
	sessionID := session.ID

	if task.WorkflowStepID == "" {
		s.logger.Debug("task has no workflow step, skipping transition",
			zap.String("session_id", sessionID))
		return false
	}

	workflowStepID := task.WorkflowStepID

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
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		return false
	}

	// Process side-effect actions first, then find the first transition action
	transitionAction := s.processTurnCompleteActions(ctx, session, currentStep)

	// If no transition action found, just apply side effects and wait
	if transitionAction == nil {
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
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
func (s *Service) processOnTurnStart(ctx context.Context, task *models.Task, session *models.TaskSession) bool {
	if session.ID == "" || s.workflowStepGetter == nil {
		return false
	}

	taskID := task.ID
	sessionID := session.ID

	if task.WorkflowStepID == "" {
		return false
	}

	workflowStepID := task.WorkflowStepID

	// Get the current workflow step
	currentStep, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil || currentStep == nil {
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
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session for on_turn_start: %w", err)
	}
	s.processOnTurnStartViaEngine(ctx, taskID, session)
	return nil
}

// executeStepTransition moves a task/session from one step to another.
// If triggerOnEnter is true, on_enter actions (like auto_start_agent) are processed.
// If false, only the step change is applied (used for on_turn_start where the user is about to send a message).
func (s *Service) executeStepTransition(ctx context.Context, taskID, sessionID string, fromStep *wfmodels.WorkflowStep, toStepID string, triggerOnEnter bool) {
	// Process on_exit actions for the step we're leaving (before the step change).
	// Freshly load the session since the caller may not have it (legacy path).
	exitSession, exitErr := s.repo.GetTaskSession(ctx, sessionID)
	if exitErr != nil {
		s.logger.Warn("failed to load session for on_exit",
			zap.String("session_id", sessionID), zap.Error(exitErr))
	} else {
		s.processOnExit(ctx, taskID, exitSession, fromStep)
	}

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
		_ = s.eventBus.Publish(ctx, events.TaskUpdated, bus.NewEvent(
			events.TaskUpdated,
			"orchestrator",
			buildTaskEventPayload(task),
		))
	}

	s.logger.Info("workflow transition completed",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", fromStep.Name),
		zap.String("to_step", targetStep.Name),
		zap.Bool("trigger_on_enter", triggerOnEnter))

	if triggerOnEnter {
		s.finalizeStepEnter(ctx, taskID, sessionID, targetStep, task.Description)
	} else {
		// on_turn_start transitions: user is about to send a message, no on_enter needed.
		// However, we still need to switch the agent profile if the target step requires
		// a different one — the user's prompt should go to the correct agent.
		currentSession, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to load session for profile switch",
				zap.String("session_id", sessionID), zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID)
			return
		}
		effectiveSession, ok := s.maybySwitchSessionForProfile(ctx, taskID, currentSession, targetStep)
		if !ok {
			return
		}
		s.setSessionWaitingForInput(ctx, taskID, effectiveSession.ID)
	}
}

// handleTaskMoved handles manual task step changes (drag-and-drop, stepper "Move here").
// It processes on_exit for the source step and on_enter for the target step,
// including auto_start_agent, enable_plan_mode, and reset_agent_context.
// When no session exists yet, it checks if the target step has auto_start_agent
// and creates a new session via StartTask if needed.
func (s *Service) handleTaskMoved(ctx context.Context, data watcher.TaskMovedEventData) {
	if data.FromStepID == "" || data.ToStepID == "" {
		s.logger.Debug("task.moved: skipping (missing step IDs)",
			zap.String("task_id", data.TaskID))
		return
	}

	if s.workflowStepGetter == nil {
		return
	}

	// No session yet — check if we need to create one via auto-start
	if data.SessionID == "" {
		s.handleTaskMovedNoSession(ctx, data)
		return
	}

	s.handleTaskMovedWithSession(ctx, data)
}

// handleTaskMovedNoSession handles the case where a task is moved but has no session.
// If the target step has auto_start_agent, it creates a session and starts the agent
// using agent/executor profile IDs from the task's metadata.
func (s *Service) handleTaskMovedNoSession(ctx context.Context, data watcher.TaskMovedEventData) {
	// Load the target step to check auto-start and plan mode flags
	step, err := s.workflowStepGetter.GetStep(ctx, data.ToStepID)
	if err != nil {
		s.logger.Warn("task.moved: failed to load target step",
			zap.String("task_id", data.TaskID),
			zap.String("to_step_id", data.ToStepID),
			zap.Error(err))
		return
	}
	if step == nil || !step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		s.logger.Debug("task.moved: no session and target step has no auto-start",
			zap.String("task_id", data.TaskID),
			zap.String("to_step_id", data.ToStepID))
		return
	}

	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil {
		s.logger.Warn("task.moved: failed to load task for auto-start",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
		return
	}

	agentProfileID := s.resolveStepAgentProfile(ctx, step)
	if agentProfileID == "" {
		agentProfileID, _ = task.Metadata[models.MetaKeyAgentProfileID].(string)
	}
	executorProfileID, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	planMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)

	s.logger.Info("task.moved: starting task (no session, auto-start step)",
		zap.String("task_id", data.TaskID),
		zap.String("to_step_id", data.ToStepID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_profile_id", executorProfileID),
		zap.Bool("plan_mode", planMode))

	_, err = s.StartTask(ctx, task.ID, agentProfileID, "", executorProfileID, 0, task.Description, data.ToStepID, planMode, nil)
	if err != nil {
		s.logger.Error("task.moved: failed to auto-start task",
			zap.String("task_id", data.TaskID),
			zap.Error(err))
	}
}

// handleTaskMovedWithSession handles the case where a task with an existing session
// is moved between steps. It processes on_exit for the source step and on_enter
// for the target step.
func (s *Service) handleTaskMovedWithSession(ctx context.Context, data watcher.TaskMovedEventData) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("task.moved: failed to load session",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	s.processStepExitAndEnter(ctx, data.TaskID, session, data.FromStepID, data.ToStepID, data.TaskDescription)
}

// processStepExitAndEnter runs the on_exit → clear review → reload session → on_enter
// sequence for a step transition. Used by handleTaskMovedWithSession (where MoveTask
// already persisted the step change in the DB).
func (s *Service) processStepExitAndEnter(ctx context.Context, taskID string, session *models.TaskSession, fromStepID, toStepID, taskDescription string) {
	// Process on_exit for the step we're leaving
	fromStep, err := s.workflowStepGetter.GetStep(ctx, fromStepID)
	if err != nil || fromStep == nil {
		s.logger.Warn("failed to load from-step for on_exit",
			zap.String("step_id", fromStepID),
			zap.Error(err))
	} else {
		s.processOnExit(ctx, taskID, session, fromStep)
	}

	targetStep, err := s.workflowStepGetter.GetStep(ctx, toStepID)
	if err != nil || targetStep == nil {
		s.logger.Warn("failed to load target step for on_enter",
			zap.String("step_id", toStepID),
			zap.Error(err))
		return
	}

	s.finalizeStepEnter(ctx, taskID, session.ID, targetStep, taskDescription)
}

// finalizeStepEnter clears review status, reloads the session, and processes on_enter
// actions for the target step. Shared by executeStepTransition and processStepExitAndEnter.
func (s *Service) finalizeStepEnter(ctx context.Context, taskID, sessionID string, targetStep *wfmodels.WorkflowStep, taskDescription string) {
	// Clear review status when moving to a new step
	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
		s.logger.Warn("failed to clear session review status",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Reload session after on_exit may have changed metadata
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load session for on_enter",
			zap.String("session_id", sessionID), zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	s.processOnEnter(ctx, taskID, session, targetStep, taskDescription)
}

// resolveStepPlanMode determines whether plan mode should be active for a step.
// Returns false for passthrough sessions, steps without enable_plan_mode, or when the agent
// doesn't support MCP. Plan mode is only cleared by explicit on_exit/on_turn_complete
// disable_plan_mode actions, not automatically when entering a non-plan-mode step.
// This preserves user-initiated plan mode across workflow transitions.
func (s *Service) resolveStepPlanMode(ctx context.Context, session *models.TaskSession, step *wfmodels.WorkflowStep, isPassthrough bool) bool {
	hasPlanMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)

	// Plan mode requires MCP support.
	if hasPlanMode && !s.resolveSessionMCPSupport(ctx, session) {
		s.logger.Warn("skipping plan mode for step: agent does not support MCP",
			zap.String("session_id", session.ID),
			zap.String("step_id", step.ID))
		hasPlanMode = false
	}

	return hasPlanMode
}

// resolveStepAgentProfile returns the effective agent profile ID for a step.
// Resolution order: step override -> workflow default -> empty (use current session's profile).
func (s *Service) resolveStepAgentProfile(ctx context.Context, step *wfmodels.WorkflowStep) string {
	if step.AgentProfileID != "" {
		return step.AgentProfileID
	}
	if s.workflowStepGetter != nil && step.WorkflowID != "" {
		wfProfileID, err := s.workflowStepGetter.GetWorkflowAgentProfileID(ctx, step.WorkflowID)
		if err != nil {
			s.logger.Warn("failed to resolve workflow agent profile, falling back to task defaults",
				zap.String("workflow_id", step.WorkflowID),
				zap.String("step_id", step.ID),
				zap.Error(err))
		} else if wfProfileID != "" {
			return wfProfileID
		}
	}
	return ""
}

// switchSessionForStep stops the current session and creates a new one with a different agent profile.
// Returns the new session, or nil + error if the switch fails.
// The new session is prepared BEFORE completing the old one: if PrepareSession fails, the old
// session remains active and the task stays recoverable.
func (s *Service) switchSessionForStep(ctx context.Context, taskID string, currentSession *models.TaskSession, newAgentProfileID string) (*models.TaskSession, error) {
	s.logger.Info("switching session for workflow step agent profile change",
		zap.String("task_id", taskID),
		zap.String("current_session", currentSession.ID),
		zap.String("current_profile", currentSession.AgentProfileID),
		zap.String("new_profile", newAgentProfileID))

	// Signal to the frontend that the task is preparing a new agent.
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to set task SCHEDULING during agent switch",
			zap.String("task_id", taskID), zap.Error(err))
	}

	// Prepare the new session BEFORE touching the old one.
	// If any step below fails, the old session remains active and the task stays recoverable.
	task, err := s.scheduler.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task for session switch: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %s not found for session switch", taskID)
	}
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get db task for session switch: %w", err)
	}

	// Create a new session with the new agent profile.
	// Reuse the same executor profile from the current session.
	sessionID, err := s.executor.PrepareSession(ctx, task, newAgentProfileID, currentSession.ExecutorID, currentSession.ExecutorProfileID, dbTask.WorkflowStepID)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare new session: %w", err)
	}

	newSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get new session: %w", err)
	}

	// Inherit the task environment from the old session — the workspace is shared
	// across sessions within the same task, so the new session can reuse the
	// existing agentctl connection and workspace files.
	if currentSession.TaskEnvironmentID != "" && newSession.TaskEnvironmentID == "" {
		newSession.TaskEnvironmentID = currentSession.TaskEnvironmentID
		newSession.UpdatedAt = time.Now().UTC()
		if err := s.repo.UpdateTaskSession(ctx, newSession); err != nil {
			s.logger.Warn("failed to copy task_environment_id to new session",
				zap.String("session_id", newSession.ID),
				zap.Error(err))
		}
	}

	// Promote the new session to primary so it's loaded when navigating back to this task.
	if err := s.repo.SetSessionPrimary(ctx, newSession.ID); err != nil {
		s.logger.Warn("failed to set new session as primary",
			zap.String("session_id", newSession.ID), zap.Error(err))
	}

	// New session is ready — now safe to stop the old agent and complete the old session.
	if currentSession.AgentExecutionID != "" {
		if err := s.agentManager.StopAgent(ctx, currentSession.AgentExecutionID, false); err != nil {
			s.logger.Warn("failed to stop agent for session switch",
				zap.String("session_id", currentSession.ID),
				zap.Error(err))
		}
	}

	// Mark the current session as completed.
	currentSession.State = models.TaskSessionStateCompleted
	now := time.Now().UTC()
	currentSession.CompletedAt = &now
	currentSession.UpdatedAt = now
	if err := s.repo.UpdateTaskSession(ctx, currentSession); err != nil {
		// New session is already created; log but don't abort.
		s.logger.Warn("failed to complete old session after switch",
			zap.String("session_id", currentSession.ID),
			zap.Error(err))
	}

	return newSession, nil
}

// maybySwitchSessionForProfile checks whether the step requires a different agent profile
// and switches the session if so. Passthrough sessions are returned unchanged.
// Returns the effective session (new or original) and whether processing should continue.
// A false return means the switch failed; the caller should return immediately.
func (s *Service) maybySwitchSessionForProfile(
	ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep,
) (*models.TaskSession, bool) {
	if s.agentManager.IsPassthroughSession(ctx, session.ID) {
		return session, true
	}
	effectiveProfile := s.resolveStepAgentProfile(ctx, step)
	if effectiveProfile == "" || effectiveProfile == session.AgentProfileID {
		return session, true
	}
	newSession, err := s.switchSessionForStep(ctx, taskID, session, effectiveProfile)
	if err != nil {
		s.logger.Error("failed to switch session for step agent profile",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return nil, false
	}
	return newSession, true
}

// processOnEnter processes the on_enter events for a step after transitioning to it.
func (s *Service) processOnEnter(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep, taskDescription string) {
	// Switch session if this step requires a different agent profile.
	var ok bool
	prevSessionID := session.ID
	if session, ok = s.maybySwitchSessionForProfile(ctx, taskID, session, step); !ok {
		return
	}
	sessionSwitched := session.ID != prevSessionID
	sessionID := session.ID
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, sessionID)

	hasPlanMode := s.resolveStepPlanMode(ctx, session, step, isPassthrough)

	if len(step.Events.OnEnter) == 0 && !sessionSwitched {
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
		return
	}

	// Process reset_agent_context FIRST — must complete before auto_start_agent.
	// Context reset works for both ACP and passthrough sessions.
	if step.HasOnEnterAction(wfmodels.OnEnterResetAgentContext) {
		if !s.resetAgentContext(ctx, taskID, session, step.Name) {
			s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
			s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
			return
		}
		s.markIdleAfterReset(ctx, taskID, sessionID, session, step, isPassthrough)
	}

	hasAutoStart := false
	for _, action := range step.Events.OnEnter {
		switch action.Type {
		case wfmodels.OnEnterEnablePlanMode:
			// Skip plan mode for passthrough — CLI manages its own state.
			// Also skip if agent doesn't support MCP (hasPlanMode is already false above).
			if !isPassthrough && hasPlanMode {
				s.setSessionPlanMode(ctx, session, true)
			}
		case wfmodels.OnEnterAutoStartAgent:
			hasAutoStart = true
		}
	}

	switch {
	case hasAutoStart && isPassthrough:
		// Passthrough path: write prompt directly to PTY stdin.
		// By the time processOnEnter runs (from an on_turn_complete transition),
		// the agent has finished its previous turn and the PTY is waiting for input.
		effectivePrompt := s.buildWorkflowPrompt(taskDescription, step, taskID, sessionID)
		if err := s.autoStartPassthroughPrompt(ctx, taskID, session, step.Name, effectivePrompt); err != nil {
			s.logger.Error("failed to auto-start passthrough agent for step",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
			s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
		}

	case hasAutoStart:
		// ACP path: build prompt from step configuration.
		// Run auto-start inline so queue state is visible before handleAgentReady
		// checks for queued messages.
		effectivePrompt := s.buildWorkflowPrompt(taskDescription, step, taskID, sessionID)
		planMode := hasPlanMode
		err := s.autoStartStepPrompt(ctx, taskID, session, step.Name, effectivePrompt, planMode, true)
		if err != nil {
			s.logger.Error("failed to auto-start agent for step",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		}

	default:
		// When the session was just switched (agent profile change) but the step
		// has no auto_start_agent, launch the agent anyway — the profile override
		// implies the user wants this agent to run on this step.
		if sessionSwitched && step.Prompt != "" {
			effectivePrompt := s.buildWorkflowPrompt(taskDescription, step, taskID, sessionID)
			planMode := hasPlanMode
			s.logger.Info("auto-launching agent after profile switch (no explicit auto_start)",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name))
			err := s.autoStartStepPrompt(ctx, taskID, session, step.Name, effectivePrompt, planMode, true)
			if err != nil {
				s.logger.Error("failed to launch agent after profile switch",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(err))
				s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
				s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
			}
			return
		}
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
	}
}

// deliverPassthroughPrompt writes a prompt to PTY stdin and marks the session as running.
// Appends \r (carriage return) to simulate pressing Enter — TUI agents in raw terminal mode
// expect CR, not LF, as the submit key. Returns an error only if writing fails.
func (s *Service) deliverPassthroughPrompt(ctx context.Context, sessionID, content string) error {
	if err := s.agentManager.WritePassthroughStdin(ctx, sessionID, content+"\r"); err != nil {
		return fmt.Errorf("write to passthrough stdin: %w", err)
	}
	if err := s.agentManager.MarkPassthroughRunning(sessionID); err != nil {
		s.logger.Warn("failed to mark passthrough as running",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	return nil
}

// autoStartPassthroughPrompt writes a workflow prompt to the PTY stdin of a
// passthrough session and marks it as running. TUI agents read stdin line-by-line;
// the idle timeout fires when output stops, triggering turn complete.
func (s *Service) autoStartPassthroughPrompt(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	stepName, prompt string,
) error {
	if err := s.deliverPassthroughPrompt(ctx, session.ID, prompt); err != nil {
		return err
	}
	s.logger.Info("auto-start: wrote prompt to passthrough stdin",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("step_name", stepName))
	return nil
}

func (s *Service) autoStartStepPrompt(
	ctx context.Context,
	taskID string, session *models.TaskSession, stepName, prompt string,
	planMode bool,
	shouldQueueIfBusy bool,
) error {
	sessionID := session.ID

	if shouldQueueIfBusy {
		queued, err := s.queueAutoStartPromptIfRunning(ctx, taskID, session, prompt, planMode)
		if err != nil {
			return err
		}
		if queued {
			return nil
		}
	}

	// Record a user message so the auto-start prompt is visible in chat history.
	s.recordAutoStartMessage(ctx, taskID, sessionID, prompt, planMode)

	// If the session is in CREATED state, the agent was never started (e.g. workspace-only
	// preparation from a blocked auto-start). PromptTask will reject CREATED sessions,
	// so use StartCreatedSession which properly launches the agent on the prepared workspace.
	// Pass skipMessageRecord=true since recordAutoStartMessage above already recorded it.
	if session.State == models.TaskSessionStateCreated {
		s.logger.Info("auto-start: session is CREATED, launching agent via StartCreatedSession",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName))
		_, err := s.StartCreatedSession(ctx, taskID, sessionID, session.AgentProfileID, prompt, true, planMode, nil)
		return err
	}

	const maxRetryAttempts = 5
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		_, err := s.PromptTask(ctx, taskID, sessionID, prompt, "", planMode, nil)
		if err == nil {
			return nil
		}

		// "already has an agent running" means the execution store still tracks
		// an active agent for this session (e.g. session state is CREATED but
		// the agent was launched by a concurrent path). Queue instead of retrying.
		if isAgentAlreadyRunningError(err) && shouldQueueIfBusy {
			if queueErr := s.queueAutoStartPrompt(ctx, taskID, sessionID, prompt, planMode); queueErr != nil {
				return queueErr
			}
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

// recordAutoStartMessage creates a user message for a workflow auto-start prompt
// so it appears in the chat history. The prompt content includes system-injected
// tags which are stripped when displayed to users via ToAPI().
func (s *Service) recordAutoStartMessage(ctx context.Context, taskID, sessionID, prompt string, planMode bool) {
	if s.messageCreator == nil || prompt == "" {
		return
	}
	turnID := s.getActiveTurnID(sessionID)
	if turnID == "" {
		s.startTurnForSession(ctx, sessionID)
		turnID = s.getActiveTurnID(sessionID)
	}
	meta := NewUserMessageMeta().WithPlanMode(planMode)
	metaMap := meta.ToMap()
	if metaMap == nil {
		metaMap = make(map[string]interface{})
	}
	metaMap["workflow_auto_start"] = true
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, metaMap); err != nil {
		s.logger.Error("failed to create auto-start user message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

func (s *Service) queueAutoStartPromptIfRunning(
	ctx context.Context,
	taskID string, session *models.TaskSession, prompt string,
	planMode bool,
) (bool, error) {
	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		return false, nil
	}
	if err := s.queueAutoStartPrompt(ctx, taskID, session.ID, prompt, planMode); err != nil {
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

// markIdleAfterReset flips a freshly-reset session to WAITING_FOR_INPUT so a
// following auto_start_agent sends the prompt directly instead of queueing
// against a stale RUNNING state. processOnEnter runs from handleAgentReady,
// which loads the session before the turn finishes — the in-memory pointer
// still reads RUNNING even though the agent is now idle. Without this flip,
// queueAutoStartPromptIfRunning queues the message and PromptTask later
// rejects the drained queued send because the DB row also still reads RUNNING.
//
// Skip the flip when:
//   - state was not RUNNING/STARTING (e.g. CREATED, where resetAgentContext
//     early-returns true without restarting and autoStartStepPrompt routes
//     the prompt through StartCreatedSession);
//   - the session is passthrough AND auto_start_agent will write to PTY stdin
//     next (the agent is actively processing, not idle).
//
// Uses updateTaskSessionState directly rather than setSessionWaitingForInput
// because the helper would also flip the task to TaskStateReview, which would
// be wrong here — auto_start_agent runs next and should leave the task as
// IN_PROGRESS.
func (s *Service) markIdleAfterReset(
	ctx context.Context,
	taskID, sessionID string,
	session *models.TaskSession,
	step *wfmodels.WorkflowStep,
	isPassthrough bool,
) {
	if session.State != models.TaskSessionStateRunning &&
		session.State != models.TaskSessionStateStarting {
		return
	}
	if isPassthrough && step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		return
	}
	s.updateTaskSessionState(ctx, taskID, sessionID, models.TaskSessionStateWaitingForInput, "", false, session)
	session.State = models.TaskSessionStateWaitingForInput
}

// resetAgentContext restarts the agent subprocess with a fresh ACP session, clearing
// the agent's conversation context. The workspace environment is preserved.
func (s *Service) resetAgentContext(ctx context.Context, taskID string, session *models.TaskSession, stepName string) bool {
	sessionID := session.ID

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

	if err := s.agentManager.ResetAgentContext(ctx, session.AgentExecutionID); err != nil {
		s.logger.Error("failed to reset agent context",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName),
			zap.Error(err))
		return false
	}

	// Clear the stored ACP session ID using json_set to avoid clobbering other keys.
	if updateErr := s.repo.SetSessionMetadataKey(ctx, sessionID, "acp_session_id", ""); updateErr != nil {
		s.logger.Warn("failed to clear ACP session ID from session metadata",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	return true
}

// resolveSessionMCPSupport checks if the agent for a session supports MCP.
// Returns true by default when the profile cannot be resolved (e.g. no profile ID set)
// so that plan mode is not blocked unnecessarily.
func (s *Service) resolveSessionMCPSupport(ctx context.Context, session *models.TaskSession) bool {
	if session.AgentProfileID == "" {
		return true
	}
	profileInfo, err := s.agentManager.ResolveAgentProfile(ctx, session.AgentProfileID)
	if err != nil {
		s.logger.Warn("failed to resolve agent profile for MCP check",
			zap.String("session_id", session.ID),
			zap.String("profile_id", session.AgentProfileID),
			zap.Error(err))
		return true
	}
	return profileInfo.SupportsMCP
}

// processOnExit processes the on_exit events for a step when leaving it.
// This is called before transitioning to the next step. Only side-effect actions
// are supported (no transitions — those are decided by on_turn_complete).
func (s *Service) processOnExit(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep) {
	if len(step.Events.OnExit) == 0 {
		return
	}

	// Skip plan mode management for passthrough sessions — the CLI manages its own state.
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, session.ID)

	for _, action := range step.Events.OnExit {
		if action.Type == wfmodels.OnExitDisablePlanMode && !isPassthrough {
			s.clearSessionPlanMode(ctx, session)
			s.logger.Debug("on_exit: disabled plan mode",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("step_name", step.Name))
		}
	}
}

// clearSessionPlanMode clears plan mode from session metadata.
func (s *Service) clearSessionPlanMode(ctx context.Context, session *models.TaskSession) {
	s.setSessionPlanMode(ctx, session, false)
}

// setSessionPlanMode sets or clears plan mode in session metadata.
// Uses targeted metadata update to avoid overwriting other session fields.
func (s *Service) setSessionPlanMode(ctx context.Context, session *models.TaskSession, enabled bool) {
	// Update in-memory struct for callers that read session.Metadata.
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	if enabled {
		session.Metadata["plan_mode"] = true
	} else {
		delete(session.Metadata, "plan_mode")
	}
	// Persist using json_set to atomically set one key without clobbering others.
	if err := s.repo.SetSessionMetadataKey(ctx, session.ID, "plan_mode", enabled); err != nil {
		s.logger.Warn("failed to update session plan mode",
			zap.String("session_id", session.ID),
			zap.Bool("enabled", enabled),
			zap.Error(err))
	}
}

// processTurnCompleteActions processes on_turn_complete actions for a step:
// it executes side-effect actions and returns the first eligible transition action.
func (s *Service) processTurnCompleteActions(ctx context.Context, session *models.TaskSession, step *wfmodels.WorkflowStep) *wfmodels.OnTurnCompleteAction {
	var transitionAction *wfmodels.OnTurnCompleteAction
	for i := range step.Events.OnTurnComplete {
		action := &step.Events.OnTurnComplete[i]
		switch action.Type {
		case wfmodels.OnTurnCompleteDisablePlanMode:
			s.clearSessionPlanMode(ctx, session)
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
// An optional preloaded session avoids re-reading from DB (which can miss recent writes
// on the read-only WAL connection).
func (s *Service) publishSessionWaitingEvent(ctx context.Context, taskID, sessionID, stepID string, preloadedSession ...*models.TaskSession) {
	if s.eventBus == nil {
		return
	}
	eventData := map[string]interface{}{
		"task_id":          taskID,
		"session_id":       sessionID,
		"workflow_step_id": stepID,
		"new_state":        string(models.TaskSessionStateWaitingForInput),
	}
	// Include agent_profile_id and session metadata so the frontend can
	// identify the agent (e.g. MCP support) without waiting for SSR hydration.
	var session *models.TaskSession
	if len(preloadedSession) > 0 && preloadedSession[0] != nil {
		session = preloadedSession[0]
	} else if s, err := s.repo.GetTaskSession(ctx, sessionID); err == nil {
		session = s
	}
	if session != nil {
		if session.AgentProfileID != "" {
			eventData["agent_profile_id"] = session.AgentProfileID
		}
		if session.TaskEnvironmentID != "" {
			eventData["task_environment_id"] = session.TaskEnvironmentID
		}
		if len(session.Metadata) > 0 {
			eventData["session_metadata"] = session.Metadata
		}
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

// buildMachineState builds an engine.MachineState from pre-loaded session and task objects,
// avoiding redundant DB reads in the workflow engine.
func (s *Service) buildMachineState(ctx context.Context, task *models.Task, session *models.TaskSession) engine.MachineState {
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, session.ID)
	return assembleMachineState(task, session, isPassthrough)
}

// assembleMachineState creates an engine.MachineState from pre-loaded models.
// Shared by Service.buildMachineState and workflowStore.LoadState to avoid duplication.
func assembleMachineState(task *models.Task, session *models.TaskSession, isPassthrough bool) engine.MachineState {
	currentStepID := task.WorkflowStepID
	var data map[string]any
	if session.Metadata != nil {
		if wd, ok := session.Metadata["workflow_data"].(map[string]any); ok {
			data = wd
		}
	}
	return engine.MachineState{
		TaskID:          task.ID,
		SessionID:       session.ID,
		WorkflowID:      task.WorkflowID,
		CurrentStepID:   currentStepID,
		SessionState:    string(session.State),
		TaskDescription: task.Description,
		IsPassthrough:   isPassthrough,
		Data:            data,
	}
}

// processOnTurnCompleteViaEngine uses the workflow engine to evaluate on_turn_complete
// actions and drive step transitions. Falls back to the legacy method when the engine
// is not initialized. Returns true if a step transition occurred.
func (s *Service) processOnTurnCompleteViaEngine(ctx context.Context, taskID string, session *models.TaskSession) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to load task for on_turn_complete",
			zap.String("task_id", taskID), zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	if s.workflowEngine == nil {
		return s.processOnTurnComplete(ctx, task, session)
	}

	if session.ID == "" || s.workflowStepGetter == nil {
		return false
	}

	if task.WorkflowStepID == "" {
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	// Skip workflow step actions for ephemeral tasks (quick chat) - they have no workflow
	if task.IsEphemeral {
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	state := s.buildMachineState(ctx, task, session)
	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:         taskID,
		SessionID:      session.ID,
		Trigger:        engine.TriggerOnTurnComplete,
		EvaluateOnly:   true,
		PreloadedState: &state,
	})
	if err != nil {
		s.logger.Error("workflow engine error on_turn_complete",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	if !result.Transitioned {
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	s.logger.Info("engine: on_turn_complete transition",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("from_step_id", result.FromStepID),
		zap.String("to_step_id", result.ToStepID))

	return s.applyEngineTransition(ctx, taskID, session, result, engine.TriggerOnTurnComplete, task.Description, true)
}

// applyEngineTransition applies an engine-evaluated transition: on_exit, DB transition,
// data patches, and optionally on_enter processing. Returns true if the transition was applied.
func (s *Service) applyEngineTransition(
	ctx context.Context, taskID string, session *models.TaskSession,
	result engine.HandleResult, trigger engine.Trigger, taskDescription string,
	triggerOnEnter bool,
) bool {
	// Validate the target step exists BEFORE persisting the transition.
	// This prevents the task from being moved to an invalid step_id
	// (e.g., a template-level alias like "review" that doesn't resolve to a real UUID).
	var targetStep *wfmodels.WorkflowStep
	if triggerOnEnter {
		var err error
		targetStep, err = s.workflowStepGetter.GetStep(ctx, result.ToStepID)
		if err != nil {
			s.logger.Warn("target step not found, skipping transition",
				zap.String("step_id", result.ToStepID),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
			return false
		}
	} else {
		// Even without on_enter, load the target step — needed for profile switch check.
		var err error
		targetStep, err = s.workflowStepGetter.GetStep(ctx, result.ToStepID)
		if err != nil {
			s.logger.Warn("target step not found, skipping transition",
				zap.String("step_id", result.ToStepID),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
			return false
		}
	}

	fromStep, err := s.workflowStepGetter.GetStep(ctx, result.FromStepID)
	if err != nil {
		s.logger.Warn("failed to load from-step for on_exit",
			zap.String("step_id", result.FromStepID),
			zap.Error(err))
	} else {
		s.processOnExit(ctx, taskID, session, fromStep)
	}

	if err := s.workflowStore.ApplyTransition(ctx, taskID, session.ID, result.FromStepID, result.ToStepID, trigger); err != nil {
		s.logger.Error("failed to apply engine transition",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	if len(result.DataPatch) > 0 {
		if err := s.workflowStore.PersistData(ctx, session.ID, result.DataPatch); err != nil {
			s.logger.Warn("failed to persist workflow data patch",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}

	if !triggerOnEnter {
		// on_turn_start transitions: user is about to send a message, no on_enter needed.
		// However, we still need to switch the agent profile if the target step requires
		// a different one — the user's prompt should go to the correct agent.
		effectiveSession, ok := s.maybySwitchSessionForProfile(ctx, taskID, session, targetStep)
		if !ok {
			return false
		}
		s.setSessionWaitingForInput(ctx, taskID, effectiveSession.ID)
		return true
	}

	s.processOnEnter(ctx, taskID, session, targetStep, taskDescription)
	return true
}

// processOnTurnStartViaEngine uses the workflow engine to evaluate on_turn_start
// actions. Falls back to the legacy method when the engine is not initialized.
// Returns true if a step transition occurred.
func (s *Service) processOnTurnStartViaEngine(ctx context.Context, taskID string, session *models.TaskSession) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to load task for on_turn_start",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}

	if s.workflowEngine == nil {
		return s.processOnTurnStart(ctx, task, session)
	}

	if session.ID == "" || s.workflowStepGetter == nil {
		return false
	}

	if task.WorkflowStepID == "" {
		return false
	}

	// Skip workflow step actions for ephemeral tasks (quick chat) - they have no workflow
	if task.IsEphemeral {
		return false
	}

	state := s.buildMachineState(ctx, task, session)
	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:         taskID,
		SessionID:      session.ID,
		Trigger:        engine.TriggerOnTurnStart,
		EvaluateOnly:   true,
		PreloadedState: &state,
	})
	if err != nil {
		s.logger.Error("workflow engine error on_turn_start",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return false
	}

	if !result.Transitioned {
		return false
	}

	s.logger.Info("engine: on_turn_start transition",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("from_step_id", result.FromStepID),
		zap.String("to_step_id", result.ToStepID))

	// on_turn_start does NOT trigger on_enter (user's message is the next prompt).
	return s.applyEngineTransition(ctx, taskID, session, result, engine.TriggerOnTurnStart, "", false)
}

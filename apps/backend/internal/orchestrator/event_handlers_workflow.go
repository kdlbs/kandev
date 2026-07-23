package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/sysprompt"
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

	if s.turnCompleteBlockedByUserInput(ctx, taskID, sessionID, session) {
		return false
	}

	// ADR 0015 — explicit completion signal gating (legacy path mirror of
	// processOnTurnCompleteViaEngine). Steps marked
	// `auto_advance_requires_signal=true` wait for a step_complete_kandev
	// signal before evaluating their transition actions.
	if currentStep.AutoAdvanceRequiresSignal {
		signal, has := models.LoadPendingStepSignal(session.Metadata)
		if !has || signal.StepID != currentStep.ID {
			s.logger.Debug("on_turn_complete gated on explicit signal (legacy path)",
				zap.String("step_id", currentStep.ID))
			s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
			return false
		}
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
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session for on_turn_start: %w", err)
	}
	if isTerminalSessionState(session.State) {
		return &executor.SessionStateSupersededError{
			SessionID: session.ID,
			State:     session.State,
		}
	}
	// ADR 0015 — a fresh user message before the pending signal's
	// transition has fired cancels the signal (re-open semantics). The
	// user is continuing the conversation; this step is no longer "done".
	s.clearPendingStepSignal(ctx, session)
	s.processOnTurnStartViaEngine(ctx, taskID, session)
	return nil
}

// executeStepTransition moves a task/session from one step to another.
// If triggerOnEnter is true, on_enter actions (like auto_start_agent) are processed.
// If false, only the step change is applied (used for on_turn_start where the user is about to send a message).
func (s *Service) executeStepTransition(ctx context.Context, taskID, sessionID string, fromStep *wfmodels.WorkflowStep, toStepID string, triggerOnEnter bool) {
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
	if err := s.validateTransitionWIPLimit(ctx, task, targetStep); err != nil {
		s.logger.Warn("workflow transition rejected by WIP limit",
			zap.String("task_id", taskID),
			zap.String("to_step_id", toStepID),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	// Process on_exit actions for the step we're leaving (before the step change).
	// Freshly load the session since the caller may not have it (legacy path).
	exitSession, exitErr := s.repo.GetTaskSession(ctx, sessionID)
	if exitErr != nil {
		s.logger.Warn("failed to load session for on_exit",
			zap.String("session_id", sessionID), zap.Error(exitErr))
	} else {
		s.processOnExit(ctx, taskID, exitSession, fromStep)
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

	// Publish task updated event via the task service so the payload carries
	// the full context (session counts, primary session, repositories).
	s.publishTaskUpdated(ctx, task)
	s.processParentChildrenCompletedForTerminalStepMove(ctx, taskID, toStepID)

	s.logger.Info("workflow transition completed",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", fromStep.Name),
		zap.String("to_step", targetStep.Name),
		zap.Bool("trigger_on_enter", triggerOnEnter))

	if s.workflowStore != nil {
		s.workflowStore.pullNextTaskOnVacate(ctx, fromStep.ID, taskID)
	}

	if triggerOnEnter {
		// ADR 0015 — clear any pending completion-signal bag for the
		// step we just left. Only on_turn_complete transitions trigger
		// gating, so the triggerOnEnter=true branch is the only one
		// that could have consumed a signal; on_turn_start moves leave
		// the bag alone (it's still tied to an unsignaled step we have
		// not left). The session struct isn't used after this point,
		// so skip the extra GetTaskSession round-trip and write
		// straight to the DB by session_id.
		s.clearPendingStepSignalByID(ctx, sessionID)
		// Automated transitions always clear review: the agent just completed
		// a turn, so any pending review from a prior step is stale regardless
		// of whether the new step has auto_start_agent. Match the engine path's
		// asynchronous on_enter dispatch: terminal-event handlers own the
		// session cancel guard through this transition, and inline auto-start
		// would re-enter that non-reentrant guard from PromptTask.
		go s.finalizeStepEnter(
			context.WithoutCancel(ctx),
			taskID,
			sessionID,
			targetStep,
			task.Description,
			true,
		)
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

func (s *Service) validateTransitionWIPLimit(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil || targetStep.WIPLimit <= 0 || task.WorkflowStepID == targetStep.ID {
		return nil
	}
	limitsRepo, ok := s.repo.(workflowMoveLimitsRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, targetStep.ID, task.ID)
	if err != nil {
		return fmt.Errorf("count target workflow step tasks: %w", err)
	}
	if occupants >= targetStep.WIPLimit {
		return fmt.Errorf("WIP limit exceeded for workflow step %s: limit %d already occupied", targetStep.ID, targetStep.WIPLimit)
	}
	return nil
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

	s.processParentChildrenCompletedForTerminalStepMove(ctx, data.TaskID, data.ToStepID)

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

	workflowAgentProfileID := s.resolveStepAgentProfile(ctx, step)
	agentProfileID := workflowAgentProfileID
	if agentProfileID == "" {
		agentProfileID, _ = task.Metadata[models.MetaKeyAgentProfileID].(string)
	}
	executorID, _ := task.Metadata[models.MetaKeyExecutorID].(string)
	executorProfileID, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	planMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)

	s.logger.Info("task.moved: starting task (no session, auto-start step)",
		zap.String("task_id", data.TaskID),
		zap.String("to_step_id", data.ToStepID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.String("executor_profile_id", executorProfileID),
		zap.Bool("plan_mode", planMode))

	// Async: event bus delivers synchronously; blocking here → HTTP timeout (see handleTaskMovedWithSession doc).
	go func() {
		asyncCtx := context.WithoutCancel(ctx)
		startAgentProfileID := agentProfileID
		if workflowAgentProfileID != "" {
			startAgentProfileID = ""
		}
		_, err := s.StartTask(asyncCtx, task.ID, startAgentProfileID, executorID, executorProfileID, "", task.Description, data.ToStepID, planMode, true, nil)
		if err != nil {
			s.logger.Error("task.moved: failed to auto-start task",
				zap.String("task_id", data.TaskID),
				zap.Error(err))
		}
	}()
}

// handleTaskMovedWithSession handles the case where a task with an existing session
// is moved between steps. It processes on_exit for the source step and on_enter
// for the target step.
//
// The on_exit/on_enter processing is launched asynchronously because this handler
// runs synchronously inside the in-memory event bus Publish call. If processOnEnter
// blocks (e.g., auto_start_agent waiting for the agent turn), the MoveTask HTTP
// handler that published the event also blocks, causing browser request timeouts.
func (s *Service) handleTaskMovedWithSession(ctx context.Context, data watcher.TaskMovedEventData) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("task.moved: failed to load session",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}

	go s.processStepExitAndEnter(context.WithoutCancel(ctx), data.TaskID, session, data.FromStepID, data.ToStepID, data.TaskDescription)
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

	clearReview := targetStep.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)
	s.finalizeStepEnter(ctx, taskID, session.ID, targetStep, taskDescription, clearReview)
}

// finalizeStepEnter optionally clears review status, reloads the session, and
// processes on_enter actions for the target step. Shared by executeStepTransition
// and processStepExitAndEnter.
func (s *Service) finalizeStepEnter(ctx context.Context, taskID, sessionID string, targetStep *wfmodels.WorkflowStep, taskDescription string, clearReview bool) {
	if clearReview {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
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

// tagSessionAsWorkflowSwitched records that a session's profile came from a
// workflow step override rather than direct user selection. Uses the atomic
// SetSessionMetadataKey (json_set) so other metadata keys are preserved.
func (s *Service) tagSessionAsWorkflowSwitched(ctx context.Context, sessionID string) {
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyCreatedBy, models.SessionCreatedByWorkflowSwitch); err != nil {
		s.logger.Warn("failed to persist workflow-switch tag",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

// switchSessionForStep activates a session for the new agent profile.
// If an existing session on this task already uses the target profile it is
// reused (re-promoted to primary, brought out of COMPLETED if it had been
// switched away from previously). Otherwise a new session is prepared.
// In both cases the previous session is stopped and marked COMPLETED.
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

	existing, lookupErr := s.findReusableSessionForProfile(ctx, taskID, newAgentProfileID, currentSession.ID)
	if lookupErr != nil {
		s.logger.Warn("failed to look up reusable session, falling through to create new",
			zap.String("task_id", taskID),
			zap.String("agent_profile_id", newAgentProfileID),
			zap.Error(lookupErr))
	}
	if existing != nil {
		return s.reuseSessionForStep(ctx, taskID, currentSession, existing)
	}

	return s.createNewSessionForStep(ctx, taskID, currentSession, newAgentProfileID)
}

// findReusableSessionForProfile returns the most-recently-updated session on
// this task that uses the target profile (and is not the session being
// switched away from), or nil if none exists. Failed/cancelled sessions are
// excluded — those are dead and shouldn't be revived implicitly.
func (s *Service) findReusableSessionForProfile(ctx context.Context, taskID, profileID, excludeSessionID string) (*models.TaskSession, error) {
	if profileID == "" {
		return nil, nil
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var best *models.TaskSession
	for _, sess := range sessions {
		if sess.ID == excludeSessionID {
			continue
		}
		if sess.AgentProfileID != profileID {
			continue
		}
		// Skip user-cancelled sessions — those are explicit stops and
		// shouldn't be auto-revived. FAILED sessions are reused (the failure
		// may have been transient; either way the user expects "one session
		// per profile per task" so we revive rather than orphan a duplicate).
		if sess.State == models.TaskSessionStateCancelled {
			continue
		}
		if best == nil || sess.UpdatedAt.After(best.UpdatedAt) {
			best = sess
		}
	}
	return best, nil
}

// reuseSessionForStep promotes an existing session to primary, brings it out
// of COMPLETED/FAILED if needed, and stops + completes the previous session.
// The agent for the reused session is not relaunched here — when a prompt
// arrives, the autoStart/PromptTask paths handle the launch.
//
// Previously-launched sessions (executors_running record exists, has resume
// token) are flipped to WAITING_FOR_INPUT so PromptTask's ensureSessionRunning
// lazy-resumes them via ResumeSession.
//
// Never-launched sessions (e.g. PrepareSession created the row but the
// workflow switched away before the agent started) have no executors_running
// record. They go to CREATED so autoStartStepPrompt routes through
// StartCreatedSession → LaunchPreparedSession (a full fresh launch).
func (s *Service) reuseSessionForStep(ctx context.Context, taskID string, currentSession, existing *models.TaskSession) (*models.TaskSession, error) {
	s.logger.Info("reusing existing session for profile",
		zap.String("task_id", taskID),
		zap.String("current_session", currentSession.ID),
		zap.String("reused_session", existing.ID),
		zap.String("reused_profile", existing.AgentProfileID),
		zap.String("reused_state", string(existing.State)))

	if existing.State == models.TaskSessionStateCompleted || existing.State == models.TaskSessionStateFailed {
		s.reviveReusedSession(ctx, existing)
	}

	s.tagSessionAsWorkflowSwitched(ctx, existing.ID)

	if err := s.SetPrimarySession(ctx, existing.ID); err != nil {
		s.logger.Warn("failed to set reused session as primary",
			zap.String("session_id", existing.ID), zap.Error(err))
	}

	// Transfer any queued message and pending move from the session being
	// switched away from to the reused session — without this, a hand-off
	// prompt queued via move_task_kandev on the previous session is orphaned
	// and gets delivered to the wrong agent the next time that previous
	// session is reused (e.g. on the on_turn_complete bounce back).
	if s.messageQueue != nil {
		if err := s.messageQueue.TransferSession(ctx, currentSession.ID, existing.ID); err != nil {
			// Fail closed: the workflow switch reuses an existing session, but
			// orphaning a queued hand-off prompt on the previous session would
			// silently misroute the next prompt. Stop here and surface the
			// error so the caller can decide whether to retry.
			return nil, fmt.Errorf("transfer queued state to reused session: %w", err)
		}
	}

	s.completeAndStopSession(ctx, taskID, currentSession)
	return existing, nil
}

// reviveReusedSession flips a terminal (COMPLETED/FAILED) session back to a
// state where the downstream autoStart/PromptTask paths can launch its agent.
// The target state depends on whether the session was ever launched:
//   - Has executors_running record → WAITING_FOR_INPUT, lazy-resume from token
//   - No record → CREATED, fresh launch via StartCreatedSession
//
// The previous error message (from a prior FAILED state) is cleared so the
// frontend stops surfacing stale red banners on a now-active session.
func (s *Service) reviveReusedSession(ctx context.Context, session *models.TaskSession) {
	wasLaunched := false
	if running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID); err == nil && running != nil {
		wasLaunched = true
	}
	if wasLaunched {
		session.State = models.TaskSessionStateWaitingForInput
	} else {
		session.State = models.TaskSessionStateCreated
	}
	session.CompletedAt = nil
	session.ErrorMessage = ""
	session.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to revive reused session out of COMPLETED",
			zap.String("session_id", session.ID),
			zap.String("target_state", string(session.State)),
			zap.Error(err))
	}
}

// createNewSessionForStep is the original switch-and-create-fresh-session path,
// used when there is no existing session for the target profile.
func (s *Service) createNewSessionForStep(ctx context.Context, taskID string, currentSession *models.TaskSession, newAgentProfileID string) (*models.TaskSession, error) {
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

	// Tag the session as workflow-spawned for provenance: its agent profile
	// was selected by the workflow step override rather than direct user choice.
	s.tagSessionAsWorkflowSwitched(ctx, newSession.ID)

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

	// Transfer any queued message (e.g. a move_task_kandev hand-off prompt) and
	// pending move from the old session to the new one — the queue is keyed by
	// session ID, and without this the prompt would never reach the new agent.
	if s.messageQueue != nil {
		if err := s.messageQueue.TransferSession(ctx, currentSession.ID, newSession.ID); err != nil {
			s.logger.Error("transfer queue to new session failed; queued prompts on the previous session will not be drained",
				zap.String("from_session_id", currentSession.ID),
				zap.String("to_session_id", newSession.ID),
				zap.Error(err))
			// Continue anyway: the new session is already created and committed
			// upstream. Failing closed here would leave the workflow in a
			// half-switched state (new session exists but caller thinks it
			// failed). The error is surfaced via logs and the orphaned entries
			// stay safely in the old session for manual recovery.
		}
	}

	// Promote the new session to primary so it's loaded when navigating back to this task.
	// Use SetPrimarySession (not repo.SetSessionPrimary) to broadcast a task.updated WS
	// event — the frontend reads primarySessionId from the task to render the star icon.
	if err := s.SetPrimarySession(ctx, newSession.ID); err != nil {
		s.logger.Warn("failed to set new session as primary",
			zap.String("session_id", newSession.ID), zap.Error(err))
	}

	s.completeAndStopSession(ctx, taskID, currentSession)
	return newSession, nil
}

// completeAndStopSession stops the agent for a session and marks it COMPLETED.
// Used by both the reuse path and the create-new path to terminate the
// previous session in a uniform way. The state transition is the only session
// row write here: SetPrimarySession already cleared the old primary flag, and
// writing the caller's stale full row could resurrect a concurrently stopped
// session.
func (s *Service) completeAndStopSession(ctx context.Context, taskID string, session *models.TaskSession) {
	// Flip state to COMPLETED *before* stopping the agent. StopAgent fires an
	// agent.completed event, and handleAgentCompleted's terminal-state guard
	// only short-circuits when the session is already in a terminal state. If
	// we stopped first, the event would fire while state is still RUNNING (or
	// WAITING_FOR_INPUT for the deferred-move flow), the guard would miss it,
	// and processOnTurnCompleteViaEngine would evaluate the *new* (already
	// transitioned) step's on_turn_complete — re-firing the very transition we
	// just performed and ping-ponging the task between steps.
	s.updateTaskSessionState(ctx, taskID, session.ID, models.TaskSessionStateCompleted, "", false)

	if execID, err := s.agentManager.GetExecutionIDForSession(ctx, session.ID); err == nil && execID != "" {
		if stopErr := s.agentManager.StopAgent(ctx, execID, false); stopErr != nil {
			s.logger.Warn("failed to stop agent for session switch",
				zap.String("session_id", session.ID),
				zap.Error(stopErr))
		}
	}
}

// maybySwitchSessionForProfile checks whether the step requires a different agent profile
// and switches the session if so. Passthrough sessions are returned unchanged.
// When the step has no explicit step/workflow profile, the current session is
// preserved so workflow advancement does not reset the user's current agent mode.
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
		if effectiveProfile != "" {
			s.tagSessionAsWorkflowSwitched(ctx, session.ID)
		}
		if !session.IsPrimary {
			if err := s.SetPrimarySession(ctx, session.ID); err != nil {
				s.logger.Warn("failed to preserve session as primary for workflow step",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.String("step_id", step.ID),
					zap.Error(err))
			} else {
				session.IsPrimary = true
			}
		}
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

	// Stale session.State left over from a previous turn (e.g. the agent's
	// agent.ready fired before this goroutine resumed) would otherwise trick
	// queueAutoStartPromptIfRunning into queueing the auto-start prompt against
	// a session that's actually idle — and nothing would drain it because no
	// future agent.ready is pending. Flip to WAITING_FOR_INPUT when activeTurns
	// confirms no in-flight turn.
	//
	// This must run before the len(step.Events.OnEnter)==0 early-return below.
	// A stale-RUNNING session on a no-OnEnter step should still transition to
	// WAITING_FOR_INPUT and drain its queue; without the pre-flip it would
	// early-return unchanged, leaving session.State==RUNNING with no drain path.
	s.flipStaleRunningToWaiting(ctx, taskID, session, isPassthrough)

	hasPlanMode := s.resolveStepPlanMode(ctx, session, step, isPassthrough)

	if len(step.Events.OnEnter) == 0 && !sessionSwitched {
		// Active-turn case (e.g. move_task_kandev mid-turn): the agent is still
		// running and will fire agent.ready when the turn ends. Don't flip state
		// to WAITING here — handleAgentReady's RUNNING/STARTING guard would then
		// silence the event and orphan the queue. handleAgentReady runs
		// on_turn_complete against the new step and drains the queue itself.
		if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
			return
		}
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
		s.drainQueuedMessageForPromptableSession(ctx, sessionID)
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
		case wfmodels.OnEnterSetSessionMode:
			mode, _ := action.Config["mode"].(string)
			s.applyStepSessionMode(ctx, session, mode, isPassthrough)
		case wfmodels.OnEnterAutoStartAgent:
			hasAutoStart = true
		}
	}

	switch {
	case hasAutoStart && isPassthrough:
		// Passthrough path: write prompt directly to PTY stdin.
		// By the time processOnEnter runs (from an on_turn_complete transition),
		// the agent has finished its previous turn and the PTY is waiting for input.
		effectivePrompt := s.buildWorkflowPrompt(ctx, taskDescription, step, taskID, sessionID, isPassthrough)
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
		// When called from applyEngineTransition (on_turn_complete), processOnEnter
		// runs in a goroutine and the session is already WAITING_FOR_INPUT, so
		// autoStartStepPrompt sends the prompt directly via PromptTask.
		effectivePrompt := s.buildWorkflowPrompt(ctx, taskDescription, step, taskID, sessionID, isPassthrough)
		if err := s.autoStartStepPrompt(ctx, taskID, session, step, effectivePrompt, hasPlanMode, true); err != nil {
			s.logger.Error("failed to auto-start agent for step",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name),
				zap.Error(err))
			s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
			s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
		}

	default:
		// When the session was just switched (agent profile change) but the step
		// has no auto_start_agent, launch the agent anyway — the profile override
		// implies the user wants this agent to run on this step.
		if sessionSwitched && step.Prompt != "" {
			effectivePrompt := s.buildWorkflowPrompt(ctx, taskDescription, step, taskID, sessionID, isPassthrough)
			planMode := hasPlanMode
			stepID := step.ID
			s.logger.Info("auto-launching agent after profile switch (no explicit auto_start)",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", step.Name))
			// Launch asynchronously because processOnEnter may also be called
			// synchronously from finalizeStepEnter (manual task move). In that path,
			// autoStartStepPrompt would block the caller's goroutine.
			go func() {
				asyncCtx := context.WithoutCancel(ctx)
				err := s.autoStartStepPrompt(asyncCtx, taskID, session, step, effectivePrompt, planMode, true)
				if err != nil {
					s.logger.Error("failed to launch agent after profile switch",
						zap.String("task_id", taskID),
						zap.String("session_id", sessionID),
						zap.Error(err))
					s.setSessionWaitingForInput(asyncCtx, taskID, sessionID, session)
					s.publishSessionWaitingEvent(asyncCtx, taskID, sessionID, stepID, session)
					s.drainQueuedMessageForPromptableSession(asyncCtx, sessionID)
				}
			}()
			return
		}
		// Same active-turn guard as the no-on_enter branch above: if the agent
		// is still mid-turn, leave state alone so handleAgentReady can run on
		// turn end. See that branch for the full rationale.
		if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
			return
		}
		s.setSessionWaitingForInput(ctx, taskID, sessionID, session)
		s.publishSessionWaitingEvent(ctx, taskID, sessionID, step.ID, session)
		// handleAgentReady early-returns when a workflow transition occurs (#677),
		// so user-queued messages would otherwise stick forever on transitions to
		// steps without auto_start_agent (e.g. Review). Drain here to match the
		// pre-#677 behavior where handleAgentReady always drained after returning
		// from inline processOnEnter.
		s.drainQueuedMessageForPromptableSession(ctx, sessionID)
	}
}

// applyPendingMove applies a deferred move_task_kandev call now that the agent's
// turn has ended. Synchronous: updates the task's step in the DB, runs on_exit
// for the source step and on_enter for the target step. Bypasses
// task.Service.MoveTask (and the task.moved event) so the orchestrator's async
// task.moved handler doesn't run a second processStepExitAndEnter for the same
// transition. The message queue is left intact — any user-supplied prompt
// already queued by handleMoveTask is delivered by the on_enter path or by
// drainQueuedMessageForPromptableSession.
func (s *Service) applyPendingMove(ctx context.Context, taskID, sessionID string, session *models.TaskSession, move *messagequeue.PendingMove) {
	// reinsertPendingMove restores the move so a future agent.ready can retry.
	// Used on early failure paths (load errors, config issues) where the state
	// hasn't been touched yet. NOT used after ApplyTransition has executed —
	// at that point the workflow has either advanced or is in a corrupted state
	// and re-attempting the move on the next turn would just re-trip the same
	// failure (or worse, double-apply on a now-half-transitioned task).
	reinsertPendingMove := func() {
		if s.messageQueue == nil {
			return
		}
		s.messageQueue.SetPendingMove(ctx, sessionID, move)
	}

	if s.workflowStepGetter == nil || s.workflowStore == nil {
		s.logger.Warn("cannot apply pending move: workflow components missing",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		reinsertPendingMove()
		return
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Error("failed to load task for pending move",
			zap.String("task_id", taskID),
			zap.Error(err))
		reinsertPendingMove()
		return
	}
	fromStepID := task.WorkflowStepID
	if fromStepID == move.WorkflowStepID {
		// Step already matches — nothing to transition. Just leave the queued
		// prompt for the natural drain path. Don't reinsert: the move is
		// effectively complete since the task is already at the target step.
		s.logger.Info("pending move target equals current step; skipping transition",
			zap.String("task_id", taskID),
			zap.String("step_id", fromStepID))
		s.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID)
		return
	}

	targetStep, err := s.workflowStepGetter.GetStep(ctx, move.WorkflowStepID)
	if err != nil || targetStep == nil {
		s.logger.Error("failed to load target step for pending move",
			zap.String("task_id", taskID),
			zap.String("target_step_id", move.WorkflowStepID),
			zap.Error(err))
		reinsertPendingMove()
		return
	}
	if targetStep.WorkflowID != move.WorkflowID {
		s.logger.Error("pending move target step belongs to a different workflow; dropping move",
			zap.String("task_id", taskID),
			zap.String("target_step_id", move.WorkflowStepID),
			zap.String("step_workflow_id", targetStep.WorkflowID),
			zap.String("move_workflow_id", move.WorkflowID))
		// Do NOT reinsert: the move is invalid and retrying would keep failing.
		// The hand-off prompt was queued by handleMoveTask before the move was
		// applied. Since the move is being dropped, the on_enter path that would
		// have drained the queue won't run. Drop the orphan so it can't be
		// misdelivered to the source step's agent on a future turn (it was
		// authored for the move's *target* step) — mirrors the cleanup done on
		// the ApplyTransition failure path below.
		if s.messageQueue != nil {
			if _, ok := s.messageQueue.TakeQueued(ctx, sessionID); ok {
				s.publishQueueStatusEvent(ctx, sessionID)
				s.logger.Warn("dropped hand-off prompt after pending-move workflow mismatch",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID))
			}
		}
		return
	}

	// Mark the session WAITING_FOR_INPUT before processOnEnter runs. The agent
	// just finished its turn; the active-turn guard in processOnEnter would
	// otherwise see RUNNING and skip the on_enter processing.
	s.setSessionWaitingForInput(ctx, taskID, sessionID, session)

	if err := s.workflowStore.ApplyTransition(ctx, taskID, sessionID, fromStepID, move.WorkflowStepID, engine.TriggerOnEnter); err != nil {
		s.logger.Error("failed to apply pending move transition",
			zap.String("task_id", taskID),
			zap.Error(err))
		// The hand-off prompt was queued by handleMoveTask before the move was
		// applied. Now that the move failed, the on_enter path that would have
		// drained the queue won't run, and handleAgentReady has already returned.
		// Drop the orphan so it can't be misdelivered to the source step's agent
		// on a future turn (it was authored for the move's *target* step).
		if s.messageQueue != nil {
			if _, ok := s.messageQueue.TakeQueued(ctx, sessionID); ok {
				s.publishQueueStatusEvent(ctx, sessionID)
				s.logger.Warn("dropped hand-off prompt after pending-move transition failure",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID))
			}
		}
		return
	}

	s.logger.Info("applying pending move",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", fromStepID),
		zap.String("to_step_id", move.WorkflowStepID))

	s.syncTaskStateForPendingMove(ctx, taskID, fromStepID, move.WorkflowStepID)

	// Run on_exit + on_enter asynchronously. This call originated from
	// handleAgentReady on the WS event reader goroutine; processStepExitAndEnter
	// can take many seconds (resume + agentctl bootstrap) and would otherwise
	// block that reader, queueing other agent events behind it and creating
	// race conditions with concurrent on_turn_complete / agent.completed events
	// for the same session. The DB transition is already persisted above, so
	// it's safe to defer the rest.
	taskDescription := task.Description
	go s.processStepExitAndEnter(context.WithoutCancel(ctx), taskID, session, fromStepID, move.WorkflowStepID, taskDescription)
}

func (s *Service) syncTaskStateForPendingMove(ctx context.Context, taskID, fromStepID, toStepID string) {
	if s.workflowStepIsTerminal(ctx, toStepID) {
		s.markTaskCompletedForTerminalStep(ctx, taskID, toStepID)
		return
	}
	if fromStepID == toStepID || !s.workflowStepIsTerminal(ctx, fromStepID) {
		return
	}

	s.taskRuntimeStateMu.Lock()
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.taskRuntimeStateMu.Unlock()
		s.logger.Warn("pending move state sync: failed to load task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if task.WorkflowStepID != toStepID || task.State != v1.TaskStateCompleted {
		s.taskRuntimeStateMu.Unlock()
		return
	}

	oldState := task.State
	task.State = v1.TaskStateTODO
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.taskRuntimeStateMu.Unlock()
		s.logger.Warn("pending move state sync: failed to reopen completed task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	s.taskRuntimeStateMu.Unlock()
	s.publishTaskUpdated(ctx, task)
	s.publishTaskStateChanged(ctx, task, oldState)
}

// drainQueuedMessageForPromptableSession acquires sessionID's cancelInFlight
// guard and takes+dispatches the next queued message, blocking until any
// concurrent cancel/interrupt/drain for the same session finishes first —
// see the Service.cancelInFlight field doc comment for why every
// take-and-dispatch decision must serialize through this one guard rather
// than risk two callers racing to steal the same entry. Blocking (not
// skipping on contention) matters here because, unlike
// handleAgentReady/handleAgentBootReady's own take-decision (where a losing
// side can rely on the winning side's take-and-dispatch, or a future turn's
// own agent.ready, to eventually retry), callers of this function —
// workflow on_enter branches, manual drain requests, CI automation — have
// no other future trigger that would retry a skipped drain; skipping on
// contention here could strand an already-queued message.
//
// Callers must ensure the session is ready for input *before* calling this
// — exactly as before this was guarded — but must not have already claimed
// the guard themselves; use drainQueuedMessageForPromptableSessionLocked
// instead when the guard is already held (e.g. inside
// cancelAndTakeForPeerMessage, handleAgentReady, or applyPendingMove).
//
// Reloads the session and re-confirms promptability *after* acquiring the
// guard rather than trusting the caller's own earlier check: callers like
// processOnEnter typically call setSessionWaitingForInput and then this
// function without holding the guard across both, so a concurrent
// cancel/interrupt for the same session could land in between and this
// call would otherwise blindly take a message for a session that turned
// out to no longer be idle.
func (s *Service) drainQueuedMessageForPromptableSession(ctx context.Context, sessionID string) bool {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload session before drain",
			zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		s.logger.Debug("skipping drain: session is not promptable once the guard is held",
			zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	return s.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID)
}

// drainQueuedMessageForPromptableSessionLocked takes the next queued
// message and dispatches it for execution. Callers must ensure the session
// is ready for input first, AND must already hold sessionID's
// cancelInFlight lock — this neither acquires nor releases it. Use the
// public drainQueuedMessageForPromptableSession instead when the guard is
// not already held.
//
// Backs off without taking anything when isQueuedDispatchInFlight reports
// a different dispatch already handed off for this session — see the
// Service.dispatchingQueued field doc comment for the double-dispatch
// window this closes.
func (s *Service) drainQueuedMessageForPromptableSessionLocked(ctx context.Context, sessionID string) bool {
	if s.messageQueue == nil || s.isQueuedDispatchInFlight(sessionID) {
		return false
	}
	queuedMsg, ok := s.messageQueue.TakeQueued(ctx, sessionID)
	return s.dispatchTakenQueuedMessage(ctx, sessionID, queuedMsg, ok)
}

// dispatchTakenQueuedMessage publishes the queue-status update and dispatches
// queuedMsg for execution, given the (message, ok) pair returned by a Take*
// call on the message queue (TakeQueued or TakeQueuedEntry). Shared so
// InterruptForPeerMessage's targeted-entry take gets the same empty-message
// guard and dispatch behavior as the FIFO-head drain.
func (s *Service) dispatchTakenQueuedMessage(ctx context.Context, sessionID string, queuedMsg *messagequeue.QueuedMessage, ok bool) bool {
	if !ok || queuedMsg == nil {
		return false
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	if queuedMsg.Content == "" && len(queuedMsg.Attachments) == 0 {
		s.logger.Warn("skipping empty queued message after transition",
			zap.String("session_id", sessionID),
			zap.String("queue_id", queuedMsg.ID))
		return false
	}
	// Reserve entryID as sessionID's "dispatch in flight" token *before*
	// handing off to the async goroutine — see the Service.dispatchingQueued
	// field doc comment for why session.State alone isn't a reliable busy
	// signal until executeQueuedMessage's own promptTask call reaches its
	// guarded claim step, several DB round-trips later.
	s.markQueuedDispatchInFlight(sessionID, queuedMsg.ID)
	go s.executeQueuedMessage(sessionID, queuedMsg)
	return true
}

// deliverPassthroughPrompt writes a prompt to PTY stdin and marks the session as running.
// Uses the per-agent PlanPassthroughStdinChunks so Claude's inter-chunk SubmitDelay is
// honored here too (queued / workflow-auto-start path); other agents stay on the single
// atomic write. Falls back to the simple "\r" append if config resolution fails so a
// transient lookup error never silently swallows the prompt.
func (s *Service) deliverPassthroughPrompt(ctx context.Context, sessionID, content string) error {
	pt, cfgErr := s.agentManager.ResolvePassthroughConfig(ctx, sessionID)
	if cfgErr != nil {
		s.logger.Warn("failed to resolve passthrough config, falling back to \\r submit",
			zap.String("session_id", sessionID),
			zap.Error(cfgErr))
	}
	// Mark RUNNING before any writes so concurrent PromptTask / queued-message
	// delivery is blocked by checkSessionPromptable during the inter-chunk
	// SubmitDelay window (150ms for Claude). Mark error is non-fatal.
	if err := s.agentManager.MarkPassthroughRunning(sessionID); err != nil {
		s.logger.Warn("failed to mark passthrough as running before prompt",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	if cfgErr != nil {
		if err := s.agentManager.WritePassthroughStdin(ctx, sessionID, content+"\r"); err != nil {
			return fmt.Errorf("write to passthrough stdin: %w", err)
		}
		return nil
	}
	for _, chunk := range agents.PlanPassthroughStdinChunks(content, pt) {
		if chunk.DelayBefore > 0 {
			time.Sleep(chunk.DelayBefore)
		}
		if err := s.agentManager.WritePassthroughStdin(ctx, sessionID, chunk.Data); err != nil {
			return fmt.Errorf("write to passthrough stdin: %w", err)
		}
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

// metaKeyUserMessageRecorded marks a queued workflow auto-start message
// whose chat-history user row was already inserted by recordAutoStartMessage
// before the prompt was queued. executeQueuedMessage reads this flag to skip
// its own CreateUserMessage and avoid the duplicate observed when PromptTask
// failed transiently and the queue was later drained via boot_ready.
const metaKeyUserMessageRecorded = "user_message_recorded"

type workflowMessageOrigin struct {
	StepID    string
	StepName  string
	StepColor string
}

func workflowOriginFromStep(step *wfmodels.WorkflowStep) workflowMessageOrigin {
	if step == nil {
		return workflowMessageOrigin{}
	}
	return workflowMessageOrigin{
		StepID:    step.ID,
		StepName:  step.Name,
		StepColor: step.Color,
	}
}

func workflowMessageMetadata(planMode bool, origin workflowMessageOrigin, references []v1.EntityReference) map[string]interface{} {
	meta := NewUserMessageMeta().
		WithPlanMode(planMode).
		WithAutoStart(true).
		WithWorkflowStep(origin.StepID, origin.StepName, origin.StepColor).
		WithEntityReferences(references).
		ToMap()
	if meta == nil {
		meta = make(map[string]interface{})
	}
	meta["workflow_auto_start"] = true
	return meta
}

func (s *Service) autoStartStepPrompt(
	ctx context.Context,
	taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep, prompt string,
	planMode bool,
	shouldQueueIfBusy bool,
) error {
	sessionID := session.ID
	origin := workflowOriginFromStep(step)
	stepName := origin.StepName

	// Take any queued message (e.g. from move_task_kandev with a hand-off
	// prompt) and merge it with the step's auto-start prompt — auto-start
	// content first, hand-off after — and forward attachments verbatim.
	// Track the original message so terminal failure paths can restore it
	// instead of dropping the user's prompt or attachments on the floor.
	takenMsg, mergedPrompt, attachments, references := s.takeAndMergeHandoffMessage(ctx, sessionID, prompt)
	prompt = mergedPrompt
	agentPrompt := AppendEntityReferenceContext(prompt, references)

	// requeueTaken puts the original queued message back so a manual retry can
	// pick it up. Skip when shouldQueueIfBusy successfully re-queued the
	// concatenated prompt (the content is already preserved there).
	requeueTaken := func() {
		if takenMsg == nil {
			return
		}
		s.requeueMessage(ctx, takenMsg, takenMsg.QueuedBy)
	}

	if shouldQueueIfBusy {
		// userMessageRecorded=false: recordAutoStartMessage has not run yet —
		// the drain side (executeQueuedMessage) is responsible for inserting
		// the chat-history row.
		queued, err := s.queueAutoStartPromptIfRunning(ctx, taskID, session, prompt, planMode, attachments, origin, false, references)
		if err != nil {
			requeueTaken()
			return err
		}
		if queued {
			return nil
		}
	}

	// Record a user message so the auto-start prompt is visible in chat
	// history. For CREATED sessions the agent has not started yet, so this is
	// the first prompt of the task — wrap with the Kandev MCP system block
	// before persisting (and before passing downstream) so the DB row matches
	// what the agent receives. StartCreatedSession's wrap is idempotent
	// (HasKandevContext guard) so the pre-wrap doesn't double.
	// The HasKandevContext check on `agentPrompt` also guards against any future
	// caller that ever pre-wraps before reaching here (none today).
	// Passthrough sessions skip the wrap: the prompt is typed straight into
	// the agent CLI's TTY and the user sees it verbatim.
	recordedPrompt := agentPrompt
	if session.State == models.TaskSessionStateCreated && !session.IsPassthrough && (agentPrompt != "" || len(attachments) > 0) && !sysprompt.HasKandevContext(agentPrompt) {
		isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
		if err != nil {
			requeueTaken()
			return fmt.Errorf("resolve MCP mode for workflow auto-start: %w", err)
		}
		configMode, _ := session.Metadata["config_mode"].(bool)
		requiresSignal := step != nil && step.AutoAdvanceRequiresSignal
		recordedPrompt = sysprompt.InjectKandevContextWithOptions(taskID, sessionID, agentPrompt, sysprompt.KandevContextOptions{
			RequiresCompletionSignal:       requiresSignal,
			IncludeCoordinatorTaskControls: !isOfficeTask && !configMode,
		})
	}
	userMsgRecorded := s.recordAutoStartMessage(ctx, taskID, sessionID, recordedPrompt, planMode, origin, references)

	// If the session is in CREATED state, the agent was never started (e.g. workspace-only
	// preparation from a blocked auto-start). PromptTask will reject CREATED sessions,
	// so use StartCreatedSession which properly launches the agent on the prepared workspace.
	// Pass skipMessageRecord=true since recordAutoStartMessage above already recorded it.
	if session.State == models.TaskSessionStateCreated {
		s.logger.Info("auto-start: session is CREATED, launching agent via StartCreatedSession",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName))
		_, err := s.StartCreatedSession(ctx, taskID, sessionID, session.AgentProfileID, recordedPrompt, true, planMode, true, attachments)
		if err != nil {
			requeueTaken()
		}
		return err
	}

	const maxRetryAttempts = 5
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		_, err := s.PromptTask(ctx, taskID, sessionID, agentPrompt, "", planMode, attachments, false)
		if err == nil {
			return nil
		}

		// ErrExecutionNotFound means ResumeSession landed on an execution that
		// the lifecycle manager no longer has (e.g. the post-resume agent
		// process failed to start and runAgentProcessAsync removed it). The
		// session's stored AgentExecutionID is now stale. Recover by clearing
		// it and routing through StartCreatedSession for a fresh launch — the
		// prompt is baked into LaunchPreparedSession so we don't lose it.
		if errors.Is(err, executor.ErrExecutionNotFound) {
			s.logger.Warn("auto-start: PromptTask hit missing execution; falling back to fresh launch",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", stepName))
			return s.fallbackFreshLaunchOnMissingExecution(ctx, taskID, sessionID, agentPrompt, planMode, takenMsg, attachments)
		}

		// "already has an agent running" means the execution store still tracks
		// an active agent for this session (e.g. session state is CREATED but
		// the agent was launched by a concurrent path). Queue instead of retrying.
		// Pass userMsgRecorded so the drain skips CreateUserMessage only when the
		// chat row was successfully inserted above; a failed write passes false,
		// letting the drain re-attempt insertion.
		if isAgentAlreadyRunningError(err) && shouldQueueIfBusy {
			if queueErr := s.queueAutoStartPrompt(ctx, taskID, sessionID, prompt, planMode, attachments, origin, userMsgRecorded, references); queueErr != nil {
				requeueTaken()
				return queueErr
			}
			return nil
		}

		if !isSessionBusyError(err) && !isTransientPromptError(err) && !isSessionResetInProgressError(err) {
			requeueTaken()
			return err
		}

		if shouldQueueIfBusy {
			// Pass userMsgRecorded so the drain skips CreateUserMessage only when
			// the chat row was successfully inserted above by recordAutoStartMessage.
			if queueErr := s.queueAutoStartPrompt(ctx, taskID, sessionID, prompt, planMode, attachments, origin, userMsgRecorded, references); queueErr != nil {
				requeueTaken()
				return queueErr
			}
			return nil
		}

		if attempt == maxRetryAttempts {
			requeueTaken()
			return err
		}

		delay := time.Duration(50*(1<<(attempt-1))) * time.Millisecond
		select {
		case <-ctx.Done():
			requeueTaken()
			return fmt.Errorf("auto-start context canceled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return nil
}

// fallbackFreshLaunchOnMissingExecution recovers from a PromptTask that returned
// ErrExecutionNotFound — the session's stored AgentExecutionID points at an
// execution the lifecycle manager doesn't have, so the resume path is dead.
// Clear the stale ID, flip state to CREATED, and route through StartCreatedSession
// (which uses LaunchPreparedSession with the prompt baked in — bypassing resume).
// On further failure, the queued message is restored so a manual retry recovers it.
func (s *Service) fallbackFreshLaunchOnMissingExecution(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
	takenMsg *messagequeue.QueuedMessage,
	attachments []v1.MessageAttachment,
) error {
	requeue := func() {
		if takenMsg != nil {
			s.requeueMessage(ctx, takenMsg, takenMsg.QueuedBy)
		}
	}

	// Keep coordinator stop outside the reset-to-CREATED / fresh-runtime
	// registration window. If fallback wins, stop observes the newly
	// registered execution after this guard is released; if stop won earlier,
	// the guarded state CAS below sees CANCELLED and aborts the replacement.
	cancelLock, releaseCancelLock := s.acquireCancelInFlightGuard(sessionID)
	cancelLock.Lock()
	fresh, err := s.resetSessionForFreshFallback(ctx, sessionID)
	cancelLock.Unlock()
	releaseCancelLock()
	if err != nil {
		requeue()
		return err
	}

	if _, err := s.StartCreatedSession(ctx, taskID, sessionID, fresh.AgentProfileID, prompt, true, planMode, true, attachments); err != nil {
		s.logger.Error("auto-start fallback: fresh launch failed",
			zap.String("session_id", sessionID), zap.Error(err))
		requeue()
		return err
	}
	return nil
}

func (s *Service) resetSessionForFreshFallback(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	s.taskRuntimeStateMu.Lock()
	defer s.taskRuntimeStateMu.Unlock()

	fresh, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Error("auto-start fallback: failed to load session",
			zap.String("session_id", sessionID), zap.Error(err))
		return nil, err
	}
	if fresh == nil {
		return nil, fmt.Errorf("auto-start fallback: session %q is nil", sessionID)
	}
	if isTerminalSessionState(fresh.State) {
		return nil, &executor.SessionStateSupersededError{
			SessionID: fresh.ID,
			State:     fresh.State,
		}
	}

	reset := *fresh
	reset.State = models.TaskSessionStateCreated
	reset.UpdatedAt = time.Now().UTC()
	if err := s.persistFullTaskSessionIfCurrent(ctx, &reset, fresh.State); err != nil {
		s.logger.Error("auto-start fallback: failed to reset session for fresh launch",
			zap.String("session_id", sessionID), zap.Error(err))
		return nil, err
	}

	// Drop the executors_running row only after the guarded state reset wins.
	// The next StartCreatedSession takes the full LaunchAgent path and creates a
	// fresh row via lifecycle.persistExecutorRunning.
	if delErr := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); delErr != nil &&
		!errors.Is(delErr, models.ErrExecutorRunningNotFound) {
		s.logger.Warn("auto-start fallback: failed to clear executors_running for fresh launch",
			zap.String("session_id", sessionID), zap.Error(delErr))
	}
	return &reset, nil
}

// takeAndMergeHandoffMessage drains any queued hand-off message for the session
// (set by handleMoveTask via move_task_kandev or by drainQueuedMessageForPromptableSession)
// and merges its content + attachments into the auto-start prompt. Returns the
// original queued message (so terminal failure paths can re-queue it via
// requeueMessage), merged prompt, converted attachments, and structurally
// normalized references.
func (s *Service) takeAndMergeHandoffMessage(ctx context.Context, sessionID, basePrompt string) (*messagequeue.QueuedMessage, string, []v1.MessageAttachment, []v1.EntityReference) {
	if s.messageQueue == nil {
		return nil, basePrompt, nil, nil
	}
	msg, ok := s.messageQueue.TakeQueued(ctx, sessionID)
	if !ok || msg == nil || (msg.Content == "" && len(msg.Attachments) == 0) {
		return nil, basePrompt, nil, nil
	}
	prompt := basePrompt
	if msg.Content != "" {
		prompt = basePrompt + "\n\n" + msg.Content
	}
	var attachments []v1.MessageAttachment
	if len(msg.Attachments) > 0 {
		attachments = make([]v1.MessageAttachment, 0, len(msg.Attachments))
		for _, a := range msg.Attachments {
			attachments = append(attachments, v1.MessageAttachment{
				Type:         a.Type,
				Data:         a.Data,
				MimeType:     a.MimeType,
				Name:         a.Name,
				DeliveryMode: a.DeliveryMode,
			})
		}
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	references := entityrefs.NormalizePersisted(msg.Metadata[messagequeue.MetadataEntityReferences])
	return msg, prompt, attachments, references
}

// recordAutoStartMessage creates a user message for a workflow auto-start prompt
// so it appears in the chat history. The prompt content includes system-injected
// tags which are stripped when displayed to users via ToAPI().
// Returns true when the chat row was successfully inserted, false otherwise
// (messageCreator nil, empty prompt, or DB write failure). Callers that queue
// the prompt after this call must pass the return value to queueAutoStartPrompt
// as userMessageRecorded, so the drain side only skips CreateUserMessage when
// the write actually succeeded.
func (s *Service) recordAutoStartMessage(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
	origin workflowMessageOrigin,
	references []v1.EntityReference,
) bool {
	if s.messageCreator == nil || prompt == "" {
		return false
	}
	turnID := s.getActiveTurnID(sessionID)
	if turnID == "" {
		s.startTurnForSession(ctx, sessionID)
		turnID = s.getActiveTurnID(sessionID)
	}
	// auto_start tags this seed prompt as automation-originated so the
	// github cleanup filter (HasUserAuthoredMessage) skips it — without
	// this tag, a workflow auto-start fired on a PR-watch task makes the
	// task look user-authored and the cleanup loop preserves it on merge,
	// re-creating the exact pileup the cleanup_policy work fixes.
	// workflow_auto_start is the original tag this function set; preserved
	// for any consumer reading it directly.
	metaMap := workflowMessageMetadata(planMode, origin, references)
	if err := s.messageCreator.CreateUserMessage(ctx, taskID, prompt, sessionID, turnID, metaMap); err != nil {
		s.logger.Error("failed to create auto-start user message",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}
	return true
}

// queueAutoStartPromptIfRunning queues the workflow auto-start prompt only
// when the session is currently RUNNING/STARTING, returning queued=true on
// success. Callers must ensure session.State is fresh — a stale RUNNING flag
// (no in-flight turn) would queue a prompt that nothing drains. processOnEnter
// runs flipStaleRunningToWaiting before reaching this path; applyEngineTransition
// flips the same way inline. New call sites must do the same.
func (s *Service) queueAutoStartPromptIfRunning(
	ctx context.Context,
	taskID string, session *models.TaskSession, prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
	origin workflowMessageOrigin,
	userMessageRecorded bool,
	references []v1.EntityReference,
) (bool, error) {
	if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
		return false, nil
	}
	if err := s.queueAutoStartPrompt(ctx, taskID, session.ID, prompt, planMode, attachments, origin, userMessageRecorded, references); err != nil {
		return false, err
	}
	return true, nil
}

func toQueuedAttachments(attachments []v1.MessageAttachment) []messagequeue.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	queued := make([]messagequeue.MessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		queued = append(queued, messagequeue.MessageAttachment{
			Type:         attachment.Type,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			DeliveryMode: attachment.DeliveryMode,
		})
	}
	return queued
}

// queueAutoStartPrompt persists a workflow auto-start prompt for later drain.
// userMessageRecorded must be the return value of recordAutoStartMessage: true
// only when CreateUserMessage actually succeeded. The flag is stamped onto the
// queue metadata so executeQueuedMessage skips its own CreateUserMessage and
// avoids the duplicate-user-message bug observed when PromptTask failed
// transiently and the queue drained on boot_ready. Passing false (failed write
// or pre-record queue path) lets the drain side record the message instead.
// Callers that queue BEFORE recordAutoStartMessage runs (e.g.
// queueAutoStartPromptIfRunning's early-busy path) must pass false. Prompt
// content stays raw here; references remain metadata until drain-time context
// is built, preventing duplicate system blocks across retries.
func (s *Service) queueAutoStartPrompt(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
	origin workflowMessageOrigin,
	userMessageRecorded bool,
	references []v1.EntityReference,
) error {
	if s.messageQueue == nil {
		return fmt.Errorf("message queue is not configured")
	}
	meta := workflowMessageMetadata(planMode, origin, references)
	if userMessageRecorded {
		meta[metaKeyUserMessageRecorded] = true
	}
	_, err := s.messageQueue.QueueMessageWithMetadata(
		ctx,
		sessionID,
		taskID,
		prompt,
		"",
		messagequeue.QueuedByWorkflow,
		planMode,
		toQueuedAttachments(attachments),
		meta,
	)
	if err != nil {
		return fmt.Errorf("failed to queue workflow auto-start prompt: %w", err)
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	s.scheduleAutoResumeForWorkflowQueue(ctx, sessionID)
	return nil
}

// scheduleAutoResumeForWorkflowQueue kicks off a background resume when a
// workflow auto-start prompt was just queued but no live agent process exists
// to drain it. No-op when the agent is already running — handleAgentReady
// will drain on the next turn end. Uses the same tryEnsureExecution path as
// EnsureSession (office panels), which drives ResumeSession → agent.boot_ready
// → handleAgentBootReady → drainQueuedMessageAfterTransition.
//
// Covers the case where the execution is dead at queue time (e.g. agent
// crashed just before the on_enter transition). If the agent is alive when
// the queue is written but dies later, the queue is drained by the next
// handleAgentBootReady (manual or automatic resume).
func (s *Service) scheduleAutoResumeForWorkflowQueue(ctx context.Context, sessionID string) {
	if s.executor == nil {
		return
	}
	if exec, ok := s.executor.GetExecutionBySession(sessionID); ok && exec != nil {
		return
	}
	go s.tryEnsureExecution(context.WithoutCancel(ctx), sessionID)
}

// flipStaleRunningToWaiting flips the session to WAITING_FOR_INPUT when its
// state claims RUNNING/STARTING but the orchestrator's authoritative
// activeTurns map shows no in-flight turn. This catches the manual-move race
// where processOnEnter runs from the task.moved goroutine with a session
// pointer loaded *before* the previous turn's agent.ready fired (or after that
// agent.ready already completed but the DB hadn't propagated yet). Without
// this flip, queueAutoStartPromptIfRunning would queue the auto-start prompt
// against a session no longer mid-turn, and no future agent.ready would fire
// to drain it.
//
// Skip when:
//   - session.State is not RUNNING/STARTING (CREATED routes through
//     StartCreatedSession; terminal states are immutable);
//   - the session is passthrough (PTY-driven, manages its own RUNNING/idle
//     transitions via MarkPassthroughRunning);
//   - a context reset is in progress (resetAgentContext owns the state
//     machine until it completes and runs markIdleAfterReset);
//   - activeTurns has an entry for the session (a turn is genuinely in flight,
//     queueing is correct and agent.ready will drain).
//
// applyEngineTransition (engine-driven on_turn_complete path) already does
// this flip inline at the call site; that path stays untouched — the check
// here is idempotent, so even if both fired, the second is a no-op.
//
// Returns true when the flip happened (mostly useful for tests and logs).
func (s *Service) flipStaleRunningToWaiting(ctx context.Context, taskID string, session *models.TaskSession, isPassthrough bool) bool {
	if session.State != models.TaskSessionStateRunning &&
		session.State != models.TaskSessionStateStarting {
		return false
	}
	if isPassthrough {
		return false
	}
	// Guards against a concurrent goroutine running resetAgentContext for the
	// same session — not the sequential reset_agent_context OnEnter action that
	// may run later in processOnEnter after this function returns. If both
	// reset_agent_context and auto_start_agent appear in OnEnter, the flip here
	// is still correct: resetAgentContext runs after this returns, then
	// markIdleAfterReset sees WAITING_FOR_INPUT and no-ops (idempotent).
	if s.isSessionResetInProgress(session.ID) {
		return false
	}
	if _, hasActiveTurn := s.activeTurns.Load(session.ID); hasActiveTurn {
		return false
	}
	priorState := session.State
	s.updateTaskSessionState(ctx, taskID, session.ID, models.TaskSessionStateWaitingForInput, "", false, session)
	session.State = models.TaskSessionStateWaitingForInput
	s.logger.Info("flipped stale RUNNING session to WAITING_FOR_INPUT (no active turn registered)",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("prior_state", string(priorState)))
	return true
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

	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || executionID == "" {
		s.logger.Debug("no agent execution for context reset, skipping",
			zap.String("session_id", sessionID))
		return true
	}

	s.logger.Info("resetting agent context for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("step_name", stepName),
		zap.String("agent_execution_id", executionID))

	s.setSessionResetInProgress(sessionID, true)
	defer s.setSessionResetInProgress(sessionID, false)

	if err := s.agentManager.ResetAgentContext(ctx, executionID); err != nil {
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

// SetSessionPlanModeByID looks up the session and writes plan_mode in its metadata.
// Skips passthrough sessions, which manage plan mode in the underlying CLI.
// Public entry point for client-driven plan-mode toggles (e.g. the "Implement plan"
// affordance) so the change is server-authoritative and survives page refresh.
func (s *Service) SetSessionPlanModeByID(ctx context.Context, sessionID string, enabled bool) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if s.agentManager.IsPassthroughSession(ctx, session.ID) {
		return nil
	}
	s.setSessionPlanMode(ctx, session, enabled)
	return nil
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

// applyStepSessionMode applies a workflow-declared session permission mode to a
// session entering a step (set_session_mode action, issue #1183). It persists the
// mode to metadata (durable + restored on reset) and best-effort applies it to a
// running agent via ACP session/set_mode. Passthrough sessions manage their own
// mode in the underlying CLI and are skipped, mirroring plan-mode handling.
func (s *Service) applyStepSessionMode(ctx context.Context, session *models.TaskSession, mode string, isPassthrough bool) {
	if mode == "" || isPassthrough {
		return
	}
	// Persist for durability (SSR / backend restart) and so Part 1's reset
	// re-apply has the right value. Mirror the in-memory struct for callers
	// that read session.Metadata afterwards.
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	session.Metadata[models.SessionMetaKeySessionMode] = mode
	s.persistSessionMode(ctx, session.ID, mode)

	// Apply live when an agent is running. When none is (e.g. the step also
	// auto-starts the agent fresh), this is a no-op and the profile default
	// governs the new session — the declared mode stays persisted.
	if err := s.agentManager.SetSessionModeBySessionID(ctx, session.ID, mode); err != nil {
		s.logger.Debug("set_session_mode: could not apply mode to a live agent (persisted for next launch/reset)",
			zap.String("session_id", session.ID),
			zap.String("mode", mode),
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
		metaKeyTaskID:      taskID,
		metaKeySessionID:   sessionID,
		"workflow_step_id": stepID,
		metaKeyNewState:    string(models.TaskSessionStateWaitingForInput),
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
		if !session.UpdatedAt.IsZero() {
			eventData[metaKeyUpdatedAt] = session.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
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

// publishSessionCreatedEvent publishes a session state change event for CREATED.
// PrepareTaskSession only writes the row to the DB without going through
// updateTaskSessionState, so without this the frontend's per-task session list
// stays empty until a manual reload (e.g. the kanban preview "No agents yet"
// staleness bug). Mirrors publishSessionWaitingEvent's payload shape so the
// existing session.state_changed handler can upsert the new session into the
// store.
func (s *Service) publishSessionCreatedEvent(ctx context.Context, taskID, sessionID, stepID string) {
	if s.eventBus == nil {
		return
	}
	eventData := map[string]interface{}{
		metaKeyTaskID:    taskID,
		metaKeySessionID: sessionID,
		metaKeyNewState:  string(models.TaskSessionStateCreated),
	}
	if stepID != "" {
		eventData["workflow_step_id"] = stepID
	}
	if session, err := s.repo.GetTaskSession(ctx, sessionID); err == nil && session != nil {
		if !session.UpdatedAt.IsZero() {
			eventData[metaKeyUpdatedAt] = session.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		if session.AgentProfileID != "" {
			eventData["agent_profile_id"] = session.AgentProfileID
		}
		if len(session.AgentProfileSnapshot) > 0 {
			eventData["agent_profile_snapshot"] = session.AgentProfileSnapshot
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

	// ADR 0015 — explicit completion signal gating. Steps marked
	// `auto_advance_requires_signal=true` only transition when the agent
	// (or the manual fallback button) has written the pending bag entry.
	// On gate-fail we set the session to WAITING_FOR_INPUT and bail —
	// either the user replies (clearing the bag) or a later
	// step_complete_kandev call triggers the out-of-band subscriber.
	//
	// Fail closed on step-load errors: a missing/broken step record must
	// not silently bypass the gate (which would let a signal-required step
	// auto-advance whenever the loader hiccups). Block the transition and
	// let the next turn re-evaluate after the underlying error clears.
	currentStep, stepErr := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if stepErr != nil || currentStep == nil {
		s.logger.Warn("on_turn_complete: failed to load current step for signal gating, blocking transition",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", task.WorkflowStepID),
			zap.Error(stepErr))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}
	if s.turnCompleteBlockedByUserInput(ctx, taskID, session.ID, session) {
		return false
	}
	if currentStep.AutoAdvanceRequiresSignal {
		signal, has := models.LoadPendingStepSignal(session.Metadata)
		if !has || signal.StepID != task.WorkflowStepID {
			s.logger.Debug("on_turn_complete gated on explicit signal (none received yet)",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.String("step_id", task.WorkflowStepID))
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
			return false
		}
		s.logger.Info("on_turn_complete consuming explicit signal",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", task.WorkflowStepID),
			zap.String("source", signal.Source))
		// Bag is consumed once the transition executes (in
		// applyEngineTransition's stamp + clear), so don't clear here —
		// otherwise a failed transition would lose the signal.
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

	terminalTarget := s.workflowStepIsTerminal(ctx, targetStep.ID)

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

	// ADR 0015 — a successful on_turn_complete transition consumes any
	// pending step-completion signal for the source step. The bag must be
	// cleared so the next step's gating starts from a clean slate.
	if trigger == engine.TriggerOnTurnComplete {
		s.clearPendingStepSignal(ctx, session)
	}

	if len(result.DataPatch) > 0 {
		if err := s.workflowStore.PersistData(ctx, session.ID, result.DataPatch); err != nil {
			s.logger.Warn("failed to persist workflow data patch",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}

	if terminalTarget {
		s.markTaskCompletedForTerminalStep(ctx, taskID, targetStep.ID)
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

	// When triggered from on_turn_complete, the agent has finished its turn but
	// handleAgentReady returns early without setting WAITING_FOR_INPUT (because the
	// transition already occurred). The session is still RUNNING in the DB.
	// Flip to WAITING_FOR_INPUT so that autoStartStepPrompt in processOnEnter sends
	// the prompt directly instead of queueing it — the queue would never be drained
	// because handleAgentReady already returned.
	//
	// Mirror setSessionWaitingForInput's task-state side effect: write
	// tasks.state = REVIEW so the kanban card drops out of IN_PROGRESS. Without
	// this, an engine-driven on_turn_complete transition would persist the
	// new workflow step + flip the session but leave tasks.state stale at
	// IN_PROGRESS, leaving the spinner spinning in the new column even though
	// the agent has paused. If the target step's on_enter starts another agent,
	// setSessionRunning will flip tasks.state back to IN_PROGRESS — the
	// REVIEW write is a safe intermediate when no sibling session is already
	// working; otherwise the task should remain IN_PROGRESS until all active
	// agent work has paused.
	if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
		s.updateTaskSessionState(ctx, taskID, session.ID, models.TaskSessionStateWaitingForInput, "", false, session)
		session.State = models.TaskSessionStateWaitingForInput
		if !terminalTarget {
			s.writeTaskReviewState(ctx, taskID, session.ID)
		}
	}

	// Launch processOnEnter asynchronously to avoid blocking the stream reader goroutine.
	// When triggered from on_turn_complete, the entire call chain runs in the WebSocket
	// stream reader goroutine (G_reader). processOnEnter may call resetAgentContext →
	// ResetAgentContext → sendStreamRequest, which blocks G_reader waiting for a response
	// that can only be delivered by G_reader reading from the same WebSocket — a deadlock.
	// The DB transition is already persisted above, so it's safe to process on_enter async.
	s.launchProcessOnEnter(context.WithoutCancel(ctx), taskID, session, targetStep, taskDescription)
	return true
}

func (s *Service) launchProcessOnEnter(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	targetStep *wfmodels.WorkflowStep,
	taskDescription string,
) {
	go func() {
		defer func() {
			if s.onProcessOnEnterComplete != nil {
				s.onProcessOnEnterComplete()
			}
		}()
		s.processOnEnter(ctx, taskID, session, targetStep, taskDescription)
	}()
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

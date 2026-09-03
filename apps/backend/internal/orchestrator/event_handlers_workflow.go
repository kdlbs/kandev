package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type turnCompletionCause string

var (
	errDeferredMoveAlreadyApplied           = errors.New("deferred move already applied")
	errReusableSessionNoLongerActive        = errors.New("reusable session is no longer active")
	errWorkflowAutoStartSessionTerminalized = errors.New("workflow auto-start session terminalized")
	errContextResetCancellationConflict     = errors.New("context reset cancellation is already in progress")
)

type workflowAutoStartSessionTerminalizedError struct {
	state models.TaskSessionState
}

func (e *workflowAutoStartSessionTerminalizedError) Error() string {
	return errWorkflowAutoStartSessionTerminalized.Error()
}

func (e *workflowAutoStartSessionTerminalizedError) Unwrap() error {
	return errWorkflowAutoStartSessionTerminalized
}

func newWorkflowAutoStartSessionTerminalizedError(session *models.TaskSession) error {
	var state models.TaskSessionState
	if session != nil {
		state = session.State
	}
	return &workflowAutoStartSessionTerminalizedError{state: state}
}

func workflowAutoStartWasCancelled(err error) bool {
	var terminalized *workflowAutoStartSessionTerminalizedError
	return errors.As(err, &terminalized) && terminalized.state == models.TaskSessionStateCancelled
}

type taskMetadataKeyRemover interface {
	RemoveTaskMetadataKey(context.Context, string, string) (bool, error)
}

type taskMetadataKeySetter interface {
	SetTaskMetadataKey(context.Context, string, string, interface{}) error
}

type taskMetadataKeySetterIfNoActiveSession interface {
	SetTaskMetadataKeyIfNoActiveSession(context.Context, string, string, interface{}) (bool, error)
}

type lifecycleTaskMetadataLister interface {
	ListTasksWithMetadataKey(context.Context, string) ([]*models.Task, error)
}

type taskMovedLifecyclePrerequisites struct {
	session    *models.TaskSession
	fromStep   *wfmodels.WorkflowStep
	targetStep *wfmodels.WorkflowStep
}

const (
	turnCompletionCauseAgentTurn        turnCompletionCause = "agent_turn"
	turnCompletionCauseUserCancellation turnCompletionCause = "user_cancellation"
)

// processOnTurnComplete processes the on_turn_complete events for the current step.
// Returns true if a transition occurred (step change happened).
func (s *Service) processOnTurnComplete(ctx context.Context, task *models.Task, session *models.TaskSession) bool {
	return s.processOnTurnCompleteWithCause(ctx, task, session, turnCompletionCauseAgentTurn)
}

func (s *Service) processOnTurnCompleteWithCause(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	cause turnCompletionCause,
) bool {
	if s.shouldSkipLegacyTurnCompletion(task, session, cause) {
		return false
	}

	taskID := task.ID
	sessionID := session.ID
	currentStep, ok := s.loadLegacyTurnCompletionStep(ctx, task, session)
	if !ok || !s.shouldRunLegacyTurnCompletion(ctx, task, session, currentStep, cause) {
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
	if cause == turnCompletionCauseUserCancellation {
		ctx = cancellationTransitionAttribution(ctx)
	}
	s.executeStepTransition(ctx, taskID, sessionID, currentStep, targetStepID, true)
	return true
}

func (s *Service) shouldSkipLegacyTurnCompletion(
	task *models.Task,
	session *models.TaskSession,
	cause turnCompletionCause,
) bool {
	if task == nil || session == nil || session.ID == "" || s.workflowStepGetter == nil {
		return true
	}
	if cause != turnCompletionCauseUserCancellation {
		return false
	}
	if isTerminalSessionState(session.State) {
		return true
	}
	if task.IsEphemeral || task.IsFromOffice || taskArchived(task) {
		s.logger.Debug("skipping user cancellation completion for ineligible task",
			zap.String("task_id", task.ID))
		return true
	}
	return false
}

func (s *Service) loadLegacyTurnCompletionStep(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
) (*wfmodels.WorkflowStep, bool) {
	if task.WorkflowStepID == "" {
		s.logger.Debug("task has no workflow step, skipping transition",
			zap.String("session_id", session.ID))
		return nil, false
	}
	currentStep, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || currentStep == nil {
		s.logger.Warn("failed to get workflow step for transition",
			zap.String("workflow_step_id", task.WorkflowStepID),
			zap.Error(err))
		return nil, false
	}
	return currentStep, true
}

func (s *Service) shouldRunLegacyTurnCompletion(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	currentStep *wfmodels.WorkflowStep,
	cause turnCompletionCause,
) bool {
	if len(currentStep.Events.OnTurnComplete) == 0 {
		s.logger.Debug("step has no on_turn_complete actions, waiting for user",
			zap.String("step_id", currentStep.ID),
			zap.String("step_name", currentStep.Name))
		s.setSessionWaitingForInput(ctx, task.ID, session.ID, session)
		return false
	}
	if cause == turnCompletionCauseUserCancellation && !currentStep.CancelTriggersTurnComplete {
		s.logger.Debug("user cancellation completion is disabled for step",
			zap.String("step_id", currentStep.ID),
			zap.String("step_name", currentStep.Name))
		return false
	}
	if s.turnCompleteBlockedByUserInput(ctx, task.ID, session.ID, session) {
		return false
	}
	if cause != turnCompletionCauseUserCancellation && currentStep.AutoAdvanceRequiresSignal {
		signal, has := models.LoadPendingStepSignal(session.Metadata)
		if !has || signal.StepID != currentStep.ID {
			s.logger.Debug("on_turn_complete gated on explicit signal (legacy path)",
				zap.String("step_id", currentStep.ID))
			s.setSessionWaitingForInput(ctx, task.ID, session.ID, session)
			return false
		}
	}
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

// ProcessOnTurnStartResult reports whether the initiating prompt must wait for
// WIP admission before it can be delivered.
type ProcessOnTurnStartResult struct {
	Queued bool
}

// ProcessOnTurnStart is the public API for triggering on_turn_start events.
// Called by message handlers before sending a prompt to the agent.
func (s *Service) ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) (ProcessOnTurnStartResult, error) {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	ctx = withWorkflowProfileSwitchGuardHeld(ctx, sessionID, "")
	if err := s.waitForCancellationWithGuard(ctx, sessionID, lock.Unlock, lock.Lock); err != nil {
		return ProcessOnTurnStartResult{}, err
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ProcessOnTurnStartResult{}, fmt.Errorf("load session for on_turn_start: %w", err)
	}
	if isTerminalSessionState(session.State) {
		return ProcessOnTurnStartResult{}, &executor.SessionStateSupersededError{
			SessionID: session.ID,
			State:     session.State,
		}
	}
	// ADR 0015 — a fresh user message before the pending signal's
	// transition has fired cancels the signal (re-open semantics). The
	// user is continuing the conversation; this step is no longer "done".
	s.clearPendingStepSignal(ctx, session)
	s.processOnTurnStartViaEngine(ctx, taskID, session)
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		// Do not let a read race turn an unknown admission state into an
		// immediate prompt. The caller can retry after reconciliation.
		return ProcessOnTurnStartResult{Queued: true}, fmt.Errorf("load task admission after on_turn_start: %w", err)
	}
	return ProcessOnTurnStartResult{
		Queued: task != nil && !task.WIPAdmitted && task.QueuedForStepID != "",
	}, nil
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
	// Atomically admit the target step before exit side effects. A full target
	// is represented as a durable destination queue entry instead of a failed
	// transition.
	task.WorkflowStepID = toStepID
	task.UpdatedAt = time.Now().UTC()
	// executeStepTransition's two callers are always a genuine session turn:
	// on_turn_complete (triggerOnEnter=true) or on_turn_start (false).
	legacyTrigger := engine.TriggerOnTurnStart
	if triggerOnEnter {
		legacyTrigger = engine.TriggerOnTurnComplete
	}
	transitionCtx := engineTransitionAttribution(ctx, sessionID, legacyTrigger)
	if err := s.updateTransitionTaskWithCapacity(transitionCtx, task, targetStep); err != nil {
		s.logger.Warn("workflow transition rejected or failed",
			zap.String("task_id", taskID),
			zap.String("from_step", fromStep.Name),
			zap.String("to_step", targetStep.Name),
			zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}
	queued := task.QueuedForStepID != ""

	// Process on_exit only after the transition is durably admitted. Freshly
	// load the session since the caller may not have it (legacy path).
	exitSession, exitErr := s.repo.GetTaskSession(ctx, sessionID)
	if exitErr != nil {
		s.logger.Warn("failed to load session for on_exit",
			zap.String("session_id", sessionID), zap.Error(exitErr))
	} else {
		s.processOnExit(ctx, taskID, exitSession, fromStep)
	}

	// Publish task updated event via the task service so the payload carries
	// the full context (session counts, primary session, repositories).
	s.publishTaskUpdated(ctx, task)
	if !queued {
		s.processParentChildrenCompletedForTerminalStepMove(ctx, taskID, toStepID)
	}

	s.logger.Info("workflow transition completed",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step", fromStep.Name),
		zap.String("to_step", targetStep.Name),
		zap.Bool("trigger_on_enter", triggerOnEnter))

	if s.workflowStore != nil {
		s.workflowStore.pullNextTaskOnVacate(ctx, fromStep.ID, taskID)
	}
	if queued {
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return
	}

	// ADR 0015 — record the audit row before the pending signal (if any)
	// is cleared below. Only the triggerOnEnter=true (legacy
	// on_turn_complete) branch could have consumed a signal; the
	// on_turn_start branch records with no signal metadata.
	var consumedSignal *models.PendingStepCompletionSignal
	if triggerOnEnter && exitSession != nil {
		if signal, has := models.LoadPendingStepSignal(exitSession.Metadata); has && signal.StepID == fromStep.ID {
			consumedSignal = &signal
		}
	}
	trigger := wfmodels.StepTransitionTriggerTurnStart
	if triggerOnEnter {
		trigger = wfmodels.StepTransitionTriggerAutoComplete
	}
	s.recordAutoStepTransition(ctx, sessionID, fromStep.ID, toStepID, consumedSignal, trigger)

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
			fromStep,
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
		effectiveSession, ok := s.maybySwitchSessionForProfile(ctx, taskID, currentSession, targetStep, fromStep)
		if !ok {
			return
		}
		s.setSessionWaitingForInput(ctx, taskID, effectiveSession.ID)
	}
}

func (s *Service) updateTransitionTaskWithCapacity(
	ctx context.Context,
	task *models.Task,
	targetStep *wfmodels.WorkflowStep,
) error {
	if targetStep == nil {
		return s.repo.UpdateTask(ctx, task)
	}
	admissionRepo, ok := s.repo.(workflowMoveAdmissionRepository)
	if !ok {
		return fmt.Errorf("workflow step admission repository unavailable for step %s", targetStep.ID)
	}
	_, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStep.ID, targetStep.WIPLimit)
	return err
}

// engineTriggerIsSessionOriginated reports whether an engine.Trigger is
// caused by a session's own turn (or an on_exit/on_enter reached through
// one). Every other trigger — a children-completed rollup, a scheduled
// evaluation, or any future non-session trigger — is system-caused even when
// the call site had to resolve *a* session to satisfy the engine's
// MachineState API shape, as processOnChildrenCompleted does.
func engineTriggerIsSessionOriginated(trigger engine.Trigger) bool {
	switch trigger {
	case engine.TriggerOnTurnStart, engine.TriggerOnTurnComplete, engine.TriggerOnEnter, engine.TriggerOnExit:
		return true
	default:
		return false
	}
}

// engineTransitionAttribution wraps ctx with the engine_transition trigger.
// An engine_transition is agent when it came from a session's turn
// (on_turn_start, on_turn_complete, or an on_exit/on_enter reached through
// one — the sessionID this function receives) and system when it came from a
// non-session trigger such as a children-completed rollup or a scheduled
// evaluation. Actor kind is decided by the engine trigger itself, not by
// sessionID presence alone: on_children_completed always resolves a session
// (the parent's active one) purely to satisfy the engine's MachineState API
// shape, and that session did not cause the transition.
//
// Does not overwrite an attribution the caller already set explicitly (the
// same outermost-caller-wins rule ApplyTransition uses) — executeStepTransition's
// legacy on_turn_complete path pre-wraps ctx with cancellationTransitionAttribution
// when the completion was forced by a cancellation, and that must survive.
func engineTransitionAttribution(ctx context.Context, sessionID string, trigger engine.Trigger) context.Context {
	if steptelemetry.HasTrigger(ctx) {
		return ctx
	}
	attribution := steptelemetry.Attribution{Trigger: steptelemetry.TriggerEngineTransition, ActorKind: steptelemetry.ActorSystem}
	if sessionID != "" && engineTriggerIsSessionOriginated(trigger) {
		attribution.ActorKind = steptelemetry.ActorAgent
		attribution.ActorID = sessionID
		attribution.SessionID = sessionID
	}
	return steptelemetry.WithAttribution(ctx, attribution)
}

// cancellationTransitionAttribution wraps ctx with the user_cancellation
// trigger for an on_turn_complete that only ran because a caller force-closed
// the turn (turnCompletionCauseUserCancellation), rather than the turn
// completing on its own — reached through the same code path an
// engine_transition is, but not attributable to the session's agent. Actor
// kind follows the same identity-on-context rule as manual_move: human with
// the cancelling request's user ID when authenticated, system with no ID
// otherwise. Deliberately does not set SessionID — the session is where the
// cancellation landed, not who caused it.
func cancellationTransitionAttribution(ctx context.Context) context.Context {
	actorKind, actorID := steptelemetry.HumanOrSystemActor(ctx)
	return steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerUserCancellation,
		ActorKind: actorKind,
		ActorID:   actorID,
	})
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
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil {
		return
	}
	queued := data.QueuedForStepID != "" && !data.WIPAdmitted
	manualBarrier := !queued && manualMoveLifecyclePending(task)
	prerequisites, ok := s.loadTaskMovedLifecyclePrerequisites(ctx, data, queued, manualBarrier)
	if !ok {
		return
	}
	if queued {
		if task.WorkflowStepID != data.ToStepID || task.QueuedForStepID != data.QueuedForStepID || task.WIPAdmitted {
			return
		}
		if !s.ensureQueuedMoveExitDescriptor(ctx, task, data.FromStepID) {
			return
		}
	} else if task.WorkflowStepID != data.ToStepID {
		// The move event is published after the task write. A replay for a
		// later move must not re-run the older destination lifecycle.
		return
	}
	queuePromotionToken := queuePromotionLifecycleToken(task)
	if data.QueuePromotion && !s.claimTaskEventMetadata(ctx, task, models.MetaKeyQueuePromotionPending) {
		return
	}

	if !queued {
		s.processParentChildrenCompletedForTerminalStepMove(ctx, data.TaskID, data.ToStepID)
	}

	// No session yet — check if we need to create one via auto-start
	if data.SessionID == "" {
		if queued {
			// A task without a session has no source-session side effect to
			// run. Persist the completed no-op so promotion is not blocked by
			// the queued-move barrier.
			if s.persistQueuedMoveExitCompletion(ctx, task.ID) {
				s.clearManualMoveLifecyclePending(ctx, task.ID)
				s.continueQueuedMoveLifecycle(ctx, task.ID, data.FromStepID)
			}
			return
		}
		if manualBarrier {
			if s.persistManualMoveLifecycleCompletion(ctx, task.ID) {
				s.continueManualMoveLifecycle(ctx, task.ID)
			}
			return
		}
		if prerequisites != nil && prerequisites.targetStep != nil {
			s.autoStartTaskForLoadedStep(ctx, task, prerequisites.targetStep, "task.moved", data.QueuePromotion, data.StepTransitionID)
		} else {
			s.handleTaskMovedNoSession(ctx, data)
		}
		return
	}

	if prerequisites != nil && prerequisites.session != nil {
		s.handleTaskMovedWithLoadedSession(ctx, data, prerequisites.session, prerequisites.fromStep, prerequisites.targetStep, manualBarrier, queuePromotionToken)
		return
	}
	s.handleTaskMovedWithBarrier(ctx, data, manualBarrier, queuePromotionToken)
}

func (s *Service) loadTaskMovedLifecyclePrerequisites(
	ctx context.Context,
	data watcher.TaskMovedEventData,
	queued bool,
	manualBarrier bool,
) (*taskMovedLifecyclePrerequisites, bool) {
	if !queued && !data.QueuePromotion && !manualBarrier {
		return nil, true
	}
	prerequisites := &taskMovedLifecyclePrerequisites{}
	if data.QueuePromotion || manualBarrier {
		targetStep, ok := s.loadTaskMovedPromotionTarget(ctx, data)
		if !ok {
			return nil, false
		}
		prerequisites.targetStep = targetStep
	}
	if data.SessionID == "" {
		return prerequisites, true
	}
	session, fromStep, ok := s.loadTaskMovedSessionPrerequisites(ctx, data)
	if !ok {
		return nil, false
	}
	prerequisites.session = session
	prerequisites.fromStep = fromStep
	return prerequisites, true
}

func (s *Service) loadTaskMovedPromotionTarget(ctx context.Context, data watcher.TaskMovedEventData) (*wfmodels.WorkflowStep, bool) {
	targetStep, err := s.workflowStepGetter.GetStep(ctx, data.ToStepID)
	if err != nil || targetStep == nil {
		s.logger.Warn("task.moved: failed to load promotion target step",
			zap.String("task_id", data.TaskID),
			zap.String("step_id", data.ToStepID),
			zap.Error(err))
		return nil, false
	}
	return targetStep, true
}

func (s *Service) loadTaskMovedSessionPrerequisites(
	ctx context.Context,
	data watcher.TaskMovedEventData,
) (*models.TaskSession, *wfmodels.WorkflowStep, bool) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		s.logger.Warn("task.moved: failed to load lifecycle session",
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return nil, nil, false
	}
	fromStep, err := s.workflowStepGetter.GetStep(ctx, data.FromStepID)
	if err != nil || fromStep == nil {
		s.logger.Warn("task.moved: failed to load lifecycle source step",
			zap.String("task_id", data.TaskID),
			zap.String("step_id", data.FromStepID),
			zap.Error(err))
		return nil, nil, false
	}
	return session, fromStep, true
}

// handleTaskMovedNoSession handles the case where a task is moved but has no session.
// If the target step has auto_start_agent, it creates a session and starts the agent
// using agent/executor profile IDs from the task's metadata.
func (s *Service) handleTaskMovedNoSession(ctx context.Context, data watcher.TaskMovedEventData) {
	s.autoStartTaskForStep(ctx, data.TaskID, data.ToStepID, "task.moved", data.StepTransitionID)
}

func (s *Service) handleTaskQueuePromoted(ctx context.Context, data watcher.TaskEventData) {
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil {
		s.logger.Warn("task.queue_promoted: failed to load task", zap.String("task_id", data.TaskID), zap.Error(err))
		return
	}
	if task.QueuedForStepID != "" || !task.WIPAdmitted {
		return
	}
	if queuedMoveExitPending(task) || manualMoveLifecyclePending(task) {
		// A manual move has not finished its lifecycle yet.
		// Keep the promotion token durable; source-exit completion will trigger
		// queue reconciliation and retry destination entry.
		return
	}
	targetStep, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || targetStep == nil {
		s.logger.Warn("task.queue_promoted: failed to load target step",
			zap.String("task_id", task.ID), zap.String("step_id", task.WorkflowStepID), zap.Error(err))
		return
	}
	if err := s.syncTaskStateForQueuePromotion(ctx, task, targetStep); err != nil {
		s.logger.Warn("task.queue_promoted: failed to synchronize task state",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	session, sessionErr := s.repo.GetActiveTaskSessionByTaskID(ctx, task.ID)
	if errors.Is(sessionErr, models.ErrTaskSessionNotFound) {
		// Queued tasks deliberately have no session until promotion. Continue
		// into the no-session destination-entry path so deferred auto-start can run.
		session = nil
		sessionErr = nil
	}
	if sessionErr != nil {
		s.logger.Warn("task.queue_promoted: failed to load active session",
			zap.String("task_id", task.ID), zap.Error(sessionErr))
		return
	}
	if session == nil && s.dependencyBlocksAutoStart(ctx, task.ID, "task.queue_promoted") {
		// Promotion controls WIP admission, not dependency readiness. Keep both
		// the promotion lifecycle token and deferred launch intent intact so the
		// dependency-resolution path can start the task once its blockers clear.
		return
	}
	// Read the source descriptor before claiming the one-shot promotion token.
	// The claim removes the marker durably and a repository implementation may
	// return a freshly materialized task on a later read; source ownership must
	// be captured while this delivery still owns the descriptor.
	sourceStep, sourceErr := s.loadQueuePromotionSourceStep(ctx, task)
	if sourceErr != nil {
		s.logger.Warn("task.queue_promoted: failed to load source step",
			zap.String("task_id", task.ID), zap.Error(sourceErr))
		return
	}
	queuePromotionToken := queuePromotionLifecycleToken(task)
	if !s.claimTaskEventMetadata(ctx, task, models.MetaKeyQueuePromotionPending) {
		return
	}
	if remover, ok := s.repo.(taskMetadataKeyRemover); ok {
		_, _ = remover.RemoveTaskMetadataKey(ctx, task.ID, models.MetaKeyQueuedMoveExitCompleted)
	}
	s.processParentChildrenCompletedForTerminalStepMove(ctx, task.ID, targetStep.ID)
	if session != nil {
		go func() {
			if err := s.finalizeStepEnter(context.WithoutCancel(ctx), task.ID, session.ID, targetStep, task.Description, targetStep.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent), sourceStep); err != nil {
				s.restoreTaskLifecycleToken(context.WithoutCancel(ctx), task.ID, models.MetaKeyQueuePromotionPending, queuePromotionToken, "task.queue_promoted")
				return
			}
			if s.onTaskQueuePromotionEntryComplete != nil {
				s.onTaskQueuePromotionEntryComplete()
			}
		}()
		return
	}
	s.autoStartTaskForLoadedStep(ctx, task, targetStep, "task.queue_promoted", true, data.StepTransitionID)
}

// handleTaskCreated lets a task that is created directly onto a step whose
// on_enter carries auto_start_agent actually launch. Every other trigger
// that reaches autoStartTaskForStep represents a task ENTERING a step via a
// transition (task.moved, task.queue_promoted, dependency resolution); a
// freshly created task enters its start step too, but nothing evaluated
// on_enter for it before this handler existed.
//
// The guard is a POSITIVE opt-in (models.HasAutoStartOnCreateIntent), not an
// absence check. An earlier version launched unless the task carried
// MetaKeyDeferredLaunch, treating every other absence as "no launch opinion,
// please auto-start" — but REST/MCP/WS creates without start_agent or
// prepare_session never set that key either, so a task explicitly created
// without a launch request got auto-started anyway. Requiring an explicit
// MetaKeyAutoStartOnCreate marker means only a producer that actually wants
// create-time on_enter evaluation gets it (today: CreateOfficeTaskInWorkflow,
// for materialized heavy-routine runs); every other producer's silence is
// left exactly as before.
//
// Office tasks (task.IsFromOffice) are excluded even when opted in: the
// office subscriber's own task.created handler already queues a run for
// every office task using a different idempotency key than
// autoStartOfficeTaskForLoadedStep's, so both firing would double-queue the
// same task. No current opt-in producer creates office tasks — heavy
// routines are never IsFromOffice — but the exclusion is explicit rather
// than incidental on that non-overlap.
//
// No MetaKeyDeferredLaunch check is needed here: repository.CreateTask
// unconditionally clears that key (models.DropWIPDeferredLaunch) for any
// task admitted directly onto a step, which is every task this handler
// fires for — a fresh task lands here precisely because it wasn't queued.
// A task that IS queued (QueuedForStepID != "") keeps the key, but
// autoStartTaskForStep's own leading check already returns before doing
// anything with it. Either way the key can never change this handler's
// outcome, so checking it here would be dead code.
//
// The opt-in itself is claimed via claimTaskEventMetadata before launching,
// the same one-shot-token pattern used for MetaKeyQueuePromotionPending: a
// duplicate task.created delivery for the same task would otherwise see the
// key still present and launch a second time. Only the delivery that wins
// the atomic RemoveTaskMetadataKey proceeds.
func (s *Service) handleTaskCreated(ctx context.Context, data watcher.TaskEventData) {
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil {
		if err != nil {
			s.logger.Warn("task.created: failed to load task", zap.String("task_id", data.TaskID), zap.Error(err))
		}
		return
	}
	if !autoStartOnCreateActionable(task) {
		return
	}
	if !s.claimTaskEventMetadata(ctx, task, models.MetaKeyAutoStartOnCreate) {
		return
	}
	s.autoStartTaskForStep(ctx, task.ID, task.WorkflowStepID, events.TaskCreated, data.StepTransitionID)
}

func (s *Service) claimTaskEventMetadata(ctx context.Context, task *models.Task, key string) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	if _, marked := task.Metadata[key]; !marked {
		// One-shot lifecycle events use metadata removal as their claim. Its
		// absence means another delivery already claimed the event.
		return false
	}
	remover, ok := s.repo.(taskMetadataKeyRemover)
	if !ok {
		s.logger.Warn("task lifecycle event cannot be claimed: repository lacks metadata-key removal",
			zap.String("task_id", task.ID), zap.String("metadata_key", key))
		return false
	}
	claimed, err := remover.RemoveTaskMetadataKey(ctx, task.ID, key)
	if err != nil {
		s.logger.Warn("failed to claim task lifecycle event", zap.String("task_id", task.ID), zap.String("metadata_key", key), zap.Error(err))
		return false
	}
	return claimed
}

func (s *Service) restoreTaskLifecycleToken(ctx context.Context, taskID, key string, value interface{}, eventName string) {
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		s.logger.Warn(eventName+": repository cannot restore lifecycle token",
			zap.String("task_id", taskID), zap.String("metadata_key", key))
		return
	}
	if value == nil {
		value = true
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, key, value); err != nil {
		s.logger.Warn(eventName+": failed to restore lifecycle token",
			zap.String("task_id", taskID), zap.String("metadata_key", key), zap.Error(err))
	}
}

func queuePromotionLifecycleToken(task *models.Task) interface{} {
	if task != nil && task.Metadata != nil {
		if value, ok := task.Metadata[models.MetaKeyQueuePromotionPending]; ok {
			return value
		}
	}
	return true
}

// lifecycleSweepStuckWarningInterval controls how long
// reconcileTaskLifecycleTokens waits before logging a "still recovering"
// warning while the sweep is still in flight. It is a var so tests can
// shorten it. This is a log-only signal: it never cancels the sweep, because
// the resume calls it drives run under context.WithoutCancel
// (task_operations.go), so cancellation would not work anyway.
var lifecycleSweepStuckWarningInterval = 60 * time.Second

// reconcileTaskLifecycleTokens scans durable lifecycle markers at startup.
// A small fixed worker pool and bounded per-task attempts prevent a corrupted
// or repeatedly failing row from creating an unbounded goroutine storm.
func (s *Service) reconcileTaskLifecycleTokens(ctx context.Context) {
	lister, ok := s.repo.(lifecycleTaskMetadataLister)
	if !ok {
		return
	}
	pending, err := lister.ListTasksWithMetadataKey(ctx, models.MetaKeyQueuedMoveExitPending)
	if err != nil {
		s.logger.Warn("failed to list queued move exit tokens for recovery", zap.Error(err))
		return
	}
	promotions, err := lister.ListTasksWithMetadataKey(ctx, models.MetaKeyQueuePromotionPending)
	if err != nil {
		s.logger.Warn("failed to list queue promotion tokens for recovery", zap.Error(err))
		return
	}
	manualPending, err := lister.ListTasksWithMetadataKey(ctx, models.MetaKeyManualMoveLifecyclePending)
	if err != nil {
		s.logger.Warn("failed to list manual move lifecycle tokens for recovery", zap.Error(err))
		return
	}
	manualCompleted, err := lister.ListTasksWithMetadataKey(ctx, models.MetaKeyManualMoveLifecycleCompleted)
	if err != nil {
		s.logger.Warn("failed to list completed manual move lifecycle tokens for recovery", zap.Error(err))
		return
	}
	autoStarts, err := lister.ListTasksWithMetadataKey(ctx, models.MetaKeyAutoStartOnCreate)
	if err != nil {
		s.logger.Warn("failed to list auto-start-on-create tokens for recovery", zap.Error(err))
		return
	}
	// autoStarts is the raw, pre-filter list; only entries that pass
	// autoStartOnCreateActionable below are added to jobs, so it is left out
	// of this capacity hint rather than causing routine over-allocation.
	jobs := make(map[string]struct{}, len(pending)+len(promotions)+len(manualPending)+len(manualCompleted))
	for _, task := range pending {
		if task != nil {
			jobs[task.ID] = struct{}{}
		}
	}
	for _, task := range promotions {
		if task != nil {
			jobs[task.ID] = struct{}{}
		}
	}
	for _, task := range manualPending {
		if task != nil {
			jobs[task.ID] = struct{}{}
		}
	}
	for _, task := range manualCompleted {
		if task != nil {
			jobs[task.ID] = struct{}{}
		}
	}
	for _, task := range autoStarts {
		// ListTasksWithMetadataKey matches key EXISTENCE, which is broader
		// than what handleTaskCreated will act on. Rows it refuses keep their
		// key forever, so admitting them would schedule work that can never
		// converge, on every single startup.
		if autoStartOnCreateActionable(task) {
			jobs[task.ID] = struct{}{}
		}
	}
	if len(jobs) == 0 {
		return
	}

	start := time.Now()
	taskCount := len(jobs)
	s.logger.Info("startup lifecycle sweep starting", zap.Int("task_count", taskCount))

	var processed atomic.Int64
	sweepDone := make(chan struct{})
	warnDone := make(chan struct{})
	go func() {
		defer close(warnDone)
		s.warnIfLifecycleSweepStuck(taskCount, &processed, start, sweepDone)
	}()

	jobIDs := make(chan string)
	workerCount := 4
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for taskID := range jobIDs {
				s.recoverTaskLifecycleToken(ctx, taskID)
				processed.Add(1)
			}
		}()
	}
	for taskID := range jobs {
		jobIDs <- taskID
	}
	close(jobIDs)
	wg.Wait()
	close(sweepDone)
	<-warnDone

	s.logger.Info("startup lifecycle sweep finished",
		zap.Int("task_count", taskCount),
		zap.Duration("elapsed", time.Since(start)))
}

// warnIfLifecycleSweepStuck logs a warning if the startup lifecycle sweep is
// still running after lifecycleSweepStuckWarningInterval, so a slow sweep is
// diagnosable from logs instead of requiring a goroutine dump. It is
// log-only: it never cancels the sweep, and it always exits (either the
// timer fires once, or done closes first).
func (s *Service) warnIfLifecycleSweepStuck(taskCount int, processed *atomic.Int64, start time.Time, done <-chan struct{}) {
	timer := time.NewTimer(lifecycleSweepStuckWarningInterval)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		s.logger.Warn("startup lifecycle sweep still running",
			zap.Int("task_count", taskCount),
			zap.Int64("processed", processed.Load()),
			zap.Duration("elapsed", time.Since(start)))
	}
}

func (s *Service) recoverTaskLifecycleToken(ctx context.Context, taskID string) {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if !s.recoverTaskLifecycleAttempt(ctx, taskID) {
			return
		}
		if attempt+1 == maxAttempts {
			return
		}
		if !waitForLifecycleRecovery(ctx) {
			return
		}
	}
}

func (s *Service) recoverTaskLifecycleAttempt(ctx context.Context, taskID string) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	if queuedMoveExitPending(task) {
		if !s.recoverQueuedMoveExit(ctx, task) {
			return false
		}
		task, err = s.repo.GetTask(ctx, taskID)
		if err != nil || task == nil {
			return false
		}
	}
	if manualMoveLifecyclePending(task) {
		if !s.recoverManualMoveLifecycle(ctx, task) {
			return false
		}
		task, err = s.repo.GetTask(ctx, taskID)
		if err != nil || task == nil {
			return false
		}
	}
	if manualMoveLifecycleCompleted(task) {
		s.continueManualMoveLifecycle(ctx, taskID)
	}
	if _, pending := task.Metadata[models.MetaKeyQueuePromotionPending]; pending {
		s.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: taskID})
	}
	if autoStartOnCreateActionable(task) {
		s.recoverAutoStartOnCreate(ctx, task)
	}
	latest, err := s.repo.GetTask(ctx, taskID)
	if err != nil || latest == nil {
		return false
	}
	return queuedMoveExitPending(latest) || manualMoveLifecyclePending(latest) ||
		manualMoveLifecycleCompleted(latest) || hasQueuePromotionPending(latest) ||
		autoStartOnCreateActionable(latest)
}

func (s *Service) recoverQueuedMoveExit(ctx context.Context, task *models.Task) bool {
	sourceStepID := queuedMoveExitSourceStep(task)
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, task.ID)
	if sourceStepID == "" || err != nil || session == nil || s.workflowStepGetter == nil {
		return false
	}
	fromStep, err := s.workflowStepGetter.GetStep(ctx, sourceStepID)
	if err != nil || fromStep == nil {
		return false
	}
	// Startup recovery is already running in a bounded worker. Run this
	// serialized lifecycle operation directly so the next attempt observes
	// its durable completion instead of racing an unbounded detached goroutine.
	s.processQueuedMoveExit(ctx, task.ID, session, fromStep, sourceStepID)
	return true
}

func (s *Service) recoverManualMoveLifecycle(ctx context.Context, task *models.Task) bool {
	if task == nil || s.workflowStepGetter == nil {
		return false
	}
	sourceStepID := manualMoveLifecycleSourceStep(task)
	if sourceStepID == "" {
		return false
	}
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, task.ID)
	if err != nil {
		return false
	}
	if session == nil {
		if !s.persistManualMoveLifecycleCompletion(ctx, task.ID) {
			return false
		}
		s.continueManualMoveLifecycle(ctx, task.ID)
		return true
	}
	fromStep, err := s.workflowStepGetter.GetStep(ctx, sourceStepID)
	if err != nil || fromStep == nil {
		return false
	}
	targetStep, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || targetStep == nil {
		return false
	}
	// Startup recovery is already bounded by the caller's worker pool. Run the
	// lifecycle directly so the next attempt observes durable completion.
	s.processManualMoveLifecycleWithFeederBarrier(
		ctx, task.ID, session, fromStep, targetStep,
		sourceStepID, task.WorkflowStepID, task.Description,
	)
	return true
}

func hasQueuePromotionPending(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, pending := task.Metadata[models.MetaKeyQueuePromotionPending]
	return pending
}

func (s *Service) loadWorkflowStepForLifecycle(ctx context.Context, stepID, role string) (*wfmodels.WorkflowStep, error) {
	if stepID == "" {
		return nil, fmt.Errorf("%s workflow step ID is empty", role)
	}
	if s.workflowStepGetter == nil {
		return nil, fmt.Errorf("workflow step getter is unavailable for %s workflow step %q", role, stepID)
	}
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		return nil, fmt.Errorf("load %s workflow step %q: %w", role, stepID, err)
	}
	if step == nil {
		return nil, fmt.Errorf("%s workflow step %q was not found", role, stepID)
	}
	return step, nil
}

func (s *Service) loadQueuePromotionSourceStep(ctx context.Context, task *models.Task) (*wfmodels.WorkflowStep, error) {
	sourceStepID := queuePromotionSourceStep(task)
	if sourceStepID == "" {
		return nil, nil
	}
	return s.loadWorkflowStepForLifecycle(ctx, sourceStepID, "queue promotion source")
}

func queuePromotionSourceStep(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	value, ok := task.Metadata[models.MetaKeyQueuePromotionPending]
	if !ok {
		return ""
	}
	descriptor, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	sourceStepID, _ := descriptor["from_step_id"].(string)
	return sourceStepID
}

// autoStartOnCreateActionable reports whether the startup sweep can still act
// on a task's create-time auto-start opt-in (models.MetaKeyAutoStartOnCreate).
//
// It must stay identical to handleTaskCreated's own guard, because the sweep
// both admits jobs and decides "retry" with it. ListTasksWithMetadataKey
// matches key existence, but handleTaskCreated requires a positive bool and
// skips office tasks, and it returns BEFORE claiming the key in either case.
// Treating existence alone as "pending" therefore produced a token no code
// path could clear: recovery saw it still present, reported retry, and burned
// the whole attempt budget on every startup, with the row re-listed on the
// next one indefinitely. Every other key in this sweep is cleared by its
// handler, and docs/specs/startup-listener-before-recovery/spec.md attributes
// a non-converging boot loop to lifecycle tokens that stay pending, so a token
// class the sweep cannot drain is a regression rather than a cosmetic mismatch.
func autoStartOnCreateActionable(task *models.Task) bool {
	if task == nil || task.IsFromOffice {
		return false
	}
	return models.HasAutoStartOnCreateIntent(task.Metadata)
}

// recoverAutoStartOnCreate replays a lost task.created delivery for a task that
// still carries an actionable create-time opt-in.
//
// The opt-in is NOT proof that no launch happened. handleTaskCreated is the
// only function that claims this key; a manual StartTask, a task.moved
// auto-start and handleTaskQueuePromoted all launch without touching it. So
// after a lost delivery — the very failure this sweep repairs — an operator
// starting the task by hand leaves the token behind, and replaying it here
// would launch a second agent onto a task that is already running or already
// finished. Neither autoStartTaskForStep nor startTask has an existing-session
// guard, so that launch would go all the way through.
func (s *Service) recoverAutoStartOnCreate(ctx context.Context, task *models.Task) {
	sessions, err := s.repo.ListTaskSessions(ctx, task.ID)
	if err != nil {
		// Leave the token untouched. The caller's exit check still sees it as
		// actionable and spends one of its bounded attempts retrying.
		s.logger.Warn("failed to check existing sessions before auto-start-on-create recovery",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	if len(sessions) > 0 {
		// The task was started by one of those other paths, so this one-shot
		// token is spent. Claim it rather than merely skipping, so the row
		// converges instead of being re-scanned on every future startup.
		if s.claimTaskEventMetadata(ctx, task, models.MetaKeyAutoStartOnCreate) {
			s.logger.Info("discarded spent auto-start-on-create token: task already has a session",
				zap.String("task_id", task.ID), zap.Int("session_count", len(sessions)))
		}
		return
	}
	// Re-enter handleTaskCreated exactly like a live delivery would, so the
	// office/opt-in/claim guards run once, in one place.
	s.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: task.ID})
}

func waitForLifecycleRecovery(ctx context.Context) bool {
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func queuedMoveExitSourceStep(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	value, ok := task.Metadata[models.MetaKeyQueuedMoveExitPending]
	if !ok {
		return ""
	}
	if descriptor, ok := value.(map[string]interface{}); ok {
		source, _ := descriptor["from_step_id"].(string)
		return source
	}
	return ""
}

func (s *Service) syncTaskStateForQueuePromotion(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil {
		return nil
	}
	next, err := s.workflowStepGetter.GetNextStepByPosition(ctx, targetStep.WorkflowID, targetStep.Position)
	if err != nil {
		return fmt.Errorf("load next step after promoted step %s: %w", targetStep.ID, err)
	}
	oldState := task.State
	if wfmodels.IsTerminalStep(targetStep, next) {
		if !models.IsTerminalTaskState(task.State) {
			task.State = v1.TaskStateCompleted
		}
	} else if task.State == v1.TaskStateCompleted {
		task.State = v1.TaskStateTODO
	}
	if oldState == task.State {
		return nil
	}
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("persist promoted task state: %w", err)
	}
	s.publishTaskUpdated(ctx, task)
	s.publishTaskStateChanged(ctx, task, oldState)
	return nil
}

func (s *Service) autoStartTaskForStep(ctx context.Context, taskID, stepID, eventName string, stepTransitionID int64) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn(eventName+": failed to load task for auto-start",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if task == nil || task.QueuedForStepID != "" {
		return
	}
	// Dependency gate. Sits here — the single automated-launch chokepoint — so
	// one check covers task.moved, task.queue_promoted, watcher auto-start and
	// dependency resolution itself. Placed BEFORE launchDeferredTask so a
	// blocked task neither starts a session nor consumes its launch intent.
	if s.dependencyBlocksAutoStart(ctx, taskID, eventName) {
		return
	}
	if hasQueuePromotionPending(task) {
		// A dependency may resolve after same-step WIP promotion was admitted.
		// Resume through the promotion handler so destination-entry lifecycle is
		// claimed and its token participates in deferred-launch failure recovery.
		s.handleTaskQueuePromoted(ctx, watcher.TaskEventData{
			TaskID: task.ID, StepTransitionID: stepTransitionID,
		})
		return
	}
	if task != nil && s.shouldSkipTerminalPRAutoStart(ctx, task) {
		return
	}
	if s.launchDeferredTask(ctx, task, eventName, false) {
		return
	}

	// Load the target step to check auto-start and plan mode flags
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		s.logger.Warn(eventName+": failed to load target step",
			zap.String("task_id", taskID),
			zap.String("to_step_id", stepID),
			zap.Error(err))
		return
	}
	s.autoStartTaskForLoadedStep(ctx, task, step, eventName, false, stepTransitionID)
}

func (s *Service) autoStartTaskForLoadedStep(ctx context.Context, task *models.Task, step *wfmodels.WorkflowStep, eventName string, restoreQueuePromotion bool, stepTransitionID int64) {
	if task == nil || task.QueuedForStepID != "" || step == nil {
		return
	}
	if s.shouldSkipTerminalPRAutoStart(ctx, task) {
		return
	}
	if s.launchDeferredTask(ctx, task, eventName, restoreQueuePromotion) {
		return
	}
	if step == nil || !step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		s.logger.Debug(eventName+": target step has no auto-start",
			zap.String("task_id", task.ID),
			zap.String("to_step_id", step.ID))
		return
	}

	if s.isOfficeTask(ctx, task.ID) {
		s.autoStartOfficeTaskForLoadedStep(ctx, task, step, eventName, restoreQueuePromotion, stepTransitionID)
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
	queuePromotionToken := queuePromotionLifecycleToken(task)

	s.logger.Info(eventName+": starting task (no session, auto-start step)",
		zap.String("task_id", task.ID),
		zap.String("to_step_id", step.ID),
		zap.String("agent_profile_id", agentProfileID),
		zap.String("executor_id", executorID),
		zap.String("executor_profile_id", executorProfileID),
		zap.Bool("plan_mode", planMode))

	// Async: event bus delivers synchronously; blocking here → HTTP timeout (see handleTaskMovedWithSession doc).
	go func() {
		asyncCtx := context.WithoutCancel(ctx)

		// When the task has the permanent auto-start guard marker, it means
		// both this promotion path and the watcher's synchronous path (autoStartReviewTask)
		// can fire. Only the first atomic claim of MetaKeyAutoStartClaimed wins;
		// the loser skips launch because the winner will handle it.
		// Absent guard = ordinary (non-watcher) auto-start; proceed as before.
		_, hasGuard := task.Metadata[models.MetaKeyAutoStartGuard]
		if hasGuard {
			if !s.claimAutoStart(asyncCtx, task.ID, eventName) {
				s.logger.Debug(eventName+": auto-start claim lost; watcher path will launch",
					zap.String("task_id", task.ID))
				return
			}
		}

		startAgentProfileID := agentProfileID
		if workflowAgentProfileID != "" {
			startAgentProfileID = ""
		}
		_, err := s.StartTask(asyncCtx, task.ID, startAgentProfileID, executorID, executorProfileID, "", task.Description, step.ID, planMode, true, nil)
		if err != nil {
			s.logger.Error(eventName+": failed to auto-start task",
				zap.String("task_id", task.ID),
				zap.Error(err))
			s.handleAutoStartFailure(asyncCtx, task.ID, eventName, hasGuard, restoreQueuePromotion, queuePromotionToken)
		}
	}()
}

// autoStartOfficeTaskForLoadedStep is the Office-aware counterpart of the
// kanban launch path above. An Office task has no session-based launch route
// (see validateOfficeRuntimeEnv) — StartTask can never succeed for it, it
// just logs "office runtime context is incomplete" and starts nothing. Queue
// a run through the engine's Office adapters instead, the same mechanism the
// scheduler's recovery sweep and the "assign task" flow already use to start
// Office work (internal/office/service/scheduler_recovery.go).
func (s *Service) autoStartOfficeTaskForLoadedStep(ctx context.Context, task *models.Task, step *wfmodels.WorkflowStep, eventName string, restoreQueuePromotion bool, stepTransitionID int64) {
	queuePromotionToken := queuePromotionLifecycleToken(task)
	// Async for the same reason as the kanban path above: the event bus
	// delivers synchronously and blocking here would stall the HTTP handler
	// that published the move.
	go func() {
		asyncCtx := context.WithoutCancel(ctx)

		_, hasGuard := task.Metadata[models.MetaKeyAutoStartGuard]
		if hasGuard {
			if !s.claimAutoStart(asyncCtx, task.ID, eventName) {
				s.logger.Debug(eventName+": auto-start claim lost; watcher path will launch",
					zap.String("task_id", task.ID))
				return
			}
		}

		outcome, err := s.queueOfficeAutoStartRun(asyncCtx, task, step, stepTransitionID)
		if err != nil {
			s.logger.Error(eventName+": failed to queue office auto-start run",
				zap.String("task_id", task.ID),
				zap.Error(err))
			s.handleAutoStartFailure(asyncCtx, task.ID, eventName, hasGuard, restoreQueuePromotion, queuePromotionToken)
			return
		}
		// Log the outcome only after the attempt actually resolves — the log
		// site this replaced ran before the goroutine below it, so it
		// asserted a queued run whether or not one was. QueueOutcomeQueued
		// is the only case that means a new runs row was actually inserted;
		// deduped/coalesced are logged at Debug so a re-entry that is
		// correctly suppressed within its own window does not read as a
		// launch failure.
		if outcome == engine.QueueOutcomeQueued {
			s.logger.Info(eventName+": queued office run (no session, auto-start step)",
				zap.String("task_id", task.ID),
				zap.String("to_step_id", step.ID))
			return
		}
		s.logger.Debug(eventName+": office auto-start run not queued",
			zap.String("task_id", task.ID),
			zap.String("to_step_id", step.ID),
			zap.String("outcome", string(outcome)))
	}()
}

// officeAutoStartRunReason mirrors office/service.RunReasonTaskAssigned
// ("task_assigned"). Kept as a literal rather than importing
// internal/office/service: the two packages intentionally avoid importing
// each other (see runsServiceEngineAdapter in backendapp/main.go), so a
// shared reason string is duplicated rather than pulled across the boundary.
const officeAutoStartRunReason = "task_assigned"

// queueOfficeAutoStartRun makes auto_start_agent Office-aware: instead of
// StartTask (kanban-only), it resolves an agent the same way a "primary"
// queue_run target would — preferring the task's current runner participant,
// falling back to its assignee — and queues a run through engineRunQueue.
func (s *Service) queueOfficeAutoStartRun(ctx context.Context, task *models.Task, step *wfmodels.WorkflowStep, stepTransitionID int64) (engine.QueueOutcome, error) {
	if s.engineRunQueue == nil {
		return "", fmt.Errorf("office run queue is not wired")
	}
	agentProfileID := task.AssigneeAgentProfileID
	if s.enginePrimary != nil {
		resolved, err := s.enginePrimary.PrimaryAgentProfileID(ctx, step.ID, task.ID)
		if err != nil {
			return "", fmt.Errorf("resolve primary agent for office auto-start on task %s: %w", task.ID, err)
		}
		if resolved != "" {
			agentProfileID = resolved
		}
	}
	if agentProfileID == "" {
		return "", fmt.Errorf("no agent profile resolved for office auto-start on task %s", task.ID)
	}
	return s.engineRunQueue.QueueRun(ctx, engine.QueueRunRequest{
		AgentProfileID: agentProfileID,
		TaskID:         task.ID,
		WorkflowStepID: step.ID,
		Reason:         officeAutoStartRunReason,
		IdempotencyKey: officeAutoStartIdempotencyKey(task, agentProfileID, step.ID, stepTransitionID),
		Payload:        map[string]any{"task_id": task.ID},
	})
}

// officeAutoStartIdempotencyKey uses the immutable workflow-step transition
// row as the per-entry component. A legacy event without that field uses the
// task timestamp as a compatibility fallback until the event is republished.
func officeAutoStartIdempotencyKey(task *models.Task, agentProfileID, stepID string, stepTransitionID int64) string {
	entryID := strconv.FormatInt(stepTransitionID, 10)
	if stepTransitionID == 0 {
		entryID = "legacy:" + task.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		officeAutoStartRunReason, task.ID, agentProfileID, stepID, entryID)
}

// handleAutoStartFailure records a failed auto-start attempt. It restores the
// MetaKeyAutoStartClaimed token only when hasGuard is true: claimAutoStart is
// only ever called (and the token only ever taken) when the task carries
// MetaKeyAutoStartGuard, so restoring it unconditionally stamps
// auto_start_claimed onto tasks that never carried the guard, corrupting the
// invariant documented at MetaKeyAutoStartGuard. It also stamps
// MetaKeyAutoStartFailed so the failure surfaces on the task card instead of
// only in backend logs.
func (s *Service) handleAutoStartFailure(ctx context.Context, taskID, eventName string, hasGuard, restoreQueuePromotion bool, queuePromotionToken interface{}) {
	if hasGuard {
		s.restoreAutoStartClaim(ctx, taskID, eventName)
	}
	if restoreQueuePromotion {
		s.restoreTaskLifecycleToken(ctx, taskID, models.MetaKeyQueuePromotionPending, queuePromotionToken, eventName)
	}
	s.setTaskAutoStartFailedMarker(ctx, taskID, eventName)
}

func (s *Service) launchDeferredTask(ctx context.Context, task *models.Task, eventName string, restoreQueuePromotion bool) bool {
	if task.Metadata == nil {
		return false
	}
	raw, ok := task.Metadata[models.MetaKeyDeferredLaunch]
	if !ok {
		return false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var intent struct {
		Intent            string                 `json:"intent"`
		AgentProfileID    string                 `json:"agent_profile_id"`
		ExecutorID        string                 `json:"executor_id"`
		ExecutorProfileID string                 `json:"executor_profile_id"`
		UserID            string                 `json:"user_id"`
		RecordRecentUse   bool                   `json:"record_recent_use"`
		Prompt            string                 `json:"prompt"`
		PlanMode          bool                   `json:"plan_mode"`
		Attachments       []v1.MessageAttachment `json:"attachments"`
	}
	if err := json.Unmarshal(encoded, &intent); err != nil || intent.AgentProfileID == "" {
		s.logger.Warn(eventName+": invalid deferred launch intent", zap.String("task_id", task.ID), zap.Error(err))
		return true
	}
	launchIntent := IntentStart
	if intent.Intent == "prepare" {
		launchIntent = IntentPrepare
	}
	go func() {
		launchCtx := context.WithoutCancel(ctx)
		metadataClaimed, claimOK := s.claimDeferredLaunch(launchCtx, task.ID, eventName)
		if !claimOK {
			return
		}
		launchResp, launchErr := s.LaunchSession(launchCtx, &LaunchSessionRequest{
			TaskID: task.ID, Intent: launchIntent, AgentProfileID: intent.AgentProfileID,
			ExecutorID: intent.ExecutorID, ExecutorProfileID: intent.ExecutorProfileID,
			WorkflowStepID: task.WorkflowStepID, Prompt: intent.Prompt,
			PlanMode: intent.PlanMode, Attachments: intent.Attachments, LaunchWorkspace: true,
		})
		if launchErr != nil || launchResp == nil || !launchResp.Success {
			if launchErr == nil {
				launchErr = errors.New("deferred launch returned an unsuccessful response")
			}
			s.logger.Error(eventName+": failed to launch deferred task", zap.String("task_id", task.ID), zap.Error(launchErr))
			s.restoreDeferredLaunch(launchCtx, task.ID, raw, eventName, metadataClaimed)
			if restoreQueuePromotion {
				s.restoreTaskLifecycleToken(launchCtx, task.ID, models.MetaKeyQueuePromotionPending, queuePromotionLifecycleToken(task), eventName)
			}
			return
		}
		if intent.RecordRecentUse {
			s.recordSuccessfulDeferredTaskProfileAsync(launchCtx, intent.UserID, launchResp.AgentProfileID)
		}
		delete(task.Metadata, models.MetaKeyDeferredLaunch)
		if metadataClaimed {
			task.UpdatedAt = time.Now().UTC()
			s.publishTaskUpdated(launchCtx, task)
			return
		}
		task.UpdatedAt = time.Now().UTC()
		if updateErr := s.repo.UpdateTask(launchCtx, task); updateErr == nil {
			s.publishTaskUpdated(launchCtx, task)
		}
	}()
	return true
}

func (s *Service) claimDeferredLaunch(ctx context.Context, taskID, eventName string) (bool, bool) {
	remover, ok := s.repo.(taskMetadataKeyRemover)
	if !ok {
		return false, true
	}
	claimed, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyDeferredLaunch)
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn(eventName+": failed to claim deferred launch", zap.String("task_id", taskID), zap.Error(err))
		}
		return false, false
	}
	return true, true
}

func (s *Service) restoreDeferredLaunch(ctx context.Context, taskID string, raw interface{}, eventName string, claimed bool) {
	if !claimed {
		return
	}
	setter, ok := s.repo.(interface {
		SetTaskMetadataKey(context.Context, string, string, interface{}) error
	})
	if !ok {
		return
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyDeferredLaunch, raw); err != nil {
		s.logger.Warn(eventName+": failed to restore deferred launch intent", zap.String("task_id", taskID), zap.Error(err))
	}
}

// claimAutoStart atomically removes the MetaKeyAutoStartClaimed token from a
// task. It returns true only for the first caller that removes the key.
// Both Path A (event-driven autoStartTaskForStep) and Path B (watcher's
// synchronous autoStartReviewTask) must call this when the token is present;
// only the winner proceeds to StartTask. When the token is absent (ordinary
// non-watcher auto-start), the caller skips the claim and launches as before.
func (s *Service) claimAutoStart(ctx context.Context, taskID, eventName string) bool {
	remover, ok := s.repo.(taskMetadataKeyRemover)
	if !ok {
		return true // repo doesn't support atomic removal; allow launch
	}
	claimed, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyAutoStartClaimed)
	if err != nil {
		s.logger.Warn(eventName+": failed to claim auto-start token",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return claimed
}

// restoreAutoStartClaim puts the MetaKeyAutoStartClaimed token back when a
// launch fails so a later event trigger can retry. Mirrors restoreDeferredLaunch.
func (s *Service) restoreAutoStartClaim(ctx context.Context, taskID, eventName string) {
	setter, ok := s.repo.(interface {
		SetTaskMetadataKey(context.Context, string, string, interface{}) error
	})
	if !ok {
		return
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyAutoStartClaimed, true); err != nil {
		s.logger.Warn(eventName+": failed to restore auto-start claim",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// setTaskAutoStartFailedMarker stamps MetaKeyAutoStartFailed on a task and
// republishes task.updated so the failure surfaces on the card. Cleared by
// clearTaskAutoStartFailedMarker when a session for the task next reaches
// STARTING/RUNNING.
func (s *Service) setTaskAutoStartFailedMarker(ctx context.Context, taskID, eventName string) {
	setter, ok := s.repo.(taskMetadataKeySetterIfNoActiveSession)
	if !ok {
		s.logger.Warn(eventName+": repository cannot conditionally set auto-start-failed marker",
			zap.String("task_id", taskID))
		return
	}
	written, err := setter.SetTaskMetadataKeyIfNoActiveSession(ctx, taskID, models.MetaKeyAutoStartFailed, true)
	if err != nil {
		s.logger.Warn(eventName+": failed to set auto-start-failed marker",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if !written {
		s.logger.Debug(eventName+": skipped auto-start-failed marker because task work is active",
			zap.String("task_id", taskID))
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		s.logger.Warn(eventName+": failed to load task for auto-start-failed publish",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	s.publishTaskUpdated(ctx, task)
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
	s.handleTaskMovedWithBarrier(ctx, data, false, nil)
}

func (s *Service) handleTaskMovedWithBarrier(ctx context.Context, data watcher.TaskMovedEventData, manualBarrier bool, queuePromotionToken interface{}) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		s.logger.Warn("task.moved: failed to load session",
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return
	}
	s.fromStepAndTargetForTaskMoved(ctx, data, session, nil, nil, manualBarrier, queuePromotionToken)
}

func (s *Service) handleTaskMovedWithLoadedSession(
	ctx context.Context,
	data watcher.TaskMovedEventData,
	session *models.TaskSession,
	fromStep, targetStep *wfmodels.WorkflowStep,
	manualBarrier bool,
	queuePromotionToken interface{},
) {
	s.fromStepAndTargetForTaskMoved(ctx, data, session, fromStep, targetStep, manualBarrier, queuePromotionToken)
}

func (s *Service) fromStepAndTargetForTaskMoved(
	ctx context.Context,
	data watcher.TaskMovedEventData,
	session *models.TaskSession,
	fromStep, targetStep *wfmodels.WorkflowStep,
	manualBarrier bool,
	queuePromotionToken interface{},
) {
	if session == nil {
		return
	}

	if data.QueuedForStepID != "" && !data.WIPAdmitted {
		go s.processQueuedMoveExit(
			context.WithoutCancel(ctx), data.TaskID, session, fromStep, data.FromStepID,
		)
		return
	}
	if manualBarrier {
		go s.processManualMoveLifecycleWithFeederBarrier(
			context.WithoutCancel(ctx), data.TaskID, session, fromStep, targetStep,
			data.FromStepID, data.ToStepID, data.TaskDescription,
		)
		return
	}
	go func() {
		if err := s.processStepExitAndEnterWithSteps(
			context.WithoutCancel(ctx), data.TaskID, session, fromStep, targetStep,
			data.FromStepID, data.ToStepID, data.TaskDescription, data.QueuePromotion, queuePromotionToken,
		); err != nil {
			s.logger.Warn("task.moved: step exit and enter lifecycle failed",
				zap.String("task_id", data.TaskID),
				zap.String("from_step_id", data.FromStepID),
				zap.String("to_step_id", data.ToStepID),
				zap.Error(err))
		}
	}()
}

func (s *Service) processStepExit(ctx context.Context, taskID string, session *models.TaskSession, fromStepID string) {
	fromStep, err := s.loadWorkflowStepForLifecycle(ctx, fromStepID, "queued move source")
	if err != nil {
		s.logger.Warn("failed to load from-step for queued move on_exit",
			zap.String("step_id", fromStepID), zap.Error(err))
		return
	}
	if err := s.processStepExitWithStep(ctx, taskID, session, fromStep, fromStepID); err != nil {
		s.logger.Warn("failed to process queued move on_exit",
			zap.String("task_id", taskID), zap.String("step_id", fromStepID), zap.Error(err))
	}
}

func (s *Service) processStepExitWithStep(ctx context.Context, taskID string, session *models.TaskSession, fromStep *wfmodels.WorkflowStep, fromStepID string) error {
	if fromStep == nil {
		var err error
		fromStep, err = s.loadWorkflowStepForLifecycle(ctx, fromStepID, "queued move source")
		if err != nil {
			return err
		}
	}
	s.processOnExit(ctx, taskID, session, fromStep)
	return nil
}

// ensureQueuedMoveExitDescriptor records the source step on the durable
// queued-move token. Older rows may contain only true; the event payload lets
// the first delivery upgrade those rows before the asynchronous side effect.
func (s *Service) ensureQueuedMoveExitDescriptor(ctx context.Context, task *models.Task, fromStepID string) bool {
	if task == nil || fromStepID == "" || task.Metadata == nil {
		return false
	}
	if _, completed := task.Metadata[models.MetaKeyQueuedMoveExitCompleted]; completed {
		return false
	}
	value, marked := task.Metadata[models.MetaKeyQueuedMoveExitPending]
	if !marked {
		return false
	}
	if descriptor, ok := value.(map[string]interface{}); ok {
		if source, _ := descriptor["from_step_id"].(string); source == fromStepID {
			return true
		}
	}
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		return false
	}
	if err := setter.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyQueuedMoveExitPending, map[string]interface{}{
		"from_step_id": fromStepID,
	}); err != nil {
		s.logger.Warn("failed to persist queued move source step",
			zap.String("task_id", task.ID), zap.Error(err))
		return false
	}
	return true
}

func queuedMoveExitCompleted(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyQueuedMoveExitCompleted]
	return completed
}

func manualMoveLifecyclePending(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	if _, pending := task.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyManualMoveLifecycleCompleted]
	return !completed
}

func manualMoveLifecycleCompleted(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyManualMoveLifecycleCompleted]
	return completed
}

func manualMoveLifecycleSourceStep(task *models.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	value, ok := task.Metadata[models.MetaKeyManualMoveLifecyclePending]
	if !ok {
		return ""
	}
	if descriptor, ok := value.(map[string]interface{}); ok {
		source, _ := descriptor["from_step_id"].(string)
		return source
	}
	return ""
}

// processQueuedMoveExit runs the source-step side effect exactly once per
// task. The in-memory lock serializes duplicate event deliveries, while the
// pending/completed metadata pair makes the ordering recoverable after a
// process restart.
func (s *Service) processQueuedMoveExit(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	fromStep *wfmodels.WorkflowStep,
	fromStepID string,
) {
	lockValue, _ := s.queuedMoveLifecycleLocks.LoadOrStore(taskID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || queuedMoveExitCompleted(task) {
		return
	}
	if _, pending := task.Metadata[models.MetaKeyQueuedMoveExitPending]; !pending {
		return
	}
	if sourceStepID := queuedMoveExitSourceStep(task); sourceStepID != "" && sourceStepID != fromStepID {
		// A stale delivery from an earlier move must not execute against the
		// marker for a newer queued move on the same task.
		return
	}
	if s.onQueuedMoveExitStart != nil {
		s.onQueuedMoveExitStart()
	}
	if err := s.processStepExitWithStep(ctx, taskID, session, fromStep, fromStepID); err != nil {
		s.logger.Warn("failed to process queued move on_exit",
			zap.String("task_id", taskID), zap.String("step_id", fromStepID), zap.Error(err))
		return
	}
	if !s.persistQueuedMoveExitCompletion(ctx, taskID) {
		return
	}
	s.clearManualMoveLifecyclePending(ctx, taskID)
	if s.onQueuedMoveExitComplete != nil {
		s.onQueuedMoveExitComplete()
	}
	s.continueQueuedMoveLifecycle(ctx, taskID, fromStepID)
}

func (s *Service) persistQueuedMoveExitCompletion(ctx context.Context, taskID string) bool {
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		s.logger.Warn("queued move source-exit completion cannot be persisted",
			zap.String("task_id", taskID))
		return false
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyQueuedMoveExitCompleted, true); err != nil {
		s.logger.Warn("failed to persist queued move source-exit completion",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	if remover, ok := s.repo.(taskMetadataKeyRemover); ok {
		if _, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyQueuedMoveExitPending); err != nil {
			s.logger.Warn("failed to clear queued move source-exit token",
				zap.String("task_id", taskID), zap.Error(err))
		}
	}
	return true
}

func (s *Service) clearManualMoveLifecyclePending(ctx context.Context, taskID string) {
	remover, ok := s.repo.(taskMetadataKeyRemover)
	if !ok {
		return
	}
	if _, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyManualMoveLifecyclePending); err != nil {
		s.logger.Warn("failed to clear manual move lifecycle token",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

func (s *Service) persistManualMoveLifecycleCompletion(ctx context.Context, taskID string) bool {
	setter, ok := s.repo.(taskMetadataKeySetter)
	if !ok {
		s.logger.Warn("manual move lifecycle completion cannot be persisted",
			zap.String("task_id", taskID))
		return false
	}
	if err := setter.SetTaskMetadataKey(ctx, taskID, models.MetaKeyManualMoveLifecycleCompleted, true); err != nil {
		s.logger.Warn("failed to persist manual move lifecycle completion",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	s.clearManualMoveLifecyclePending(ctx, taskID)
	return true
}

func (s *Service) continueManualMoveLifecycle(ctx context.Context, taskID string) {
	latest, err := s.repo.GetTask(ctx, taskID)
	if err != nil || latest == nil {
		return
	}
	if !manualMoveLifecycleCompleted(latest) {
		return
	}
	if s.feederPulls == nil {
		s.logger.Warn("manual move lifecycle has no feeder reconciler",
			zap.String("task_id", taskID))
		return
	}
	s.feederPulls.ReconcileFeederPulls(ctx, latest.WorkflowID, latest.WorkflowStepID)
}

// processManualMoveLifecycleWithFeederBarrier runs the original move lifecycle
// before waking feeder pulls. The per-task lock and durable pending/completed
// markers make duplicate deliveries and restart recovery safe.
func (s *Service) processManualMoveLifecycleWithFeederBarrier(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	fromStep, targetStep *wfmodels.WorkflowStep,
	fromStepID, toStepID, taskDescription string,
) {
	lockValue, _ := s.queuedMoveLifecycleLocks.LoadOrStore(taskID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || !manualMoveLifecyclePending(task) {
		return
	}
	if sourceStepID := manualMoveLifecycleSourceStep(task); sourceStepID != "" && sourceStepID != fromStepID {
		return
	}
	if task.WorkflowStepID != toStepID {
		return
	}
	if s.onManualMoveLifecycleStart != nil {
		s.onManualMoveLifecycleStart()
	}
	if err := s.processStepExitAndEnterWithSteps(
		ctx, taskID, session, fromStep, targetStep,
		fromStepID, toStepID, taskDescription, false, nil,
	); err != nil {
		s.logger.Warn("manual move lifecycle stopped before completion",
			zap.String("task_id", taskID), zap.String("from_step_id", fromStepID),
			zap.String("to_step_id", toStepID), zap.Error(err))
		return
	}
	if !s.persistManualMoveLifecycleCompletion(ctx, taskID) {
		return
	}
	s.continueManualMoveLifecycle(ctx, taskID)
}

func (s *Service) continueQueuedMoveLifecycle(ctx context.Context, taskID, vacatedStepID string) {
	latest, err := s.repo.GetTask(ctx, taskID)
	if err != nil || latest == nil {
		return
	}
	if hasQueuePromotionPending(latest) {
		s.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: taskID})
	} else if latest.QueuedForStepID != "" && s.workflowStore != nil && vacatedStepID != "" {
		s.workflowStore.pullNextTaskOnVacate(ctx, vacatedStepID, "")
	}
}

// processStepExitAndEnter runs the on_exit → clear review → reload session → on_enter
// sequence for a step transition. Used by handleTaskMovedWithSession (where MoveTask
// already persisted the step change in the DB).
func (s *Service) processStepExitAndEnter(ctx context.Context, taskID string, session *models.TaskSession, fromStepID, toStepID, taskDescription string) error {
	// Process on_exit for the step we're leaving
	if err := s.processStepExitAndEnterWithSteps(ctx, taskID, session, nil, nil, fromStepID, toStepID, taskDescription, false, nil); err != nil {
		s.logger.Warn("step exit and enter lifecycle failed",
			zap.String("task_id", taskID), zap.String("from_step_id", fromStepID),
			zap.String("to_step_id", toStepID), zap.Error(err))
		return err
	}
	return nil
}

func (s *Service) processStepExitAndEnterWithSteps(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	fromStep, targetStep *wfmodels.WorkflowStep,
	fromStepID, toStepID, taskDescription string, queuePromotion bool, queuePromotionToken interface{},
) error {
	if fromStep == nil {
		var err error
		fromStep, err = s.loadWorkflowStepForLifecycle(ctx, fromStepID, "transition source")
		if err != nil {
			if queuePromotion {
				s.restoreTaskLifecycleToken(ctx, taskID, models.MetaKeyQueuePromotionPending, queuePromotionToken, "task.moved source lookup")
			}
			return err
		}
	}
	s.processOnExit(ctx, taskID, session, fromStep)

	if targetStep == nil {
		var err error
		targetStep, err = s.loadWorkflowStepForLifecycle(ctx, toStepID, "transition target")
		if err != nil {
			if queuePromotion {
				s.restoreTaskLifecycleToken(ctx, taskID, models.MetaKeyQueuePromotionPending, queuePromotionToken, "task.moved target lookup")
			}
			return err
		}
	}

	clearReview := targetStep.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent)
	if err := s.finalizeStepEnter(ctx, taskID, session.ID, targetStep, taskDescription, clearReview, fromStep); err != nil {
		if queuePromotion {
			s.restoreTaskLifecycleToken(ctx, taskID, models.MetaKeyQueuePromotionPending, queuePromotionToken, "task.moved")
		}
		return err
	}
	return nil
}

// finalizeStepEnter optionally clears review status, reloads the session, and
// processes on_enter actions for the target step. Shared by executeStepTransition
// and processStepExitAndEnter.
func (s *Service) finalizeStepEnter(ctx context.Context, taskID, sessionID string, targetStep *wfmodels.WorkflowStep, taskDescription string, clearReview bool, sourceStep *wfmodels.WorkflowStep) error {
	if clearReview {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return fmt.Errorf("clear session review status: %w", err)
		}
	}

	// Reload session after on_exit may have changed metadata
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to load session for on_enter",
			zap.String("session_id", sessionID), zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, sessionID)
		return fmt.Errorf("load session for on_enter: %w", err)
	}

	// entryID 0: this path (manual move / legacy on_turn_start/complete) does
	// not attach a step-entry ResultHolder before ApplyTransition, so no
	// entry was allocated for this step-entry. processOnEnter's engine-owned
	// action cases treat entryID==0 as "not this Build round's dispatch
	// path" and skip with a log rather than executing — see
	// docs/specs/workflow-on-enter-action-dispatch/spec.md and the task
	// plan's scope note for why E2-E5 dispatch is deferred.
	s.processOnEnter(ctx, taskID, session, targetStep, taskDescription, 0, sourceStep)
	return nil
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
		meta, err := s.getWorkflowMeta(ctx, step.WorkflowID)
		if err != nil {
			s.logger.Warn("failed to resolve workflow agent profile, falling back to task defaults",
				zap.String("workflow_id", step.WorkflowID),
				zap.String("step_id", step.ID),
				zap.Error(err))
		} else if meta.AgentProfileID != "" {
			return meta.AgentProfileID
		}
	}
	return ""
}

// resolveStepProfileSessionStartPolicy returns the destination step's session
// start behavior. Invalid or absent values use the safe reuse default.
func (s *Service) resolveStepProfileSessionStartPolicy(step *wfmodels.WorkflowStep) models.WorkflowProfileSessionStartPolicy {
	if step == nil {
		return models.WorkflowProfileSessionStartPolicyReuse
	}
	return models.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy))
}

// resolveStepProfileSessionEndPolicy returns the source step's session end
// behavior. Invalid or absent values use the safe complete default.
func (s *Service) resolveStepProfileSessionEndPolicy(step *wfmodels.WorkflowStep) models.WorkflowProfileSessionEndPolicy {
	if step == nil {
		return models.WorkflowProfileSessionEndPolicyComplete
	}
	return models.NormalizeWorkflowProfileSessionEndPolicy(string(step.ProfileSessionEndPolicy))
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
// If a nonterminal session on this task already uses the target profile, it
// is reused (re-promoted to primary). Otherwise a new session is prepared —
// including when the only matching session is terminal (COMPLETED, FAILED,
// or CANCELLED): workflow re-entry never resumes a terminal session's ACP
// conversation, because prior-completion state in that conversation can
// mislead the agent into replaying stale routing intent (see
// findReusableSessionForProfile). In both cases the previous session is
// stopped and either marked COMPLETED or parked according to the source
// step's end policy.
func (s *Service) switchSessionForStep(ctx context.Context, taskID string, currentSession *models.TaskSession, newAgentProfileID string) (*models.TaskSession, error) {
	return s.switchSessionForStepWithPolicies(
		ctx,
		taskID,
		currentSession,
		newAgentProfileID,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyComplete,
	)
}

func (s *Service) switchSessionForStepWithPolicies(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	newAgentProfileID string,
	startPolicy models.WorkflowProfileSessionStartPolicy,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (*models.TaskSession, error) {
	startPolicy = models.NormalizeWorkflowProfileSessionStartPolicy(string(startPolicy))
	endPolicy = models.NormalizeWorkflowProfileSessionEndPolicy(string(endPolicy))
	s.logger.Info("switching session for workflow step agent profile change",
		zap.String("task_id", taskID),
		zap.String("current_session", currentSession.ID),
		zap.String("current_profile", currentSession.AgentProfileID),
		zap.String("new_profile", newAgentProfileID),
		zap.String("profile_session_start_policy", string(startPolicy)),
		zap.String("profile_session_end_policy", string(endPolicy)))
	var existing *models.TaskSession
	if startPolicy == models.WorkflowProfileSessionStartPolicyReuse {
		var lookupErr error
		existing, lookupErr = s.findReusableSessionForProfile(ctx, taskID, newAgentProfileID, currentSession.ID)
		if lookupErr != nil {
			s.logger.Warn("failed to look up reusable session, falling through to create new",
				zap.String("task_id", taskID),
				zap.String("agent_profile_id", newAgentProfileID),
				zap.Error(lookupErr))
		}
	}
	targetSession := currentSession
	if existing != nil {
		targetSession = existing
	}

	// Validate managed-credential repository bindings before mutating either
	// session's ownership. Reusing or replacing the current session only to
	// discover at launch that a repository binding is irreparable would leave
	// the task stuck on a doomed session with no recovery path back to the one
	// that was still working.
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task for session switch preflight: %w", err)
	}
	if dbTask == nil {
		return nil, fmt.Errorf("task %s not found for session switch preflight", taskID)
	}
	if err := s.executor.PreflightManagedGitCredentials(
		ctx, dbTask.WorkspaceID, taskID, targetSession.ExecutorID, targetSession.ExecutorProfileID,
	); err != nil {
		return nil, err
	}

	// Signal to the frontend that the task is preparing a new agent.
	if err := s.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); err != nil {
		s.logger.Warn("failed to set task SCHEDULING during agent switch",
			zap.String("task_id", taskID), zap.Error(err))
	}

	if existing != nil {
		reused, err := s.reuseSessionForStepWithEndPolicy(ctx, taskID, currentSession, existing, endPolicy)
		if err == nil {
			return reused, nil
		}
		if !errors.Is(err, errReusableSessionNoLongerActive) {
			return nil, err
		}
		s.logger.Info("reusable session became terminal before workflow promotion; creating fresh session",
			zap.String("task_id", taskID),
			zap.String("session_id", existing.ID),
			zap.String("agent_profile_id", newAgentProfileID))
	}

	return s.createNewSessionForStepWithEndPolicy(ctx, taskID, currentSession, newAgentProfileID, endPolicy)
}

// findReusableSessionForProfile returns the most-recently-updated
// *nonterminal* session on this task that uses the target profile (and is
// not the session being switched away from), or nil if none exists.
//
// Terminal sessions (COMPLETED, FAILED, CANCELLED) are always excluded —
// they are historical endpoints, not workflow-reusable. A prior incident
// showed why: reviving a COMPLETED session lazily resumed its persisted ACP
// conversation, which still contained the agent's earlier completion state.
// Seeing the task routed back to that step, the agent reasonably inferred
// its prior completion had been cancelled and moved the task backward,
// re-arming the same cycle on the next re-entry. Terminal-profile re-entry
// always goes through createNewSessionForStep instead, which gets a fresh
// ACP conversation and the canonical current task/workflow context.
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
		if isTerminalSessionState(sess.State) {
			continue
		}
		if best == nil || sess.UpdatedAt.After(best.UpdatedAt) {
			best = sess
		}
	}
	return best, nil
}

func (s *Service) reuseSessionForStepWithEndPolicy(
	ctx context.Context,
	taskID string,
	currentSession, existing *models.TaskSession,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (*models.TaskSession, error) {
	endPolicy = models.NormalizeWorkflowProfileSessionEndPolicy(string(endPolicy))
	s.logger.Info("reusing existing session for profile",
		zap.String("task_id", taskID),
		zap.String("current_session", currentSession.ID),
		zap.String("reused_session", existing.ID),
		zap.String("reused_profile", existing.AgentProfileID),
		zap.String("reused_state", string(existing.State)))

	promoted, err := s.setNonterminalSessionPrimary(ctx, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("conditional primary promotion: %w", err)
	}
	if !promoted {
		return nil, errReusableSessionNoLongerActive
	}
	if task, taskErr := s.repo.GetTask(ctx, taskID); taskErr != nil {
		s.logger.Warn("failed to load task after promoting reused session",
			zap.String("task_id", taskID), zap.Error(taskErr))
	} else if task != nil {
		s.publishTaskUpdated(ctx, task)
	}
	s.tagSessionAsWorkflowSwitched(ctx, existing.ID)

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

	parked, err := s.finishWorkflowProfileSwitchSource(ctx, taskID, currentSession, endPolicy)
	if err != nil {
		if endPolicy == models.WorkflowProfileSessionEndPolicyPark && !parked && currentSession.IsPrimary {
			s.restoreWorkflowProfileSwitchSourcePrimary(ctx, currentSession)
			s.restoreWorkflowProfileSwitchSourceQueue(ctx, existing.ID, currentSession.ID)
		}
		return nil, err
	}
	return existing, nil
}

// setNonterminalSessionPrimary promotes a workflow-reused session only when
// its persisted state is still nonterminal.
func (s *Service) setNonterminalSessionPrimary(ctx context.Context, sessionID string) (bool, error) {
	return s.repo.SetSessionPrimaryIfNonterminal(ctx, sessionID)
}

// createNewSessionForStep is the original switch-and-create-fresh-session path,
// used when there is no existing session for the target profile.
func (s *Service) createNewSessionForStep(ctx context.Context, taskID string, currentSession *models.TaskSession, newAgentProfileID string) (*models.TaskSession, error) {
	return s.createNewSessionForStepWithEndPolicy(
		ctx,
		taskID,
		currentSession,
		newAgentProfileID,
		models.WorkflowProfileSessionEndPolicyComplete,
	)
}

func (s *Service) createNewSessionForStepWithEndPolicy(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	newAgentProfileID string,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (*models.TaskSession, error) {
	endPolicy = models.NormalizeWorkflowProfileSessionEndPolicy(string(endPolicy))
	// Prepare the new session BEFORE touching the old one.
	// If any step below fails, the old session remains active and the task stays recoverable.
	newSession, err := s.prepareWorkflowReplacementSession(ctx, taskID, currentSession, newAgentProfileID)
	if err != nil {
		return nil, err
	}

	// Tag the session as workflow-spawned for provenance: its agent profile
	// was selected by the workflow step override rather than direct user choice.
	s.tagSessionAsWorkflowSwitched(ctx, newSession.ID)

	// Promote the new session to primary so it's loaded when navigating back to this task.
	// Use SetPrimarySession (not repo.SetSessionPrimary) to broadcast a task.updated WS
	// event — the frontend reads primarySessionId from the task to render the star icon.
	if err := s.SetPrimarySession(ctx, newSession.ID); err != nil {
		return nil, fmt.Errorf("failed to promote new workflow session: %w", err)
	}

	// Transfer any queued message (e.g. a move_task_kandev hand-off prompt) and
	// pending move from the old session to the new one only after promotion
	// succeeds. If promotion fails, the source remains primary and its queue
	// must remain attached to that source rather than being stranded on an
	// unpromoted destination.
	s.transferWorkflowProfileSwitchQueue(ctx, currentSession.ID, newSession.ID)

	parked, err := s.finishWorkflowProfileSwitchSource(ctx, taskID, currentSession, endPolicy)
	if err != nil {
		if endPolicy == models.WorkflowProfileSessionEndPolicyPark && !parked && currentSession.IsPrimary {
			s.restoreWorkflowProfileSwitchSourcePrimary(ctx, currentSession)
			s.restoreWorkflowProfileSwitchSourceQueue(ctx, newSession.ID, currentSession.ID)
		}
		return nil, err
	}
	return newSession, nil
}

func (s *Service) prepareWorkflowReplacementSession(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	newAgentProfileID string,
) (*models.TaskSession, error) {
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
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.ValidateProfile(ctx, newAgentProfileID); err != nil {
			return nil, fmt.Errorf("failed to validate workflow replacement profile: %w", err)
		}
	}

	// Create a new session with the new agent profile.
	// Reuse the same executor profile from the current session.
	sessionID, err := s.executor.PrepareSessionForExistingEnvironment(ctx, task, newAgentProfileID, currentSession.ExecutorID, currentSession.ExecutorProfileID, dbTask.WorkflowStepID, currentSession.TaskEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare new session: %w", err)
	}
	newSession, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get new session: %w", err)
	}
	launchProfileID, err := s.resolveDynamicLaunchExecution(ctx, newSession, newAgentProfileID, true)
	if err != nil {
		resolutionErr := fmt.Errorf("failed to resolve workflow replacement profile: %w", err)
		if deleteErr := s.repo.DeleteTaskSession(ctx, sessionID); deleteErr != nil {
			s.logger.Warn("failed to delete workflow replacement after profile resolution failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.Error(deleteErr))
			if terminalErr := s.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateFailed, resolutionErr.Error()); terminalErr != nil {
				s.logger.Warn("failed to terminalize workflow replacement after delete failure",
					zap.String("task_id", taskID),
					zap.String("session_id", sessionID),
					zap.Error(terminalErr))
			}
		}
		return nil, resolutionErr
	}
	if _, err := s.executor.LaunchPreparedSession(ctx, task, sessionID, executor.LaunchOptions{
		AgentProfileID: launchProfileID,
		ExecutorID:     currentSession.ExecutorID,
		WorkflowStepID: dbTask.WorkflowStepID,
		StartAgent:     false,
	}); err != nil {
		return nil, fmt.Errorf("failed to attach workflow replacement workspace: %w", err)
	}

	return newSession, nil
}

func (s *Service) transferWorkflowProfileSwitchQueue(ctx context.Context, fromSessionID, toSessionID string) {
	if s.messageQueue == nil {
		return
	}
	if err := s.messageQueue.TransferSession(ctx, fromSessionID, toSessionID); err != nil {
		s.logger.Error("transfer queue to new session failed; queued prompts on the previous session will not be drained",
			zap.String("from_session_id", fromSessionID),
			zap.String("to_session_id", toSessionID),
			zap.Error(err))
		// Continue anyway: the new session is already created and committed
		// upstream. Failing closed here would leave the workflow in a
		// half-switched state (new session exists but caller thinks it
		// failed). The error is surfaced via logs and the orphaned entries
		// stay safely in the old session for manual recovery.
	}
}

func (s *Service) finishWorkflowProfileSwitchSource(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	endPolicy models.WorkflowProfileSessionEndPolicy,
) (bool, error) {
	if endPolicy == models.WorkflowProfileSessionEndPolicyPark {
		return s.parkSessionForProfileSwitch(ctx, taskID, session)
	}
	s.completeAndStopSession(ctx, taskID, session)
	return true, nil
}

func (s *Service) restoreWorkflowProfileSwitchSourcePrimary(ctx context.Context, session *models.TaskSession) {
	if err := s.SetPrimarySession(ctx, session.ID); err != nil {
		s.logger.Warn("failed to restore source session after parked profile switch failure",
			zap.String("session_id", session.ID),
			zap.Error(err))
	}
}

func (s *Service) restoreWorkflowProfileSwitchSourceQueue(ctx context.Context, fromSessionID, toSessionID string) {
	if s.messageQueue == nil {
		return
	}
	if err := s.messageQueue.TransferSession(ctx, fromSessionID, toSessionID); err != nil {
		s.logger.Error("failed to return queued state to restored source session",
			zap.String("from_session_id", fromSessionID),
			zap.String("to_session_id", toSessionID),
			zap.Error(err))
	}
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

// prepareWorkflowStepSession resolves the session that must execute a workflow
// step before any on_enter action runs. Steps without an effective profile keep
// the current session. Profile changes delegate to the existing reuse/create
// lifecycle helpers regardless of the source session transport.
func (s *Service) prepareWorkflowStepSession(
	ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep,
	sourceStep *wfmodels.WorkflowStep,
) (*models.TaskSession, bool, error) {
	if session == nil {
		return nil, false, fmt.Errorf("workflow step session is nil")
	}
	if step == nil {
		return nil, false, fmt.Errorf("workflow step is nil")
	}
	ctx = withWorkflowMetaCache(ctx)
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
		return session, false, nil
	}
	startPolicy := s.resolveStepProfileSessionStartPolicy(step)
	if sourceStep == nil {
		return nil, false, fmt.Errorf("workflow profile switch source step is unavailable")
	}
	endPolicy := s.resolveStepProfileSessionEndPolicy(sourceStep)
	newSession, err := s.switchSessionForStepWithPolicies(ctx, taskID, session, effectiveProfile, startPolicy, endPolicy)
	if err != nil {
		return nil, false, err
	}
	return newSession, true, nil
}

func (s *Service) preflightWorkflowStepCredentials(
	ctx context.Context,
	taskID string,
	currentSession *models.TaskSession,
	targetStep *wfmodels.WorkflowStep,
) error {
	if currentSession == nil || targetStep == nil {
		return nil
	}
	ctx = withWorkflowMetaCache(ctx)
	effectiveProfile := s.resolveStepAgentProfile(ctx, targetStep)
	if effectiveProfile == "" || effectiveProfile == currentSession.AgentProfileID {
		return nil
	}
	startPolicy := s.resolveStepProfileSessionStartPolicy(targetStep)
	targetSession := currentSession
	if startPolicy == models.WorkflowProfileSessionStartPolicyReuse {
		existing, err := s.findReusableSessionForProfile(ctx, taskID, effectiveProfile, currentSession.ID)
		if err != nil {
			return fmt.Errorf("find reusable session for credential preflight: %w", err)
		}
		if existing != nil {
			targetSession = existing
		}
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task for credential preflight: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found for credential preflight", taskID)
	}
	return s.executor.PreflightManagedGitCredentials(
		ctx, task.WorkspaceID, taskID, targetSession.ExecutorID, targetSession.ExecutorProfileID,
	)
}

// maybySwitchSessionForProfile preserves the legacy processOnEnter failure
// handling while sharing the error-returning step-entry preflight with direct
// workflow-engine dispatch.
func (s *Service) maybySwitchSessionForProfile(
	ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep,
	sourceStep *wfmodels.WorkflowStep,
) (*models.TaskSession, bool) {
	effective, _, err := s.prepareWorkflowStepSession(ctx, taskID, session, step, sourceStep)
	if err != nil {
		s.logger.Error("failed to switch session for step agent profile",
			zap.String("task_id", taskID),
			zap.String("step_id", step.ID),
			zap.Error(err))
		if session != nil {
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		}
		return nil, false
	}
	return effective, true
}

// dispatchOnEnterActions runs each on_enter action declared on step, in
// order, and reports whether an auto_start_agent action was among them.
// Extracted from processOnEnter to keep that function's cognitive
// complexity within the repo's lint threshold.
//
// AC-C2: if clear_decisions fails, no further on_enter action for this step
// entry may execute — stale decision rows plus a fresh participant fan-out
// could otherwise satisfy wait_for_quorum without every current-round
// participant actually voting. The loop stops entirely (not just further
// engine-owned actions) the moment that happens.
//
// AC-F1: a caller that loses a *live* claim on an engine-owned position
// (dispatchEngineOwnedOnEnterAction's abandon=true) must abandon the entry
// the same way — it does not skip to a later position, because that later
// action could execute while the winner is still committing an earlier
// one (e.g. enqueueing queue_run_for_each_participant's fan-out before the
// winner's clear_decisions delete commits). This is a normal outcome, not
// a fault, so the loop stops silently (no WARNING/ERROR) rather than via
// AC-C2's error-logged abort.
type onEnterDispatchResult struct {
	hasAutoStart bool
	aborted      bool
}

func (s *Service) dispatchOnEnterActions(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep, entryID int64, isPassthrough, hasPlanMode bool) onEnterDispatchResult {
	result := onEnterDispatchResult{}
dispatchLoop:
	for i, action := range step.Events.OnEnter {
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
			result.hasAutoStart = true
		case wfmodels.OnEnterResetAgentContext, wfmodels.OnEnterConfigureSession:
			// Already handled earlier in processOnEnter (context reset must run
			// before auto_start_agent; session config runs right after it).
		case wfmodels.OnEnterClearDecisions, wfmodels.OnEnterQueueRunForEachParticipant:
			abandon, failed, cause := s.dispatchEngineOwnedOnEnterAction(ctx, taskID, step, action, i, entryID)
			if abandon {
				s.logger.Debug("processOnEnter: lost a live claim to a concurrent dispatch of this step entry, abandoning remaining on_enter actions",
					zap.String("workflow_id", step.WorkflowID),
					zap.String("step_id", step.ID),
					zap.String("task_id", taskID),
					zap.String("action_type", string(action.Type)),
				)
				result.aborted = true
				break dispatchLoop
			}
			if failed && action.Type == wfmodels.OnEnterClearDecisions {
				s.logger.Error("processOnEnter: clear_decisions failed, aborting remaining on_enter actions for step entry",
					zap.String("workflow_id", step.WorkflowID),
					zap.String("step_id", step.ID),
					zap.String("task_id", taskID),
					zap.String("cause", cause),
				)
				result.aborted = true
				break dispatchLoop
			}
		case wfmodels.OnEnterQueueRun, wfmodels.OnEnterRunCodeReview, wfmodels.OnEnterEnsureParticipantSeat:
			// Session-independent, ledger-owned: engine.DispatchStepEntry
			// (internal/workflow/engine/entrydispatch.go) dispatches these
			// synchronously after commit via every step-transition writer
			// (Repository.dispatchStepEntry in step_entry_dispatch.go).
			// processOnEnter must not also dispatch them here or they would
			// run twice — see docs/specs/workflow-on-enter-action-dispatch/spec.md.
			// This is a known, recognized type, not the AC-A6 default warning case.
		default:
			// AC-A6: a genuinely unrecognized on_enter action type. Warn
			// instead of silently discarding it — this is the exact failure
			// mode the step-entry dispatch fix exists to close.
			s.logger.Warn("processOnEnter: unrecognized on_enter action type",
				zap.String("task_id", taskID),
				zap.String("step_id", step.ID),
				zap.String("action_type", string(action.Type)),
			)
		}
	}
	return result
}

// processOnEnter processes the on_enter events for a step after transitioning to it.
// entryID is the workflow_step_entries row allocated for this step-entry by
// the write site that persisted the transition (0 when no entry was
// allocated for this call path — see finalizeStepEnter's call site). Only a
// non-zero entryID lets the engine-owned on_enter cases below dispatch;
// this is this Build round's E1-only scope boundary, not a general
// precondition of the marker CAS mechanism itself.
func (s *Service) processOnEnter(ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep, taskDescription string, entryID int64, sourceStep *wfmodels.WorkflowStep) {
	// One GetWorkflowMeta read shared by profile resolution and prompt build.
	ctx = withWorkflowMetaCache(ctx)

	// Switch session if this step requires a different agent profile.
	var ok bool
	prevSessionID := session.ID
	if session, ok = s.maybySwitchSessionForProfile(ctx, taskID, session, step, sourceStep); !ok {
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

	// Conditional session configuration is applied after a context reset so the
	// new ACP session receives the workflow-selected settings before any
	// auto-start prompt is dispatched. It never switches or creates a tab.
	s.applyWorkflowSessionConfigOnEnter(ctx, taskID, session, step)

	dispatchResult := s.dispatchOnEnterActions(ctx, taskID, session, step, entryID, isPassthrough, hasPlanMode)
	if dispatchResult.aborted {
		return
	}

	s.launchAfterOnEnterDispatch(ctx, taskID, session, step, taskDescription, hasPlanMode, dispatchResult.hasAutoStart, sessionSwitched)
}

// launchAfterOnEnterDispatch runs the auto-start decision that follows
// on_enter action dispatch: passthrough vs. ACP auto-start, the
// profile-switch fallback launch, and the plain wait-for-input path.
// Extracted from processOnEnter to keep that function's complexity within
// the repo's lint thresholds.
//
//nolint:cyclop,funlen,gocognit // profile-switch recovery has independent terminal and retry branches.
func (s *Service) launchAfterOnEnterDispatch(
	ctx context.Context, taskID string, session *models.TaskSession, step *wfmodels.WorkflowStep,
	taskDescription string, hasPlanMode, hasAutoStart, sessionSwitched bool,
) {
	sessionID := session.ID
	isPassthrough := s.agentManager.IsPassthroughSession(ctx, sessionID)

	switch {
	case hasAutoStart && isPassthrough && session.State != models.TaskSessionStateCreated:
		// Started passthrough path: write prompt directly to PTY stdin.
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
			if errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
				if workflowAutoStartWasCancelled(err) {
					s.logger.Info("workflow auto-start cancelled before dispatch; not creating replacement",
						zap.String("task_id", taskID), zap.String("session_id", sessionID))
					return
				}
				s.logger.Info("creating fresh workflow session after reused session terminalized",
					zap.String("task_id", taskID), zap.String("session_id", sessionID))
				replacement, replacementErr := s.createNewSessionForStep(
					ctx, taskID, session, session.AgentProfileID,
				)
				if replacementErr != nil {
					s.logger.Error("failed to create replacement after reused session terminalized",
						zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(replacementErr))
					return
				}
				replacementPrompt := s.buildWorkflowPrompt(
					ctx, taskDescription, step, taskID, replacement.ID, isPassthrough,
				)
				if replacementErr = s.autoStartStepPrompt(
					ctx, taskID, replacement, step, replacementPrompt, hasPlanMode, true,
				); replacementErr != nil {
					s.logger.Error("failed to auto-start replacement after reused session terminalized",
						zap.String("task_id", taskID), zap.String("session_id", replacement.ID), zap.Error(replacementErr))
					s.setSessionWaitingForInput(ctx, taskID, replacement.ID, replacement)
					s.publishSessionWaitingEvent(ctx, taskID, replacement.ID, step.ID, replacement)
				}
				return
			}
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
			//
			// Before dispatching, reload the session under the cancel-in-flight guard
			// to atomically detect concurrent terminalization. If the session is
			// already terminal, create a replacement directly for automatic recovery states instead of attempting
			// an auto-start that will fail and lose the hand-off. An explicit cancellation remains terminal.
			go func() {
				asyncCtx := context.WithoutCancel(ctx)
				lock, release := s.acquireCancelInFlightGuard(sessionID)
				lock.Lock()
				fresh, reloadErr := s.repo.GetTaskSession(asyncCtx, sessionID)
				if reloadErr != nil || fresh == nil || isTerminalSessionState(fresh.State) {
					lock.Unlock()
					release()
					if reloadErr != nil {
						s.logger.Error("implicit profile switch: failed to reload session before dispatch",
							zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(reloadErr))
						return
					}
					if fresh != nil && fresh.State == models.TaskSessionStateCancelled {
						s.logger.Info("implicit profile switch cancelled before dispatch; not creating replacement",
							zap.String("task_id", taskID), zap.String("session_id", sessionID))
						return
					}
					s.logger.Info("implicit profile switch: reused session terminalized before dispatch, creating replacement",
						zap.String("task_id", taskID), zap.String("session_id", sessionID))
					replacement, replacementErr := s.createNewSessionForStep(
						asyncCtx, taskID, session, session.AgentProfileID,
					)
					if replacementErr != nil {
						s.logger.Error("failed to create replacement after terminalized profile switch",
							zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(replacementErr))
						return
					}
					replacementPrompt := s.buildWorkflowPrompt(
						asyncCtx, taskDescription, step, taskID, replacement.ID, isPassthrough,
					)
					if replacementErr = s.autoStartStepPrompt(
						asyncCtx, taskID, replacement, step, replacementPrompt, planMode, true,
					); replacementErr != nil {
						s.logger.Error("failed to auto-start replacement after terminalized profile switch",
							zap.String("task_id", taskID), zap.String("session_id", replacement.ID), zap.Error(replacementErr))
						s.setSessionWaitingForInput(asyncCtx, taskID, replacement.ID, replacement)
						s.publishSessionWaitingEvent(asyncCtx, taskID, replacement.ID, stepID, replacement)
					}
					return
				}
				lock.Unlock()
				release()

				err := s.autoStartStepPrompt(asyncCtx, taskID, fresh, step, effectivePrompt, planMode, true)
				if err != nil {
					if errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
						if workflowAutoStartWasCancelled(err) {
							s.logger.Info("implicit profile switch cancelled during dispatch; not creating replacement",
								zap.String("task_id", taskID), zap.String("session_id", sessionID))
							return
						}
						s.logger.Info("implicit profile switch: reused session terminalized during dispatch, creating replacement",
							zap.String("task_id", taskID), zap.String("session_id", sessionID))
						replacement, replacementErr := s.createNewSessionForStep(
							asyncCtx, taskID, fresh, fresh.AgentProfileID,
						)
						if replacementErr != nil {
							s.logger.Error("failed to create replacement after terminalized dispatch",
								zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(replacementErr))
							return
						}
						replacementPrompt := s.buildWorkflowPrompt(
							asyncCtx, taskDescription, step, taskID, replacement.ID, isPassthrough,
						)
						if replacementErr = s.autoStartStepPrompt(
							asyncCtx, taskID, replacement, step, replacementPrompt, planMode, true,
						); replacementErr != nil {
							s.logger.Error("failed to auto-start replacement after terminalized dispatch",
								zap.String("task_id", taskID), zap.String("session_id", replacement.ID), zap.Error(replacementErr))
							s.setSessionWaitingForInput(asyncCtx, taskID, replacement.ID, replacement)
							s.publishSessionWaitingEvent(asyncCtx, taskID, replacement.ID, stepID, replacement)
						}
						return
					}
					s.logger.Error("failed to launch agent after profile switch",
						zap.String("task_id", taskID),
						zap.String("session_id", sessionID),
						zap.Error(err))
					s.setSessionWaitingForInput(asyncCtx, taskID, sessionID, fresh)
					s.publishSessionWaitingEvent(asyncCtx, taskID, sessionID, stepID, fresh)
					s.drainQueuedMessageForPromptableSession(asyncCtx, sessionID)
				}
			}()
			return
		}
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

// dispatchEngineOwnedOnEnterAction executes an engine-owned on_enter action
// once for this step-entry. The marker CAS prevents duplicate execution.
func (s *Service) dispatchEngineOwnedOnEnterAction(
	ctx context.Context, taskID string, step *wfmodels.WorkflowStep, action wfmodels.OnEnterAction, position int, entryID int64,
) (abandon, failed bool, cause string) {
	if entryID == 0 {
		return false, false, ""
	}
	compiled, ok := engine.CompileOnEnterAction(action)
	if !ok {
		return false, false, ""
	}
	callback := s.engineOnEnterCallback(action.Type)
	if callback == nil {
		return false, false, ""
	}
	operationID := fmt.Sprintf("step_entry:%d:%d", entryID, position)
	claimed, err := s.repo.ClaimStepEntryMarker(ctx, entryID, position, string(action.Type), operationID, time.Now())
	if err != nil {
		s.logger.Error("processOnEnter: claim step entry marker failed",
			zap.Error(err), zap.String("task_id", taskID), zap.String("step_id", step.ID),
			zap.Int64("entry_id", entryID), zap.Int("position", position))
		return false, true, err.Error()
	}
	if !claimed {
		priorState, priorCause, found, stateErr := s.repo.GetStepEntryMarkerState(ctx, entryID, position)
		if stateErr != nil || !found {
			return false, false, ""
		}
		switch priorState {
		case stepentry.MarkerFailed:
			return false, true, priorCause
		case stepentry.MarkerInProgress:
			return true, false, ""
		default:
			return false, false, ""
		}
	}

	if action.Type == wfmodels.OnEnterClearDecisions {
		if handled, atomicFailed, atomicCause := s.dispatchClearDecisionsAtomic(ctx, taskID, step, entryID, position); handled {
			return false, atomicFailed, atomicCause
		}
	}

	in := engine.ActionInput{
		Trigger:     engine.TriggerOnEnter,
		State:       engine.MachineState{TaskID: taskID, CurrentStepID: step.ID, WorkflowID: step.WorkflowID},
		Step:        engine.StepSpec{ID: step.ID, WorkflowID: step.WorkflowID, Name: step.Name, Position: step.Position},
		Action:      compiled,
		OperationID: operationID,
	}
	state, execCause := stepentry.MarkerDone, ""
	if _, execErr := callback.Execute(ctx, in); execErr != nil {
		state, execCause = stepentry.MarkerFailed, execErr.Error()
		s.logger.Error("processOnEnter: engine-owned on_enter action failed",
			zap.Error(execErr), zap.String("task_id", taskID), zap.String("step_id", step.ID),
			zap.String("action_type", string(action.Type)))
	}
	if err := s.repo.CompleteStepEntryMarker(ctx, entryID, position, state, execCause, time.Now()); err != nil {
		s.logger.Error("processOnEnter: complete step entry marker failed",
			zap.Error(err), zap.String("task_id", taskID), zap.String("step_id", step.ID),
			zap.Int64("entry_id", entryID), zap.Int("position", position))
	}
	return false, state == stepentry.MarkerFailed, execCause
}

// dispatchClearDecisionsAtomic executes clear_decisions through
// Repository.ClearStepDecisionsAndCompleteMarker (AC-B6) when it is safe to
// do so: only when engineDecisions is confirmed to be
// *workflowadapters.DecisionAdapter, the type SetEngineDecisionStore is
// wired with at the one production construction site
// (backendapp/main.go:1501, NewDecisionAdapter(repos.Workflow) —
// repos.Workflow is always the real workflow repository, sharing s.repo's
// writer *sqlx.DB per backendapp/storage.go). Test doubles
// (fakeDecisionStore, failingDecisionStore in step_entry_dispatch_test.go)
// are not that type — no test constructs a DecisionAdapter — so handled is
// false for them and the caller falls back to the pre-existing, non-atomic
// callback.Execute + CompleteStepEntryMarker sequence, preserving every
// existing AC-C2/AC-D3/AC-D4 test's error-injection behavior unchanged.
//
// When handled is true, failed/cause report the outcome exactly like
// dispatchEngineOwnedOnEnterAction's own return values. On error, the
// transaction rolled back — neither the delete nor the marker update
// committed, so the marker is left in_progress, which is precisely AC-B6's
// invariant (in_progress proves the delete did not commit). No compensating
// write is attempted; returning failed=true is enough to trigger AC-C2's
// abort in dispatchOnEnterActions.
func (s *Service) dispatchClearDecisionsAtomic(
	ctx context.Context, taskID string, step *wfmodels.WorkflowStep, entryID int64, position int,
) (handled, failed bool, cause string) {
	if _, ok := s.engineDecisions.(*workflowadapters.DecisionAdapter); !ok {
		return false, false, ""
	}
	if _, err := s.repo.ClearStepDecisionsAndCompleteMarker(ctx, taskID, step.ID, entryID, position, time.Now()); err != nil {
		s.logger.Error("processOnEnter: atomic clear_decisions failed",
			zap.Error(err), zap.String("task_id", taskID), zap.String("step_id", step.ID),
			zap.Int64("entry_id", entryID), zap.Int("position", position))
		return true, true, err.Error()
	}
	return true, false, ""
}

// engineOnEnterCallback returns the Phase 2 callback for an engine-owned
// on_enter action kind. The returned callback's Execute reports its own
// ErrActionNotYetWired when the required adapter isn't wired (kanban-only
// deployments) — that surfaces as a failed marker with a clear cause rather
// than a silent no-op, matching AC-A6's "never discard" intent.
func (s *Service) engineOnEnterCallback(t wfmodels.OnEnterActionType) engine.ActionCallback {
	switch t {
	case wfmodels.OnEnterClearDecisions:
		return engine.ClearDecisionsCallback{Decisions: s.engineDecisions}
	case wfmodels.OnEnterQueueRunForEachParticipant:
		return engine.QueueRunForEachParticipantCallback{Adapter: s.engineRunQueue, Participants: s.engineParticipants}
	default:
		return nil
	}
}

// applyPendingMove applies a deferred move_task_kandev call now that the agent's
// turn has ended. Synchronous: updates the task's step in the DB, runs on_exit
// for the source step and on_enter for the target step. Bypasses
// task.Service.MoveTask (and the task.moved event) so the orchestrator's async
// task.moved handler doesn't run a second processStepExitAndEnter for the same
// transition. The move hand-off prompt is correlated by MoveID and removed
// only when the move is already complete or rejected; unrelated queued
// messages remain available for the normal drain path.
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
	if move.MoveID == "" {
		move.MoveID = legacyPendingMoveID(sessionID, move)
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
		if err := s.workflowStore.MarkDeferredMoveApplied(ctx, taskID, move.MoveID); errors.Is(err, errDeferredMoveAlreadyApplied) {
			s.logger.Info("dropping already-applied pending move at current target",
				zap.String("task_id", taskID), zap.String("move_id", move.MoveID))
		} else if err != nil {
			s.logger.Error("failed to record pending move at current target",
				zap.String("task_id", taskID), zap.String("move_id", move.MoveID), zap.Error(err))
			reinsertPendingMove()
			return
		}
		// Step already matches. The move is complete, so consume its hand-off
		// prompt before the natural drain path handles unrelated messages.
		s.logger.Info("pending move target equals current step; skipping transition",
			zap.String("task_id", taskID),
			zap.String("step_id", fromStepID))
		s.removePendingMoveHandoffPrompt(ctx, sessionID, taskID, move.MoveID)
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
		s.removePendingMoveHandoffPrompt(ctx, sessionID, taskID, move.MoveID)
		return
	}

	// Mark the session WAITING_FOR_INPUT before processOnEnter runs. The agent
	// just finished its turn; the active-turn guard in processOnEnter would
	// otherwise see RUNNING and skip the on_enter processing.
	s.setSessionWaitingForInput(ctx, taskID, sessionID, session)

	// sessionID is the target task's queue/execution session. It is not
	// necessarily the agent that called move_task_kandev (cross-task hand-offs
	// are supported), so use the sender persisted with the pending move.
	deferredMoveAttribution := steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerMCPDeferredMove,
		ActorKind: steptelemetry.ActorSystem,
	}
	if move.SenderSessionID != "" {
		deferredMoveAttribution.ActorKind = steptelemetry.ActorAgent
		deferredMoveAttribution.ActorID = move.SenderSessionID
		deferredMoveAttribution.SessionID = move.SenderSessionID
	}
	deferredMoveCtx := steptelemetry.WithAttribution(ctx, deferredMoveAttribution)
	if err := s.workflowStore.ApplyDeferredMoveTransition(deferredMoveCtx, taskID, sessionID, fromStepID, move.WorkflowStepID, move.MoveID); errors.Is(err, errDeferredMoveAlreadyApplied) {
		s.logger.Info("dropping already-applied pending move", zap.String("task_id", taskID), zap.String("move_id", move.MoveID))
		s.removePendingMoveHandoffPrompt(ctx, sessionID, taskID, move.MoveID)
		return
	} else if err != nil {
		s.logger.Error("failed to apply pending move transition",
			zap.String("task_id", taskID),
			zap.Error(err))
		// The hand-off prompt was queued by handleMoveTask before the move was
		// applied. Now that the move failed, the on_enter path that would have
		// drained the queue won't run, and handleAgentReady has already returned.
		// Drop the orphan so it can't be misdelivered to the source step's agent
		// on a future turn (it was authored for the move's *target* step).
		s.removePendingMoveHandoffPrompt(ctx, sessionID, taskID, move.MoveID)
		return
	}

	// ADR 0015 — record the audit row now that the transition is durably
	// persisted. This is the agent-initiated move_task_kandev path (the move
	// couldn't apply inline because the calling session was still
	// RUNNING/STARTING); the idle-session path records through
	// task/service.MoveTaskWithOptions instead.
	actor := wfmodels.StepTransitionActorAgent
	if move.Actor != "" {
		actor = wfmodels.StepTransitionActor(move.Actor)
	}
	s.recordManualStepTransition(ctx, sessionID, fromStepID, move.WorkflowStepID, actor)

	s.logger.Info("applying pending move",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", fromStepID),
		zap.String("to_step_id", move.WorkflowStepID))
	if stored, loadErr := s.repo.GetTask(ctx, taskID); loadErr == nil && stored != nil && stored.QueuedForStepID == move.WorkflowStepID && !stored.WIPAdmitted {
		// The destination is visible and queued, but its entry lifecycle is
		// deferred until promotion. The source exit still runs once now.
		go s.processStepExit(context.WithoutCancel(ctx), taskID, session, fromStepID)
		return
	}

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

func legacyPendingMoveID(sessionID string, move *messagequeue.PendingMove) string {
	identity := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s",
		sessionID, move.TaskID, move.WorkflowID, move.WorkflowStepID, move.Position,
		move.QueuedAt.UTC().Format(time.RFC3339Nano), move.Actor, move.SenderSessionID)
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("legacy-%x", sum[:])
}

// removePendingMoveHandoffPrompt reports whether cleanup is complete. A false
// result means queue storage failed and a durable caller must preserve enough
// correlation state to retry.
func (s *Service) removePendingMoveHandoffPrompt(ctx context.Context, sessionID, taskID, moveID string) bool {
	if s.messageQueue == nil {
		return true
	}
	entryID, listed := s.pendingMoveHandoffPromptID(ctx, sessionID, taskID, moveID)
	if !listed {
		return false
	}
	if entryID == "" {
		return true
	}
	_, removed, err := s.messageQueue.TakeQueuedEntry(ctx, sessionID, entryID)
	if err != nil {
		s.logger.Warn("failed to remove pending-move hand-off prompt",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	if removed {
		s.pendingMoveHandoffPromptRemoved(ctx, sessionID, taskID, moveID, entryID)
	}
	return true
}

func (s *Service) pendingMoveHandoffPromptID(ctx context.Context, sessionID, taskID, moveID string) (string, bool) {
	entries, _, err := s.messageQueue.SnapshotSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to list pending-move hand-off prompts",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return "", false
	}
	for _, entry := range entries {
		if entry.TaskID != taskID || entry.QueuedBy != messagequeue.QueuedByMoveTask {
			continue
		}
		entryMoveID, _ := entry.Metadata[messagequeue.MetadataDeferredMoveID].(string)
		matches := entryMoveID == moveID
		if entryMoveID == "" && strings.HasPrefix(moveID, "legacy-") {
			matches = true
		}
		if !matches {
			continue
		}
		return entry.ID, true
	}
	return "", true
}

func (s *Service) pendingMoveHandoffPromptRemoved(
	ctx context.Context,
	sessionID, taskID, moveID, entryID string,
) {
	if entryID == "" {
		return
	}
	s.publishQueueStatusEvent(ctx, sessionID)
	s.logger.Warn("dropped pending-move hand-off prompt",
		zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.String("move_id", moveID))
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
type queueDrainOutcome uint8

const (
	queueDrainSkipped queueDrainOutcome = iota
	queueDrainDispatched
	queueDrainPaused
	queueDrainTaskAdmissionReadFailed
)

func (s *Service) drainQueuedMessageForPromptableSession(ctx context.Context, sessionID string) bool {
	return s.drainQueuedMessageForPromptableSessionOutcome(ctx, sessionID) == queueDrainDispatched
}

func (s *Service) drainQueuedMessageForPromptableSessionOutcome(ctx context.Context, sessionID string) queueDrainOutcome {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(sessionID) {
		s.logger.Debug("skipping drain while cancellation is in progress",
			zap.String("session_id", sessionID))
		return queueDrainSkipped
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload session before drain",
			zap.String("session_id", sessionID), zap.Error(err))
		return queueDrainSkipped
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		s.logger.Debug("skipping drain: session is not promptable once the guard is held",
			zap.String("session_id", sessionID), zap.Error(err))
		return queueDrainSkipped
	}
	return s.drainQueuedMessageForPromptableSessionLockedOutcome(ctx, sessionID)
}

// drainQueuedMessageForPromptableSessionWithTaskAdmission is the enqueue-side
// drain. It rechecks WIP admission after taking the session guard and directly
// before reserving the queue head, so a stale pre-enqueue task snapshot cannot
// bypass a workflow capacity wait.
func (s *Service) drainQueuedMessageForPromptableSessionWithTaskAdmission(
	ctx context.Context,
	taskID, sessionID string,
) queueDrainOutcome {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(sessionID) {
		return queueDrainSkipped
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload session before admission-gated drain",
			zap.String("session_id", sessionID), zap.Error(err))
		return queueDrainSkipped
	}
	if session == nil {
		return queueDrainSkipped
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		return queueDrainSkipped
	}
	// QueueUserPrompt can race a live clarification between its enqueue-side
	// checks and this guarded drain. Re-read the clarification ownership while
	// the guard is held so a parent question or human clarification still owns
	// the turn when the queue head is reserved. Detached bundles are allowed by
	// sessionHasLiveClarification and can therefore make progress here.
	if s.sessionHasLiveClarification(ctx, sessionID) {
		return queueDrainSkipped
	}
	return s.drainQueuedMessageForPromptableSessionLockedWithTaskAdmission(ctx, taskID, sessionID)
}

// drainQueuedMessageForPromptableSessionLocked takes the next queued
// message and dispatches it for execution. Callers must ensure the session
// is ready for input first, AND must already hold sessionID's
// cancelInFlight lock — this neither acquires nor releases it. Use the
// public drainQueuedMessageForPromptableSession instead when the guard is
// not already held.
//
// Backs off without taking anything when any queued dispatch is still settling
// for this session. This covers a different dispatch already handed off,
// or an admitted-but-not-yet-dispatched steer. Send Now uses the phase-specific
// helpers to supersede only a pending automatic FIFO reservation.
func (s *Service) drainQueuedMessageForPromptableSessionLocked(ctx context.Context, sessionID string) bool {
	return s.drainQueuedMessageForPromptableSessionLockedOutcome(ctx, sessionID) == queueDrainDispatched
}

func (s *Service) drainQueuedMessageForPromptableSessionLockedOutcome(ctx context.Context, sessionID string) queueDrainOutcome {
	return s.drainQueuedMessageForPromptableSessionLockedWithTaskAdmission(ctx, "", sessionID)
}

func (s *Service) drainQueuedMessageForPromptableSessionLockedWithTaskAdmission(
	ctx context.Context,
	taskID, sessionID string,
) queueDrainOutcome {
	if s.messageQueue == nil || s.isCancelInFlight(sessionID) ||
		s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return queueDrainSkipped
	}
	if taskID != "" {
		task, err := s.repo.GetTask(ctx, taskID)
		if err != nil {
			s.logger.Warn("failed to reload task before admission-gated queue reservation",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
			return queueDrainTaskAdmissionReadFailed
		}
		if task == nil || (!task.WIPAdmitted && task.QueuedForStepID != "") {
			return queueDrainSkipped
		}
	}
	queuedMsg, ok, autoRun := s.messageQueue.ReserveQueuedWithAutoRun(ctx, sessionID)
	if !autoRun {
		return queueDrainPaused
	}
	if s.dispatchTakenQueuedMessage(ctx, sessionID, queuedMsg, ok) {
		return queueDrainDispatched
	}
	return queueDrainSkipped
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
	// Reserve entryID before handing off to the async goroutine. The worker
	// transitions this reservation to accepted under the same guard used by
	// Send Now before it performs any visible prompt side effects.
	reservation := s.markQueuedDispatchInFlightWithSourceLocked(sessionID, queuedMsg.ID, queuedMsg)
	go s.executeQueuedMessageWithReservation(sessionID, queuedMsg, reservation)
	return true
}

type passthroughRunningPreparer interface {
	PreparePassthroughRunning(sessionID string) (func(), error)
}

// deliverPassthroughPrompt writes a prompt to PTY stdin and marks the session as running.
// Uses the per-agent PlanPassthroughStdinChunks so Claude's inter-chunk SubmitDelay is
// honored here too (queued / workflow-auto-start path); other agents stay on the single
// atomic write. Falls back to the simple "\r" append if config resolution fails so a
// transient lookup error never silently swallows the prompt.
//
// Callers that already hold the per-session cancellation guard must use
// PreparePassthroughRunning + writePassthroughPrompt instead (see handleAgentReady); this
// function publishes agent.running synchronously and will re-enter the guard via the event
// subscriber. Remaining callers: autoStartPassthroughPrompt (workflow auto-start) and the
// legacy non-preparer fallback in handleAgentReady.
func (s *Service) deliverPassthroughPrompt(ctx context.Context, sessionID, content string) error {
	// Mark RUNNING before any writes so concurrent PromptTask / queued-message
	// delivery is blocked by checkSessionPromptable during the inter-chunk
	// SubmitDelay window (150ms for Claude). Mark error is non-fatal.
	if err := s.agentManager.MarkPassthroughRunning(sessionID); err != nil {
		s.logger.Warn("failed to mark passthrough as running before prompt",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	return s.writePassthroughPrompt(ctx, sessionID, content)
}

// writePassthroughPrompt writes a prompt to PTY stdin without changing runtime
// state. The ready-event path uses this after preparing the running state so it
// can publish the captured event after releasing the session guard.
func (s *Service) writePassthroughPrompt(ctx context.Context, sessionID, content string) error {
	pt, cfgErr := s.agentManager.ResolvePassthroughConfig(ctx, sessionID)
	if cfgErr != nil {
		s.logger.Warn("failed to resolve passthrough config, falling back to \\r submit",
			zap.String("session_id", sessionID),
			zap.Error(cfgErr))
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

func (s *Service) resolveAutoStartPromptContext(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (bool, *models.Task, bool, error) {
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return false, nil, false, fmt.Errorf("resolve MCP mode for workflow auto-start: %w", err)
	}
	if isOfficeTask {
		return true, nil, false, nil
	}

	taskForPrompt, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, nil, false, fmt.Errorf("load task for autopilot prompt: %w", err)
	}
	if session.State != models.TaskSessionStateCreated {
		return false, taskForPrompt, false, nil
	}

	configMode, _ := session.Metadata["config_mode"].(bool)
	if configMode {
		return false, taskForPrompt, false, nil
	}
	titleOwner, err := s.ClaimTaskTitleSession(ctx, taskID, session.ID)
	if err != nil {
		return false, nil, false, fmt.Errorf("claim task title for workflow auto-start: %w", err)
	}
	return false, taskForPrompt, titleOwner, nil
}

// handleCreatedAutoStartLaunchFailure preserves a workflow prompt when the
// created-session launch loses a concurrent-start race. The prompt was already
// recorded in chat history, so queueing it is the only way to deliver it after
// the session becomes ready. If queueing is not applicable or fails, restore
// the hand-off message that was taken before the prompt was merged.
func (s *Service) handleCreatedAutoStartLaunchFailure(
	ctx context.Context,
	taskID, sessionID, stepName, prompt string,
	launchErr error,
	planMode, shouldQueueIfBusy, userMessageRecorded bool,
	attachments []v1.MessageAttachment,
	origin workflowMessageOrigin,
	references []v1.EntityReference,
	takenMsg *messagequeue.QueuedMessage,
) {
	promptQueued := false
	if shouldQueueIfBusy && (isAgentAlreadyRunningError(launchErr) ||
		isSessionBusyError(launchErr) || isTransientPromptError(launchErr)) {
		if queueErr := s.queueAutoStartPrompt(
			ctx, taskID, sessionID, prompt, planMode,
			attachments, origin, userMessageRecorded, references,
		); queueErr != nil {
			s.logger.Warn("failed to queue auto-start prompt after launch failure",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", stepName),
				zap.Error(queueErr))
		} else {
			promptQueued = true
		}
	}
	if !promptQueued && takenMsg != nil {
		s.requeueMessage(ctx, takenMsg, takenMsg.QueuedBy)
	}
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
	// what the agent receives. The mode-aware injectors canonicalize any
	// pre-wrapped runtime context from current server state.
	// Passthrough sessions skip the wrap: the prompt is typed straight into
	// the agent CLI's TTY and the user sees it verbatim.
	recordedPrompt := agentPrompt
	titleOwner := false
	isOfficeTask := false
	var taskForPrompt *models.Task
	needsRuntimeContext := session.State == models.TaskSessionStateCreated ||
		(step != nil && step.HasOnEnterAction(wfmodels.OnEnterResetAgentContext))
	if needsRuntimeContext {
		var contextErr error
		isOfficeTask, taskForPrompt, titleOwner, contextErr = s.resolveAutoStartPromptContext(ctx, taskID, session)
		if contextErr != nil {
			requeueTaken()
			return contextErr
		}
	}
	var pullRequestTargetContext string
	if needsRuntimeContext {
		recordedPrompt, pullRequestTargetContext = s.addTaskPullRequestTargetContext(
			ctx, taskID, recordedPrompt, session.IsPassthrough,
		)
	}
	if (session.State == models.TaskSessionStateCreated || step.HasOnEnterAction(wfmodels.OnEnterResetAgentContext)) && !session.IsPassthrough && (agentPrompt != "" || len(attachments) > 0) {
		configMode, _ := session.Metadata["config_mode"].(bool)
		requiresSignal := step != nil && step.AutoAdvanceRequiresSignal
		referenceContext := EntityReferenceContext(references)
		if isOfficeTask {
			recordedPrompt = sysprompt.InjectOfficeContextWithOptions(
				taskID, sessionID, recordedPrompt, requiresSignal,
				referenceContext, pullRequestTargetContext,
			)
		} else {
			recordedPrompt = sysprompt.InjectKandevContextWithOptions(taskID, sessionID, recordedPrompt, sysprompt.KandevContextOptions{
				RequiresCompletionSignal:       requiresSignal,
				IncludeCoordinatorTaskControls: !configMode,
				IncludeTaskTitleTool:           !configMode && titleOwner,
				Autopilot:                      taskForPrompt != nil && taskForPrompt.Autopilot,
				IncludeUserQuestionTool:        taskForPrompt == nil || !taskForPrompt.Autopilot,
				IncludeParentQuestionTool:      taskForPrompt != nil && taskForPrompt.Autopilot && taskForPrompt.ParentID != "",
			}, referenceContext, pullRequestTargetContext)
		}
	}
	userMsgRecorded := s.recordAutoStartMessage(ctx, taskID, sessionID, recordedPrompt, planMode, origin, references)

	// If the session is in CREATED state, the agent was never started (e.g. workspace-only
	// preparation from a blocked auto-start). PromptTask will reject CREATED sessions,
	// so use StartCreatedSession which properly launches the agent on the prepared workspace.
	// Pass skipMessageRecord=true since recordAutoStartMessage above already recorded it.
	// Guard against concurrent terminalization before dispatching to a CREATED
	// session. By the time a nonterminal session was selected, promoted, and the
	// prompt built, a concurrent completion/cancellation may have terminalized
	// it. Reload under the cancel-in-flight guard and bail out with the
	// standard terminalization error so the caller can create a replacement.
	// The guard is held through the entire StartCreatedSession call to prevent
	// terminalization between the reload and the admission of the launch.
	if session.State == models.TaskSessionStateCreated {
		lock, release := s.acquireCancelInFlightGuard(sessionID)
		lock.Lock()
		fresh, reloadErr := s.repo.GetTaskSession(ctx, sessionID)
		if reloadErr != nil {
			lock.Unlock()
			release()
			requeueTaken()
			return reloadErr
		}
		if fresh == nil || isTerminalSessionState(fresh.State) {
			lock.Unlock()
			release()
			requeueTaken()
			return newWorkflowAutoStartSessionTerminalizedError(fresh)
		}
		var releaseOnce sync.Once
		heldRelease := func() {
			releaseOnce.Do(func() {
				lock.Unlock()
				release()
			})
		}

		s.logger.Info("auto-start: session is CREATED, launching agent via StartCreatedSession",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName))
		_, err := s.StartCreatedSession(
			ctx, taskID, sessionID, session.AgentProfileID,
			recordedPrompt, true, planMode, true, attachments, references,
		)
		// Release the guard as soon as StartCreatedSession has admitted the
		// launch (succeeded or failed definitively). The deferred release
		// via heldRelease ensures the guard is always released even on panic.
		defer heldRelease()
		if err != nil {
			s.handleCreatedAutoStartLaunchFailure(
				ctx, taskID, sessionID, stepName, prompt, err,
				planMode, shouldQueueIfBusy, userMsgRecorded,
				attachments, origin, references, takenMsg,
			)
		}
		return err
	}

	const maxRetryAttempts = 5
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		_, err := s.promptTask(ctx, taskID, sessionID, recordedPrompt, "", planMode, attachments, false, promptTaskOptions{
			requireNonterminalSession: true,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
			requeueTaken()
			return err
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
			return s.fallbackFreshLaunchOnMissingExecution(
				ctx, taskID, sessionID, recordedPrompt, planMode, takenMsg, attachments, references,
			)
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
	references []v1.EntityReference,
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
	if s.isCancelInFlight(sessionID) {
		cancelLock.Unlock()
		releaseCancelLock()
		requeue()
		return &executor.SessionStateSupersededError{SessionID: sessionID, State: models.TaskSessionStateCancelled}
	}
	fresh, err := s.resetSessionForFreshFallback(ctx, sessionID)
	cancelLock.Unlock()
	releaseCancelLock()
	if err != nil {
		requeue()
		return err
	}

	if _, err := s.StartCreatedSession(
		ctx, taskID, sessionID, fresh.AgentProfileID,
		prompt, true, planMode, true, attachments, references,
	); err != nil {
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

// takeAndMergeHandoffMessage drains an Auto-run-enabled hand-off message for
// the session (set by handleMoveTask via move_task_kandev or by
// drainQueuedMessageForPromptableSession) and merges its content + attachments
// into the auto-start prompt. Paused queues keep the hand-off parked. Returns
// the original queued message (so terminal failure paths can re-queue it via
// requeueMessage), merged prompt, converted attachments, and structurally
// normalized references.
func (s *Service) takeAndMergeHandoffMessage(ctx context.Context, sessionID, basePrompt string) (*messagequeue.QueuedMessage, string, []v1.MessageAttachment, []v1.EntityReference) {
	if s.messageQueue == nil {
		return nil, basePrompt, nil, nil
	}
	msg, ok := s.messageQueue.TakeQueuedIfAutoRun(ctx, sessionID)
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
				AttachmentID: a.AttachmentID,
				Data:         a.Data,
				MimeType:     a.MimeType,
				Name:         a.Name,
				SizeBytes:    a.SizeBytes,
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
			AttachmentID: attachment.AttachmentID,
			Data:         attachment.Data,
			MimeType:     attachment.MimeType,
			Name:         attachment.Name,
			SizeBytes:    attachment.SizeBytes,
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

	// A CREATED session has never been prompted, so there is no agent
	// conversation to clear. Its execution may still be workspace-only
	// (prepared, never started), in which case a "restart" here would *start*
	// the subprocess — and auto_start_agent, which reads State==CREATED as
	// "the agent was never started", would then start it a second time. The
	// second start is rejected by agentctl's running-process guard, failing
	// the task and dropping the step prompt. Leave the start to
	// autoStartStepPrompt: the process it launches begins on a fresh ACP
	// session regardless, so nothing is lost by skipping.
	if session.State == models.TaskSessionStateCreated {
		s.logger.Debug("session has no agent context to reset, skipping",
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName))
		return true
	}

	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()
	resetGuard := s.lockCancelInFlightGuard(sessionID)
	s.setSessionResetInProgress(sessionID, true)
	defer func() {
		// Publish the end of reset while the same guard is still held. A prompt
		// that acquires the guard after this point observes a settled marker.
		s.setSessionResetInProgress(sessionID, false)
		resetGuard.release()
	}()

	if err := s.quiesceActiveResetTurn(ctx, taskID, sessionID, stepName, resetGuard); err != nil {
		s.logger.Error("failed to quiesce active turn before context reset",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName),
			zap.Error(err))
		return false
	}

	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil || executionID == "" {
		// No in-memory execution exists yet — most commonly a lazily-resumed
		// session whose process has not been relaunched since the last run.
		// The resume path (applyRunningRecordToResumeRequest) reads the ACP
		// resume token straight from the executors_running row, bypassing any
		// in-memory execution lookup entirely, so leaving that token in place
		// here would let the next lazy launch reconnect to the pre-reset
		// conversation and silently skip the reset. Clear the same persisted
		// state the live-execution path clears below so the reset survives
		// until the agent's first turn regardless of when the process starts.
		s.logger.Debug("no live agent execution for context reset, clearing persisted resume state",
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName))
		if err := s.clearResumeToken(ctx, sessionID); err != nil {
			s.logger.Error("failed to clear lazy resume token before context reset",
				zap.String("task_id", taskID),
				zap.String("session_id", sessionID),
				zap.String("step_name", stepName),
				zap.Error(err))
			return false
		}
		s.clearPersistedResetState(ctx, sessionID, session)
		return true
	}

	s.logger.Info("resetting agent context for workflow step",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("step_name", stepName),
		zap.String("agent_execution_id", executionID))

	previousACPSessionID := s.currentACPSessionID(sessionID)
	if err := s.agentManager.ResetAgentContext(ctx, executionID); err != nil {
		s.logger.Error("failed to reset agent context",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName),
			zap.Error(err))
		s.reconcileFailedContextReset(ctx, taskID, session, executionID, previousACPSessionID)
		return false
	}

	// Clear the old resume token only after the provider reset succeeds. This
	// keeps a valid recovery token when the runtime reset fails. A fresh ACP
	// session event can race this clear, so persist the lifecycle manager's
	// current session ID again below after the clear.
	if err := s.clearResumeToken(ctx, sessionID); err != nil {
		s.logger.Error("failed to clear resume token after context reset",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.String("step_name", stepName),
			zap.Error(err))
		return false
	}
	if acpSessionID := s.currentACPSessionID(sessionID); acpSessionID != "" {
		s.storeResumeToken(ctx, taskID, sessionID, executionID, acpSessionID, "")
	}

	// Clear the remaining persisted state (ACP session metadata, context window)
	// after the provider reset succeeds. The token is handled explicitly above.
	s.clearPersistedResetState(ctx, sessionID, session)
	return true
}

// quiesceActiveResetTurn stops an in-flight turn through the internal silent
// cancellation coordinator before the provider conversation is replaced. It
// deliberately does not call Service.CancelAgent: that path evaluates the
// user's configured cancellation completion and creates a visible message.
func (s *Service) quiesceActiveResetTurn(
	ctx context.Context,
	taskID, sessionID, stepName string,
	resetGuard *lockedCancelInFlightGuard,
) error {
	turnID, err := s.activeResetTurnID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("inspect active turn for context reset: %w", err)
	}
	if turnID == "" {
		if s.currentCancellation(sessionID) != nil {
			return errContextResetCancellationConflict
		}
		return nil
	}
	if s.turnService == nil {
		// The in-memory cache is the only available identity when no durable
		// turn service is wired. The lifecycle capture then applies its own
		// best-effort cancellation behavior without an expected durable ID.
		turnID = ""
	}
	if _, err := s.cancelAgentSilentWithGuardActionKindExclusiveConflict(
		ctx, taskID, sessionID, resetGuard.unlock, resetGuard.relock,
		nil, cancellationKindInternal, turnID, errContextResetCancellationConflict,
	); err != nil {
		return fmt.Errorf("cancel active turn for context reset at %s: %w", stepName, err)
	}
	return nil
}

// hasActiveResetTurn combines the in-memory admission records with the
// durable turn service. Either record is enough to fail closed before a
// provider context replacement; missing an active turn could let the old
// provider stream race the new conversation.
func (s *Service) hasActiveResetTurn(ctx context.Context, sessionID string) (bool, error) {
	turnID, err := s.activeResetTurnID(ctx, sessionID)
	return turnID != "", err
}

func (s *Service) activeResetTurnID(ctx context.Context, sessionID string) (string, error) {
	if s.turnService != nil {
		turnID, err := s.peekActiveTurnID(ctx, sessionID)
		if err != nil {
			return "", err
		}
		if turnID != "" {
			return turnID, nil
		}
	}
	if value, ok := s.activeTurns.Load(sessionID); ok {
		turnID, _ := value.(string)
		return turnID, nil
	}
	return s.reservedPromptTurnID(sessionID), nil
}

// reconcileFailedContextReset handles a reset that moved the live ACP session
// before a later reset step failed. The old cursor and context metadata belong
// to the previous ACP session and must not survive that partial transition.
func (s *Service) reconcileFailedContextReset(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	executionID string,
	previousACPSessionID string,
) {
	liveACPSessionID := s.currentACPSessionID(session.ID)
	if liveACPSessionID == "" || liveACPSessionID == previousACPSessionID {
		return
	}

	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		s.logger.Warn("failed to inspect execution after partial context reset",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
		return
	}
	if running.AgentExecutionID != executionID {
		return
	}

	s.storeResumeToken(ctx, taskID, session.ID, executionID, liveACPSessionID, "")
	s.clearPersistedResetState(ctx, session.ID, session)
}

// clearPersistedResetState clears the durable, DB-backed session state that a
// later lazy resume would otherwise pick back up: the stored ACP session ID
// in session metadata and the persisted context window. The resume token is
// cleared explicitly by resetAgentContext so a reset failure can retain it.
func (s *Service) clearPersistedResetState(ctx context.Context, sessionID string, session *models.TaskSession) {
	// Clear the stored ACP session ID using json_set to avoid clobbering other keys.
	if updateErr := s.repo.SetSessionMetadataKey(ctx, sessionID, "acp_session_id", ""); updateErr != nil {
		s.logger.Warn("failed to clear ACP session ID from session metadata",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	if updateErr := s.clearContextWindowForReset(ctx, sessionID); updateErr != nil {
		s.logger.Warn("failed to clear context window from session metadata",
			zap.String("session_id", sessionID),
			zap.Error(updateErr))
	}
	// Keep the in-memory event snapshot aligned with the new provider
	// conversation even when persistence fails; the initiating client clears
	// its cache after the provider reset succeeds and must not receive stale data
	// back through the final processOnEnter state event.
	clearInMemoryContextWindow(session)
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
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}

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
// assembleMachineState accepts a nil session for the AC-62/F38 case of a
// task with zero task_sessions rows (sessionID == ""): CurrentStepID is
// always derived from the task row, never the session, so SessionID and
// SessionState are left at their zero values and no workflow_data is
// available in that case.
func assembleMachineState(task *models.Task, session *models.TaskSession, isPassthrough bool) engine.MachineState {
	currentStepID := task.WorkflowStepID
	state := engine.MachineState{
		TaskID:          task.ID,
		WorkflowID:      task.WorkflowID,
		CurrentStepID:   currentStepID,
		TaskDescription: task.Description,
		IsPassthrough:   isPassthrough,
	}
	if session == nil {
		return state
	}
	state.SessionID = session.ID
	state.SessionState = string(session.State)
	if session.Metadata != nil {
		if wd, ok := session.Metadata["workflow_data"].(map[string]any); ok {
			state.Data = wd
		}
	}
	return state
}

// processOnTurnCompleteViaEngine uses the workflow engine to evaluate on_turn_complete
// actions and drive step transitions. Falls back to the legacy method when the engine
// is not initialized. Returns true if a step transition occurred.
func (s *Service) processOnTurnCompleteViaEngine(ctx context.Context, taskID string, session *models.TaskSession) bool {
	return s.processOnTurnCompleteViaEngineWithCause(ctx, taskID, session, turnCompletionCauseAgentTurn)
}

func (s *Service) processOnTurnCompleteViaEngineWithCause(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	cause turnCompletionCause,
) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to load task for on_turn_complete",
			zap.String("task_id", taskID), zap.Error(err))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}

	unlock, task, proceed := s.acquireTurnCompletionCriticalSection(ctx, taskID, session, task)
	defer unlock()
	if !proceed {
		return false
	}

	if s.workflowEngine == nil {
		return s.processOnTurnCompleteWithCause(ctx, task, session, cause)
	}

	if !s.prepareEngineTurnCompletion(ctx, taskID, task, session, cause) {
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

	if cause == turnCompletionCauseUserCancellation {
		ctx = cancellationTransitionAttribution(ctx)
	}
	return s.applyEngineTransitionWithMode(ctx, taskID, session, result, engine.TriggerOnTurnComplete, task.Description, transitionLifecycleWithOnEnter)
}

// acquireTurnCompletionCriticalSection serializes on_turn_complete
// processing per session (see turnCompletionLocks' field comment for the
// double-allocation/double-dispatch race this closes) and detects a
// duplicate call that lost the race for the lock. proceed is false when
// this call must stop and treat itself as a redundant duplicate; the
// caller must still invoke unlock regardless of proceed. session == nil
// callers (none exist today, but the type allows it) skip locking
// entirely, matching pre-fix behavior.
//
// Two independent duplicate signals are checked, because neither alone
// covers every case:
//
//  1. Lock contention (acquireTurnCompletionLock's contended return).
//     Several independent event sources (normal agent turn completion,
//     agent-exit, step_complete_kandev's out-of-band signal, user
//     cancellation) can each call this for the same session; if this call
//     had to wait for another one already in flight for the same session,
//     that other call is processing (or has just finished processing) the
//     same physical turn, so this one backs off unconditionally — no DB
//     read is needed or even safe to reason from, since a call that lost
//     the lock race has no reliable way to tell whether the winner's
//     commit is what it would observe. This is the only signal that
//     reliably catches a duplicate whose entire pre-lock read happens
//     *after* the winner already committed: e.g. two concurrent normal
//     agent-turn-completion signals for the same physical turn (one via
//     the ready path, one via agent-exit), where the second caller's very
//     first DB read can already reflect the first caller's full commit,
//     making any comparison of "my early read" vs. "my read under the
//     lock" pass trivially even though the two calls are genuine
//     duplicates. Contention can only be true when two calls for the same
//     session truly overlap in time, so it never misfires against a
//     legitimate call that arrives well after a prior one has finished
//     (cancellation reconciliation, onStepCompletionSignaled's retry,
//     clarification-dismissal) — those find the lock free.
//  2. The WorkflowStepID comparison (observedTask vs. a fresh read taken
//     after acquiring the lock without contention). This catches a
//     duplicate whose pre-lock snapshot predates the winner's commit: the
//     step moved out from under it while it waited for the (uncontended,
//     from this call's perspective) lock.
//  3. The session-generation comparison (turnCompletionConsumedGeneration):
//     closes the one gap the first two leave open. When two calls for the
//     same session are fully sequential — no true overlap, so contention
//     never fires — the second caller's own pre-lock GetTask already
//     reflects the first caller's commit, so signal 2 also passes
//     trivially: current and observedTask agree, just both already at the
//     winner's destination. Nothing in either DB read distinguishes that
//     from a legitimate, later call that happens to arrive once the
//     session already sits at that step. What does distinguish them is
//     the caller-supplied session snapshot's UpdatedAt: a genuinely new
//     call only exists because something touched the session after the
//     winner's transition committed (a new turn starting, cancellation
//     reconciliation, ...), which bumps UpdatedAt past the generation the
//     winner recorded. A duplicate redelivery carries the same (or an
//     older) snapshot it was constructed from before that commit, so its
//     UpdatedAt is not strictly newer, and it is rejected here instead of
//     being evaluated against the step the winner already moved to.
func (s *Service) acquireTurnCompletionCriticalSection(
	ctx context.Context, taskID string, session *models.TaskSession, observedTask *models.Task,
) (unlock func(), fresh *models.Task, proceed bool) {
	if session == nil {
		return func() {}, observedTask, true
	}
	unlock, contended := s.acquireTurnCompletionLock(session.ID)
	if contended {
		return unlock, observedTask, false
	}
	if lastConsumed, ok := s.turnCompletionConsumedGeneration.Load(session.ID); ok {
		if !session.UpdatedAt.After(lastConsumed.(time.Time)) {
			return unlock, observedTask, false
		}
	}
	current, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		// Best-effort re-check: fall back to the already-validated snapshot
		// rather than fail a call that succeeded its primary read.
		s.turnCompletionConsumedGeneration.Store(session.ID, session.UpdatedAt)
		return unlock, observedTask, true
	}
	if current.WorkflowStepID != observedTask.WorkflowStepID {
		return unlock, current, false
	}
	s.turnCompletionConsumedGeneration.Store(session.ID, session.UpdatedAt)
	return unlock, current, true
}

func (s *Service) prepareEngineTurnCompletion(
	ctx context.Context,
	taskID string,
	task *models.Task,
	session *models.TaskSession,
	cause turnCompletionCause,
) bool {
	if s.shouldSkipEngineTurnCompletion(ctx, taskID, task, session, cause) {
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
	if cause == turnCompletionCauseUserCancellation && !currentStep.CancelTriggersTurnComplete {
		s.logger.Debug("user cancellation completion is disabled for step",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", currentStep.ID))
		return false
	}
	return s.allowEngineSignalCompletion(ctx, taskID, session, currentStep, cause)
}

func (s *Service) shouldSkipEngineTurnCompletion(
	ctx context.Context,
	taskID string,
	task *models.Task,
	session *models.TaskSession,
	cause turnCompletionCause,
) bool {
	if session == nil || session.ID == "" || s.workflowStepGetter == nil {
		return true
	}
	if task.WorkflowStepID == "" {
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return true
	}
	// Skip workflow step actions for ephemeral tasks (quick chat) - they have no workflow.
	// Explicit user cancellation also excludes Office and archived tasks: their
	// lifecycle is owned by their scheduler/archive path, not Kanban completion.
	if task.IsEphemeral || (cause == turnCompletionCauseUserCancellation && (task.IsFromOffice || taskArchived(task))) {
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return true
	}
	return false
}

func (s *Service) allowEngineSignalCompletion(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	currentStep *wfmodels.WorkflowStep,
	cause turnCompletionCause,
) bool {
	if cause == turnCompletionCauseUserCancellation || !currentStep.AutoAdvanceRequiresSignal {
		return true
	}
	signal, has := models.LoadPendingStepSignal(session.Metadata)
	if !has || signal.StepID != currentStep.ID {
		s.logger.Debug("on_turn_complete gated on explicit signal (none received yet)",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.String("step_id", currentStep.ID))
		s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		return false
	}
	s.logger.Info("on_turn_complete consuming explicit signal",
		zap.String("task_id", taskID),
		zap.String("session_id", session.ID),
		zap.String("step_id", currentStep.ID),
		zap.String("source", signal.Source))
	// Bag is consumed once the transition executes (in
	// applyEngineTransition's stamp + clear), so don't clear here —
	// otherwise a failed transition would lose the signal.
	return true
}

// transitionLifecycleMode identifies the caller-owned part of a transition.
// Guarded decisions use a session-independent mode because the session passed
// by the engine is the decider's session, not the assignee's destination
// session. Keeping this distinction explicit prevents the on_turn_start
// lifecycle from being reused for quorum transitions by accident.
type transitionLifecycleMode uint8

const (
	transitionLifecycleWithOnEnter transitionLifecycleMode = iota
	transitionLifecycleOnTurnStart
	transitionLifecycleGuardedDecision
)

// applyGuardedTransitionLifecycle is the service-owned lifecycle bridge for
// quorum re-evaluation. The engine still selects the target and owns the CAS,
// but transition history and task-level terminal handling stay in the
// orchestrator path. The decider session is not changed.
//
// Session-shaped on_enter work is deliberately NOT triggered here:
// sessionID is the decider's session (reviewer/approver), not the task's
// assignee, so dispatching on_enter would hand auto_start_agent's session
// continuation that same decider session. Office's reactivity
// (office/dashboard.runReactivityForDecision) is what wakes the assignee.
// Engine-owned on_enter actions (clear_decisions, ensure_participant_seat,
// queue_run_for_each_participant) are unaffected — they run unconditionally
// via the CAS commit's dispatchStepEntry call below.
func (s *Service) applyGuardedTransitionLifecycle(
	ctx context.Context, taskID, sessionID, fromStepID, toStepID string, trigger engine.Trigger,
) (bool, error) {
	if s.workflowStore == nil {
		return false, errors.New("workflow store is not initialized")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("load task for guarded transition lifecycle: %w", err)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("load session for guarded transition lifecycle: %w", err)
	}

	casAttempted := false
	casApplied := false
	lifecycleApplied := s.applyEngineTransitionWithCommitMode(
		ctx,
		taskID,
		session,
		engine.HandleResult{Transitioned: true, FromStepID: fromStepID, ToStepID: toStepID},
		trigger,
		task.Description,
		transitionLifecycleGuardedDecision,
		func(commitCtx context.Context) (bool, error) {
			casAttempted = true
			transitionCtx := commitCtx
			if !steptelemetry.HasTrigger(transitionCtx) {
				transitionCtx = engineTransitionAttribution(transitionCtx, sessionID, trigger)
			}
			committedTask, oldWorkflowID, applied, commitErr := s.workflowStore.applyTransitionIfAtStepRaw(
				transitionCtx, taskID, fromStepID, toStepID,
			)
			if commitErr != nil {
				return false, commitErr
			}
			if !applied {
				return false, nil
			}
			casApplied = true
			// The raw CAS commit intentionally has no side effects. The shared
			// lifecycle helper performs the remaining task-level work below.
			s.publishTaskUpdated(ctx, committedTask, oldWorkflowID)
			s.workflowStore.pullNextTaskOnVacate(ctx, fromStepID, taskID)
			return true, nil
		},
	)
	if lifecycleApplied || casApplied {
		return true, nil
	}
	if casAttempted {
		// A false CAS is an expected concurrent re-evaluation outcome.
		return false, nil
	}
	return false, errors.New("guarded transition lifecycle did not apply")
}

// applyEngineTransition is the legacy wrapper for session-originated
// transitions. Guarded decisions must use applyEngineTransitionWithMode so
// they cannot be mistaken for an on_turn_start transition.
func (s *Service) applyEngineTransition(
	ctx context.Context, taskID string, session *models.TaskSession,
	result engine.HandleResult, trigger engine.Trigger, taskDescription string,
	triggerOnEnter bool,
) bool {
	mode := transitionLifecycleOnTurnStart
	if triggerOnEnter {
		mode = transitionLifecycleWithOnEnter
	}
	return s.applyEngineTransitionWithMode(ctx, taskID, session, result, trigger, taskDescription, mode)
}

// applyEngineTransitionWithCommit preserves the legacy helper contract for
// session-originated transitions. Guarded decisions use the explicit mode
// variant so they cannot be mistaken for on_turn_start.
func (s *Service) applyEngineTransitionWithCommit(
	ctx context.Context, taskID string, session *models.TaskSession,
	result engine.HandleResult, trigger engine.Trigger, taskDescription string,
	triggerOnEnter bool, commit func(context.Context) (bool, error),
) bool {
	mode := transitionLifecycleOnTurnStart
	if triggerOnEnter {
		mode = transitionLifecycleWithOnEnter
	}
	return s.applyEngineTransitionWithCommitMode(ctx, taskID, session, result, trigger, taskDescription, mode, commit)
}

// applyEngineTransitionWithMode applies an engine-evaluated transition with
// an explicit lifecycle mode: on_exit, DB transition, data patches, and
// optional on_enter processing. Returns true if the transition was applied.
func (s *Service) applyEngineTransitionWithMode(
	ctx context.Context, taskID string, session *models.TaskSession,
	result engine.HandleResult, trigger engine.Trigger, taskDescription string,
	mode transitionLifecycleMode,
) bool {
	return s.applyEngineTransitionWithCommitMode(ctx, taskID, session, result, trigger, taskDescription, mode,
		func(commitCtx context.Context) (bool, error) {
			if err := s.workflowStore.ApplyTransition(commitCtx, taskID, session.ID, result.FromStepID, result.ToStepID, trigger); err != nil {
				return false, err
			}
			return true, nil
		})
}

// applyEngineTransitionWithCommitMode applies the shared lifecycle around a
// transition commit. The default path commits through ApplyTransition. The
// quorum decision path supplies a CAS commit so lifecycle hooks cannot be
// bypassed by the engine's guarded-transition re-evaluation.
//
//nolint:cyclop,gocognit,funlen // this coordinates independent transition lifecycle stages.
func (s *Service) applyEngineTransitionWithCommitMode(
	ctx context.Context, taskID string, session *models.TaskSession,
	result engine.HandleResult, trigger engine.Trigger, taskDescription string,
	mode transitionLifecycleMode, commit func(context.Context) (bool, error),
) bool {
	ctx = withWorkflowMetaCache(ctx)
	sessionLifecycle := mode != transitionLifecycleGuardedDecision
	// Validate the target step exists BEFORE persisting the transition.
	// This prevents the task from being moved to an invalid step_id
	// (e.g., a template-level alias like "review" that doesn't resolve to a real UUID).
	targetStep, err := s.workflowStepGetter.GetStep(ctx, result.ToStepID)
	if err != nil {
		s.logger.Warn("target step not found, skipping transition",
			zap.String("step_id", result.ToStepID),
			zap.Error(err))
		if sessionLifecycle {
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		}
		return false
	}
	if sessionLifecycle {
		if err := s.preflightWorkflowStepCredentials(ctx, taskID, session, targetStep); err != nil {
			s.logger.Warn("target profile credential preflight failed, skipping transition",
				zap.String("task_id", taskID),
				zap.String("step_id", result.ToStepID),
				zap.Error(err))
			return false
		}
	}

	terminalTarget := s.workflowStepIsTerminal(ctx, targetStep.ID)

	fromStep, err := s.loadWorkflowStepForLifecycle(ctx, result.FromStepID, "transition source")
	if err != nil {
		s.logger.Warn("failed to load from-step for on_exit",
			zap.String("step_id", result.FromStepID),
			zap.Error(err))
		if sessionLifecycle {
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		}
		return false
	}
	if sessionLifecycle {
		s.processOnExit(ctx, taskID, session, fromStep)
	}

	// A ResultHolder is only attached when this transition will actually
	// trigger on_enter (triggerOnEnter) — an on_turn_start transition never
	// reaches launchProcessOnEnter below, and a guarded decision is dispatched
	// through the repository's session-independent step-entry path.
	var stepEntry *stepentry.AllocationResult
	applyCtx := ctx
	if mode == transitionLifecycleWithOnEnter {
		stepEntry = &stepentry.AllocationResult{}
		applyCtx = stepentry.WithResultHolder(applyCtx, stepEntry)
	}
	applied, err := commit(applyCtx)
	if err != nil {
		s.logger.Error("failed to apply engine transition",
			zap.String("task_id", taskID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		if sessionLifecycle {
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		}
		return false
	}
	if !applied {
		return false
	}

	// ADR 0015 — record the audit row before the pending signal (if any) is
	// cleared. Only an on_turn_complete transition can have consumed a signal;
	// guarded decision transitions do not mutate the decider's signal bag.
	var consumedSignal *models.PendingStepCompletionSignal
	if sessionLifecycle && trigger == engine.TriggerOnTurnComplete {
		if signal, has := models.LoadPendingStepSignal(session.Metadata); has && signal.StepID == result.FromStepID {
			consumedSignal = &signal
		}
	}
	historyTrigger := wfmodels.StepTransitionTriggerAutoComplete
	switch trigger {
	case engine.TriggerOnTurnStart:
		historyTrigger = wfmodels.StepTransitionTriggerTurnStart
	case engine.TriggerOnChildrenCompleted:
		historyTrigger = wfmodels.StepTransitionTriggerChildrenCompleted
	}
	s.recordAutoStepTransition(ctx, session.ID, result.FromStepID, result.ToStepID, consumedSignal, historyTrigger)

	// ADR 0015 — a successful on_turn_complete transition consumes any
	// pending step-completion signal for the source step. The bag must be
	// cleared so the next step's gating starts from a clean slate. A guarded
	// decision must leave the decider session untouched.
	if sessionLifecycle && trigger == engine.TriggerOnTurnComplete {
		s.clearPendingStepSignal(ctx, session)
	}

	if len(result.DataPatch) > 0 && sessionLifecycle {
		if err := s.workflowStore.PersistData(ctx, session.ID, result.DataPatch); err != nil {
			s.logger.Warn("failed to persist workflow data patch",
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}

	queuedTask, queuedErr := s.repo.GetTask(ctx, taskID)
	if queuedErr == nil && queuedTask != nil && queuedTask.QueuedForStepID == result.ToStepID && !queuedTask.WIPAdmitted {
		// The source transition is committed, but destination state and
		// on_enter behavior wait for the queue promotion event.
		if sessionLifecycle {
			s.setSessionWaitingForInput(ctx, taskID, session.ID, session)
		}
		return true
	}

	if terminalTarget {
		s.markTaskCompletedForTerminalStep(ctx, taskID, targetStep.ID)
	}

	if mode == transitionLifecycleGuardedDecision {
		return true
	}
	if mode == transitionLifecycleOnTurnStart {
		// on_turn_start transitions: user is about to send a message, no
		// on_enter needed. We still need to switch the agent profile if the
		// target step requires a different one — the next prompt should go
		// to the correct agent.
		effectiveSession, ok := s.maybySwitchSessionForProfile(ctx, taskID, session, targetStep, fromStep)
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
	var stepEntryID int64
	if stepEntry != nil {
		stepEntryID = stepEntry.EntryID
	}
	// The caller may still hold the source session guard, but this work runs
	// asynchronously after that guard is released. Clear the synchronous
	// ownership marker so profile-switch parking acquires its own guard.
	s.launchProcessOnEnter(withoutWorkflowProfileSwitchGuard(context.WithoutCancel(ctx)), taskID, session, targetStep, taskDescription, stepEntryID, fromStep)
	return true
}

func (s *Service) launchProcessOnEnter(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	targetStep *wfmodels.WorkflowStep,
	taskDescription string,
	entryID int64,
	sourceStep *wfmodels.WorkflowStep,
) {
	go func() {
		defer func() {
			if s.onProcessOnEnterComplete != nil {
				s.onProcessOnEnterComplete()
			}
		}()
		s.processOnEnter(ctx, taskID, session, targetStep, taskDescription, entryID, sourceStep)
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
	return s.applyEngineTransitionWithMode(ctx, taskID, session, result, engine.TriggerOnTurnStart, "", transitionLifecycleOnTurnStart)
}

package orchestrator

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/review"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ReviewRunner launches native code-review passes. Satisfied by
// *review.Runner; declared here so the orchestrator does not depend on the
// concrete type and tests can substitute a fake.
type ReviewRunner interface {
	Launch(ctx context.Context, req review.RunRequest) (*taskmodels.TaskReviewRun, error)
}

// buildWorkflowCallbacks creates the callback registry for the workflow engine.
// Each callback wraps an existing orchestrator Service method, keeping side-effect
// logic in the orchestrator while letting the engine drive evaluation.
//
// Phase 2 (ADR-0004) callbacks — queue_run, clear_decisions,
// queue_run_for_each_participant — are registered conditionally based on
// the orchestrator's wired adapters. If adapters are missing, triggers
// without an operation id retain the legacy no-op behavior. Operation-bearing
// triggers fail before execution so late wiring can retry them safely.
func buildWorkflowCallbacks(svc *Service) engine.MapRegistry {
	r := engine.MapRegistry{
		engine.ActionEnablePlanMode:    &enablePlanModeCallback{svc: svc},
		engine.ActionDisablePlanMode:   &disablePlanModeCallback{svc: svc},
		engine.ActionResetAgentContext: &resetAgentContextCallback{svc: svc},
		engine.ActionAutoStartAgent:    &autoStartAgentCallback{svc: svc},
		engine.ActionSetWorkflowData:   &setWorkflowDataCallback{},
		engine.ActionSetSessionMode:    &setSessionModeCallback{svc: svc},
	}
	if svc.engineRunQueue != nil {
		r[engine.ActionQueueRun] = engine.QueueRunCallback{
			Adapter:      svc.engineRunQueue,
			Participants: svc.engineParticipants,
			CEOResolver:  svc.engineCEOResolver,
			Primary:      svc.enginePrimary,
			TaskSteps:    workflowTargetStepResolver(svc.enginePrimary, svc.engineParticipants),
			Logger:       svc.logger,
		}
		if svc.engineParticipants != nil {
			r[engine.ActionQueueRunForEachParticipant] = engine.QueueRunForEachParticipantCallback{
				Adapter:      svc.engineRunQueue,
				Participants: svc.engineParticipants,
				Decisions:    svc.engineDecisions,
				Logger:       svc.logger,
			}
		}
	}
	if svc.engineDecisions != nil {
		r[engine.ActionClearDecisions] = engine.ClearDecisionsCallback{Decisions: svc.engineDecisions}
	}
	if svc.reviewRunner != nil {
		r[engine.ActionRunCodeReview] = &runCodeReviewCallback{svc: svc}
	}
	if svc.engineTaskCreator != nil {
		r[engine.ActionCreateChildTask] = engine.CreateChildTaskCallback{Creator: svc.engineTaskCreator}
	}
	if svc.engineWorkflowSwitcher != nil {
		r[engine.ActionSwitchWorkflow] = engine.SwitchWorkflowCallback{
			Switcher: svc.engineWorkflowSwitcher,
			Dispatch: switchWorkflowDispatcher(svc),
		}
	}
	if svc.engineParticipantSeatWriter != nil {
		r[engine.ActionEnsureParticipantSeat] = engine.EnsureParticipantSeatCallback{
			Writer: svc.engineParticipantSeatWriter,
			Caster: svc.engineParticipantSeatCaster,
			Logger: svc.logger,
		}
	}
	return r
}

// workflowTargetStepResolver picks the primary adapter first because it is the
// queue_run primary resolver, then falls back to participants. Nil means
// cross-task queue_run targets will surface ErrActionNotYetWired.
func workflowTargetStepResolver(
	primary engine.PrimaryAgentResolver,
	participants engine.ParticipantStore,
) engine.TargetTaskStepResolver {
	if resolver, ok := primary.(engine.TargetTaskStepResolver); ok {
		return resolver
	}
	if resolver, ok := participants.(engine.TargetTaskStepResolver); ok {
		return resolver
	}
	return nil
}

// switchWorkflowDispatcher returns the closure SwitchWorkflowCallback uses
// to fire on_exit / on_enter. It reads svc.workflowEngine lazily — at
// registration time the engine may not yet be initialised, but it is
// guaranteed by the time the closure runs (callbacks only execute after
// HandleTrigger).
func switchWorkflowDispatcher(svc *Service) engine.DispatchTriggerFn {
	return func(ctx context.Context, taskID, sessionID string, trigger engine.Trigger, operationID, sourceStepID string) error {
		eng := svc.workflowEngine
		if eng == nil {
			return nil // engine not initialised; treat as no-op
		}
		// Direct on_enter preflight can create/promote sessions and retire the
		// initiating session, so it must not run for a replayed operation. The
		// engine performs the same check before executing actions, but that is
		// necessarily after this dispatcher-side preparation.
		if operationID != "" && svc.workflowStore != nil {
			applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
			if err != nil {
				return err
			}
			if applied {
				return nil
			}
		}
		ctx = withWorkflowMetaCache(ctx)
		var preloadedState *engine.MachineState
		if trigger == engine.TriggerOnEnter {
			var err error
			var sourceStep *wfmodels.WorkflowStep
			if sourceStepID != "" {
				sourceStep, err = svc.loadWorkflowStepForLifecycle(ctx, sourceStepID, "workflow switch source")
				if err != nil {
					return err
				}
			}
			sessionID, preloadedState, err = svc.prepareDirectWorkflowStepEntry(ctx, taskID, sessionID, sourceStep)
			if err != nil {
				return err
			}
		}
		input := engine.HandleInput{
			TaskID:         taskID,
			SessionID:      sessionID,
			Trigger:        trigger,
			OperationID:    operationID,
			PreloadedState: preloadedState,
		}
		// on_enter: the ledger-driven DispatchStepEntry path (fired by the
		// step-transition writer this switch's AddTaskToWorkflow call
		// commits through) now owns the session-independent half of the
		// entry sequence. Running the full HandleTrigger here would execute
		// those actions a second time. Session-shaped actions have no other
		// production path on this route, so they still run through the
		// engine's callback registry.
		// on_exit is unaffected — it is a different trigger, entirely out of
		// this requirement's scope.
		var err error
		if trigger == engine.TriggerOnEnter {
			_, err = eng.HandleTriggerSessionShapedOnly(ctx, input)
		} else {
			_, err = eng.HandleTrigger(ctx, input)
		}
		return err
	}
}

func (s *Service) prepareDirectWorkflowStepEntry(
	ctx context.Context, taskID, sessionID string,
	sourceStep *wfmodels.WorkflowStep,
) (string, *engine.MachineState, error) {
	ctx = withWorkflowMetaCache(ctx)
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", nil, fmt.Errorf("load task for workflow step entry: %w", err)
	}
	if task == nil {
		return "", nil, fmt.Errorf("task %s not found for workflow step entry", taskID)
	}
	if s.workflowStepGetter == nil || task.WorkflowStepID == "" {
		return "", nil, fmt.Errorf("workflow step is not available for task %s", taskID)
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil {
		return "", nil, fmt.Errorf("load destination workflow step: %w", err)
	}
	if step == nil {
		return "", nil, fmt.Errorf("destination workflow step %s not found", task.WorkflowStepID)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("load session for workflow step entry: %w", err)
	}
	effectiveSession, _, err := s.prepareWorkflowStepSession(ctx, taskID, session, step, sourceStep)
	if err != nil {
		return "", nil, fmt.Errorf("prepare session for workflow step entry: %w", err)
	}
	state := s.buildMachineState(ctx, task, effectiveSession)
	return effectiveSession.ID, &state, nil
}

// enablePlanModeCallback enables plan mode on the session.
type enablePlanModeCallback struct {
	svc *Service
}

func (c *enablePlanModeCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for enable plan mode: %w", err)
	}
	c.svc.setSessionPlanMode(ctx, session, true)
	return engine.ActionResult{}, nil
}

// disablePlanModeCallback disables plan mode on the session.
type disablePlanModeCallback struct {
	svc *Service
}

func (c *disablePlanModeCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for disable plan mode: %w", err)
	}
	c.svc.clearSessionPlanMode(ctx, session)
	return engine.ActionResult{}, nil
}

// setSessionModeCallback applies a workflow-declared session permission mode
// (e.g. "acceptEdits") when entering a step. See issue #1183.
type setSessionModeCallback struct {
	svc *Service
}

func (c *setSessionModeCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	// Skip before any DB lookup: passthrough sessions manage their own mode in
	// the CLI, and an action with no mode is a no-op. Guarding here keeps a
	// skipped action from failing on a session-load error, and mirrors the
	// enable/disable plan-mode callbacks.
	if in.Action.SetSessionMode == nil || in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for set session mode: %w", err)
	}
	// Passthrough is already excluded above, so pass false explicitly; the
	// isPassthrough parameter exists for the legacy processOnEnter call site.
	c.svc.applyStepSessionMode(ctx, session, in.Action.SetSessionMode.Mode, false)
	return engine.ActionResult{}, nil
}

// resetAgentContextCallback restarts the agent subprocess with a fresh ACP session.
type resetAgentContextCallback struct {
	svc *Service
}

func (c *resetAgentContextCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return engine.ActionResult{}, fmt.Errorf("load session for reset agent context: %w", err)
	}
	ok := c.svc.resetAgentContext(ctx, in.State.TaskID, session, in.Step.Name)
	if !ok {
		return engine.ActionResult{}, fmt.Errorf("failed to reset agent context for session %s", in.State.SessionID)
	}
	return engine.ActionResult{}, nil
}

// autoStartAgentCallback sends the auto-start prompt for a workflow step.
type autoStartAgentCallback struct {
	svc *Service
	// launch overrides the direct step launch in tests; nil uses LaunchSession.
	launch func(ctx context.Context, in engine.ActionInput) error
}

// maxBusyAutoStartAttempts bounds how often a QueueIfBusy auto-start retries
// after the launch is refused as busy and the session is no longer mid-turn.
const maxBusyAutoStartAttempts = 3

func (c *autoStartAgentCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.State.IsPassthrough {
		return engine.ActionResult{}, nil
	}

	if in.Action.AutoStartAgent == nil || !in.Action.AutoStartAgent.QueueIfBusy {
		if err := c.launchStep(ctx, in); err != nil {
			return engine.ActionResult{}, fmt.Errorf("auto-start via LaunchSession failed: %w", err)
		}
		return engine.ActionResult{}, nil
	}

	// A launch refused as busy never reached the agent, so the prompt is still
	// owed: queue it if the session is mid-turn, otherwise launch again. Every
	// attempt shares one handoff claim so a refused attempt cannot consume it.
	ctx, admissionErr := c.svc.withWorkflowAutoStartAdmission(ctx, in.State.TaskID, in.State.SessionID)
	if admissionErr != nil {
		return engine.ActionResult{}, admissionErr
	}
	handoffOnce := newStepHandoffOnce()
	ctx = withWorkflowPromptFallback(ctx, in.State.TaskID, in.Step.ID)
	ctx = withStepHandoffOnce(ctx, in.State.TaskID, in.Step.ID, handoffOnce)
	// Each busy rejection, including the last, is followed by a mid-turn check.
	var err error
	for launches := 0; ; launches++ {
		queued, queueErr := c.queueStepPromptIfBusy(ctx, in, handoffOnce)
		if queueErr != nil {
			return engine.ActionResult{}, queueErr
		}
		if queued {
			return engine.ActionResult{}, nil
		}
		if launches == maxBusyAutoStartAttempts {
			break
		}
		err = c.launchStep(ctx, in)
		if err == nil {
			return engine.ActionResult{}, nil
		}
		if !isPromptRejectedAsBusy(err) {
			break
		}
		ctx = context.WithValue(ctx, workflowAutoStartRetryKey{}, true)
	}
	return engine.ActionResult{}, fmt.Errorf("auto-start via LaunchSession failed: %w", err)
}

func (c *autoStartAgentCallback) launchStep(ctx context.Context, in engine.ActionInput) error {
	if c.launch != nil {
		return c.launch(ctx, in)
	}
	_, err := c.svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:         in.State.TaskID,
		Intent:         IntentWorkflowStep,
		SessionID:      in.State.SessionID,
		WorkflowStepID: in.Step.ID,
	})
	return err
}

// queueStepPromptIfBusy queues the step's auto-start prompt when the session
// is mid-turn, so the message queue delivers it once the current turn ends.
// It reports false, without side effects, when the session can take a prompt
// now and the caller should launch directly.
func (c *autoStartAgentCallback) queueStepPromptIfBusy(
	ctx context.Context, in engine.ActionInput, handoffOnce *stepHandoffOnce,
) (bool, error) {
	session, err := c.svc.repo.GetTaskSession(ctx, in.State.SessionID)
	if err != nil {
		return false, fmt.Errorf("load session for queued auto-start: %w", err)
	}
	if err := c.svc.validateWorkflowAutoStartAdmission(ctx, session); err != nil {
		return false, err
	}
	if !sessionMidTurn(session) {
		return false, nil
	}
	if hook := c.svc.afterBusyAutoStartSessionRead; hook != nil {
		hook()
	}
	step, err := c.svc.workflowStepGetter.GetStep(ctx, in.Step.ID)
	if err != nil {
		return false, fmt.Errorf("load workflow step for queued auto-start: %w", err)
	}
	task, err := c.svc.repo.GetTask(ctx, in.State.TaskID)
	if err != nil {
		return false, fmt.Errorf("load task for queued auto-start: %w", err)
	}
	if err := c.svc.withWorkflowAutoStartInputAdmission(ctx, session, step, true, func() error {
		prompt, promptReferenceContext, err := c.svc.buildWorkflowEntryPrompt(
			ctx, task.Description, step, task.ID, session.ID, session.QueueIncarnationID, session.IsPassthrough,
		)
		if err != nil {
			return fmt.Errorf("build workflow prompt for queued auto-start: %w", err)
		}
		planMode := step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode)
		return c.svc.autoStartStepPromptWithPromptContext(
			ctx, task.ID, session, step, prompt, planMode, true, handoffOnce, promptReferenceContext,
		)
	}); err != nil {
		return false, fmt.Errorf("queue auto-start prompt: %w", err)
	}

	c.svc.logger.Info("queued workflow step auto-start until the current turn ends",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("workflow_step_id", step.ID))
	c.drainIfTurnEnded(ctx, session.ID)
	return true, nil
}

// drainIfTurnEnded delivers the queued prompt when the turn ended while it was
// being queued: that turn's ready handler may already have drained an empty
// queue, and no later turn end would deliver it. The drain takes the session
// guard, so it is skipped when the caller already holds it; that caller drains
// the queue itself once it settles the turn.
func (c *autoStartAgentCallback) drainIfTurnEnded(ctx context.Context, sessionID string) {
	if workflowProfileSwitchGuardIsHeld(ctx, sessionID) || cancelInFlightGuardHeldForSession(ctx, sessionID) {
		return
	}
	latest, err := c.svc.repo.GetTaskSession(ctx, sessionID)
	if err != nil || latest == nil || sessionMidTurn(latest) {
		return
	}
	c.svc.drainQueuedMessageForPromptableSession(ctx, sessionID)
}

func sessionMidTurn(session *taskmodels.TaskSession) bool {
	return session.State == taskmodels.TaskSessionStateRunning ||
		session.State == taskmodels.TaskSessionStateStarting
}

// isPromptRejectedAsBusy reports whether a launch was refused before the
// prompt was handed to the agent because the session was running a turn. A
// failure after acceptance, such as a stream disconnect, is not one: queueing
// that prompt again could start a duplicate turn.
func isPromptRejectedAsBusy(err error) bool {
	return isAgentAlreadyRunningError(err) || isSessionBusyError(err)
}

// runCodeReviewCallback starts a native code-review pass over the task's changed
// files when it enters the step.
//
// A review failure never blocks the transition: the run row records the reason
// and the Review panel surfaces it, but a task must not get stuck in a step
// because no reviewer was configured or the provider was down. That is why this
// logs and returns nil instead of propagating the error.
type runCodeReviewCallback struct {
	svc *Service
}

func (c *runCodeReviewCallback) Execute(ctx context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	profileID := ""
	if in.Action.RunCodeReview != nil {
		profileID = in.Action.RunCodeReview.AgentProfileID
	}
	_, err := c.svc.reviewRunner.Launch(ctx, review.RunRequest{
		TaskID:         in.State.TaskID,
		SessionID:      in.State.SessionID,
		AgentProfileID: profileID,
		Trigger:        taskmodels.ReviewTriggerWorkflowStep,
		WorkflowStepID: in.Step.ID,
		EntryID:        in.EntryID,
	})
	if err != nil {
		c.svc.logger.Warn("workflow step code review did not start",
			zap.String("task_id", in.State.TaskID),
			zap.String("workflow_step_id", in.Step.ID),
			zap.Error(err))
	}
	return engine.ActionResult{}, nil
}

// setWorkflowDataCallback writes key/value data into the workflow data bag.
type setWorkflowDataCallback struct{}

func (c *setWorkflowDataCallback) Execute(_ context.Context, in engine.ActionInput) (engine.ActionResult, error) {
	if in.Action.SetWorkflowData == nil {
		return engine.ActionResult{}, nil
	}
	return engine.ActionResult{
		DataPatch: map[string]any{
			in.Action.SetWorkflowData.Key: in.Action.SetWorkflowData.Value,
		},
	}, nil
}

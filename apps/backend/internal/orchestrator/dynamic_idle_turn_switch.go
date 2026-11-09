package orchestrator

import (
	"context"
	"fmt"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// idleTurnCandidateComparisonApplies reports whether a prompt is a chat user
// turn that may compare candidates before it reaches a warm runtime.
// Synchronous callers that read the PromptResult, replays, recovery, queue and
// lifecycle deliveries, explicit model switches and prompts bound to an
// existing execution keep their admitted candidate, because they carry
// ownership or a result that only the ordinary prompt path settles.
func idleTurnCandidateComparisonApplies(
	ctx context.Context, model string, origin launchOrigin, options promptTaskOptions,
) bool {
	return options.chatUserTurn && origin == launchOriginManual && model == "" && retryLaunchInContext(ctx) == nil &&
		!options.internalContinuation && options.recoveryAction == "" && options.claimEntryID == "" &&
		!options.lifecyclePrompt && options.liveExecutionFence == nil && options.resumeAttempt == nil &&
		!options.allowRouteActionPrompt && !options.initialTaskBriefDispatchOwner &&
		options.ceilingEntryBinding == nil && promptCarriesNoDispatchOwnership(options)
}

// promptCarriesNoDispatchOwnership reports that no caller callback, recovery
// record or delivery claim waits on the ordinary prompt dispatch.
func promptCarriesNoDispatchOwnership(options promptTaskOptions) bool {
	return options.promptDispatchRecovery == nil &&
		options.expectedCurrentTurnID == "" && options.expectedSessionIdentity == nil &&
		options.deliveryProtocol == "" && options.deliveryClaimUpdater == nil &&
		options.afterClaim == nil && options.afterDispatch == nil && options.beforeDispatch == nil &&
		options.beforeProviderAdmission == nil && options.afterDispatchAdmission == nil &&
		options.onAccepted == nil && !options.firstLaunchPromptContext &&
		!options.preserveInitialSubmissionReplay && options.retryPrompt == nil
}

// switchIdleDynamicCandidateForNewTurn applies the keep_model_while_running=false
// comparison at the new-turn boundary of a warm dynamic session. It runs before
// the prompt claims the session, and only while the session is idle: waiting
// for input, no active turn, no live background work, and a runtime whose user
// shells do not belong to the agent execution. When the comparison keeps the
// current candidate, the prompt continues on the warm runtime. When another
// candidate wins, the turn is handed over through the dynamic successor path:
// the idle predecessor is stopped and the successor launches with this prompt
// and the bounded continuation package. handled reports that the prompt was
// launched (or failed) here and the caller must not dispatch it again.
func (s *Service) switchIdleDynamicCandidateForNewTurn(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	origin launchOrigin,
	options promptTaskOptions,
) (result *PromptResult, handled bool, err error) {
	if !idleTurnCandidateComparisonApplies(ctx, model, origin, options) ||
		cancelInFlightGuardHeldForSession(ctx, sessionID) {
		return nil, false, nil
	}
	executionID, warm := s.warmKeepModelOffExecution(ctx, taskID, sessionID)
	if !warm {
		return nil, false, nil
	}
	releaseRouteAction := s.acquireRouteActionOperationLock(sessionID)
	defer releaseRouteAction()
	// Like the detached successor worker, hold the session guard from the idle
	// check through the replacement launch so no turn is admitted in between.
	guard, releaseGuard := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	defer func() {
		guard.Unlock()
		releaseGuard()
	}()
	ctx = withCancelInFlightGuardHeldForSession(ctx, sessionID)
	ctx = withExecutionReplacementStopOwnerGuardHeldForSession(ctx, sessionID)
	session, state, idle := s.idleDynamicTurnBoundary(ctx, sessionID, executionID)
	if !idle {
		return nil, false, nil
	}
	previousExecutionProfileID := session.ExecutionProfileID
	decision, _, err := s.reevaluatedDynamicLaunchDecision(ctx, session, *state)
	if err != nil {
		return nil, true, err
	}
	if decision.ExecutionProfileID == previousExecutionProfileID {
		// The warm runtime keeps serving the turn under the claimed generation.
		if err := s.markDynamicRouteActive(ctx, sessionID, decision.Generation); err != nil {
			return nil, true, err
		}
		return nil, false, nil
	}
	s.logger.Info("switching idle dynamic session to a different candidate for a new turn",
		zap.String("task_id", taskID), zap.String("session_id", sessionID),
		zap.String("from", previousExecutionProfileID), zap.String("to", decision.ExecutionProfileID),
		zap.Int64("generation", decision.Generation))
	return s.handOverIdleDynamicTurn(
		ctx, taskID, sessionID, executionID, decision.Generation,
		capturedPrompt{text: prompt, planMode: planMode, attachments: attachments},
		dynamicSuccessorPrompt{
			references:               options.entityReferences,
			promptReferenceContext:   options.promptReferenceContext,
			promptReferencesPrepared: options.promptReferencesPrepared,
			deliverySubmissionID:     options.deliverySubmissionID,
		},
	)
}

// warmKeepModelOffExecution returns the live execution of a dynamic session
// whose profile does not keep its model. A cold session keeps its own resume path.
func (s *Service) warmKeepModelOffExecution(ctx context.Context, taskID, sessionID string) (string, bool) {
	if s.profileExecutionResolver == nil || !s.profileExecutionResolver.Enabled() {
		return "", false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID ||
		session.RouteGeneration <= 0 || session.ExecutionProfileID == "" ||
		s.profileExecutionResolver.KeepModelWhileRunning(ctx, session.AgentProfileID) {
		return "", false
	}
	execution, ok := s.executor.GetExecutionBySession(sessionID)
	if !ok || execution == nil || execution.AgentExecutionID == "" {
		return "", false
	}
	return execution.AgentExecutionID, true
}

// idleDynamicTurnBoundary reloads the session under the route-action lock and
// reports whether its warm runtime is idle and the route is the active one the
// runtime serves. Any running or unknown work keeps the current candidate.
func (s *Service) idleDynamicTurnBoundary(
	ctx context.Context, sessionID, executionID string,
) (*models.TaskSession, *dynamicruntime.RouteState, bool) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.State != models.TaskSessionStateWaitingForInput ||
		session.IsPassthrough || session.AgentExecutionID != executionID ||
		session.RouteState != dynamicRouteStatusActive {
		return nil, nil, false
	}
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return nil, nil, false
	}
	state, err := loader.LoadRouteState(ctx, sessionID)
	if err != nil || state == nil || state.Status != dynamicRouteStatusActive ||
		state.LogicalProfileID != session.AgentProfileID || state.Generation != session.RouteGeneration ||
		state.ExecutionProfileID != session.ExecutionProfileID {
		return nil, nil, false
	}
	if !s.idleDynamicRuntime(ctx, sessionID, executionID) {
		return nil, nil, false
	}
	return session, state, true
}

// idleDynamicRuntime reports that no turn, cancellation, queued dispatch, steer,
// user-shell ownership or background work would be interrupted by a stop.
func (s *Service) idleDynamicRuntime(ctx context.Context, sessionID, executionID string) bool {
	if s.isCancelInFlight(sessionID) || s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return false
	}
	if s.turnService != nil {
		activeTurn, turnErr := s.turnService.GetActiveTurn(ctx, sessionID)
		if (turnErr != nil && !isNoActiveTurnError(turnErr)) || activeTurn != nil {
			return false
		}
	}
	// User shells of a standalone runtime live in the backend's interactive
	// runner, keyed by task environment, so stopping the agent execution does
	// not end them. Other runtimes host shells inside the execution.
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.AgentExecutionID != executionID ||
		running.Runtime != agentruntime.RuntimeStandalone {
		return false
	}
	return s.backgroundWorkSettled(ctx, sessionID)
}

// backgroundWorkSettled samples the agent's background workloads with the
// parked-probe budget. Only an explicit "settled" answer allows a handover.
func (s *Service) backgroundWorkSettled(ctx context.Context, sessionID string) bool {
	if s.executor == nil {
		return false
	}
	budget := s.backgroundProbeConfig.Budget
	if budget <= 0 {
		budget = defaultParkedProbeBudget
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	result, err := s.executor.ProbeBackgroundWorkloads(probeCtx, sessionID)
	return err == nil && result == executor.ProbeResultSettled
}

// handOverIdleDynamicTurn stops the idle predecessor and launches the claimed
// successor candidate with the new turn's prompt, using the same steps as a
// dynamic successor relaunch. A failure marks the claimed generation
// action_required so the session keeps its route recovery surface.
func (s *Service) handOverIdleDynamicTurn(
	ctx context.Context,
	taskID, sessionID, predecessorExecutionID string,
	generation int64,
	prompt capturedPrompt,
	direct dynamicSuccessorPrompt,
) (result *PromptResult, handled bool, err error) {
	succeeded := false
	defer func() {
		if !succeeded {
			s.markDynamicRouteActionRequired(context.WithoutCancel(ctx), sessionID, generation, "keep_model_reevaluation_handover_failed")
		}
	}()
	if !s.stopDynamicRelaunchPredecessor(ctx, predecessorExecutionID) {
		return nil, true, fmt.Errorf("stop idle dynamic predecessor %q before candidate switch", predecessorExecutionID)
	}
	if !s.resetDynamicRelaunchSession(ctx, sessionID, models.TaskSessionStateCreated) {
		return nil, true, fmt.Errorf("reset idle dynamic session %q before candidate switch", sessionID)
	}
	s.retireExecutionActivityAndPublish(ctx, taskID, sessionID, predecessorExecutionID)
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil, true, fmt.Errorf("reload dynamic session %q for candidate switch: %w", sessionID, err)
	}
	if _, err := s.startDynamicSuccessorCreatedSession(ctx, taskID, session, prompt, direct); err != nil {
		return nil, true, err
	}
	succeeded = true
	return &PromptResult{}, true, nil
}

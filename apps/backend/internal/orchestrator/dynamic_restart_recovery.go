package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// Route states that a resume re-claims before the runtime is ready.
const (
	dynamicRouteStatusStarting = "starting"
	dynamicRouteStatusRetrying = "retrying"
)

// activateResumedDynamicRoute completes the durable "starting" -> "active"
// route transition for a resumed dynamic session whose runtime is ready. It
// returns the transition error so the resume path does not report success while
// the claimed route is still "starting".
func (s *Service) activateResumedDynamicRoute(ctx context.Context, session *models.TaskSession) error {
	if session == nil || session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return nil
	}
	if session.RouteState != dynamicRouteStatusStarting && session.RouteState != dynamicRouteStatusRetrying {
		return nil
	}
	return s.markDynamicRouteActive(context.WithoutCancel(ctx), session.ID, session.RouteGeneration)
}

// completeReadyStrandedStartingRoute completes the durable "starting" ->
// "active" transition for a claim whose generation already has a live,
// prompt-ready runtime. It is the reconciling counterpart of the process-start
// callback: it retries the activation write for the same generation without
// stopping or replacing the runtime, and reports whether the session
// projection now reads "active".
func (s *Service) completeReadyStrandedStartingRoute(ctx context.Context, session *models.TaskSession) bool {
	if s.profileExecutionResolver == nil || !s.profileExecutionResolver.Enabled() {
		return false
	}
	if !isStartingRouteClaim(session) || !routeActivationSessionState(session.State) {
		return false
	}
	lock, release := s.acquireCancelInFlightGuard(session.ID)
	lock.Lock()
	defer func() { lock.Unlock(); release() }()
	if s.isCancelInFlight(session.ID) {
		return false
	}
	current, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil || !sameStartingRouteOwner(current, session) {
		return false
	}
	if !s.hasLivePromptReadyAgent(ctx, session) {
		return false
	}
	if s.markDynamicRouteActive(ctx, session.ID, session.RouteGeneration) != nil {
		return false
	}
	projection := s.activeRouteProjection(ctx, session)
	if projection == nil {
		return false
	}
	session.RouteState = projection.RouteState
	session.RouteReason = projection.RouteReason
	session.UpdatedAt = projection.UpdatedAt
	return true
}

func sameStartingRouteOwner(current, expected *models.TaskSession) bool {
	return isStartingRouteClaim(current) && sameRouteActivationOwner(current, expected)
}

func sameRouteActivationOwner(current, expected *models.TaskSession) bool {
	return current != nil && routeActivationSessionState(current.State) &&
		current.RouteGeneration == expected.RouteGeneration &&
		current.ExecutionProfileID == expected.ExecutionProfileID &&
		current.AgentExecutionID == expected.AgentExecutionID
}

func (s *Service) activeRouteProjection(ctx context.Context, expected *models.TaskSession) *models.TaskSession {
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return nil
	}
	state, err := loader.LoadRouteState(ctx, expected.ID)
	if err != nil || state == nil || state.Generation != expected.RouteGeneration ||
		state.Status != dynamicRouteStatusActive {
		return nil
	}
	current, err := s.repo.GetTaskSession(ctx, expected.ID)
	if err != nil || !sameRouteActivationOwner(current, expected) || current.RouteState != dynamicRouteStatusActive {
		return nil
	}
	return current
}

// isStartingRouteClaim reports whether a session carries a claimed dynamic
// route that has not reached "active": a positive generation and execution
// profile behind a "starting" route state.
func isStartingRouteClaim(session *models.TaskSession) bool {
	return session != nil && session.RouteState == dynamicRouteStatusStarting &&
		session.RouteGeneration > 0 && session.ExecutionProfileID != ""
}

// hasLivePromptReadyAgent reports whether the session already has a running
// agent that can accept an immediate prompt, the ownership evidence a
// reconciling completion needs before it retries the durable activation.
func (s *Service) hasLivePromptReadyAgent(ctx context.Context, session *models.TaskSession) bool {
	if s.agentManager == nil || session.AgentExecutionID == "" ||
		!s.agentManager.IsAgentRunningForSession(ctx, session.ID) ||
		!s.agentManager.IsAgentReadyForPrompt(ctx, session.ID) {
		return false
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, session.ID)
	if err != nil || executionID != session.AgentExecutionID {
		return false
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	return err == nil && running != nil && running.AgentExecutionID == executionID &&
		running.ExecutionProfileID == session.ExecutionProfileID
}

func routeActivationSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting || state == models.TaskSessionStateRunning ||
		state == models.TaskSessionStateWaitingForInput
}

// explicitRecoveryOwnsStrandedStartingRoute reports whether an explicit
// recovery action may resume a dynamic session whose route claim is still
// "starting" with no live runtime behind it. A live launch owner keeps the
// dynamic_route_pending guard; passive resumes are unchanged.
func (s *Service) explicitRecoveryOwnsStrandedStartingRoute(
	ctx context.Context,
	session *models.TaskSession,
	options executor.ResumeOptions,
) bool {
	if session == nil || options.RecoveryAction == "" ||
		session.RouteState != dynamicRouteStatusStarting ||
		session.RouteGeneration <= 0 || session.ExecutionProfileID == "" {
		return false
	}
	return s.agentManager == nil || !s.agentManager.IsAgentRunningForSession(ctx, session.ID)
}

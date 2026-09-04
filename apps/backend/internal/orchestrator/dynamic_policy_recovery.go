package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type dynamicRouteStateLister interface {
	ListPendingRouteStates(context.Context) ([]dynamicruntime.RouteState, error)
}

type dynamicStartingRouteLister interface {
	ListStartingRouteStates(context.Context) ([]dynamicruntime.RouteState, error)
}

// startDynamicPolicyRecovery restores only durable waits whose dispatch has
// not started. A persisted retrying state is intentionally left for manual
// recovery because a process restart cannot prove whether its launch crossed
// the provider boundary.
func (s *Service) startDynamicPolicyRecovery(ctx context.Context) {
	if s.profileExecutionResolver == nil || !s.profileExecutionResolver.Enabled() {
		return
	}
	lister, ok := s.repo.(dynamicRouteStateLister)
	if !ok {
		return
	}
	recoveryCtx, cancel := context.WithCancel(context.Background())
	s.dynamicRecoveryMu.Lock()
	s.dynamicRecoveryCtx = recoveryCtx
	s.dynamicRecoveryCancel = cancel
	if s.dynamicRecoveryTimers == nil {
		s.dynamicRecoveryTimers = make(map[string]*time.Timer)
	}
	s.dynamicRecoveryMu.Unlock()

	states, err := lister.ListPendingRouteStates(ctx)
	if err != nil {
		s.logger.Warn("failed to restore dynamic policy recovery deadlines", zap.Error(err))
		return
	}
	for _, state := range states {
		s.scheduleDynamicPolicyRecovery(state.SessionID, state.Generation, state.PolicyStateJSON)
	}
}

// reconcileOrphanedDynamicStartingRoutes recovers routes left at "starting"
// with no launch owner: a launch that failed after claiming a generation but
// before reaching a terminal route status (see routeDynamicAgentFailure and
// LaunchDynamicRouteAction) can strand a route this way, and no timer or
// event fires for a durable status that is not a pending wait. A route that
// reached MarkActive is no longer "starting", so this sweep cannot demote a
// healthy idling dynamic session.
func (s *Service) reconcileOrphanedDynamicStartingRoutes(ctx context.Context) {
	if s.profileExecutionResolver == nil {
		return
	}
	lister, ok := s.repo.(dynamicStartingRouteLister)
	if !ok {
		return
	}
	states, err := lister.ListStartingRouteStates(ctx)
	if err != nil {
		s.logger.Warn("failed to list starting dynamic route states", zap.Error(err))
		return
	}
	for _, state := range states {
		s.reconcileOrphanedDynamicStartingRoute(ctx, state)
	}
}

func (s *Service) reconcileOrphanedDynamicStartingRoute(ctx context.Context, state dynamicruntime.RouteState) {
	session, err := s.repo.GetTaskSession(ctx, state.SessionID)
	if err != nil || session == nil || session.RouteGeneration != state.Generation {
		return
	}
	if !isOrphanableDynamicSessionState(session.State) {
		return
	}
	s.markDynamicRouteActionRequired(ctx, state.SessionID, state.Generation, "orphaned_starting_route")
}

// isOrphanableDynamicSessionState reports whether a session's current state
// is consistent with "a route claimed as starting has no in-flight launch
// owner". STARTING means a launch was under way when the process that owned
// it stopped; IDLE (office runs only) means the executor was already torn
// down between turns with no successor launched. Every other state either
// already has a live owner (RUNNING), has not attempted a launch for this
// claim yet (CREATED — PrepareSession's background workspace launch claims a
// route ahead of the user's first prompt, which can sit unstarted for a
// long time by design), or is a terminal/parked outcome the ordinary
// session UI already explains (WAITING_FOR_INPUT, COMPLETED, FAILED,
// CANCELLED). Flagging those would put a stale "Retry / Try next / Stop"
// banner on a session with nothing left to recover — including every
// dynamic session that predates MarkActive, which is "starting" for exactly
// this reason and not because anything is actually stuck.
func isOrphanableDynamicSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting || state == models.TaskSessionStateIdle
}

func (s *Service) stopDynamicPolicyRecovery() {
	s.dynamicRecoveryMu.Lock()
	defer s.dynamicRecoveryMu.Unlock()
	if s.dynamicRecoveryCancel != nil {
		s.dynamicRecoveryCancel()
	}
	for sessionID, timer := range s.dynamicRecoveryTimers {
		timer.Stop()
		delete(s.dynamicRecoveryTimers, sessionID)
	}
	s.dynamicRecoveryCtx = nil
	s.dynamicRecoveryCancel = nil
}

func (s *Service) scheduleDynamicPolicyRecovery(sessionID string, generation int64, rawPolicyState string) {
	if sessionID == "" || generation <= 0 {
		return
	}
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(rawPolicyState), &policyState); err != nil || policyState.Deadline == nil {
		return
	}
	deadline := policyState.Deadline.UTC()
	s.dynamicRecoveryMu.Lock()
	ctx := s.dynamicRecoveryCtx
	if ctx == nil {
		s.dynamicRecoveryMu.Unlock()
		return
	}
	if previous := s.dynamicRecoveryTimers[sessionID]; previous != nil {
		previous.Stop()
	}
	delay := time.Until(deadline)
	if delay < 0 {
		delay = 0
	}
	s.dynamicRecoveryTimers[sessionID] = time.AfterFunc(delay, func() {
		s.runDynamicPolicyRecovery(ctx, sessionID, generation)
	})
	s.dynamicRecoveryMu.Unlock()
}

func (s *Service) runDynamicPolicyRecovery(ctx context.Context, sessionID string, generation int64) {
	s.removeDynamicPolicyRecoveryTimer(sessionID)
	if !s.dynamicPolicyRecoveryActive(ctx) {
		return
	}
	loader, ok := s.repo.(dynamicRouteStateLoader)
	if !ok {
		return
	}
	// Hold the same per-session guard ApplyRouteAction holds through the
	// durable claim and task-session projection. Release it before lifecycle
	// launch because that path acquires the session lifecycle lock.
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	lock.Lock()
	locked := true
	defer func() {
		if locked {
			lock.Unlock()
			release()
		}
	}()
	state, ok := loadDueDynamicPolicyState(ctx, loader, sessionID, generation)
	if !ok {
		return
	}
	if s.rescheduleEarlyDynamicPolicyRecovery(sessionID, generation, state.PolicyStateJSON) {
		return
	}
	resolved, err := s.profileExecutionResolver.ResumePendingRoute(ctx, sessionID, generation)
	if s.handleDynamicPolicyResumeError(ctx, loader, sessionID, generation, err) {
		return
	}
	if !s.persistDynamicPolicyRecovery(ctx, sessionID, generation, resolved) {
		return
	}
	lock.Unlock()
	release()
	locked = false
	if err := s.LaunchDynamicRouteAction(ctx, sessionID); err != nil {
		s.markDynamicPolicyRecoveryActionRequired(ctx, sessionID, generation, err)
	}
}

func (s *Service) removeDynamicPolicyRecoveryTimer(sessionID string) {
	s.dynamicRecoveryMu.Lock()
	delete(s.dynamicRecoveryTimers, sessionID)
	s.dynamicRecoveryMu.Unlock()
}

func (s *Service) dynamicPolicyRecoveryActive(ctx context.Context) bool {
	return ctx != nil && ctx.Err() == nil &&
		s.profileExecutionResolver != nil && s.profileExecutionResolver.Enabled()
}

func loadDueDynamicPolicyState(
	ctx context.Context,
	loader dynamicRouteStateLoader,
	sessionID string,
	generation int64,
) (*dynamicruntime.RouteState, bool) {
	state, err := loader.LoadRouteState(ctx, sessionID)
	if err != nil || state == nil || state.Generation != generation {
		return nil, false
	}
	if state.Status != string(routingpolicy.DecisionRetry) && state.Status != string(routingpolicy.DecisionWaitForReset) {
		return nil, false
	}
	return state, true
}

func (s *Service) rescheduleEarlyDynamicPolicyRecovery(sessionID string, generation int64, rawPolicyState string) bool {
	deadline := dynamicPolicyDeadline(rawPolicyState)
	if deadline == nil || !time.Now().UTC().Before(*deadline) {
		return false
	}
	s.scheduleDynamicPolicyRecovery(sessionID, generation, rawPolicyState)
	return true
}

func (s *Service) handleDynamicPolicyResumeError(
	ctx context.Context,
	loader dynamicRouteStateLoader,
	sessionID string,
	generation int64,
	err error,
) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, dynamicruntime.ErrRecoveryNotDue) {
		if refreshed, loadErr := loader.LoadRouteState(ctx, sessionID); loadErr == nil && refreshed != nil {
			s.scheduleDynamicPolicyRecovery(sessionID, generation, refreshed.PolicyStateJSON)
		}
	}
	if !errors.Is(err, dynamicruntime.ErrStaleGeneration) && !errors.Is(err, dynamicruntime.ErrRouteStateNotFound) {
		s.logger.Warn("dynamic policy recovery could not resume route",
			zap.String("session_id", sessionID), zap.Error(err))
	}
	return true
}

func (s *Service) persistDynamicPolicyRecovery(
	ctx context.Context,
	sessionID string,
	generation int64,
	resolved agentruntime.ProfileExecution,
) bool {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration != generation || session.ExecutionProfileID != resolved.ExecutionProfileID {
		return false
	}
	applyDynamicRouteDecisionProjection(session, resolved.Decision)
	session.RouteState = resolved.Decision.Status
	session.State = models.TaskSessionStateCreated
	session.DownstreamACPSessionID = ""
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to persist dynamic policy retry state",
			zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	return true
}

func dynamicPolicyDeadline(rawPolicyState string) *time.Time {
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(rawPolicyState), &policyState); err != nil || policyState.Deadline == nil {
		return nil
	}
	deadline := policyState.Deadline.UTC()
	return &deadline
}

func (s *Service) markDynamicPolicyRecoveryActionRequired(
	ctx context.Context, sessionID string, generation int64, launchErr error,
) {
	if s.profileExecutionResolver != nil {
		if err := s.profileExecutionResolver.MarkRouteActionRequired(ctx, sessionID, generation); err != nil {
			s.logger.Warn("failed to sync durable route state to action_required after launch failure",
				zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	session.RouteState = "action_required"
	session.RouteReason = "route_action_launch_failed"
	session.State = models.TaskSessionStateWaitingForInput
	session.ErrorMessage = launchErr.Error()
	session.DownstreamACPSessionID = ""
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to persist dynamic policy launch failure",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

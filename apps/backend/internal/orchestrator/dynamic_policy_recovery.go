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

// startDynamicPolicyRecovery restores only durable waits whose dispatch has
// not started. A persisted retrying state is intentionally left for manual
// recovery because a process restart cannot prove whether its launch crossed
// the provider boundary.
func (s *Service) startDynamicPolicyRecovery(ctx context.Context) {
	if s.profileExecutionResolver == nil {
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
	s.persistDynamicPolicyRecovery(ctx, sessionID, generation, resolved)
}

func (s *Service) removeDynamicPolicyRecoveryTimer(sessionID string) {
	s.dynamicRecoveryMu.Lock()
	delete(s.dynamicRecoveryTimers, sessionID)
	s.dynamicRecoveryMu.Unlock()
}

func (s *Service) dynamicPolicyRecoveryActive(ctx context.Context) bool {
	return ctx != nil && ctx.Err() == nil && s.profileExecutionResolver != nil
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
) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.RouteGeneration != generation || session.ExecutionProfileID != resolved.ExecutionProfileID {
		return
	}
	applyDynamicRouteDecisionProjection(session, resolved.Decision)
	session.RouteState = resolved.Decision.Status
	session.State = models.TaskSessionStateCreated
	session.DownstreamACPSessionID = ""
	if err := s.repo.UpdateTaskSession(ctx, session); err != nil {
		s.logger.Warn("failed to persist dynamic policy retry state",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if err := s.LaunchDynamicRouteAction(ctx, sessionID); err != nil {
		s.markDynamicPolicyRecoveryActionRequired(ctx, sessionID, err)
	}
}

func dynamicPolicyDeadline(rawPolicyState string) *time.Time {
	var policyState dynamicruntime.PolicyState
	if err := json.Unmarshal([]byte(rawPolicyState), &policyState); err != nil || policyState.Deadline == nil {
		return nil
	}
	deadline := policyState.Deadline.UTC()
	return &deadline
}

func (s *Service) markDynamicPolicyRecoveryActionRequired(ctx context.Context, sessionID string, launchErr error) {
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

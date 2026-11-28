package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Mid-turn provider failover is the workspace opt-in exception to the
// pre-result safety contract of dynamic routing. By default a classified provider failure that arrives after the attempt
// produced output or effects never replaces the provider; the session keeps
// its ordinary recovery surface. When the task's workspace enables
// DynamicMidTurnFailoverEnabled, a provider-capacity failure instead performs
// the same atomic Try next route action a person would submit, once per
// route generation. The failed prompt and its tools are never replayed: the
// successor receives only the bounded continuation package built from durable
// task state, which tells it to continue the existing work.

const (
	// midTurnFailoverAdmissionAttempts bounds how long the detached worker
	// waits for the failed turn to settle before giving up. The failure
	// handlers complete the turn under the session guard the worker also
	// takes, so in practice the first attempt is admitted.
	midTurnFailoverAdmissionAttempts = 20
	midTurnFailoverAdmissionDelay    = 100 * time.Millisecond
)

// errAutomaticRouteActionNotAdmissible reports that the trigger of an
// unattended route action no longer holds under the route-action lock.
var errAutomaticRouteActionNotAdmissible = errors.New("automatic route action is no longer admissible")

// midTurnProviderCapacityFailure reports whether a classified failure is a
// provider-capacity condition another candidate can serve. The set matches
// the codes the shared policy already treats as provider capacity
// (routingerr.Decide long retry): rate limits, exhausted quota or usage, and
// a temporarily unavailable, overloaded or capacity-limited provider or
// model. Codes whose classification forbids fallback, local network and
// agent-transport failures, account restrictions, credential or
// configuration errors, user denials, task/runtime errors and every
// unclassified failure are excluded.
func midTurnProviderCapacityFailure(classified *routingerr.Error) bool {
	if classified == nil || !classified.FallbackAllowed {
		return false
	}
	switch classified.Code {
	case routingerr.CodeRateLimited, routingerr.CodeQuotaLimited,
		routingerr.CodeProviderUnavailable, routingerr.CodeProviderOverloaded,
		routingerr.CodeModelCapacity:
		return true
	default:
		return false
	}
}

// midTurnEvidence reports current, known evidence that the failed attempt
// produced output or effects. Pre-result-safe failures keep the ordinary
// policy path, and unknown evidence fails closed.
func midTurnEvidence(data watcher.AgentEventData) bool {
	return data.DynamicRouteAttempt && data.EvidenceKnown && data.AgentExecutionID != "" &&
		(data.OutputObserved || data.EffectObserved)
}

// midTurnCurrentAttempt fences the trigger on the live dynamic prompt
// attempt: the execution and prompt generation captured when the prompt
// began must still own the session's evidence record. A late failure of an
// earlier attempt, or one whose evidence was reset, never hands off.
func (s *Service) midTurnCurrentAttempt(data watcher.AgentEventData, session *models.TaskSession) bool {
	return midTurnEvidence(data) && data.PromptGeneration != 0 && !isTerminalSessionState(session.State) &&
		s.currentDynamicPromptAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
}

type midTurnFailoverFailure struct {
	executionProfileID string
	code               routingerr.Code
}

// midTurnFailoverChain records the consecutive automatic handoffs of one
// session since its last completed turn. failedGeneration is the generation
// whose failure owns the pending or finished handoff; successorGeneration is
// the generation that handoff claimed (zero until it is claimed).
// failedExecutions keeps every execution that already handed off, so a
// redelivered failure is never charged to the successor's generation.
type midTurnFailoverChain struct {
	failedGeneration    int64
	successorGeneration int64
	failures            []midTurnFailoverFailure
	failedExecutions    map[string]struct{}
}

type midTurnFailoverState struct {
	mu     sync.Mutex
	chains map[string]*midTurnFailoverChain
}

// claimMidTurnFailover admits at most one automatic handoff per failed route
// generation and stops a chain that sees the same classified code on the
// same candidate again before any turn completes.
func (st *midTurnFailoverState) claim(
	sessionID string,
	generation int64,
	executionID, executionProfileID string,
	code routingerr.Code,
) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.chains == nil {
		st.chains = make(map[string]*midTurnFailoverChain)
	}
	chain := st.chains[sessionID]
	if chain != nil {
		if _, seen := chain.failedExecutions[executionID]; seen || chain.failedGeneration == generation {
			// A redelivery of a failure that already handed off, or a second
			// failure in a generation that already handed off.
			return false
		}
		if chain.successorGeneration != generation {
			// The route changed by other means since the last automatic
			// handoff, so this failure starts a new chain.
			chain = &midTurnFailoverChain{failedExecutions: chain.failedExecutions}
		}
	} else {
		chain = &midTurnFailoverChain{}
	}
	if chain.failedExecutions == nil {
		chain.failedExecutions = make(map[string]struct{})
	}
	chain.failedExecutions[executionID] = struct{}{}
	for _, failure := range chain.failures {
		if failure.executionProfileID == executionProfileID && failure.code == code {
			// Keep the record so redeliveries of this failure stay declined.
			chain.failedGeneration = generation
			chain.successorGeneration = 0
			st.chains[sessionID] = chain
			return false
		}
	}
	chain.failures = append(chain.failures, midTurnFailoverFailure{executionProfileID: executionProfileID, code: code})
	chain.failedGeneration = generation
	chain.successorGeneration = 0
	st.chains[sessionID] = chain
	return true
}

// pending reports whether the handoff claimed for generation still owns the
// session, i.e. no completed turn reset the chain in the meantime.
func (st *midTurnFailoverState) pending(sessionID string, generation int64) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	chain := st.chains[sessionID]
	return chain != nil && chain.failedGeneration == generation && chain.successorGeneration == 0
}

func (st *midTurnFailoverState) recordSuccessor(sessionID string, failedGeneration, successorGeneration int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	chain := st.chains[sessionID]
	if chain == nil || chain.failedGeneration != failedGeneration {
		return
	}
	chain.successorGeneration = successorGeneration
}

func (st *midTurnFailoverState) reset(sessionID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.chains, sessionID)
}

// resetMidTurnFailoverChain ends a session's consecutive-failure chain after
// a turn completes, so a later failure is not treated as a repeat.
func (s *Service) resetMidTurnFailoverChain(sessionID string) {
	if sessionID == "" {
		return
	}
	s.midTurnFailovers.reset(sessionID)
}

// workspaceAllowsMidTurnFailover reads the per-workspace opt-in. Any lookup
// failure keeps the default (off).
func (s *Service) workspaceAllowsMidTurnFailover(ctx context.Context, taskID string) bool {
	workspace := s.taskWorkspace(ctx, taskID)
	return workspace != nil && workspace.DynamicMidTurnFailoverEnabled
}

// taskWorkspace returns the workspace of a task, or nil when any lookup
// fails, so per-workspace dynamic routing opt-ins keep their default (off).
func (s *Service) taskWorkspace(ctx context.Context, taskID string) *models.Workspace {
	if taskID == "" || s.repo == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return nil
	}
	workspace, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return nil
	}
	return workspace
}

// scheduleMidTurnProviderFailover runs after the ordinary failure route
// declined a classified failure. It never changes that decline: the caller
// still settles the failed turn into its normal recovery state. When the
// failure qualifies, a detached worker then submits one automatic Try next
// for the failed generation once that turn has settled.
func (s *Service) scheduleMidTurnProviderFailover(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
	classified *routingerr.Error,
) bool {
	if session == nil || !midTurnProviderCapacityFailure(classified) || !s.midTurnCurrentAttempt(data, session) ||
		s.isOfficeSession(ctx, session.ID) || !s.workspaceAllowsMidTurnFailover(ctx, session.TaskID) {
		return false
	}
	generation := session.RouteGeneration
	if !s.midTurnFailovers.claim(session.ID, generation, data.AgentExecutionID, session.ExecutionProfileID, classified.Code) {
		s.logger.Info("mid-turn provider failover declined; route left for manual recovery",
			zap.String("session_id", session.ID), zap.Int64("route_generation", generation),
			zap.String("error_code", string(classified.Code)))
		return false
	}
	s.dynamicSuccessorMu.Lock()
	if s.dynamicSuccessorStopped {
		s.dynamicSuccessorMu.Unlock()
		return false
	}
	if s.dynamicSuccessorCtx == nil {
		s.dynamicSuccessorCtx, s.dynamicSuccessorCancel = context.WithCancel(context.Background())
	}
	workerCtx := s.dynamicSuccessorCtx
	s.dynamicSuccessorWorkers.Add(1)
	s.dynamicSuccessorMu.Unlock()
	sessionID, executionID := session.ID, data.AgentExecutionID
	failure := midTurnFailoverFailure{executionProfileID: session.ExecutionProfileID, code: classified.Code}
	go func() {
		defer s.dynamicSuccessorWorkers.Done()
		runCtx, cancel := context.WithTimeout(workerCtx, dynamicSuccessorDetachedTimeout)
		defer cancel()
		s.runMidTurnProviderFailover(runCtx, sessionID, executionID, generation, failure)
	}()
	return true
}

func (s *Service) runMidTurnProviderFailover(
	ctx context.Context,
	sessionID, executionID string,
	generation int64,
	failure midTurnFailoverFailure,
) {
	request := RouteActionRequest{
		SessionID: sessionID, Action: RouteActionTryNext, ExpectedGeneration: generation, Automatic: true,
	}
	admit := func(ctx context.Context) bool {
		return s.midTurnFailoverAdmissible(ctx, sessionID, executionID, generation)
	}
	var result *RouteActionResult
	var err error
	for attempt := 0; attempt < midTurnFailoverAdmissionAttempts; attempt++ {
		result, err = s.applyAutomaticRouteAction(ctx, request, admit)
		if !errors.Is(err, ErrRouteActionActiveTurn) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(midTurnFailoverAdmissionDelay):
		}
	}
	if err != nil {
		s.logger.Warn("mid-turn provider failover did not run; route left for manual recovery",
			zap.String("session_id", sessionID), zap.Int64("route_generation", generation), zap.Error(err))
		return
	}
	if result == nil {
		return
	}
	s.midTurnFailovers.recordSuccessor(sessionID, generation, result.RouteGeneration)
	if result.State == dynamicRouteStatusWaiting {
		// No candidate could take over: the route is exhausted, which is
		// where an opted-in automatic failed-route retry applies.
		s.scheduleFailedRouteAutoRetry(ctx, sessionID, dynamicruntime.FailedRouteFailure{
			ExecutionProfileID: failure.executionProfileID, Code: failure.code,
		})
	}
}

// midTurnFailoverAdmissible confirms, under the route-action lock, that the
// failed generation still owns a settled session: no completed turn reset the
// chain, the route did not move, and no other execution took the session. The
// execution projection is empty once the failed runtime is cleaned up.
func (s *Service) midTurnFailoverAdmissible(ctx context.Context, sessionID, executionID string, generation int64) bool {
	if !s.midTurnFailovers.pending(sessionID, generation) {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	return err == nil && session != nil && session.RouteGeneration == generation &&
		(session.AgentExecutionID == "" || session.AgentExecutionID == executionID) &&
		session.State == models.TaskSessionStateWaitingForInput
}

// applyAutomaticRouteAction is ApplyRouteAction for an unattended caller: it
// takes the same per-session route-action lock and settled-turn check, and
// hands the request to the same backend route-action transaction. There is
// no caller identity to authorize, so the admit callback re-validates the
// trigger under the lock instead.
func (s *Service) applyAutomaticRouteAction(
	ctx context.Context,
	request RouteActionRequest,
	admit func(context.Context) bool,
) (*RouteActionResult, error) {
	if s.routeActionHandler == nil {
		return nil, errors.New("dynamic route actions are not configured")
	}
	releaseRouteAction := s.acquireRouteActionOperationLock(request.SessionID)
	defer releaseRouteAction()
	if err := s.rejectRouteActionDuringActiveTurn(ctx, request.SessionID); err != nil {
		return nil, err
	}
	if admit != nil && !admit(ctx) {
		return nil, errAutomaticRouteActionNotAdmissible
	}
	return s.routeActionHandler(withAutomaticRouteAction(ctx), request)
}

type automaticRouteActionKey struct{}

func withAutomaticRouteAction(ctx context.Context) context.Context {
	return context.WithValue(ctx, automaticRouteActionKey{}, true)
}

func automaticRouteActionFromContext(ctx context.Context) bool {
	automatic, _ := ctx.Value(automaticRouteActionKey{}).(bool)
	return automatic
}

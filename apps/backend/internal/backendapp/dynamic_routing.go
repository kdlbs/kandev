package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// dynamicRouteActionHandler owns the complete route action transaction:
// selection, durable attribution, successor launch, and the final recovery
// state when launch fails. The optional launcher keeps focused handler tests
// able to exercise the durable selection boundary without constructing the
// full orchestrator.
func dynamicRouteActionHandler(
	repo *sqliterepo.Repository,
	profileRepo settingsstore.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
	launchers ...func(context.Context, string) error,
) func(context.Context, orchestrator.RouteActionRequest) (*orchestrator.RouteActionResult, error) {
	var launchSuccessor func(context.Context, string) error
	if len(launchers) > 0 {
		launchSuccessor = launchers[0]
	}
	return func(ctx context.Context, request orchestrator.RouteActionRequest) (*orchestrator.RouteActionResult, error) {
		return applyDynamicRouteAction(ctx, repo, profileRepo, resolver, launchSuccessor, request)
	}
}

func applyDynamicRouteAction(
	ctx context.Context,
	repo *sqliterepo.Repository,
	profileRepo settingsstore.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
	launchSuccessor func(context.Context, string) error,
	request orchestrator.RouteActionRequest,
) (*orchestrator.RouteActionResult, error) {
	session, err := loadDynamicRouteActionSession(ctx, repo, profileRepo, resolver, request.SessionID)
	if err != nil {
		return nil, err
	}
	// expectedState is the session lifecycle state last confirmed under this
	// in-memory copy. The selection and error writes below are conditioned on
	// it, so a terminal transition (e.g. cancellation) that lands
	// concurrently with route selection is detected instead of clobbered by a
	// whole-row write built from this stale snapshot. The post-launch
	// recovery write is different: the launch attempt itself can mutate
	// state, so that write re-reads and gates on the state current at
	// recovery time instead (see recoverDynamicRouteAction).
	expectedState := session.State
	if err := repairDynamicRouteAfterLaunchFailure(
		ctx, session, request.ExpectedGeneration, resolver.MarkRouteRecoveryActionRequired,
	); err != nil {
		return nil, routeActionError(ctx, repo, session, err)
	}
	decision, err := resolveDynamicRouteAction(ctx, resolver, request, session)
	if err != nil {
		return handleDynamicRouteActionError(ctx, repo, session, expectedState, err)
	}
	changed, err := persistDynamicRouteSelection(ctx, repo, session, expectedState, decision)
	if err != nil {
		return nil, err
	}
	if !changed {
		return reloadRouteActionResult(ctx, repo, session.ID)
	}
	if request.Action == orchestrator.RouteActionCancelWait || request.Action == orchestrator.RouteActionStop {
		return routeActionResult(ctx, repo, session), nil
	}
	return finishDynamicRouteAction(ctx, repo, resolver, session.ID, decision.Generation, launchSuccessor)
}

func loadDynamicRouteActionSession(
	ctx context.Context,
	repo *sqliterepo.Repository,
	profileRepo settingsstore.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
	sessionID string,
) (*models.TaskSession, error) {
	if repo == nil || profileRepo == nil || resolver == nil {
		return nil, errors.New("dynamic route actions are not configured")
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	profile, err := profileRepo.GetAgentProfile(ctx, session.AgentProfileID)
	if err != nil {
		return nil, err
	}
	agent, err := profileRepo.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return nil, err
	}
	if agent.Name != agents.DynamicAgentID {
		return nil, errors.New("route actions require a dynamic profile")
	}
	return session, nil
}

func resolveDynamicRouteAction(
	ctx context.Context,
	resolver *agentruntime.ProfileExecutionResolver,
	request orchestrator.RouteActionRequest,
	session *models.TaskSession,
) (agentruntime.ProfileExecution, error) {
	return resolver.ResolveRouteAction(
		ctx,
		session.ID,
		session.AgentProfileID,
		session.ExecutionProfileID,
		request.ExpectedGeneration,
		string(request.Action),
	)
}

func handleDynamicRouteActionError(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	expectedState models.TaskSessionState,
	err error,
) (*orchestrator.RouteActionResult, error) {
	var noCandidate *dynamicruntime.NoEligibleCandidateError
	if !errors.As(err, &noCandidate) {
		return nil, routeActionError(ctx, repo, session, err)
	}
	session.RouteGeneration = noCandidate.Generation
	session.RouteState = "waiting"
	session.RouteReason = "no_eligible_candidate"
	session.DownstreamACPSessionID = ""
	changed, updateErr := repo.UpdateTaskSessionIfCurrentState(ctx, session, expectedState)
	if updateErr != nil {
		return nil, routeActionPersistenceError(ctx, repo, session, updateErr)
	}
	if !changed {
		return reloadRouteActionResult(ctx, repo, session.ID)
	}
	return routeActionResult(ctx, repo, session), nil
}

func persistDynamicRouteSelection(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	expectedState models.TaskSessionState,
	decision agentruntime.ProfileExecution,
) (bool, error) {
	session.ExecutionProfileID = decision.ExecutionProfileID
	session.RouteGeneration = decision.Generation
	session.RouteState = decision.Decision.Status
	if session.RouteState == "" {
		session.RouteState = "starting"
	}
	session.RouteReason = decision.Decision.Reason
	session.RouteErrorCode = string(decision.Decision.ErrorCode)
	session.RouteErrorClass = string(decision.Decision.ErrorClass)
	session.RouteCatalogueVersion = decision.Decision.CatalogueVersion
	session.RouteRetryOrdinal = decision.Decision.RetryOrdinal
	session.RoutePendingOutcome = string(decision.Decision.PendingOutcome)
	session.RouteDeadline = decision.Decision.Deadline
	// A candidate change never carries a provider-native ACP identity across
	// profiles. The conductor will populate this after a fresh launch.
	session.DownstreamACPSessionID = ""
	changed, err := repo.UpdateTaskSessionIfCurrentState(ctx, session, expectedState)
	if err != nil {
		return false, routeActionPersistenceError(ctx, repo, session, err)
	}
	return changed, nil
}

func finishDynamicRouteAction(
	ctx context.Context,
	repo *sqliterepo.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
	sessionID string,
	expectedGeneration int64,
	launchSuccessor func(context.Context, string) error,
) (*orchestrator.RouteActionResult, error) {
	if launchSuccessor == nil {
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return routeActionResult(ctx, repo, session), nil
	}
	if err := launchSuccessor(ctx, sessionID); err != nil {
		return recoverDynamicRouteAction(ctx, repo, resolver, sessionID, expectedGeneration, err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return routeActionResult(ctx, repo, session), nil
}

// recoverDynamicRouteAction records why a successor launch failed after a
// route action was accepted. The launcher itself may mutate session state
// before failing (a real launch failure resets the row to CREATED ahead of
// the launch attempt), so the write here is gated on a fresh reload rather
// than on any state captured before launchSuccessor ran: a stale pre-launch
// snapshot would never match the launcher's own mutation and this recovery
// write would be silently dropped. A reloaded state of CANCELLED means a
// concurrent cancellation has already terminated the session, and that must
// win over resurrecting it into WAITING_FOR_INPUT.
func recoverDynamicRouteAction(
	ctx context.Context,
	repo *sqliterepo.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
	sessionID string,
	expectedGeneration int64,
	launchErr error,
) (*orchestrator.RouteActionResult, error) {
	if resolver != nil {
		_ = resolver.MarkRouteRecoveryActionRequired(ctx, sessionID, expectedGeneration)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.RouteGeneration != expectedGeneration {
		return routeActionResult(ctx, repo, session), nil
	}
	if session.State == models.TaskSessionStateCancelled {
		return routeActionResult(ctx, repo, session), nil
	}
	reloadedState := session.State
	session.RouteState = "action_required"
	session.RouteReason = orchestrator.RouteActionLaunchFailedReason
	session.State = models.TaskSessionStateWaitingForInput
	session.ErrorMessage = launchErr.Error()
	session.DownstreamACPSessionID = ""
	changed, err := repo.UpdateTaskSessionIfCurrentState(ctx, session, reloadedState)
	if err != nil {
		return nil, routeActionPersistenceError(ctx, repo, session, err)
	}
	if !changed {
		return reloadRouteActionResult(ctx, repo, sessionID)
	}
	return routeActionResult(ctx, repo, session), nil
}

// reloadRouteActionResult reloads the session that a guarded write refused to
// overwrite (because its state moved since expectedState was captured) and
// reports that superseded/terminal state instead of the write the caller
// intended.
func reloadRouteActionResult(
	ctx context.Context,
	repo *sqliterepo.Repository,
	sessionID string,
) (*orchestrator.RouteActionResult, error) {
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return routeActionResult(ctx, repo, session), nil
}

func repairDynamicRouteAfterLaunchFailure(
	ctx context.Context,
	session *models.TaskSession,
	expectedGeneration int64,
	mark func(context.Context, string, int64) error,
) error {
	if session == nil || mark == nil ||
		session.RouteReason != orchestrator.RouteActionLaunchFailedReason ||
		session.RouteGeneration != expectedGeneration {
		return nil
	}
	if err := mark(ctx, session.ID, expectedGeneration); err != nil {
		return fmt.Errorf("restore dynamic route after failed launch: %w", err)
	}
	return nil
}

func routeActionResult(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
) *orchestrator.RouteActionResult {
	result := &orchestrator.RouteActionResult{
		SessionID:          session.ID,
		LogicalProfileID:   session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		RouteGeneration:    session.RouteGeneration,
		State:              session.RouteState,
		Reason:             session.RouteReason,
		ErrorCode:          session.RouteErrorCode,
		ErrorClass:         session.RouteErrorClass,
		CatalogueVersion:   session.RouteCatalogueVersion,
		RetryOrdinal:       session.RouteRetryOrdinal,
		Deadline:           session.RouteDeadline,
		PendingOutcome:     session.RoutePendingOutcome,
	}
	if repo != nil {
		if state, err := repo.LoadRouteState(ctx, session.ID); err == nil && state != nil {
			result.ProfileVersion = state.ProfileVersion
			var policyState dynamicruntime.PolicyState
			if jsonErr := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); jsonErr == nil {
				result.ErrorCode = string(policyState.FailureCode)
				result.ErrorClass = string(policyState.FailureClass)
				result.CatalogueVersion = policyState.CatalogueVersion
				result.RetryOrdinal = policyState.RetryOrdinal
				result.Deadline = policyState.Deadline
				result.PendingOutcome = string(policyState.PendingOutcome)
			}
		}
	}
	return result
}

func routeActionError(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	err error,
) error {
	if !errors.Is(err, dynamicruntime.ErrStaleGeneration) &&
		!errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		return err
	}
	return routeActionConflict(ctx, repo, session, err)
}

// routeActionPersistenceError reports the durable route state when the
// session projection cannot be updated after the engine has claimed a
// generation. The caller must not retry against the old session generation.
func routeActionPersistenceError(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	err error,
) error {
	return routeActionConflict(ctx, repo, session, fmt.Errorf("persist route action session state: %w", err))
}

func routeActionConflict(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	err error,
) error {
	state, loadErr := repo.LoadRouteState(ctx, session.ID)
	if loadErr != nil {
		return fmt.Errorf("%w; load authoritative route state: %v", err, loadErr)
	}
	result := &orchestrator.RouteActionResult{
		SessionID:          session.ID,
		LogicalProfileID:   session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		RouteGeneration:    session.RouteGeneration,
		State:              session.RouteState,
		ErrorCode:          session.RouteErrorCode,
		ErrorClass:         session.RouteErrorClass,
		CatalogueVersion:   session.RouteCatalogueVersion,
		RetryOrdinal:       session.RouteRetryOrdinal,
		Deadline:           session.RouteDeadline,
		PendingOutcome:     session.RoutePendingOutcome,
	}
	if state != nil {
		result.ExecutionProfileID = state.ExecutionProfileID
		result.RouteGeneration = state.Generation
		result.ProfileVersion = state.ProfileVersion
		result.State = state.Status
		result.LogicalProfileID = state.LogicalProfileID
		var policyState dynamicruntime.PolicyState
		if jsonErr := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); jsonErr == nil {
			result.ErrorCode = string(policyState.FailureCode)
			result.ErrorClass = string(policyState.FailureClass)
			result.CatalogueVersion = policyState.CatalogueVersion
			result.RetryOrdinal = policyState.RetryOrdinal
			result.Deadline = policyState.Deadline
			result.PendingOutcome = string(policyState.PendingOutcome)
		}
	}
	return &orchestrator.RouteActionConflictError{Result: result, Err: err}
}

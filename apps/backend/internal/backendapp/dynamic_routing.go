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
	decision, err := resolveDynamicRouteAction(ctx, resolver, request, session)
	if err != nil {
		return handleDynamicRouteActionError(ctx, repo, session, err)
	}
	if err := persistDynamicRouteSelection(ctx, repo, session, decision); err != nil {
		return nil, err
	}
	if request.Action == orchestrator.RouteActionCancelWait || request.Action == orchestrator.RouteActionStop {
		return routeActionResult(ctx, repo, session), nil
	}
	return finishDynamicRouteAction(ctx, repo, session.ID, launchSuccessor)
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
	if updateErr := repo.UpdateTaskSession(ctx, session); updateErr != nil {
		return nil, routeActionPersistenceError(ctx, repo, session, updateErr)
	}
	return routeActionResult(ctx, repo, session), nil
}

func persistDynamicRouteSelection(
	ctx context.Context,
	repo *sqliterepo.Repository,
	session *models.TaskSession,
	decision agentruntime.ProfileExecution,
) error {
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
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		return routeActionPersistenceError(ctx, repo, session, err)
	}
	return nil
}

func finishDynamicRouteAction(
	ctx context.Context,
	repo *sqliterepo.Repository,
	sessionID string,
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
		return recoverDynamicRouteAction(ctx, repo, sessionID, err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return routeActionResult(ctx, repo, session), nil
}

func recoverDynamicRouteAction(
	ctx context.Context,
	repo *sqliterepo.Repository,
	sessionID string,
	launchErr error,
) (*orchestrator.RouteActionResult, error) {
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	session.RouteState = "action_required"
	session.RouteReason = "route_action_launch_failed"
	session.State = models.TaskSessionStateWaitingForInput
	session.ErrorMessage = launchErr.Error()
	session.DownstreamACPSessionID = ""
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		return nil, routeActionPersistenceError(ctx, repo, session, err)
	}
	return routeActionResult(ctx, repo, session), nil
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
	if !errors.Is(err, dynamicruntime.ErrStaleGeneration) {
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

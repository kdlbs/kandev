package backendapp

import (
	"context"
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

// dynamicRouteActionHandler owns the small durable handoff around a route
// action. Downstream ACP launch/restart is performed by the conductor; this
// boundary claims the next generation and records the authoritative session
// attribution before any replacement launch can begin.
func dynamicRouteActionHandler(
	repo *sqliterepo.Repository,
	profileRepo settingsstore.Repository,
	resolver *agentruntime.ProfileExecutionResolver,
) func(context.Context, orchestrator.RouteActionRequest) (*orchestrator.RouteActionResult, error) {
	return func(ctx context.Context, request orchestrator.RouteActionRequest) (*orchestrator.RouteActionResult, error) {
		if repo == nil || profileRepo == nil || resolver == nil {
			return nil, errors.New("dynamic route actions are not configured")
		}
		session, err := repo.GetTaskSession(ctx, request.SessionID)
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

		exclude := ""
		if request.Action == orchestrator.RouteActionTryNext {
			exclude = session.ExecutionProfileID
		}
		decision, err := resolver.Resolve(ctx, session.ID, session.AgentProfileID, request.ExpectedGeneration, exclude)
		if err != nil {
			var noCandidate *dynamicruntime.NoEligibleCandidateError
			if errors.As(err, &noCandidate) {
				session.RouteGeneration = noCandidate.Generation
				session.RouteState = "waiting"
				session.RouteReason = "no_eligible_candidate"
				session.DownstreamACPSessionID = ""
				if updateErr := repo.UpdateTaskSession(ctx, session); updateErr != nil {
					return nil, updateErr
				}
				return &orchestrator.RouteActionResult{
					SessionID: session.ID, LogicalProfileID: session.AgentProfileID,
					RouteGeneration: noCandidate.Generation, State: session.RouteState,
					Reason: session.RouteReason,
				}, nil
			}
			return nil, routeActionError(ctx, repo, session, err)
		}
		session.ExecutionProfileID = decision.ExecutionProfileID
		session.RouteGeneration = decision.Generation
		session.RouteState = "starting"
		session.RouteReason = decision.Decision.Reason
		// A candidate change never carries a provider-native ACP identity across
		// profiles. The conductor will populate this after a fresh launch.
		session.DownstreamACPSessionID = ""
		if err := repo.UpdateTaskSession(ctx, session); err != nil {
			return nil, err
		}
		return &orchestrator.RouteActionResult{
			SessionID:          session.ID,
			LogicalProfileID:   session.AgentProfileID,
			ExecutionProfileID: session.ExecutionProfileID,
			RouteGeneration:    decision.Generation,
			ProfileVersion:     decision.ProfileVersion,
			State:              session.RouteState,
			Reason:             decision.Decision.Reason,
		}, nil
	}
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
	state, loadErr := repo.LoadRouteState(ctx, session.ID)
	if loadErr != nil {
		return fmt.Errorf("load authoritative route state: %w", loadErr)
	}
	result := &orchestrator.RouteActionResult{
		SessionID:          session.ID,
		LogicalProfileID:   session.AgentProfileID,
		ExecutionProfileID: session.ExecutionProfileID,
		RouteGeneration:    session.RouteGeneration,
		State:              session.RouteState,
	}
	if state != nil {
		result.ExecutionProfileID = state.ExecutionProfileID
		result.RouteGeneration = state.Generation
		result.ProfileVersion = state.ProfileVersion
		result.State = state.Status
	}
	return &orchestrator.RouteActionConflictError{Result: result, Err: err}
}

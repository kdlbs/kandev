package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/usage"
)

// ResolveExisting returns the persisted concrete execution for a logical
// session without advancing its route generation. It is used by resume paths
// after a restart, where selecting again would either fence a valid session
// or silently move it to another candidate.
func (r *ProfileExecutionResolver) ResolveExisting(
	ctx context.Context,
	sessionID, profileID, executionProfileID string,
	generation, profileVersion int64,
	reason string,
) (ProfileExecution, error) {
	if err := r.ValidateProfile(ctx, profileID); err != nil {
		return ProfileExecution{}, err
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if agent.Name != agents.DynamicAgentID {
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profileID, AgentName: agent.Name, Profile: profile}, nil
	}
	if executionProfileID == "" || generation <= 0 {
		return ProfileExecution{}, errors.New("dynamic session has no persisted execution profile")
	}
	if r.engine != nil && r.engine.Circuits().IsHeld(dynamic.ResourceKey(dynamic.ScopeAccountBlock, executionProfileID)) {
		return ProfileExecution{}, dynamic.ErrExecutionProfileHeld
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, executionProfileID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("resolve existing execution profile %s: %w", executionProfileID, err)
	}
	if concrete == nil || concrete.DeletedAt != nil || !concrete.Enabled {
		return ProfileExecution{}, fmt.Errorf("existing execution profile %s is unavailable", executionProfileID)
	}
	concreteAgentName, err := r.agentNameForProfile(ctx, concrete)
	if err != nil {
		return ProfileExecution{}, err
	}
	return ProfileExecution{
		LogicalProfileID: profileID, ExecutionProfileID: executionProfileID,
		AgentName:  concreteAgentName,
		Generation: generation, ProfileVersion: profileVersion, Profile: concrete,
		Decision: dynamic.RouteDecision{
			SessionID: sessionID, LogicalProfileID: profileID,
			ExecutionProfileID: executionProfileID, Generation: generation,
			ProfileVersion: profileVersion, Reason: reason,
			UsagePolicy: r.existingSelectionUsage(sessionID, executionProfileID, generation),
		},
	}, nil
}

func (r *ProfileExecutionResolver) existingSelectionUsage(sessionID, profileID string, generation int64) *usage.ExecutionUsagePolicy {
	if r.engine == nil {
		return nil
	}
	state, ok := r.engine.State(sessionID)
	if !ok || state.Generation != generation || state.ExecutionProfileID != profileID || state.UsagePolicy == nil || state.UsagePolicy.ProfileID != profileID {
		return nil
	}
	return usage.CloneExecutionUsagePolicy(state.UsagePolicy)
}

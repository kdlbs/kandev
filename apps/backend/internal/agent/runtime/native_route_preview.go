package runtime

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
)

// PreviewExecutionDetails checks reviewer eligibility without allocating a
// generation, native owner or provider probe before a real review run exists.
func (r *ProfileExecutionResolver) PreviewExecutionDetails(ctx context.Context, profileID string) (ProfileExecution, error) {
	if !r.NativeRouteOwnersEnabled() || r.profiles == nil || r.engine == nil {
		return ProfileExecution{}, ErrDynamicRoutingDisabled
	}
	profile, err := r.profiles.GetAgentProfile(ctx, profileID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if profile == nil {
		return ProfileExecution{}, fmt.Errorf("preview profile unavailable")
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return ProfileExecution{}, err
	}
	if agent == nil {
		return ProfileExecution{}, fmt.Errorf("preview profile family unavailable")
	}
	if agent.Name != agents.DynamicAgentID {
		if !executionCandidateAllowed(ctx, profile, agent.Name) {
			return ProfileExecution{}, ErrExecutionCandidateExcluded
		}
		return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: profile.ID, AgentName: agent.Name, Profile: profile}, nil
	}
	return r.previewDynamicReviewExecution(ctx, profileID)
}

func (r *ProfileExecutionResolver) previewDynamicReviewExecution(ctx context.Context, profileID string) (ProfileExecution, error) {
	config, err := r.loadDynamicProfile(ctx, profileID, "")
	if err != nil {
		return ProfileExecution{}, err
	}
	candidateID, err := r.engine.PreviewReviewerCandidate(ctx, config)
	if err != nil {
		return ProfileExecution{}, err
	}
	if candidateID == "" {
		return ProfileExecution{}, dynamic.ErrNoEligibleCandidate
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, candidateID)
	if err != nil {
		return ProfileExecution{}, fmt.Errorf("preview execution profile: %w", err)
	}
	if concrete == nil {
		return ProfileExecution{}, fmt.Errorf("preview execution profile is unavailable")
	}
	name, err := r.agentNameForProfile(ctx, concrete)
	if err != nil {
		return ProfileExecution{}, err
	}
	return ProfileExecution{LogicalProfileID: profileID, ExecutionProfileID: concrete.ID, AgentName: name, Profile: concrete}, nil
}

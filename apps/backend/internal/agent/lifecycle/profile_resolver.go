package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/settings/store"
)

// StoreProfileResolver implements ProfileResolver using the agent settings store
type StoreProfileResolver struct {
	store store.Repository
}

// NewStoreProfileResolver creates a new profile resolver using the given store
func NewStoreProfileResolver(store store.Repository) *StoreProfileResolver {
	return &StoreProfileResolver{store: store}
}

// ResolveProfile looks up an agent profile by ID and returns the profile info
func (r *StoreProfileResolver) ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	// Get the profile from the store
	profile, err := r.store.GetAgentProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %w", err)
	}

	// Get the parent agent to get the agent name
	agent, err := r.store.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found for profile: %w", err)
	}

	return &AgentProfileInfo{
		ProfileID:                  profile.ID,
		ProfileName:                profile.Name,
		AgentID:                    agent.ID,
		AgentName:                  agent.Name,
		Model:                      profile.Model,
		AutoApprove:                profile.AutoApprove,
		DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
		Plan:                       profile.Plan,
	}, nil
}

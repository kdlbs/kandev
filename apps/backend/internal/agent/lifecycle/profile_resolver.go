package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// StoreProfileResolver implements ProfileResolver using the agent settings store
type StoreProfileResolver struct {
	store    store.Repository
	registry *registry.Registry
}

// NewStoreProfileResolver creates a new profile resolver using the given store and registry.
// The registry is used to look up the agent's default model when the profile doesn't specify one.
func NewStoreProfileResolver(store store.Repository, reg *registry.Registry) *StoreProfileResolver {
	return &StoreProfileResolver{store: store, registry: reg}
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

	// Determine the model to use: profile's model, or fall back to agent's default model
	model := profile.Model
	if model == "" && r.registry != nil {
		// Look up the agent type in the registry to get the default model
		if agentType, ok := r.registry.Get(agent.Name); ok {
			model = agentType.ModelConfig.DefaultModel
		}
	}

	return &AgentProfileInfo{
		ProfileID:                  profile.ID,
		ProfileName:                profile.Name,
		AgentID:                    agent.ID,
		AgentName:                  agent.Name,
		Model:                      model,
		AutoApprove:                profile.AutoApprove,
		DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
		AllowIndexing:              profile.AllowIndexing,
		CLIPassthrough:             profile.CLIPassthrough,
		Plan:                       profile.Plan,
	}, nil
}

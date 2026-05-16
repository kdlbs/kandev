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

	// Resolve agent capabilities from the registry.
	model, nativeSessionResume := r.resolveAgentCapabilities(agent.Name, profile.Model)

	return &AgentProfileInfo{
		ProfileID:                  profile.ID,
		ProfileName:                profile.Name,
		AgentID:                    agent.ID,
		AgentName:                  agent.Name,
		Model:                      model,
		Mode:                       profile.Mode,
		AutoApprove:                profile.AutoApprove,
		DangerouslySkipPermissions: profile.DangerouslySkipPermissions,
		AllowIndexing:              profile.AllowIndexing,
		CLIFlags:                   profile.CLIFlags,
		CLIPassthrough:             profile.CLIPassthrough,
		NativeSessionResume:        nativeSessionResume,
		SupportsMCP:                agent.SupportsMCP,
	}, nil
}

// resolveAgentCapabilities looks up the agent in the registry and returns the
// effective model and whether the agent supports native session resume.
// The model comes straight from the profile; static per-agent defaults have
// been removed. Empty model means "agent picks its own default".
func (r *StoreProfileResolver) resolveAgentCapabilities(agentName, profileModel string) (string, bool) {
	if r.registry == nil {
		return profileModel, false
	}
	ag, ok := r.registry.Get(agentName)
	if !ok {
		return profileModel, false
	}
	var nativeSessionResume bool
	if rt := ag.Runtime(); rt != nil {
		nativeSessionResume = rt.SessionConfig.NativeSessionResume
	}
	return profileModel, nativeSessionResume
}

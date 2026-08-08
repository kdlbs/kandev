package profilebinding

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

var (
	ErrProfileNotFound        = errors.New("agent profile not found")
	ErrProfileIneligible      = errors.New("agent profile is not eligible for utility execution")
	ErrLegacyBindingAmbiguous = errors.New("legacy utility binding does not match exactly one profile")
)

type profileRepository interface {
	GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error)
	ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error)
	ListAgents(ctx context.Context) ([]*models.Agent, error)
	GetAgent(ctx context.Context, id string) (*models.Agent, error)
}

// Resolver is the single utility-agent profile eligibility boundary.
type Resolver struct {
	profiles    profileRepository
	isInference func(agentID string) bool
}

// LegacyMatcher resolves an old agent/model pair to one eligible profile.
// It is intentionally separate from Resolve so migrations can distinguish
// an ambiguous or unavailable legacy value without changing saved bindings.
type LegacyMatcher interface {
	MatchLegacy(ctx context.Context, agentID, model string) (*models.AgentProfile, error)
}

func New(profiles profileRepository, isInference func(agentID string) bool) *Resolver {
	return &Resolver{profiles: profiles, isInference: isInference}
}

func (r *Resolver) Resolve(ctx context.Context, id string) (*models.AgentProfile, error) {
	profile, err := r.profiles.GetAgentProfileIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrProfileNotFound
	}
	agent, err := r.profiles.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return nil, err
	}
	if agent == nil || !eligible(profile, r.isInference, agent.Name) {
		return nil, ErrProfileIneligible
	}
	resolved := *profile
	resolved.AgentID = agent.Name
	return &resolved, nil
}

//nolint:cyclop // legacy matching has independent eligibility and ambiguity checks.
func (r *Resolver) MatchLegacy(ctx context.Context, agentID, model string) (*models.AgentProfile, error) {
	matches := make([]*models.AgentProfile, 0, 1)
	agents, err := r.profiles.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		if agent == nil || (agent.ID != agentID && agent.Name != agentID) {
			continue
		}
		profiles, err := r.profiles.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile != nil && profile.Model == model && eligible(profile, r.isInference, agent.Name) {
				matches = append(matches, profile)
			}
		}
	}
	if len(matches) != 1 {
		return nil, ErrLegacyBindingAmbiguous
	}
	resolved := *matches[0]
	for _, agent := range agents {
		if agent != nil && (agent.ID == resolved.AgentID || agent.Name == agentID) {
			resolved.AgentID = agent.Name
			break
		}
	}
	return &resolved, nil
}

func eligible(profile *models.AgentProfile, isInference func(string) bool, agentID string) bool {
	return profile.DeletedAt == nil && profile.Enabled && profile.WorkspaceID == "" &&
		!profile.CLIPassthrough && isInference != nil && isInference(agentID)
}

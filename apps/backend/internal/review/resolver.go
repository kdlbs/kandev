package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ReviewerIdentity is the (agent CLI, model, mode) triple a review pass runs as.
// It is deliberately separate from the task's own agent profile: the point of the
// feature is that a different model can review than implemented.
type ReviewerIdentity struct {
	AgentID string
	Model   string
	Mode    string
	// Source names where the identity came from, for the run record and logs:
	// "agent_profile", "utility_agent", or "user_default".
	Source string
}

// Identity sources.
const (
	SourceAgentProfile = "agent_profile"
	SourceUtilityAgent = "utility_agent"
	SourceUserDefault  = "user_default"
)

// ReviewerProfile is the subset of an agent profile the resolver needs.
type ReviewerProfile struct {
	AgentID        string
	Model          string
	Mode           string
	CLIPassthrough bool
	Name           string
}

// ProfileLookup resolves an agent profile ID. Returns found=false for an unknown
// profile.
type ProfileLookup interface {
	ReviewerProfile(ctx context.Context, profileID string) (profile ReviewerProfile, found bool, err error)
}

// UtilityAgentLookup resolves the built-in `code-review` utility agent, which
// supplies the default reviewer identity for the on-demand path.
type UtilityAgentLookup interface {
	// CodeReviewAgent returns the code-review utility agent's agent/model and
	// whether it is enabled. found=false means the row is missing entirely.
	CodeReviewAgent(ctx context.Context) (agentID, model string, enabled, found bool, err error)
}

// DefaultsLookup resolves the user's default utility agent/model pair, used when
// the code-review utility agent leaves the model blank.
type DefaultsLookup interface {
	DefaultUtilitySettings(ctx context.Context) (agentID, model string, err error)
}

// Resolver picks the reviewer identity for a run.
type Resolver struct {
	Profiles ProfileLookup
	Utility  UtilityAgentLookup
	Defaults DefaultsLookup
}

// NewResolver builds a resolver. Any lookup may be nil; a nil lookup simply
// contributes no candidate identity.
func NewResolver(profiles ProfileLookup, utility UtilityAgentLookup, defaults DefaultsLookup) *Resolver {
	return &Resolver{Profiles: profiles, Utility: utility, Defaults: defaults}
}

// Resolve returns the reviewer identity for a run.
//
// Precedence:
//  1. an explicit agent profile (the workflow-step path), which is what makes a
//     per-step reviewing model possible;
//  2. the enabled built-in `code-review` utility agent;
//  3. the user's default utility agent/model.
//
// The agent/model pair is treated as inseparable: a default model from one
// provider must never be sent to another provider's ACP config, so a blank model
// pulls both halves from the defaults rather than just the model.
//
// Fails closed with ErrAgentUnavailable — never guesses an agent — because a
// wrong provider/model pair produces either a hard provider error or, worse, a
// confident review from a model the user did not choose.
func (r *Resolver) Resolve(ctx context.Context, agentProfileID string) (ReviewerIdentity, error) {
	if strings.TrimSpace(agentProfileID) != "" {
		return r.resolveFromProfile(ctx, strings.TrimSpace(agentProfileID))
	}
	identity, err := r.resolveFromUtilityAgent(ctx)
	if err == nil {
		return identity, nil
	}
	// Only an absent or disabled code-review agent defers to the user defaults.
	// A configured-but-inconsistent one is a hard failure: falling back there
	// would quietly review with a provider/model pair the user never chose.
	if !errors.Is(err, errDeferToDefaults) {
		return ReviewerIdentity{}, err
	}
	return r.resolveFromDefaults(ctx)
}

// errDeferToDefaults marks the "code-review utility agent is absent or turned
// off" case, where falling back to the user's default agent/model is correct.
var errDeferToDefaults = errors.New("defer to default utility settings")

func (r *Resolver) resolveFromProfile(ctx context.Context, profileID string) (ReviewerIdentity, error) {
	if r.Profiles == nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: agent profiles are not available", ErrAgentUnavailable)
	}
	profile, found, err := r.Profiles.ReviewerProfile(ctx, profileID)
	if err != nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: load agent profile %s: %v", ErrAgentUnavailable, profileID, err)
	}
	if !found {
		return ReviewerIdentity{}, fmt.Errorf("%w: agent profile %s no longer exists", ErrAgentUnavailable, profileID)
	}
	if profile.CLIPassthrough {
		return ReviewerIdentity{}, fmt.Errorf(
			"%w: agent profile %q is CLI-passthrough only and cannot run a structured review",
			ErrAgentUnavailable, describeProfile(profile, profileID))
	}
	if profile.AgentID == "" {
		return ReviewerIdentity{}, fmt.Errorf("%w: agent profile %q has no agent configured",
			ErrAgentUnavailable, describeProfile(profile, profileID))
	}
	model := profile.Model
	if model == "" {
		// The profile names an agent but no model; borrow the user's default
		// model only when it belongs to the same agent.
		model = r.sameAgentDefaultModel(ctx, profile.AgentID)
	}
	if model == "" {
		return ReviewerIdentity{}, fmt.Errorf("%w: agent profile %q has no model configured",
			ErrAgentUnavailable, describeProfile(profile, profileID))
	}
	return ReviewerIdentity{
		AgentID: profile.AgentID,
		Model:   model,
		Mode:    profile.Mode,
		Source:  SourceAgentProfile,
	}, nil
}

func describeProfile(profile ReviewerProfile, profileID string) string {
	if profile.Name != "" {
		return profile.Name
	}
	return profileID
}

// sameAgentDefaultModel returns the user's default model only when the default
// belongs to agentID, so a model is never crossed between providers.
func (r *Resolver) sameAgentDefaultModel(ctx context.Context, agentID string) string {
	if r.Defaults == nil {
		return ""
	}
	defaultAgent, defaultModel, err := r.Defaults.DefaultUtilitySettings(ctx)
	if err != nil || defaultAgent != agentID {
		return ""
	}
	return defaultModel
}

func (r *Resolver) resolveFromUtilityAgent(ctx context.Context) (ReviewerIdentity, error) {
	if r.Utility == nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: %w: no code-review utility agent is wired",
			ErrAgentUnavailable, errDeferToDefaults)
	}
	agentID, model, enabled, found, err := r.Utility.CodeReviewAgent(ctx)
	if err != nil {
		// A lookup failure is a real fault, not an unconfigured feature; do not
		// mask it by silently reviewing with the defaults.
		return ReviewerIdentity{}, fmt.Errorf("%w: load code-review utility agent: %v", ErrAgentUnavailable, err)
	}
	if !found {
		return ReviewerIdentity{}, fmt.Errorf("%w: %w: the code-review utility agent is missing",
			ErrAgentUnavailable, errDeferToDefaults)
	}
	if !enabled {
		return ReviewerIdentity{}, fmt.Errorf("%w: %w: the code-review utility agent is disabled",
			ErrAgentUnavailable, errDeferToDefaults)
	}
	// A configured utility agent with a blank model means "use my defaults";
	// take the agent and model together so the pair stays consistent.
	if model == "" {
		defaults, defErr := r.resolveFromDefaults(ctx)
		if defErr != nil {
			return ReviewerIdentity{}, defErr
		}
		if agentID == "" {
			agentID = defaults.AgentID
		}
		if agentID != defaults.AgentID {
			return ReviewerIdentity{}, fmt.Errorf(
				"%w: the code-review utility agent uses %s but the default model belongs to %s; set a model on the code-review agent in Settings > Utility Agents",
				ErrAgentUnavailable, agentID, defaults.AgentID)
		}
		return ReviewerIdentity{AgentID: agentID, Model: defaults.Model, Source: SourceUtilityAgent}, nil
	}
	if agentID == "" {
		return ReviewerIdentity{}, fmt.Errorf("%w: the code-review utility agent has no agent configured", ErrAgentUnavailable)
	}
	return ReviewerIdentity{AgentID: agentID, Model: model, Source: SourceUtilityAgent}, nil
}

func (r *Resolver) resolveFromDefaults(ctx context.Context) (ReviewerIdentity, error) {
	if r.Defaults == nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: no default utility agent is configured", ErrAgentUnavailable)
	}
	agentID, model, err := r.Defaults.DefaultUtilitySettings(ctx)
	if err != nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: load default utility settings: %v", ErrAgentUnavailable, err)
	}
	if agentID == "" || model == "" {
		return ReviewerIdentity{}, fmt.Errorf(
			"%w: no inference-capable agent and model are configured; set one in Settings > Utility Agents",
			ErrAgentUnavailable)
	}
	return ReviewerIdentity{AgentID: agentID, Model: model, Source: SourceUserDefault}, nil
}

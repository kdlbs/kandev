package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
)

var (
	// ErrNoDistinctReviewer means no configured reviewer is independent of the writer.
	ErrNoDistinctReviewer = errors.New("reviewer would be the origin's own model/profile")
	// ErrOriginUnreadable means the actual writer model cannot be established.
	ErrOriginUnreadable = errors.New("the origin execution identity is unavailable")
)

// OriginIdentity is supplied by the trusted writer/session boundary, never by
// source metadata. Actual snapshot values retain the model that wrote the diff
// even if the profile's configuration has since changed.
type OriginIdentity struct {
	ProfileID          string
	ExecutionProfileID string
	SessionID          string
	AgentID            string
	Model              string
}

// OriginFromSession captures only the trusted session's writer identity.
func OriginFromSession(session *models.TaskSession) *OriginIdentity {
	if session == nil || (session.AgentProfileID == "" && session.ExecutionProfileID == "") {
		return nil
	}
	origin := &OriginIdentity{ProfileID: session.AgentProfileID, ExecutionProfileID: session.ExecutionProfileID, SessionID: session.ID}
	actualID := session.ExecutionProfileID
	if actualID == "" {
		actualID = session.AgentProfileID
	}
	if models.StringFromAny(session.AgentProfileSnapshot["id"]) == actualID {
		origin.AgentID = models.StringFromAny(session.AgentProfileSnapshot["agent_name"])
		origin.Model = models.StringFromAny(session.AgentProfileSnapshot["model"])
	}
	return origin
}

type originExclusion struct {
	profileID, executionProfileID string
	agentID, model                string
	defaults                      DefaultsLookup
	defaultOnce                   sync.Once
	defaultAgent, defaultModel    string
}

type originExclusionKey struct{}

type originProfileLookup interface {
	ReviewerOriginProfile(context.Context, string) (ReviewerProfile, bool, error)
}

func (r *Resolver) originExclusion(ctx context.Context, origin *OriginIdentity, required bool) (*originExclusion, error) {
	if origin == nil {
		if required {
			return nil, fmt.Errorf("%w: %w", ErrAgentUnavailable, ErrOriginUnreadable)
		}
		return nil, nil
	}
	ex := &originExclusion{profileID: origin.ProfileID, executionProfileID: origin.ExecutionProfileID, agentID: origin.AgentID, model: origin.Model, defaults: r.Defaults}
	if ex.agentID != "" && ex.model != "" {
		return ex, nil
	}
	id := origin.ExecutionProfileID
	if id == "" {
		id = origin.ProfileID
	}
	if r.Profiles == nil || id == "" {
		return nil, fmt.Errorf("%w: %w", ErrAgentUnavailable, ErrOriginUnreadable)
	}
	lookup := r.Profiles.ReviewerProfile
	if raw, ok := r.Profiles.(originProfileLookup); ok {
		lookup = raw.ReviewerOriginProfile
	}
	profile, found, err := lookup(ctx, id)
	if err != nil || !found || profile.AgentID == "" || profile.Model == "" {
		return nil, errors.Join(fmt.Errorf("%w: %w", ErrAgentUnavailable, ErrOriginUnreadable), err)
	}
	ex.agentID, ex.model = profile.AgentID, profile.Model
	return ex, nil
}

// OriginExclusionActive reports whether this request must exclude its writer.
func OriginExclusionActive(ctx context.Context) bool {
	_, ok := ctx.Value(originExclusionKey{}).(*originExclusion)
	return ok
}

// ReviewerCandidateAllowed compares the actual candidate with the captured writer.
func ReviewerCandidateAllowed(ctx context.Context, profileID, agentID, model string) bool {
	ex, _ := ctx.Value(originExclusionKey{}).(*originExclusion)
	if ex == nil {
		return true
	}
	if profileID != "" && (profileID == ex.profileID || profileID == ex.executionProfileID) {
		return false
	}
	model, known := ReviewerEffectiveModel(ctx, agentID, model)
	return agentID != "" && known && (agentID != ex.agentID || model != ex.model)
}

// ReviewerEffectiveModel applies the existing same-agent default without
// guessing a model for another provider or for an unreadable setting.
func ReviewerEffectiveModel(ctx context.Context, agentID, model string) (string, bool) {
	if model != "" {
		return model, true
	}
	ex, _ := ctx.Value(originExclusionKey{}).(*originExclusion)
	if ex == nil || ex.defaults == nil || agentID == "" {
		return "", false
	}
	ex.defaultOnce.Do(func() {
		agent, selected, err := ex.defaults.DefaultUtilitySettings(ctx)
		if err == nil {
			ex.defaultAgent, ex.defaultModel = agent, selected
		}
	})
	if ex.defaultAgent != agentID || ex.defaultModel == "" {
		return "", false
	}
	return ex.defaultModel, true
}

func reviewerIdentityAllowed(ctx context.Context, identity ReviewerIdentity) bool {
	profileID, agentID, model := identity.ProfileID, identity.AgentID, identity.Model
	if route := identity.Route; route != nil {
		profileID, agentID, model = route.ExecutionProfileID, route.AgentID, route.Model
	}
	return ReviewerCandidateAllowed(ctx, profileID, agentID, model)
}

// ResolveExcluding preserves configured precedence while excluding the writer.
func (r *Resolver) ResolveExcluding(ctx context.Context, profileID string, origin *OriginIdentity) (ReviewerIdentity, error) {
	ex, err := r.originExclusion(ctx, origin, false)
	if err != nil {
		return ReviewerIdentity{}, err
	}
	return r.resolveDistinct(withOriginExclusion(ctx, ex), profileID)
}

// ResolveRequiredOrigin fails closed if the caller cannot identify the writer.
func (r *Resolver) ResolveRequiredOrigin(ctx context.Context, profileID string, origin *OriginIdentity) (ReviewerIdentity, error) {
	if origin == nil {
		return ReviewerIdentity{}, fmt.Errorf("%w: %w", ErrAgentUnavailable, ErrOriginUnreadable)
	}
	return r.ResolveExcluding(ctx, profileID, origin)
}

func (r *Resolver) resolveDistinct(ctx context.Context, profileID string) (ReviewerIdentity, error) {
	if profileID = strings.TrimSpace(profileID); profileID != "" {
		identity, err := r.resolveFromProfile(ctx, profileID)
		if err == nil && reviewerIdentityAllowed(ctx, identity) {
			return identity, nil
		}
		if err != nil && !errors.Is(err, ErrNoDistinctReviewer) {
			return ReviewerIdentity{}, err
		}
	}
	identity, err := r.resolveFromUtilityAgent(ctx)
	if err == nil && reviewerIdentityAllowed(ctx, identity) {
		return identity, nil
	}
	if err != nil && !errors.Is(err, errDeferToDefaults) && !errors.Is(err, ErrNoDistinctReviewer) {
		return ReviewerIdentity{}, err
	}
	identity, err = r.resolveFromDefaults(ctx)
	if err != nil {
		return ReviewerIdentity{}, err
	}
	if !reviewerIdentityAllowed(ctx, identity) {
		return ReviewerIdentity{}, fmt.Errorf("%w: %w", ErrAgentUnavailable, ErrNoDistinctReviewer)
	}
	return identity, nil
}

func (r *Runner) prepareOriginContext(ctx context.Context, req *RunRequest) (context.Context, error) {
	ctx, err := r.resolver.WithOriginExclusion(ctx, req.Origin, req.OriginRequired)
	if err != nil {
		return ctx, err
	}
	req.originExclusion, _ = ctx.Value(originExclusionKey{}).(*originExclusion)
	return ctx, nil
}

// WithOriginExclusion returns ctx carrying the writer exclusion for origin.
// A nil origin leaves ctx unchanged unless required, which fails closed.
func (r *Resolver) WithOriginExclusion(ctx context.Context, origin *OriginIdentity, required bool) (context.Context, error) {
	ex, err := r.originExclusion(ctx, origin, required)
	if err != nil {
		return ctx, err
	}
	return withOriginExclusion(ctx, ex), nil
}

func withOriginExclusion(ctx context.Context, ex *originExclusion) context.Context {
	if ex == nil {
		return ctx
	}
	return context.WithValue(ctx, originExclusionKey{}, ex)
}

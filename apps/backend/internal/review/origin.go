package review

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	ex := &originExclusion{profileID: origin.ProfileID, executionProfileID: origin.ExecutionProfileID, agentID: origin.AgentID, model: origin.Model}
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
	return agentID != "" && model != "" && (agentID != ex.agentID || model != ex.model)
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
	ex, err := r.resolver.originExclusion(ctx, req.Origin, req.OriginRequired)
	if err != nil {
		return ctx, err
	}
	req.originExclusion = ex
	return withOriginExclusion(ctx, ex), nil
}

func withOriginExclusion(ctx context.Context, ex *originExclusion) context.Context {
	if ex == nil {
		return ctx
	}
	return context.WithValue(ctx, originExclusionKey{}, ex)
}

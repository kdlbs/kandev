package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// resourceCircuitCandidate builds the resource identity of one concrete
// (fixed) execution profile, using the same circuit key a dynamic selection
// would use for it: the shared credential fingerprint when a binding resolver
// is configured, otherwise the profile's own key.
func (r *ProfileExecutionResolver) resourceCircuitCandidate(
	ctx context.Context,
	executionProfileID string,
) (dynamic.Candidate, bool) {
	if r == nil || !r.Enabled() || r.engine == nil || r.profiles == nil || executionProfileID == "" {
		return dynamic.Candidate{}, false
	}
	profile, err := r.profiles.GetAgentProfile(ctx, executionProfileID)
	if err != nil || profile == nil || profile.DeletedAt != nil {
		return dynamic.Candidate{}, false
	}
	return dynamic.Candidate{ID: executionProfileID, Enabled: true, BindingKey: r.ResourceCircuitKeyForProfile(profile)}, true
}

// ResourceCircuitKeyForProfile uses the supplied immutable launch profile,
// never a later settings read, to name its shared resource.
func (r *ProfileExecutionResolver) ResourceCircuitKeyForProfile(profile *settingsmodels.AgentProfile) string {
	if r == nil || profile == nil || profile.ID == "" {
		return ""
	}
	if r.bindingResolver != nil {
		return dynamic.ResourceKey(
			dynamic.ScopeCredential,
			r.bindingResolver.Resolve(profileCredentialBindingDescriptor(profile), profile.ID),
		)
	}
	return dynamic.ResourceKey(dynamic.ScopeProfile, profile.ID)
}

// RecordFixedProfileFailure charges the immutable resource captured by the
// execution. An unknown resource cannot charge a currently configured secret.
func (r *ProfileExecutionResolver) RecordFixedProfileFailure(
	_ context.Context,
	executionProfileID, resourceKey string,
	failure *routingerr.Error,
) {
	if failure == nil {
		return
	}
	if r == nil || !r.Enabled() || r.engine == nil || executionProfileID == "" {
		return
	}
	if resourceKey == "" && !routingerr.HoldsUntilRelease(failure.Code) {
		return
	}
	candidate := dynamic.Candidate{ID: executionProfileID, Enabled: true, BindingKey: resourceKey}
	r.engine.RecordResourceFailure(
		dynamic.Profile{ID: executionProfileID, Candidates: []dynamic.Candidate{candidate}},
		candidate.ID, failure,
	)
}

// ResetFixedProfileCircuitFailures clears only the consecutive count of a
// concrete profile's shared resource after its owner confirms a clean turn.
// It does not close a circuit, release a hold, or change a probe lease.
func (r *ProfileExecutionResolver) ResetFixedProfileCircuitFailures(_ context.Context, resourceKey string) bool {
	if r == nil || !r.Enabled() || r.engine == nil || !r.engine.Circuits().EscalationEnabled() {
		return false
	}
	if resourceKey == "" {
		return false
	}
	return r.engine.Circuits().ResetFailures(resourceKey)
}

// FixedProfileSuspension reports the resource circuit refusing a new dispatch
// for one fixed profile at now. A profile that cannot be resolved reports an
// unknown verdict instead of a healthy one; a resolvable profile with no
// circuit reports known and unblocked.
func (r *ProfileExecutionResolver) FixedProfileSuspension(
	ctx context.Context,
	executionProfileID string,
	now time.Time,
) (dynamic.Suspension, bool) {
	candidate, ok := r.resourceCircuitCandidate(ctx, executionProfileID)
	if !ok {
		return dynamic.Suspension{}, false
	}
	suspension, blocked := r.engine.CandidateSuspension(candidate, now)
	if !blocked {
		return dynamic.Suspension{}, true
	}
	return suspension, true
}

// CapturedFixedProfileSuspension checks the resource of a retained execution.
// Unknown legacy resource identity must not be replaced with today's secret.
func (r *ProfileExecutionResolver) CapturedFixedProfileSuspension(executionProfileID, resourceKey string, now time.Time) (dynamic.Suspension, bool) {
	if r == nil || !r.Enabled() || r.engine == nil || executionProfileID == "" {
		return dynamic.Suspension{}, false
	}
	suspension, blocked := r.engine.CandidateSuspension(dynamic.Candidate{
		ID: executionProfileID, Enabled: true, BindingKey: resourceKey,
	}, now)
	return suspension, blocked || resourceKey != ""
}

package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// resourceCircuitCandidate builds the resource identity of one concrete
// (fixed) execution profile, using the same circuit key a dynamic selection
// would use for it: the shared credential fingerprint when a binding resolver
// is configured, otherwise the profile's own key.
func (r *ProfileExecutionResolver) resourceCircuitCandidate(
	ctx context.Context,
	executionProfileID string,
) (dynamic.Candidate, bool) {
	if r == nil || r.engine == nil || r.profiles == nil || executionProfileID == "" {
		return dynamic.Candidate{}, false
	}
	profile, err := r.profiles.GetAgentProfile(ctx, executionProfileID)
	if err != nil || profile == nil || profile.DeletedAt != nil {
		return dynamic.Candidate{}, false
	}
	candidate := dynamic.Candidate{
		ID: executionProfileID, Enabled: true,
		BindingKey: dynamic.ResourceKey(dynamic.ScopeProfile, executionProfileID),
	}
	if r.bindingResolver != nil {
		candidate.BindingKey = dynamic.ResourceKey(
			dynamic.ScopeCredential,
			r.bindingResolver.Resolve(profileCredentialBindingDescriptor(profile), executionProfileID),
		)
	}
	return candidate, true
}

// RecordFixedProfileFailure records a fixed profile's classified failure
// against its resource circuit. A profile that cannot be resolved records
// nothing.
func (r *ProfileExecutionResolver) RecordFixedProfileFailure(
	ctx context.Context,
	executionProfileID string,
	failure *routingerr.Error,
) {
	if failure == nil {
		return
	}
	candidate, ok := r.resourceCircuitCandidate(ctx, executionProfileID)
	if !ok {
		return
	}
	r.engine.RecordResourceFailure(
		dynamic.Profile{ID: executionProfileID, Candidates: []dynamic.Candidate{candidate}},
		candidate.ID, failure,
	)
}

// ResetFixedProfileCircuitFailures clears only the consecutive count of a
// concrete profile's shared resource after its owner confirms a clean turn.
// It does not close a circuit, release a hold, or change a probe lease.
func (r *ProfileExecutionResolver) ResetFixedProfileCircuitFailures(ctx context.Context, executionProfileID string) bool {
	if r == nil || r.engine == nil || !r.engine.Circuits().EscalationEnabled() {
		return false
	}
	candidate, ok := r.resourceCircuitCandidate(ctx, executionProfileID)
	if !ok {
		return false
	}
	return r.engine.Circuits().ResetFailures(candidate.BindingKey)
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

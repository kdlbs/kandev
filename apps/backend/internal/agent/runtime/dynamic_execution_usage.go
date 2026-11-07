package runtime

import (
	"context"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/usage"
	"time"
)

// CandidateExecutionUsageReader must use the original session's actual actor,
// never the backend account or another profile's representative host cache.
type CandidateExecutionUsageReader func(context.Context, string, *models.AgentProfile) *usage.ExecutionUsageReport
type NativeCandidateExecutionUsageReader func(context.Context, string, string, *models.AgentProfile) *usage.ExecutionUsageReport

func (r *ProfileExecutionResolver) SetNativeCandidateExecutionUsageReader(reader NativeCandidateExecutionUsageReader) {
	r.nativeCandidateUsageReader = reader
}

// Installed before service start. Nil preserves the disabled legacy path.
func (r *ProfileExecutionResolver) SetCandidateExecutionUsageReader(reader CandidateExecutionUsageReader) {
	r.candidateUsageReader = reader
}

func (r *ProfileExecutionResolver) LoadDynamicProfileForSession(ctx context.Context, profileID, sessionID string) (dynamic.Profile, error) {
	return r.loadSelectionProfile(ctx, profileID, sessionID)
}

func (r *ProfileExecutionResolver) loadSelectionProfile(ctx context.Context, profileID, sessionID string) (dynamic.Profile, error) {
	profile, err := r.loadDynamicProfile(ctx, profileID)
	if err != nil || sessionID == "" {
		return profile, err
	}
	if !r.engine.OrderedTiersEnabled() && r.candidateUsageReader == nil && r.nativeCandidateUsageReader == nil {
		return profile, err
	}
	// This runs before Engine takes its selection mutex. The production reader
	// only peeks existing observations and cannot create an actor or provider call.
	for index := range profile.Candidates {
		r.applyCandidateUsage(ctx, &profile.Candidates[index], profileID, sessionID)
	}
	return profile, nil
}

func (r *ProfileExecutionResolver) applyCandidateUsage(ctx context.Context, candidate *dynamic.Candidate, profileID, sessionID string) {
	if !candidate.Enabled {
		return
	}
	concrete, err := r.profiles.GetAgentProfile(ctx, candidate.ID)
	if err != nil || concrete == nil {
		return
	}
	report, scoped := usage.ReadCandidateUsage(ctx, concrete.ID, concrete.Model)
	if !scoped {
		switch {
		case nativeInferenceRoute(sessionID) && r.nativeCandidateUsageReader != nil:
			report = r.nativeCandidateUsageReader(ctx, sessionID, profileID, concrete)
		case r.candidateUsageReader != nil:
			report = r.candidateUsageReader(ctx, sessionID, concrete)
		default:
			return
		}
	}
	model := concrete.Model
	// A request-scoped producer may use a persisted session model override.
	if scoped && report != nil && report.Model != "" {
		model = report.Model
	}
	candidate.UsagePolicy = usage.SelectionPolicyForReport(concrete.ID, model, report)
	candidate.Usage = dynamic.CandidateUsageFromReport(report, model, time.Now())
}

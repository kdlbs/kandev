package usage

import (
	"context"
	"sync"
)

// CandidateUsageReader must report the actual prepared model, including any
// session runtime override; a profile default is not an execution observation.
type CandidateUsageReader func(context.Context, string) *ExecutionUsageReport
type candidateUsageContextKey struct{}
type candidateUsageContext struct {
	reader   CandidateUsageReader
	mu       sync.Mutex
	selected map[string]ExecutionUsagePolicy
}

// This request-local projection uses original profile references only. It is
// neither a shared account registry nor an allowance/claim/usage counter.
func WithCandidateUsageReader(ctx context.Context, reader CandidateUsageReader) context.Context {
	return context.WithValue(ctx, candidateUsageContextKey{}, &candidateUsageContext{reader: reader, selected: make(map[string]ExecutionUsagePolicy)})
}

func ReadCandidateUsage(ctx context.Context, profileID, model string) (*ExecutionUsageReport, bool) {
	state, ok := ctx.Value(candidateUsageContextKey{}).(*candidateUsageContext)
	if !ok || state.reader == nil {
		return nil, false
	}
	report := state.reader(ctx, profileID)
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.selected, profileID)
	if report != nil && report.SourceKnown && !report.BindingChanged && report.Current != nil && report.Model != "" {
		sample := report.Current
		state.selected[profileID] = ExecutionUsagePolicy{ExpectedProvider: sample.Provider, ExpectedAccountRef: sample.AccountRef, ExpectedOrganizationRef: sample.OrganizationRef, ExpectedModel: report.Model}
	}
	return report, true
}

func ApplySelectedUsage(ctx context.Context, policy *ExecutionUsagePolicy) {
	state, ok := ctx.Value(candidateUsageContextKey{}).(*candidateUsageContext)
	if !ok || policy == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	selected, ok := state.selected[policy.ProfileID]
	if !ok {
		return
	}
	policy.ExpectedProvider = selected.ExpectedProvider
	policy.ExpectedAccountRef = selected.ExpectedAccountRef
	policy.ExpectedOrganizationRef = selected.ExpectedOrganizationRef
	policy.ExpectedModel = selected.ExpectedModel
}

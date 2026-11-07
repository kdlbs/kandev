package usage

import "context"

func CloneExecutionUsagePolicy(policy *ExecutionUsagePolicy) *ExecutionUsagePolicy {
	if policy == nil {
		return nil
	}
	copy := *policy
	return &copy
}

// This contains existing provider references, not a new identity or authority.
func SelectionPolicyForReport(profileID, model string, report *ExecutionUsageReport) *ExecutionUsagePolicy {
	if report == nil || !report.SourceKnown || report.BindingChanged || report.Current == nil || model == "" || report.Model != model {
		return nil
	}
	sample := report.Current
	if sample.Provider == "" || sample.AccountRef == "" {
		return nil
	}
	return &ExecutionUsagePolicy{ProfileID: profileID, ExpectedProvider: sample.Provider, ExpectedAccountRef: sample.AccountRef, ExpectedOrganizationRef: sample.OrganizationRef, ExpectedModel: model}
}

// Each actual attempt replaces the request-local selected projection. An
// unknown successor must not inherit a previous candidate's proved account.
func WithSelectedUsagePolicy(ctx context.Context, policy *ExecutionUsagePolicy) context.Context {
	state := &candidateUsageContext{selected: make(map[string]ExecutionUsagePolicy)}
	if policy != nil {
		state.selected[policy.ProfileID] = *policy
	}
	return context.WithValue(ctx, candidateUsageContextKey{}, state)
}

func ExecutionUsageSelectionChanged(policy ExecutionUsagePolicy, input ExecutionUsageInput, reason, model string, checkModel bool) bool {
	if policy.ExpectedAccountRef == "" {
		return false
	}
	if reason != "ok" || input.ExpectedAccountRef != policy.ExpectedAccountRef || input.ExpectedOrganizationRef != policy.ExpectedOrganizationRef {
		return true
	}
	source, sourceReason := resolveExecutionUsageSource(input)
	if sourceReason != "" || source.provider != policy.ExpectedProvider {
		return true
	}
	return checkModel && model != policy.ExpectedModel
}

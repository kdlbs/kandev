package dynamic

import (
	"github.com/kandev/kandev/internal/agent/usage"
	"time"
)

// CandidateUsageFromReport consumes an execution-bound native report. Callers
// must establish the execution namespace; a profile ID alone is not proof.
// Rank by the tightest applicable measured window, keeping its exact bucket.
func CandidateUsageFromReport(report *usage.ExecutionUsageReport, model string, now time.Time) CandidateUsage {
	if report == nil || !report.SourceKnown || report.BindingChanged || report.Model == "" || report.Model != model {
		return CandidateUsage{}
	}
	current, previous := report.Current.ProviderSample(), report.Previous.ProviderSample()
	if current == nil || current.AccountRef == "" {
		return CandidateUsage{}
	}
	limited := CandidateUsage{}
	if report.Limited && report.BlockingWindow != "" {
		limited = normalizedCandidateUsage(CandidateUsage{Bucket: UsageBucket{Provider: current.Provider, OrganizationRef: current.OrganizationRef, AccountRef: current.AccountRef, Window: report.BlockingWindow}, FetchedAt: current.FetchedAt, Limited: true}, now)
	}
	var result CandidateUsage
	for _, window := range current.Windows {
		if !usage.ExecutionUsageWindowApplies(window, model) {
			continue
		}
		metrics := usage.WindowMetricsForLabel(current, previous, window.Label, now)
		if !metrics.Known {
			return limited
		}
		if result.Known && result.RemainingPct <= metrics.RemainingPct {
			continue
		}
		result = CandidateUsage{
			Bucket:    UsageBucket{Provider: current.Provider, OrganizationRef: current.OrganizationRef, AccountRef: current.AccountRef, Window: window.Label, Model: window.Model},
			FetchedAt: current.FetchedAt, Known: true, RemainingPct: metrics.RemainingPct,
			ConsumptionRateKnown: metrics.ConsumptionRateKnown, ConsumptionPctPerHour: metrics.ConsumptionPctPerHour,
			ResetAtKnown: metrics.ResetAtKnown, ResetAt: metrics.ResetAt,
		}
	}
	result.Limited = limited.Limited
	return result
}

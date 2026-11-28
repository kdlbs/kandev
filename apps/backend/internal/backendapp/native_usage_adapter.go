package backendapp

import (
	"context"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	"time"
)

func officeUsageProvider(enabled bool, reader func(context.Context, string) ([]agentusage.ProfileExecutionUsage, error), fallback officeagents.UsageProvider) officeagents.UsageProvider {
	if enabled {
		return &nativeUsageProviderAdapter{read: reader}
	}
	return fallback
}

// Native Office reads observe original actors. They never register a host
// credential path as proof of a queued run or use a representative profile.
type nativeUsageProviderAdapter struct {
	read func(context.Context, string) ([]agentusage.ProfileExecutionUsage, error)
}

func (a *nativeUsageProviderAdapter) GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	entries, err := a.read(ctx, profileID)
	if err != nil {
		return nil, err
	}
	var selected *agentusage.ProviderUsage
	model := ""
	for _, entry := range entries {
		report := entry.Report
		sample, known := officeObservedUsage(report, time.Now())
		if !known {
			return nil, nil
		}
		if selected != nil && (selected.Provider != sample.Provider || selected.AccountRef != sample.AccountRef || selected.OrganizationRef != sample.OrganizationRef || model != report.Model) {
			return nil, nil
		}
		if selected == nil || sample.FetchedAt.After(selected.FetchedAt) {
			selected = sample
			model = report.Model
		}
	}
	return selected, nil
}

// The legacy Office DTO cannot express unknown per-window values/resets.
// Keep those unknown instead of rendering its legacy zero/reset fallbacks;
// the native profile usage surface exposes their full provenance separately.
func officeObservedUsage(report *agentusage.ExecutionUsageReport, now time.Time) (*agentusage.ProviderUsage, bool) {
	if report == nil || !report.SourceKnown || report.BindingChanged || report.Current == nil {
		return nil, false
	}
	sample := report.Current.ProviderSample()
	if sample.AccountRef == "" || sample.Provider == "" || sample.FetchedAt.IsZero() || sample.FetchedAt.After(now) || now.Sub(sample.FetchedAt) > 5*time.Minute || len(sample.Windows) == 0 {
		return nil, false
	}
	for _, window := range sample.Windows {
		if window.Observation == nil || !window.Observation.UtilizationObserved || !window.Observation.ResetAtObserved {
			return nil, false
		}
	}
	return sample, true
}

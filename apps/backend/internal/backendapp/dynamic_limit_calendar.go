package backendapp

import (
	"context"
	"strings"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// profileUsageReader is the observed-usage seam of one concrete profile.
type profileUsageReader interface {
	GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error)
}

// providerLimitLister is the operator-entered provider settings seam.
type providerLimitLister interface {
	ListProviderLimits(ctx context.Context) ([]dynamicruntime.ProviderLimit, error)
}

// dynamicLimitCalendar answers the reset instants a usage-limit suspension
// may wait for. Observed provider usage wins over operator settings, because
// it is the provider's own account state.
type dynamicLimitCalendar struct {
	usage   profileUsageReader
	limits  providerLimitLister
	timeout time.Duration
}

const limitCalendarUsageTimeout = 5 * time.Second

// minMonthlyWindow tells a monthly usage window from a five-hour or weekly one
// when the provider label is not "monthly".
const minMonthlyWindow = 27 * 24 * time.Hour

func newDynamicLimitCalendar(usage profileUsageReader, limits providerLimitLister) *dynamicLimitCalendar {
	return &dynamicLimitCalendar{usage: usage, limits: limits, timeout: limitCalendarUsageTimeout}
}

// ExhaustedUntil implements dynamic.LimitCalendar.
func (c *dynamicLimitCalendar) ExhaustedUntil(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
	now time.Time,
) (time.Time, bool) {
	if !c.matchesSelectedAccount(ctx, candidate) {
		return time.Time{}, false
	}
	if adapter, ok := c.usage.(*usageProviderAdapter); ok && adapter.tierLimits != nil && !candidate.RemoteExecution {
		if reset, found := adapter.tierLimits.windowReset(ctx, candidate.ID, "", now, true); found {
			return reset, true
		}
	}
	windows := c.observedWindows(ctx, candidate)
	if isDailyQuotaModel(candidate.ModelID) && hasExhaustedDailyWindow(windows) {
		// OpenRouter's free tier counts requests per UTC day, so its free daily
		// quota frees at the next UTC midnight whatever the provider's window
		// reset says. Waiting for the window's own reset would resume a fraction
		// of a second after the daily count was already rebuilt.
		return nextUTCMidnight(now), true
	}
	var until time.Time
	for _, window := range windows {
		if !window.Exhausted() || !window.ResetAt.After(now) {
			continue
		}
		if window.ResetAt.After(until) {
			until = window.ResetAt
		}
	}
	return until, !until.IsZero()
}

// MonthlyReset implements dynamic.LimitCalendar.
func (c *dynamicLimitCalendar) MonthlyReset(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
	now time.Time,
) (time.Time, bool) {
	if !c.matchesSelectedAccount(ctx, candidate) {
		return time.Time{}, false
	}
	if reset, ok := observedMonthlyReset(c.observedWindows(ctx, candidate), now); ok {
		return reset, true
	}
	if adapter, ok := c.usage.(*usageProviderAdapter); ok && adapter.tierLimits != nil && !candidate.RemoteExecution {
		if reset, found := adapter.tierLimits.windowReset(ctx, candidate.ID, "month", now, false); found {
			return reset, true
		}
	}
	limit, ok := c.providerLimit(ctx, dynamicruntime.ProviderOf(candidate.ModelID))
	if !ok {
		return time.Time{}, false
	}
	return limit.NextMonthlyReset(now)
}

func (c *dynamicLimitCalendar) matchesSelectedAccount(ctx context.Context, candidate dynamicruntime.Candidate) bool {
	if candidate.UsagePolicy == nil {
		return !candidate.RequireExecutionUsage
	}
	adapter, ok := c.usage.(*usageProviderAdapter)
	return ok && adapter.tierLimits != nil && adapter.tierLimits.matchesSelectedAccount(ctx, candidate)
}

// observedWindows returns the usage windows that are this candidate's own
// account state. A remote execution authenticates elsewhere, so the host's
// reading is never borrowed for it.
func (c *dynamicLimitCalendar) observedWindows(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
) []agentusage.UtilizationWindow {
	if c == nil || c.usage == nil || candidate.RemoteExecution || candidate.ID == "" {
		return nil
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	observed, err := c.usage.GetUsage(ctx, candidate.ID)
	if err != nil || observed == nil {
		return nil
	}
	windows := make([]agentusage.UtilizationWindow, 0, len(observed.Windows))
	for _, window := range observed.Windows {
		if window.ModelID != "" && window.ModelID != candidate.ModelID {
			continue
		}
		windows = append(windows, window)
	}
	return windows
}

func (c *dynamicLimitCalendar) providerLimit(ctx context.Context, provider string) (dynamicruntime.ProviderLimit, bool) {
	if c == nil || c.limits == nil || provider == "" {
		return dynamicruntime.ProviderLimit{}, false
	}
	limits, err := c.limits.ListProviderLimits(ctx)
	if err != nil {
		return dynamicruntime.ProviderLimit{}, false
	}
	for _, limit := range limits {
		if dynamicruntime.NormalizeProvider(limit.Provider) == provider {
			return limit, true
		}
	}
	return dynamicruntime.ProviderLimit{}, false
}

// observedMonthlyReset returns the earliest future reset among monthly usage
// windows.
func observedMonthlyReset(windows []agentusage.UtilizationWindow, now time.Time) (time.Time, bool) {
	var reset time.Time
	for _, window := range windows {
		if !isMonthlyWindow(window) || !window.ResetAt.After(now) {
			continue
		}
		if reset.IsZero() || window.ResetAt.Before(reset) {
			reset = window.ResetAt
		}
	}
	return reset, !reset.IsZero()
}

func isMonthlyWindow(window agentusage.UtilizationWindow) bool {
	if strings.EqualFold(strings.TrimSpace(window.Label), "monthly") {
		return true
	}
	return time.Duration(window.DurationSeconds)*time.Second >= minMonthlyWindow
}

// providerOpenRouterFree is the one provider whose free tier is known to count
// its quota per UTC day rather than per rolling window.
const providerOpenRouterFree = "openrouter"

// dailyWindowLabelMax bounds how long a window may be and still be read as a
// daily one. A daily quota is a provider's own statement about a day, so the
// length is only a fallback for a provider that labels its window numerically.
const dailyWindowLabelMax = 36 * time.Hour

// dailyWindowLabels are the labels a provider uses for its daily quota.
var dailyWindowLabels = []string{"daily", "per day", "per-day"}

// isDailyQuotaModel reports whether this model's free quota is counted per UTC
// day. Only a free model qualifies: a paid model's daily window is one of
// several and must not shorten the block past its own reset.
func isDailyQuotaModel(modelID string) bool {
	return dynamicruntime.ProviderOf(modelID) == providerOpenRouterFree && dynamicruntime.IsFreeModel(modelID)
}

// isDailyWindow reports whether a usage window is the provider's daily quota.
func isDailyWindow(window agentusage.UtilizationWindow) bool {
	label := strings.ToLower(strings.TrimSpace(window.Label))
	for _, daily := range dailyWindowLabels {
		if label == daily {
			return true
		}
	}
	duration := time.Duration(window.DurationSeconds) * time.Second
	return duration > 0 && duration <= dailyWindowLabelMax
}

// hasExhaustedDailyWindow reports whether any observed window is the consumed
// daily quota. A window that is not exhausted proves nothing about a daily
// limit, so it leaves the caller on its ordinary path.
func hasExhaustedDailyWindow(windows []agentusage.UtilizationWindow) bool {
	for _, window := range windows {
		if window.Exhausted() && isDailyWindow(window) {
			return true
		}
	}
	return false
}

// nextUTCMidnight returns the first UTC midnight strictly after now, which is
// when a per-UTC-day quota is rebuilt.
func nextUTCMidnight(now time.Time) time.Time {
	midnight := now.UTC().Truncate(24 * time.Hour)
	return midnight.Add(24 * time.Hour)
}

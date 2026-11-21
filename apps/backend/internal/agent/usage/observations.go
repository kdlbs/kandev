package usage

import (
	"math"
	"time"
)

// WindowMetrics contains measured provider-window facts. It establishes no
// relationship between a host observation and an executor's actual account.
type WindowMetrics struct {
	Known                 bool
	RemainingPct          float64
	ConsumptionRateKnown  bool
	ConsumptionPctPerHour float64
	// ResetAt is the provider reset time that observedWindow already proved is
	// explicit and still in the future. A reset with no such proof stays zero
	// and ResetAtKnown false.
	ResetAt      time.Time
	ResetAtKnown bool
}

// SnapshotPair reads at most two distinct successful samples without fetching.
// A recent refresh failure is unknown here; legacy cache reads stay unchanged.
func (c *UsageCache) SnapshotPair(key string) (current, previous *ProviderUsage) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry := c.entries[key]
	if entry == nil || entry.err != nil {
		return nil, nil
	}
	return cloneProviderUsage(entry.sample), cloneProviderUsage(entry.previous)
}

func cloneProviderUsage(source *ProviderUsage) *ProviderUsage {
	if source == nil {
		return nil
	}
	copy := *source
	if source.FreeModelDailyRequests != nil {
		counter := *source.FreeModelDailyRequests
		copy.FreeModelDailyRequests = &counter
	}
	copy.Windows = append([]UtilizationWindow(nil), source.Windows...)
	for index := range copy.Windows {
		if source.Windows[index].Observation != nil {
			observation := *source.Windows[index].Observation
			copy.Windows[index].Observation = &observation
		}
	}
	return &copy
}

// WindowMetricsForLabel never treats missing provenance or a cache read as a
// fresh measurement. Exact provider/account/window/reset identity is required
// for a rate; a reset, account change or repeated sample cannot supply one.
func WindowMetricsForLabel(current, previous *ProviderUsage, label string, now time.Time) WindowMetrics {
	result := WindowMetrics{}
	if !freshAccountSample(current, now) {
		return result
	}
	window, ok := observedWindow(current, label, now)
	if !ok {
		return result
	}
	result.Known, result.RemainingPct = true, 100-window.UtilizationPct
	result.ResetAt, result.ResetAtKnown = window.ResetAt, true
	if !comparableAccountSamples(current, previous) {
		return result
	}
	interval := current.FetchedAt.Sub(previous.FetchedAt)
	prior, ok := observedWindow(previous, label, now)
	if !ok || interval > 2*cacheTTL || !prior.ResetAt.Equal(window.ResetAt) || prior.Model != window.Model || prior.UtilizationPct > window.UtilizationPct {
		return result
	}
	result.ConsumptionRateKnown = true
	result.ConsumptionPctPerHour = (window.UtilizationPct - prior.UtilizationPct) / interval.Hours()
	return result
}

func observedWindow(sample *ProviderUsage, label string, now time.Time) (UtilizationWindow, bool) {
	var found UtilizationWindow
	count := 0
	for _, window := range sample.Windows {
		if window.Label == label {
			found = window
			count++
		}
	}
	if label == "" || count != 1 || found.Observation == nil || !found.Observation.UtilizationObserved || !found.Observation.ResetAtObserved || !found.ResetAt.After(now) {
		return found, false
	}
	return found, !math.IsNaN(found.UtilizationPct) && !math.IsInf(found.UtilizationPct, 0) && found.UtilizationPct >= 0 && found.UtilizationPct <= 100
}

func freshAccountSample(sample *ProviderUsage, now time.Time) bool {
	return sample != nil && sample.Provider != "" && sample.AccountRef != "" && !sample.FetchedAt.IsZero() && !sample.FetchedAt.After(now) && now.Sub(sample.FetchedAt) <= cacheTTL
}

func comparableAccountSamples(current, previous *ProviderUsage) bool {
	return previous != nil && previous.Provider == current.Provider && previous.AccountRef == current.AccountRef && previous.OrganizationRef == current.OrganizationRef && !previous.FetchedAt.IsZero() && previous.FetchedAt.Before(current.FetchedAt)
}

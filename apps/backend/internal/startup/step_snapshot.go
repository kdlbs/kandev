package startup

import "time"

// StepSnapshot is the wire shape of the currently active step. Fields are
// pointers so an absent value (per the measure's boundary rules) is omitted
// from JSON rather than rendered as a misleading zero.
type StepSnapshot struct {
	ID             StepID   `json:"id"`
	LabelKey       string   `json:"label_key"`
	Measure        Measure  `json:"measure"`
	Unit           Unit     `json:"unit"`
	ElapsedMS      int64    `json:"elapsed_ms"`
	Done           *int64   `json:"done,omitempty"`
	Total          *int64   `json:"total,omitempty"`
	RatePerSecond  *float64 `json:"rate_per_second,omitempty"`
	ETAMS          *int64   `json:"eta_ms,omitempty"`
	SinceAdvanceMS *int64   `json:"since_advance_ms,omitempty"`
	Stalled        bool     `json:"stalled"`
}

func (a *stepActivation) snapshot(now time.Time) *StepSnapshot {
	s := &StepSnapshot{
		ID:        a.spec.ID,
		LabelKey:  a.spec.LabelKey,
		Measure:   a.measure,
		Unit:      a.spec.Unit,
		ElapsedMS: now.Sub(a.startedAt).Milliseconds(),
	}
	if a.measure == MeasureOpaque {
		return s
	}

	done := a.done
	s.Done = &done
	if a.measure == MeasureCounted && a.hasTotal {
		total := a.total
		s.Total = &total
	}

	sinceAdvance := a.sinceAdvanceMS(now)
	s.SinceAdvanceMS = &sinceAdvance
	s.Stalled = sinceAdvance >= stallThresholdMS

	if a.hasAdvancedOnce {
		rate := a.rate(now)
		s.RatePerSecond = &rate
		if rate > 0 && a.measure == MeasureCounted && a.hasTotal {
			if eta, ok := a.etaMS(rate); ok {
				s.ETAMS = &eta
			}
		}
	}
	return s
}

func (a *stepActivation) sinceAdvanceMS(now time.Time) int64 {
	base := a.startedAt
	if a.hasAdvancedOnce {
		base = a.lastAdvanceAt
	}
	return now.Sub(base).Milliseconds()
}

func (a *stepActivation) etaMS(rate float64) (int64, bool) {
	remaining := a.total - a.done
	if remaining < 0 {
		remaining = 0
	}
	ms := float64(remaining) / rate * 1000
	if ms > maxETAMs {
		return 0, false
	}
	return int64(ms), true
}

// rate is the mean units-per-second increase over a trailing 30 s window,
// falling back to the step's start below two samples so a burst of very
// early advances cannot spike the estimate.
func (a *stepActivation) rate(now time.Time) float64 {
	windowStart, windowDone := a.rateWindowStart(now)
	elapsed := now.Sub(windowStart)
	if elapsed < time.Millisecond {
		elapsed = time.Millisecond
	}
	delta := a.done - windowDone
	if delta < 0 {
		delta = 0
	}
	return float64(delta) / elapsed.Seconds()
}

// rateWindowStart returns the time and cumulative done count at the older
// edge of the rate window: the oldest recorded sample no older than 30 s,
// or the step's start when fewer than two samples have ever been recorded.
// The returned timestamp is the point immediately before the first sample in
// the window, paired with the cumulative count at that point. When every
// sample is older than the window, it returns "now" paired with the current
// done count, which yields a zero rate rather than reviving a stale burst.
func (a *stepActivation) rateWindowStart(now time.Time) (time.Time, int64) {
	if len(a.samples) < 2 {
		return a.startedAt, a.anchorDone
	}
	cutoff := now.Add(-rateWindow)
	cumulative := a.anchorDone
	previousAt := a.startedAt
	for _, s := range a.samples {
		if !s.at.Before(cutoff) {
			return previousAt, cumulative
		}
		cumulative += s.delta
		previousAt = s.at
	}
	return now, a.done
}

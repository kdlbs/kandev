package dream

import "time"

// Health states.
const (
	HealthOff     = "off"
	HealthWaiting = "waiting"
	HealthRunning = "running"
	HealthFresh   = "fresh"
	HealthStale   = "stale"
	HealthFailed  = "failed"
)

// Waiting conditions, which the Learning section maps to copy and a fix.
const (
	WaitAutonomyOff       = "autonomy_off"
	WaitPaused            = "paused"
	WaitContainment       = "containment"
	WaitSpendUnmeasurable = "spend_unmeasurable"
	WaitCeiling           = "ceiling"
	WaitNoEvidence        = "no_evidence"
	WaitSpacing           = "spacing"
	WaitOverdue           = "overdue"
	WaitLastFailed        = "last_failed"
)

const (
	// StaleAfter and FailedAfter are the ages of an unmet evidence debt at
	// which the health turns stale and failed.
	StaleAfter  = 36 * time.Hour
	FailedAfter = 72 * time.Hour
	// Spacing is the least time between two dream episodes starting.
	Spacing = 24 * time.Hour
)

// HealthInput is the stored state the health is a pure function of.
type HealthInput struct {
	Now             time.Time
	Enabled         bool
	Running         bool
	Autonomy        bool
	Paused          bool
	ContainmentOK   bool
	SpendMeasurable bool
	AtCeiling       bool
	// LastFailedReason is set when the newest dream ended failed.
	LastFailed       bool
	LastFailedReason string
	// LastAcceptedAt is when the newest accepted dream finished; nil when none.
	LastAcceptedAt *time.Time
	// DebtSince is when the evidence debt began; nil when none exists.
	DebtSince *time.Time
	// NextAfter is when the 24-hour spacing ends; nil or past when it has.
	NextAfter *time.Time
}

// HealthState is the computed state with its condition and detail.
type HealthState struct {
	State string
	// Condition is a Wait* code, empty when the state needs none.
	Condition string
	// Detail carries the failed reason or the next-dream time.
	Detail    string
	NextAfter *time.Time
}

// Health decides the state by the first matching row of the precedence table.
func Health(in HealthInput) HealthState {
	switch {
	case !in.Enabled:
		return HealthState{State: HealthOff}
	case in.Running:
		return HealthState{State: HealthRunning}
	}
	if c := admissionCondition(in); c != "" {
		return HealthState{State: HealthWaiting, Condition: c}
	}
	debtAge := time.Duration(-1)
	if in.DebtSince != nil {
		debtAge = in.Now.Sub(*in.DebtSince)
	}
	switch {
	case in.LastFailed:
		return HealthState{State: HealthFailed, Condition: WaitLastFailed, Detail: in.LastFailedReason}
	case debtAge > FailedAfter:
		return HealthState{State: HealthFailed, Condition: WaitOverdue}
	case debtAge > StaleAfter:
		return HealthState{State: HealthStale, Condition: WaitOverdue}
	}
	if in.LastAcceptedAt != nil && (in.Now.Sub(*in.LastAcceptedAt) <= StaleAfter || in.DebtSince == nil) {
		return HealthState{State: HealthFresh}
	}
	if in.DebtSince != nil {
		if in.NextAfter != nil && in.Now.Before(*in.NextAfter) {
			return HealthState{State: HealthWaiting, Condition: WaitSpacing, NextAfter: in.NextAfter}
		}
		return HealthState{State: HealthFresh}
	}
	return HealthState{State: HealthWaiting, Condition: WaitNoEvidence}
}

// admissionCondition names the first failing admission condition, paused
// before autonomy as the scheduler orders them.
func admissionCondition(in HealthInput) string {
	switch {
	case in.Paused:
		return WaitPaused
	case !in.Autonomy:
		return WaitAutonomyOff
	case !in.ContainmentOK:
		return WaitContainment
	case !in.SpendMeasurable:
		return WaitSpendUnmeasurable
	case in.AtCeiling:
		return WaitCeiling
	}
	return ""
}

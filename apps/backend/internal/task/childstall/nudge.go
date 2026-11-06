package childstall

// Nudge routing for a missing-completion-signal stall.
//
// A child that ends a turn without the completion signal its step requires is
// reminded in its own session instead of waking the parent: the parent alert is
// what the operator turns into a child nudge anyway. Only a child that keeps
// stalling on the same workflow entry after repeated nudges escalates to the
// parent, once. Every other cause is delivered to the parent directly and does
// not use this state.

// NudgeAction is the routing decision for one qualified missing-signal stall.
type NudgeAction string

const (
	// NudgeActionNudgeChild sends a short reminder to the child's own session.
	NudgeActionNudgeChild NudgeAction = "nudge_child"
	// NudgeActionAlertParent escalates to the parent once the child has stalled
	// on the same workflow entry for EscalateAfter consecutive nudged turns.
	NudgeActionAlertParent NudgeAction = "alert_parent"
	// NudgeActionSuppress drops the candidate after the streak already
	// escalated, so one streak alerts the parent at most once.
	NudgeActionSuppress NudgeAction = "suppress"
)

// NudgeStreak is the consecutive missing-signal stall state for one child
// session and workflow entry.
type NudgeStreak struct {
	Cause        Cause
	TransitionID int64
	Count        int
	Escalated    bool
}

// NudgePolicy bounds how many consecutive nudged stalls precede a parent alert.
type NudgePolicy struct {
	// EscalateAfter is the consecutive missing-signal count at which the parent
	// is alerted once. A value below 1 disables escalation (always nudge).
	EscalateAfter int
}

// DefaultNudgeEscalateAfter is the shipped escalation threshold: a child that
// stalls on the same entry this many times in a row is reported to its parent.
const DefaultNudgeEscalateAfter = 3

// AdvanceNudge folds one qualified missing-signal stall into the streak and
// returns the next action. A change of cause or workflow entry starts a new
// streak, so a child that advances its step is nudged afresh rather than
// escalating on stalls that are no longer consecutive. Once a streak has
// escalated, further stalls on the same entry are suppressed.
func AdvanceNudge(prev NudgeStreak, cause Cause, transitionID int64, policy NudgePolicy) (NudgeStreak, NudgeAction) {
	if prev.Cause != cause || prev.TransitionID != transitionID {
		prev = NudgeStreak{Cause: cause, TransitionID: transitionID}
	}
	if prev.Escalated {
		return prev, NudgeActionSuppress
	}
	prev.Count++
	if policy.EscalateAfter >= 1 && prev.Count >= policy.EscalateAfter {
		prev.Escalated = true
		return prev, NudgeActionAlertParent
	}
	return prev, NudgeActionNudgeChild
}

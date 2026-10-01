// Package ledger records one row per coordinator turn: its stamp, call digest,
// frozen board snapshot and verdict. It only observes; nothing here can fail or
// delay a turn, a proposal or a decision.
package ledger

import "slices"

// Verdict values, in precedence order.
const (
	VerdictBlocked       = "blocked"
	VerdictActed         = "acted"
	VerdictProposed      = "proposed"
	VerdictNeedsYou      = "needs_you"
	VerdictNothingNeeded = "nothing_needed"
)

// Turn trigger values.
const (
	TriggerMessage = "message"
	TriggerWake    = "wake"
	TriggerDream   = "dream"
)

// VerdictInput is everything Verdict reads; it holds stored facts only.
type VerdictInput struct {
	Outcome        string
	Trigger        string
	WakeKinds      []string
	Calls          int
	Refused        int
	CallsTruncated bool
	// RetryPending is true while a call of the turn's session is parked on the
	// retry list, so the digest may be incomplete.
	RetryPending bool
	Proposals    int
	// Acted is true when an automatic approval executed inside the turn.
	Acted bool
	// ActionPending is the pending-interaction read at grade time.
	ActionPending bool
}

var blockingOutcomes = []string{"failed", "cancelled", "interrupted", "stopped_at_ceiling", "stopped_by_pause"}

// Verdict grades a turn: blocked, acted, proposed, needs_you, nothing_needed,
// first match wins. It performs no I/O.
func Verdict(in VerdictInput) string {
	if slices.Contains(blockingOutcomes, in.Outcome) || allCallsRefused(in) {
		return VerdictBlocked
	}
	if in.Acted {
		return VerdictActed
	}
	if in.Proposals > 0 {
		return VerdictProposed
	}
	if in.Trigger == TriggerWake && in.ActionPending &&
		(slices.Contains(in.WakeKinds, "question") || slices.Contains(in.WakeKinds, "permission")) {
		return VerdictNeedsYou
	}
	return VerdictNothingNeeded
}

func allCallsRefused(in VerdictInput) bool {
	return in.Calls > 0 && in.Refused == in.Calls && !in.CallsTruncated && !in.RetryPending && in.Proposals == 0
}

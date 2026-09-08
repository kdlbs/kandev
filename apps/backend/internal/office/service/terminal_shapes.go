package service

import "time"

// TerminalShape is the derived classification of a terminal run
// (REQ-OFFICE-LOOP-LIVENESS-005). It never widens runs.status or
// runs.outcome — see docs/specs/task-delivery-ledger/spec.md, which
// owns runs.outcome.
type TerminalShape string

const (
	// ShapePreActivation is a run predating the point this capability's
	// columns were confirmed present, or read before that point was ever
	// published — never a defect (AC-005.6).
	ShapePreActivation TerminalShape = "pre_activation"
	// ShapeLaunchedCompleted is a run that reported success and named a
	// session.
	ShapeLaunchedCompleted TerminalShape = "launched_completed"
	// ShapeLaunchedFailed is a run that reached a terminal failure and
	// named a session.
	ShapeLaunchedFailed TerminalShape = "launched_failed"
	// ShapeSilentSuccess is a run whose outcome asserts work happened but
	// which never named a session — NFR-1 made observable (AC-005.4).
	ShapeSilentSuccess TerminalShape = "silent_success"
	// ShapeUnlaunchedSkipped is a legitimate no-launch skip, kept apart
	// from a launch failure (AC-005.5).
	ShapeUnlaunchedSkipped TerminalShape = "unlaunched_skipped"
	// ShapeUnlaunchedFailed is a terminal failure that never named a
	// session.
	ShapeUnlaunchedFailed TerminalShape = "unlaunched_failed"
	// ShapeUnclassified is every combination none of the above predicates
	// claim, including a null outcome, an outcome this document does not
	// enumerate (such as the historical no_agent_launched), and an
	// unknown status — the row that makes the classification total
	// (AC-005.2).
	ShapeUnclassified TerminalShape = "unclassified"
)

var terminalFailedStatuses = map[string]bool{
	RunStatusFailed:    true,
	"timed_out":        true,
	RunStatusCancelled: true,
}

var skipOutcomes = map[string]bool{
	RunOutcomeIdleSkipped:   true,
	RunOutcomeBudgetBlocked: true,
	RunOutcomeAgentInactive: true,
	RunOutcomeTaskTreeHeld:  true,
}

// ClassifyTerminalRun classifies one terminal run (status not in
// {queued, claimed} — AC-005.9 excludes those from every terminal
// total; they are the stuck-run detector's input instead) into exactly
// one TerminalShape. Reads only the four persisted values named in its
// signature, never a free-text message (AC-005.7), and evaluates
// top-to-bottom with first match winning — the ordering is load-bearing:
// pre_activation first so the activation guard cannot be bypassed by a
// later row matching, silent_success below launched_completed so the
// session test is what separates them, and above unclassified so a null
// outcome never absorbs it.
//
// activationInstant/activationPublished carry the boot-time probe result
// (persisted separately in kandev_meta); a run requested before
// activation, or read when activation was never published, is always
// pre_activation regardless of what it would otherwise classify as.
func ClassifyTerminalRun(
	status string,
	outcome *string,
	sessionID string,
	requestedAt time.Time,
	activationInstant time.Time,
	activationPublished bool,
) TerminalShape {
	if !activationPublished || requestedAt.Before(activationInstant) {
		return ShapePreActivation
	}

	outcomeVal := ""
	if outcome != nil {
		outcomeVal = *outcome
	}
	return classifyByOutcome(status, outcomeVal, sessionID != "")
}

// classifyByOutcome applies the first-match-wins predicate table once
// activation is confirmed, split out from ClassifyTerminalRun to keep
// each function's branching within the complexity limit.
func classifyByOutcome(status, outcomeVal string, launched bool) TerminalShape {
	processed := outcomeVal == RunOutcomeProcessed
	failedTerminal := terminalFailedStatuses[status]

	switch {
	case status == RunStatusFinished && processed && launched:
		return ShapeLaunchedCompleted
	case failedTerminal && launched:
		return ShapeLaunchedFailed
	case status == RunStatusFinished && processed && !launched:
		return ShapeSilentSuccess
	case status == RunStatusFinished && !launched && skipOutcomes[outcomeVal]:
		return ShapeUnlaunchedSkipped
	case failedTerminal && !launched:
		return ShapeUnlaunchedFailed
	default:
		return ShapeUnclassified
	}
}

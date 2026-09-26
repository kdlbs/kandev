package lifecycle

import (
	"errors"
	"time"
)

// ErrUncertainPromptDelivery marks a prompt whose transport accepted the
// request but whose terminal outcome was not established after disconnect.
// Callers must reconcile the durable submission before any retry.
var ErrUncertainPromptDelivery = errors.New("prompt delivery outcome is uncertain")

type SubmissionReconciliationState string

const (
	SubmissionConnected   SubmissionReconciliationState = "connected"
	SubmissionReconciling SubmissionReconciliationState = "reconciling"
	SubmissionRecovered   SubmissionReconciliationState = "recovered"
	SubmissionUncertain   SubmissionReconciliationState = "uncertain"
	SubmissionStopped     SubmissionReconciliationState = "stopped"
)

type ReconciliationEvidence struct {
	State            SubmissionReconciliationState
	TerminalObserved bool
	TerminalOutcome  string
	OwnerReachable   bool
	JournalAvailable bool
	Now              time.Time
	Deadline         time.Time
	StopRequested    bool
}

type ReconciliationDecision struct {
	State              SubmissionReconciliationState
	QueueBlocked       bool
	StopAvailable      bool
	CanRetryStateQuery bool
	Outcome            string
}

// DecideSubmissionReconciliation turns a transport break into a bounded
// state query. It never authorizes another prompt dispatch.
func DecideSubmissionReconciliation(e ReconciliationEvidence) ReconciliationDecision {
	decision := ReconciliationDecision{
		State:              SubmissionReconciling,
		QueueBlocked:       true,
		StopAvailable:      true,
		CanRetryStateQuery: true,
	}
	if e.StopRequested {
		decision.State = SubmissionStopped
		decision.CanRetryStateQuery = false
		return decision
	}
	if e.TerminalObserved {
		decision.State = SubmissionRecovered
		decision.QueueBlocked = false
		decision.CanRetryStateQuery = false
		decision.Outcome = e.TerminalOutcome
		return decision
	}
	if !e.JournalAvailable || (!e.Deadline.IsZero() && !e.Now.IsZero() && !e.Now.Before(e.Deadline)) {
		decision.State = SubmissionUncertain
		decision.CanRetryStateQuery = false
		return decision
	}
	if e.OwnerReachable {
		decision.State = SubmissionReconciling
	}
	return decision
}

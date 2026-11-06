// Package childstall classifies a settled child-task turn from structured
// task-system evidence. It is pure: callers gather the evidence and persist
// the decision. No transcript text, elapsed-time heuristic, or paid inference
// participates in a decision.
package childstall

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Cause is the actionable category of a qualified stall.
type Cause string

const (
	CauseInputRequired           Cause = "input_required"
	CauseQuota                   Cause = "quota"
	CauseExecutionError          Cause = "execution_error"
	CauseMissingCompletionSignal Cause = "missing_completion_signal"
)

// Outcome is the producer state a decision moves a candidate to.
type Outcome string

const (
	// OutcomeSettling means turn-end handling may still be in progress;
	// classify again later.
	OutcomeSettling Outcome = "settling"
	// OutcomeSuppressed means the settlement is not actionable.
	OutcomeSuppressed Outcome = "suppressed"
	// OutcomeUnknown means identity evidence is missing; never delivered.
	OutcomeUnknown Outcome = "unknown"
	// OutcomeHeld links the candidate to a pending parent question whose own
	// delivery has not yet gone unanswered.
	OutcomeHeld Outcome = "held"
	// OutcomeQualified means the candidate should alert the parent.
	OutcomeQualified Outcome = "qualified"
)

// Suppression and unknown reasons. They form a closed set so they can be
// counted without unbounded label values.
const (
	ReasonUnknownEntry      = "unknown_entry"
	ReasonAbandoned         = "abandoned"
	ReasonCancelled         = "cancelled"
	ReasonChildMissing      = "child_missing"
	ReasonChildArchived     = "child_archived"
	ReasonChildTerminal     = "child_terminal"
	ReasonReparented        = "reparented"
	ReasonWorkspaceChanged  = "workspace_changed"
	ReasonResumed           = "resumed"
	ReasonEntryChanged      = "entry_changed"
	ReasonSessionClosed     = "session_closed"
	ReasonSignalUnprocessed = "signal_unprocessed"
	ReasonOrdinary          = "ordinary"
	ReasonQuestionAnswered  = "question_answered"
)

// ChildSnapshot is the child task's current state.
type ChildSnapshot struct {
	ParentID           string
	WorkspaceID        string
	State              v1.TaskState
	Archived           bool
	LatestTransitionID int64
}

// PendingInput is one unresolved input request in the settling session.
// ParentQuestionID is set when the request is a pending parent question.
type PendingInput struct {
	MessageID        string
	ParentQuestionID string
}

// Evidence is everything a classification reads. Child is nil when the child
// task no longer exists.
type Evidence struct {
	Start              models.ChildStallStart
	Settlement         string
	Child              *ChildSnapshot
	SessionState       models.TaskSessionState
	SessionErrorClass  string
	SessionActiveTurn  bool
	PendingInputs      []PendingInput
	StepRequiresSignal bool
	PendingStepSignal  bool
	// BlockedReason is the short reason a completion signal for the current
	// step was refused by the ownership/profile gate, when one was recorded. It
	// lets a missing-signal reminder name the real block instead of implying the
	// agent simply forgot to signal.
	BlockedReason string
	// SettledFor is how long ago the turn completed.
	SettledFor time.Duration
}

// Decision is the classification result.
type Decision struct {
	Outcome       Outcome
	Cause         Cause
	Reason        string
	QuestionID    string
	BlockedReason string
}

// Policy bounds the settling wait.
type Policy struct {
	// SettleGrace is the minimum age of a settlement before it is classified.
	SettleGrace time.Duration
	// SignalWait bounds how long an unprocessed completion signal defers
	// classification before the candidate is suppressed.
	SignalWait time.Duration
}

// QuotaErrorClass is the session route error class that marks a quota limit.
const QuotaErrorClass = "quota_limited"

// Classify returns the decision for one settled child turn.
func Classify(evidence Evidence, policy Policy) Decision {
	if evidence.Start.TransitionID == 0 {
		return Decision{Outcome: OutcomeUnknown, Reason: ReasonUnknownEntry}
	}
	if evidence.Settlement == models.ChildStallSettlementAbandoned {
		return suppressed(ReasonAbandoned)
	}
	if evidence.SettledFor < policy.SettleGrace {
		return Decision{Outcome: OutcomeSettling}
	}
	if reason, stale := staleReason(evidence); stale {
		return suppressed(reason)
	}
	return classifyCause(evidence, policy)
}

// staleReason reports why the settlement predicate no longer holds.
func staleReason(evidence Evidence) (string, bool) {
	child := evidence.Child
	switch {
	case child == nil:
		return ReasonChildMissing, true
	case child.Archived:
		return ReasonChildArchived, true
	case child.State == v1.TaskStateCompleted || child.State == v1.TaskStateCancelled:
		return ReasonChildTerminal, true
	case child.ParentID != evidence.Start.ParentTaskID:
		return ReasonReparented, true
	case child.WorkspaceID != evidence.Start.WorkspaceID:
		return ReasonWorkspaceChanged, true
	case evidence.SessionActiveTurn || sessionExecuting(evidence.SessionState):
		return ReasonResumed, true
	case child.LatestTransitionID != evidence.Start.TransitionID:
		return ReasonEntryChanged, true
	}
	return "", false
}

func classifyCause(evidence Evidence, policy Policy) Decision {
	for _, input := range evidence.PendingInputs {
		if input.ParentQuestionID != "" {
			return Decision{Outcome: OutcomeHeld, Cause: CauseInputRequired, QuestionID: input.ParentQuestionID}
		}
	}
	if len(evidence.PendingInputs) > 0 {
		return qualified(CauseInputRequired)
	}
	if evidence.Settlement == models.ChildStallSettlementCancelled {
		return suppressed(ReasonCancelled)
	}
	switch evidence.SessionState {
	case models.TaskSessionStateFailed:
		if evidence.SessionErrorClass == QuotaErrorClass {
			return qualified(CauseQuota)
		}
		return qualified(CauseExecutionError)
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		return suppressed(ReasonSessionClosed)
	}
	if !evidence.StepRequiresSignal {
		return suppressed(ReasonOrdinary)
	}
	if evidence.PendingStepSignal {
		if evidence.SettledFor < policy.SignalWait {
			return Decision{Outcome: OutcomeSettling}
		}
		return suppressed(ReasonSignalUnprocessed)
	}
	decision := qualified(CauseMissingCompletionSignal)
	decision.BlockedReason = evidence.BlockedReason
	return decision
}

func sessionExecuting(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateRunning || state == models.TaskSessionStateStarting
}

func suppressed(reason string) Decision {
	return Decision{Outcome: OutcomeSuppressed, Reason: reason}
}

func qualified(cause Cause) Decision {
	return Decision{Outcome: OutcomeQualified, Cause: cause}
}

// QuestionPromotion is the evidence for promoting a held parent-question
// candidate.
type QuestionPromotion struct {
	QuestionPending bool
	// QuestionQueued is true while the question prompt is still queued for
	// the parent.
	QuestionQueued bool
	// ParentIdle is true when the parent primary session is promptable and
	// has no active turn.
	ParentIdle bool
	// ParentSettledAfterQuestion is true when a parent turn that started
	// after the question was created has completed at least the settle grace
	// ago.
	ParentSettledAfterQuestion bool
}

// PromoteHeld decides whether a held parent-question candidate becomes an
// alert. It returns OutcomeHeld while the original delivery may still answer.
func PromoteHeld(evidence QuestionPromotion) Decision {
	if !evidence.QuestionPending {
		return suppressed(ReasonQuestionAnswered)
	}
	if evidence.QuestionQueued || !evidence.ParentIdle || !evidence.ParentSettledAfterQuestion {
		return Decision{Outcome: OutcomeHeld, Cause: CauseInputRequired}
	}
	return qualified(CauseInputRequired)
}

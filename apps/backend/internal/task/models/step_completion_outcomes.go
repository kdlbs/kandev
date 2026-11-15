package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskCompletionSnapshot is one statement-consistent view of a task's current
// route, used for completion result projection.
type TaskCompletionSnapshot struct {
	State            v1.TaskState
	WorkflowID       string
	StepID           string
	QueuedForStepID  string
	EntryID          int64
	PrimarySessionID string
}

// SessionMetaKeyLastStepCompletionReceipt stores one bounded receipt for the
// most recently accepted step-completion source. It is written together with
// the pending signal and survives bag consumption, so a retry from the same
// source can distinguish acceptance from a committed transition.
const SessionMetaKeyLastStepCompletionReceipt = "last_step_completion_receipt"

// StepCompletionReceiptVersion is the current persisted receipt schema.
const StepCompletionReceiptVersion = 1

// Step-completion receipt outcomes. A committed outcome proves a database move,
// not prompt delivery.
const (
	StepCompletionOutcomePending            = "pending"
	StepCompletionOutcomeBlocked            = "blocked"
	StepCompletionOutcomeNotConfigured      = "not_configured"
	StepCompletionOutcomeSuperseded         = "superseded"
	StepCompletionOutcomeTransitionApplied  = "transition_applied"
	StepCompletionOutcomeDestinationQueued  = "destination_queued"
	StepCompletionOutcomeUnproven           = "unproven"
	stepCompletionReceiptOutcomeCommitted   = StepCompletionOutcomeTransitionApplied
	stepCompletionReceiptOutcomeQueuedProof = StepCompletionOutcomeDestinationQueued
)

// Step-completion safe reason codes. They never carry free-form payload data.
const (
	StepCompletionReasonSignalCancelled         = "signal_cancelled"
	StepCompletionReasonWorkflowEntryChanged    = "workflow_entry_changed"
	StepCompletionReasonSupersededByLaterSource = "superseded_by_later_source"
	StepCompletionReasonDestinationUnavailable  = "destination_unavailable"
	StepCompletionReasonTransitionRefused       = "transition_refused"
	StepCompletionReasonStepNotSignalGated      = "step_not_signal_gated"
	StepCompletionReasonNoAutomaticMove         = "no_automatic_move"
)

// StepCompletionTransition is the persisted proof of the move a winning
// completion signal produced.
type StepCompletionTransition struct {
	LedgerID   int64  `json:"ledger_id"`
	WorkflowID string `json:"workflow_id,omitempty"`
	StepID     string `json:"step_id"`
	Queued     bool   `json:"queued,omitempty"`
}

// StepCompletionReceipt is the one bounded durable record of an accepted
// completion source and its known outcome.
type StepCompletionReceipt struct {
	Version                  int                       `json:"version"`
	SignalID                 string                    `json:"signal_id"`
	TaskID                   string                    `json:"task_id,omitempty"`
	SessionID                string                    `json:"session_id,omitempty"`
	SourceTurnID             string                    `json:"source_turn_id,omitempty"`
	StepID                   string                    `json:"step_id"`
	WorkflowStepTransitionID *int64                    `json:"workflow_step_transition_id,omitempty"`
	AcceptedAt               time.Time                 `json:"accepted_at"`
	Outcome                  string                    `json:"outcome"`
	Reason                   string                    `json:"reason,omitempty"`
	Transition               *StepCompletionTransition `json:"transition,omitempty"`
}

// NewStepCompletionSignalID mints a stable identity for an accepted signal.
// Callers mint only for a winning claim so a losing candidate leaves no ID.
func NewStepCompletionSignalID() string { return uuid.NewString() }

// NewStepCompletionReceipt builds the pending receipt written with a winning
// signal claim.
func NewStepCompletionReceipt(taskID, sessionID string, signal PendingStepCompletionSignal, acceptedAt time.Time) StepCompletionReceipt {
	return StepCompletionReceipt{
		Version:                  StepCompletionReceiptVersion,
		SignalID:                 signal.SignalID,
		TaskID:                   taskID,
		SessionID:                sessionID,
		SourceTurnID:             signal.SourceTurnID,
		StepID:                   signal.StepID,
		WorkflowStepTransitionID: cloneEntryID(signal.WorkflowStepTransitionID),
		AcceptedAt:               acceptedAt,
		Outcome:                  StepCompletionOutcomePending,
	}
}

// MatchesSource reports whether the receipt belongs to the exact source key of
// the given signal, including the originating source turn and workflow entry.
func (r StepCompletionReceipt) MatchesSource(signal PendingStepCompletionSignal) bool {
	return r.StepID == signal.StepID &&
		r.SameEntry(signal.WorkflowStepTransitionID) &&
		r.SourceTurnID == signal.SourceTurnID
}

// SameEntry compares provenance entries where nil means unproven.
func (r StepCompletionReceipt) SameEntry(entry *int64) bool {
	if r.WorkflowStepTransitionID == nil || entry == nil {
		return r.WorkflowStepTransitionID == nil && entry == nil
	}
	return *r.WorkflowStepTransitionID == *entry
}

// IsCommitted reports whether the receipt proves a database transition.
func (r StepCompletionReceipt) IsCommitted() bool {
	return r.Outcome == stepCompletionReceiptOutcomeCommitted ||
		r.Outcome == stepCompletionReceiptOutcomeQueuedProof
}

// HasProof reports whether the receipt carries transition proof.
func (r StepCompletionReceipt) HasProof() bool { return r.Transition != nil && r.IsCommitted() }

// WithTransition records the committed move on the receipt.
func (r StepCompletionReceipt) WithTransition(transition StepCompletionTransition) StepCompletionReceipt {
	r.Outcome = StepCompletionOutcomeTransitionApplied
	if transition.Queued {
		r.Outcome = StepCompletionOutcomeDestinationQueued
	}
	r.Reason = ""
	r.Transition = &transition
	return r
}

// LoadStepCompletionReceipt decodes the bounded receipt from session metadata.
// It tolerates both typed values and JSON-rehydrated maps.
func LoadStepCompletionReceipt(metadata map[string]interface{}) (StepCompletionReceipt, bool) {
	if metadata == nil {
		return StepCompletionReceipt{}, false
	}
	raw, ok := metadata[SessionMetaKeyLastStepCompletionReceipt]
	if !ok || raw == nil {
		return StepCompletionReceipt{}, false
	}
	switch v := raw.(type) {
	case StepCompletionReceipt:
		return v, validStepCompletionReceipt(v)
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return StepCompletionReceipt{}, false
		}
		var out StepCompletionReceipt
		if err := json.Unmarshal(data, &out); err != nil {
			return StepCompletionReceipt{}, false
		}
		return out, validStepCompletionReceipt(out)
	}
	return StepCompletionReceipt{}, false
}

func validStepCompletionReceipt(r StepCompletionReceipt) bool {
	if r.SignalID == "" || r.StepID == "" {
		return false
	}
	if r.Outcome == "" {
		return false
	}
	if r.WorkflowStepTransitionID != nil && *r.WorkflowStepTransitionID < 0 {
		return false
	}
	if r.Transition != nil && r.Transition.LedgerID < 0 {
		return false
	}
	return true
}

// SignalFromReceipt reconstructs the observable signal identity of a consumed
// source for a duplicate response. It never supplies payload text.
func SignalFromReceipt(receipt StepCompletionReceipt) PendingStepCompletionSignal {
	return PendingStepCompletionSignal{
		SignalID:                 receipt.SignalID,
		SourceTurnID:             receipt.SourceTurnID,
		WorkflowStepTransitionID: cloneEntryID(receipt.WorkflowStepTransitionID),
		StepID:                   receipt.StepID,
		SignaledAt:               receipt.AcceptedAt,
	}
}

func cloneEntryID(entry *int64) *int64 {
	if entry == nil {
		return nil
	}
	value := *entry
	return &value
}

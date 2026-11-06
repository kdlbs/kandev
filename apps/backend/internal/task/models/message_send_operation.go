package models

import (
	"errors"
	"time"
)

// MetaKeySendOperationID rides on the delivered artifact (the recorded user
// message's metadata) so an operator reading a transcript can tie a delivered
// message back to the operation identity that produced it. It is provenance
// only: nothing in the delivery path reads it back, and its absence never
// changes a delivery.
const MetaKeySendOperationID = "send_operation_id"

// Send-operation states. The set is closed: a row is claimed as
// SendOperationStatePending before the first delivery side effect and settles
// exactly once into Committed (dispatch accepted) or Failed. Failed may also
// retain an unknown outcome; only an exact dispatch rejection is retry safe.
const (
	SendOperationStatePending   = "pending"
	SendOperationStateCommitted = "committed"
	SendOperationStateFailed    = "failed"
)

// Send-operation delivery statuses, mirroring the message_task_kandev response
// status enum. Empty until the operation settles: a pending row has no delivery
// outcome yet.
const (
	SendOperationDeliverySent    = "sent"
	SendOperationDeliveryQueued  = "queued"
	SendOperationDeliveryStarted = "started"
)

// SendOperationFailureOutcomeUnknown is the failure code stamped by the
// stale-pending sweep on a row whose owning process died between the claim and
// the settlement. It is a stable code, never free text, so a rejection can
// never echo prompt content back to a caller.
const (
	SendOperationFailureOutcomeUnknown   = "outcome_unknown"
	SendOperationFailureDispatchRejected = "dispatch_rejected"
)

// ErrSendOperationNotFound means no retained operation row exists for the
// requested sender pair and identity. Retention bounds this observation; it
// cannot prove that a historical send never happened.
var ErrSendOperationNotFound = errors.New("send operation not found")

// ErrSendOperationNotClaimed means the insert-claim lost the unique-index race
// for this identity, so another call owns the delivery. The caller reads the
// existing row and either replays it as a read or rejects the mismatch.
var ErrSendOperationNotClaimed = errors.New("send operation identity already claimed")

// TaskMessageSendOperation is the durable record of one logical agent-to-agent
// send. It exists so a caller whose message_task_kandev response was lost can
// learn what happened instead of retrying blind: the row is claimed durably
// before the first delivery side effect and settled afterwards with the
// identity of the artifact that was actually delivered.
//
// RequestedPrompt, RequestedDeliveryMode, RequestedSessionID and RequestedQuestionID store the
// delivery-determining request fields verbatim, exactly as the caller sent
// them. A replay is compared against them by direct value equality; no digest,
// hash, checksum, or signature of them is computed, stored, or compared. That
// comparison answers only "is this the same send" and is not a
// content-integrity or tamper-detection control.
type TaskMessageSendOperation struct {
	ID              string
	OperationID     string
	SenderTaskID    string
	SenderSessionID string
	TargetTaskID    string
	TargetSessionID string

	RequestedPrompt       string
	RequestedDeliveryMode string
	RequestedSessionID    string
	RequestedQuestionID   string

	State          string
	DeliveryStatus string
	MessageID      string
	QueuedEntryID  string
	FailureCode    string

	CreatedAt time.Time
	UpdatedAt time.Time
	SettledAt *time.Time
}

// RetrySafe reports whether the caller may send the same content again under a
// fresh identity. Only a definite failure qualifies: a pending row means the
// first attempt may still commit, and a committed row means it did.
func (o *TaskMessageSendOperation) RetrySafe() bool {
	return o != nil && o.State == SendOperationStateFailed &&
		o.FailureCode == SendOperationFailureDispatchRejected
}

// MatchesRequest reports whether a replayed send carries exactly the
// delivery-determining fields this operation was claimed with. It is a plain
// field-by-field equality check against the stored values.
func (o *TaskMessageSendOperation) MatchesRequest(targetTaskID, prompt, deliveryMode, requestedSessionID, requestedQuestionID string) bool {
	if o == nil {
		return false
	}
	return o.TargetTaskID == targetTaskID && o.RequestedPrompt == prompt &&
		o.RequestedDeliveryMode == deliveryMode &&
		o.RequestedSessionID == requestedSessionID &&
		o.RequestedQuestionID == requestedQuestionID
}

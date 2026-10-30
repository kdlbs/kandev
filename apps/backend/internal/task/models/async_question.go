package models

import (
	"errors"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

// AsyncQuestionStatus is the canonical lifecycle state of one async question
// request. Only pending can transition; a terminal state never reopens.
type AsyncQuestionStatus string

const (
	AsyncQuestionStatusPending   AsyncQuestionStatus = "pending"
	AsyncQuestionStatusAnswered  AsyncQuestionStatus = "answered"
	AsyncQuestionStatusRejected  AsyncQuestionStatus = "rejected"
	AsyncQuestionStatusWithdrawn AsyncQuestionStatus = "withdrawn"
	AsyncQuestionStatusStale     AsyncQuestionStatus = "stale"
)

// Terminal reports whether the status can no longer transition.
func (s AsyncQuestionStatus) Terminal() bool {
	switch s {
	case AsyncQuestionStatusAnswered, AsyncQuestionStatusRejected,
		AsyncQuestionStatusWithdrawn, AsyncQuestionStatusStale:
		return true
	default:
		return false
	}
}

// AsyncQuestionRecipient identifies who owns the answer decision.
type AsyncQuestionRecipient string

const (
	AsyncQuestionRecipientUser   AsyncQuestionRecipient = "user"
	AsyncQuestionRecipientParent AsyncQuestionRecipient = "parent"
)

// AsyncQuestionResponderKind records which authorized responder produced the
// winning outcome.
type AsyncQuestionResponderKind string

const (
	AsyncQuestionResponderUser   AsyncQuestionResponderKind = "user"
	AsyncQuestionResponderParent AsyncQuestionResponderKind = "parent"
)

// AsyncQuestionOutcome is the one winning terminal answer for a request. The
// answer payload preserves item identity, option IDs, free text, and who
// answered.
type AsyncQuestionOutcome struct {
	Answers      []protocol.Answer `json:"answers,omitempty"`
	Rejected     bool              `json:"rejected,omitempty"`
	RejectReason string            `json:"reject_reason,omitempty"`
	// Text is the bundle-level free-text answer a direct parent may give
	// instead of one structured answer per item.
	Text            string                     `json:"text,omitempty"`
	RespondedAt     time.Time                  `json:"responded_at"`
	ResponderKind   AsyncQuestionResponderKind `json:"responder_kind"`
	ResponderTaskID string                     `json:"responder_task_id,omitempty"`
	ResponderID     string                     `json:"responder_id,omitempty"`
	Version         int64                      `json:"version"`
}

// AsyncQuestion is one durable, task-owned request bundle. Item IDs remain
// local to the bundle; ID identifies the whole request.
type AsyncQuestion struct {
	ID                         string
	TaskID                     string
	WorkspaceID                string
	SessionID                  string
	SessionIncarnationID       string
	TurnID                     string
	Recipient                  AsyncQuestionRecipient
	ParentTaskID               string
	ParentSessionID            string
	ParentSessionIncarnationID string
	Questions                  []protocol.Question
	Context                    string
	RetryKey                   string
	PayloadDigest              string
	Status                     AsyncQuestionStatus
	Outcome                    *AsyncQuestionOutcome
	WithdrawReason             string
	Version                    int64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	ResolvedAt                 *time.Time
	// Forwarded identifies a parent request the operator can see through an
	// authorized forward. It is derived from task_async_question_forwards.
	Forwarded bool
}

// AsyncQuestionForward links one parent request to exactly one forwarding
// parent session. The child request keeps its canonical identity.
type AsyncQuestionForward struct {
	QuestionID      string
	ParentTaskID    string
	ParentSessionID string
	Reason          string
	CreatedAt       time.Time
}

// AsyncQuestionDeliveryKind identifies which correlated continuation one
// delivery intent carries.
type AsyncQuestionDeliveryKind string

const (
	AsyncQuestionDeliveryOwner          AsyncQuestionDeliveryKind = "owner"
	AsyncQuestionDeliveryParentQuestion AsyncQuestionDeliveryKind = "parent_question"
	AsyncQuestionDeliveryParentOutcome  AsyncQuestionDeliveryKind = "parent_outcome"
	AsyncQuestionDeliveryWake           AsyncQuestionDeliveryKind = "wake"
)

// AsyncQuestionDeliveryState is the durable outbox state. An intent is
// admitted only after the exact queue admission receipt exists.
type AsyncQuestionDeliveryState string

const (
	AsyncQuestionDeliveryPending   AsyncQuestionDeliveryState = "pending"
	AsyncQuestionDeliveryAdmitted  AsyncQuestionDeliveryState = "admitted"
	AsyncQuestionDeliveryAbandoned AsyncQuestionDeliveryState = "abandoned"
)

// AsyncQuestionDelivery is one immutable outbox row. AdmissionID is derived
// from request, outcome version, kind, and destination so a crash after
// admission repeats the same id and produces no second queue row.
type AsyncQuestionDelivery struct {
	AdmissionID              string
	QuestionID               string
	OutcomeVersion           int64
	Kind                     AsyncQuestionDeliveryKind
	DestinationTaskID        string
	DestinationSession       string
	DestinationIncarnationID string
	Content                  string
	Metadata                 map[string]interface{}
	State                    AsyncQuestionDeliveryState
	Attempts                 int
	NextAttemptAt            time.Time
	LastError                string
	CreatedAt                time.Time
	AdmittedAt               *time.Time
}

// AsyncQuestionWaitStatus is the explicit dependency gate phase.
type AsyncQuestionWaitStatus string

const (
	AsyncQuestionWaitRequested        AsyncQuestionWaitStatus = "requested"
	AsyncQuestionWaitReleaseRequested AsyncQuestionWaitStatus = "release_requested"
	AsyncQuestionWaitParked           AsyncQuestionWaitStatus = "parked"
	AsyncQuestionWaitReleased         AsyncQuestionWaitStatus = "released"
	AsyncQuestionWaitCancelled        AsyncQuestionWaitStatus = "cancelled"
)

// AsyncQuestionWait is one explicit dependency gate captured against a single
// conversational turn. Request existence alone never establishes a gate.
type AsyncQuestionWait struct {
	ID            string
	WorkspaceID   string
	TaskID        string
	SessionID     string
	TurnID        string
	DependencyIDs []string
	Status        AsyncQuestionWaitStatus
	Reason        string
	ReleasedBy    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ListAsyncQuestionsOptions filters bounded question reads. Every predicate
// is applied inside the query, never as a post-query filter.
type ListAsyncQuestionsOptions struct {
	// Sidecar applies the same per-user hide/snooze semantics as blocking
	// bundles. Only=true enumerates hidden rows.
	Sidecar *ClarificationSidecarFilter
	// RequireAnswerable restricts reads to a current, non-terminal destination
	// with a matching session incarnation and workspace.
	RequireAnswerable bool
	WorkspaceIDs      []string
	WorkspaceID       string
	TaskID            string
	SessionID         string
	// AuthorSessionID restricts reads to requests authored by exactly this
	// session (the checkpoint read).
	AuthorSessionID string
	// ParentTaskID restricts reads to direct children of this parent task.
	ParentTaskID string
	// IncludeUnforwardedParents controls whether parent-authored requests are
	// returned without an authorized forward. Operator and Inbox reads keep it
	// false; direct-parent discovery sets it true.
	IncludeUnforwardedParents bool
	Statuses                  []AsyncQuestionStatus
	// OpenOnly restricts to pending requests.
	OpenOnly bool
	// QuestionIDs optionally narrows to a bounded identity set.
	QuestionIDs     []string
	CursorCreatedAt time.Time
	CursorID        string
	Limit           int
}

// AsyncQuestionPage is one bounded page ordered by creation time then ID.
type AsyncQuestionPage struct {
	Questions []*AsyncQuestion
	HasMore   bool
}

// ErrAsyncQuestionStale reports a request whose current task/session/parent
// binding no longer authorizes an answer or forward.
var ErrAsyncQuestionStale = errors.New("async question binding is no longer current")

// Package questions implements the task-owned async question lifecycle.
// Agents ask early, keep working, read answers at checkpoints, and pause only
// through an explicit dependency wait. The package depends on narrow ports so
// it never imports the orchestrator or message queue directly.
package questions

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

var (
	// ErrNotFound reports an absent or inaccessible request.
	ErrNotFound = errors.New("async question not found")
	// ErrNotPending reports an attempted transition from a terminal state.
	ErrNotPending = errors.New("async question is not pending")
	// ErrValidation reports malformed question or answer content.
	ErrValidation = errors.New("async question validation failed")
	// ErrConflict reports a reused identity with different content.
	ErrConflict = errors.New("async question identity conflict")
	// ErrUnauthorized reports a caller that does not own the request binding.
	ErrUnauthorized = errors.New("async question operation is not authorized")
	// ErrWaitActive reports a second explicit wait for one session.
	ErrWaitActive = errors.New("an explicit question wait is already active")
	// ErrQueueUnavailable reports a delivery port that is not wired.
	ErrQueueUnavailable = errors.New("async question delivery queue is unavailable")
	// ErrTranscriptWriterUnavailable reports that transcript delivery is not wired.
	ErrTranscriptWriterUnavailable = errors.New("async question transcript writer is unavailable")
	// ErrDestinationTerminal reports a destination that can no longer accept
	// an answer. The intent is abandoned, never redirected.
	ErrDestinationTerminal = errors.New("async question destination is terminal")
	// ErrDestinationUnavailable reports a task destination with no active
	// session yet. The intent remains pending and is retried.
	ErrDestinationUnavailable = errors.New("async question destination is unavailable")
)

// Store is the durable repository port. Implementations must make resolution,
// outcome recording, and delivery insertion one transaction.
type Store interface {
	CreateAsyncQuestion(ctx context.Context, question *models.AsyncQuestion, deliveries []*models.AsyncQuestionDelivery) (*models.AsyncQuestion, bool, error)
	GetAsyncQuestion(ctx context.Context, id string) (*models.AsyncQuestion, error)
	ListAsyncQuestions(ctx context.Context, opts models.ListAsyncQuestionsOptions) (*models.AsyncQuestionPage, error)
	CountOpenAsyncQuestions(ctx context.Context, opts models.ListAsyncQuestionsOptions) (int, error)
	NextHiddenAsyncQuestionSnooze(ctx context.Context, workspaceID, userID string, now time.Time) (*time.Time, error)
	ResolveAsyncQuestion(ctx context.Context, id string, outcome models.AsyncQuestionOutcome, deliveries []*models.AsyncQuestionDelivery) (*models.AsyncQuestion, bool, error)
	ResolveAsyncQuestionWithWake(ctx context.Context, id string, outcome models.AsyncQuestionOutcome, deliveries []*models.AsyncQuestionDelivery, buildWake models.AsyncQuestionWakeBuilder) (*models.AsyncQuestion, bool, error)
	WithdrawAsyncQuestion(ctx context.Context, id, authorSessionID, reason string, deliveries []*models.AsyncQuestionDelivery, buildWake models.AsyncQuestionWakeBuilder) (*models.AsyncQuestion, bool, []*models.AsyncQuestionWait, error)
	ForwardAsyncQuestion(ctx context.Context, forward *models.AsyncQuestionForward) (*models.AsyncQuestion, error)
	ForwardAsyncQuestionWithWake(ctx context.Context, forward *models.AsyncQuestionForward, buildWake models.AsyncQuestionWakeBuilder) (*models.AsyncQuestion, error)
	CreateAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error
	ListAsyncQuestionDeliveries(ctx context.Context, now time.Time, limit int) ([]*models.AsyncQuestionDelivery, error)
	ListAsyncQuestionDeliveriesForQuestion(ctx context.Context, questionID string, kind models.AsyncQuestionDeliveryKind) ([]*models.AsyncQuestionDelivery, error)
	MarkAsyncQuestionDeliveryAdmitted(ctx context.Context, admissionID string, at time.Time) error
	MarkAsyncQuestionDeliveryFailed(ctx context.Context, admissionID string, attempts int, nextAttemptAt time.Time, message string) error
	MarkAsyncQuestionDeliveryAbandoned(ctx context.Context, admissionID, message string) error
	CreateAsyncQuestionWait(ctx context.Context, wait *models.AsyncQuestionWait) (*models.AsyncQuestionWait, error)
	GetActiveAsyncQuestionWait(ctx context.Context, sessionID string) (*models.AsyncQuestionWait, error)
	GetAsyncQuestionWait(ctx context.Context, waitID string) (*models.AsyncQuestionWait, error)
	ListAsyncQuestionWaits(ctx context.Context, statuses []models.AsyncQuestionWaitStatus, limit int) ([]*models.AsyncQuestionWait, error)
	SetAsyncQuestionWaitStatus(ctx context.Context, waitID string, status models.AsyncQuestionWaitStatus, releasedBy string) error
	SetAsyncQuestionWaitStatusFrom(ctx context.Context, waitID string, from, to models.AsyncQuestionWaitStatus, releasedBy string) (bool, error)
	CancelAsyncQuestionWaitsForSession(ctx context.Context, sessionID, reason string) (int, error)
	MarkStaleAsyncQuestionsForSession(ctx context.Context, sessionID, reason string) (int, error)
	StaleAsyncQuestionsForSession(ctx context.Context, sessionID, reason string, buildWake models.AsyncQuestionWakeBuilder) ([]*models.AsyncQuestion, error)
	ReconcileAsyncQuestionAuthority(ctx context.Context, limit int, buildWake models.AsyncQuestionWakeBuilder) ([]*models.AsyncQuestion, error)
	DeleteAsyncQuestionsForTask(ctx context.Context, taskID string) error
}

// DeliveryQueue admits one immutable continuation intent with its stored
// identity. Implementations must return nil only after the exact admission
// receipt exists.
type DeliveryQueue interface {
	AdmitAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error
}

// DeliveryQueueRetirer removes a parent prompt only while its exact queue
// admission remains undispatched.
type DeliveryQueueRetirer interface {
	RemoveAsyncQuestionDelivery(ctx context.Context, delivery *models.AsyncQuestionDelivery) error
}

// TranscriptWriter creates the durable chat entry for an async request.
// Implementations use the question identity to make retries idempotent.
type TranscriptWriter interface {
	WriteAsyncQuestionTranscriptEntry(
		ctx context.Context,
		delivery *models.AsyncQuestionDelivery,
		question *models.AsyncQuestion,
	) error
}

// Notifier publishes a workspace-scoped question change after commit. It is
// optional: Inbox reads and periodic refresh still converge without it.
type Notifier interface {
	AsyncQuestionChanged(ctx context.Context, change QuestionChange)
}

// QuestionChange carries task routing identity for bounded workspace events.
type QuestionChange struct {
	WorkspaceID  string
	TaskID       string
	ParentTaskID string
	SessionID    string
	Recipient    models.AsyncQuestionRecipient
	Kind         string
	QuestionID   string
}

// SessionStateReader reports whether a wait destination is still answerable.
type SessionStateReader interface {
	AsyncQuestionSessionState(ctx context.Context, sessionID string) (state string, archived bool, found bool, err error)
}

// TaskLookup resolves the immutable task/session binding a request needs.
type TaskLookup interface {
	AsyncQuestionTaskBinding(ctx context.Context, sessionID string) (*TaskBinding, error)
}

// TaskBinding is the stored task ownership of one session.
type TaskBinding struct {
	TaskID               string
	WorkspaceID          string
	SessionIncarnationID string
	ParentTaskID         string
	ParentSessionID      string
	ParentAutopilot      bool
}

// AnswerInput is one structured operator or parent answer.
type AnswerInput struct {
	Answers      []Answer
	Rejected     bool
	RejectReason string
	SubmissionID string
	// Text is a bundle-level free-text answer used by direct parent replies.
	Text        string
	Responder   models.AsyncQuestionResponderKind
	ResponderID string
}

// Answer mirrors the wire answer type without importing clarification into
// the durable model layer.
type Answer struct {
	QuestionID      string
	SelectedOptions []string
	CustomText      string
}

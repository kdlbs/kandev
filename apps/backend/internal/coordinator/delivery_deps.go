package coordinator

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
)

// ConversationReader is the read surface admission uses on the coordinator's
// conversation session. A read error is never "absent".
type ConversationReader interface {
	// PrimarySession returns the task's primary session, nil when it has none.
	PrimarySession(ctx context.Context, taskID string) (*ConversationSession, error)
	// SessionState returns a session's persisted state; exists is false when the session is gone.
	SessionState(ctx context.Context, sessionID string) (state string, exists bool, err error)
	// ActionPending reports whether the session has a pending clarification or permission.
	ActionPending(ctx context.Context, sessionID string) (bool, error)
	// Queued reports whether the session has a queued message.
	Queued(ctx context.Context, sessionID string) (bool, error)
	// Turn returns a session turn, nil when it no longer exists.
	Turn(ctx context.Context, turnID string) (*TurnInfo, error)
}

// TurnInfo is the part of a session turn the missed-settle check reads.
type TurnInfo struct {
	Completed bool
}

// ConversationSession is a session id and its persisted state.
type ConversationSession struct {
	ID    string
	State string
}

// WakeMessage identifies a stored wake message.
type WakeMessage struct {
	ID     string
	TurnID string
}

// WakeMessageFinder finds and marks wake messages and completes an orphan
// turn. Satisfied by an adapter over the message store and the orchestrator.
type WakeMessageFinder interface {
	// FindWakeMessage returns the message whose wake turn id equals wakeTurnID,
	// created at or after since, or nil when there is none.
	FindWakeMessage(ctx context.Context, sessionID, wakeTurnID string, since time.Time) (*WakeMessage, error)
	MarkOrphan(ctx context.Context, sessionID, messageID string) error
	CompleteOrphanTurn(ctx context.Context, sessionID, turnID string) error
}

// WakeSender sends the wake message into the conversation. Satisfied by the
// orchestrator service.
type WakeSender interface {
	PromptUnattendedWake(ctx context.Context, in orchestrator.UnattendedWakePrompt) (string, error)
}

// SetDeliveryDeps wires the conversation reader, the message finder and the
// wake sender that admission and delivery use.
func (s *Service) SetDeliveryDeps(reader ConversationReader, finder WakeMessageFinder, sender WakeSender) {
	s.convReader = reader
	s.wakeFinder = finder
	s.wakeSender = sender
}

package messagequeue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Service manages queued messages for sessions.
// Uses in-memory storage as queued messages are best-effort and transient.
type Service struct {
	// In-memory storage: sessionID -> QueuedMessage
	queued map[string]*QueuedMessage
	// In-memory storage: sessionID -> PendingMove (deferred move_task_kandev moves)
	pendingMoves map[string]*PendingMove
	mu           sync.RWMutex
	logger       *logger.Logger
}

// NewService creates a new message queue service
func NewService(log *logger.Logger) *Service {
	return &Service{
		queued:       make(map[string]*QueuedMessage),
		pendingMoves: make(map[string]*PendingMove),
		logger:       log.WithFields(zap.String("component", "message-queue")),
	}
}

// TransferSession moves any queued message and pending move from one session
// to another. Used by workflow session switches (when a target step has a
// different agent profile and switchSessionForStep creates a new session) so
// a prompt queued by move_task_kandev on the old session reaches the new one.
// Existing entries on the destination session are preserved if no source entry
// is present; otherwise the source entry replaces the destination entry.
func (s *Service) TransferSession(ctx context.Context, oldSessionID, newSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg, ok := s.queued[oldSessionID]; ok {
		msg.SessionID = newSessionID
		s.queued[newSessionID] = msg
		delete(s.queued, oldSessionID)
		s.logger.Info("transferred queued message between sessions",
			zap.String("from_session_id", oldSessionID),
			zap.String("to_session_id", newSessionID),
			zap.String("queue_id", msg.ID))
	}

	if move, ok := s.pendingMoves[oldSessionID]; ok {
		s.pendingMoves[newSessionID] = move
		delete(s.pendingMoves, oldSessionID)
		s.logger.Info("transferred pending move between sessions",
			zap.String("from_session_id", oldSessionID),
			zap.String("to_session_id", newSessionID))
	}
}

// SetPendingMove records a pending move for a session (replaces any existing one).
// The move is applied by handleAgentReady when the agent's current turn completes.
func (s *Service) SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) {
	s.mu.Lock()
	defer s.mu.Unlock()
	move.QueuedAt = time.Now()
	s.pendingMoves[sessionID] = move
	s.logger.Info("pending move recorded",
		zap.String("session_id", sessionID),
		zap.String("task_id", move.TaskID),
		zap.String("workflow_step_id", move.WorkflowStepID))
}

// TakePendingMove retrieves and removes the pending move for a session.
func (s *Service) TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	move, exists := s.pendingMoves[sessionID]
	if !exists {
		return nil, false
	}
	delete(s.pendingMoves, sessionID)
	return move, true
}

// QueueMessage queues a message for a session (replaces existing queued message)
func (s *Service) QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := &QueuedMessage{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		QueuedAt:    time.Now(),
		QueuedBy:    userID,
	}

	s.queued[sessionID] = msg
	s.logger.Info("message queued",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.Int("content_length", len(content)))

	return msg, nil
}

// TakeQueued retrieves and removes the queued message for a session
func (s *Service) TakeQueued(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, exists := s.queued[sessionID]
	if !exists {
		return nil, false
	}

	delete(s.queued, sessionID)
	s.logger.Info("message dequeued",
		zap.String("session_id", sessionID),
		zap.String("queue_id", msg.ID))

	return msg, true
}

// CancelQueued removes a queued message without consuming it
func (s *Service) CancelQueued(ctx context.Context, sessionID string) (*QueuedMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, exists := s.queued[sessionID]
	if !exists {
		return nil, fmt.Errorf("no queued message for session %s", sessionID)
	}

	delete(s.queued, sessionID)
	s.logger.Info("message queue cancelled",
		zap.String("session_id", sessionID),
		zap.String("queue_id", msg.ID))

	return msg, nil
}

// GetStatus returns the queue status for a session
func (s *Service) GetStatus(ctx context.Context, sessionID string) *QueueStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, exists := s.queued[sessionID]
	return &QueueStatus{
		IsQueued: exists,
		Message:  msg,
	}
}

// AppendContent appends content to an existing queued message.
// Returns an error if no message is queued (caller should use QueueMessage instead).
func (s *Service) AppendContent(ctx context.Context, sessionID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, exists := s.queued[sessionID]
	if !exists {
		return fmt.Errorf("no queued message for session %s", sessionID)
	}

	msg.Content = msg.Content + "\n\n---\n\n" + content
	s.logger.Info("content appended to queued message",
		zap.String("session_id", sessionID),
		zap.Int("appended_length", len(content)),
		zap.Int("total_length", len(msg.Content)))

	return nil
}

// UpdateMessage updates an existing queued message (for arrow up editing)
func (s *Service) UpdateMessage(ctx context.Context, sessionID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, exists := s.queued[sessionID]
	if !exists {
		return fmt.Errorf("no queued message for session %s", sessionID)
	}

	msg.Content = content
	s.logger.Info("queued message updated",
		zap.String("session_id", sessionID),
		zap.Int("new_length", len(content)))

	return nil
}

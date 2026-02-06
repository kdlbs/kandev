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
	mu     sync.RWMutex
	logger *logger.Logger
}

// NewService creates a new message queue service
func NewService(log *logger.Logger) *Service {
	return &Service{
		queued: make(map[string]*QueuedMessage),
		logger: log.WithFields(zap.String("component", "message-queue")),
	}
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

// Package messagequeue persists per-session FIFO message queues and deferred
// workflow moves. The queue lets a user (or another agent via the
// message_task_kandev MCP tool) enqueue follow-up prompts while the target
// session is still busy with a turn; handleAgentReady drains one entry per
// turn after each turn completes.
//
// Storage is abstracted behind Repository (SQLite in production, in-memory
// for tests). The service enforces a per-session cap (default 10) and surfaces
// overflow as ErrQueueFull rather than silently dropping the new message.
package messagequeue

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// Service manages queued messages for sessions, backed by Repository.
type Service struct {
	repo          Repository
	maxPerSession int
	logger        *logger.Logger
}

// NewService creates a Service backed by the supplied repository. maxPerSession
// is the per-session cap (entries beyond this return ErrQueueFull on insert);
// pass 0 to disable the cap.
func NewService(repo Repository, maxPerSession int, log *logger.Logger) *Service {
	return &Service{
		repo:          repo,
		maxPerSession: maxPerSession,
		logger:        log.WithFields(zap.String("component", "message-queue")),
	}
}

// NewServiceMemory returns a Service backed by an in-memory repository.
// Convenience constructor for tests; cap defaults to 10 for parity with prod.
func NewServiceMemory(log *logger.Logger) *Service {
	return NewService(NewMemoryRepository(), DefaultMaxPerSession, log)
}

// MaxPerSession returns the configured per-session cap.
func (s *Service) MaxPerSession() int { return s.maxPerSession }

// QueueMessage appends a new entry to the session's FIFO queue. Returns
// ErrQueueFull when the cap is exceeded.
func (s *Service) QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, error) {
	return s.QueueMessageWithMetadata(ctx, sessionID, taskID, content, model, userID, planMode, attachments, nil)
}

// QueueMessageWithMetadata is like QueueMessage but stores extra metadata that
// is propagated to the resulting Message row when the queued message is
// drained (e.g. sender_task_id for messages sent via message_task_kandev).
func (s *Service) QueueMessageWithMetadata(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}) (*QueuedMessage, error) {
	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadata,
		QueuedBy:    userID,
	}
	if err := s.repo.Insert(ctx, msg, s.maxPerSession); err != nil {
		if errors.Is(err, ErrQueueFull) {
			s.logger.Info("queue full",
				zap.String("session_id", sessionID),
				zap.Int("max", s.maxPerSession))
		}
		return nil, err
	}
	s.logger.Info("message queued",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("entry_id", msg.ID),
		zap.Int64("position", msg.Position),
		zap.Int("content_length", len(content)))
	return msg, nil
}

// AppendContent appends content onto the session's tail entry when the tail's
// queued_by matches userID. Otherwise inserts a new entry. Returns
// ErrQueueFull when an insert would exceed the cap.
func (s *Service) AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, bool, error) {
	msg, appended, err := s.repo.AppendOrInsertTail(ctx, sessionID, taskID, content, model, userID, planMode, attachments, nil, s.maxPerSession)
	if err != nil {
		return nil, false, err
	}
	s.logger.Info("queue append-or-insert",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID),
		zap.Bool("appended", appended))
	return msg, appended, nil
}

// TakeQueued atomically removes and returns the head entry. Returns nil, false
// when the queue is empty.
func (s *Service) TakeQueued(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	msg, err := s.repo.TakeHead(ctx, sessionID)
	if err != nil {
		s.logger.Error("take head failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if msg == nil {
		return nil, false
	}
	s.logger.Info("message dequeued",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID))
	return msg, true
}

// UpdateMessage replaces the content (and optionally attachments) of a queued
// entry. Returns ErrEntryNotFound when the entry was already drained or
// when the queuedBy guard rejects the caller.
func (s *Service) UpdateMessage(ctx context.Context, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	if err := s.repo.UpdateContent(ctx, entryID, content, attachments, queuedBy); err != nil {
		return err
	}
	s.logger.Info("queued entry updated", zap.String("entry_id", entryID))
	return nil
}

// RemoveEntry deletes a single entry by id. Returns ErrEntryNotFound if absent.
func (s *Service) RemoveEntry(ctx context.Context, entryID string) error {
	if err := s.repo.DeleteByID(ctx, entryID); err != nil {
		return err
	}
	s.logger.Info("queued entry removed", zap.String("entry_id", entryID))
	return nil
}

// CancelAll clears every queued entry for a session. Returns the number of
// rows removed.
func (s *Service) CancelAll(ctx context.Context, sessionID string) (int, error) {
	n, err := s.repo.DeleteAllBySession(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	s.logger.Info("queue cancel-all",
		zap.String("session_id", sessionID),
		zap.Int("removed", n))
	return n, nil
}

// GetStatus returns the full pending list and capacity info for a session.
func (s *Service) GetStatus(ctx context.Context, sessionID string) *QueueStatus {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		s.logger.Error("list queued failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return &QueueStatus{Entries: nil, Count: 0, Max: s.maxPerSession}
	}
	if entries == nil {
		entries = []QueuedMessage{}
	}
	return &QueueStatus{
		Entries: entries,
		Count:   len(entries),
		Max:     s.maxPerSession,
	}
}

// TransferSession moves any queued messages and pending move from one session
// to another. Used by workflow session switches (when a target step has a
// different agent profile and switchSessionForStep creates a new session).
func (s *Service) TransferSession(ctx context.Context, oldSessionID, newSessionID string) {
	if err := s.repo.TransferSession(ctx, oldSessionID, newSessionID); err != nil {
		s.logger.Error("transfer session failed",
			zap.String("from_session_id", oldSessionID),
			zap.String("to_session_id", newSessionID),
			zap.Error(err))
		return
	}
	s.logger.Info("transferred queue between sessions",
		zap.String("from_session_id", oldSessionID),
		zap.String("to_session_id", newSessionID))
}

// SetPendingMove records a pending move for a session (replaces any existing one).
// The move is applied by handleAgentReady when the agent's current turn completes.
func (s *Service) SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) {
	if err := s.repo.SetPendingMove(ctx, sessionID, move); err != nil {
		s.logger.Error("set pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	s.logger.Info("pending move recorded",
		zap.String("session_id", sessionID),
		zap.String("task_id", move.TaskID),
		zap.String("workflow_step_id", move.WorkflowStepID))
}

// TakePendingMove retrieves and removes the pending move for a session.
func (s *Service) TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, bool) {
	move, err := s.repo.TakePendingMove(ctx, sessionID)
	if err != nil {
		s.logger.Error("take pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if move == nil {
		return nil, false
	}
	return move, true
}

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
	"fmt"

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
	metadataCopy := copyMessageMetadata(metadata, 0)
	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadataCopy,
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

// RestoreMessage restores a dequeued message at its original FIFO position
// after a delivery failure. It preserves the entry's identity, position,
// queued_at time, and queued_by ownership.
func (s *Service) RestoreMessage(ctx context.Context, msg *QueuedMessage) (*QueuedMessage, error) {
	if msg == nil {
		return nil, errors.New("queued message is nil")
	}
	restored := *msg
	restored.Metadata = copyMessageMetadata(msg.Metadata, 0)
	if err := s.repo.Restore(ctx, &restored, s.maxPerSession); err != nil {
		return nil, err
	}
	s.logger.Info("message restored at original queue position",
		zap.String("session_id", restored.SessionID),
		zap.String("task_id", restored.TaskID),
		zap.String("entry_id", restored.ID),
		zap.Int64("position", restored.Position))
	return &restored, nil
}

// QueueMessageWithCoalesceKey replaces an existing pending entry with the same
// coalesce key, session, and queued_by value. When no matching entry exists it
// inserts a new tail entry if allowInsert is true; otherwise ErrEntryNotFound is
// returned. The returned bool is true when an existing entry was replaced.
func (s *Service) QueueMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, error) {
	metadataCopy := copyMessageMetadata(metadata, 1)
	metadataCopy[MetadataCoalesceKey] = coalesceKey
	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadataCopy,
		QueuedBy:    userID,
	}
	queued, replaced, err := s.repo.InsertOrReplaceByCoalesceKey(ctx, msg, coalesceKey, s.maxPerSession, allowInsert)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			s.logger.Info("queue full",
				zap.String("session_id", sessionID),
				zap.Int("max", s.maxPerSession))
		}
		return nil, false, err
	}
	s.logger.Info("message queued with coalesce key",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("entry_id", queued.ID),
		zap.String("coalesce_key", coalesceKey),
		zap.Bool("replaced", replaced),
		zap.Int64("position", queued.Position),
		zap.Int("content_length", len(content)))
	return queued, replaced, nil
}

// QueueLifecycleMessageWithCoalesceKey accepts a lifecycle entry only while
// its task remains active. accepted is false for a normal archive/delete win.
func (s *Service) QueueLifecycleMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(ctx, sessionID, taskID, content, model, userID, planMode, attachments, metadata, coalesceKey, allowInsert, false)
}

// RequeueLifecycleMessageWithCoalesceKey preserves the generation captured by
// an existing durable row. This prevents a stale retry from becoming a new
// prompt after the task was archived and then unarchived.
func (s *Service) RequeueLifecycleMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(ctx, sessionID, taskID, content, model, userID, planMode, attachments, metadata, coalesceKey, allowInsert, true)
}

func (s *Service) queueLifecycleMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert, isRetry bool) (*QueuedMessage, bool, bool, error) {
	metadataCopy := clearReservedMetadata(metadata)
	generation, err := s.repo.LifecycleGeneration(ctx, taskID)
	if err != nil {
		return nil, false, false, err
	}
	if isRetry {
		// Rows written before generation metadata existed belong to generation
		// zero. They may retry while no purge occurred, but a later archive
		// advances generation and rejects them just like newer rows.
		expected, ok := lifecycleGenerationFromMetadata(metadataCopy)
		if !ok {
			expected = 0
		}
		if expected != generation {
			return nil, false, false, nil
		}
	}
	metadataCopy[MetadataCoalesceKey] = coalesceKey
	metadataCopy[MetadataLifecycleDurable] = true
	metadataCopy[MetadataLifecycleGeneration] = generation
	msg := &QueuedMessage{SessionID: sessionID, TaskID: taskID, Content: content, Model: model, PlanMode: planMode, Attachments: attachments, Metadata: metadataCopy, QueuedBy: userID}
	queued, replaced, err := s.repo.InsertOrReplaceLifecycleByCoalesceKey(ctx, msg, coalesceKey, s.maxPerSession, allowInsert)
	if errors.Is(err, ErrTaskInactive) || errors.Is(err, ErrLifecycleCancelled) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return queued, replaced, true, nil
}

// PurgeTask is a backend-only task lifecycle operation. Client deletion APIs
// retain their reserved-entry protections.
func (s *Service) PurgeTask(ctx context.Context, taskID string) (int, error) {
	return s.repo.PurgeTask(ctx, taskID)
}

func lifecycleGenerationFromMetadata(metadata map[string]interface{}) (int64, bool) {
	if metadata == nil {
		return 0, false
	}
	switch value := metadata[MetadataLifecycleGeneration].(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}

// ReserveQueued atomically takes an ordinary head entry or reserves a durable
// lifecycle head entry. A reserved lifecycle row survives until acknowledged.
func (s *Service) ReserveQueued(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	msg, err := s.repo.ReserveHead(ctx, sessionID)
	if err != nil {
		s.logger.Error("reserve head failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	return msg, msg != nil
}

// AcknowledgeQueued removes a server-reserved entry after prompt acceptance.
func (s *Service) AcknowledgeQueued(ctx context.Context, sessionID, entryID string) error {
	err := s.repo.AcknowledgeByID(ctx, sessionID, entryID)
	if errors.Is(err, ErrEntryNotFound) {
		return nil
	}
	return err
}

// IsCurrentLifecycleReservation verifies that msg is still the durable row
// accepted for the task's current lifecycle generation. A task archive/delete
// removes that row and advances the generation, so callers can discard a
// deferred reservation before it performs any prompt side effects.
func (s *Service) IsCurrentLifecycleReservation(ctx context.Context, msg *QueuedMessage) bool {
	if msg == nil || !msg.IsDurableLifecycle() {
		return false
	}
	expectedGeneration, ok := lifecycleGenerationFromMetadata(msg.Metadata)
	if !ok {
		expectedGeneration = 0
	}
	generation, err := s.repo.LifecycleGeneration(ctx, msg.TaskID)
	if err != nil || generation != expectedGeneration {
		return false
	}
	entries, err := s.repo.ListBySession(ctx, msg.SessionID)
	if err != nil {
		return false
	}
	for i := range entries {
		entry := &entries[i]
		if entry.ID == msg.ID && entry.TaskID == msg.TaskID && entry.IsDurableLifecycle() {
			return true
		}
	}
	return false
}

func copyMessageMetadata(metadata map[string]interface{}, extraCapacity int) map[string]interface{} {
	if len(metadata) == 0 && extraCapacity == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(metadata)+extraCapacity)
	for key, value := range metadata {
		out[key] = value
	}
	return out
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

// TakeQueuedEntry atomically removes and returns the entry identified by
// entryID, regardless of its FIFO position. Returns nil, false, nil when
// the entry no longer exists (already drained or removed by a concurrent
// path); returns a non-nil error on a genuine repository failure — the two
// are deliberately distinguished so InterruptForPeerMessage can treat "not
// found" (safe to fall back to a FIFO-head drain) differently from "the
// repository call itself failed" (unsafe to fall back: a transient error
// here says nothing about which entry is actually at the FIFO head, and
// falling back anyway risks dispatching the wrong message while still
// reporting "sent" for the parent's — see InterruptForPeerMessage). Used by
// InterruptForPeerMessage to dispatch the specific message that triggered
// the interrupt instead of whatever happens to be at the FIFO head.
func (s *Service) TakeQueuedEntry(ctx context.Context, sessionID, entryID string) (*QueuedMessage, bool, error) {
	msg, err := s.repo.TakeByID(ctx, sessionID, entryID)
	if err != nil {
		s.logger.Error("take by id failed",
			zap.String("session_id", sessionID),
			zap.String("entry_id", entryID),
			zap.Error(err))
		return nil, false, err
	}
	if msg == nil {
		return nil, false, nil
	}
	s.logger.Info("message dequeued by id",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID))
	return msg, true, nil
}

// UpdateMessage replaces the content (and optionally attachments) of a queued
// entry. The sessionID scope is mandatory — callers can't update an entry by
// guessing its UUID across sessions. Returns ErrEntryNotFound when the entry
// was already drained, the session doesn't own it, or the queuedBy guard
// rejects the caller.
func (s *Service) UpdateMessage(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	return s.UpdateMessageWithMetadata(ctx, sessionID, entryID, content, attachments, nil, queuedBy)
}

// UpdateMessageWithMetadata atomically edits queue content and applies
// metadata replacements while retaining unrelated metadata keys.
func (s *Service) UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	if err := s.repo.UpdateContentAndMetadata(ctx, sessionID, entryID, content, attachments, metadataUpdates, queuedBy); err != nil {
		return err
	}
	s.logger.Info("queued entry updated",
		zap.String("session_id", sessionID),
		zap.String("entry_id", entryID))
	return nil
}

// RemoveEntry deletes a single entry. The sessionID scope is mandatory — see
// the rationale on the Repository.DeleteByID contract for why. Returns
// ErrEntryNotFound when no entry matches.
func (s *Service) RemoveEntry(ctx context.Context, sessionID, entryID string) error {
	if err := s.repo.DeleteByID(ctx, sessionID, entryID); err != nil {
		return err
	}
	s.logger.Info("queued entry removed",
		zap.String("session_id", sessionID),
		zap.String("entry_id", entryID))
	return nil
}

// MergeIntoAbove folds the entry identified by entryID into the entry directly
// above it within the same session. See Repository.MergeIntoAbove for the merge
// rules and error mapping (ErrEntryNotFound / ErrNoMergeTarget).
func (s *Service) MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*QueuedMessage, error) {
	merged, err := s.repo.MergeIntoAbove(ctx, sessionID, entryID, queuedBy)
	if err != nil {
		return nil, err
	}
	s.logger.Info("queued entry merged into entry above",
		zap.String("session_id", sessionID),
		zap.String("entry_id", merged.ID))
	return merged, nil
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
		return &QueueStatus{Entries: []QueuedMessage{}, Count: 0, Max: s.maxPerSession}
	}
	pending := make([]QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		// A reserved lifecycle row is already being delivered; listing it next
		// to the message it produced reads as a stuck duplicate.
		if entry.IsReservedInFlight() {
			continue
		}
		pending = append(pending, entry)
	}
	return &QueueStatus{
		Entries: pending,
		Count:   len(pending),
		Max:     s.maxPerSession,
	}
}

// SnapshotSession returns the complete persisted queue state for rollback.
// Unlike GetStatus, it includes durable lifecycle rows reserved by an in-flight
// delivery so a later restore cannot erase them before acknowledgement.
func (s *Service) SnapshotSession(ctx context.Context, sessionID string) ([]QueuedMessage, *PendingMove, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot queued messages: %w", err)
	}
	move, err := s.repo.GetPendingMove(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot pending move: %w", err)
	}
	return entries, move, nil
}

// TransferSession moves any queued messages and pending move from one session
// to another. Used by workflow session switches. Returns an error so the
// caller can fail closed instead of silently leaving entries orphaned on the
// old session — a transfer that no-ops without a signal would let the workflow
// step move forward while the queue sticks behind.
func (s *Service) TransferSession(ctx context.Context, oldSessionID, newSessionID string) error {
	if err := s.repo.TransferSession(ctx, oldSessionID, newSessionID); err != nil {
		s.logger.Error("transfer session failed",
			zap.String("from_session_id", oldSessionID),
			zap.String("to_session_id", newSessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("transferred queue between sessions",
		zap.String("from_session_id", oldSessionID),
		zap.String("to_session_id", newSessionID))
	return nil
}

// RestoreSession replaces a session's queue and pending move from a snapshot,
// preserving queued-message identity fields.
func (s *Service) RestoreSession(ctx context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	if err := s.repo.ReplaceSession(ctx, sessionID, entries, pendingMove); err != nil {
		s.logger.Error("restore session queue failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("restored session queue",
		zap.String("session_id", sessionID),
		zap.Int("entries", len(entries)),
		zap.Bool("pending_move", pendingMove != nil))
	return nil
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

// GetPendingMove retrieves the pending move for a session without removing it.
func (s *Service) GetPendingMove(ctx context.Context, sessionID string) (*PendingMove, bool) {
	move, err := s.repo.GetPendingMove(ctx, sessionID)
	if err != nil {
		s.logger.Error("get pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if move == nil {
		return nil, false
	}
	return move, true
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

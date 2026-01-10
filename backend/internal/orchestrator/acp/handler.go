// Package acp handles ACP message aggregation, storage, and distribution.
package acp

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/acp/protocol"
	"go.uber.org/zap"
)

// MessageStore interface for storing ACP messages
type MessageStore interface {
	// Store saves an ACP message to the database
	Store(ctx context.Context, msg *protocol.Message) error

	// GetMessages retrieves messages for a task
	GetMessages(ctx context.Context, taskID string, limit int, since time.Time) ([]*protocol.Message, error)

	// GetLatestProgress retrieves the most recent progress for a task
	GetLatestProgress(ctx context.Context, taskID string) (*protocol.ProgressData, error)
}

// Handler manages ACP message processing
type Handler struct {
	store  MessageStore
	logger *logger.Logger

	// Per-task message buffers for aggregation
	buffers map[string]*messageBuffer
	mu      sync.RWMutex

	// Callbacks for real-time streaming
	listeners  map[string][]MessageListener
	listenerMu sync.RWMutex
}

// MessageListener is called when a new ACP message arrives for a task
type MessageListener func(msg *protocol.Message)

// messageBuffer holds recent messages for a task
type messageBuffer struct {
	taskID     string
	messages   []*protocol.Message
	maxSize    int
	lastUpdate time.Time
}

// NewHandler creates a new ACP handler
func NewHandler(store MessageStore, log *logger.Logger) *Handler {
	return &Handler{
		store:     store,
		logger:    log,
		buffers:   make(map[string]*messageBuffer),
		listeners: make(map[string][]MessageListener),
	}
}

// ProcessMessage handles an incoming ACP message
// It stores the message, updates buffers, and notifies listeners
func (h *Handler) ProcessMessage(ctx context.Context, msg *protocol.Message) error {
	// Store the message
	if err := h.store.Store(ctx, msg); err != nil {
		h.logger.Error("failed to store ACP message", zap.Error(err), zap.String("task_id", msg.TaskID))
		return err
	}

	// Update buffer
	h.mu.Lock()
	buf, exists := h.buffers[msg.TaskID]
	if !exists {
		buf = &messageBuffer{
			taskID:   msg.TaskID,
			messages: make([]*protocol.Message, 0, 100),
			maxSize:  100,
		}
		h.buffers[msg.TaskID] = buf
	}
	buf.messages = append(buf.messages, msg)
	if len(buf.messages) > buf.maxSize {
		buf.messages = buf.messages[1:]
	}
	buf.lastUpdate = time.Now()
	h.mu.Unlock()

	// Notify listeners
	h.listenerMu.RLock()
	listeners := h.listeners[msg.TaskID]
	h.listenerMu.RUnlock()

	for _, listener := range listeners {
		listener(msg)
	}

	h.logger.Debug("processed ACP message", zap.String("task_id", msg.TaskID), zap.String("type", string(msg.Type)))
	return nil
}

// AddListener adds a listener for a specific task
// Returns a function to remove the listener
func (h *Handler) AddListener(taskID string, listener MessageListener) func() {
	h.listenerMu.Lock()
	h.listeners[taskID] = append(h.listeners[taskID], listener)
	h.listenerMu.Unlock()

	return func() {
		h.RemoveListener(taskID, listener)
	}
}

// RemoveListener removes a specific listener
func (h *Handler) RemoveListener(taskID string, listener MessageListener) {
	h.listenerMu.Lock()
	defer h.listenerMu.Unlock()

	listeners := h.listeners[taskID]
	for i, l := range listeners {
		// Compare function pointers
		if &l == &listener {
			h.listeners[taskID] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
}

// GetRecentMessages returns recent messages from the buffer
func (h *Handler) GetRecentMessages(taskID string, limit int) []*protocol.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	buf, exists := h.buffers[taskID]
	if !exists {
		return nil
	}

	messages := buf.messages
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	// Return a copy to avoid race conditions
	result := make([]*protocol.Message, len(messages))
	copy(result, messages)
	return result
}

// GetTaskProgress returns the latest progress for a task
func (h *Handler) GetTaskProgress(taskID string) (*protocol.ProgressData, error) {
	return h.store.GetLatestProgress(context.Background(), taskID)
}

// CleanupTask removes all data for a completed task
func (h *Handler) CleanupTask(taskID string) {
	h.mu.Lock()
	delete(h.buffers, taskID)
	h.mu.Unlock()

	h.listenerMu.Lock()
	delete(h.listeners, taskID)
	h.listenerMu.Unlock()

	h.logger.Info("cleaned up task data", zap.String("task_id", taskID))
}


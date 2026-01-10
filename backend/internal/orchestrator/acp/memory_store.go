package acp

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/acp/protocol"
)

// MemoryMessageStore is an in-memory implementation of MessageStore
type MemoryMessageStore struct {
	messages   map[string][]*protocol.Message
	mu         sync.RWMutex
	maxPerTask int
}

// NewMemoryMessageStore creates a new in-memory message store
func NewMemoryMessageStore(maxPerTask int) *MemoryMessageStore {
	if maxPerTask <= 0 {
		maxPerTask = 1000
	}
	return &MemoryMessageStore{
		messages:   make(map[string][]*protocol.Message),
		maxPerTask: maxPerTask,
	}
}

// Store saves an ACP message to the in-memory store
func (s *MemoryMessageStore) Store(ctx context.Context, msg *protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.messages[msg.TaskID]
	messages = append(messages, msg)

	// Trim if exceeding max
	if len(messages) > s.maxPerTask {
		messages = messages[len(messages)-s.maxPerTask:]
	}

	s.messages[msg.TaskID] = messages
	return nil
}

// GetMessages retrieves messages for a task
func (s *MemoryMessageStore) GetMessages(ctx context.Context, taskID string, limit int, since time.Time) ([]*protocol.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := s.messages[taskID]
	if messages == nil {
		return []*protocol.Message{}, nil
	}

	// Filter by time if specified
	var filtered []*protocol.Message
	for _, msg := range messages {
		if msg.Timestamp.After(since) {
			filtered = append(filtered, msg)
		}
	}

	// Apply limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	// Return a copy
	result := make([]*protocol.Message, len(filtered))
	copy(result, filtered)
	return result, nil
}

// GetLatestProgress retrieves the most recent progress for a task
func (s *MemoryMessageStore) GetLatestProgress(ctx context.Context, taskID string) (*protocol.ProgressData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := s.messages[taskID]
	if messages == nil {
		return nil, nil
	}

	// Find the latest progress message (iterate backwards)
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Type == protocol.MessageTypeProgress {
			return extractProgressData(msg), nil
		}
	}

	return nil, nil
}

// extractProgressData extracts ProgressData from a message's Data map
func extractProgressData(msg *protocol.Message) *protocol.ProgressData {
	data := &protocol.ProgressData{}

	if progress, ok := msg.Data["progress"].(float64); ok {
		data.Progress = int(progress)
	} else if progress, ok := msg.Data["progress"].(int); ok {
		data.Progress = progress
	}

	if message, ok := msg.Data["message"].(string); ok {
		data.Message = message
	}

	if currentFile, ok := msg.Data["current_file"].(string); ok {
		data.CurrentFile = currentFile
	}

	if filesProcessed, ok := msg.Data["files_processed"].(float64); ok {
		data.FilesProcessed = int(filesProcessed)
	} else if filesProcessed, ok := msg.Data["files_processed"].(int); ok {
		data.FilesProcessed = filesProcessed
	}

	if totalFiles, ok := msg.Data["total_files"].(float64); ok {
		data.TotalFiles = int(totalFiles)
	} else if totalFiles, ok := msg.Data["total_files"].(int); ok {
		data.TotalFiles = totalFiles
	}

	return data
}

// Delete removes all messages for a task
func (s *MemoryMessageStore) Delete(ctx context.Context, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, taskID)
	return nil
}


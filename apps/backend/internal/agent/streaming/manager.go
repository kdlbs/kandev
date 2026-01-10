package streaming

import (
	"context"
	"fmt"
	"io"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// Manager manages ACP stream readers for multiple agent instances
type Manager struct {
	eventBus bus.EventBus
	logger   *logger.Logger

	readers map[string]*StreamReader // by instanceID
	mu      sync.RWMutex
}

// NewManager creates a new streaming manager
func NewManager(eventBus bus.EventBus, log *logger.Logger) *Manager {
	return &Manager{
		eventBus: eventBus,
		logger:   log.WithFields(zap.String("component", "streaming-manager")),
		readers:  make(map[string]*StreamReader),
	}
}

// StartStreaming starts streaming for an agent instance
func (m *Manager) StartStreaming(ctx context.Context, instanceID, taskID string, reader io.ReadCloser) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already streaming for this instance
	if existing, exists := m.readers[instanceID]; exists {
		if existing.IsRunning() {
			return fmt.Errorf("streaming already active for instance %s", instanceID)
		}
		// Previous reader exists but not running, replace it
	}

	// Create new stream reader
	streamReader := NewStreamReader(instanceID, taskID, m.eventBus, m.logger)
	m.readers[instanceID] = streamReader

	// Start reading
	if err := streamReader.Start(ctx, reader); err != nil {
		delete(m.readers, instanceID)
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	m.logger.Info("started streaming for instance",
		zap.String("instance_id", instanceID),
		zap.String("task_id", taskID))

	return nil
}

// StopStreaming stops streaming for an agent instance
func (m *Manager) StopStreaming(instanceID string) error {
	m.mu.Lock()
	reader, exists := m.readers[instanceID]
	if !exists {
		m.mu.Unlock()
		return nil // Not streaming, no error
	}
	delete(m.readers, instanceID)
	m.mu.Unlock()

	if err := reader.Stop(); err != nil {
		m.logger.Warn("error stopping stream reader",
			zap.String("instance_id", instanceID),
			zap.Error(err))
		return err
	}

	m.logger.Info("stopped streaming for instance",
		zap.String("instance_id", instanceID))

	return nil
}

// StopAll stops all stream readers
func (m *Manager) StopAll() {
	m.mu.Lock()
	readers := make([]*StreamReader, 0, len(m.readers))
	instanceIDs := make([]string, 0, len(m.readers))
	for id, reader := range m.readers {
		readers = append(readers, reader)
		instanceIDs = append(instanceIDs, id)
	}
	m.readers = make(map[string]*StreamReader)
	m.mu.Unlock()

	// Stop all readers outside the lock
	for i, reader := range readers {
		if err := reader.Stop(); err != nil {
			m.logger.Warn("error stopping stream reader during shutdown",
				zap.String("instance_id", instanceIDs[i]),
				zap.Error(err))
		}
	}

	m.logger.Info("stopped all stream readers", zap.Int("count", len(readers)))
}

// IsStreaming returns true if streaming is active for an instance
func (m *Manager) IsStreaming(instanceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reader, exists := m.readers[instanceID]
	if !exists {
		return false
	}
	return reader.IsRunning()
}

// GetActiveCount returns the number of active stream readers
func (m *Manager) GetActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, reader := range m.readers {
		if reader.IsRunning() {
			count++
		}
	}
	return count
}

// GetReader returns the stream reader for an instance, if it exists
func (m *Manager) GetReader(instanceID string) (*StreamReader, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	reader, exists := m.readers[instanceID]
	return reader, exists
}


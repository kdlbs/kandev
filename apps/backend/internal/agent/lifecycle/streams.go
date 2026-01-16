package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

// StreamCallbacks defines callbacks for stream events
type StreamCallbacks struct {
	OnSessionUpdate func(instance *AgentInstance, update agentctl.SessionUpdate)
	OnPermission    func(instance *AgentInstance, notification *agentctl.PermissionNotification)
	OnGitStatus     func(instance *AgentInstance, update *agentctl.GitStatusUpdate)
	OnFileChange    func(instance *AgentInstance, notification *agentctl.FileChangeNotification)
}

// StreamManager manages WebSocket streams to agent instances
type StreamManager struct {
	logger    *logger.Logger
	callbacks StreamCallbacks
}

// NewStreamManager creates a new StreamManager
func NewStreamManager(log *logger.Logger, callbacks StreamCallbacks) *StreamManager {
	return &StreamManager{
		logger:    log.WithFields(zap.String("component", "stream-manager")),
		callbacks: callbacks,
	}
}

// ConnectAll connects to all streams for an instance.
// If ready is non-nil, it will be closed when the updates stream is connected.
func (sm *StreamManager) ConnectAll(instance *AgentInstance, ready chan<- struct{}) {
	go sm.connectUpdatesStream(instance, ready)
	go sm.connectPermissionStream(instance)
	go sm.connectGitStatusStream(instance)
	go sm.connectFileChangesStream(instance)
}

// ReconnectAll reconnects to all streams (used after backend restart).
// This waits for agentctl to be ready before connecting to streams.
func (sm *StreamManager) ReconnectAll(instance *AgentInstance) {
	sm.logger.Info("reconnecting to agent streams after recovery",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID))

	// Wait a moment for any startup operations to settle
	time.Sleep(500 * time.Millisecond)

	// Check if agentctl is responsive
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := instance.agentctl.WaitForReady(ctx, 10*time.Second); err != nil {
		sm.logger.Warn("agentctl not ready for stream reconnection",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		// Don't return - still try to connect to streams
	}

	// Reconnect to WebSocket streams
	sm.ConnectAll(instance, nil)

	sm.logger.Info("agent streams reconnected",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID))
}

// connectUpdatesStream handles the updates WebSocket stream with ready signaling
func (sm *StreamManager) connectUpdatesStream(instance *AgentInstance, ready chan<- struct{}) {
	ctx := context.Background()

	err := instance.agentctl.StreamUpdates(ctx, func(update agentctl.SessionUpdate) {
		if sm.callbacks.OnSessionUpdate != nil {
			sm.callbacks.OnSessionUpdate(instance, update)
		}
	})

	// Signal that the stream connection attempt is complete (success or failure)
	// StreamUpdates returns immediately after establishing the WebSocket connection
	// and starting the read goroutine, so this signals that we're ready to receive updates
	if ready != nil {
		close(ready)
	}

	if err != nil {
		sm.logger.Error("failed to connect to updates stream",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	}
}

// connectPermissionStream handles the permission WebSocket stream
func (sm *StreamManager) connectPermissionStream(instance *AgentInstance) {
	ctx := context.Background()

	err := instance.agentctl.StreamPermissions(ctx, func(notification *agentctl.PermissionNotification) {
		if sm.callbacks.OnPermission != nil {
			sm.callbacks.OnPermission(instance, notification)
		}
	})
	if err != nil {
		sm.logger.Error("failed to connect to permission stream",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	}
}

// connectGitStatusStream handles git status stream with retry logic
func (sm *StreamManager) connectGitStatusStream(instance *AgentInstance) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := instance.agentctl.StreamGitStatus(ctx, func(update *agentctl.GitStatusUpdate) {
			if sm.callbacks.OnGitStatus != nil {
				sm.callbacks.OnGitStatus(instance, update)
			}
		})

		if err == nil {
			// Connection closed normally
			return
		}

		sm.logger.Debug("git status stream connection failed, retrying",
			zap.String("instance_id", instance.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	sm.logger.Error("failed to connect to git status stream after retries",
		zap.String("instance_id", instance.ID),
		zap.Int("max_retries", maxRetries))
}

// connectFileChangesStream handles file changes stream with retry logic
func (sm *StreamManager) connectFileChangesStream(instance *AgentInstance) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := instance.agentctl.StreamFileChanges(ctx, func(notification *agentctl.FileChangeNotification) {
			if sm.callbacks.OnFileChange != nil {
				sm.callbacks.OnFileChange(instance, notification)
			}
		})

		if err == nil {
			// Connection closed normally
			return
		}

		sm.logger.Debug("file changes stream connection failed, retrying",
			zap.String("instance_id", instance.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	sm.logger.Error("failed to connect to file changes stream after retries",
		zap.String("instance_id", instance.ID),
		zap.Int("max_retries", maxRetries))
}

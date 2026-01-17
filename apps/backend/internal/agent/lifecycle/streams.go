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
	OnSessionUpdate func(execution *AgentExecution, update agentctl.SessionUpdate)
	OnPermission    func(execution *AgentExecution, notification *agentctl.PermissionNotification)
	OnGitStatus     func(execution *AgentExecution, update *agentctl.GitStatusUpdate)
	OnFileChange    func(execution *AgentExecution, notification *agentctl.FileChangeNotification)
}

// StreamManager manages WebSocket streams to agent executions
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

// ConnectAll connects to all streams for an execution.
// If ready is non-nil, it will be closed when the updates stream is connected.
func (sm *StreamManager) ConnectAll(execution *AgentExecution, ready chan<- struct{}) {
	go sm.connectUpdatesStream(execution, ready)
	go sm.connectPermissionStream(execution)
	go sm.connectGitStatusStream(execution)
	go sm.connectFileChangesStream(execution)
}

// ReconnectAll reconnects to all streams (used after backend restart).
// This waits for agentctl to be ready before connecting to streams.
func (sm *StreamManager) ReconnectAll(execution *AgentExecution) {
	sm.logger.Info("reconnecting to agent streams after recovery",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))

	// Wait a moment for any startup operations to settle
	time.Sleep(500 * time.Millisecond)

	// Check if agentctl is responsive
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := execution.agentctl.WaitForReady(ctx, 10*time.Second); err != nil {
		sm.logger.Warn("agentctl not ready for stream reconnection",
			zap.String("instance_id", execution.ID),
			zap.Error(err))
		// Don't return - still try to connect to streams
	}

	// Reconnect to WebSocket streams
	sm.ConnectAll(execution, nil)

	sm.logger.Info("agent streams reconnected",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))
}

// connectUpdatesStream handles the updates WebSocket stream with ready signaling
func (sm *StreamManager) connectUpdatesStream(execution *AgentExecution, ready chan<- struct{}) {
	ctx := context.Background()

	err := execution.agentctl.StreamUpdates(ctx, func(update agentctl.SessionUpdate) {
		if sm.callbacks.OnSessionUpdate != nil {
			sm.callbacks.OnSessionUpdate(execution, update)
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
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	}
}

// connectPermissionStream handles the permission WebSocket stream
func (sm *StreamManager) connectPermissionStream(execution *AgentExecution) {
	ctx := context.Background()

	sm.logger.Debug("connecting to permission stream",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))

	err := execution.agentctl.StreamPermissions(ctx, func(notification *agentctl.PermissionNotification) {
		sm.logger.Debug("received permission notification from stream",
			zap.String("pending_id", notification.PendingID),
			zap.String("title", notification.Title))
		if sm.callbacks.OnPermission != nil {
			sm.callbacks.OnPermission(execution, notification)
		}
	})
	if err != nil {
		sm.logger.Error("failed to connect to permission stream",
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	}
}

// connectGitStatusStream handles git status stream with retry logic
func (sm *StreamManager) connectGitStatusStream(execution *AgentExecution) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := execution.agentctl.StreamGitStatus(ctx, func(update *agentctl.GitStatusUpdate) {
			if sm.callbacks.OnGitStatus != nil {
				sm.callbacks.OnGitStatus(execution, update)
			}
		})

		if err == nil {
			// Connection closed normally
			return
		}

		sm.logger.Debug("git status stream connection failed, retrying",
			zap.String("instance_id", execution.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	sm.logger.Error("failed to connect to git status stream after retries",
		zap.String("instance_id", execution.ID),
		zap.Int("max_retries", maxRetries))
}

// connectFileChangesStream handles file changes stream with retry logic
func (sm *StreamManager) connectFileChangesStream(execution *AgentExecution) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := execution.agentctl.StreamFileChanges(ctx, func(notification *agentctl.FileChangeNotification) {
			if sm.callbacks.OnFileChange != nil {
				sm.callbacks.OnFileChange(execution, notification)
			}
		})

		if err == nil {
			// Connection closed normally
			return
		}

		sm.logger.Debug("file changes stream connection failed, retrying",
			zap.String("instance_id", execution.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	sm.logger.Error("failed to connect to file changes stream after retries",
		zap.String("instance_id", execution.ID),
		zap.Int("max_retries", maxRetries))
}

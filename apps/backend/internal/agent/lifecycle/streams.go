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
	OnAgentEvent    func(execution *AgentExecution, event agentctl.AgentEvent)
	OnGitStatus     func(execution *AgentExecution, update *agentctl.GitStatusUpdate)
	OnGitCommit     func(execution *AgentExecution, commit *agentctl.GitCommitNotification)
	OnGitReset      func(execution *AgentExecution, reset *agentctl.GitResetNotification)
	OnFileChange    func(execution *AgentExecution, notification *agentctl.FileChangeNotification)
	OnShellOutput   func(execution *AgentExecution, data string)
	OnShellExit     func(execution *AgentExecution, code int)
	OnProcessOutput func(execution *AgentExecution, output *agentctl.ProcessOutput)
	OnProcessStatus func(execution *AgentExecution, status *agentctl.ProcessStatusUpdate)
}

// StreamManager manages WebSocket streams to agent executions
type StreamManager struct {
	logger     *logger.Logger
	callbacks  StreamCallbacks
	mcpHandler agentctl.MCPHandler
}

// NewStreamManager creates a new StreamManager
func NewStreamManager(log *logger.Logger, callbacks StreamCallbacks, mcpHandler agentctl.MCPHandler) *StreamManager {
	return &StreamManager{
		logger:     log.WithFields(zap.String("component", "stream-manager")),
		callbacks:  callbacks,
		mcpHandler: mcpHandler,
	}
}

// ConnectAll connects to all streams for an execution.
// If ready is non-nil, it is closed when the updates stream connection attempt
// completes (success or failure). Agent operations require the updates stream;
// workspace stream readiness is handled independently.
func (sm *StreamManager) ConnectAll(execution *AgentExecution, ready chan<- struct{}) {
	go sm.connectUpdatesStream(execution, ready)
	go sm.connectWorkspaceStream(execution, nil)
}

// ReconnectAll reconnects to all streams (used after backend restart).
// This waits for agentctl to be ready before connecting to streams.
func (sm *StreamManager) ReconnectAll(execution *AgentExecution) {
	sm.logger.Debug("reconnecting to agent streams after recovery",
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

	sm.logger.Debug("agent streams reconnected",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))
}

// connectUpdatesStream handles the updates WebSocket stream with ready signaling
func (sm *StreamManager) connectUpdatesStream(execution *AgentExecution, ready chan<- struct{}) {
	ctx := execution.SessionTraceContext()

	err := execution.agentctl.StreamUpdates(ctx, func(event agentctl.AgentEvent) {
		if sm.callbacks.OnAgentEvent != nil {
			sm.callbacks.OnAgentEvent(execution, event)
		}
	}, sm.mcpHandler, func(disconnectErr error) {
		// WebSocket dropped — signal promptDoneCh so SendPrompt doesn't hang forever.
		// Only signal on unexpected errors (not normal close).
		if disconnectErr != nil {
			select {
			case execution.promptDoneCh <- PromptCompletionSignal{
				IsError: true,
				Error:   "agent stream disconnected: " + disconnectErr.Error(),
			}:
			default:
			}
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

// buildWorkspaceCallbacks creates the WorkspaceStreamCallbacks for a given execution,
// wiring each callback to the StreamManager's registered handlers.
func (sm *StreamManager) buildWorkspaceCallbacks(execution *AgentExecution) agentctl.WorkspaceStreamCallbacks {
	return agentctl.WorkspaceStreamCallbacks{
		OnShellOutput: func(data string) {
			if sm.callbacks.OnShellOutput != nil {
				sm.callbacks.OnShellOutput(execution, data)
			}
		},
		OnShellExit: func(code int) {
			if sm.callbacks.OnShellExit != nil {
				sm.callbacks.OnShellExit(execution, code)
			}
		},
		OnGitStatus: func(update *agentctl.GitStatusUpdate) {
			if sm.callbacks.OnGitStatus != nil {
				sm.callbacks.OnGitStatus(execution, update)
			}
		},
		OnGitCommit: func(commit *agentctl.GitCommitNotification) {
			if sm.callbacks.OnGitCommit != nil {
				sm.callbacks.OnGitCommit(execution, commit)
			}
		},
		OnGitReset: func(reset *agentctl.GitResetNotification) {
			if sm.callbacks.OnGitReset != nil {
				sm.callbacks.OnGitReset(execution, reset)
			}
		},
		OnFileChange: func(notification *agentctl.FileChangeNotification) {
			if sm.callbacks.OnFileChange != nil {
				sm.callbacks.OnFileChange(execution, notification)
			}
		},
		OnProcessOutput: func(output *agentctl.ProcessOutput) {
			if sm.callbacks.OnProcessOutput != nil {
				sm.callbacks.OnProcessOutput(execution, output)
			}
		},
		OnProcessStatus: func(status *agentctl.ProcessStatusUpdate) {
			if sm.callbacks.OnProcessStatus != nil {
				sm.callbacks.OnProcessStatus(execution, status)
			}
		},
		OnConnected: func() {
			sm.logger.Debug("workspace stream connected",
				zap.String("instance_id", execution.ID))
		},
		OnError: func(err string) {
			sm.logger.Debug("workspace stream error",
				zap.String("instance_id", execution.ID),
				zap.String("error", err))
		},
	}
}

// connectWorkspaceStream handles the unified workspace stream with retry logic
func (sm *StreamManager) connectWorkspaceStream(execution *AgentExecution, ready chan<- struct{}) {
	ctx := execution.SessionTraceContext()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second
	signaled := false

	// Helper to signal ready (only once)
	signalReady := func() {
		if !signaled && ready != nil {
			close(ready)
			signaled = true
		}
	}

	// Ensure we signal ready even on failure (so callers don't hang)
	defer signalReady()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		callbacks := sm.buildWorkspaceCallbacks(execution)

		ws, err := execution.agentctl.StreamWorkspace(ctx, callbacks)
		if err != nil {
			sm.logger.Debug("workspace stream connection failed, retrying",
				zap.String("instance_id", execution.ID),
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))

			if attempt < maxRetries {
				time.Sleep(backoff)
				backoff *= 2 // Exponential backoff
			}
			continue
		}

		// Store the workspace stream on the execution for shell I/O
		execution.SetWorkspaceStream(ws)
		sm.logger.Debug("connected to unified workspace stream",
			zap.String("instance_id", execution.ID))

		// Signal that workspace stream is ready
		signalReady()

		// Wait for the stream to close (it stays open until disconnected)
		<-ws.Done()
		return
	}

	sm.logger.Error("failed to connect to workspace stream after retries",
		zap.String("instance_id", execution.ID),
		zap.Int("max_retries", maxRetries))
}

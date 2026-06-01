package lifecycle

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

// StreamCallbacks defines callbacks for stream events
type StreamCallbacks struct {
	OnAgentEvent       func(execution *AgentExecution, event agentctl.AgentEvent)
	OnStreamDisconnect func(execution *AgentExecution, err error)
	OnGitStatus        func(execution *AgentExecution, update *agentctl.GitStatusUpdate)
	OnGitCommit        func(execution *AgentExecution, commit *agentctl.GitCommitNotification)
	OnGitReset         func(execution *AgentExecution, reset *agentctl.GitResetNotification)
	OnBranchSwitch     func(execution *AgentExecution, branchSwitch *agentctl.GitBranchSwitchNotification)
	OnFileChange       func(execution *AgentExecution, notification *agentctl.FileChangeNotification)
	OnShellOutput      func(execution *AgentExecution, data string)
	OnShellExit        func(execution *AgentExecution, code int)
	OnProcessOutput    func(execution *AgentExecution, output *agentctl.ProcessOutput)
	OnProcessStatus    func(execution *AgentExecution, status *agentctl.ProcessStatusUpdate)
}

// StreamManager manages WebSocket streams to agent executions
type StreamManager struct {
	logger     *logger.Logger
	callbacks  StreamCallbacks
	mcpHandler agentctl.MCPHandler
	// stopCh is the Manager-owned shutdown signal. SessionTraceContext is
	// deliberately uncancellable (it carries a long-lived span), so the
	// reconnect/backoff loops below select on stopCh to drain on Manager.Stop.
	// nil is treated as "no shutdown signal" and falls back to the prior
	// uncancellable behaviour (compat with constructors used by isolated tests).
	stopCh  <-chan struct{}
	wg      sync.WaitGroup
	wgMu    sync.Mutex
	stopped bool
}

type stopChannelContext struct {
	context.Context
	stopCh <-chan struct{}
}

func (c stopChannelContext) Done() <-chan struct{} {
	if c.stopCh == nil {
		return c.Context.Done()
	}
	return c.stopCh
}

func (c stopChannelContext) Err() error {
	select {
	case <-c.stopCh:
		return context.Canceled
	default:
		return c.Context.Err()
	}
}

// NewStreamManager creates a new StreamManager.
//
// stopCh is the Manager-owned shutdown signal used by the workspace-stream
// retry backoff to drain cleanly. Pass nil from tests that exercise the
// manager in isolation; production callers wire it from Manager.stopCh.
func NewStreamManager(log *logger.Logger, callbacks StreamCallbacks, mcpHandler agentctl.MCPHandler, stopCh <-chan struct{}) *StreamManager {
	return &StreamManager{
		logger:     log.WithFields(zap.String("component", "stream-manager")),
		callbacks:  callbacks,
		mcpHandler: mcpHandler,
		stopCh:     stopCh,
	}
}

// ConnectAll connects to all streams for an execution.
// If ready is non-nil, it is closed when the updates stream connection attempt
// completes (success or failure). Agent operations require the updates stream;
// workspace stream readiness is handled independently.
func (sm *StreamManager) ConnectAll(execution *AgentExecution, ready chan<- struct{}) {
	sm.connectUpdatesStreamAsync(execution, ready)
	sm.ConnectWorkspaceStream(execution, nil)
}

func (sm *StreamManager) connectUpdatesStreamAsync(execution *AgentExecution, ready chan<- struct{}) {
	if !sm.start(func() {
		sm.connectUpdatesStream(execution, ready)
	}) && ready != nil {
		close(ready)
	}
}

// ConnectWorkspaceStream starts the workspace stream and tracks the goroutine
// so shutdown and tests can wait for it to drain after stopCh closes.
func (sm *StreamManager) ConnectWorkspaceStream(execution *AgentExecution, ready chan<- struct{}) {
	if !sm.start(func() {
		sm.connectWorkspaceStream(execution, ready)
	}) && ready != nil {
		close(ready)
	}
}

// ConnectMCPStream opens the passthrough MCP proxy stream under goroutine
// tracking so it drains cleanly on shutdown (mirrors ConnectWorkspaceStream).
func (sm *StreamManager) ConnectMCPStream(execution *AgentExecution) {
	sm.start(func() {
		sm.connectMCPStream(execution)
	})
}

// Wait blocks until all StreamManager-owned stream goroutines have exited.
func (sm *StreamManager) Wait() {
	sm.wgMu.Lock()
	sm.stopped = true
	sm.wgMu.Unlock()
	sm.wg.Wait()
}

func (sm *StreamManager) start(fn func()) bool {
	sm.wgMu.Lock()
	defer sm.wgMu.Unlock()
	if sm.stopped {
		return false
	}
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		fn()
	}()
	return true
}

// ReconnectAll reconnects to all streams (used after backend restart).
// This waits for agentctl to be ready before connecting to streams.
func (sm *StreamManager) ReconnectAll(execution *AgentExecution) {
	sm.logger.Debug("reconnecting to agent streams after recovery",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))

	// Wait a moment for any startup operations to settle. Selecting on
	// stopCh lets shutdown drain this goroutine without burning the full
	// 500ms when the manager is already stopping.
	if !sm.sleepOrStop(500 * time.Millisecond) {
		return
	}

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

// sleepOrStop blocks for d or until the Manager begins shutting down.
// Returns true when the timer fires, false when stopCh closes first.
// A nil stopCh degrades to time.Sleep semantics (always returns true).
func (sm *StreamManager) sleepOrStop(d time.Duration) bool {
	if sm.stopCh == nil {
		time.Sleep(d)
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-sm.stopCh:
		return false
	}
}

// streamContext preserves the execution's session trace values while making
// in-flight WebSocket dials cancellable by the manager shutdown signal.
func (sm *StreamManager) streamContext(execution *AgentExecution) context.Context {
	ctx := execution.SessionTraceContext()
	if sm.stopCh == nil {
		return ctx
	}
	return stopChannelContext{Context: ctx, stopCh: sm.stopCh}
}

// connectUpdatesStream handles the updates WebSocket stream with ready signaling
func (sm *StreamManager) connectUpdatesStream(execution *AgentExecution, ready chan<- struct{}) {
	ctx := sm.streamContext(execution)

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
			// Notify lifecycle manager so it can proactively update execution status
			if sm.callbacks.OnStreamDisconnect != nil {
				sm.callbacks.OnStreamDisconnect(execution, disconnectErr)
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

// connectMCPStream opens the agent updates WebSocket for a PASSTHROUGH session
// purely to drain the MCP request channel: the agentctl instance serves /mcp and
// proxies tool calls to the backend over this stream, so without it kandev MCP
// tool calls hang. Passthrough agents don't speak ACP, so no agent events arrive
// here (the PTY drives the UI). On disconnect it only logs — it must NOT signal
// promptDoneCh or OnStreamDisconnect (which would mark the execution failed);
// passthrough completion is detected via PTY idle, and a normal session end
// closing this stream is expected, not an error.
func (sm *StreamManager) connectMCPStream(execution *AgentExecution) {
	ctx := sm.streamContext(execution)
	err := execution.agentctl.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, sm.mcpHandler, func(disconnectErr error) {
		if disconnectErr != nil {
			sm.logger.Debug("passthrough MCP stream disconnected",
				zap.String("execution_id", execution.ID),
				zap.Error(disconnectErr))
		}
	})
	if err != nil {
		sm.logger.Error("failed to connect passthrough MCP stream",
			zap.String("execution_id", execution.ID),
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
		OnBranchSwitch: func(branchSwitch *agentctl.GitBranchSwitchNotification) {
			if sm.callbacks.OnBranchSwitch != nil {
				sm.callbacks.OnBranchSwitch(execution, branchSwitch)
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
	ctx := sm.streamContext(execution)

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

	// Idempotency guard: if a workspace stream is already attached, another
	// goroutine has connected it (e.g. workspace-only ensure followed by full
	// launch promotion). Treat as success and exit cleanly.
	if execution.GetWorkspaceStream() != nil {
		sm.logger.Debug("workspace stream already attached, skipping connect",
			zap.String("instance_id", execution.ID))
		return
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Re-check before each retry in case another goroutine connected meanwhile.
		if execution.GetWorkspaceStream() != nil {
			sm.logger.Debug("workspace stream attached during retry, exiting",
				zap.String("instance_id", execution.ID),
				zap.Int("attempt", attempt))
			return
		}

		callbacks := sm.buildWorkspaceCallbacks(execution)

		ws, err := execution.agentctl.StreamWorkspace(ctx, callbacks)
		if err != nil {
			sm.logger.Debug("workspace stream connection failed, retrying",
				zap.String("instance_id", execution.ID),
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
				zap.Error(err))

			if attempt < maxRetries {
				// Exit early on Manager shutdown so the backoff doesn't
				// strand a goroutine after Stop() returns.
				if !sm.sleepOrStop(backoff) {
					return
				}
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

		// Wait for the stream to close. Also exits on Manager shutdown so the
		// goroutine drains when the remote end keeps the connection open —
		// in that case we close ws ourselves so the underlying WS read/write
		// loops in agentctl.WorkspaceStream also exit. ws.Close is idempotent
		// via closeOnce. A nil stopCh (isolated tests) blocks on the nil
		// channel forever, which degrades to plain <-ws.Done() semantics.
		select {
		case <-ws.Done():
		case <-sm.stopCh:
			ws.Close()
		}
		// Block until the stream's read/write goroutines have fully unwound
		// before returning. Done()/Close only signal shutdown, so without this
		// the StreamManager's wg releases while a blocked websocket read is
		// still draining — stranding a goroutine that leak detection catches.
		ws.Wait()
		execution.ClearWorkspaceStream(ws)
		return
	}

	sm.logger.Error("failed to connect to workspace stream after retries",
		zap.String("instance_id", execution.ID),
		zap.Int("max_retries", maxRetries))
}

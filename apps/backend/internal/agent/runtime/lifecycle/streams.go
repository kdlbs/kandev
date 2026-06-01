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
	// stopCh is the Manager-owned shutdown signal. The retry/backoff and
	// connected `<-ws.Done() / <-stop>` select read from it so they drain on
	// Manager.Stop. May be nil when isolated tests don't care about external
	// shutdown; waitCh below covers Wait-driven drains in that case.
	stopCh <-chan struct{}
	// waitCh is closed by Wait() so retry/backoff and the connected select
	// drain even when the external stopCh isn't closed by the caller (or is
	// nil). Together with stopCh this makes Wait an absolute drain barrier,
	// which goleak.VerifyTestMain depends on under CI load.
	waitCh     chan struct{}
	waitChOnce sync.Once
	wg         sync.WaitGroup
	wgMu       sync.Mutex
	stopped    bool
}

// stopChannelContext wraps a parent ctx with two auxiliary stop channels.
// Done() returns a per-instance merged channel that closes when any of
// parent.Done(), primary or secondary fires. The merge goroutine spawned by
// Done() exits as soon as any signal fires, and Wait()'s waitCh close
// guarantees that happens at teardown time.
//
// We keep merge spawn behind a sync.Once so repeated Done() calls (the runtime
// re-asks every select tick) don't pile up goroutines.
type stopChannelContext struct {
	context.Context
	primary   <-chan struct{}
	secondary <-chan struct{}

	once   sync.Once
	merged chan struct{}
}

func (c *stopChannelContext) Done() <-chan struct{} {
	if c.primary == nil && c.secondary == nil {
		return c.Context.Done()
	}
	c.once.Do(func() {
		c.merged = make(chan struct{})
		go c.mergeStops()
	})
	return c.merged
}

func (c *stopChannelContext) mergeStops() {
	defer close(c.merged)
	switch {
	case c.primary != nil && c.secondary != nil:
		select {
		case <-c.primary:
		case <-c.secondary:
		case <-c.Context.Done():
		}
	case c.primary != nil:
		select {
		case <-c.primary:
		case <-c.Context.Done():
		}
	default:
		select {
		case <-c.secondary:
		case <-c.Context.Done():
		}
	}
}

func (c *stopChannelContext) Err() error {
	if c.primary != nil {
		select {
		case <-c.primary:
			return context.Canceled
		default:
		}
	}
	if c.secondary != nil {
		select {
		case <-c.secondary:
			return context.Canceled
		default:
		}
	}
	return c.Context.Err()
}

// NewStreamManager creates a new StreamManager.
//
// stopCh is the Manager-owned shutdown signal used by the workspace-stream
// retry backoff to drain cleanly. Pass nil from tests that exercise the
// manager in isolation; production callers wire it from Manager.stopCh.
// Either way, Wait() closes a per-StreamManager internal channel that the
// same drain sites observe — so Wait remains an absolute drain barrier.
func NewStreamManager(log *logger.Logger, callbacks StreamCallbacks, mcpHandler agentctl.MCPHandler, stopCh <-chan struct{}) *StreamManager {
	return &StreamManager{
		logger:     log.WithFields(zap.String("component", "stream-manager")),
		callbacks:  callbacks,
		mcpHandler: mcpHandler,
		stopCh:     stopCh,
		waitCh:     make(chan struct{}),
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
// Closes the internal waitCh first so any goroutine still parked in the retry
// backoff or the connected `<-ws.Done() / <-stop>` select drains without
// depending on the caller having closed the external stopCh.
func (sm *StreamManager) Wait() {
	sm.wgMu.Lock()
	sm.stopped = true
	sm.wgMu.Unlock()
	sm.waitChOnce.Do(func() { close(sm.waitCh) })
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
// Returns true when the timer fires, false when either the external stopCh
// or the internal Wait-driven waitCh fires first.
func (sm *StreamManager) sleepOrStop(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	if sm.stopCh == nil {
		select {
		case <-timer.C:
			return true
		case <-sm.waitCh:
			return false
		}
	}
	select {
	case <-timer.C:
		return true
	case <-sm.stopCh:
		return false
	case <-sm.waitCh:
		return false
	}
}

// streamContext preserves the execution's session trace values while making
// in-flight WebSocket dials cancellable by either the external Manager
// shutdown signal or StreamManager.Wait's internal drain signal.
func (sm *StreamManager) streamContext(execution *AgentExecution) context.Context {
	return &stopChannelContext{
		Context:   execution.SessionTraceContext(),
		primary:   sm.stopCh,
		secondary: sm.waitCh,
	}
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

		// Wait for the stream to close. Also exits on Manager shutdown / Wait
		// so the goroutine drains when the remote end keeps the connection
		// open — in that case we close ws ourselves so the underlying WS
		// read/write loops in agentctl.WorkspaceStream also exit. ws.Close
		// is idempotent via closeOnce. The waitCh branch covers the case
		// where the caller never closes external stopCh (or stopCh is nil)
		// but still calls Wait — without it, isolated tests that triggered
		// this select would leak under CI scheduling.
		shutdown := func() {
			ws.Close()
		}
		if sm.stopCh == nil {
			select {
			case <-ws.Done():
			case <-sm.waitCh:
				shutdown()
			}
		} else {
			select {
			case <-ws.Done():
			case <-sm.stopCh:
				shutdown()
			case <-sm.waitCh:
				shutdown()
			}
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

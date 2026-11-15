package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

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
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return
	}
	err := client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, sm.mcpHandlerFor(execution), func(disconnectErr error) {
		if disconnectErr != nil {
			sm.logger.Debug("passthrough MCP stream disconnected",
				zap.String("execution_id", execution.ID),
				zap.Error(disconnectErr))
		}
	})
	releaseClient()
	if err != nil {
		sm.logger.Error("failed to connect passthrough MCP stream",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	}
}

// buildWorkspaceCallbacks creates callbacks scoped to the agentctl client that
// owns the workspace stream. Their lifetime follows that stream across ACP startup.
func (sm *StreamManager) buildWorkspaceCallbacks(execution *AgentExecution, streamClient ...*agentctl.Client) agentctl.WorkspaceStreamCallbacks {
	var sourceClient *agentctl.Client
	if len(streamClient) > 0 {
		sourceClient = streamClient[0]
	}
	forward := func(callback func()) {
		if sourceClient == nil {
			callback()
			return
		}
		execution.withAgentCtlClient(sourceClient, callback)
	}
	return agentctl.WorkspaceStreamCallbacks{
		OnShellOutput: func(data string) {
			if sm.callbacks.OnShellOutput != nil {
				forward(func() { sm.callbacks.OnShellOutput(execution, data) })
			}
		},
		OnShellExit: func(code int) {
			if sm.callbacks.OnShellExit != nil {
				forward(func() { sm.callbacks.OnShellExit(execution, code) })
			}
		},
		OnGitStatus: func(update *agentctl.GitStatusUpdate) {
			if sm.callbacks.OnGitStatus != nil {
				forward(func() { sm.callbacks.OnGitStatus(execution, update) })
			}
		},
		OnGitCommit: func(commit *agentctl.GitCommitNotification) {
			if sm.callbacks.OnGitCommit != nil {
				forward(func() { sm.callbacks.OnGitCommit(execution, commit) })
			}
		},
		OnGitReset: func(reset *agentctl.GitResetNotification) {
			if sm.callbacks.OnGitReset != nil {
				forward(func() { sm.callbacks.OnGitReset(execution, reset) })
			}
		},
		OnBranchSwitch: func(branchSwitch *agentctl.GitBranchSwitchNotification) {
			if sm.callbacks.OnBranchSwitch != nil {
				forward(func() { sm.callbacks.OnBranchSwitch(execution, branchSwitch) })
			}
		},
		OnFileChange: func(notification *agentctl.FileChangeNotification) {
			if sm.callbacks.OnFileChange != nil {
				forward(func() { sm.callbacks.OnFileChange(execution, notification) })
			}
		},
		OnWorktreeInventory: func(inventory *agentctl.WorktreeInventory) {
			if sm.callbacks.OnWorktreeInventory != nil {
				forward(func() { sm.callbacks.OnWorktreeInventory(execution, inventory) })
			}
		},
		OnProcessOutput: func(output *agentctl.ProcessOutput) {
			if sm.callbacks.OnProcessOutput != nil {
				forward(func() { sm.callbacks.OnProcessOutput(execution, output) })
			}
		},
		OnProcessStatus: func(status *agentctl.ProcessStatusUpdate) {
			if sm.callbacks.OnProcessStatus != nil {
				forward(func() { sm.callbacks.OnProcessStatus(execution, status) })
			}
		},
		OnConnected: func() {
			forward(func() {
				sm.logger.Debug("workspace stream connected",
					zap.String("instance_id", execution.ID))
			})
		},
		OnError: func(err string) {
			forward(func() {
				sm.logger.Debug("workspace stream error",
					zap.String("instance_id", execution.ID),
					zap.String("error", err))
			})
		},
	}
}

// waitForWorkspaceStreamShutdown blocks until the stream closes or the
// manager stops, closing the stream itself when shutdown is initiated here so
// the underlying websocket loops unwind.
func (sm *StreamManager) waitForWorkspaceStreamShutdown(ws *agentctl.WorkspaceStream) {
	if sm.stopCh == nil {
		select {
		case <-ws.Done():
		case <-sm.waitCh:
			ws.Close()
		}
		return
	}
	select {
	case <-ws.Done():
	case <-sm.stopCh:
		ws.Close()
	case <-sm.waitCh:
		ws.Close()
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

		client, releaseClient := execution.AcquireAgentCtlClient()
		if client == nil {
			return
		}
		callbacks := sm.buildWorkspaceCallbacks(execution, client)
		ws, err := client.StreamWorkspace(ctx, callbacks)
		releaseClient()
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
		advisoryCtx, advisoryCancel := context.WithCancel(ctx)
		sm.startRuntimeStorageAdvisoryWithContext(advisoryCtx, execution)
		defer advisoryCancel()

		// Wait for the stream to close. Also exits on Manager shutdown / Wait
		// so the goroutine drains when the remote end keeps the connection
		// open — in that case we close ws ourselves so the underlying WS
		// read/write loops in agentctl.WorkspaceStream also exit. ws.Close
		// is idempotent via closeOnce. The waitCh branch covers the case
		// where the caller never closes external stopCh (or stopCh is nil)
		// but still calls Wait — without it, isolated tests that triggered
		// this select would leak under CI scheduling.
		sm.waitForWorkspaceStreamShutdown(ws)
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

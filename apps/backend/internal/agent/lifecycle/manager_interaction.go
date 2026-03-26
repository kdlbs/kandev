package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// WasSessionInitialized reports whether the execution completed ACP session setup.
// Returns false if the execution is not found or init hasn't completed.
func (m *Manager) WasSessionInitialized(executionID string) bool {
	exec, exists := m.executionStore.Get(executionID)
	if !exists {
		return false
	}
	return exec.sessionInitialized
}

// GetSessionAuthMethods returns auth methods for a session's execution.
// It first checks cached methods from agent_capabilities events. When the agent
// failed before reporting capabilities (e.g., immediate auth error), it falls
// back to static auth methods derived from the agent type.
func (m *Manager) GetSessionAuthMethods(sessionID string) []streams.AuthMethodInfo {
	exec, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	if methods := exec.GetAuthMethods(); len(methods) > 0 {
		return methods
	}
	return fallbackAuthMethods(exec.AgentID)
}

// fallbackAuthMethods returns static auth methods for known agent types.
// Used when the agent failed before sending agent_capabilities (e.g., auth error
// on first prompt before the agent could report its own auth methods).
func fallbackAuthMethods(agentID string) []streams.AuthMethodInfo {
	switch agentID {
	case "claude-acp":
		return []streams.AuthMethodInfo{
			{
				ID:          "claude-auth-login",
				Name:        "Anthropic Authentication",
				Description: "Log in to your Anthropic account",
				TerminalAuth: &streams.TerminalAuth{
					Command: "claude",
					Args:    []string{"auth", "login"},
					Label:   "Log in with Claude CLI",
				},
			},
		}
	case "auggie":
		return []streams.AuthMethodInfo{
			{
				ID:          "auggie-login",
				Name:        "Auggie Authentication",
				Description: "Log in to your Auggie account",
				TerminalAuth: &streams.TerminalAuth{
					Command: "auggie",
					Args:    []string{"login"},
					Label:   "Log in with Auggie CLI",
				},
			},
		}
	default:
		return nil
	}
}

// PromptAgent sends a follow-up prompt to a running agent
// Attachments (images) are passed to the agent if provided
func (m *Manager) PromptAgent(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment) (*PromptResult, error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return nil, fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}
	return m.sessionManager.SendPrompt(ctx, execution, prompt, true, attachments)
}

// CancelAgent interrupts the current agent turn without terminating the process,
// allowing the user to send a new prompt.
func (m *Manager) CancelAgent(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	m.logger.Info("cancelling agent turn",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))

	if err := execution.agentctl.Cancel(ctx); err != nil {
		m.logger.Error("failed to cancel agent turn",
			zap.String("execution_id", executionID),
			zap.Error(err))
		return fmt.Errorf("failed to cancel agent: %w", err)
	}

	// Don't clear buffers or mark ready here.
	// The agent will respond to the original prompt with StopReason=cancelled,
	// which triggers handleCompleteEvent() to properly flush buffers and mark state.
	// Clearing here would race with in-flight notifications and lose content.

	m.logger.Info("agent cancel sent, waiting for turn completion",
		zap.String("execution_id", executionID))

	// Wait for the in-flight SendPrompt to finish processing the cancel completion.
	// Without this, a follow-up PromptAgent races on promptDoneCh with two readers.
	execution.promptFinishedMu.Lock()
	ch := execution.promptFinished
	execution.promptFinishedMu.Unlock()

	if ch != nil {
		select {
		case <-ch:
			m.logger.Debug("in-flight prompt finished after cancel",
				zap.String("execution_id", executionID))
		case <-time.After(10 * time.Second):
			m.logger.Warn("timed out waiting for in-flight prompt to finish after cancel",
				zap.String("execution_id", executionID))
			return fmt.Errorf("timed out waiting for in-flight prompt to finish after cancel")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// SetSessionMode changes the session mode for a running agent.
// Always uses the execution's current ACPSessionID rather than the caller's copy,
// which may be stale during startup/restart when the session is being re-initialized.
func (m *Manager) SetSessionMode(ctx context.Context, executionID, _ string, modeID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	if !execution.sessionInitialized || execution.ACPSessionID == "" {
		return fmt.Errorf("execution %q ACP session is not ready", executionID)
	}
	return execution.agentctl.SetMode(ctx, execution.ACPSessionID, modeID)
}

// SetSessionModeBySessionID changes the session mode for a running agent by session ID.
func (m *Manager) SetSessionModeBySessionID(ctx context.Context, sessionID, modeID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	return m.SetSessionMode(ctx, execution.ID, execution.ACPSessionID, modeID)
}

// SetSessionModel changes the session model for a running agent.
func (m *Manager) SetSessionModel(ctx context.Context, executionID, modelID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	return execution.agentctl.SetModel(ctx, modelID)
}

// SetSessionModelBySessionID changes the session model for a running agent by session ID.
func (m *Manager) SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	return m.SetSessionModel(ctx, execution.ID, modelID)
}

// AuthenticateBySessionID triggers authentication for a given auth method on the agent.
func (m *Manager) AuthenticateBySessionID(ctx context.Context, sessionID, methodID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}
	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	return execution.agentctl.Authenticate(ctx, methodID)
}

// ResetAgentContext resets the agent's conversation context. For ACP agents that support
// session reset, this creates a new session on the existing connection (fast, no process restart).
// For all other agents, this falls back to RestartAgentProcess (full subprocess restart).
func (m *Manager) ResetAgentContext(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}

	// Passthrough agents always need full restart
	if execution.PassthroughProcessID != "" {
		return m.RestartAgentProcess(ctx, executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	// Resolve agent config and MCP servers for session reset
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		m.logger.Info("cannot resolve agent config for session reset, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.RestartAgentProcess(ctx, executionID)
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		m.logger.Warn("cannot resolve MCP servers for session reset, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.RestartAgentProcess(ctx, executionID)
	}

	// Try session-level reset (only ACP adapters support this)
	newSessionID, err := execution.agentctl.ResetSession(ctx, execution.WorkspacePath, mcpServers)
	if err != nil {
		m.logger.Info("session reset not supported, falling back to process restart",
			zap.String("execution_id", executionID), zap.Error(err))
		return m.RestartAgentProcess(ctx, executionID)
	}

	// Success — update execution state without restarting process
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.ACPSessionID = newSessionID
		exec.Status = v1.AgentStatusReady
		exec.needsResumeContext = false
		exec.resumeContextInjected = false

		exec.messageMu.Lock()
		exec.messageBuffer.Reset()
		exec.thinkingBuffer.Reset()
		exec.currentMessageID = ""
		exec.currentThinkingID = ""
		exec.messageMu.Unlock()

		// Drain any stale prompt completion signal
		select {
		case <-exec.promptDoneCh:
		default:
		}
	})

	m.logger.Info("agent context reset via session (no process restart)",
		zap.String("execution_id", executionID),
		zap.String("session_id", execution.SessionID),
		zap.String("new_acp_session_id", newSessionID))

	m.eventPublisher.PublishAgentEvent(ctx, events.AgentContextReset, execution)
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentReady, execution)
	return nil
}

// CancelAgentBySessionID cancels the current agent turn for a specific session
func (m *Manager) CancelAgentBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}

	return m.CancelAgent(ctx, execution.ID)
}

// StopAgent stops an agent execution
func (m *Manager) StopAgent(ctx context.Context, executionID string, force bool) error {
	return m.StopAgentWithReason(ctx, executionID, "", force)
}

// StopAgentWithReason stops an agent execution and passes a semantic reason to runtime teardown.
func (m *Manager) StopAgentWithReason(ctx context.Context, executionID string, reason string, force bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.logger.Info("stopping agent",
		zap.String("execution_id", executionID),
		zap.String("reason", reason),
		zap.Bool("force", force),
		zap.String("runtime", execution.RuntimeName))

	// Try to gracefully stop via agentctl first, then always close connections
	if execution.agentctl != nil {
		if !force {
			if err := execution.agentctl.Stop(ctx); err != nil {
				m.logger.Warn("failed to stop agent via agentctl",
					zap.String("execution_id", executionID),
					zap.Error(err))
			}
		}
		execution.agentctl.Close()
	}

	// Stop the agent execution via the runtime that created it
	m.stopAgentViaBackend(ctx, executionID, execution, reason, force)

	// Update execution status and remove from tracking
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.Status = v1.AgentStatusStopped
		now := time.Now()
		exec.FinishedAt = &now
	})

	// End session trace span
	execution.EndSessionSpan()

	m.executionStore.Remove(executionID)
	m.clearRemoteStatus(execution.SessionID)

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID))

	// Publish stopped event
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStopped, execution)

	return nil
}

// StopBySessionID stops the agent for a specific session
func (m *Manager) StopBySessionID(ctx context.Context, sessionID string, force bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent running for session %q", sessionID)
	}

	return m.StopAgent(ctx, execution.ID, force)
}

// RestartAgentProcess stops the agent subprocess and starts a fresh one, clearing the agent's
// conversation context. For ACP agents this restarts via agentctl with a new ACP session.
// For passthrough (TUI) agents this kills the PTY process and relaunches without --resume.
func (m *Manager) RestartAgentProcess(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found: %w", executionID, ErrExecutionNotFound)
	}

	// Passthrough agents: kill PTY and relaunch fresh (no --resume).
	if execution.PassthroughProcessID != "" {
		return m.restartPassthroughProcess(ctx, execution)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	m.logger.Info("restarting agent process for context reset",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID))

	// Resolve agent config early — needed for both command rebuild and ACP session init
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return fmt.Errorf("failed to get agent config for restart: %w", err)
	}

	// 1. Close WebSocket streams (updates + workspace)
	execution.agentctl.Close()

	// 2. Stop the agent subprocess via agentctl (keeps agentctl server alive)
	if err := execution.agentctl.Stop(ctx); err != nil {
		m.logger.Warn("failed to stop agent subprocess during restart",
			zap.String("execution_id", executionID),
			zap.Error(err))
		// Continue — the process may already be stopped
	}

	// 3. Rebuild agent command without resume flags and reset execution state
	freshCmd, freshContinueCmd := m.buildFreshAgentCommand(ctx, execution, agentConfig)
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.ACPSessionID = ""
		exec.Status = v1.AgentStatusStarting
		exec.ErrorMessage = ""
		exec.needsResumeContext = false
		exec.resumeContextInjected = false
		exec.sessionInitialized = false
		exec.AgentCommand = freshCmd
		exec.ContinueCommand = freshContinueCmd

		exec.messageMu.Lock()
		exec.messageBuffer.Reset()
		exec.thinkingBuffer.Reset()
		exec.currentMessageID = ""
		exec.currentThinkingID = ""
		exec.messageMu.Unlock()

		// Drain any stale prompt completion signal
		select {
		case <-exec.promptDoneCh:
		default:
		}
	})

	// 4. Wait for agentctl to be ready (it should still be running)
	if err := execution.agentctl.WaitForReady(ctx, 30*time.Second); err != nil {
		m.updateExecutionError(executionID, "agentctl not ready after restart: "+err.Error())
		return fmt.Errorf("agentctl not ready after restart: %w", err)
	}

	// 5. Reconfigure and start new agent subprocess
	approvalPolicy, _ := m.resolveApprovalPolicyAndDisplayName(ctx, execution)
	taskDescription := getTaskDescriptionFromMetadata(execution)

	if _, err := m.configureAndStartAgent(ctx, execution, taskDescription, approvalPolicy); err != nil {
		m.updateExecutionError(executionID, "failed to restart agent: "+err.Error())
		return fmt.Errorf("failed to restart agent: %w", err)
	}

	// 6. Wait for agent process to initialize
	if err := execution.agentctl.WaitForReady(ctx, 10*time.Second); err != nil {
		m.logger.Warn("agent process slow to initialize after restart, continuing",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to resolve MCP config for restart: %w", err)
	}

	if err := m.initializeACPSessionForRestart(ctx, execution, agentConfig, mcpServers); err != nil {
		m.updateExecutionError(executionID, "failed to initialize ACP session after restart: "+err.Error())
		return fmt.Errorf("failed to initialize ACP session after restart: %w", err)
	}

	m.logger.Info("agent process restarted with fresh context",
		zap.String("execution_id", executionID),
		zap.String("session_id", execution.SessionID),
		zap.String("new_acp_session_id", execution.ACPSessionID))

	m.eventPublisher.PublishAgentEvent(ctx, events.AgentContextReset, execution)
	return nil
}

// initializeACPSessionForRestart connects streams and creates a new ACP session without
// sending an initial prompt. The caller (workflow processOnEnter) handles prompting separately.
func (m *Manager) initializeACPSessionForRestart(
	ctx context.Context,
	execution *AgentExecution,
	agentConfig agents.Agent,
	mcpServers []agentctltypes.McpServer,
) error {
	// Connect WebSocket streams
	if m.streamManager != nil {
		updatesReady := make(chan struct{})
		m.streamManager.ConnectAll(execution, updatesReady)

		select {
		case <-updatesReady:
			m.logger.Debug("updates stream ready after restart")
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for agent stream to connect after restart")
		}
	}

	// Initialize ACP session (always session/new since ACPSessionID was cleared)
	result, err := m.sessionManager.InitializeSession(
		ctx,
		execution.agentctl,
		agentConfig,
		"", // empty — force session/new
		execution.WorkspacePath,
		mcpServers,
	)
	if err != nil {
		return fmt.Errorf("ACP session initialization failed: %w", err)
	}

	execution.ACPSessionID = result.SessionID

	if m.sessionManager.eventPublisher != nil {
		m.sessionManager.eventPublisher.PublishACPSessionCreated(execution, result.SessionID)
	}

	// Mark execution as ready
	m.executionStore.UpdateStatus(execution.ID, v1.AgentStatusReady)
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentReady, execution)

	return nil
}

// GetExecution returns an agent execution by ID.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// The returned execution pointer should not be modified directly - use the Manager's
// methods to update execution state (MarkReady, MarkCompleted, UpdateStatus).
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecution(executionID string) (*AgentExecution, bool) {
	return m.executionStore.Get(executionID)
}

// GetExecutionBySessionID returns the agent execution for a session from the in-memory store only.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// A session can have at most one active execution at a time. If a session exists
// but has no active execution, this returns (nil, false).
//
// For workspace-oriented operations that need restart recovery,
// use GetOrEnsureExecution instead.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecutionBySessionID(sessionID string) (*AgentExecution, bool) {
	return m.executionStore.GetBySessionID(sessionID)
}

// IsRemoteSession checks whether a session is associated with a remote executor
// (e.g., sprites). It first checks the in-memory execution store, then falls back
// to the database via WorkspaceInfoProvider. This is useful when the execution
// hasn't been recreated yet after a backend restart.
func (m *Manager) IsRemoteSession(ctx context.Context, sessionID string) bool {
	// Check in-memory execution first (fast path).
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		if execution.RuntimeName == string(executor.NameSprites) {
			return true
		}
		if execution.Metadata != nil {
			if isRemote, ok := execution.Metadata[MetadataKeyIsRemote].(bool); ok && isRemote {
				return true
			}
		}
		return false
	}

	// Fall back to database records (post-restart, execution not yet recreated).
	if m.workspaceInfoProvider == nil {
		return false
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil || info == nil {
		return false
	}
	if models.IsRemoteExecutorType(models.ExecutorType(info.ExecutorType)) {
		return true
	}
	// Backwards compatibility: old records may only have RuntimeName set.
	return info.RuntimeName == string(executor.NameSprites) || info.RuntimeName == string(executor.NameRemoteDocker)
}

// GetAvailableCommandsForSession returns the available slash commands for a session.
// Returns nil if the session doesn't exist or has no commands stored.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetAvailableCommandsForSession(sessionID string) []streams.AvailableCommand {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetAvailableCommands()
}

// GetModeStateForSession returns the cached session mode state.
// Returns nil if the session has no execution or no mode state cached.
func (m *Manager) GetModeStateForSession(sessionID string) *CachedModeState {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetModeState()
}

// GetModelStateForSession returns the cached session model state.
// Returns nil if the session has no execution or no model state cached.
func (m *Manager) GetModelStateForSession(sessionID string) *CachedModelState {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil
	}
	return execution.GetModelState()
}

// ListExecutions returns all currently tracked agent executions.
//
// Returns a snapshot of all executions in memory at the time of call. The returned slice
// contains pointers to execution objects that may be modified by other goroutines after
// this method returns. Do not modify execution state directly - use Manager methods instead.
//
// The list includes executions in all states:
//   - Starting (process launching, agentctl initializing)
//   - Running (actively processing prompts)
//   - Ready (waiting for user input)
//   - Completed/Failed (finished but not yet removed)
//
// Thread-safe: Can be called concurrently. Returns a new slice on each call.
//
// Typical usage: Status endpoints, debugging, cleanup loops.
func (m *Manager) ListExecutions() []*AgentExecution {
	return m.executionStore.List()
}

// IsAgentRunningForSession checks if an agent process is running or starting for a session.
//
// For passthrough sessions (direct PTY mode), it checks whether the PTY process is alive
// in the InteractiveRunner. For ACP sessions, it probes agentctl's status endpoint.
//
// Returns true if:
//   - Passthrough process is alive in the InteractiveRunner
//   - Agent status is "running" (actively processing prompts)
//   - Agent status is "starting" (process launched but not yet ready)
//
// Returns false if:
//   - No execution exists for this session
//   - Passthrough process ID is set but process is not alive
//   - agentctl client is not available
//   - Status check fails (network/timeout error)
//   - Agent is in any other state (stopped, failed, etc.)
func (m *Manager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	// First check if we have an execution tracked for this session
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
		return false
	}

	// Passthrough sessions run as direct PTY processes via InteractiveRunner,
	// bypassing agentctl's ACP protocol. Check the process directly.
	if execution.PassthroughProcessID != "" {
		if runner := m.GetInteractiveRunner(); runner != nil {
			return runner.IsProcessReadyOrPending(execution.PassthroughProcessID)
		}
		return false
	}

	// Probe agentctl status to verify the agent process is running
	if execution.agentctl == nil {
		return false
	}

	status, err := execution.agentctl.GetStatus(ctx)
	if err != nil {
		m.logger.Debug("failed to get agentctl status",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}

	return status.IsAgentRunning()
}

// UpdateStatus updates the status of an execution
func (m *Manager) UpdateStatus(executionID string, status v1.AgentStatus) error {
	if err := m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.Status = status
	}); err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return fmt.Errorf("execution %q not found", executionID)
		}
		return err
	}

	m.logger.Debug("updated execution status",
		zap.String("execution_id", executionID),
		zap.String("status", string(status)))

	return nil
}

// MarkReady marks an execution as ready for follow-up prompts.
//
// This transitions the execution to the "ready" state, indicating the agent has finished
// processing the current prompt and is waiting for user input. This is called:
//   - After agent initialization completes (session loaded, workspace ready)
//   - After agent finishes processing a prompt (via stream completion event)
//   - After cancelling an agent turn (to allow new prompts)
//
// State Machine Transitions:
//
//	Starting -> Ready (after initialization)
//	Running  -> Ready (after prompt completion)
//	Any      -> Ready (after cancel)
//
// Publishes an AgentReady event to notify subscribers (frontend, orchestrator).
//
// Returns error if execution not found.
func (m *Manager) MarkReady(executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	// Skip if already ready (prevents duplicate events)
	if execution.Status == v1.AgentStatusReady {
		return nil
	}

	m.executionStore.UpdateStatus(executionID, v1.AgentStatusReady)

	m.logger.Info("execution ready for follow-up prompts",
		zap.String("execution_id", executionID))

	// Publish ready event
	m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentReady, execution)

	return nil
}

// MarkCompleted marks an execution as completed or failed.
//
// This is called when the agent process terminates, either successfully or with an error.
// The final status is determined by exit code and error message:
//
//   - exitCode == 0 && errorMessage == "" → AgentStatusCompleted (success)
//   - Otherwise                            → AgentStatusFailed (failure)
//
// Parameters:
//   - executionID: The execution to mark as completed
//   - exitCode: Process exit code (0 = success, non-zero = failure)
//   - errorMessage: Human-readable error description (empty string if no error)
//
// State Machine:
//
//	This is a terminal state transition - no further state changes are expected after this.
//	Typical flow: Starting -> Running -> Ready -> ... -> Completed/Failed
//
// Publishes either AgentCompleted or AgentFailed event depending on final status.
//
// Returns error if execution not found.
func (m *Manager) MarkCompleted(executionID string, exitCode int, errorMessage string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	// Guard against duplicate completion (e.g. ACP prompt error + process exit error).
	// MarkCompleted is a terminal transition — once in Completed/Failed, skip re-publishing.
	if execution.Status == v1.AgentStatusCompleted || execution.Status == v1.AgentStatusFailed {
		m.logger.Warn("ignoring duplicate MarkCompleted for already-terminal execution",
			zap.String("execution_id", executionID),
			zap.String("current_status", string(execution.Status)),
			zap.Int("exit_code", exitCode))
		return nil
	}

	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		now := time.Now()
		exec.FinishedAt = &now
		exec.ExitCode = &exitCode
		exec.ErrorMessage = errorMessage

		if exitCode == 0 && errorMessage == "" {
			exec.Status = v1.AgentStatusCompleted
		} else {
			exec.Status = v1.AgentStatusFailed
		}
	})

	// End session trace span
	execution.EndSessionSpan()

	m.logger.Info("execution completed",
		zap.String("execution_id", executionID),
		zap.Int("exit_code", exitCode),
		zap.String("status", string(execution.Status)))

	// Publish completion event
	eventType := events.AgentCompleted
	if execution.Status == v1.AgentStatusFailed {
		eventType = events.AgentFailed
	}
	m.eventPublisher.PublishAgentEvent(context.Background(), eventType, execution)

	return nil
}

// RespondToPermission sends a response to an agent's permission request.
//
// When an agent requests permission (e.g., to run a bash command, modify files, etc.),
// it pauses execution and waits for user approval. This method sends the user's response.
//
// Parameters:
//   - executionID: The agent execution waiting for permission
//   - pendingID: Unique ID of the permission request (from permission request event)
//   - optionID: The user-selected option ID (from the permission request's options array)
//   - cancelled: If true, indicates user cancelled/rejected the permission request.
//     When cancelled=true, optionID is ignored.
//
// Response Semantics:
//   - cancelled=false, optionID="approve" → User approved the action
//   - cancelled=false, optionID="deny"    → User explicitly denied the action
//   - cancelled=true, optionID=""         → User cancelled/closed the dialog
//
// After receiving the response, the agent will either:
//   - Continue executing (if approved)
//   - Skip the action and report failure (if denied/cancelled)
//
// Timeout: 30 seconds for agentctl to acknowledge the response.
//
// Returns error if execution not found, agentctl unavailable, or communication fails.
func (m *Manager) RespondToPermission(executionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("agent execution not found: %s", executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("agent execution has no agentctl client: %s", executionID)
	}

	m.logger.Info("responding to permission request",
		zap.String("execution_id", executionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return execution.agentctl.RespondToPermission(ctx, pendingID, optionID, cancelled)
}

// RespondToPermissionBySessionID sends a response to a permission request using session ID.
//
// Convenience method that looks up the execution by session ID and delegates to RespondToPermission.
// See RespondToPermission for parameter semantics and behavior.
func (m *Manager) RespondToPermissionBySessionID(sessionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	return m.RespondToPermission(execution.ID, pendingID, optionID, cancelled)
}

// stopAgentViaBackend stops the agent execution via the runtime that created it.
func (m *Manager) stopAgentViaBackend(ctx context.Context, executionID string, execution *AgentExecution, reason string, force bool) {
	if execution.RuntimeName == "" || m.executorRegistry == nil {
		return
	}
	rt, err := m.executorRegistry.GetBackend(executor.Name(execution.RuntimeName))
	if err != nil {
		m.logger.Warn("failed to get runtime for stopping execution",
			zap.String("execution_id", executionID),
			zap.String("runtime", execution.RuntimeName),
			zap.Error(err))
		return
	}
	m.stopPassthroughProcess(ctx, executionID, execution, rt)
	runtimeInstance := &ExecutorInstance{
		InstanceID:           execution.ID,
		TaskID:               execution.TaskID,
		ContainerID:          execution.ContainerID,
		StandaloneInstanceID: execution.standaloneInstanceID,
		StandalonePort:       execution.standalonePort,
		Metadata:             execution.Metadata,
		StopReason:           reason,
	}
	if err := rt.StopInstance(ctx, runtimeInstance, force); err != nil {
		m.logger.Warn("failed to stop runtime instance, continuing with cleanup",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}
}

// stopPassthroughProcess stops the passthrough interactive process if one is running.
func (m *Manager) stopPassthroughProcess(ctx context.Context, executionID string, execution *AgentExecution, rt ExecutorBackend) {
	if execution.PassthroughProcessID == "" {
		return
	}
	interactiveRunner := rt.GetInteractiveRunner()
	if interactiveRunner == nil {
		return
	}
	if err := interactiveRunner.Stop(ctx, execution.PassthroughProcessID); err != nil {
		m.logger.Warn("failed to stop passthrough process",
			zap.String("execution_id", executionID),
			zap.String("process_id", execution.PassthroughProcessID),
			zap.Error(err))
		return
	}
	m.logger.Info("passthrough process stopped",
		zap.String("execution_id", executionID),
		zap.String("process_id", execution.PassthroughProcessID))
}

// buildFreshAgentCommand rebuilds the agent command without resume flags by going through
// the standard BuildCommandString pipeline with an empty SessionID. This works for all
// agent types because each agent's BuildCommand respects SessionID="" to skip resume flags.
func (m *Manager) buildFreshAgentCommand(ctx context.Context, execution *AgentExecution, agentConfig agents.Agent) (initial, continueCmd string) {
	var profileInfo *AgentProfileInfo
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		pi, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if err == nil {
			profileInfo = pi
		}
	}

	model := ""
	autoApprove := false
	permissionValues := make(map[string]bool)
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
		permissionValues["auto_approve"] = profileInfo.AutoApprove
		permissionValues["allow_indexing"] = profileInfo.AllowIndexing
		permissionValues["dangerously_skip_permissions"] = profileInfo.DangerouslySkipPermissions
	}
	if override, ok := execution.Metadata["model_override"].(string); ok && override != "" {
		model = override
	}

	opts := agents.CommandOptions{
		Model:            model,
		SessionID:        "", // Fresh start — no resume flags
		AutoApprove:      autoApprove,
		PermissionValues: permissionValues,
	}
	return m.commandBuilder.BuildCommandString(agentConfig, opts),
		m.commandBuilder.BuildContinueCommandString(agentConfig, opts)
}

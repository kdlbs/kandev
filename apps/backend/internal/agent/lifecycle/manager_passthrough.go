package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MarkPassthroughRunning marks a passthrough execution as running when user submits input.
// This is called when Enter key is detected in the terminal handler.
// It updates the execution status and publishes an AgentRunning event.
func (m *Manager) MarkPassthroughRunning(sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	if execution.PassthroughProcessID == "" {
		return fmt.Errorf("session %s is not in passthrough mode", sessionID)
	}

	// Only publish if not already running (prevents duplicate events)
	if execution.Status != v1.AgentStatusRunning {
		m.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)
		m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentRunning, execution)
	}

	return nil
}

// WritePassthroughStdin writes data to the agent process stdin in passthrough mode.
// Returns an error if the session is not in passthrough mode or if writing fails.
// Note: For terminal handler input, use MarkPassthroughRunning directly since
// the terminal handler writes to PTY directly for performance.
func (m *Manager) WritePassthroughStdin(ctx context.Context, sessionID string, data string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	if execution.PassthroughProcessID == "" {
		return fmt.Errorf("session %s is not in passthrough mode", sessionID)
	}

	// Get the interactive runner from runtime
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available")
	}

	// Write to stdin
	if err := interactiveRunner.WriteStdin(execution.PassthroughProcessID, data); err != nil {
		return err
	}

	return nil
}

// IsPassthroughSession checks if the given session is running in passthrough (PTY) mode.
func (m *Manager) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return false
	}
	return execution.PassthroughProcessID != ""
}

// ResizePassthroughPTY resizes the PTY for a passthrough process.
// Returns an error if the session is not in passthrough mode or if resizing fails.
func (m *Manager) ResizePassthroughPTY(ctx context.Context, sessionID string, cols, rows uint16) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	if execution.PassthroughProcessID == "" {
		return fmt.Errorf("session %s is not in passthrough mode", sessionID)
	}

	// Get the interactive runner from runtime
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available")
	}

	return interactiveRunner.ResizeBySession(sessionID, cols, rows)
}

// GetPassthroughBuffer returns the buffered output from the passthrough process.
// This is used for new subscribers to catch up on output.
func (m *Manager) GetPassthroughBuffer(ctx context.Context, sessionID string) (string, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return "", fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	if execution.PassthroughProcessID == "" {
		return "", fmt.Errorf("session %s is not in passthrough mode", sessionID)
	}

	// Get the interactive runner from runtime
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return "", fmt.Errorf("interactive runner not available")
	}

	chunks, ok := interactiveRunner.GetBuffer(execution.PassthroughProcessID)
	if !ok {
		return "", fmt.Errorf("passthrough process not found")
	}

	// Concatenate all chunks into a single string
	var buffer strings.Builder
	for _, chunk := range chunks {
		buffer.WriteString(chunk.Data)
	}

	return buffer.String(), nil
}

// buildPassthroughEnv builds the environment map for a passthrough session,
// including Kandev metadata and required credentials from the agent runtime config.
func (m *Manager) buildPassthroughEnv(ctx context.Context, execution *AgentExecution, requiredEnv []string) map[string]string {
	env := make(map[string]string)
	env["KANDEV_TASK_ID"] = execution.TaskID
	env["KANDEV_SESSION_ID"] = execution.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = execution.AgentProfileID
	if m.credsMgr != nil {
		for _, credKey := range requiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}
	return env
}

// startPassthroughShell starts the shell session for a passthrough execution.
// Non-fatal errors are logged with the provided warning message.
func (m *Manager) startPassthroughShell(ctx context.Context, execution *AgentExecution, shellWarnMsg string) {
	if execution.agentctl == nil {
		return
	}
	if err := execution.agentctl.StartShell(ctx); err != nil {
		m.logger.Warn(shellWarnMsg,
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	} else {
		m.logger.Info("shell session started for passthrough mode",
			zap.String("execution_id", execution.ID))
	}
}

// passthroughAgentCommand validates passthrough support and builds the command for a passthrough session.
// Returns the PassthroughAgent, PassthroughConfig, RuntimeConfig pointer, command, and any error.
func (m *Manager) passthroughAgentCommand(execution *AgentExecution, profileInfo *AgentProfileInfo) (agents.PassthroughAgent, agents.PassthroughConfig, *agents.RuntimeConfig, agents.Command, error) {
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return nil, agents.PassthroughConfig{}, nil, agents.Command{}, fmt.Errorf("failed to get agent config: %w", err)
	}

	ptAgent, ok := agentConfig.(agents.PassthroughAgent)
	if !ok {
		return nil, agents.PassthroughConfig{}, nil, agents.Command{}, fmt.Errorf("agent %s does not support passthrough mode", agentConfig.ID())
	}

	pt := ptAgent.PassthroughConfig()
	rt := agentConfig.Runtime()
	taskDescription := getTaskDescriptionFromMetadata(execution)

	cmd := ptAgent.BuildPassthroughCommand(agents.PassthroughOptions{
		Model:            profileModel(profileInfo),
		SessionID:        execution.ACPSessionID,
		Prompt:           taskDescription,
		PermissionValues: profilePermissionValues(profileInfo),
	})
	if cmd.IsEmpty() {
		return nil, agents.PassthroughConfig{}, nil, agents.Command{}, fmt.Errorf("passthrough command is empty for agent %s", agentConfig.ID())
	}
	return ptAgent, pt, rt, cmd, nil
}

// startInteractiveProcess launches the interactive PTY process for a passthrough session.
// Returns the process info on success.
func (m *Manager) startInteractiveProcess(ctx context.Context, execution *AgentExecution, pt agents.PassthroughConfig, env map[string]string, cmd agents.Command) (*process.InteractiveProcessInfo, error) {
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available for passthrough mode")
	}

	// Some agents (like Codex) require the terminal to be connected first because
	// they query the terminal for cursor position on startup.
	startReq := process.InteractiveStartRequest{
		SessionID:       execution.SessionID,
		Command:         cmd.Args(),
		WorkingDir:      execution.WorkspacePath,
		Env:             env,
		PromptPattern:   pt.PromptPattern,
		IdleTimeout:     pt.IdleTimeout,
		BufferMaxBytes:  pt.BufferMaxBytes,
		StatusDetector:  pt.StatusDetector,
		CheckInterval:   pt.CheckInterval,
		StabilityWindow: pt.StabilityWindow,
		ImmediateStart:  !pt.WaitForTerminal,
		DefaultCols:     120,
		DefaultRows:     40,
	}

	processInfo, err := interactiveRunner.Start(ctx, startReq)
	if err != nil {
		m.updateExecutionError(execution.ID, "failed to start passthrough session: "+err.Error())
		return nil, fmt.Errorf("failed to start passthrough session: %w", err)
	}
	return processInfo, nil
}

// startPassthroughSession starts an agent in passthrough mode (direct terminal interaction).
// Instead of using ACP protocol, the agent's stdin/stdout is passed through directly.
func (m *Manager) startPassthroughSession(ctx context.Context, execution *AgentExecution, profileInfo *AgentProfileInfo) error {
	_, pt, rt, cmd, err := m.passthroughAgentCommand(execution, profileInfo)
	if err != nil {
		return err
	}

	m.logger.Info("passthrough command built",
		zap.String("session_id", execution.SessionID),
		zap.Strings("full_command", cmd.Args()))

	env := m.buildPassthroughEnv(ctx, execution, rt.RequiredEnv)

	processInfo, err := m.startInteractiveProcess(ctx, execution, pt, env, cmd)
	if err != nil {
		return err
	}

	execution.PassthroughProcessID = processInfo.ID

	m.logger.Info("passthrough session started",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID),
		zap.String("process_id", processInfo.ID),
		zap.Strings("command", cmd.Args()))

	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")
	m.startPassthroughShell(ctx, execution, "failed to start shell for passthrough session")

	if m.streamManager != nil && execution.agentctl != nil {
		go m.streamManager.connectWorkspaceStream(execution, nil)
	}

	return nil
}

// profileModel extracts the model from profile info, returning empty string if nil.
func profileModel(p *AgentProfileInfo) string {
	if p == nil {
		return ""
	}
	return p.Model
}

// profilePermissionValues builds a permission values map from profile info.
func profilePermissionValues(p *AgentProfileInfo) map[string]bool {
	if p == nil {
		return nil
	}
	return map[string]bool{
		"auto_approve":                 p.AutoApprove,
		"dangerously_skip_permissions": p.DangerouslySkipPermissions,
		"allow_indexing":               p.AllowIndexing,
	}
}

// ResumePassthroughSession restarts a passthrough session after backend restart.
// This is called when user reconnects to a terminal but the PTY process is no longer running.
// If the agent supports resume, it uses the resume flag to continue the last conversation.
// Otherwise, it starts a fresh CLI session with the same profile settings.
func (m *Manager) ResumePassthroughSession(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no execution found for session: %s", sessionID)
	}

	// Get agent config
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	ptAgent, ok := agentConfig.(agents.PassthroughAgent)
	if !ok {
		return fmt.Errorf("agent %s does not support passthrough mode", agentConfig.ID())
	}

	pt := ptAgent.PassthroughConfig()
	rt := agentConfig.Runtime()

	// Get profile info for permission settings
	var profileInfo *AgentProfileInfo
	if m.profileResolver != nil && execution.AgentProfileID != "" {
		profileInfo, _ = m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
	}

	// Build the resume command
	cmd := ptAgent.BuildPassthroughCommand(agents.PassthroughOptions{
		Model:            profileModel(profileInfo),
		Resume:           true,
		PermissionValues: profilePermissionValues(profileInfo),
	})
	if cmd.IsEmpty() {
		return fmt.Errorf("passthrough resume command is empty for agent %s", agentConfig.ID())
	}

	m.logger.Info("resuming passthrough session",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID),
		zap.Strings("command", cmd.Args()))

	// Get the interactive runner
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available")
	}

	// Build environment variables including required credentials
	env := m.buildPassthroughEnv(ctx, execution, rt.RequiredEnv)

	// Start the interactive process.
	// Always use immediate start on resume — the terminal WebSocket is already connected,
	// so we don't need to wait for a resize to get exact dimensions. The first resize
	// from the terminal will correct the dimensions. Without this, TUI apps that use
	// WaitForTerminal would never start because the frontend may not send resizes
	// to a process it doesn't know about yet.
	startReq := process.InteractiveStartRequest{
		SessionID:       sessionID,
		Command:         cmd.Args(),
		WorkingDir:      execution.WorkspacePath,
		Env:             env,
		IdleTimeout:     pt.IdleTimeout,
		BufferMaxBytes:  pt.BufferMaxBytes,
		StatusDetector:  pt.StatusDetector,
		CheckInterval:   pt.CheckInterval,
		StabilityWindow: pt.StabilityWindow,
		ImmediateStart:  true,
		DefaultCols:     120,
		DefaultRows:     40,
	}

	processInfo, err := interactiveRunner.Start(ctx, startReq)
	if err != nil {
		return fmt.Errorf("failed to start passthrough session: %w", err)
	}

	// Update the execution with new process ID
	execution.PassthroughProcessID = processInfo.ID

	m.logger.Info("passthrough session resumed",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID),
		zap.String("process_id", processInfo.ID))

	// Start shell session for workspace shell access (right panel terminal).
	// This needs to be done after resume since the shell process was killed on backend restart.
	m.startPassthroughShell(ctx, execution, "failed to start shell for resumed passthrough session")

	// Connect to workspace stream for shell/git/file features.
	// Only connect if not already connected (process restart reuses the same agentctl).
	if m.streamManager != nil && execution.agentctl != nil && execution.GetWorkspaceStream() == nil {
		go m.streamManager.connectWorkspaceStream(execution, nil)
	}

	return nil
}

// handlePassthroughTurnComplete is called when turn detection fires for a passthrough session.
// This marks the execution as ready for follow-up prompts when the agent finishes processing.
func (m *Manager) handlePassthroughTurnComplete(sessionID string) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		m.logger.Debug("turn complete for unknown session (may have ended)",
			zap.String("session_id", sessionID))
		return
	}

	m.logger.Info("passthrough turn complete",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	// Mark execution as ready for follow-up prompts
	// This publishes AgentReady event to notify subscribers
	if err := m.MarkReady(execution.ID); err != nil {
		m.logger.Error("failed to mark execution as ready after passthrough turn complete",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
	}
}

// handlePassthroughOutput handles output from a passthrough process and publishes it to the event bus.
// This is called when running in standalone mode without a WorkspaceTracker.
func (m *Manager) handlePassthroughOutput(output *agentctltypes.ProcessOutput) {
	if output == nil {
		return
	}

	execution, exists := m.executionStore.GetBySessionID(output.SessionID)
	if !exists {
		m.logger.Debug("passthrough output for unknown session",
			zap.String("session_id", output.SessionID))
		return
	}

	// Convert to agentctl client type for event publisher
	clientOutput := &agentctl.ProcessOutput{
		SessionID: output.SessionID,
		ProcessID: output.ProcessID,
		Kind:      output.Kind,
		Stream:    output.Stream,
		Data:      output.Data,
		Timestamp: output.Timestamp,
	}

	m.eventPublisher.PublishProcessOutput(execution, clientOutput)
}

// handlePassthroughStatus handles status updates from a passthrough process and publishes to the event bus.
// This is called when running in standalone mode without a WorkspaceTracker.
// When the process exits while a WebSocket is connected, it attempts auto-restart with rate limiting.
func (m *Manager) handlePassthroughStatus(status *agentctltypes.ProcessStatusUpdate) {
	if status == nil {
		return
	}

	execution, exists := m.executionStore.GetBySessionID(status.SessionID)
	if !exists {
		m.logger.Debug("passthrough status for unknown session",
			zap.String("session_id", status.SessionID))
		return
	}

	// Convert to agentctl client type for event publisher
	clientStatus := &agentctl.ProcessStatusUpdate{
		SessionID:  status.SessionID,
		ProcessID:  status.ProcessID,
		Kind:       status.Kind,
		Command:    status.Command,
		ScriptName: status.ScriptName,
		WorkingDir: status.WorkingDir,
		Status:     status.Status,
		ExitCode:   status.ExitCode,
		Timestamp:  status.Timestamp,
	}

	m.eventPublisher.PublishProcessStatus(execution, clientStatus)

	// Check if process exited and should be auto-restarted
	// Only restart if this is the ACTUAL passthrough process, not user shell terminals
	// Run asynchronously to allow the old process to be cleaned up first
	if status.Status == agentctltypes.ProcessStatusExited || status.Status == agentctltypes.ProcessStatusFailed {
		// Only trigger auto-restart for the passthrough process, not for user shell terminals
		if execution.PassthroughProcessID != "" && status.ProcessID == execution.PassthroughProcessID {
			go m.handlePassthroughExit(execution, status)
		} else {
			m.logger.Debug("process exited but not the passthrough process, skipping auto-restart",
				zap.String("session_id", status.SessionID),
				zap.String("exited_process_id", status.ProcessID),
				zap.String("passthrough_process_id", execution.PassthroughProcessID))
		}
	}
}

// handlePassthroughExit handles auto-restart logic when a passthrough process exits.
// This function is called asynchronously to allow the old process to be cleaned up first.
func (m *Manager) handlePassthroughExit(execution *AgentExecution, status *agentctltypes.ProcessStatusUpdate) {
	const restartDelay = 500 * time.Millisecond
	const cleanupDelay = 100 * time.Millisecond // Wait for old process cleanup

	// Wait a bit for the old process to be cleaned up from the process map
	time.Sleep(cleanupDelay)

	sessionID := execution.SessionID

	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		m.logger.Debug("no interactive runner available for auto-restart",
			zap.String("session_id", sessionID))
		return
	}

	// Check if WebSocket is still connected (use session-level tracking which survives process deletion)
	if !interactiveRunner.HasActiveWebSocketBySession(sessionID) {
		m.logger.Debug("no active WebSocket, skipping auto-restart",
			zap.String("session_id", sessionID))
		return
	}

	exitCode := 0
	if status.ExitCode != nil {
		exitCode = *status.ExitCode
	}

	m.logger.Info("passthrough process exited with active WebSocket, attempting auto-restart",
		zap.String("session_id", sessionID),
		zap.Int("exit_code", exitCode))

	// Send restart notification to terminal (use session-level to survive process deletion)
	restartMsg := "\r\n\x1b[33m[Agent exited. Restarting...]\x1b[0m\r\n"
	if err := interactiveRunner.WriteToDirectOutputBySession(sessionID, []byte(restartMsg)); err != nil {
		m.logger.Debug("failed to write restart message to terminal",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Delay before restart
	time.Sleep(restartDelay)

	// Check WebSocket is still connected after delay (use session-level tracking)
	if !interactiveRunner.HasActiveWebSocketBySession(sessionID) {
		m.logger.Debug("WebSocket disconnected during restart delay, aborting",
			zap.String("session_id", sessionID))
		return
	}

	// Attempt restart
	ctx := context.Background()
	if err := m.ResumePassthroughSession(ctx, sessionID); err != nil {
		m.logger.Error("failed to auto-restart passthrough session",
			zap.String("session_id", sessionID),
			zap.Error(err))

		// Send error message to terminal
		errorMsg := fmt.Sprintf("\r\n\x1b[31m[Restart failed: %s]\x1b[0m\r\n", err.Error())
		if writeErr := interactiveRunner.WriteToDirectOutputBySession(sessionID, []byte(errorMsg)); writeErr != nil {
			m.logger.Debug("failed to write restart error message to terminal",
				zap.String("session_id", sessionID),
				zap.Error(writeErr))
		}
		return
	}

	// Connect the session's existing WebSocket to the new process
	if interactiveRunner.ConnectSessionWebSocket(execution.PassthroughProcessID) {
		m.logger.Info("passthrough session auto-restarted and reconnected WebSocket",
			zap.String("session_id", sessionID),
			zap.String("new_process_id", execution.PassthroughProcessID))
	} else {
		m.logger.Warn("passthrough session restarted but failed to reconnect WebSocket",
			zap.String("session_id", sessionID),
			zap.String("new_process_id", execution.PassthroughProcessID))
	}
}

// GetInteractiveRunner returns the interactive runner for passthrough mode.
// Returns nil if the runtime is not available or doesn't support passthrough.
func (m *Manager) GetInteractiveRunner() *process.InteractiveRunner {
	if m.executorRegistry == nil {
		return nil
	}
	standaloneRT, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return nil
	}
	return standaloneRT.GetInteractiveRunner()
}

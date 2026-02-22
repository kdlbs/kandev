package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/common/appctx"
)

// EnsureWorkspaceExecutionForSession ensures an agentctl execution exists for a specific task session.
// This is used when the frontend provides a session ID (e.g., from URL path /task/[id]/[sessionId]).
// If an execution already exists for the session, it returns it. Otherwise, it creates a new execution
// using the session's workspace configuration from the database.
func (m *Manager) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) (*AgentExecution, error) {
	// Check if execution already exists
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		return execution, nil
	}

	// Need to create a new execution - get workspace info for the specific session
	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("workspace info provider not configured")
	}

	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace info for session %s: %w", sessionID, err)
	}

	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("no workspace path available for session %s", sessionID)
	}

	m.logger.Info("creating execution for task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("acp_session_id", info.ACPSessionID))

	execution, err := m.createExecution(ctx, taskID, info)
	if err != nil {
		return nil, err
	}

	// For workspace-only executions (no agent), wait for agentctl to be ready
	// then connect the workspace stream so process output can be received
	go func() {
		// Use detached context that respects stopCh for graceful shutdown
		waitCtx, cancel := appctx.Detached(ctx, m.stopCh, 60*time.Second)
		defer cancel()

		if err := execution.agentctl.WaitForReady(waitCtx, 60*time.Second); err != nil {
			m.logger.Error("agentctl not ready for workspace stream connection",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
			return
		}

		// Connect workspace stream for process output (agent stream not needed for workspace-only)
		if m.streamManager != nil {
			m.logger.Info("connecting workspace stream for workspace-only execution",
				zap.String("execution_id", execution.ID))
			go m.streamManager.connectWorkspaceStream(execution, nil)
		}
	}()

	return execution, nil
}

// EnsurePassthroughExecution ensures an execution exists for a passthrough session
// and starts the passthrough process if needed. This is called when the terminal
// handler receives a connection for a session that might need recovery after backend restart.
//
// The sessionID is required. If taskID is empty, it will be looked up from:
// 1. The existing execution (if any)
// 2. The workspace info provider
//
// Returns the execution with a running passthrough process, or an error.
func (m *Manager) EnsurePassthroughExecution(ctx context.Context, sessionID string) (*AgentExecution, error) {
	// Check if execution already exists with a running passthrough process
	if execution, exists := m.executionStore.GetBySessionID(sessionID); exists {
		if execution.PassthroughProcessID != "" {
			return execution, nil
		}
		// Execution exists but no passthrough process - will try to start it
		return m.resumeExistingExecution(ctx, sessionID, execution)
	}

	// No execution exists - need to create one from session info
	return m.createExecutionFromSessionInfo(ctx, sessionID)
}

// resumeExistingExecution starts the passthrough process for an existing execution
// that has no running process (e.g., after backend restart).
func (m *Manager) resumeExistingExecution(ctx context.Context, sessionID string, execution *AgentExecution) (*AgentExecution, error) {
	m.logger.Info("execution exists but passthrough process not running, starting",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	if err := m.ResumePassthroughSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("resume passthrough session %s: %w", sessionID, err)
	}

	// Get updated execution with process ID
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("execution disappeared after resuming passthrough session %s", sessionID)
	}
	return execution, nil
}

// createExecutionFromSessionInfo creates a new execution for a passthrough session
// when no execution exists (e.g., backend restarted and execution store was cleared).
func (m *Manager) createExecutionFromSessionInfo(ctx context.Context, sessionID string) (*AgentExecution, error) {
	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("cannot restore session %s: workspace info provider not configured", sessionID)
	}

	// Get workspace info from the provider (looks up session to get taskID, workspace path, etc.)
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, "", sessionID)
	if err != nil {
		return nil, fmt.Errorf("get workspace info for session %s: %w", sessionID, err)
	}

	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("session %s has no workspace path configured", sessionID)
	}

	if info.TaskID == "" {
		return nil, fmt.Errorf("session %s has no associated task ID", sessionID)
	}

	// Verify this session should use passthrough mode
	if err := m.verifyPassthroughEnabled(ctx, sessionID, info.AgentProfileID); err != nil {
		return nil, err
	}

	// Create the execution
	m.logger.Info("creating execution for passthrough session",
		zap.String("task_id", info.TaskID),
		zap.String("session_id", sessionID),
		zap.String("workspace_path", info.WorkspacePath))

	execution, err := m.createExecution(ctx, info.TaskID, info)
	if err != nil {
		return nil, fmt.Errorf("create execution for session %s: %w", sessionID, err)
	}

	// Start the passthrough process using resume command (recovery after restart)
	m.logger.Info("starting passthrough process for session",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	if err := m.ResumePassthroughSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("start passthrough process for session %s: %w", sessionID, err)
	}

	// Get updated execution with process ID
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil, fmt.Errorf("execution disappeared after starting passthrough session %s", sessionID)
	}

	return execution, nil
}

// verifyPassthroughEnabled checks if the session's profile has CLI passthrough enabled.
func (m *Manager) verifyPassthroughEnabled(ctx context.Context, sessionID, profileID string) error {
	if m.profileResolver == nil || profileID == "" {
		return fmt.Errorf("session %s has no profile configured for passthrough mode", sessionID)
	}

	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		m.logger.Warn("failed to resolve profile for passthrough check",
			zap.String("session_id", sessionID),
			zap.String("profile_id", profileID),
			zap.Error(err))
		return fmt.Errorf("session %s: failed to resolve profile %s: %w", sessionID, profileID, err)
	}

	if profileInfo == nil || !profileInfo.CLIPassthrough {
		return fmt.Errorf("session %s is not configured for CLI passthrough mode", sessionID)
	}

	return nil
}

// createExecution creates an agentctl execution.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createExecution(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentExecution, error) {
	// Get the default runtime for on-demand execution creation
	rt, err := m.getDefaultExecutorBackend()
	if err != nil {
		return nil, fmt.Errorf("no runtime configured: %w", err)
	}

	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}

	executionID := uuid.New().String()

	agentConfig, ok := m.registry.Get(info.AgentID)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", info.AgentID)
	}

	req := &ExecutorCreateRequest{
		InstanceID:     executionID,
		TaskID:         taskID,
		SessionID:      info.SessionID,
		AgentProfileID: info.AgentProfileID,
		WorkspacePath:  info.WorkspacePath,
		Protocol:       string(agentConfig.Runtime().Protocol),
		AgentConfig:    agentConfig,
	}

	runtimeInstance, err := rt.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	execution := runtimeInstance.ToAgentExecution(req)
	execution.RuntimeName = string(rt.Name())

	// Set the ACP session ID for session resumption
	if info.ACPSessionID != "" {
		execution.ACPSessionID = info.ACPSessionID
	}

	// Create trace span for workspace-only execution
	_, sessionSpan := tracing.TraceSessionStart(
		context.Background(), taskID, info.SessionID, executionID,
	)
	execution.SetSessionSpan(sessionSpan)
	if execution.agentctl != nil {
		execution.agentctl.SetTraceContext(execution.SessionTraceContext())
	}

	m.executionStore.Add(execution)
	go m.pollOneRemoteStatus(context.Background(), execution)

	go m.waitForAgentctlReady(execution)

	m.logger.Info("execution created",
		zap.String("execution_id", executionID),
		zap.String("task_id", taskID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("runtime", execution.RuntimeName))

	return execution, nil
}

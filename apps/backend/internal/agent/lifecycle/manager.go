// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/worktree"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Manager manages agent instance lifecycles
type Manager struct {
	registry        *registry.Registry
	eventBus        bus.EventBus
	credsMgr        CredentialsManager
	profileResolver ProfileResolver
	worktreeMgr     *worktree.Manager
	logger          *logger.Logger

	// Agent runtime abstraction (Docker, Standalone, K8s, SSH, etc.)
	runtime Runtime

	// Refactored components for separation of concerns
	executionStore   *ExecutionStore   // Thread-safe execution tracking
	commandBuilder   *CommandBuilder   // Builds agent commands from registry config
	sessionManager   *SessionManager   // Handles ACP session initialization
	streamManager    *StreamManager    // Manages WebSocket streams
	eventPublisher   *EventPublisher   // Publishes lifecycle events
	containerManager *ContainerManager // Manages Docker containers (optional, nil for non-Docker runtimes)

	// Workspace info provider for on-demand instance creation
	workspaceInfoProvider WorkspaceInfoProvider

	// Background cleanup
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewManager creates a new lifecycle manager.
// The runtime parameter is the agent execution runtime (Docker, Standalone, etc.).
// The containerManager parameter is optional and used for Docker cleanup (pass nil for non-Docker runtimes).
func NewManager(
	reg *registry.Registry,
	eventBus bus.EventBus,
	runtime Runtime,
	containerManager *ContainerManager,
	credsMgr CredentialsManager,
	profileResolver ProfileResolver,
	log *logger.Logger,
) *Manager {
	componentLogger := log.WithFields(zap.String("component", "lifecycle-manager"))

	// Initialize command builder
	commandBuilder := NewCommandBuilder()

	// Initialize session manager
	sessionManager := NewSessionManager(log)

	// Initialize event publisher
	eventPublisher := NewEventPublisher(eventBus, log)

	// Initialize execution store
	executionStore := NewExecutionStore()

	mgr := &Manager{
		registry:         reg,
		eventBus:         eventBus,
		runtime:          runtime,
		credsMgr:         credsMgr,
		profileResolver:  profileResolver,
		logger:           componentLogger,
		executionStore:   executionStore,
		commandBuilder:   commandBuilder,
		sessionManager:   sessionManager,
		eventPublisher:   eventPublisher,
		containerManager: containerManager,
		cleanupInterval:  30 * time.Second,
		stopCh:           make(chan struct{}),
	}

	// Initialize stream manager with callbacks that delegate to manager methods
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnAgentEvent:  mgr.handleAgentEvent,
		OnGitStatus:   mgr.handleGitStatusUpdate,
		OnFileChange:  mgr.handleFileChangeNotification,
		OnShellOutput: mgr.handleShellOutput,
		OnShellExit:   mgr.handleShellExit,
	})

	// Set session manager dependencies for full orchestration
	sessionManager.SetDependencies(eventPublisher, mgr.streamManager, executionStore)

	if runtime != nil {
		mgr.logger.Info("initialized with runtime", zap.String("runtime", runtime.Name()))
	}

	return mgr
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
}

// SetWorkspaceInfoProvider sets the provider for workspace information
func (m *Manager) SetWorkspaceInfoProvider(provider WorkspaceInfoProvider) {
	m.workspaceInfoProvider = provider
}

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

	return m.createExecution(ctx, taskID, info)
}

// createExecution creates an agentctl execution.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createExecution(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentExecution, error) {
	if m.runtime == nil {
		return nil, fmt.Errorf("no runtime configured")
	}

	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}

	executionID := uuid.New().String()

	agentConfig, err := m.registry.Get(info.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent type %q not found in registry: %w", info.AgentID, err)
	}

	req := &RuntimeCreateRequest{
		InstanceID:     executionID,
		TaskID:         taskID,
		SessionID:      info.SessionID,
		AgentProfileID: info.AgentProfileID,
		WorkspacePath:  info.WorkspacePath,
		Protocol:       string(agentConfig.Protocol),
	}

	runtimeInstance, err := m.runtime.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	execution := runtimeInstance.ToAgentExecution(req)

	// Set the ACP session ID for session resumption
	if info.ACPSessionID != "" {
		execution.ACPSessionID = info.ACPSessionID
	}

	m.executionStore.Add(execution)

	go m.waitForAgentctlReady(execution)

	m.logger.Info("execution created",
		zap.String("execution_id", executionID),
		zap.String("task_id", taskID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("runtime", m.runtime.Name()))

	return execution, nil
}

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	runtimeName := "none"
	if m.runtime != nil {
		runtimeName = m.runtime.Name()
	}
	m.logger.Info("starting lifecycle manager", zap.String("runtime", runtimeName))

	if m.runtime == nil {
		m.logger.Warn("no runtime configured")
		return nil
	}

	// Check runtime health
	if err := m.runtime.HealthCheck(ctx); err != nil {
		m.logger.Warn("runtime health check failed",
			zap.String("runtime", runtimeName),
			zap.Error(err))
		// Continue anyway - it might come up later
	} else {
		m.logger.Info("runtime is healthy", zap.String("runtime", runtimeName))
	}

	// Try to recover executions from previous runs
	recovered, err := m.runtime.RecoverInstances(ctx)
	if err != nil {
		m.logger.Warn("failed to recover executions", zap.Error(err))
	} else if len(recovered) > 0 {
		for _, ri := range recovered {
			execution := &AgentExecution{
				ID:                   ri.InstanceID,
				TaskID:               ri.TaskID,
				SessionID:            ri.SessionID,
				ContainerID:          ri.ContainerID,
				ContainerIP:          ri.ContainerIP,
				WorkspacePath:        ri.WorkspacePath,
				Status:               v1.AgentStatusRunning,
				StartedAt:            time.Now(),
				Metadata:             ri.Metadata,
				agentctl:             ri.Client,
				standaloneInstanceID: ri.StandaloneInstanceID,
				standalonePort:       ri.StandalonePort,
			}
			m.executionStore.Add(execution)
		}
		m.logger.Info("recovered executions", zap.Int("count", len(recovered)))
	}

	// Start cleanup loop when container manager is available (Docker mode)
	if m.containerManager != nil {
		m.wg.Add(1)
		go m.cleanupLoop(ctx)
		m.logger.Info("cleanup loop started")
	}

	return nil
}

// GetRecoveredExecutions returns a snapshot of all currently tracked executions
// This can be used by the orchestrator to sync with the database
func (m *Manager) GetRecoveredExecutions() []RecoveredExecution {
	executions := m.executionStore.List()
	result := make([]RecoveredExecution, 0, len(executions))
	for _, exec := range executions {
		result = append(result, RecoveredExecution{
			ExecutionID:    exec.ID,
			TaskID:         exec.TaskID,
			SessionID:      exec.SessionID,
			ContainerID:    exec.ContainerID,
			AgentProfileID: exec.AgentProfileID,
		})
	}
	return result
}

// Stop stops the lifecycle manager
func (m *Manager) Stop() error {
	m.logger.Info("stopping lifecycle manager")

	close(m.stopCh)
	m.wg.Wait()

	return nil
}

// StopAllAgents attempts a graceful shutdown of all active agents.
func (m *Manager) StopAllAgents(ctx context.Context) error {
	executions := m.executionStore.List()
	if len(executions) == 0 {
		return nil
	}

	var errs []error
	for _, exec := range executions {
		if err := m.StopAgent(ctx, exec.ID, false); err != nil {
			errs = append(errs, err)
			m.logger.Warn("failed to stop agent during shutdown",
				zap.String("execution_id", exec.ID),
				zap.Error(err))
		}
	}

	return errors.Join(errs...)
}

// Launch launches a new agent for a task
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
	m.logger.Info("launching agent",
		zap.String("task_id", req.TaskID),
		zap.String("agent_profile_id", req.AgentProfileID),
		zap.Bool("use_worktree", req.UseWorktree))

	// 1. Resolve the agent profile to get agent type info
	var agentTypeName string
	var profileInfo *AgentProfileInfo
	var err error
	if m.profileResolver != nil {
		profileInfo, err = m.profileResolver.ResolveProfile(ctx, req.AgentProfileID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve agent profile: %w", err)
		}
		// Map agent name to registry ID (e.g., "auggie" -> "auggie-agent")
		agentTypeName = profileInfo.AgentName + "-agent"
		m.logger.Info("resolved agent profile",
			zap.String("profile_id", req.AgentProfileID),
			zap.String("agent_name", profileInfo.AgentName),
			zap.String("agent_type", agentTypeName))
	} else {
		// Fallback: treat AgentProfileID as agent type directly (for backward compat)
		agentTypeName = req.AgentProfileID
		m.logger.Warn("no profile resolver configured, using profile ID as agent type",
			zap.String("agent_type", agentTypeName))
	}

	// 2. Get agent config from registry
	agentConfig, err := m.registry.Get(agentTypeName)
	if err != nil {
		return nil, fmt.Errorf("agent type %q not found in registry: %w", agentTypeName, err)
	}

	if !agentConfig.Enabled {
		return nil, fmt.Errorf("agent type %q is disabled", agentTypeName)
	}

	// 3. Check if session already has an agent running
	if req.SessionID != "" {
		if existingExecution, exists := m.executionStore.GetBySessionID(req.SessionID); exists {
			return nil, fmt.Errorf("session %q already has an agent running (execution: %s)", req.SessionID, existingExecution.ID)
		}
	}

	// 4. Handle worktree creation/reuse if enabled
	workspacePath := req.WorkspacePath
	var mainRepoGitDir string // Path to main repo's .git directory for container mount
	var worktreeID string     // Store worktree ID for session resumption
	var worktreeBranch string // Store worktree branch for API responses
	if req.UseWorktree && m.worktreeMgr != nil && req.RepositoryPath != "" {
		wt, err := m.getOrCreateWorktree(ctx, req)
		if err != nil {
			m.logger.Warn("failed to create worktree, falling back to direct mount",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
			// Fall back to direct mount if worktree creation fails
			// Use RepositoryPath as the workspace
			workspacePath = req.RepositoryPath
		} else {
			workspacePath = wt.Path
			worktreeID = wt.ID
			worktreeBranch = wt.Branch
			// Git worktrees reference the main repo's .git directory via a .git file
			// The worktree metadata in .git/worktrees/{name} contains a 'commondir' file
			// that points back to the main .git directory (usually '../..')
			// We need to mount the entire .git directory for git commands to work
			mainRepoGitDir = filepath.Join(req.RepositoryPath, ".git")
		}
	} else if req.RepositoryPath != "" && workspacePath == "" {
		// No worktree, but we have a repository path - use it as workspace
		workspacePath = req.RepositoryPath
	}

	// 5. Generate a new execution ID
	executionID := uuid.New().String()

	// 6. Prepare request with worktree path
	reqWithWorktree := *req
	reqWithWorktree.WorkspacePath = workspacePath

	// Store task description in metadata for StartAgentProcess
	if reqWithWorktree.Metadata == nil {
		reqWithWorktree.Metadata = make(map[string]interface{})
	}
	if req.TaskDescription != "" {
		reqWithWorktree.Metadata["task_description"] = req.TaskDescription
	}
	if req.SessionID != "" {
		reqWithWorktree.Metadata["session_id"] = req.SessionID
	}

	// 6b. Add profile settings to environment
	if profileInfo != nil {
		if reqWithWorktree.Env == nil {
			reqWithWorktree.Env = make(map[string]string)
		}
		if profileInfo.Model != "" {
			reqWithWorktree.Env["AGENT_MODEL"] = profileInfo.Model
		}
		if profileInfo.AutoApprove {
			reqWithWorktree.Env["AGENTCTL_AUTO_APPROVE_PERMISSIONS"] = "true"
		}
		if profileInfo.DangerouslySkipPermissions {
			reqWithWorktree.Env["AGENT_DANGEROUSLY_SKIP_PERMISSIONS"] = "true"
		}
		if profileInfo.Plan != "" {
			reqWithWorktree.Env["AGENT_PLAN"] = profileInfo.Plan
		}
	}

	// 7. Launch via runtime - creates agentctl instance with workspace access only
	// Agent subprocess is NOT started - call StartAgentProcess() explicitly
	if m.runtime == nil {
		return nil, fmt.Errorf("no runtime configured")
	}

	// Build environment variables
	env := m.buildEnvForRuntime(executionID, &reqWithWorktree, agentConfig)

	// Create runtime request (agent command not included - started explicitly later)
	runtimeReq := &RuntimeCreateRequest{
		InstanceID:     executionID,
		TaskID:         reqWithWorktree.TaskID,
		SessionID:      reqWithWorktree.SessionID,
		AgentProfileID: reqWithWorktree.AgentProfileID,
		WorkspacePath:  reqWithWorktree.WorkspacePath,
		Protocol:       string(agentConfig.Protocol),
		Env:            env,
		Metadata:       reqWithWorktree.Metadata,
		WorktreeID:     worktreeID,
		WorktreeBranch: worktreeBranch,
		MainRepoGitDir: mainRepoGitDir,
		AgentConfig:    agentConfig,
	}

	runtimeInstance, err := m.runtime.CreateInstance(ctx, runtimeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Convert to AgentExecution
	execution := runtimeInstance.ToAgentExecution(runtimeReq)

	// Set ACP session ID for session resumption (used by InitializeSession)
	if req.ACPSessionID != "" {
		execution.ACPSessionID = req.ACPSessionID
	}

	// Build agent command string for later use with StartAgentProcess
	model := ""
	autoApprove := false
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
	}
	cmdOpts := CommandOptions{
		Model:       model,
		SessionID:   req.ACPSessionID,
		AutoApprove: autoApprove,
	}
	execution.AgentCommand = m.commandBuilder.BuildCommandString(agentConfig, cmdOpts)

	// 8. Track the execution
	m.executionStore.Add(execution)

	// 9. Publish agent.started event
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStarted, execution)
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlStarting, execution, "")

	// 10. Wait for agentctl to be ready (for shell/workspace access)
	// NOTE: This does NOT start the agent process - call StartAgentProcess() explicitly
	go m.waitForAgentctlReady(execution)

	runtimeName := "unknown"
	if m.runtime != nil {
		runtimeName = m.runtime.Name()
	}
	m.logger.Info("agentctl execution created (agent not started)",
		zap.String("execution_id", executionID),
		zap.String("task_id", req.TaskID),
		zap.String("runtime", runtimeName))

	return execution, nil
}

// StartAgentProcess configures and starts the agent subprocess for an execution.
// This must be called after Launch() to actually start the agent (e.g., auggie, codex).
// The command is built internally based on the execution's agent profile.
func (m *Manager) StartAgentProcess(ctx context.Context, executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}

	if execution.AgentCommand == "" {
		return fmt.Errorf("execution %q has no agent command configured", executionID)
	}

	// Wait for agentctl to be ready
	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.updateExecutionError(executionID, "agentctl not ready: "+err.Error())
		return fmt.Errorf("agentctl not ready: %w", err)
	}

	// Get task description from metadata
	taskDescription := ""
	if execution.Metadata != nil {
		if desc, ok := execution.Metadata["task_description"].(string); ok {
			taskDescription = desc
		}
	}

	// Build environment for the agent process
	env := map[string]string{}
	if taskDescription != "" {
		env["TASK_DESCRIPTION"] = taskDescription
	}

	// Configure the agent command
	if err := execution.agentctl.ConfigureAgent(ctx, execution.AgentCommand, env); err != nil {
		return fmt.Errorf("failed to configure agent: %w", err)
	}

	// Start the agent process
	if err := execution.agentctl.Start(ctx); err != nil {
		m.updateExecutionError(executionID, "failed to start agent: "+err.Error())
		return fmt.Errorf("failed to start agent: %w", err)
	}

	m.logger.Info("agent process started",
		zap.String("execution_id", executionID),
		zap.String("task_id", execution.TaskID),
		zap.String("command", execution.AgentCommand))

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	// Get agent config for ACP session initialization
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	// Initialize ACP session
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription); err != nil {
		m.updateExecutionError(executionID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	m.emitSessionStatusEvent(execution, agentConfig)

	return nil
}

// buildEnvForRuntime builds environment variables for any runtime.
// This is the unified method used by the runtime interface.
func (m *Manager) buildEnvForRuntime(executionID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig) map[string]string {
	env := make(map[string]string)

	// Copy request environment
	for k, v := range req.Env {
		env[k] = v
	}

	// Add standard variables for recovery after backend restart
	env["KANDEV_INSTANCE_ID"] = executionID
	env["KANDEV_TASK_ID"] = req.TaskID
	env["KANDEV_SESSION_ID"] = req.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = req.AgentProfileID
	env["TASK_DESCRIPTION"] = req.TaskDescription

	// Add required credentials from agent config
	if m.credsMgr != nil && agentConfig != nil {
		ctx := context.Background()
		for _, credKey := range agentConfig.RequiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}

	return env
}

// getOrCreateWorktree creates a new worktree or reuses an existing one for session resumption.
// If worktree_id is in metadata, it tries to reuse that specific worktree.
// Otherwise, creates a new worktree with a unique random suffix.
func (m *Manager) getOrCreateWorktree(ctx context.Context, req *LaunchRequest) (*worktree.Worktree, error) {
	// Check if we have a worktree_id in metadata for session resumption
	var worktreeID string
	if req.Metadata != nil {
		if id, ok := req.Metadata["worktree_id"].(string); ok && id != "" {
			worktreeID = id
		}
	}

	// Create request with optional WorktreeID for resumption
	createReq := worktree.CreateRequest{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		TaskTitle:            req.TaskTitle,
		RepositoryID:         req.RepositoryID,
		RepositoryPath:       req.RepositoryPath,
		BaseBranch:           req.BaseBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		WorktreeID:           worktreeID, // If set, will try to reuse this worktree
	}

	wt, err := m.worktreeMgr.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	if worktreeID != "" && wt.ID == worktreeID {
		m.logger.Info("reusing existing worktree for session resumption",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("worktree_path", wt.Path),
			zap.String("branch", wt.Branch))
	} else {
		m.logger.Info("created new worktree for task",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID),
			zap.String("worktree_path", wt.Path),
			zap.String("branch", wt.Branch))
	}

	return wt, nil
}

// waitForAgentctlReady waits for the agentctl HTTP server to be ready.
// This enables shell/workspace features without starting the agent process.
func (m *Manager) waitForAgentctlReady(execution *AgentExecution) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m.logger.Info("waiting for agentctl to be ready",
		zap.String("execution_id", execution.ID),
		zap.String("url", execution.agentctl.BaseURL()))

	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("execution_id", execution.ID),
			zap.Error(err))
		m.updateExecutionError(execution.ID, "agentctl not ready: "+err.Error())
		m.eventPublisher.PublishAgentctlEvent(context.Background(), events.AgentctlError, execution, err.Error())
		return
	}

	m.logger.Info("agentctl ready - shell/workspace access available",
		zap.String("execution_id", execution.ID))
	m.eventPublisher.PublishAgentctlEvent(context.Background(), events.AgentctlReady, execution, "")
}

// getAgentConfigForExecution retrieves the agent configuration for an execution.
// The execution must have AgentCommand set (which includes the agent type).
func (m *Manager) getAgentConfigForExecution(execution *AgentExecution) (*registry.AgentTypeConfig, error) {
	if execution.AgentProfileID == "" {
		return nil, fmt.Errorf("execution %s has no agent profile ID", execution.ID)
	}

	if m.profileResolver == nil {
		return nil, fmt.Errorf("profile resolver not configured")
	}

	profileInfo, err := m.profileResolver.ResolveProfile(context.Background(), execution.AgentProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve profile: %w", err)
	}

	// Map agent name to registry ID (e.g., "auggie" -> "auggie-agent")
	agentTypeName := profileInfo.AgentName + "-agent"
	agentConfig, err := m.registry.Get(agentTypeName)
	if err != nil {
		return nil, fmt.Errorf("agent type not found: %s", agentTypeName)
	}

	return agentConfig, nil
}

// initializeACPSession delegates to SessionManager for full ACP session initialization and prompting
func (m *Manager) initializeACPSession(ctx context.Context, execution *AgentExecution, agentConfig *registry.AgentTypeConfig, taskDescription string) error {
	return m.sessionManager.InitializeAndPrompt(ctx, execution, agentConfig, taskDescription, m.MarkReady)
}

// emitSessionStatusEvent emits a session_status event indicating whether the session was resumed or new.
func (m *Manager) emitSessionStatusEvent(execution *AgentExecution, agentConfig *registry.AgentTypeConfig) {
	wasResumed := false
	if execution.ACPSessionID != "" {
		if agentConfig.SessionConfig.ResumeViaACP {
			wasResumed = true
		} else if agentConfig.SessionConfig.ResumeFlag != "" && agentConfig.SessionConfig.SupportsRecovery() {
			wasResumed = true
		}
	}

	sessionStatus := streams.SessionStatusNew
	if wasResumed {
		sessionStatus = streams.SessionStatusResumed
	}

	m.eventPublisher.PublishAgentStreamEvent(execution, streams.AgentEvent{
		Type:          streams.EventTypeSessionStatus,
		SessionID:     execution.ACPSessionID,
		SessionStatus: sessionStatus,
	})
}

// handlePermissionRequestEvent processes permission requests from the unified agent event stream
func (m *Manager) handlePermissionRequestEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	m.eventPublisher.PublishPermissionRequest(execution, event)
}

// handleAgentEvent processes incoming agent events from the agent
func (m *Manager) handleAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	// Handle different event types based on the Type field
	switch event.Type {
	case "message_chunk":
		m.updateExecutionProgress(execution.ID, 50)

		// Accumulate message content for saving as comment when a step completes
		if event.Text != "" {
			execution.messageMu.Lock()
			execution.messageBuffer.WriteString(event.Text)
			execution.messageMu.Unlock()
		}

	case "reasoning":
		// Accumulate reasoning/thinking content
		execution.messageMu.Lock()
		if event.ReasoningText != "" {
			execution.reasoningBuffer.WriteString(event.ReasoningText)
		}
		if event.ReasoningSummary != "" {
			execution.summaryBuffer.WriteString(event.ReasoningSummary)
		}
		execution.messageMu.Unlock()

	case "tool_call":
		// Tool call starting marks a step boundary - flush the accumulated message
		// This way, each agent response before a tool use becomes a separate comment
		// Include the flushed text in the event for the orchestrator to save
		if flushedText := m.flushMessageBuffer(execution); flushedText != "" {
			event.Text = flushedText
		}

		m.logger.Debug("tool call started",
			zap.String("execution_id", execution.ID),
			zap.String("tool_call_id", event.ToolCallID),
			zap.String("tool_name", event.ToolName))
		m.updateExecutionProgress(execution.ID, 60)
		// Tool call message creation is handled by orchestrator via AgentStreamEvent

	case "tool_update":
		// Check if tool call completed - orchestrator will create/update the message
		switch event.ToolStatus {
		case "complete", "completed":
			m.updateExecutionProgress(execution.ID, 80)
		case "error", "failed":
			// Error status is passed through the stream event
		}

	case "plan":
		m.logger.Debug("agent plan update",
			zap.String("execution_id", execution.ID))

	case "error":
		m.logger.Error("agent error",
			zap.String("execution_id", execution.ID),
			zap.String("error", event.Error))

	case "complete":
		m.logger.Info("agent turn complete",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", event.SessionID))

		// Flush accumulated message buffer and include in the event
		// The orchestrator will save this as an agent message
		if flushedText := m.flushMessageBuffer(execution); flushedText != "" {
			event.Text = flushedText
		}

		// Mark agent as READY for follow-up prompts
		if err := m.MarkReady(execution.ID); err != nil {
			m.logger.Error("failed to mark execution as ready after complete",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
		}

	case "permission_request":
		m.logger.Debug("permission request received",
			zap.String("execution_id", execution.ID),
			zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))

		// Handle permission request inline
		m.handlePermissionRequestEvent(execution, event)
	}

	// Publish agent stream event to event bus for WebSocket streaming
	m.eventPublisher.PublishAgentStreamEvent(execution, event)
}

// handleGitStatusUpdate processes git status updates from the workspace tracker
func (m *Manager) handleGitStatusUpdate(execution *AgentExecution, update *agentctl.GitStatusUpdate) {
	// Store git status in execution metadata
	m.executionStore.UpdateMetadata(execution.ID, func(metadata map[string]interface{}) map[string]interface{} {
		metadata["git_status"] = map[string]interface{}{
			"branch":        update.Branch,
			"remote_branch": update.RemoteBranch,
			"modified":      update.Modified,
			"added":         update.Added,
			"deleted":       update.Deleted,
			"untracked":     update.Untracked,
			"renamed":       update.Renamed,
			"ahead":         update.Ahead,
			"behind":        update.Behind,
			"timestamp":     update.Timestamp,
		}
		return metadata
	})

	// Publish git status update to event bus for WebSocket streaming
	m.eventPublisher.PublishGitStatus(execution, update)
}

// handleFileChangeNotification processes file change notifications from the workspace tracker
func (m *Manager) handleFileChangeNotification(execution *AgentExecution, notification *agentctl.FileChangeNotification) {
	m.eventPublisher.PublishFileChange(execution, notification)
}

// handleShellOutput processes shell output from the workspace stream
func (m *Manager) handleShellOutput(execution *AgentExecution, data string) {
	m.eventPublisher.PublishShellOutput(execution, data)
}

// handleShellExit processes shell exit events from the workspace stream
func (m *Manager) handleShellExit(execution *AgentExecution, code int) {
	m.eventPublisher.PublishShellExit(execution, code)
}

// flushMessageBuffer extracts any accumulated message from the buffer and returns it.
// This is called when a tool use starts or on complete to get the agent's response.
func (m *Manager) flushMessageBuffer(execution *AgentExecution) string {
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	execution.messageMu.Unlock()

	return strings.TrimSpace(agentMessage)
}

// updateExecutionProgress updates an execution's progress
func (m *Manager) updateExecutionProgress(executionID string, progress int) {
	m.executionStore.UpdateProgress(executionID, progress)
}

// updateExecutionError updates an execution with an error
func (m *Manager) updateExecutionError(executionID, errorMsg string) {
	m.executionStore.UpdateError(executionID, errorMsg)
}

// PromptAgent sends a follow-up prompt to a running agent
func (m *Manager) PromptAgent(ctx context.Context, executionID string, prompt string) (*PromptResult, error) {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return nil, fmt.Errorf("execution %q not found", executionID)
	}
	return m.sessionManager.SendPrompt(ctx, execution, prompt, true, m.MarkReady)
}

// StopAgent stops an agent execution
func (m *Manager) StopAgent(ctx context.Context, executionID string, force bool) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	runtimeName := "unknown"
	if m.runtime != nil {
		runtimeName = m.runtime.Name()
	}
	m.logger.Info("stopping agent",
		zap.String("execution_id", executionID),
		zap.Bool("force", force),
		zap.String("runtime", runtimeName))

	// Try to gracefully stop via agentctl first
	if execution.agentctl != nil && !force {
		if err := execution.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("execution_id", executionID),
				zap.Error(err))
		}
		execution.agentctl.Close()
	}

	// Stop the agent execution via runtime
	if m.runtime != nil {
		runtimeInstance := &RuntimeInstance{
			InstanceID:           execution.ID,
			TaskID:               execution.TaskID,
			ContainerID:          execution.ContainerID,
			StandaloneInstanceID: execution.standaloneInstanceID,
			StandalonePort:       execution.standalonePort,
		}
		if err := m.runtime.StopInstance(ctx, runtimeInstance, force); err != nil {
			return err
		}
	}

	// Update execution status and remove from tracking
	m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		exec.Status = v1.AgentStatusStopped
		now := time.Now()
		exec.FinishedAt = &now
	})
	m.executionStore.Remove(executionID)

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

// GetExecution returns an agent execution by ID
func (m *Manager) GetExecution(executionID string) (*AgentExecution, bool) {
	return m.executionStore.Get(executionID)
}

// GetExecutionBySessionID returns the agent execution for a session
func (m *Manager) GetExecutionBySessionID(sessionID string) (*AgentExecution, bool) {
	return m.executionStore.GetBySessionID(sessionID)
}

// GetExecutionByContainerID returns the agent execution for a container
func (m *Manager) GetExecutionByContainerID(containerID string) (*AgentExecution, bool) {
	return m.executionStore.GetByContainerID(containerID)
}

// ListExecutions returns all active executions
func (m *Manager) ListExecutions() []*AgentExecution {
	return m.executionStore.List()
}

// IsAgentRunningForSession checks if an agent is actually running for a session
// This probes agentctl's status endpoint to verify the agent process is running
func (m *Manager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	// First check if we have an execution tracked for this session
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists {
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
	if !m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.Status = status
	}) {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.logger.Debug("updated execution status",
		zap.String("execution_id", executionID),
		zap.String("status", string(status)))

	return nil
}

// UpdateProgress updates the progress of an execution
func (m *Manager) UpdateProgress(executionID string, progress int) error {
	if !m.executionStore.WithLock(executionID, func(execution *AgentExecution) {
		execution.Progress = progress
	}) {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.logger.Debug("updated execution progress",
		zap.String("execution_id", executionID),
		zap.Int("progress", progress))

	return nil
}

// MarkReady marks an execution as ready for follow-up prompts
func (m *Manager) MarkReady(executionID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.executionStore.UpdateStatus(executionID, v1.AgentStatusReady)

	m.logger.Info("execution ready for follow-up prompts",
		zap.String("execution_id", executionID))

	// Publish ready event
	m.eventPublisher.PublishAgentEvent(context.Background(), events.AgentReady, execution)

	return nil
}

// MarkCompleted marks an execution as completed
func (m *Manager) MarkCompleted(executionID string, exitCode int, errorMessage string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
		now := time.Now()
		exec.FinishedAt = &now
		exec.ExitCode = &exitCode
		exec.ErrorMessage = errorMessage

		if exitCode == 0 && errorMessage == "" {
			exec.Status = v1.AgentStatusCompleted
			exec.Progress = 100
		} else {
			exec.Status = v1.AgentStatusFailed
		}
	})

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

// RemoveExecution removes a completed execution from tracking
func (m *Manager) RemoveExecution(executionID string) {
	m.executionStore.Remove(executionID)
	m.logger.Debug("removed execution from tracking",
		zap.String("execution_id", executionID))
}

// CleanupStaleExecutionBySessionID removes a stale agent execution from tracking without trying to stop it.
// This is used when we detect the agent process has stopped but the execution is still tracked.
func (m *Manager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil // No execution to clean up
	}

	m.logger.Info("cleaning up stale agent execution",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	// Close agentctl connection if it exists
	if execution.agentctl != nil {
		execution.agentctl.Close()
	}

	// Remove from execution store
	m.executionStore.Remove(execution.ID)

	return nil
}

// cleanupLoop runs periodic cleanup of stale containers
func (m *Manager) cleanupLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("cleanup loop stopped (context cancelled)")
			return
		case <-m.stopCh:
			m.logger.Info("cleanup loop stopped")
			return
		case <-ticker.C:
			m.performCleanup(ctx)
		}
	}
}

// performCleanup checks for and cleans up stale containers (Docker mode only)
func (m *Manager) performCleanup(ctx context.Context) {
	m.logger.Debug("running cleanup check")

	// Skip cleanup if container manager is not available
	if m.containerManager == nil {
		m.logger.Debug("skipping cleanup - no container manager")
		return
	}

	// List all kandev-managed containers
	containers, err := m.containerManager.ListManagedContainers(ctx)
	if err != nil {
		m.logger.Error("failed to list containers for cleanup", zap.Error(err))
		return
	}

	for _, container := range containers {
		// Check if container is exited and we're tracking it
		if container.State == "exited" {
			execution, tracked := m.executionStore.GetByContainerID(container.ID)
			if tracked {
				// Get container info to get exit code
				info, err := m.containerManager.GetContainerInfo(ctx, container.ID)
				if err != nil {
					m.logger.Warn("failed to get container info during cleanup",
						zap.String("container_id", container.ID),
						zap.Error(err))
					continue
				}

				// Mark execution as completed
				errorMsg := ""
				if info.ExitCode != 0 {
					errorMsg = fmt.Sprintf("container exited with code %d", info.ExitCode)
				}
				_ = m.MarkCompleted(execution.ID, info.ExitCode, errorMsg)

				// Remove the container
				if err := m.containerManager.RemoveContainer(ctx, container.ID, false); err != nil {
					m.logger.Warn("failed to remove container during cleanup",
						zap.String("container_id", container.ID),
						zap.Error(err))
				}

				// Remove the execution from tracking so new agents can be launched
				m.RemoveExecution(execution.ID)
			}
		}
	}
}

// RespondToPermission sends a response to a permission request for an agent execution
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

// RespondToPermissionBySessionID sends a response to a permission request for a session
func (m *Manager) RespondToPermissionBySessionID(sessionID, pendingID, optionID string, cancelled bool) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return fmt.Errorf("no agent execution found for session: %s", sessionID)
	}

	return m.RespondToPermission(execution.ID, pendingID, optionID, cancelled)
}

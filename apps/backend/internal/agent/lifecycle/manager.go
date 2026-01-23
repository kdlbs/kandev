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

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/worktree"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/appctx"
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
	mcpProvider     McpConfigProvider
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
	mcpProvider McpConfigProvider,
	log *logger.Logger,
) *Manager {
	componentLogger := log.WithFields(zap.String("component", "lifecycle-manager"))

	// Initialize command builder
	commandBuilder := NewCommandBuilder()

	// Create stop channel for graceful shutdown
	stopCh := make(chan struct{})

	// Initialize session manager
	sessionManager := NewSessionManager(log, stopCh)

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
		mcpProvider:      mcpProvider,
		logger:           componentLogger,
		executionStore:   executionStore,
		commandBuilder:   commandBuilder,
		sessionManager:   sessionManager,
		eventPublisher:   eventPublisher,
		containerManager: containerManager,
		cleanupInterval:  30 * time.Second,
		stopCh:           stopCh,
	}

	// Initialize stream manager with callbacks that delegate to manager methods
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnAgentEvent:    mgr.handleAgentEvent,
		OnGitStatus:     mgr.handleGitStatusUpdate,
		OnFileChange:    mgr.handleFileChangeNotification,
		OnShellOutput:   mgr.handleShellOutput,
		OnShellExit:     mgr.handleShellExit,
		OnProcessOutput: mgr.handleProcessOutput,
		OnProcessStatus: mgr.handleProcessStatus,
	})

	// Set session manager dependencies for full orchestration
	sessionManager.SetDependencies(eventPublisher, mgr.streamManager, executionStore)

	if runtime != nil {
		mgr.logger.Info("initialized with runtime", zap.String("runtime", string(runtime.Name())))
	}

	return mgr
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation.
//
// This must be called before launching agents if Git worktree support is enabled in the runtime.
// The worktree manager creates isolated Git working directories for each agent execution,
// allowing multiple agents to work on the same repository without conflicts.
//
// Call this during initialization, typically when setting up the orchestrator service.
// If not set, agents will work directly in the repository's main working directory.
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
}

// SetWorkspaceInfoProvider sets the provider for workspace information.
//
// The WorkspaceInfoProvider interface allows the lifecycle manager to dynamically create
// agent executions on-demand when the frontend connects to a session that doesn't have
// an active execution yet. This enables session resume after server restart or when
// accessing a session via URL (/task/[id]/[sessionId]).
//
// The provider must implement:
//   - GetWorkspaceInfoBySessionID(ctx, sessionID) - Returns workspace path, worktree info,
//     and MCP servers configured for the session
//
// This is typically called during initialization, with the task service as the provider.
// Without this, EnsureWorkspaceExecutionForSession will fail.
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
			go m.streamManager.connectWorkspaceStream(execution)
		}
	}()

	return execution, nil
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
		zap.String("runtime", string(m.runtime.Name())))

	return execution, nil
}
// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	runtimeName := "none"
	if m.runtime != nil {
		runtimeName = string(m.runtime.Name())
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

	// Resolve MCP servers early so they're available for protocols that need them at startup (e.g., Codex)
	acpMcpServers, err := m.resolveMcpServersWithParams(ctx, reqWithWorktree.AgentProfileID, reqWithWorktree.Metadata, agentConfig)
	if err != nil {
		m.logger.Warn("failed to resolve MCP servers for launch", zap.Error(err))
		// Continue without MCP servers - not a fatal error
	}

	// Convert ACP MCP servers to runtime config format
	var mcpServers []McpServerConfig
	for _, srv := range acpMcpServers {
		mcpServers = append(mcpServers, McpServerConfig{
			Name:    srv.Name,
			URL:     srv.URL,
			Type:    srv.Type,
			Command: srv.Command,
			Args:    srv.Args,
		})
	}

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
		McpServers:     mcpServers,
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
	// Allow model override from request (for dynamic model switching)
	if req.ModelOverride != "" {
		model = req.ModelOverride
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
		runtimeName = string(m.runtime.Name())
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

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		m.updateExecutionError(executionID, "failed to resolve MCP config: "+err.Error())
		return fmt.Errorf("failed to resolve MCP config: %w", err)
	}

	// Capture the original ACP session ID before initialization overwrites it.
	// This is used to determine if we're resuming an existing session or starting a new one.
	providedACPSessionID := execution.ACPSessionID

	// Initialize ACP session
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, mcpServers); err != nil {
		m.updateExecutionError(executionID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	m.emitSessionStatusEvent(execution, agentConfig, providedACPSessionID)

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
	opStart := time.Now()
	// Use detached context that respects stopCh for graceful shutdown
	ctx, cancel := appctx.Detached(context.Background(), m.stopCh, 60*time.Second)
	defer cancel()

	m.logger.Info("waiting for agentctl to be ready",
		zap.String("execution_id", execution.ID),
		zap.String("url", execution.agentctl.BaseURL()))

	if err := execution.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", time.Since(opStart)),
			zap.Error(err))
		m.updateExecutionError(execution.ID, "agentctl not ready: "+err.Error())
		// Use the timeout context for event publishing instead of a fresh Background context
		m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlError, execution, err.Error())
		return
	}

	elapsed := time.Since(opStart)
	if elapsed > 10*time.Second {
		m.logger.Warn("agentctl ready took longer than expected",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	} else {
		m.logger.Info("agentctl ready - shell/workspace access available",
			zap.String("execution_id", execution.ID),
			zap.Duration("duration", elapsed))
	}
	// Use the timeout context for event publishing instead of a fresh Background context
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")
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

// ResolveAgentProfile resolves an agent profile ID to profile information.
// This is exposed for external callers (like the orchestrator executor) to get profile info.
// The profile's model is guaranteed to be non-empty as it's validated at creation time.
func (m *Manager) ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.profileResolver == nil {
		return nil, fmt.Errorf("profile resolver not configured")
	}
	return m.profileResolver.ResolveProfile(ctx, profileID)
}

// resolveMcpServers centralizes MCP resolution for a session:
// - loads per-agent MCP config,
// - applies executor-scoped transport rules, allow/deny lists, URL rewrites, and env injection,
// - converts to ACP stdio server definitions used during session initialization.
func (m *Manager) resolveMcpServers(ctx context.Context, execution *AgentExecution, agentConfig *registry.AgentTypeConfig) ([]agentctltypes.McpServer, error) {
	if execution == nil {
		return nil, nil
	}
	return m.resolveMcpServersWithParams(ctx, execution.AgentProfileID, execution.Metadata, agentConfig)
}

// resolveMcpServersWithParams resolves MCP servers with explicit parameters.
// This is used by Launch() before the execution object exists.
func (m *Manager) resolveMcpServersWithParams(ctx context.Context, profileID string, metadata map[string]interface{}, agentConfig *registry.AgentTypeConfig) ([]agentctltypes.McpServer, error) {
	if m.mcpProvider == nil || agentConfig == nil {
		return nil, nil
	}

	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, nil
	}

	config, err := m.mcpProvider.GetConfigByProfileID(ctx, profileID)
	if err != nil {
		if errors.Is(err, mcpconfig.ErrAgentMcpUnsupported) || errors.Is(err, mcpconfig.ErrAgentProfileNotFound) {
			return nil, nil
		}
		m.logger.Warn("failed to load MCP config",
			zap.String("profile_id", profileID),
			zap.Error(err))
		return nil, err
	}
	if config == nil || !config.Enabled {
		return nil, nil
	}

	policy := mcpconfig.DefaultPolicyForRuntime(runtimeName(m.runtime))
	executorID := ""
	if metadata != nil {
		if value, ok := metadata["executor_id"].(string); ok {
			executorID = value
		}
		if value, ok := metadata["executor_mcp_policy"]; ok {
			updated, policyWarnings, err := mcpconfig.ApplyExecutorPolicy(policy, value)
			if err != nil {
				return nil, fmt.Errorf("invalid executor MCP policy: %w", err)
			}
			policy = updated
			for _, warning := range policyWarnings {
				m.logger.Warn("mcp policy warning",
					zap.String("profile_id", profileID),
					zap.String("executor_id", executorID),
					zap.String("warning", warning))
			}
		}
	}
	resolved, warnings, err := mcpconfig.Resolve(config, policy)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		m.logger.Warn("mcp config warning",
			zap.String("profile_id", profileID),
			zap.String("executor_id", executorID),
			zap.String("warning", warning))
	}

	return mcpconfig.ToACPServers(resolved), nil
}

func runtimeName(rt Runtime) runtime.Name {
	if rt == nil {
		return runtime.NameUnknown
	}
	return rt.Name()
}

// initializeACPSession delegates to SessionManager for full ACP session initialization and prompting
func (m *Manager) initializeACPSession(ctx context.Context, execution *AgentExecution, agentConfig *registry.AgentTypeConfig, taskDescription string, mcpServers []agentctltypes.McpServer) error {
	return m.sessionManager.InitializeAndPrompt(ctx, execution, agentConfig, taskDescription, mcpServers, m.MarkReady)
}

// emitSessionStatusEvent emits a session_status event indicating whether the session was resumed or new.
// providedACPSessionID is the ACP session ID that was provided BEFORE initialization (for resumption).
// This must be captured before initializeACPSession runs, as that overwrites execution.ACPSessionID.
func (m *Manager) emitSessionStatusEvent(execution *AgentExecution, agentConfig *registry.AgentTypeConfig, providedACPSessionID string) {
	wasResumed := false
	if providedACPSessionID != "" {
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
		// Accumulate message content and stream on newline boundaries for real-time feedback
		if event.Text != "" {
			execution.messageMu.Lock()
			execution.messageBuffer.WriteString(event.Text)

			// Check if buffer contains newlines - if so, flush up to the last newline
			bufContent := execution.messageBuffer.String()
			lastNewline := strings.LastIndex(bufContent, "\n")
			if lastNewline != -1 {
				// Extract content up to and including the last newline
				toFlush := bufContent[:lastNewline+1]
				remainder := bufContent[lastNewline+1:]

				// Reset buffer with remainder
				execution.messageBuffer.Reset()
				execution.messageBuffer.WriteString(remainder)
				execution.messageMu.Unlock()

				// Publish streaming message event if there's content to flush
				if strings.TrimSpace(toFlush) != "" {
					m.publishStreamingMessage(execution, toFlush)
				}
			} else {
				execution.messageMu.Unlock()
			}
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
		// Tool call message creation is handled by orchestrator via AgentStreamEvent

	case "tool_update":
		// Tool update handled by orchestrator via AgentStreamEvent

	case "plan":
		m.logger.Debug("agent plan update",
			zap.String("execution_id", execution.ID))

	case "error":
		// Flush any accumulated content and clear streaming state on error
		m.flushMessageBuffer(execution)

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

	case "context_window":
		m.logger.Debug("context window update received",
			zap.String("execution_id", execution.ID),
			zap.Int64("size", event.ContextWindowSize),
			zap.Int64("used", event.ContextWindowUsed),
			zap.Float64("efficiency", event.ContextEfficiency))

		// Publish context window event to event bus for orchestrator to persist
		m.eventPublisher.PublishContextWindow(
			execution,
			event.ContextWindowSize,
			event.ContextWindowUsed,
			event.ContextWindowRemaining,
			event.ContextEfficiency,
		)
		// Return early - context window events don't need to be published as stream events
		return
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

// handleProcessOutput processes script process output from the workspace stream
func (m *Manager) handleProcessOutput(execution *AgentExecution, output *agentctl.ProcessOutput) {
	if output == nil {
		return
	}
	m.logger.Debug("lifecycle received process output",
		zap.String("session_id", output.SessionID),
		zap.String("process_id", output.ProcessID),
		zap.String("kind", string(output.Kind)),
		zap.String("stream", output.Stream),
		zap.Int("bytes", len(output.Data)),
	)
	m.eventPublisher.PublishProcessOutput(execution, output)
}

// handleProcessStatus processes script process status updates from the workspace stream
func (m *Manager) handleProcessStatus(execution *AgentExecution, status *agentctl.ProcessStatusUpdate) {
	if status == nil {
		return
	}
	m.logger.Debug("lifecycle received process status",
		zap.String("session_id", status.SessionID),
		zap.String("process_id", status.ProcessID),
		zap.String("status", string(status.Status)),
	)
	m.eventPublisher.PublishProcessStatus(execution, status)
}

// handleShellExit processes shell exit events from the workspace stream
func (m *Manager) handleShellExit(execution *AgentExecution, code int) {
	m.eventPublisher.PublishShellExit(execution, code)
}

// publishStreamingMessage publishes a streaming message event for real-time text updates.
// It creates a new message on first call (currentMessageID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingMessage(execution *AgentExecution, content string) {
	execution.messageMu.Lock()
	isAppend := execution.currentMessageID != ""
	messageID := execution.currentMessageID

	// If this is the first chunk of a new message segment, generate the ID now
	if !isAppend {
		messageID = uuid.New().String()
		execution.currentMessageID = messageID
	}
	execution.messageMu.Unlock()

	event := AgentStreamEventData{
		Type:      "message_streaming",
		Text:      content,
		MessageID: messageID,
		IsAppend:  isAppend,
	}

	// Create payload manually to include streaming-specific fields
	payload := &AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:   execution.ID,
		TaskID:    execution.TaskID,
		SessionID: execution.SessionID,
		Data:      &event,
	}

	m.logger.Debug("publishing streaming message",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Bool("is_append", isAppend),
		zap.Int("content_length", len(content)))

	// Publish the streaming event - orchestrator will handle create/append logic
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// flushMessageBuffer extracts any accumulated message from the buffer and returns it.
// This is called when a tool use starts or on complete to get the agent's response.
// It also clears the currentMessageID to start fresh for the next message segment.
func (m *Manager) flushMessageBuffer(execution *AgentExecution) string {
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	// Clear the streaming message ID so next segment starts fresh
	currentMsgID := execution.currentMessageID
	execution.currentMessageID = ""
	execution.messageMu.Unlock()

	// If we have remaining content and an active streaming message, append it
	trimmedMessage := strings.TrimSpace(agentMessage)
	if trimmedMessage != "" && currentMsgID != "" {
		// Publish final append to the streaming message
		m.publishStreamingMessageFinal(execution, currentMsgID, trimmedMessage)
		// Return empty since we've already handled it via streaming
		return ""
	}

	return trimmedMessage
}

// publishStreamingMessageFinal publishes the final chunk of a streaming message.
// This is called during flush to append any remaining buffered content.
func (m *Manager) publishStreamingMessageFinal(execution *AgentExecution, messageID, content string) {
	event := AgentStreamEventData{
		Type:      "message_streaming",
		Text:      content,
		MessageID: messageID,
		IsAppend:  true,
	}

	payload := &AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:   execution.ID,
		TaskID:    execution.TaskID,
		SessionID: execution.SessionID,
		Data:      &event,
	}

	m.logger.Debug("publishing final streaming message chunk",
		zap.String("execution_id", execution.ID),
		zap.String("message_id", messageID),
		zap.Int("content_length", len(content)))

	m.eventPublisher.PublishAgentStreamEventPayload(payload)
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

	// Clear streaming state after cancel to ensure clean state for next prompt
	execution.messageMu.Lock()
	execution.messageBuffer.Reset()
	execution.reasoningBuffer.Reset()
	execution.summaryBuffer.Reset()
	execution.currentMessageID = ""
	execution.messageMu.Unlock()

	// Mark as ready for follow-up prompts after successful cancel
	if err := m.MarkReady(executionID); err != nil {
		m.logger.Warn("failed to mark execution as ready after cancel",
			zap.String("execution_id", executionID),
			zap.Error(err))
	}

	m.logger.Info("agent turn cancelled successfully",
		zap.String("execution_id", executionID))

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
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}

	runtimeName := "unknown"
	if m.runtime != nil {
		runtimeName = string(m.runtime.Name())
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
	_ = m.executionStore.WithLock(executionID, func(exec *AgentExecution) {
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

// GetExecutionBySessionID returns the agent execution for a session.
//
// Returns (execution, true) if found, or (nil, false) if not found.
// A session can have at most one active execution at a time. If a session exists
// but has no active execution, this returns (nil, false).
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (m *Manager) GetExecutionBySessionID(sessionID string) (*AgentExecution, bool) {
	return m.executionStore.GetBySessionID(sessionID)
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
// This probes agentctl's status endpoint to verify the agent process state. Returns true if:
//   - Agent status is "running" (actively processing prompts)
//   - Agent status is "starting" (process launched but not yet ready)
//
// Returns false if:
//   - No execution exists for this session
//   - agentctl client is not available
//   - Status check fails (network/timeout error)
//   - Agent is in any other state (stopped, failed, etc.)
//
// Note: The name "IsAgentRunning" is slightly misleading - it includes "starting" state.
// Use this to check if an agent subprocess exists for the session, regardless of ready state.
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
//   Starting -> Ready (after initialization)
//   Running  -> Ready (after prompt completion)
//   Any      -> Ready (after cancel)
//
// Publishes an AgentReady event to notify subscribers (frontend, orchestrator).
//
// Returns error if execution not found.
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
//   This is a terminal state transition - no further state changes are expected after this.
//   Typical flow: Starting -> Running -> Ready -> ... -> Completed/Failed
//
// Publishes either AgentCompleted or AgentFailed event depending on final status.
//
// Returns error if execution not found.
func (m *Manager) MarkCompleted(executionID string, exitCode int, errorMessage string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
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

// RemoveExecution removes an execution from tracking.
//
// ⚠️  WARNING: This is a potentially dangerous operation that should only be called when:
//   1. The agent process has been fully stopped (via StopAgent)
//   2. All cleanup operations have completed (worktree cleanup, container removal)
//   3. The execution is in a terminal state (Completed, Failed, or Cancelled)
//
// This method:
//   - Removes the execution from the in-memory store
//   - Makes the sessionID available for new executions
//   - Does NOT stop the agent process (call StopAgent first)
//   - Does NOT close the agentctl client (call execution.agentctl.Close() first)
//   - Does NOT clean up resources (worktrees, containers, etc.)
//
// After calling this, the executionID and sessionID can no longer be used to query
// or control the execution. Any references to this execution will become invalid.
//
// Typical usage: Called by cleanup loops or after successful StopAgent completion.
// For stale/dead executions, use CleanupStaleExecutionBySessionID instead.
func (m *Manager) RemoveExecution(executionID string) {
	m.executionStore.Remove(executionID)
	m.logger.Debug("removed execution from tracking",
		zap.String("execution_id", executionID))
}

// CleanupStaleExecutionBySessionID removes a stale execution from tracking without stopping it.
//
// A "stale" execution is one where the agent process has stopped externally (crashed, killed,
// or terminated outside of our control) but the execution is still tracked in memory.
//
// When to use this:
//   - After detecting the agentctl HTTP server is unreachable
//   - When the agent container no longer exists (Docker runtime)
//   - After server restart when recovering persisted state
//   - When IsAgentRunningForSession returns false but execution exists
//
// This method performs cleanup:
//   1. Closes the agentctl HTTP client connection
//   2. Removes the execution from the in-memory tracking store
//
// What this does NOT do:
//   - Stop the agent process (assumed already stopped)
//   - Clean up worktrees or containers (caller's responsibility)
//   - Update database session state (caller's responsibility)
//
// This is safe to call even if the process is still running - it won't send kill signals.
// Use StopAgent if you need to actively terminate a running agent.
//
// Returns nil if no execution exists for the session (idempotent).
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
//                When cancelled=true, optionID is ignored.
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

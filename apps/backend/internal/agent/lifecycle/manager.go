// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/appctx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// RuntimeFallbackPolicy controls behavior when a requested runtime is unavailable.
type RuntimeFallbackPolicy string

const (
	// RuntimeFallbackAllow silently falls back to the default runtime (current behavior).
	RuntimeFallbackAllow RuntimeFallbackPolicy = "allow"
	// RuntimeFallbackWarn falls back but logs a warning (current behavior, explicit).
	RuntimeFallbackWarn RuntimeFallbackPolicy = "warn"
	// RuntimeFallbackDeny returns an error if the requested runtime is unavailable.
	RuntimeFallbackDeny RuntimeFallbackPolicy = "deny"
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

	// RuntimeRegistry manages multiple runtimes (Docker, Standalone, etc.)
	// Each task can select its runtime based on executor type.
	runtimeRegistry *RuntimeRegistry

	// runtimeFallbackPolicy controls behavior when a requested runtime is unavailable.
	runtimeFallbackPolicy RuntimeFallbackPolicy

	// Refactored components for separation of concerns
	executionStore   *ExecutionStore        // Thread-safe execution tracking
	commandBuilder   *CommandBuilder        // Builds agent commands from registry config
	sessionManager   *SessionManager        // Handles ACP session initialization
	streamManager    *StreamManager         // Manages WebSocket streams
	eventPublisher   *EventPublisher        // Publishes lifecycle events
	containerManager *ContainerManager      // Manages Docker containers (optional, nil for non-Docker runtimes)
	historyManager   *SessionHistoryManager // Stores session history for context injection (fork_session pattern)

	// Workspace info provider for on-demand instance creation
	workspaceInfoProvider WorkspaceInfoProvider

	// mcpHandler is the MCP request dispatcher for handling MCP requests
	// from agentctl instances through the agent stream.
	mcpHandler agentctl.MCPHandler

	// Background cleanup
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewManager creates a new lifecycle manager.
// The runtimeRegistry manages multiple runtimes (Docker, Standalone, etc.) for task-specific execution.
// The containerManager parameter is optional and used for Docker cleanup (pass nil for non-Docker runtimes).
// The fallbackPolicy controls behavior when a requested runtime is unavailable.
func NewManager(
	reg *registry.Registry,
	eventBus bus.EventBus,
	runtimeRegistry *RuntimeRegistry,
	containerManager *ContainerManager,
	credsMgr CredentialsManager,
	profileResolver ProfileResolver,
	mcpProvider McpConfigProvider,
	fallbackPolicy RuntimeFallbackPolicy,
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

	// Initialize session history manager for fork_session pattern (context injection)
	historyManager, err := NewSessionHistoryManager("", log)
	if err != nil {
		log.Warn("failed to create session history manager, context injection disabled", zap.Error(err))
	}

	mgr := &Manager{
		registry:              reg,
		eventBus:              eventBus,
		runtimeRegistry:       runtimeRegistry,
		runtimeFallbackPolicy: fallbackPolicy,
		credsMgr:              credsMgr,
		profileResolver:       profileResolver,
		mcpProvider:           mcpProvider,
		logger:                componentLogger,
		executionStore:        executionStore,
		commandBuilder:        commandBuilder,
		sessionManager:        sessionManager,
		eventPublisher:        eventPublisher,
		containerManager:      containerManager,
		historyManager:        historyManager,
		cleanupInterval:       30 * time.Second,
		stopCh:                stopCh,
	}

	// Initialize stream manager with callbacks that delegate to manager methods
	// mcpHandler will be set later via SetMCPHandler
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnAgentEvent:    mgr.handleAgentEvent,
		OnGitStatus:     mgr.handleGitStatusUpdate,
		OnGitCommit:     mgr.handleGitCommitCreated,
		OnGitReset:      mgr.handleGitResetDetected,
		OnFileChange:    mgr.handleFileChangeNotification,
		OnShellOutput:   mgr.handleShellOutput,
		OnShellExit:     mgr.handleShellExit,
		OnProcessOutput: mgr.handleProcessOutput,
		OnProcessStatus: mgr.handleProcessStatus,
	}, nil)

	// Set session manager dependencies for full orchestration
	sessionManager.SetDependencies(eventPublisher, mgr.streamManager, executionStore, historyManager)

	if runtimeRegistry != nil {
		mgr.logger.Info("initialized with runtimes", zap.Int("count", len(runtimeRegistry.List())))
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

// SetMCPHandler sets the MCP request handler for dispatching MCP tool calls.
//
// MCP requests from agents flow through the agent stream (WebSocket) to the backend,
// where they are dispatched to this handler. This enables agents to use MCP tools
// like listing workspaces, boards, tasks, and asking user questions.
//
// This must be called before agents start making MCP calls. Typically set during
// initialization after the MCP handlers are created.
func (m *Manager) SetMCPHandler(handler agentctl.MCPHandler) {
	m.mcpHandler = handler
	// Update the stream manager with the handler
	m.streamManager.mcpHandler = handler
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

// getRuntimeForExecutorType returns the appropriate runtime for the given executor type.
// If the executor type is empty or the runtime is not available, behavior depends on runtimeFallbackPolicy.
func (m *Manager) getRuntimeForExecutorType(executorType string) (Runtime, error) {
	if m.runtimeRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}

	if executorType != "" {
		runtimeName := runtime.ExecutorTypeToRuntime(models.ExecutorType(executorType))
		rt, err := m.runtimeRegistry.GetRuntime(runtimeName)
		if err == nil {
			return rt, nil
		}

		// Handle fallback based on policy
		switch m.runtimeFallbackPolicy {
		case RuntimeFallbackDeny:
			return nil, fmt.Errorf("runtime %s not available and fallback is denied: %w", runtimeName, err)
		case RuntimeFallbackWarn:
			m.logger.Warn("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)),
				zap.Error(err))
		case RuntimeFallbackAllow:
			m.logger.Debug("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)))
		default:
			// Default to warn behavior for backwards compatibility
			m.logger.Warn("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)),
				zap.Error(err))
		}
	}

	return m.runtimeRegistry.GetRuntime(runtime.NameStandalone)
}

// getDefaultRuntime returns the default runtime (standalone).
// This is used when no executor type is specified.
func (m *Manager) getDefaultRuntime() (Runtime, error) {
	if m.runtimeRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}
	return m.runtimeRegistry.GetRuntime(runtime.NameStandalone)
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
	rt, err := m.getDefaultRuntime()
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

	req := &RuntimeCreateRequest{
		InstanceID:     executionID,
		TaskID:         taskID,
		SessionID:      info.SessionID,
		AgentProfileID: info.AgentProfileID,
		WorkspacePath:  info.WorkspacePath,
		Protocol:       string(agentConfig.Protocol),
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

	m.executionStore.Add(execution)

	go m.waitForAgentctlReady(execution)

	m.logger.Info("execution created",
		zap.String("execution_id", executionID),
		zap.String("task_id", taskID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("runtime", execution.RuntimeName))

	return execution, nil
}

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	if m.runtimeRegistry == nil {
		m.logger.Warn("no runtime registry configured")
		return nil
	}

	runtimeNames := m.runtimeRegistry.List()
	m.logger.Info("starting lifecycle manager", zap.Int("runtimes", len(runtimeNames)))

	// Check health of all registered runtimes
	healthResults := m.runtimeRegistry.HealthCheckAll(ctx)
	for name, err := range healthResults {
		if err != nil {
			m.logger.Warn("runtime health check failed",
				zap.String("runtime", string(name)),
				zap.Error(err))
		} else {
			m.logger.Info("runtime is healthy", zap.String("runtime", string(name)))
		}
	}

	// Try to recover executions from all runtimes
	recovered, err := m.runtimeRegistry.RecoverAll(ctx)
	if err != nil {
		m.logger.Warn("failed to recover executions from some runtimes", zap.Error(err))
	}
	if len(recovered) > 0 {
		for _, ri := range recovered {
			execution := &AgentExecution{
				ID:                   ri.InstanceID,
				TaskID:               ri.TaskID,
				SessionID:            ri.SessionID,
				ContainerID:          ri.ContainerID,
				ContainerIP:          ri.ContainerIP,
				WorkspacePath:        ri.WorkspacePath,
				RuntimeName:          ri.RuntimeName,
				Status:               v1.AgentStatusRunning,
				StartedAt:            time.Now(),
				Metadata:             ri.Metadata,
				agentctl:             ri.Client,
				standaloneInstanceID: ri.StandaloneInstanceID,
				standalonePort:       ri.StandalonePort,
			}
			m.executionStore.Add(execution)

			// Reconnect to workspace streams (shell, git, file changes) in background
			// This is needed so shell.input, git status, etc. work after backend restart
			go m.streamManager.ReconnectAll(execution)
		}
		m.logger.Info("recovered executions", zap.Int("count", len(recovered)))
	}

	// Start cleanup loop when container manager is available (Docker mode)
	if m.containerManager != nil {
		m.wg.Add(1)
		go m.cleanupLoop(ctx)
		m.logger.Info("cleanup loop started")
	}

	// Set up callbacks for passthrough mode (using standalone runtime)
	if standaloneRT, err := m.runtimeRegistry.GetRuntime(runtime.NameStandalone); err == nil {
		if interactiveRunner := standaloneRT.GetInteractiveRunner(); interactiveRunner != nil {
			// Turn complete callback
			interactiveRunner.SetTurnCompleteCallback(func(sessionID string) {
				m.handlePassthroughTurnComplete(sessionID)
			})

			// Output callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetOutputCallback(func(output *agentctltypes.ProcessOutput) {
				m.handlePassthroughOutput(output)
			})

			// Status callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetStatusCallback(func(status *agentctltypes.ProcessStatusUpdate) {
				m.handlePassthroughStatus(status)
			})

			m.logger.Info("passthrough callbacks configured")
		}
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

// StopAllAgents attempts a graceful shutdown of all active agents concurrently.
func (m *Manager) StopAllAgents(ctx context.Context) error {
	executions := m.executionStore.List()
	if len(executions) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(executions))

	for _, exec := range executions {
		wg.Add(1)
		go func(e *AgentExecution) {
			defer wg.Done()
			if err := m.StopAgent(ctx, e.ID, false); err != nil {
				errCh <- err
				m.logger.Warn("failed to stop agent during shutdown",
					zap.String("execution_id", e.ID),
					zap.Error(err))
			}
		}(exec)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Launch launches a new agent for a task
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
	m.logger.Debug("launching agent",
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
		agentTypeName = profileInfo.AgentName
		m.logger.Debug("resolved agent profile",
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
	agentConfig, ok := m.registry.Get(agentTypeName)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", agentTypeName)
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
	}

	// 7. Launch via runtime - creates agentctl instance with workspace access only
	// Agent subprocess is NOT started - call StartAgentProcess() explicitly
	// Select runtime based on executor type from request
	rt, err := m.getRuntimeForExecutorType(req.ExecutorType)
	if err != nil {
		return nil, fmt.Errorf("no runtime configured: %w", err)
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

	// Build metadata with runtime-specific fields
	metadata := make(map[string]interface{})
	if reqWithWorktree.Metadata != nil {
		for k, v := range reqWithWorktree.Metadata {
			metadata[k] = v
		}
	}
	if mainRepoGitDir != "" {
		metadata[MetadataKeyMainRepoGitDir] = mainRepoGitDir
	}
	if worktreeID != "" {
		metadata[MetadataKeyWorktreeID] = worktreeID
	}
	if worktreeBranch != "" {
		metadata[MetadataKeyWorktreeBranch] = worktreeBranch
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
		Metadata:       metadata,
		AgentConfig:    agentConfig,
		McpServers:     mcpServers,
	}

	runtimeInstance, err := rt.CreateInstance(ctx, runtimeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Convert to AgentExecution and set the runtime name
	execution := runtimeInstance.ToAgentExecution(runtimeReq)
	execution.RuntimeName = string(rt.Name())

	// Set ACP session ID for session resumption (used by InitializeSession)
	if req.ACPSessionID != "" {
		execution.ACPSessionID = req.ACPSessionID
	}

	// Build agent command string for later use with StartAgentProcess
	model := ""
	autoApprove := false
	permissionValues := make(map[string]bool)
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
		// Build permission values map from profile info
		permissionValues["auto_approve"] = profileInfo.AutoApprove
		permissionValues["allow_indexing"] = profileInfo.AllowIndexing
		permissionValues["dangerously_skip_permissions"] = profileInfo.DangerouslySkipPermissions
	}
	// Allow model override from request (for dynamic model switching)
	if req.ModelOverride != "" {
		model = req.ModelOverride
	}
	cmdOpts := CommandOptions{
		Model:            model,
		SessionID:        req.ACPSessionID,
		AutoApprove:      autoApprove,
		PermissionValues: permissionValues,
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

	m.logger.Debug("agentctl execution created (agent not started)",
		zap.String("execution_id", executionID),
		zap.String("task_id", req.TaskID),
		zap.String("runtime", execution.RuntimeName))

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

	// Check if this execution should use passthrough mode
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		profileInfo, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
		if err == nil && profileInfo.CLIPassthrough {
			return m.startPassthroughSession(ctx, execution, profileInfo)
		}
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

	// Determine approval policy for Codex
	// If auto_approve is true, use "never" (no approval needed)
	// Otherwise use "untrusted" (request approval for all actions)
	approvalPolicy := ""
	if execution.AgentProfileID != "" && m.profileResolver != nil {
		if profileInfo, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID); err == nil {
			if profileInfo.AutoApprove {
				approvalPolicy = "never"
			} else {
				approvalPolicy = "untrusted"
			}
		}
	}

	// Configure the agent command
	if err := execution.agentctl.ConfigureAgent(ctx, execution.AgentCommand, env, approvalPolicy); err != nil {
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

	// Initialize ACP session.
	// Session status events are now emitted by all adapters via the stream.
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, mcpServers); err != nil {
		m.updateExecutionError(executionID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

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
		PullBeforeWorktree:   req.PullBeforeWorktree,
		WorktreeID:           worktreeID, // If set, will try to reuse this worktree
	}

	wt, err := m.worktreeMgr.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	if worktreeID != "" && wt.ID == worktreeID {
		m.logger.Debug("reusing existing worktree for session resumption",
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

	m.logger.Debug("waiting for agentctl to be ready",
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
		m.logger.Debug("agentctl ready - shell/workspace access available",
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

	agentTypeName := profileInfo.AgentName
	agentConfig, ok := m.registry.Get(agentTypeName)
	if !ok {
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

	// Get default runtime for MCP policy (used before execution exists)
	defaultRT, _ := m.getDefaultRuntime()
	policy := mcpconfig.DefaultPolicyForRuntime(runtimeName(defaultRT))
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

// handleAgentEvent processes incoming agent events from the agent
func (m *Manager) handleAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) {
	// Log all incoming events for debugging
	m.logger.Debug("handleAgentEvent entry",
		zap.String("execution_id", execution.ID),
		zap.String("event_type", event.Type),
		zap.String("operation_id", event.OperationID),
		zap.Int("text_length", len(event.Text)))

	// Handle different event types based on the Type field
	switch event.Type {
	case "message_chunk":
		// Accumulate message content and stream on newline boundaries for real-time feedback
		if event.Text != "" {
			execution.messageMu.Lock()
			execution.messageBuffer.WriteString(event.Text)
			bufferLenAfterWrite := execution.messageBuffer.Len()
			m.logger.Debug("message_chunk written to buffer",
				zap.String("execution_id", execution.ID),
				zap.String("operation_id", event.OperationID),
				zap.Int("text_length", len(event.Text)),
				zap.Int("buffer_length_after", bufferLenAfterWrite))

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
		// Return early - message_chunk is transformed to message_streaming, no need to publish raw event
		return

	case "reasoning":
		// Stream thinking content like message chunks for real-time feedback
		// All adapters normalize reasoning to ReasoningText field
		if event.ReasoningText != "" {
			execution.messageMu.Lock()
			execution.thinkingBuffer.WriteString(event.ReasoningText)

			// Check if buffer contains newlines - if so, flush up to the last newline
			bufContent := execution.thinkingBuffer.String()
			lastNewline := strings.LastIndex(bufContent, "\n")
			if lastNewline != -1 {
				// Extract content up to and including the last newline
				toFlush := bufContent[:lastNewline+1]
				remainder := bufContent[lastNewline+1:]

				// Reset buffer with remainder
				execution.thinkingBuffer.Reset()
				execution.thinkingBuffer.WriteString(remainder)
				execution.messageMu.Unlock()

				// Publish streaming thinking event if there's content to flush
				if strings.TrimSpace(toFlush) != "" {
					m.publishStreamingThinking(execution, toFlush)
				}
			} else {
				execution.messageMu.Unlock()
			}
		}
		// Return early - reasoning is transformed to thinking_streaming, no need to publish raw event
		return

	case "tool_call":
		// Tool call starting marks a step boundary - flush the accumulated message
		// This way, each agent response before a tool use becomes a separate comment
		// Include the flushed text in the event for the orchestrator to save
		if flushedText := m.flushMessageBuffer(execution); flushedText != "" {
			event.Text = flushedText
			// Store flushed agent message to session history for context injection
			if m.historyManager != nil && execution.SessionID != "" {
				if err := m.historyManager.AppendAgentMessage(execution.SessionID, flushedText); err != nil {
					m.logger.Warn("failed to store agent message to history", zap.Error(err))
				}
			}
		}

		// Store tool call to session history for context injection
		if m.historyManager != nil && execution.SessionID != "" {
			if err := m.historyManager.AppendToolCall(execution.SessionID, event); err != nil {
				m.logger.Warn("failed to store tool call to history", zap.Error(err))
			}
		}

		m.logger.Debug("tool call started",
			zap.String("execution_id", execution.ID),
			zap.String("tool_call_id", event.ToolCallID),
			zap.String("tool_name", event.ToolName))
		// Tool call message creation is handled by orchestrator via AgentStreamEvent

	case "tool_update":
		// Store tool result to session history when complete
		if m.historyManager != nil && execution.SessionID != "" && event.ToolStatus == "complete" {
			if err := m.historyManager.AppendToolResult(execution.SessionID, event); err != nil {
				m.logger.Warn("failed to store tool result to history", zap.Error(err))
			}
		}
		// Tool update handled by orchestrator via AgentStreamEvent

	case "plan":
		m.logger.Debug("agent plan update",
			zap.String("execution_id", execution.ID))

	case "error":
		// Flush any accumulated content and clear streaming state on error
		m.flushMessageBuffer(execution)

		// Log all available error information for debugging
		m.logger.Error("agent error",
			zap.String("execution_id", execution.ID),
			zap.String("error", event.Error),
			zap.String("text", event.Text),
			zap.Any("data", event.Data))

	case "complete":
		// Check buffer content BEFORE any processing
		execution.messageMu.Lock()
		bufferContentBeforeFlush := execution.messageBuffer.String()
		currentMsgID := execution.currentMessageID
		execution.messageMu.Unlock()

		// Log preview of buffer content for debugging (first 100 chars)
		bufferPreview := bufferContentBeforeFlush
		if len(bufferPreview) > 100 {
			bufferPreview = bufferPreview[:100] + "..."
		}

		// Check if this is an error completion (agent failed to process the prompt)
		isError := false
		if event.Data != nil {
			if v, ok := event.Data["is_error"].(bool); ok {
				isError = v
			}
		}

		m.logger.Info("agent turn complete",
			zap.String("execution_id", execution.ID),
			zap.String("operation_id", event.OperationID),
			zap.String("session_id", event.SessionID),
			zap.String("current_msg_id", currentMsgID),
			zap.Int("buffer_length", len(bufferContentBeforeFlush)),
			zap.String("buffer_preview", bufferPreview),
			zap.Bool("is_error", isError))

		// Flush the message buffer to publish any remaining content as a streaming message.
		// All adapters now handle duplicate prevention at the adapter layer:
		// - They send text via message_chunk events (which accumulate in the buffer)
		// - They send complete events WITHOUT text
		// So we just flush the buffer here - no need to track streamingUsedThisTurn.
		m.flushMessageBuffer(execution)

		m.logger.Info("complete event processed",
			zap.String("execution_id", execution.ID),
			zap.String("operation_id", event.OperationID))

		// If this was an error completion, mark the execution as failed
		// The agent process will likely exit soon after sending an error result
		if isError {
			errorMsg := "agent error completion"
			if event.Error != "" {
				errorMsg = event.Error
			}
			m.logger.Warn("error completion received, marking execution as failed",
				zap.String("execution_id", execution.ID),
				zap.String("error", errorMsg))

			if err := m.MarkCompleted(execution.ID, 1, errorMsg); err != nil {
				m.logger.Error("failed to mark execution as failed after error completion",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
			}

			// Remove the execution from the store so a new one can be created
			// The AgentFailed event has already been published by MarkCompleted
			m.RemoveExecution(execution.ID)
		} else {
			// Mark agent as READY for follow-up prompts
			if err := m.MarkReady(execution.ID); err != nil {
				m.logger.Error("failed to mark execution as ready after complete",
					zap.String("execution_id", execution.ID),
					zap.Error(err))
			}
		}

	case "permission_request":
		m.logger.Debug("permission request received",
			zap.String("execution_id", execution.ID),
			zap.String("pending_id", event.PendingID),
			zap.String("title", event.PermissionTitle))

		// Publish permission request to dedicated subject for orchestrator to handle
		m.eventPublisher.PublishPermissionRequest(execution, event)
		// Return early - permission requests don't need to be published as stream events
		return

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

	case "available_commands":
		// Store available commands in the execution for later retrieval (e.g., after page refresh)
		if len(event.AvailableCommands) > 0 {
			execution.SetAvailableCommands(event.AvailableCommands)
			m.logger.Debug("stored available commands",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Int("command_count", len(event.AvailableCommands)))

			// Also publish to event bus for WebSocket broadcast
			m.eventPublisher.PublishAvailableCommands(execution, event.AvailableCommands)
		}
		// Return early - we handle broadcasting ourselves
		return
	}

	// Publish agent stream event to event bus for WebSocket streaming
	m.eventPublisher.PublishAgentStreamEvent(execution, event)
}

// handleGitStatusUpdate processes git status updates from the workspace tracker
func (m *Manager) handleGitStatusUpdate(execution *AgentExecution, update *agentctl.GitStatusUpdate) {
	// Publish git status update to event bus for WebSocket streaming and persistence
	m.eventPublisher.PublishGitStatus(execution, update)
}

// handleGitCommitCreated processes git commit events from the workspace tracker
func (m *Manager) handleGitCommitCreated(execution *AgentExecution, commit *agentctl.GitCommitNotification) {
	// Publish commit event to event bus for WebSocket streaming and orchestrator handling
	m.eventPublisher.PublishGitCommit(execution, commit)
}

// handleGitResetDetected processes git reset events from the workspace tracker
func (m *Manager) handleGitResetDetected(execution *AgentExecution, reset *agentctl.GitResetNotification) {
	// Publish reset event to event bus for orchestrator handling (commit sync)
	m.eventPublisher.PublishGitReset(execution, reset)
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
// Additionally flushes any accumulated thinking content.
func (m *Manager) flushMessageBuffer(execution *AgentExecution) string {
	execution.messageMu.Lock()
	agentMessage := execution.messageBuffer.String()
	thinkingContent := execution.thinkingBuffer.String()
	execution.messageBuffer.Reset()
	execution.thinkingBuffer.Reset()
	// Clear the streaming message IDs so next segment starts fresh
	currentMsgID := execution.currentMessageID
	currentThinkingID := execution.currentThinkingID
	execution.currentMessageID = ""
	execution.currentThinkingID = ""
	execution.messageMu.Unlock()

	// If we have remaining thinking content, publish it
	trimmedThinking := strings.TrimSpace(thinkingContent)
	if trimmedThinking != "" {
		if currentThinkingID != "" {
			// Append to existing streaming thinking message
			m.publishStreamingThinkingFinal(execution, currentThinkingID, trimmedThinking)
		} else {
			// No streaming thinking message exists yet - create one with all the content
			// This happens when thinking content has no newlines (never triggered streaming)
			m.publishStreamingThinking(execution, trimmedThinking)
		}
	}

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

// publishStreamingThinking publishes a streaming thinking event for real-time thinking updates.
// It creates a new thinking message on first call (currentThinkingID empty) or appends to existing.
// The message ID is generated and set synchronously to avoid race conditions.
func (m *Manager) publishStreamingThinking(execution *AgentExecution, content string) {
	execution.messageMu.Lock()
	isAppend := execution.currentThinkingID != ""
	thinkingID := execution.currentThinkingID

	// If this is the first chunk of a new thinking segment, generate the ID now
	if !isAppend {
		thinkingID = uuid.New().String()
		execution.currentThinkingID = thinkingID
	}
	execution.messageMu.Unlock()

	event := AgentStreamEventData{
		Type:        "thinking_streaming",
		Text:        content,
		MessageID:   thinkingID,
		IsAppend:    isAppend,
		MessageType: "thinking",
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

	// Publish the streaming event - orchestrator will handle create/append logic
	m.eventPublisher.PublishAgentStreamEventPayload(payload)
}

// publishStreamingThinkingFinal publishes the final chunk of a streaming thinking message.
// This is called during flush to append any remaining buffered thinking content.
func (m *Manager) publishStreamingThinkingFinal(execution *AgentExecution, thinkingID, content string) {
	event := AgentStreamEventData{
		Type:        "thinking_streaming",
		Text:        content,
		MessageID:   thinkingID,
		IsAppend:    true,
		MessageType: "thinking",
	}

	payload := &AgentStreamEventPayload{
		Type:      "agent/event",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		AgentID:   execution.ID,
		TaskID:    execution.TaskID,
		SessionID: execution.SessionID,
		Data:      &event,
	}

	m.logger.Debug("publishing final streaming thinking chunk",
		zap.String("execution_id", execution.ID),
		zap.String("thinking_id", thinkingID),
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
	execution.thinkingBuffer.Reset()
	execution.currentMessageID = ""
	execution.currentThinkingID = ""
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

	m.logger.Info("stopping agent",
		zap.String("execution_id", executionID),
		zap.Bool("force", force),
		zap.String("runtime", execution.RuntimeName))

	// Try to gracefully stop via agentctl first
	if execution.agentctl != nil && !force {
		if err := execution.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("execution_id", executionID),
				zap.Error(err))
		}
		execution.agentctl.Close()
	}

	// Stop the agent execution via the runtime that created it
	if execution.RuntimeName != "" && m.runtimeRegistry != nil {
		rt, err := m.runtimeRegistry.GetRuntime(runtime.Name(execution.RuntimeName))
		if err != nil {
			m.logger.Warn("failed to get runtime for stopping execution",
				zap.String("execution_id", executionID),
				zap.String("runtime", execution.RuntimeName),
				zap.Error(err))
		} else {
			// Stop passthrough process if in passthrough mode
			if execution.PassthroughProcessID != "" {
				if interactiveRunner := rt.GetInteractiveRunner(); interactiveRunner != nil {
					if err := interactiveRunner.Stop(ctx, execution.PassthroughProcessID); err != nil {
						m.logger.Warn("failed to stop passthrough process",
							zap.String("execution_id", executionID),
							zap.String("process_id", execution.PassthroughProcessID),
							zap.Error(err))
					} else {
						m.logger.Info("passthrough process stopped",
							zap.String("execution_id", executionID),
							zap.String("process_id", execution.PassthroughProcessID))
					}
				}
			}

			runtimeInstance := &RuntimeInstance{
				InstanceID:           execution.ID,
				TaskID:               execution.TaskID,
				ContainerID:          execution.ContainerID,
				StandaloneInstanceID: execution.standaloneInstanceID,
				StandalonePort:       execution.standalonePort,
			}
			if err := rt.StopInstance(ctx, runtimeInstance, force); err != nil {
				// Log the error but don't return - we still need to clean up the execution store.
				// This ensures that even if the runtime is unavailable (e.g., process crashed),
				// the execution is removed from tracking so new executions can be launched.
				m.logger.Warn("failed to stop runtime instance, continuing with cleanup",
					zap.String("execution_id", executionID),
					zap.Error(err))
			}
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
//  1. The agent process has been fully stopped (via StopAgent)
//  2. All cleanup operations have completed (worktree cleanup, container removal)
//  3. The execution is in a terminal state (Completed, Failed, or Cancelled)
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
//  1. Closes the agentctl HTTP client connection
//  2. Removes the execution from the in-memory tracking store
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

// startPassthroughSession starts an agent in passthrough mode (direct terminal interaction).
// Instead of using ACP protocol, the agent's stdin/stdout is passed through directly.
func (m *Manager) startPassthroughSession(ctx context.Context, execution *AgentExecution, profileInfo *AgentProfileInfo) error {
	// Get agent config for passthrough command
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	// Validate passthrough support
	if !agentConfig.PassthroughConfig.Supported {
		return fmt.Errorf("agent %s does not support passthrough mode", agentConfig.ID)
	}

	// Get task description from metadata for initial prompt
	taskDescription := ""
	if execution.Metadata != nil {
		if desc, ok := execution.Metadata["task_description"].(string); ok {
			taskDescription = desc
		}
	}

	// Build passthrough command with initial prompt and profile settings
	cmd := m.buildPassthroughCommand(agentConfig, execution.ACPSessionID, taskDescription, profileInfo)
	if len(cmd) == 0 {
		return fmt.Errorf("passthrough command is empty for agent %s", agentConfig.ID)
	}

	m.logger.Info("passthrough command built",
		zap.String("session_id", execution.SessionID),
		zap.Strings("full_command", cmd))

	// Get the interactive runner from runtime
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available for passthrough mode")
	}

	// Build environment variables
	env := make(map[string]string)
	env["KANDEV_TASK_ID"] = execution.TaskID
	env["KANDEV_SESSION_ID"] = execution.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = execution.AgentProfileID

	// Add required credentials from agent config
	if m.credsMgr != nil {
		for _, credKey := range agentConfig.RequiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}

	// Start the interactive process immediately with default dimensions
	// Start process - either immediately (default) or wait for terminal connection
	// Some agents (like Codex) require the terminal to be connected first because
	// they query the terminal for cursor position on startup
	startReq := process.InteractiveStartRequest{
		SessionID:         execution.SessionID,
		Command:           cmd,
		WorkingDir:        execution.WorkspacePath,
		Env:               env,
		PromptPattern:     agentConfig.PassthroughConfig.PromptPattern,
		IdleTimeoutMs:     agentConfig.PassthroughConfig.IdleTimeoutMs,
		BufferMaxBytes:    agentConfig.PassthroughConfig.BufferMaxBytes,
		StatusDetector:    agentConfig.PassthroughConfig.StatusDetector,
		CheckIntervalMs:   agentConfig.PassthroughConfig.CheckIntervalMs,
		StabilityWindowMs: agentConfig.PassthroughConfig.StabilityWindowMs,
		ImmediateStart:    !agentConfig.PassthroughConfig.WaitForTerminal,
		DefaultCols:       120,
		DefaultRows:       40,
	}

	processInfo, err := interactiveRunner.Start(ctx, startReq)
	if err != nil {
		m.updateExecutionError(execution.ID, "failed to start passthrough session: "+err.Error())
		return fmt.Errorf("failed to start passthrough session: %w", err)
	}

	// Store the passthrough process ID
	execution.PassthroughProcessID = processInfo.ID

	m.logger.Info("passthrough session started",
		zap.String("execution_id", execution.ID),
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID),
		zap.String("process_id", processInfo.ID),
		zap.Strings("command", cmd))

	// Emit agentctl ready event to indicate session is available
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlReady, execution, "")

	// Start shell session for workspace shell access.
	// In passthrough mode, the agent runs via InteractiveRunner, so the agentctl
	// process manager never starts and the shell isn't auto-created.
	// We need to start it explicitly so the right panel terminal works.
	if execution.agentctl != nil {
		if err := execution.agentctl.StartShell(ctx); err != nil {
			m.logger.Warn("failed to start shell for passthrough session",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
			// Non-fatal: continue without shell
		} else {
			m.logger.Info("shell session started for passthrough mode",
				zap.String("execution_id", execution.ID))
		}
	}

	// Connect to workspace stream for shell/git/file features even in passthrough mode.
	// The passthrough terminal handles agent interaction (center panel), while the
	// workspace stream provides shell I/O (right panel) - they work independently.
	if m.streamManager != nil && execution.agentctl != nil {
		go m.streamManager.connectWorkspaceStream(execution, nil)
	}

	return nil
}

// buildPassthroughCommand builds the command for passthrough mode.
// It uses the passthrough_cmd from config, applies profile settings as CLI flags,
// and appends resume flag if resuming or initial prompt for new sessions.
func (m *Manager) buildPassthroughCommand(agentConfig *registry.AgentTypeConfig, acpSessionID string, initialPrompt string, profileInfo *AgentProfileInfo) []string {
	// Start with passthrough_cmd
	cmd := make([]string, len(agentConfig.PassthroughConfig.PassthroughCmd))
	copy(cmd, agentConfig.PassthroughConfig.PassthroughCmd)

	// Apply model flag if configured and profile has a model
	if profileInfo != nil && profileInfo.Model != "" && agentConfig.PassthroughConfig.ModelFlag != "" {
		expanded := strings.ReplaceAll(agentConfig.PassthroughConfig.ModelFlag, "{model}", profileInfo.Model)
		// Split on first space to separate flag from value (if combined)
		parts := strings.SplitN(expanded, " ", 2)
		cmd = append(cmd, parts...)
	}

	// Apply permission settings that use CLI flags
	// Build a map of permission values from profile info
	if profileInfo != nil && agentConfig.PermissionSettings != nil {
		permissionValues := map[string]bool{
			"auto_approve":                 profileInfo.AutoApprove,
			"dangerously_skip_permissions": profileInfo.DangerouslySkipPermissions,
			"allow_indexing":               profileInfo.AllowIndexing,
		}

		for settingName, setting := range agentConfig.PermissionSettings {
			// Skip if not supported or not a CLI flag setting
			if !setting.Supported || setting.ApplyMethod != "cli_flag" || setting.CLIFlag == "" {
				continue
			}

			// Get the value for this setting
			value, exists := permissionValues[settingName]
			if !exists || !value {
				continue
			}

			// Apply the CLI flag
			if setting.CLIFlagValue != "" {
				// Flag with value: "--flag value"
				cmd = append(cmd, setting.CLIFlag, setting.CLIFlagValue)
			} else {
				// Boolean flag or multiple flags: "--flag" or "--flag1 --flag2 arg"
				// Split on spaces to handle multiple flags in one setting
				parts := strings.Fields(setting.CLIFlag)
				cmd = append(cmd, parts...)
			}
		}
	}

	// Add resume flag if:
	// 1. Session ID is provided (resuming existing session)
	// 2. Agent uses CLI-based resume (not ACP)
	// 3. Agent has a ResumeFlag configured
	if acpSessionID != "" &&
		!agentConfig.SessionConfig.NativeSessionResume &&
		agentConfig.SessionConfig.ResumeFlag != "" {
		cmd = append(cmd, agentConfig.SessionConfig.ResumeFlag, acpSessionID)
	} else if initialPrompt != "" {
		// For new sessions, add the initial prompt
		// Use PromptFlag if configured (e.g., "--prompt {prompt}"), otherwise append directly
		if agentConfig.PassthroughConfig.PromptFlag != "" {
			expanded := strings.ReplaceAll(agentConfig.PassthroughConfig.PromptFlag, "{prompt}", initialPrompt)
			parts := strings.SplitN(expanded, " ", 2)
			cmd = append(cmd, parts...)
		} else {
			// Default: append prompt as a positional argument
			cmd = append(cmd, initialPrompt)
		}
	}

	return cmd
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

	if !agentConfig.PassthroughConfig.Supported {
		return fmt.Errorf("agent %s does not support passthrough mode", agentConfig.ID)
	}

	// Get profile info for permission settings
	var profileInfo *AgentProfileInfo
	if m.profileResolver != nil && execution.AgentProfileID != "" {
		profileInfo, _ = m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
	}

	// Build the resume command
	cmd := m.buildPassthroughResumeCommand(agentConfig, profileInfo)
	if len(cmd) == 0 {
		return fmt.Errorf("passthrough resume command is empty for agent %s", agentConfig.ID)
	}

	m.logger.Info("resuming passthrough session",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID),
		zap.Strings("command", cmd),
		zap.Bool("has_resume_flag", agentConfig.PassthroughConfig.ResumeFlag != ""))

	// Get the interactive runner
	interactiveRunner := m.GetInteractiveRunner()
	if interactiveRunner == nil {
		return fmt.Errorf("interactive runner not available")
	}

	// Build environment variables
	env := make(map[string]string)
	env["KANDEV_TASK_ID"] = execution.TaskID
	env["KANDEV_SESSION_ID"] = execution.SessionID
	env["KANDEV_AGENT_PROFILE_ID"] = execution.AgentProfileID

	// Add required credentials
	if m.credsMgr != nil {
		for _, credKey := range agentConfig.RequiredEnv {
			if value, err := m.credsMgr.GetCredentialValue(ctx, credKey); err == nil && value != "" {
				env[credKey] = value
			}
		}
	}

	// Start the interactive process
	// Use the same settings as initial start (including wait_for_terminal)
	startReq := process.InteractiveStartRequest{
		SessionID:         sessionID,
		Command:           cmd,
		WorkingDir:        execution.WorkspacePath,
		Env:               env,
		IdleTimeoutMs:     agentConfig.PassthroughConfig.IdleTimeoutMs,
		BufferMaxBytes:    agentConfig.PassthroughConfig.BufferMaxBytes,
		StatusDetector:    agentConfig.PassthroughConfig.StatusDetector,
		CheckIntervalMs:   agentConfig.PassthroughConfig.CheckIntervalMs,
		StabilityWindowMs: agentConfig.PassthroughConfig.StabilityWindowMs,
		ImmediateStart:    !agentConfig.PassthroughConfig.WaitForTerminal,
		DefaultCols:       120,
		DefaultRows:       40,
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
	if execution.agentctl != nil {
		if err := execution.agentctl.StartShell(ctx); err != nil {
			m.logger.Warn("failed to start shell for resumed passthrough session",
				zap.String("execution_id", execution.ID),
				zap.Error(err))
			// Non-fatal: continue without shell
		} else {
			m.logger.Info("shell session started for resumed passthrough session",
				zap.String("execution_id", execution.ID))
		}
	}

	// Connect to workspace stream for shell/git/file features.
	// This re-establishes the connection that was lost on backend restart.
	if m.streamManager != nil && execution.agentctl != nil {
		go m.streamManager.connectWorkspaceStream(execution, nil)
	}

	return nil
}

// buildPassthroughResumeCommand builds the command for resuming a passthrough session.
// If the agent supports resume (has ResumeFlag), it adds the resume flag.
// Otherwise, it starts a fresh session with profile settings but no initial prompt.
func (m *Manager) buildPassthroughResumeCommand(agentConfig *registry.AgentTypeConfig, profileInfo *AgentProfileInfo) []string {
	// Start with passthrough_cmd
	cmd := make([]string, len(agentConfig.PassthroughConfig.PassthroughCmd))
	copy(cmd, agentConfig.PassthroughConfig.PassthroughCmd)

	// Apply model flag if configured
	if profileInfo != nil && profileInfo.Model != "" && agentConfig.PassthroughConfig.ModelFlag != "" {
		expanded := strings.ReplaceAll(agentConfig.PassthroughConfig.ModelFlag, "{model}", profileInfo.Model)
		parts := strings.SplitN(expanded, " ", 2)
		cmd = append(cmd, parts...)
	}

	// Apply permission settings that use CLI flags
	// Sort keys for deterministic output order
	if profileInfo != nil && agentConfig.PermissionSettings != nil {
		permissionValues := map[string]bool{
			"auto_approve":                 profileInfo.AutoApprove,
			"dangerously_skip_permissions": profileInfo.DangerouslySkipPermissions,
			"allow_indexing":               profileInfo.AllowIndexing,
		}

		// Get sorted keys for deterministic iteration
		settingNames := make([]string, 0, len(agentConfig.PermissionSettings))
		for name := range agentConfig.PermissionSettings {
			settingNames = append(settingNames, name)
		}
		sort.Strings(settingNames)

		for _, settingName := range settingNames {
			setting := agentConfig.PermissionSettings[settingName]
			if !setting.Supported || setting.ApplyMethod != "cli_flag" || setting.CLIFlag == "" {
				continue
			}
			value, exists := permissionValues[settingName]
			if !exists || !value {
				continue
			}
			if setting.CLIFlagValue != "" {
				cmd = append(cmd, setting.CLIFlag, setting.CLIFlagValue)
			} else {
				parts := strings.Fields(setting.CLIFlag)
				cmd = append(cmd, parts...)
			}
		}
	}

	// Add resume flag if agent supports it
	if agentConfig.PassthroughConfig.ResumeFlag != "" {
		// Split on spaces to handle flags like "--resume latest"
		parts := strings.Fields(agentConfig.PassthroughConfig.ResumeFlag)
		cmd = append(cmd, parts...)
	}

	return cmd
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
		Kind:      streams.ProcessKind(output.Kind),
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
		Kind:       streams.ProcessKind(status.Kind),
		Command:    status.Command,
		ScriptName: status.ScriptName,
		WorkingDir: status.WorkingDir,
		Status:     streams.ProcessStatus(status.Status),
		ExitCode:   status.ExitCode,
		Timestamp:  status.Timestamp,
	}

	m.eventPublisher.PublishProcessStatus(execution, clientStatus)

	// Check if process exited and should be auto-restarted
	// Run asynchronously to allow the old process to be cleaned up first
	if status.Status == agentctltypes.ProcessStatusExited || status.Status == agentctltypes.ProcessStatusFailed {
		go m.handlePassthroughExit(execution, status)
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
	if m.runtimeRegistry == nil {
		return nil
	}
	standaloneRT, err := m.runtimeRegistry.GetRuntime(runtime.NameStandalone)
	if err != nil {
		return nil
	}
	return standaloneRT.GetInteractiveRunner()
}

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

	"github.com/kandev/kandev/internal/agent/agentctl"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/worktree"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Default agentctl port
const AgentCtlPort = 9999

// AgentInstance represents a running agent instance
type AgentInstance struct {
	ID             string
	TaskID         string
	AgentProfileID string
	ContainerID    string
	ContainerIP    string // IP address of the container for agentctl communication
	WorkspacePath  string // Path to the workspace (worktree or repository path)
	ACPSessionID   string // ACP session ID to resume, if available
	AgentCommand   string // Command to start the agent subprocess
	Status         v1.AgentStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
	ExitCode       *int
	ErrorMessage   string
	Progress       int
	Metadata       map[string]interface{}

	// agentctl client for this instance
	agentctl *agentctl.Client

	// Standalone mode info (when not using Docker)
	standaloneInstanceID string // Instance ID in standalone agentctl
	standalonePort       int    // Port of the standalone instance

	// Buffers for accumulating agent response during a prompt
	messageBuffer   strings.Builder
	reasoningBuffer strings.Builder
	summaryBuffer   strings.Builder
	messageMu       sync.Mutex
}

// GetAgentCtlClient returns the agentctl client for this instance
func (ai *AgentInstance) GetAgentCtlClient() *agentctl.Client {
	return ai.agentctl
}

// LaunchRequest contains parameters for launching an agent
type LaunchRequest struct {
	TaskID          string
	SessionID       string
	TaskTitle       string // Human-readable task title for semantic worktree naming
	AgentProfileID  string
	WorkspacePath   string            // Host path to workspace (original repository path)
	TaskDescription string            // Task description to send via ACP prompt
	Env             map[string]string // Additional env vars
	ACPSessionID    string            // ACP session ID to resume, if available
	Metadata        map[string]interface{}

	// Worktree configuration
	UseWorktree    bool   // Whether to use a Git worktree for isolation
	RepositoryID   string // Repository ID for worktree tracking
	RepositoryPath string // Path to the main repository (for worktree creation)
	BaseBranch     string // Base branch for the worktree (e.g., "main")
}

// CredentialsManager interface for credential retrieval
type CredentialsManager interface {
	GetCredentialValue(ctx context.Context, key string) (value string, err error)
}

// AgentProfileInfo contains resolved profile information
type AgentProfileInfo struct {
	ProfileID                  string
	ProfileName                string
	AgentID                    string
	AgentName                  string // e.g., "auggie", "claude", "codex"
	Model                      string
	AutoApprove                bool
	DangerouslySkipPermissions bool
	Plan                       string
}

// ProfileResolver resolves agent profile IDs to profile information
type ProfileResolver interface {
	ResolveProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error)
}

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
	instanceStore    *InstanceStore    // Thread-safe instance tracking
	commandBuilder   *CommandBuilder   // Builds agent commands from registry config
	sessionManager   *SessionManager   // Handles ACP session initialization
	streamManager    *StreamManager    // Manages WebSocket streams
	eventPublisher   *EventPublisher   // Publishes lifecycle events
	containerManager *ContainerManager // Manages Docker containers (optional, nil for non-Docker runtimes)

	// Shell stream starter for auto-starting shell streams
	shellStreamStarter ShellStreamStarter

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

	// Initialize instance store
	instanceStore := NewInstanceStore()

	mgr := &Manager{
		registry:         reg,
		eventBus:         eventBus,
		runtime:          runtime,
		credsMgr:         credsMgr,
		profileResolver:  profileResolver,
		logger:           componentLogger,
		instanceStore:    instanceStore,
		commandBuilder:   commandBuilder,
		sessionManager:   sessionManager,
		eventPublisher:   eventPublisher,
		containerManager: containerManager,
		cleanupInterval:  30 * time.Second,
		stopCh:           make(chan struct{}),
	}

	// Initialize stream manager with callbacks that delegate to manager methods
	mgr.streamManager = NewStreamManager(log, StreamCallbacks{
		OnSessionUpdate: mgr.handleSessionUpdate,
		OnPermission:    mgr.handlePermissionNotification,
		OnGitStatus:     mgr.handleGitStatusUpdate,
		OnFileChange:    mgr.handleFileChangeNotification,
	})

	if runtime != nil {
		mgr.logger.Info("initialized with runtime", zap.String("runtime", runtime.Name()))
	}

	return mgr
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
}

// ShellStreamStarter is called to start shell streaming for an agent instance
type ShellStreamStarter interface {
	StartShellStream(ctx context.Context, taskID string) error
}

// SetShellStreamStarter sets the shell stream starter
func (m *Manager) SetShellStreamStarter(starter ShellStreamStarter) {
	m.shellStreamStarter = starter
}

// WorkspaceInfo contains information about a task's workspace for on-demand instance creation
type WorkspaceInfo struct {
	TaskID         string
	SessionID      string // Task session ID (from task_sessions table)
	WorkspacePath  string // Path to the workspace/repository
	AgentProfileID string // Optional - agent profile for the task
	AgentID        string // Agent type ID (e.g., "auggie-agent") - required for runtime creation
	ACPSessionID   string // Agent's session ID for conversation resumption (from session metadata)
}

// WorkspaceInfoProvider provides workspace information for tasks
type WorkspaceInfoProvider interface {
	// GetWorkspaceInfo returns workspace info for a task (uses most recent session)
	GetWorkspaceInfo(ctx context.Context, taskID string) (*WorkspaceInfo, error)
	// GetWorkspaceInfoForSession returns workspace info for a specific task session
	GetWorkspaceInfoForSession(ctx context.Context, taskID, sessionID string) (*WorkspaceInfo, error)
}

// SetWorkspaceInfoProvider sets the provider for workspace information
func (m *Manager) SetWorkspaceInfoProvider(provider WorkspaceInfoProvider) {
	m.workspaceInfoProvider = provider
}

// EnsureWorkspaceInstance ensures an agentctl instance exists for a task's workspace.
// If an instance already exists, it returns it. Otherwise, it creates a new instance
// for workspace access (shell, git, file operations) WITHOUT starting the agent process.
// This is used to restore workspace access after backend restart.
func (m *Manager) EnsureWorkspaceInstance(ctx context.Context, taskID string) (*AgentInstance, error) {
	// Check if instance already exists
	if instance, exists := m.instanceStore.GetByTaskID(taskID); exists {
		return instance, nil
	}

	// Need to create a new instance - get workspace info
	if m.workspaceInfoProvider == nil {
		return nil, fmt.Errorf("workspace info provider not configured")
	}

	info, err := m.workspaceInfoProvider.GetWorkspaceInfo(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace info: %w", err)
	}

	if info.WorkspacePath == "" {
		return nil, fmt.Errorf("no workspace path available for task %s", taskID)
	}

	m.logger.Info("creating instance for task",
		zap.String("task_id", taskID),
		zap.String("workspace_path", info.WorkspacePath))

	return m.createInstance(ctx, taskID, info)
}

// EnsureWorkspaceInstanceForSession ensures an agentctl instance exists for a specific task session.
// This is used when the frontend provides a session ID (e.g., from URL path /task/[id]/[sessionId]).
// If an instance already exists for the task, it returns it. Otherwise, it creates a new instance
// using the session's workspace configuration from the database.
func (m *Manager) EnsureWorkspaceInstanceForSession(ctx context.Context, taskID, sessionID string) (*AgentInstance, error) {
	// Check if instance already exists
	if instance, exists := m.instanceStore.GetByTaskID(taskID); exists {
		return instance, nil
	}

	// Need to create a new instance - get workspace info for the specific session
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

	m.logger.Info("creating instance for task session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("acp_session_id", info.ACPSessionID))

	return m.createInstance(ctx, taskID, info)
}

// createInstance creates an agentctl instance.
// The agent subprocess is NOT started - call ConfigureAgent + Start explicitly.
func (m *Manager) createInstance(ctx context.Context, taskID string, info *WorkspaceInfo) (*AgentInstance, error) {
	if m.runtime == nil {
		return nil, fmt.Errorf("no runtime configured")
	}

	if info.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required in WorkspaceInfo")
	}

	instanceID := uuid.New().String()

	agentConfig, err := m.registry.Get(info.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent type %q not found in registry: %w", info.AgentID, err)
	}

	req := &RuntimeCreateRequest{
		InstanceID:     instanceID,
		TaskID:         taskID,
		AgentProfileID: info.AgentProfileID,
		WorkspacePath:  info.WorkspacePath,
		Protocol:       string(agentConfig.Protocol),
	}

	runtimeInstance, err := m.runtime.CreateInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	instance := runtimeInstance.ToAgentInstance(req)

	// Set the ACP session ID for session resumption
	if info.ACPSessionID != "" {
		instance.ACPSessionID = info.ACPSessionID
	}

	m.instanceStore.Add(instance)

	go m.waitForAgentctlReady(instance)

	m.logger.Info("instance created",
		zap.String("instance_id", instanceID),
		zap.String("task_id", taskID),
		zap.String("workspace_path", info.WorkspacePath),
		zap.String("runtime", m.runtime.Name()))

	return instance, nil
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

	// Try to recover instances from previous runs
	recovered, err := m.runtime.RecoverInstances(ctx)
	if err != nil {
		m.logger.Warn("failed to recover instances", zap.Error(err))
	} else if len(recovered) > 0 {
		for _, ri := range recovered {
			instance := &AgentInstance{
				ID:                   ri.InstanceID,
				TaskID:               ri.TaskID,
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
			m.instanceStore.Add(instance)
		}
		m.logger.Info("recovered instances", zap.Int("count", len(recovered)))
	}

	// Start cleanup loop when container manager is available (Docker mode)
	if m.containerManager != nil {
		m.wg.Add(1)
		go m.cleanupLoop(ctx)
		m.logger.Info("cleanup loop started")
	}

	return nil
}

// RecoveredInstance contains info about an instance recovered from a runtime.
type RecoveredInstance struct {
	InstanceID     string
	TaskID         string
	ContainerID    string
	AgentProfileID string
}

// GetRecoveredInstances returns a snapshot of all currently tracked instances
// This can be used by the orchestrator to sync with the database
func (m *Manager) GetRecoveredInstances() []RecoveredInstance {
	instances := m.instanceStore.List()
	result := make([]RecoveredInstance, 0, len(instances))
	for _, inst := range instances {
		result = append(result, RecoveredInstance{
			InstanceID:     inst.ID,
			TaskID:         inst.TaskID,
			ContainerID:    inst.ContainerID,
			AgentProfileID: inst.AgentProfileID,
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
	instances := m.instanceStore.List()
	if len(instances) == 0 {
		return nil
	}

	var errs []error
	for _, inst := range instances {
		if err := m.StopAgent(ctx, inst.ID, false); err != nil {
			errs = append(errs, err)
			m.logger.Warn("failed to stop agent during shutdown",
				zap.String("instance_id", inst.ID),
				zap.Error(err))
		}
	}

	return errors.Join(errs...)
}

// Launch launches a new agent for a task
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentInstance, error) {
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

	// 3. Check if task already has an agent running
	if existingInstance, exists := m.instanceStore.GetByTaskID(req.TaskID); exists {
		return nil, fmt.Errorf("task %q already has an agent running (instance: %s)", req.TaskID, existingInstance.ID)
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

	// 5. Generate a new instance ID
	instanceID := uuid.New().String()

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

	// 6b. Add profile settings to environment
	if profileInfo != nil {
		if reqWithWorktree.Env == nil {
			reqWithWorktree.Env = make(map[string]string)
		}
		if profileInfo.Model != "" {
			reqWithWorktree.Env["AGENT_MODEL"] = profileInfo.Model
		}
		if profileInfo.AutoApprove {
			reqWithWorktree.Env["AGENT_AUTO_APPROVE"] = "true"
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
	env := m.buildEnvForRuntime(instanceID, &reqWithWorktree, agentConfig)

	// Create runtime request (agent command not included - started explicitly later)
	runtimeReq := &RuntimeCreateRequest{
		InstanceID:     instanceID,
		TaskID:         reqWithWorktree.TaskID,
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
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Convert to AgentInstance
	instance := runtimeInstance.ToAgentInstance(runtimeReq)

	// Set ACP session ID for session resumption (used by InitializeSession)
	if req.ACPSessionID != "" {
		instance.ACPSessionID = req.ACPSessionID
	}

	// Build agent command string for later use with StartAgentProcess
	model := ""
	if profileInfo != nil {
		model = profileInfo.Model
	}
	cmdOpts := CommandOptions{
		Model:     model,
		SessionID: req.ACPSessionID,
	}
	instance.AgentCommand = m.commandBuilder.BuildCommandString(agentConfig, cmdOpts)

	// 8. Track the instance
	m.instanceStore.Add(instance)

	// 9. Publish agent.started event
	m.publishEvent(ctx, events.AgentStarted, instance)

	// 10. Wait for agentctl to be ready (for shell/workspace access)
	// NOTE: This does NOT start the agent process - call StartAgentProcess() explicitly
	go m.waitForAgentctlReady(instance)

	runtimeName := "unknown"
	if m.runtime != nil {
		runtimeName = m.runtime.Name()
	}
	m.logger.Info("agentctl instance created (agent not started)",
		zap.String("instance_id", instanceID),
		zap.String("task_id", req.TaskID),
		zap.String("runtime", runtimeName))

	return instance, nil
}

// StartAgentProcess configures and starts the agent subprocess for an instance.
// This must be called after Launch() to actually start the agent (e.g., auggie, codex).
// The command is built internally based on the instance's agent profile.
func (m *Manager) StartAgentProcess(ctx context.Context, instanceID string) error {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	if instance.agentctl == nil {
		return fmt.Errorf("instance %q has no agentctl client", instanceID)
	}

	if instance.AgentCommand == "" {
		return fmt.Errorf("instance %q has no agent command configured", instanceID)
	}

	// Wait for agentctl to be ready
	if err := instance.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.updateInstanceError(instanceID, "agentctl not ready: "+err.Error())
		return fmt.Errorf("agentctl not ready: %w", err)
	}

	// Get task description from metadata
	taskDescription := ""
	if instance.Metadata != nil {
		if desc, ok := instance.Metadata["task_description"].(string); ok {
			taskDescription = desc
		}
	}

	// Build environment for the agent process
	env := map[string]string{}
	if taskDescription != "" {
		env["TASK_DESCRIPTION"] = taskDescription
	}

	// Configure the agent command
	if err := instance.agentctl.ConfigureAgent(ctx, instance.AgentCommand, env); err != nil {
		return fmt.Errorf("failed to configure agent: %w", err)
	}

	// Start the agent process
	if err := instance.agentctl.Start(ctx); err != nil {
		m.updateInstanceError(instanceID, "failed to start agent: "+err.Error())
		return fmt.Errorf("failed to start agent: %w", err)
	}

	m.logger.Info("agent process started",
		zap.String("instance_id", instanceID),
		zap.String("task_id", instance.TaskID),
		zap.String("command", instance.AgentCommand))

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	// Get agent config for ACP session initialization
	agentConfig, err := m.getAgentConfigForInstance(instance)
	if err != nil {
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	// Initialize ACP session
	if err := m.initializeACPSession(ctx, instance, agentConfig, taskDescription); err != nil {
		m.updateInstanceError(instanceID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	return nil
}

// buildEnvForRuntime builds environment variables for any runtime.
// This is the unified method used by the runtime interface.
func (m *Manager) buildEnvForRuntime(instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig) map[string]string {
	env := make(map[string]string)

	// Copy request environment
	for k, v := range req.Env {
		env[k] = v
	}

	// Add standard variables for recovery after backend restart
	env["KANDEV_INSTANCE_ID"] = instanceID
	env["KANDEV_TASK_ID"] = req.TaskID
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
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		TaskTitle:      req.TaskTitle,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: req.RepositoryPath,
		BaseBranch:     req.BaseBranch,
		WorktreeID:     worktreeID, // If set, will try to reuse this worktree
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
func (m *Manager) waitForAgentctlReady(instance *AgentInstance) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m.logger.Info("waiting for agentctl to be ready",
		zap.String("instance_id", instance.ID),
		zap.String("url", instance.agentctl.BaseURL()))

	if err := instance.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		m.updateInstanceError(instance.ID, "agentctl not ready: "+err.Error())
		return
	}

	m.logger.Info("agentctl ready - shell/workspace access available",
		zap.String("instance_id", instance.ID))
}



// getAgentConfigForInstance retrieves the agent configuration for an instance.
// The instance must have AgentCommand set (which includes the agent type).
func (m *Manager) getAgentConfigForInstance(instance *AgentInstance) (*registry.AgentTypeConfig, error) {
	if instance.AgentProfileID == "" {
		return nil, fmt.Errorf("instance %s has no agent profile ID", instance.ID)
	}

	if m.profileResolver == nil {
		return nil, fmt.Errorf("profile resolver not configured")
	}

	profileInfo, err := m.profileResolver.ResolveProfile(context.Background(), instance.AgentProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve profile: %w", err)
	}

	// Map agent name to registry ID (e.g., "auggie" -> "auggie-agent")
	agentTypeName := profileInfo.AgentName + "-agent"
	agentConfig, err := m.registry.GetAgentType(agentTypeName)
	if err != nil {
		return nil, fmt.Errorf("agent type not found: %s", agentTypeName)
	}

	return agentConfig, nil
}

// initializeACPSession sends the ACP initialization messages using the SessionManager
// The agentConfig is used for configuration-driven session resumption behavior
func (m *Manager) initializeACPSession(ctx context.Context, instance *AgentInstance, agentConfig *registry.AgentTypeConfig, taskDescription string) error {
	m.logger.Info("initializing ACP session",
		zap.String("instance_id", instance.ID),
		zap.String("agentctl_url", instance.agentctl.BaseURL()),
		zap.String("agent_type", agentConfig.ID),
		zap.String("existing_acp_session_id", instance.ACPSessionID),
		zap.Bool("resume_via_acp", agentConfig.SessionConfig.ResumeViaACP))

	// Use SessionManager for configuration-driven session initialization
	// This replaces the hardcoded auggie-specific logic with registry-based configuration
	result, err := m.sessionManager.InitializeSession(
		ctx,
		instance.agentctl,
		agentConfig,
		instance.ACPSessionID, // Existing session ID to resume (if any)
		instance.WorkspacePath,
	)
	if err != nil {
		m.logger.Error("session initialization failed",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		return err
	}

	m.logger.Info("ACP session initialized",
		zap.String("instance_id", instance.ID),
		zap.String("agent_name", result.AgentName),
		zap.String("agent_version", result.AgentVersion),
		zap.String("session_id", result.SessionID))

	instance.ACPSessionID = result.SessionID
	m.publishACPSessionCreated(instance, result.SessionID)

	// Set up WebSocket streams using StreamManager
	// Use a ready channel to signal when the updates stream is connected
	updatesReady := make(chan struct{})
	m.streamManager.ConnectAll(instance, updatesReady)

	// Note: Shell stream is started on-demand when a client sends shell.subscribe
	// This ensures the buffered output (including initial prompt) is received
	// when a client is actually listening

	// Wait for the updates stream to connect before sending prompt
	// This prevents race conditions where notifications are sent before streams are ready
	select {
	case <-updatesReady:
		m.logger.Debug("updates stream ready")
	case <-time.After(5 * time.Second):
		m.logger.Warn("timeout waiting for updates stream to connect, proceeding anyway")
	}

	// 7. Send the task prompt if provided
	if taskDescription != "" {
		m.logger.Info("sending ACP prompt",
			zap.String("instance_id", instance.ID),
			zap.String("session_id", instance.ACPSessionID),
			zap.String("task_description", taskDescription))

		// Clear buffers before starting prompt
		instance.messageMu.Lock()
		instance.messageBuffer.Reset()
		instance.reasoningBuffer.Reset()
		instance.summaryBuffer.Reset()
		instance.messageMu.Unlock()

		// Prompt is SYNCHRONOUS - it blocks until the agent completes the task
		// Use a long timeout context for this
		promptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		resp, err := instance.agentctl.Prompt(promptCtx, taskDescription)
		if err != nil {
			m.logger.Error("ACP prompt failed",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
			return fmt.Errorf("prompt failed: %w", err)
		}

		// Extract accumulated content from buffers
		instance.messageMu.Lock()
		agentMessage := instance.messageBuffer.String()
		instance.messageBuffer.Reset()
		instance.reasoningBuffer.Reset()
		instance.summaryBuffer.Reset()
		instance.messageMu.Unlock()

		stopReason := ""
		if resp != nil {
			stopReason = string(resp.StopReason)
		}
		m.logger.Info("ACP prompt completed",
			zap.String("instance_id", instance.ID),
			zap.String("stop_reason", stopReason))

		// Publish prompt_complete event with the agent's response (for saving as comment)
		m.publishPromptComplete(instance, agentMessage, "", "")

		// Prompt completed - mark agent as READY for follow-up prompts
		m.logger.Info("agent ready for follow-up prompts",
			zap.String("instance_id", instance.ID))
		if err := m.MarkReady(instance.ID); err != nil {
			m.logger.Error("failed to mark instance as ready",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
		}
	} else {
		m.logger.Warn("no task description provided, marking as ready",
			zap.String("instance_id", instance.ID))
		if err := m.MarkReady(instance.ID); err != nil {
			m.logger.Error("failed to mark instance as ready",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
		}
	}

	return nil
}

// handlePermissionNotification processes incoming permission requests from the agent
func (m *Manager) handlePermissionNotification(instance *AgentInstance, notification *agentctl.PermissionNotification) {
	m.logger.Info("received permission request",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID),
		zap.String("pending_id", notification.PendingID),
		zap.String("title", notification.Title),
		zap.Int("num_options", len(notification.Options)))

	// Publish permission request event to the event bus
	m.publishPermissionRequest(instance, notification)
}

// publishPermissionRequest publishes a permission request event
func (m *Manager) publishPermissionRequest(instance *AgentInstance, notification *agentctl.PermissionNotification) {
	// Convert options to a serializable format
	options := make([]map[string]interface{}, len(notification.Options))
	for i, opt := range notification.Options {
		options[i] = map[string]interface{}{
			"option_id": opt.OptionID,
			"name":      opt.Name,
			"kind":      opt.Kind,
		}
	}

	data := map[string]interface{}{
		"type":              "permission_request",
		"task_id":           instance.TaskID,
		"agent_instance_id": instance.ID,
		"pending_id":        notification.PendingID,
		"session_id":        notification.SessionID,
		"tool_call_id":      notification.ToolCallID,
		"title":             notification.Title,
		"options":           options,
		"created_at":        notification.CreatedAt,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish permission_request event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	} else {
		m.logger.Info("published permission_request event",
			zap.String("task_id", instance.TaskID),
			zap.String("pending_id", notification.PendingID),
			zap.String("title", notification.Title))
	}
}

// handleSessionUpdate processes incoming session updates from the agent
func (m *Manager) handleSessionUpdate(instance *AgentInstance, update agentctl.SessionUpdate) {
	// Handle different update types based on the Type field
	switch update.Type {
	case "message_chunk":
		m.updateInstanceProgress(instance.ID, 50)

		// Accumulate message content for saving as comment when a step completes
		if update.Text != "" {
			instance.messageMu.Lock()
			instance.messageBuffer.WriteString(update.Text)
			instance.messageMu.Unlock()
		}

	case "reasoning":
		// Accumulate reasoning/thinking content
		instance.messageMu.Lock()
		if update.ReasoningText != "" {
			instance.reasoningBuffer.WriteString(update.ReasoningText)
		}
		if update.ReasoningSummary != "" {
			instance.summaryBuffer.WriteString(update.ReasoningSummary)
		}
		instance.messageMu.Unlock()

	case "tool_call":
		// Tool call starting marks a step boundary - flush the accumulated message as a comment
		// This way, each agent response before a tool use becomes a separate comment
		m.flushMessageBufferAsComment(instance)

		m.logger.Info("tool call started",
			zap.String("instance_id", instance.ID),
			zap.String("tool_call_id", update.ToolCallID),
			zap.String("title", update.ToolTitle),
			zap.String("tool_name", update.ToolName))
		m.updateInstanceProgress(instance.ID, 60)

		// Publish tool call as a comment so it appears in the chat
		m.publishToolCall(instance, update.ToolCallID, update.ToolTitle, update.ToolStatus, update.ToolArgs)

	case "tool_update":
		// Check if tool call completed
		switch update.ToolStatus {
		case "complete", "completed":
			m.updateInstanceProgress(instance.ID, 80)
			m.publishToolCallCompleteFromUpdate(instance, update)
		case "error", "failed":
			m.publishToolCallCompleteFromUpdate(instance, update)
		}

	case "plan":
		m.logger.Info("agent plan update",
			zap.String("instance_id", instance.ID))

	case "error":
		m.logger.Error("agent error",
			zap.String("instance_id", instance.ID),
			zap.String("error", update.Error))

	case "complete":
		m.logger.Info("agent turn complete",
			zap.String("instance_id", instance.ID),
			zap.String("session_id", update.SessionID))

		// Flush accumulated message buffer as a comment
		m.flushMessageBufferAsComment(instance)

		// Mark agent as READY for follow-up prompts
		if err := m.MarkReady(instance.ID); err != nil {
			m.logger.Error("failed to mark instance as ready after complete",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
		}
	}

	// Publish session update to event bus for WebSocket streaming
	m.publishSessionUpdate(instance, update)
}

// publishSessionUpdate publishes a session update to the event bus
func (m *Manager) publishSessionUpdate(instance *AgentInstance, update agentctl.SessionUpdate) {
	if m.eventBus == nil {
		return
	}

	// Build the update data - our SessionUpdate type marshals cleanly
	updateData := map[string]interface{}{
		"type": update.Type,
	}

	if update.SessionID != "" {
		updateData["session_id"] = update.SessionID
	}
	if update.Text != "" {
		updateData["text"] = update.Text
	}
	if update.ToolCallID != "" {
		updateData["tool_call_id"] = update.ToolCallID
	}
	if update.ToolName != "" {
		updateData["tool_name"] = update.ToolName
	}
	if update.ToolTitle != "" {
		updateData["tool_title"] = update.ToolTitle
	}
	if update.ToolStatus != "" {
		updateData["tool_status"] = update.ToolStatus
	}
	if update.ToolArgs != nil {
		updateData["tool_args"] = update.ToolArgs
	}
	if update.ToolResult != nil {
		updateData["tool_result"] = update.ToolResult
	}
	if update.Error != "" {
		updateData["error"] = update.Error
	}
	if update.Data != nil {
		updateData["data"] = update.Data
	}

	// Build ACP message data
	data := map[string]interface{}{
		"type":       "session/update",
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   instance.ID,
		"task_id":    instance.TaskID,
		"session_id": update.SessionID,
		"data":       updateData,
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish session update",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	}
}

// handleGitStatusUpdate processes git status updates from the workspace tracker
func (m *Manager) handleGitStatusUpdate(instance *AgentInstance, update *agentctl.GitStatusUpdate) {
	// Store git status in instance metadata
	m.instanceStore.UpdateMetadata(instance.ID, func(metadata map[string]interface{}) map[string]interface{} {
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
	m.publishGitStatus(instance, update)
}

// publishGitStatus publishes a git status update to the event bus
func (m *Manager) publishGitStatus(instance *AgentInstance, update *agentctl.GitStatusUpdate) {
	if m.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"task_id":       instance.TaskID,
		"agent_id":      instance.ID,
		"branch":        update.Branch,
		"remote_branch": update.RemoteBranch,
		"modified":      update.Modified,
		"added":         update.Added,
		"deleted":       update.Deleted,
		"untracked":     update.Untracked,
		"renamed":       update.Renamed,
		"ahead":         update.Ahead,
		"behind":        update.Behind,
		"files":         update.Files,
		"timestamp":     update.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.GitStatusUpdated, "agent-manager", data)
	subject := events.BuildGitStatusSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish git status event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	}
}

// handleFileChangeNotification processes file change notifications from the workspace tracker
func (m *Manager) handleFileChangeNotification(instance *AgentInstance, notification *agentctl.FileChangeNotification) {
	m.publishFileChange(instance, notification)
}

// publishFileChange publishes a file change notification to the event bus
func (m *Manager) publishFileChange(instance *AgentInstance, notification *agentctl.FileChangeNotification) {
	if m.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"task_id":   instance.TaskID,
		"agent_id":  instance.ID,
		"path":      notification.Path,
		"operation": notification.Operation,
		"timestamp": notification.Timestamp.Format(time.RFC3339Nano),
	}

	event := bus.NewEvent(events.FileChangeNotified, "agent-manager", data)
	subject := events.BuildFileChangeSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish file change event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	}
}

// publishPromptComplete publishes a prompt_complete event when an agent finishes responding
// This is used to save the agent's response as a comment on the task
func (m *Manager) publishPromptComplete(instance *AgentInstance, agentMessage, reasoning, summary string) {
	if m.eventBus == nil {
		return
	}

	// Only publish if there's actual content
	if agentMessage == "" {
		return
	}

	data := map[string]interface{}{
		"type":          "prompt_complete",
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":      instance.ID,
		"task_id":       instance.TaskID,
		"agent_message": agentMessage,
	}
	if reasoning != "" {
		data["reasoning"] = reasoning
	}
	if summary != "" {
		data["summary"] = summary
	}

	event := bus.NewEvent(events.PromptComplete, "agent-manager", data)
	// Publish on task-specific subject so orchestrator can subscribe
	subject := events.PromptComplete + "." + instance.TaskID

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish prompt_complete event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	}
}

// flushMessageBufferAsComment extracts any accumulated message from the buffer and
// publishes it as a step complete event (which will be saved as a comment).
// This is called when a tool use starts to save the agent's response before the tool call.
func (m *Manager) flushMessageBufferAsComment(instance *AgentInstance) {
	instance.messageMu.Lock()
	agentMessage := instance.messageBuffer.String()
	reasoning := instance.reasoningBuffer.String()
	summary := instance.summaryBuffer.String()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	// Only publish if there's actual content (ignore whitespace-only)
	trimmed := strings.TrimSpace(agentMessage)
	if trimmed == "" {
		return
	}

	// Reuse the prompt_complete event type - the orchestrator handles it the same way
	m.publishPromptComplete(instance, agentMessage, reasoning, summary)
}

// publishToolCall publishes a tool call start event (to be saved as a comment)
func (m *Manager) publishToolCall(instance *AgentInstance, toolCallID, title, status string, args map[string]interface{}) {
	if m.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"type":         "tool_call",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     instance.ID,
		"task_id":      instance.TaskID,
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	if args != nil {
		data["args"] = args
	}

	event := bus.NewEvent(events.ToolCallStarted, "agent-manager", data)
	subject := events.ToolCallStarted + "." + instance.TaskID

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish tool_call event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	} else {
		m.logger.Debug("published tool_call event",
			zap.String("task_id", instance.TaskID),
			zap.String("tool_call_id", toolCallID),
			zap.String("title", title))
	}
}

// publishToolCallCompleteFromUpdate publishes a tool call completion event from a SessionUpdate
func (m *Manager) publishToolCallCompleteFromUpdate(instance *AgentInstance, update agentctl.SessionUpdate) {
	if m.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"type":         "tool_call_complete",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     instance.ID,
		"task_id":      instance.TaskID,
		"tool_call_id": update.ToolCallID,
		"status":       update.ToolStatus,
	}

	event := bus.NewEvent(events.ToolCallComplete, "agent-manager", data)
	subject := events.ToolCallComplete + "." + instance.TaskID

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish tool_call_complete event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	}
}

// updateInstanceProgress updates an instance's progress
func (m *Manager) updateInstanceProgress(instanceID string, progress int) {
	m.instanceStore.UpdateProgress(instanceID, progress)
}

// updateInstanceError updates an instance with an error
func (m *Manager) updateInstanceError(instanceID, errorMsg string) {
	m.instanceStore.UpdateError(instanceID, errorMsg)
}

// PromptAgent sends a follow-up prompt to a running agent
// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// PromptAgent sends a follow-up prompt to a running agent
func (m *Manager) PromptAgent(ctx context.Context, instanceID string, prompt string) (*PromptResult, error) {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}

	if instance.agentctl == nil {
		return nil, fmt.Errorf("instance %q has no agentctl client", instanceID)
	}

	// Accept both RUNNING (initial) and READY (after first prompt) states
	if instance.Status != v1.AgentStatusRunning && instance.Status != v1.AgentStatusReady {
		return nil, fmt.Errorf("instance %q is not ready for prompts (status: %s)", instanceID, instance.Status)
	}

	// Set status to RUNNING while processing
	m.instanceStore.UpdateStatus(instanceID, v1.AgentStatusRunning)

	// Clear buffers before starting new prompt
	instance.messageMu.Lock()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	m.logger.Info("sending prompt to agent",
		zap.String("instance_id", instanceID),
		zap.Int("prompt_length", len(prompt)))

	// Prompt is synchronous - blocks until agent completes
	resp, err := instance.agentctl.Prompt(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Extract accumulated content from buffers
	instance.messageMu.Lock()
	agentMessage := instance.messageBuffer.String()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	result := &PromptResult{
		StopReason:   string(resp.StopReason),
		AgentMessage: agentMessage,
	}

	// Publish prompt_complete event with the agent's response (for saving as comment)
	m.publishPromptComplete(instance, agentMessage, "", "")

	// Prompt completed - mark as READY for next prompt
	if err := m.MarkReady(instanceID); err != nil {
		m.logger.Error("failed to mark instance as ready after prompt",
			zap.String("instance_id", instanceID),
			zap.Error(err))
	}

	return result, nil
}

// StopAgent stops an agent instance
func (m *Manager) StopAgent(ctx context.Context, instanceID string, force bool) error {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	runtimeName := "unknown"
	if m.runtime != nil {
		runtimeName = m.runtime.Name()
	}
	m.logger.Info("stopping agent",
		zap.String("instance_id", instanceID),
		zap.Bool("force", force),
		zap.String("runtime", runtimeName))

	// Try to gracefully stop via agentctl first
	if instance.agentctl != nil && !force {
		if err := instance.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("instance_id", instanceID),
				zap.Error(err))
		}
		instance.agentctl.Close()
	}

	// Stop the agent instance via runtime
	if m.runtime != nil {
		runtimeInstance := &RuntimeInstance{
			InstanceID:           instance.ID,
			TaskID:               instance.TaskID,
			ContainerID:          instance.ContainerID,
			StandaloneInstanceID: instance.standaloneInstanceID,
			StandalonePort:       instance.standalonePort,
		}
		if err := m.runtime.StopInstance(ctx, runtimeInstance, force); err != nil {
			return err
		}
	}

	// Update instance status and remove from tracking
	m.instanceStore.WithLock(instanceID, func(inst *AgentInstance) {
		inst.Status = v1.AgentStatusStopped
		now := time.Now()
		inst.FinishedAt = &now
	})
	m.instanceStore.Remove(instanceID)

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("instance_id", instanceID),
		zap.String("task_id", instance.TaskID))

	// Publish stopped event
	m.publishEvent(ctx, events.AgentStopped, instance)

	return nil
}

// StopByTaskID stops the agent for a specific task
func (m *Manager) StopByTaskID(ctx context.Context, taskID string, force bool) error {
	instance, exists := m.instanceStore.GetByTaskID(taskID)
	if !exists {
		return fmt.Errorf("no agent running for task %q", taskID)
	}

	return m.StopAgent(ctx, instance.ID, force)
}

// GetInstance returns an agent instance by ID
func (m *Manager) GetInstance(instanceID string) (*AgentInstance, bool) {
	return m.instanceStore.Get(instanceID)
}

// GetInstanceByTaskID returns the agent instance for a task
func (m *Manager) GetInstanceByTaskID(taskID string) (*AgentInstance, bool) {
	return m.instanceStore.GetByTaskID(taskID)
}

// GetInstanceByContainerID returns the agent instance for a container
func (m *Manager) GetInstanceByContainerID(containerID string) (*AgentInstance, bool) {
	return m.instanceStore.GetByContainerID(containerID)
}

// ListInstances returns all active instances
func (m *Manager) ListInstances() []*AgentInstance {
	return m.instanceStore.List()
}

// IsAgentRunningForTask checks if an agent is actually running for a task
// This probes agentctl's status endpoint to verify the agent process is running
func (m *Manager) IsAgentRunningForTask(ctx context.Context, taskID string) bool {
	// First check if we have an instance tracked for this task
	instance, exists := m.GetInstanceByTaskID(taskID)
	if !exists {
		return false
	}

	// Probe agentctl status to verify the agent process is running
	if instance.agentctl == nil {
		return false
	}

	status, err := instance.agentctl.GetStatus(ctx)
	if err != nil {
		m.logger.Debug("failed to get agentctl status",
			zap.String("task_id", taskID),
			zap.Error(err))
		return false
	}

	return status.IsAgentRunning()
}

// UpdateStatus updates the status of an instance
func (m *Manager) UpdateStatus(instanceID string, status v1.AgentStatus) error {
	if !m.instanceStore.WithLock(instanceID, func(instance *AgentInstance) {
		instance.Status = status
	}) {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	m.logger.Debug("updated instance status",
		zap.String("instance_id", instanceID),
		zap.String("status", string(status)))

	return nil
}

// UpdateProgress updates the progress of an instance
func (m *Manager) UpdateProgress(instanceID string, progress int) error {
	if !m.instanceStore.WithLock(instanceID, func(instance *AgentInstance) {
		instance.Progress = progress
	}) {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	m.logger.Debug("updated instance progress",
		zap.String("instance_id", instanceID),
		zap.Int("progress", progress))

	return nil
}

// MarkReady marks an instance as ready for follow-up prompts
func (m *Manager) MarkReady(instanceID string) error {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	m.instanceStore.UpdateStatus(instanceID, v1.AgentStatusReady)

	m.logger.Info("instance ready for follow-up prompts",
		zap.String("instance_id", instanceID))

	// Publish ready event
	m.publishEvent(context.Background(), events.AgentReady, instance)

	return nil
}

// MarkCompleted marks an instance as completed
func (m *Manager) MarkCompleted(instanceID string, exitCode int, errorMessage string) error {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	m.instanceStore.WithLock(instanceID, func(inst *AgentInstance) {
		now := time.Now()
		inst.FinishedAt = &now
		inst.ExitCode = &exitCode
		inst.ErrorMessage = errorMessage

		if exitCode == 0 && errorMessage == "" {
			inst.Status = v1.AgentStatusCompleted
			inst.Progress = 100
		} else {
			inst.Status = v1.AgentStatusFailed
		}
	})

	m.logger.Info("instance completed",
		zap.String("instance_id", instanceID),
		zap.Int("exit_code", exitCode),
		zap.String("status", string(instance.Status)))

	// Publish completion event
	eventType := events.AgentCompleted
	if instance.Status == v1.AgentStatusFailed {
		eventType = events.AgentFailed
	}
	m.publishEvent(context.Background(), eventType, instance)

	return nil
}

// RemoveInstance removes a completed instance from tracking
func (m *Manager) RemoveInstance(instanceID string) {
	m.instanceStore.Remove(instanceID)
	m.logger.Debug("removed instance from tracking",
		zap.String("instance_id", instanceID))
}

// CleanupStaleInstanceByTaskID removes a stale agent instance from tracking without trying to stop it.
// This is used when we detect the agent process has stopped but the instance is still tracked.
func (m *Manager) CleanupStaleInstanceByTaskID(ctx context.Context, taskID string) error {
	instance, exists := m.instanceStore.GetByTaskID(taskID)
	if !exists {
		return nil // No instance to clean up
	}

	m.logger.Info("cleaning up stale agent instance",
		zap.String("task_id", taskID),
		zap.String("instance_id", instance.ID))

	// Close agentctl connection if it exists
	if instance.agentctl != nil {
		instance.agentctl.Close()
	}

	// Remove from instance store
	m.instanceStore.Remove(instance.ID)

	return nil
}

// publishEvent publishes an agent lifecycle event to NATS
func (m *Manager) publishEvent(ctx context.Context, eventType string, instance *AgentInstance) {
	if m.eventBus == nil {
		return
	}

	data := map[string]interface{}{
		"instance_id":      instance.ID,
		"task_id":          instance.TaskID,
		"agent_profile_id": instance.AgentProfileID,
		"container_id":     instance.ContainerID,
		"status":           string(instance.Status),
		"started_at":       instance.StartedAt,
		"progress":         instance.Progress,
		"error_message":    instance.ErrorMessage,
	}

	if instance.FinishedAt != nil {
		data["finished_at"] = *instance.FinishedAt
	}
	if instance.ExitCode != nil {
		data["exit_code"] = *instance.ExitCode
	}

	event := bus.NewEvent(eventType, "agent-manager", data)

	if err := m.eventBus.Publish(ctx, eventType, event); err != nil {
		m.logger.Error("failed to publish event",
			zap.String("event_type", eventType),
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	} else {
		m.logger.Debug("published event",
			zap.String("event_type", eventType),
			zap.String("instance_id", instance.ID))
	}
}

func (m *Manager) publishACPSessionCreated(instance *AgentInstance, sessionID string) {
	if m.eventBus == nil || sessionID == "" {
		return
	}

	data := map[string]interface{}{
		"task_id":           instance.TaskID,
		"agent_instance_id": instance.ID,
		"acp_session_id":    sessionID,
	}

	event := bus.NewEvent(events.AgentACPSessionCreated, "agent-manager", data)
	if err := m.eventBus.Publish(context.Background(), events.AgentACPSessionCreated, event); err != nil {
		m.logger.Error("failed to publish ACP session event",
			zap.String("event_type", events.AgentACPSessionCreated),
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	} else {
		m.logger.Info("published ACP session event",
			zap.String("event_type", events.AgentACPSessionCreated),
			zap.String("task_id", instance.TaskID),
			zap.String("agent_instance_id", instance.ID),
			zap.String("acp_session_id", sessionID))
	}
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
			instance, tracked := m.instanceStore.GetByContainerID(container.ID)
			if tracked {
				// Get container info to get exit code
				info, err := m.containerManager.GetContainerInfo(ctx, container.ID)
				if err != nil {
					m.logger.Warn("failed to get container info during cleanup",
						zap.String("container_id", container.ID),
						zap.Error(err))
					continue
				}

				// Mark instance as completed
				errorMsg := ""
				if info.ExitCode != 0 {
					errorMsg = fmt.Sprintf("container exited with code %d", info.ExitCode)
				}
				_ = m.MarkCompleted(instance.ID, info.ExitCode, errorMsg)

				// Remove the container
				if err := m.containerManager.RemoveContainer(ctx, container.ID, false); err != nil {
					m.logger.Warn("failed to remove container during cleanup",
						zap.String("container_id", container.ID),
						zap.Error(err))
				}

				// Remove the instance from tracking so new agents can be launched
				m.RemoveInstance(instance.ID)
			}
		}
	}
}

// RespondToPermission sends a response to a permission request for an agent instance
func (m *Manager) RespondToPermission(instanceID, pendingID, optionID string, cancelled bool) error {
	instance, exists := m.instanceStore.Get(instanceID)
	if !exists {
		return fmt.Errorf("agent instance not found: %s", instanceID)
	}

	if instance.agentctl == nil {
		return fmt.Errorf("agent instance has no agentctl client: %s", instanceID)
	}

	m.logger.Info("responding to permission request",
		zap.String("instance_id", instanceID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return instance.agentctl.RespondToPermission(ctx, pendingID, optionID, cancelled)
}

// RespondToPermissionByTaskID sends a response to a permission request for a task
func (m *Manager) RespondToPermissionByTaskID(taskID, pendingID, optionID string, cancelled bool) error {
	instance, exists := m.instanceStore.GetByTaskID(taskID)
	if !exists {
		return fmt.Errorf("no agent instance found for task: %s", taskID)
	}

	return m.RespondToPermission(instance.ID, pendingID, optionID, cancelled)
}

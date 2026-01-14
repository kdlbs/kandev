// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agentctl"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/worktree"
	"github.com/kandev/kandev/internal/common/config"
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
	ContainerIP   string // IP address of the container for agentctl communication
	WorkspacePath string // Path to the workspace (worktree or repository path)
	Status        v1.AgentStatus
	StartedAt     time.Time
	FinishedAt    *time.Time
	ExitCode      *int
	ErrorMessage  string
	Progress      int
	Metadata      map[string]interface{}

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
	TaskTitle       string                 // Human-readable task title for semantic worktree naming
	AgentProfileID  string
	WorkspacePath   string                 // Host path to workspace (original repository path)
	TaskDescription string                 // Task description to send via ACP prompt
	Env             map[string]string      // Additional env vars
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
	docker          *docker.Client
	registry        *registry.Registry
	eventBus        bus.EventBus
	credsMgr        CredentialsManager
	profileResolver ProfileResolver
	worktreeMgr     *worktree.Manager
	logger          *logger.Logger

	// Agent runtime configuration
	agentCfg      config.AgentConfig
	standaloneCtl *agentctl.StandaloneCtl // Client for standalone agentctl control API

	// Track active instances
	instances   map[string]*AgentInstance // by instance ID
	byTask      map[string]string         // taskID -> instanceID
	byContainer map[string]string         // containerID -> instanceID
	mu          sync.RWMutex

	// Background cleanup
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewManager creates a new lifecycle manager
func NewManager(
	dockerClient *docker.Client,
	reg *registry.Registry,
	eventBus bus.EventBus,
	agentCfg config.AgentConfig,
	log *logger.Logger,
) *Manager {
	mgr := &Manager{
		docker:          dockerClient,
		registry:        reg,
		eventBus:        eventBus,
		agentCfg:        agentCfg,
		logger:          log.WithFields(zap.String("component", "lifecycle-manager")),
		instances:       make(map[string]*AgentInstance),
		byTask:          make(map[string]string),
		byContainer:     make(map[string]string),
		cleanupInterval: 30 * time.Second,
		stopCh:          make(chan struct{}),
	}

	// Initialize standalone agentctl client if in standalone mode
	if agentCfg.Runtime == "standalone" {
		mgr.standaloneCtl = agentctl.NewStandaloneCtl(
			agentCfg.StandaloneHost,
			agentCfg.StandalonePort,
			log,
		)
		mgr.logger.Info("initialized for standalone mode",
			zap.String("host", agentCfg.StandaloneHost),
			zap.Int("port", agentCfg.StandalonePort))
	}

	return mgr
}

// IsStandaloneMode returns true if running in standalone mode
func (m *Manager) IsStandaloneMode() bool {
	return m.agentCfg.Runtime == "standalone"
}

// SetCredentialsManager sets the credentials manager for injecting secrets
func (m *Manager) SetCredentialsManager(credsMgr CredentialsManager) {
	m.credsMgr = credsMgr
}

// SetProfileResolver sets the profile resolver for looking up agent profiles
func (m *Manager) SetProfileResolver(resolver ProfileResolver) {
	m.profileResolver = resolver
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
}

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting lifecycle manager",
		zap.String("runtime", m.agentCfg.Runtime))

	if m.IsStandaloneMode() {
		// Check if standalone agentctl is reachable
		if err := m.standaloneCtl.Health(ctx); err != nil {
			m.logger.Warn("standalone agentctl not reachable",
				zap.String("host", m.agentCfg.StandaloneHost),
				zap.Int("port", m.agentCfg.StandalonePort),
				zap.Error(err))
			// Continue anyway - it might come up later
		} else {
			m.logger.Info("standalone agentctl is reachable")
		}
	} else {
		// Docker mode - reconcile with Docker to find running containers from previous runs
		if _, err := m.reconcileWithDocker(ctx); err != nil {
			m.logger.Warn("failed to reconcile with Docker", zap.Error(err))
			// Continue anyway - this is best-effort
		}

		// Start cleanup loop only if docker client is available
		if m.docker != nil {
			m.wg.Add(1)
			go m.cleanupLoop(ctx)
			m.logger.Info("cleanup loop started")
		} else {
			m.logger.Warn("cleanup loop not started - no docker client available")
		}
	}

	return nil
}

// RecoveredInstance contains info about an instance recovered from Docker
type RecoveredInstance struct {
	InstanceID     string
	TaskID         string
	ContainerID    string
	AgentProfileID string
}

// reconcileWithDocker finds running kandev containers and re-populates tracking maps
// Returns a list of recovered instances so the caller can sync with the database
func (m *Manager) reconcileWithDocker(ctx context.Context) ([]RecoveredInstance, error) {
	// Skip reconciliation if docker client is not available (e.g., in tests)
	if m.docker == nil {
		m.logger.Debug("skipping Docker reconciliation - no docker client")
		return nil, nil
	}

	m.logger.Info("reconciling with Docker - looking for running containers")

	// List all containers with our label
	containers, err := m.docker.ListContainers(ctx, map[string]string{
		"kandev.managed": "true",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var recovered []RecoveredInstance
	for _, ctr := range containers {
		// Only recover running containers
		if ctr.State != "running" {
			m.logger.Debug("skipping non-running container",
				zap.String("container_id", ctr.ID),
				zap.String("state", ctr.State))
			continue
		}

		// Get container details to extract labels
		labels, err := m.docker.GetContainerLabels(ctx, ctr.ID)
		if err != nil {
			m.logger.Warn("failed to get container labels",
				zap.String("container_id", ctr.ID),
				zap.Error(err))
			continue
		}

		instanceID := labels["kandev.instance_id"]
		taskID := labels["kandev.task_id"]
		agentProfileID := labels["kandev.agent_profile_id"]

		if instanceID == "" || taskID == "" {
			m.logger.Warn("container missing required labels",
				zap.String("container_id", ctr.ID))
			continue
		}

		// Get container IP
		containerIP, err := m.docker.GetContainerIP(ctx, ctr.ID)
		if err != nil {
			m.logger.Warn("failed to get container IP",
				zap.String("container_id", ctr.ID),
				zap.Error(err))
			containerIP = "localhost"
		}

		// Create instance record
		instance := &AgentInstance{
			ID:             instanceID,
			TaskID:         taskID,
			AgentProfileID: agentProfileID,
			ContainerID:    ctr.ID,
			ContainerIP: containerIP,
			Status:      v1.AgentStatusRunning,
			StartedAt:   time.Now(), // We don't know exact start time
		}

		// Create agentctl client
		instance.agentctl = agentctl.NewClient(containerIP, AgentCtlPort, m.logger)

		// Add to tracking maps
		m.instances[instanceID] = instance
		m.byTask[taskID] = instanceID
		m.byContainer[ctr.ID] = instanceID

		recovered = append(recovered, RecoveredInstance{
			InstanceID:     instanceID,
			TaskID:         taskID,
			ContainerID:    ctr.ID,
			AgentProfileID: agentProfileID,
		})

		// Reconnect to WebSocket streams for updates and permissions
		// This runs in goroutines so we don't block reconciliation
		go m.reconnectStreams(instance)

		m.logger.Info("recovered running container",
			zap.String("instance_id", instanceID),
			zap.String("task_id", taskID),
			zap.String("container_id", ctr.ID))
	}

	m.logger.Info("reconciliation complete",
		zap.Int("containers_found", len(containers)),
		zap.Int("recovered", len(recovered)))

	return recovered, nil
}

// GetRecoveredInstances returns a snapshot of all currently tracked instances
// This can be used by the orchestrator to sync with the database
func (m *Manager) GetRecoveredInstances() []RecoveredInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]RecoveredInstance, 0, len(m.instances))
	for _, inst := range m.instances {
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
	m.mu.RLock()
	if existingID, exists := m.byTask[req.TaskID]; exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("task %q already has an agent running (instance: %s)", req.TaskID, existingID)
	}
	m.mu.RUnlock()

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
			m.logger.Info("using worktree for agent",
				zap.String("task_id", req.TaskID),
				zap.String("worktree_id", worktreeID),
				zap.String("worktree_path", workspacePath),
				zap.String("main_repo_git_dir", mainRepoGitDir),
				zap.String("branch", wt.Branch))
		}
	} else if req.RepositoryPath != "" && workspacePath == "" {
		// No worktree, but we have a repository path - use it as workspace
		workspacePath = req.RepositoryPath
		m.logger.Info("using repository path as workspace (no worktree)",
			zap.String("task_id", req.TaskID),
			zap.String("workspace_path", workspacePath))
	}

	// 5. Generate a new instance ID
	instanceID := uuid.New().String()

	// 6. Prepare request with worktree path
	reqWithWorktree := *req
	reqWithWorktree.WorkspacePath = workspacePath

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

	// 7. Launch via appropriate runtime (Docker or Standalone)
	var instance *AgentInstance
	if m.IsStandaloneMode() {
		instance, err = m.launchStandalone(ctx, instanceID, &reqWithWorktree, agentConfig, profileInfo, worktreeID, worktreeBranch)
	} else {
		instance, err = m.launchDocker(ctx, instanceID, &reqWithWorktree, agentConfig, profileInfo, mainRepoGitDir, worktreeID, worktreeBranch)
	}
	if err != nil {
		return nil, err
	}

	// 8. Track the instance
	m.mu.Lock()
	m.instances[instanceID] = instance
	m.byTask[req.TaskID] = instanceID
	if instance.ContainerID != "" {
		m.byContainer[instance.ContainerID] = instanceID
	}
	m.mu.Unlock()

	// 9. Publish agent.started event
	m.publishEvent(ctx, events.AgentStarted, instance)

	// 10. Wait for agentctl to be ready and start the agent process
	go m.initializeAgent(instance, req.TaskDescription)

	m.logger.Info("agent launched successfully",
		zap.String("instance_id", instanceID),
		zap.String("task_id", req.TaskID),
		zap.String("runtime", m.agentCfg.Runtime))

	return instance, nil
}

// launchDocker launches an agent in a Docker container
func (m *Manager) launchDocker(ctx context.Context, instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig, profileInfo *AgentProfileInfo, mainRepoGitDir, worktreeID, worktreeBranch string) (*AgentInstance, error) {
	// Build container config from registry config
	containerConfig := m.buildContainerConfig(instanceID, req, agentConfig, profileInfo, mainRepoGitDir)

	// Create and start the container
	containerID, err := m.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		_ = m.docker.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get container IP for agentctl communication
	containerIP, err := m.docker.GetContainerIP(ctx, containerID)
	if err != nil {
		m.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID),
			zap.Error(err))
		containerIP = "127.0.0.1"
	}

	// Create agentctl client pointing to the container
	agentctlClient := agentctl.NewClient(containerIP, AgentCtlPort, m.logger)

	// Build instance metadata
	instanceMetadata := req.Metadata
	if instanceMetadata == nil {
		instanceMetadata = make(map[string]interface{})
	}
	if worktreeID != "" {
		instanceMetadata["worktree_id"] = worktreeID
		instanceMetadata["worktree_path"] = req.WorkspacePath
		instanceMetadata["worktree_branch"] = worktreeBranch
	}

	m.logger.Info("docker agent created",
		zap.String("instance_id", instanceID),
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP))

	return &AgentInstance{
		ID:             instanceID,
		TaskID:         req.TaskID,
		AgentProfileID: req.AgentProfileID,
		ContainerID:    containerID,
		ContainerIP:    containerIP,
		WorkspacePath:  "/workspace", // Docker mounts workspace to /workspace
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
		Progress:       0,
		Metadata:       instanceMetadata,
		agentctl:       agentctlClient,
	}, nil
}

// launchStandalone launches an agent via standalone agentctl
func (m *Manager) launchStandalone(ctx context.Context, instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig, profileInfo *AgentProfileInfo, worktreeID, worktreeBranch string) (*AgentInstance, error) {
	// Build environment variables for the agent
	env := m.buildStandaloneEnv(instanceID, req, agentConfig)

	// Build agent command from registry config
	// If Cmd is specified, join it; otherwise use default auggie command
	agentCommand := ""
	if len(agentConfig.Cmd) > 0 {
		agentCommand = strings.Join(agentConfig.Cmd, " ")
	}
	// Append model flag if agent supports it and profile has a model configured
	if agentConfig.ModelFlag != "" && profileInfo != nil && profileInfo.Model != "" {
		agentCommand = agentCommand + " " + agentConfig.ModelFlag + " " + profileInfo.Model
	}
	// If empty, agentctl will use its default (auggie --acp)

	// Create instance via control API
	createReq := &agentctl.CreateInstanceRequest{
		ID:            instanceID,
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  agentCommand,
		Protocol:      string(agentConfig.Protocol), // Pass protocol from registry
		Env:           env,
		AutoStart:     false, // We'll start via ACP initialize flow
	}

	resp, err := m.standaloneCtl.CreateInstance(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone instance: %w", err)
	}

	// Create agentctl client pointing to the instance port
	agentctlClient := agentctl.NewClient(m.agentCfg.StandaloneHost, resp.Port, m.logger)

	// Build instance metadata
	instanceMetadata := req.Metadata
	if instanceMetadata == nil {
		instanceMetadata = make(map[string]interface{})
	}
	if worktreeID != "" {
		instanceMetadata["worktree_id"] = worktreeID
		instanceMetadata["worktree_path"] = req.WorkspacePath
		instanceMetadata["worktree_branch"] = worktreeBranch
	}
	// Store standalone-specific info in metadata
	instanceMetadata["standalone_port"] = resp.Port

	m.logger.Info("standalone agent created",
		zap.String("instance_id", instanceID),
		zap.Int("port", resp.Port),
		zap.String("workspace", req.WorkspacePath))

	return &AgentInstance{
		ID:                   instanceID,
		TaskID:               req.TaskID,
		AgentProfileID:       req.AgentProfileID,
		WorkspacePath:        req.WorkspacePath,
		Status:               v1.AgentStatusRunning,
		StartedAt:            time.Now(),
		Progress:             0,
		Metadata:             instanceMetadata,
		agentctl:             agentctlClient,
		standaloneInstanceID: resp.ID,
		standalonePort:       resp.Port,
	}, nil
}

// buildStandaloneEnv builds environment variables for a standalone agent instance
func (m *Manager) buildStandaloneEnv(instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig) map[string]string {
	env := make(map[string]string)

	// Copy request environment
	for k, v := range req.Env {
		env[k] = v
	}

	// Add standard variables
	env["KANDEV_INSTANCE_ID"] = instanceID
	env["KANDEV_TASK_ID"] = req.TaskID
	env["TASK_DESCRIPTION"] = req.TaskDescription

	// Add credentials if available
	if m.credsMgr != nil {
		ctx := context.Background()
		if sessionAuth, err := m.credsMgr.GetCredentialValue(ctx, "AUGMENT_SESSION_AUTH"); err == nil && sessionAuth != "" {
			env["AUGMENT_SESSION_AUTH"] = sessionAuth
		}
	}

	// Check for ACP session resumption
	// Only pass session ID if the session file exists locally
	// Docker sessions won't exist on standalone hosts and vice versa
	if req.Metadata != nil {
		if sessionID, ok := req.Metadata["auggie_session_id"].(string); ok && sessionID != "" {
			// Check if session file exists
			homeDir, _ := os.UserHomeDir()
			sessionPath := filepath.Join(homeDir, ".augment", "sessions", sessionID+".json")
			if _, err := os.Stat(sessionPath); err == nil {
				env["AUGGIE_SESSION_ID"] = sessionID
			} else {
				// Session file doesn't exist - don't pass the ID
				// This happens when switching between Docker and standalone modes
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

// initializeAgent waits for agentctl to be ready and initializes the agent
func (m *Manager) initializeAgent(instance *AgentInstance, taskDescription string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	m.logger.Info("waiting for agentctl to be ready",
		zap.String("instance_id", instance.ID),
		zap.String("url", instance.agentctl.BaseURL()))

	// Wait for agentctl HTTP server to be ready
	if err := instance.agentctl.WaitForReady(ctx, 60*time.Second); err != nil {
		m.logger.Error("agentctl not ready",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		m.updateInstanceError(instance.ID, "agentctl not ready: "+err.Error())
		return
	}

	// Start the agent process via agentctl
	if err := instance.agentctl.Start(ctx); err != nil {
		m.logger.Error("failed to start agent via agentctl",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		m.updateInstanceError(instance.ID, "failed to start agent: "+err.Error())
		return
	}

	m.logger.Info("agent process started via agentctl",
		zap.String("instance_id", instance.ID))

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	// Initialize ACP session
	if err := m.initializeACPSession(ctx, instance, taskDescription); err != nil {
		m.logger.Error("failed to initialize ACP session",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		m.updateInstanceError(instance.ID, "failed to initialize ACP: "+err.Error())
		return
	}
}

// initializeACPSession sends the ACP initialization messages using the acp-go-sdk
func (m *Manager) initializeACPSession(ctx context.Context, instance *AgentInstance, taskDescription string) error {
	m.logger.Info("initializing ACP session",
		zap.String("instance_id", instance.ID),
		zap.String("agentctl_url", instance.agentctl.BaseURL()))

	// 1. Send initialize request
	m.logger.Info("sending ACP initialize request",
		zap.String("instance_id", instance.ID))

	agentInfo, err := instance.agentctl.Initialize(ctx, "kandev", "1.0.0")
	if err != nil {
		m.logger.Error("ACP initialize failed",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		return fmt.Errorf("initialize failed: %w", err)
	}
	agentName := "unknown"
	agentVersion := "unknown"
	if agentInfo != nil {
		agentName = agentInfo.Name
		agentVersion = agentInfo.Version
	}
	m.logger.Info("ACP initialize response received",
		zap.String("instance_id", instance.ID),
		zap.String("agent_name", agentName),
		zap.String("agent_version", agentVersion))

	// 2. Create new session
	m.logger.Info("sending ACP session/new request",
		zap.String("instance_id", instance.ID),
		zap.String("workspace_path", instance.WorkspacePath))

	sessionID, err := instance.agentctl.NewSession(ctx, instance.WorkspacePath)
	if err != nil {
		m.logger.Error("ACP session/new failed",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		return fmt.Errorf("session/new failed: %w", err)
	}
	m.logger.Info("ACP session created",
		zap.String("instance_id", instance.ID),
		zap.String("session_id", sessionID))

	// Store session ID in instance
	m.mu.Lock()
	if inst, exists := m.instances[instance.ID]; exists {
		if inst.Metadata == nil {
			inst.Metadata = make(map[string]interface{})
		}
		inst.Metadata["acp_session_id"] = sessionID
	}
	m.mu.Unlock()

	// Publish session_info event so the orchestrator can store it in task metadata
	m.publishSessionInfo(instance, sessionID)

	// 3. Set up updates stream to receive session notifications
	// Use a ready channel to signal when the stream is connected
	updatesReady := make(chan struct{})
	go m.handleUpdatesStream(instance, updatesReady)

	// 4. Set up permission stream to receive permission requests
	go m.handlePermissionStream(instance)

	// 5. Set up git status stream to track workspace changes
	go m.handleGitStatusStream(instance)

	// 6. Set up file changes stream to track filesystem changes
	go m.handleFileChangesStream(instance)

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
			zap.String("session_id", sessionID),
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

// handleUpdatesStream handles streaming session notifications from the agent.
// If ready is non-nil, it will be closed when the stream connection is established (or fails).
func (m *Manager) handleUpdatesStream(instance *AgentInstance, ready chan<- struct{}) {
	ctx := context.Background()

	err := instance.agentctl.StreamUpdates(ctx, func(update agentctl.SessionUpdate) {
		m.handleSessionUpdate(instance, update)
	})

	// Signal that the stream connection attempt is complete (success or failure)
	// StreamUpdates returns immediately after establishing the WebSocket connection
	// and starting the read goroutine, so this signals that we're ready to receive updates
	if ready != nil {
		close(ready)
	}

	if err != nil {
		m.logger.Error("failed to connect to updates stream",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	}
}

// handlePermissionStream handles streaming permission requests from the agent
func (m *Manager) handlePermissionStream(instance *AgentInstance) {
	ctx := context.Background()

	err := instance.agentctl.StreamPermissions(ctx, func(notification *agentctl.PermissionNotification) {
		m.handlePermissionNotification(instance, notification)
	})
	if err != nil {
		m.logger.Error("failed to connect to permission stream",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
	}
}

// handleGitStatusStream handles streaming git status updates from the agent workspace
func (m *Manager) handleGitStatusStream(instance *AgentInstance) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := instance.agentctl.StreamGitStatus(ctx, func(update *agentctl.GitStatusUpdate) {
			m.handleGitStatusUpdate(instance, update)
		})

		if err == nil {
			// Connection closed normally
			return
		}

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	m.logger.Error("failed to connect to git status stream after retries",
		zap.String("instance_id", instance.ID),
		zap.Int("max_retries", maxRetries))
}

// reconnectStreams reconnects to agent WebSocket streams after backend restart
// This is called for recovered instances that were running before the restart
func (m *Manager) reconnectStreams(instance *AgentInstance) {
	m.logger.Info("reconnecting to agent streams after recovery",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID))

	// Wait a moment for any startup operations to settle
	time.Sleep(500 * time.Millisecond)

	// Check if agentctl is responsive
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := instance.agentctl.WaitForReady(ctx, 10*time.Second); err != nil {
		m.logger.Warn("agentctl not ready for stream reconnection",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		// Don't return - still try to connect to streams
	}

	// Reconnect to WebSocket streams
	go m.handleUpdatesStream(instance, nil)
	go m.handlePermissionStream(instance)
	go m.handleGitStatusStream(instance)
	go m.handleFileChangesStream(instance)

	// Mark the instance as READY so it can accept prompts
	m.mu.Lock()
	instance.Status = v1.AgentStatusReady
	m.mu.Unlock()

	m.logger.Info("agent streams reconnected, ready for prompts",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID))
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
		"type":             "permission_request",
		"task_id":          instance.TaskID,
		"agent_instance_id": instance.ID,
		"pending_id":       notification.PendingID,
		"session_id":       notification.SessionID,
		"tool_call_id":     notification.ToolCallID,
		"title":            notification.Title,
		"options":          options,
		"created_at":       notification.CreatedAt,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
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
			m.logger.Error("tool call error",
				zap.String("instance_id", instance.ID))
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

// publishSessionInfo publishes a session_info event with the session ID
// This allows the orchestrator to store the session ID in task metadata for resumption
func (m *Manager) publishSessionInfo(instance *AgentInstance, sessionID string) {
	if m.eventBus == nil {
		return
	}

	// Structure must match protocol.Message so it can be parsed correctly
	// The session_id goes in the "data" field, not at the top level
	data := map[string]interface{}{
		"type":      "session_info",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":  instance.ID,
		"task_id":   instance.TaskID,
		"data": map[string]interface{}{
			"session_id": sessionID,
		},
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish session_info event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	} else {
		m.logger.Info("published session_info event",
			zap.String("task_id", instance.TaskID),
			zap.String("session_id", sessionID))
	}
}

// handleGitStatusUpdate processes git status updates from the workspace tracker
func (m *Manager) handleGitStatusUpdate(instance *AgentInstance, update *agentctl.GitStatusUpdate) {
	// Store git status in instance metadata
	m.mu.Lock()
	if inst, exists := m.instances[instance.ID]; exists {
		if inst.Metadata == nil {
			inst.Metadata = make(map[string]interface{})
		}
		inst.Metadata["git_status"] = map[string]interface{}{
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
	}
	m.mu.Unlock()

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

// handleFileChangesStream handles streaming file change notifications from the agent workspace
func (m *Manager) handleFileChangesStream(instance *AgentInstance) {
	ctx := context.Background()

	// Retry connection with exponential backoff
	maxRetries := 5
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := instance.agentctl.StreamFileChanges(ctx, func(notification *agentctl.FileChangeNotification) {
			m.publishFileChange(instance, notification)
		})

		if err == nil {
			// Connection closed normally
			return
		}

		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	m.logger.Error("failed to connect to file changes stream after retries",
		zap.String("instance_id", instance.ID),
		zap.Int("max_retries", maxRetries))
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
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return
	}

	instance.Progress = progress
}

// updateInstanceComplete marks an instance as completed
func (m *Manager) updateInstanceComplete(instanceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return
	}

	instance.Status = v1.AgentStatusCompleted
	instance.Progress = 100
	now := time.Now()
	instance.FinishedAt = &now
}

// updateInstanceError updates an instance with an error
func (m *Manager) updateInstanceError(instanceID, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return
	}

	instance.Status = v1.AgentStatusFailed
	instance.ErrorMessage = errorMsg
	now := time.Now()
	instance.FinishedAt = &now
}

// buildContainerConfig builds a Docker container config from agent registry config
// mainRepoGitDir is the host path to the main repository's .git directory (e.g., /path/to/repo/.git)
// If non-empty, it will be mounted into the container at the same path so git worktree commands work correctly
func (m *Manager) buildContainerConfig(instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig, profileInfo *AgentProfileInfo, mainRepoGitDir string) docker.ContainerConfig {
	// Build image name with tag
	imageName := agentConfig.Image
	if agentConfig.Tag != "" {
		imageName = fmt.Sprintf("%s:%s", agentConfig.Image, agentConfig.Tag)
	}

	// Expand mount templates
	mounts := make([]docker.MountConfig, 0, len(agentConfig.Mounts))
	for _, mt := range agentConfig.Mounts {
		// Skip workspace mounts if no workspace path is provided
		if strings.Contains(mt.Source, "{workspace}") && req.WorkspacePath == "" {
			m.logger.Debug("skipping workspace mount - no workspace path provided",
				zap.String("target", mt.Target))
			continue
		}
		source := m.expandMountTemplate(mt.Source, req.WorkspacePath, req.TaskID)
		mounts = append(mounts, docker.MountConfig{
			Source:   source,
			Target:   mt.Target,
			ReadOnly: mt.ReadOnly,
		})
	}

	// Add main repository's .git directory mount if using a worktree
	// Git worktrees have a .git file pointing to {repo}/.git/worktrees/{name}
	// That metadata directory has a 'commondir' file pointing back to the main .git
	// We need to mount the entire .git directory for git commands to work
	if mainRepoGitDir != "" {
		mounts = append(mounts, docker.MountConfig{
			Source:   mainRepoGitDir,
			Target:   mainRepoGitDir, // Same path inside container
			ReadOnly: false,
		})
		m.logger.Debug("added main repo .git directory mount for worktree",
			zap.String("path", mainRepoGitDir))
	}

	// Merge environment variables
	env := make([]string, 0)
	// Add default env from agent config
	for k, v := range agentConfig.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Add request-specific env vars (override defaults)
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Add standard kandev env vars
	env = append(env,
		fmt.Sprintf("KANDEV_TASK_ID=%s", req.TaskID),
		fmt.Sprintf("KANDEV_INSTANCE_ID=%s", instanceID),
	)

	// Pass protocol to agentctl inside the container
	if agentConfig.Protocol != "" {
		env = append(env, fmt.Sprintf("AGENTCTL_PROTOCOL=%s", agentConfig.Protocol))
	}

	// Configure Git to trust the workspace directory
	// This is needed because the container runs as root but files are owned by host user
	// Uses Git's environment-based config (GIT_CONFIG_COUNT, GIT_CONFIG_KEY_n, GIT_CONFIG_VALUE_n)
	env = append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*", // Trust all directories in the container
	)

	// Extract auggie_session_id from metadata for session resumption
	if req.Metadata != nil {
		if sessionID, ok := req.Metadata["auggie_session_id"].(string); ok && sessionID != "" {
			env = append(env, fmt.Sprintf("AUGGIE_SESSION_ID=%s", sessionID))
		}
	}

	// Inject required credentials from credentials manager
	if m.credsMgr != nil && len(agentConfig.RequiredEnv) > 0 {
		for _, key := range agentConfig.RequiredEnv {
			value, err := m.credsMgr.GetCredentialValue(context.Background(), key)
			if err != nil {
				m.logger.Warn("required credential not found",
					zap.String("key", key),
					zap.Error(err))
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Calculate resource limits
	memoryBytes := agentConfig.ResourceLimits.MemoryMB * 1024 * 1024
	cpuQuota := int64(agentConfig.ResourceLimits.CPUCores * 100000) // Docker CPU quota

	containerName := fmt.Sprintf("kandev-agent-%s", instanceID[:8])

	// Build command, appending model flag if agent supports it and profile has a model
	cmd := make([]string, len(agentConfig.Cmd))
	copy(cmd, agentConfig.Cmd)
	if agentConfig.ModelFlag != "" && profileInfo != nil && profileInfo.Model != "" {
		cmd = append(cmd, agentConfig.ModelFlag, profileInfo.Model)
	}

	return docker.ContainerConfig{
		Name:       containerName,
		Image:      imageName,
		Cmd:        cmd,
		Env:        env,
		WorkingDir: agentConfig.WorkingDir,
		Mounts:     mounts,
		Memory:     memoryBytes,
		CPUQuota:   cpuQuota,
		Labels: map[string]string{
			"kandev.managed":          "true",
			"kandev.instance_id":      instanceID,
			"kandev.task_id":          req.TaskID,
			"kandev.agent_profile_id": req.AgentProfileID,
		},
		AutoRemove: false, // We manage cleanup ourselves
	}
}

// expandMountTemplate expands template variables in mount source paths
func (m *Manager) expandMountTemplate(source, workspacePath, taskID string) string {
	result := source
	result = strings.ReplaceAll(result, "{workspace}", workspacePath)
	result = strings.ReplaceAll(result, "{task_id}", taskID)

	// Expand {augment_sessions} to the user's augment sessions directory
	// This allows session resumption across container runs
	if strings.Contains(result, "{augment_sessions}") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/tmp"
		}
		augmentSessionsDir := filepath.Join(homeDir, ".augment", "sessions")
		// Ensure the directory exists
		_ = os.MkdirAll(augmentSessionsDir, 0755)
		result = strings.ReplaceAll(result, "{augment_sessions}", augmentSessionsDir)
	}

	return result
}

// PromptAgent sends a follow-up prompt to a running agent
// PromptResult contains the result of a prompt operation
type PromptResult struct {
	StopReason   string // The reason the agent stopped (e.g., "end_turn")
	AgentMessage string // The agent's accumulated response message
}

// PromptAgent sends a follow-up prompt to a running agent
func (m *Manager) PromptAgent(ctx context.Context, instanceID string, prompt string) (*PromptResult, error) {
	m.mu.Lock()
	instance, exists := m.instances[instanceID]
	if !exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}

	if instance.agentctl == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("instance %q has no agentctl client", instanceID)
	}

	// Accept both RUNNING (initial) and READY (after first prompt) states
	if instance.Status != v1.AgentStatusRunning && instance.Status != v1.AgentStatusReady {
		m.mu.Unlock()
		return nil, fmt.Errorf("instance %q is not ready for prompts (status: %s)", instanceID, instance.Status)
	}

	// Set status to RUNNING while processing
	instance.Status = v1.AgentStatusRunning

	// Clear buffers before starting new prompt
	instance.messageMu.Lock()
	instance.messageBuffer.Reset()
	instance.reasoningBuffer.Reset()
	instance.summaryBuffer.Reset()
	instance.messageMu.Unlock()

	m.mu.Unlock()

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
	m.mu.RLock()
	instance, exists := m.instances[instanceID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	m.logger.Info("stopping agent",
		zap.String("instance_id", instanceID),
		zap.Bool("force", force),
		zap.String("runtime", m.agentCfg.Runtime))

	// Try to gracefully stop via agentctl first
	if instance.agentctl != nil && !force {
		if err := instance.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl",
				zap.String("instance_id", instanceID),
				zap.Error(err))
		}
		instance.agentctl.Close()
	}

	// Stop the agent instance based on runtime mode
	var err error
	if m.IsStandaloneMode() {
		err = m.stopStandaloneInstance(ctx, instance)
	} else {
		err = m.stopDockerContainer(ctx, instance, force)
	}

	if err != nil {
		return err
	}

	// Update instance status and remove from tracking maps
	m.mu.Lock()
	instance.Status = v1.AgentStatusStopped
	now := time.Now()
	instance.FinishedAt = &now
	delete(m.instances, instanceID)
	delete(m.byTask, instance.TaskID)
	if instance.ContainerID != "" {
		delete(m.byContainer, instance.ContainerID)
	}
	m.mu.Unlock()

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("instance_id", instanceID),
		zap.String("task_id", instance.TaskID))

	// Publish stopped event
	m.publishEvent(ctx, events.AgentStopped, instance)

	return nil
}

// stopDockerContainer stops a Docker container
func (m *Manager) stopDockerContainer(ctx context.Context, instance *AgentInstance, force bool) error {
	if instance.ContainerID == "" {
		return nil // No container to stop
	}

	var err error
	if force {
		err = m.docker.KillContainer(ctx, instance.ContainerID, "SIGKILL")
	} else {
		err = m.docker.StopContainer(ctx, instance.ContainerID, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

// stopStandaloneInstance stops a standalone agentctl instance
func (m *Manager) stopStandaloneInstance(ctx context.Context, instance *AgentInstance) error {
	if instance.standaloneInstanceID == "" {
		return nil // No standalone instance to stop
	}

	if err := m.standaloneCtl.DeleteInstance(ctx, instance.standaloneInstanceID); err != nil {
		return fmt.Errorf("failed to stop standalone instance: %w", err)
	}

	return nil
}

// StopByTaskID stops the agent for a specific task
func (m *Manager) StopByTaskID(ctx context.Context, taskID string, force bool) error {
	m.mu.RLock()
	instanceID, exists := m.byTask[taskID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no agent running for task %q", taskID)
	}

	return m.StopAgent(ctx, instanceID, force)
}

// GetInstance returns an agent instance by ID
func (m *Manager) GetInstance(instanceID string) (*AgentInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instance, exists := m.instances[instanceID]
	return instance, exists
}

// GetInstanceByTaskID returns the agent instance for a task
func (m *Manager) GetInstanceByTaskID(taskID string) (*AgentInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instanceID, exists := m.byTask[taskID]
	if !exists {
		return nil, false
	}

	instance, exists := m.instances[instanceID]
	return instance, exists
}

// GetInstanceByContainerID returns the agent instance for a container
func (m *Manager) GetInstanceByContainerID(containerID string) (*AgentInstance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instanceID, exists := m.byContainer[containerID]
	if !exists {
		return nil, false
	}

	instance, exists := m.instances[instanceID]
	return instance, exists
}

// ListInstances returns all active instances
func (m *Manager) ListInstances() []*AgentInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*AgentInstance, 0, len(m.instances))
	for _, instance := range m.instances {
		result = append(result, instance)
	}
	return result
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
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	instance.Status = status
	m.logger.Debug("updated instance status",
		zap.String("instance_id", instanceID),
		zap.String("status", string(status)))

	return nil
}

// UpdateProgress updates the progress of an instance
func (m *Manager) UpdateProgress(instanceID string, progress int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("instance %q not found", instanceID)
	}

	instance.Progress = progress
	m.logger.Debug("updated instance progress",
		zap.String("instance_id", instanceID),
		zap.Int("progress", progress))

	return nil
}

// MarkReady marks an instance as ready for follow-up prompts
func (m *Manager) MarkReady(instanceID string) error {
	m.mu.Lock()
	instance, exists := m.instances[instanceID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("instance %q not found", instanceID)
	}

	instance.Status = v1.AgentStatusReady
	m.mu.Unlock()

	m.logger.Info("instance ready for follow-up prompts",
		zap.String("instance_id", instanceID))

	// Publish ready event
	m.publishEvent(context.Background(), events.AgentReady, instance)

	return nil
}

// MarkCompleted marks an instance as completed
func (m *Manager) MarkCompleted(instanceID string, exitCode int, errorMessage string) error {
	m.mu.Lock()
	instance, exists := m.instances[instanceID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("instance %q not found", instanceID)
	}

	now := time.Now()
	instance.FinishedAt = &now
	instance.ExitCode = &exitCode
	instance.ErrorMessage = errorMessage

	if exitCode == 0 && errorMessage == "" {
		instance.Status = v1.AgentStatusCompleted
		instance.Progress = 100
	} else {
		instance.Status = v1.AgentStatusFailed
	}
	m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return
	}

	delete(m.instances, instanceID)
	delete(m.byTask, instance.TaskID)
	delete(m.byContainer, instance.ContainerID)

	m.logger.Debug("removed instance from tracking",
		zap.String("instance_id", instanceID))
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

	// Skip cleanup if docker client is not available (e.g., in tests)
	if m.docker == nil {
		m.logger.Debug("skipping cleanup - no docker client")
		return
	}

	// List all kandev-managed containers
	containers, err := m.docker.ListContainers(ctx, map[string]string{
		"kandev.managed": "true",
	})
	if err != nil {
		m.logger.Error("failed to list containers for cleanup", zap.Error(err))
		return
	}

	for _, container := range containers {
		// Check if container is exited and we're tracking it
		if container.State == "exited" {
			m.mu.RLock()
			instanceID, tracked := m.byContainer[container.ID]
			m.mu.RUnlock()

			if tracked {
				// Get container info to get exit code
				info, err := m.docker.GetContainerInfo(ctx, container.ID)
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
				_ = m.MarkCompleted(instanceID, info.ExitCode, errorMsg)

				// Remove the container
				if err := m.docker.RemoveContainer(ctx, container.ID, false); err != nil {
					m.logger.Warn("failed to remove container during cleanup",
						zap.String("container_id", container.ID),
						zap.Error(err))
				}

				// Remove the instance from tracking so new agents can be launched
				m.RemoveInstance(instanceID)
			}
		}
	}
}

// RespondToPermission sends a response to a permission request for an agent instance
func (m *Manager) RespondToPermission(instanceID, pendingID, optionID string, cancelled bool) error {
	m.mu.RLock()
	instance, exists := m.instances[instanceID]
	m.mu.RUnlock()

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
	// Find the instance by task ID
	m.mu.RLock()
	var instance *AgentInstance
	for _, inst := range m.instances {
		if inst.TaskID == taskID {
			instance = inst
			break
		}
	}
	m.mu.RUnlock()

	if instance == nil {
		return fmt.Errorf("no agent instance found for task: %s", taskID)
	}

	return m.RespondToPermission(instance.ID, pendingID, optionID, cancelled)
}
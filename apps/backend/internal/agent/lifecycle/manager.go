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

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agentctl"
	"github.com/kandev/kandev/internal/agent/docker"
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
	ID           string
	TaskID       string
	AgentType    string
	ContainerID  string
	ContainerIP  string // IP address of the container for agentctl communication
	Status       v1.AgentStatus
	StartedAt    time.Time
	FinishedAt   *time.Time
	ExitCode     *int
	ErrorMessage string
	Progress     int
	Metadata     map[string]interface{}

	// agentctl client for this instance
	agentctl *agentctl.Client

	// Message buffer for accumulating agent response during a prompt
	messageBuffer strings.Builder
	messageMu     sync.Mutex
}

// LaunchRequest contains parameters for launching an agent
type LaunchRequest struct {
	TaskID          string
	AgentType       string
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

// Manager manages agent instance lifecycles
type Manager struct {
	docker      *docker.Client
	registry    *registry.Registry
	eventBus    bus.EventBus
	credsMgr    CredentialsManager
	worktreeMgr *worktree.Manager
	logger      *logger.Logger

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
	log *logger.Logger,
) *Manager {
	return &Manager{
		docker:          dockerClient,
		registry:        reg,
		eventBus:        eventBus,
		logger:          log.WithFields(zap.String("component", "lifecycle-manager")),
		instances:       make(map[string]*AgentInstance),
		byTask:          make(map[string]string),
		byContainer:     make(map[string]string),
		cleanupInterval: 30 * time.Second,
		stopCh:          make(chan struct{}),
	}
}

// SetCredentialsManager sets the credentials manager for injecting secrets
func (m *Manager) SetCredentialsManager(credsMgr CredentialsManager) {
	m.credsMgr = credsMgr
}

// SetWorktreeManager sets the worktree manager for Git worktree isolation
func (m *Manager) SetWorktreeManager(worktreeMgr *worktree.Manager) {
	m.worktreeMgr = worktreeMgr
}

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting lifecycle manager")

	// Reconcile with Docker to find running containers from previous runs
	if _, err := m.reconcileWithDocker(ctx); err != nil {
		m.logger.Warn("failed to reconcile with Docker", zap.Error(err))
		// Continue anyway - this is best-effort
	}

	// Start cleanup loop
	m.wg.Add(1)
	go m.cleanupLoop(ctx)

	return nil
}

// RecoveredInstance contains info about an instance recovered from Docker
type RecoveredInstance struct {
	InstanceID  string
	TaskID      string
	ContainerID string
	AgentType   string
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
		agentType := labels["kandev.agent_type"]

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
			ID:          instanceID,
			TaskID:      taskID,
			AgentType:   agentType,
			ContainerID: ctr.ID,
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
			InstanceID:  instanceID,
			TaskID:      taskID,
			ContainerID: ctr.ID,
			AgentType:   agentType,
		})

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
			InstanceID:  inst.ID,
			TaskID:      inst.TaskID,
			ContainerID: inst.ContainerID,
			AgentType:   inst.AgentType,
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
		zap.String("agent_type", req.AgentType),
		zap.Bool("use_worktree", req.UseWorktree))

	// 1. Validate the agent type exists in registry
	agentConfig, err := m.registry.Get(req.AgentType)
	if err != nil {
		return nil, fmt.Errorf("agent type not found: %w", err)
	}

	if !agentConfig.Enabled {
		return nil, fmt.Errorf("agent type %q is disabled", req.AgentType)
	}

	// 2. Check if task already has an agent running
	m.mu.RLock()
	if existingID, exists := m.byTask[req.TaskID]; exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("task %q already has an agent running (instance: %s)", req.TaskID, existingID)
	}
	m.mu.RUnlock()

	// 3. Handle worktree creation/reuse if enabled
	workspacePath := req.WorkspacePath
	var mainRepoGitDir string // Path to main repo's .git directory for container mount
	if req.UseWorktree && m.worktreeMgr != nil && req.RepositoryPath != "" {
		wt, err := m.getOrCreateWorktree(ctx, req)
		if err != nil {
			m.logger.Warn("failed to create worktree, falling back to direct mount",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
			// Fall back to direct mount if worktree creation fails
		} else {
			workspacePath = wt.Path
			// Git worktrees reference the main repo's .git directory via a .git file
			// The worktree metadata in .git/worktrees/{name} contains a 'commondir' file
			// that points back to the main .git directory (usually '../..')
			// We need to mount the entire .git directory for git commands to work
			mainRepoGitDir = filepath.Join(req.RepositoryPath, ".git")
			m.logger.Info("using worktree for agent",
				zap.String("task_id", req.TaskID),
				zap.String("worktree_path", workspacePath),
				zap.String("main_repo_git_dir", mainRepoGitDir),
				zap.String("branch", wt.Branch))
		}
	}

	// 4. Generate a new instance ID
	instanceID := uuid.New().String()

	// 5. Build container config from registry config (use worktree path if available)
	reqWithWorktree := *req
	reqWithWorktree.WorkspacePath = workspacePath
	containerConfig := m.buildContainerConfig(instanceID, &reqWithWorktree, agentConfig, mainRepoGitDir)

	// 6. Create and start the container
	containerID, err := m.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		_ = m.docker.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// 7. Get container IP for agentctl communication
	containerIP, err := m.docker.GetContainerIP(ctx, containerID)
	if err != nil {
		m.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID),
			zap.Error(err))
		containerIP = "127.0.0.1"
	}

	// 8. Create agentctl client
	agentctlClient := agentctl.NewClient(containerIP, AgentCtlPort, m.logger)

	// 9. Track the instance
	now := time.Now()
	instance := &AgentInstance{
		ID:          instanceID,
		TaskID:      req.TaskID,
		AgentType:   req.AgentType,
		ContainerID: containerID,
		ContainerIP: containerIP,
		Status:      v1.AgentStatusRunning,
		StartedAt:   now,
		Progress:    0,
		Metadata:    req.Metadata,
		agentctl:    agentctlClient,
	}

	m.mu.Lock()
	m.instances[instanceID] = instance
	m.byTask[req.TaskID] = instanceID
	m.byContainer[containerID] = instanceID
	m.mu.Unlock()

	// 10. Publish agent.started event
	m.publishEvent(ctx, events.AgentStarted, instance)

	// 11. Wait for agentctl to be ready and start the agent process
	go m.initializeAgent(instance, req.TaskDescription)

	m.logger.Info("agent launched successfully",
		zap.String("instance_id", instanceID),
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP),
		zap.String("task_id", req.TaskID))

	return instance, nil
}

// getOrCreateWorktree creates a new worktree or reuses an existing one for the task.
// This enables task resumption - the same worktree is used across multiple agent runs.
func (m *Manager) getOrCreateWorktree(ctx context.Context, req *LaunchRequest) (*worktree.Worktree, error) {
	// First, check if a worktree already exists for this task (session resumption)
	existing, err := m.worktreeMgr.GetByTaskID(ctx, req.TaskID)
	if err != nil && err != worktree.ErrWorktreeNotFound {
		return nil, fmt.Errorf("failed to check existing worktree: %w", err)
	}

	if existing != nil {
		// Validate the existing worktree is still valid
		if m.worktreeMgr.IsValid(existing.Path) {
			m.logger.Info("reusing existing worktree for task resumption",
				zap.String("task_id", req.TaskID),
				zap.String("worktree_path", existing.Path),
				zap.String("branch", existing.Branch))
			return existing, nil
		}
		// Worktree exists in DB but is invalid - will be recreated by Create()
		m.logger.Warn("existing worktree is invalid, will recreate",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_path", existing.Path))
	}

	// Create a new worktree
	createReq := worktree.CreateRequest{
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: req.RepositoryPath,
		BaseBranch:     req.BaseBranch,
	}

	wt, err := m.worktreeMgr.Create(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	m.logger.Info("created new worktree for task",
		zap.String("task_id", req.TaskID),
		zap.String("worktree_path", wt.Path),
		zap.String("branch", wt.Branch))

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

	initResp, err := instance.agentctl.Initialize(ctx, "kandev", "1.0.0")
	if err != nil {
		m.logger.Error("ACP initialize failed",
			zap.String("instance_id", instance.ID),
			zap.Error(err))
		return fmt.Errorf("initialize failed: %w", err)
	}
	m.logger.Info("ACP initialize response received",
		zap.String("instance_id", instance.ID),
		zap.String("agent_name", initResp.AgentInfo.Name),
		zap.String("agent_version", initResp.AgentInfo.Version))

	// 2. Create new session
	m.logger.Info("sending ACP session/new request",
		zap.String("instance_id", instance.ID))

	sessionID, err := instance.agentctl.NewSession(ctx, "/workspace")
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
	go m.handleUpdatesStream(instance)

	// 4. Set up permission stream to receive permission requests
	go m.handlePermissionStream(instance)

	// 5. Send the task prompt if provided
	if taskDescription != "" {
		m.logger.Info("sending ACP prompt",
			zap.String("instance_id", instance.ID),
			zap.String("session_id", sessionID),
			zap.String("task_description", taskDescription))

		// Clear message buffer before starting prompt
		instance.messageMu.Lock()
		instance.messageBuffer.Reset()
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

		// Extract accumulated message from buffer
		instance.messageMu.Lock()
		agentMessage := instance.messageBuffer.String()
		instance.messageBuffer.Reset()
		instance.messageMu.Unlock()

		stopReason := ""
		if resp != nil {
			stopReason = string(resp.StopReason)
		}
		m.logger.Info("ACP prompt completed",
			zap.String("instance_id", instance.ID),
			zap.String("stop_reason", stopReason))

		// Publish prompt_complete event with the agent's response (for saving as comment)
		m.publishPromptComplete(instance, agentMessage)

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

// handleUpdatesStream handles streaming session notifications from the agent
func (m *Manager) handleUpdatesStream(instance *AgentInstance) {
	ctx := context.Background()

	err := instance.agentctl.StreamUpdates(ctx, func(notification agentctl.SessionNotification) {
		m.handleSessionNotification(instance, notification)
	})
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

// handleSessionNotification processes incoming session notifications from the agent
func (m *Manager) handleSessionNotification(instance *AgentInstance, notification agentctl.SessionNotification) {
	m.logger.Debug("received session notification",
		zap.String("instance_id", instance.ID),
		zap.String("session_id", string(notification.SessionId)))

	update := notification.Update

	// Handle different update types
	if update.AgentMessageChunk != nil {
		m.logger.Debug("agent message chunk",
			zap.String("instance_id", instance.ID))
		m.updateInstanceProgress(instance.ID, 50)

		// Accumulate message content for saving as comment when a step completes
		if update.AgentMessageChunk.Content.Text != nil {
			instance.messageMu.Lock()
			instance.messageBuffer.WriteString(update.AgentMessageChunk.Content.Text.Text)
			instance.messageMu.Unlock()
		}
	}

	if update.AgentThoughtChunk != nil {
		m.logger.Debug("agent thought chunk",
			zap.String("instance_id", instance.ID))
	}

	if update.ToolCall != nil {
		// Tool call starting marks a step boundary - flush the accumulated message as a comment
		// This way, each agent response before a tool use becomes a separate comment
		m.flushMessageBufferAsComment(instance)

		toolCallID := string(update.ToolCall.ToolCallId)
		toolTitle := update.ToolCall.Title
		toolStatus := string(update.ToolCall.Status)
		toolKind := string(update.ToolCall.Kind)

		m.logger.Info("tool call started",
			zap.String("instance_id", instance.ID),
			zap.String("tool_call_id", toolCallID),
			zap.String("title", toolTitle),
			zap.String("kind", toolKind))
		m.updateInstanceProgress(instance.ID, 60)

		// Extract args from the tool call for display
		args := map[string]interface{}{}

		// Add tool kind
		if toolKind != "" {
			args["kind"] = toolKind
		}

		// Add locations (file paths)
		if len(update.ToolCall.Locations) > 0 {
			locations := make([]map[string]interface{}, len(update.ToolCall.Locations))
			for i, loc := range update.ToolCall.Locations {
				locMap := map[string]interface{}{"path": loc.Path}
				if loc.Line != nil {
					locMap["line"] = *loc.Line
				}
				locations[i] = locMap
			}
			args["locations"] = locations

			// Also set primary path for convenience
			args["path"] = update.ToolCall.Locations[0].Path
		}

		// Add raw input if available
		if update.ToolCall.RawInput != nil {
			args["raw_input"] = update.ToolCall.RawInput
		}

		// Publish tool call as a comment so it appears in the chat
		m.publishToolCall(instance, toolCallID, toolTitle, toolStatus, args)
	}

	if update.ToolCallUpdate != nil {
		toolCallID := string(update.ToolCallUpdate.ToolCallId)

		m.logger.Debug("tool call update",
			zap.String("instance_id", instance.ID),
			zap.String("tool_call_id", toolCallID))

		// Check if tool call completed
		if update.ToolCallUpdate.Status != nil {
			status := string(*update.ToolCallUpdate.Status)
			switch status {
			case "complete":
				m.logger.Info("tool call completed",
					zap.String("instance_id", instance.ID))
				m.updateInstanceProgress(instance.ID, 80)
				// Publish tool call completion
				m.publishToolCallComplete(instance, toolCallID, update.ToolCallUpdate)
			case "error":
				m.logger.Error("tool call error",
					zap.String("instance_id", instance.ID))
				m.publishToolCallComplete(instance, toolCallID, update.ToolCallUpdate)
			}
		}
	}

	if update.Plan != nil {
		m.logger.Info("agent plan update",
			zap.String("instance_id", instance.ID),
			zap.Int("num_entries", len(update.Plan.Entries)))
	}

	// Publish session notification to event bus for WebSocket streaming
	m.publishSessionNotification(instance, notification)
}

// publishSessionNotification publishes a session notification to the event bus
func (m *Manager) publishSessionNotification(instance *AgentInstance, notification agentctl.SessionNotification) {
	if m.eventBus == nil {
		return
	}

	// Build ACP message data from the session notification
	data := map[string]interface{}{
		"type":        "session/update",
		"timestamp":   time.Now(),
		"agent_id":    instance.ID,
		"task_id":     instance.TaskID,
		"session_id":  string(notification.SessionId),
		"data":        notification.Update,
	}

	event := bus.NewEvent(events.ACPMessage, "agent-manager", data)
	subject := events.BuildACPSubject(instance.TaskID)

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish session notification",
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

// publishPromptComplete publishes a prompt_complete event when an agent finishes responding
// This is used to save the agent's response as a comment on the task
func (m *Manager) publishPromptComplete(instance *AgentInstance, agentMessage string) {
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

	event := bus.NewEvent(events.PromptComplete, "agent-manager", data)
	// Publish on task-specific subject so orchestrator can subscribe
	subject := events.PromptComplete + "." + instance.TaskID

	if err := m.eventBus.Publish(context.Background(), subject, event); err != nil {
		m.logger.Error("failed to publish prompt_complete event",
			zap.String("instance_id", instance.ID),
			zap.String("task_id", instance.TaskID),
			zap.Error(err))
	} else {
		m.logger.Info("published prompt_complete event",
			zap.String("task_id", instance.TaskID),
			zap.Int("message_length", len(agentMessage)))
	}
}

// flushMessageBufferAsComment extracts any accumulated message from the buffer and
// publishes it as a step complete event (which will be saved as a comment).
// This is called when a tool use starts to save the agent's response before the tool call.
func (m *Manager) flushMessageBufferAsComment(instance *AgentInstance) {
	instance.messageMu.Lock()
	agentMessage := instance.messageBuffer.String()
	instance.messageBuffer.Reset()
	instance.messageMu.Unlock()

	// Only publish if there's actual content (ignore whitespace-only)
	trimmed := strings.TrimSpace(agentMessage)
	if trimmed == "" {
		return
	}

	m.logger.Debug("flushing message buffer as step comment",
		zap.String("instance_id", instance.ID),
		zap.String("task_id", instance.TaskID),
		zap.Int("message_length", len(agentMessage)))

	// Reuse the prompt_complete event type - the orchestrator handles it the same way
	m.publishPromptComplete(instance, agentMessage)
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

// publishToolCallComplete publishes a tool call completion event
func (m *Manager) publishToolCallComplete(instance *AgentInstance, toolCallID string, update *acp.SessionToolCallUpdate) {
	if m.eventBus == nil {
		return
	}

	status := ""
	if update.Status != nil {
		status = string(*update.Status)
	}

	data := map[string]interface{}{
		"type":         "tool_call_complete",
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":     instance.ID,
		"task_id":      instance.TaskID,
		"tool_call_id": toolCallID,
		"status":       status,
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
func (m *Manager) buildContainerConfig(instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig, mainRepoGitDir string) docker.ContainerConfig {
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

	return docker.ContainerConfig{
		Name:       containerName,
		Image:      imageName,
		Cmd:        agentConfig.Cmd,
		Env:        env,
		WorkingDir: agentConfig.WorkingDir,
		Mounts:     mounts,
		Memory:     memoryBytes,
		CPUQuota:   cpuQuota,
		Labels: map[string]string{
			"kandev.managed":     "true",
			"kandev.instance_id": instanceID,
			"kandev.task_id":     req.TaskID,
			"kandev.agent_type":  req.AgentType,
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

	// Clear message buffer before starting new prompt
	instance.messageMu.Lock()
	instance.messageBuffer.Reset()
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

	// Extract accumulated message from buffer
	instance.messageMu.Lock()
	agentMessage := instance.messageBuffer.String()
	instance.messageBuffer.Reset()
	instance.messageMu.Unlock()

	result := &PromptResult{
		StopReason:   string(resp.StopReason),
		AgentMessage: agentMessage,
	}

	// Publish prompt_complete event with the agent's response (for saving as comment)
	m.publishPromptComplete(instance, agentMessage)

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
		zap.Bool("force", force))

	// Try to gracefully stop via agentctl first
	if instance.agentctl != nil && !force {
		if err := instance.agentctl.Stop(ctx); err != nil {
			m.logger.Warn("failed to stop agent via agentctl, will stop container",
				zap.String("instance_id", instanceID),
				zap.Error(err))
		}
		instance.agentctl.Close()
	}

	// Stop the container
	var err error
	if force {
		err = m.docker.KillContainer(ctx, instance.ContainerID, "SIGKILL")
	} else {
		err = m.docker.StopContainer(ctx, instance.ContainerID, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Update instance status and remove from tracking maps
	m.mu.Lock()
	instance.Status = v1.AgentStatusStopped
	now := time.Now()
	instance.FinishedAt = &now
	delete(m.instances, instanceID)
	delete(m.byTask, instance.TaskID)
	delete(m.byContainer, instance.ContainerID)
	m.mu.Unlock()

	m.logger.Info("agent stopped and removed from tracking",
		zap.String("instance_id", instanceID),
		zap.String("task_id", instance.TaskID))

	// Publish stopped event
	m.publishEvent(ctx, events.AgentStopped, instance)

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
		"instance_id":   instance.ID,
		"task_id":       instance.TaskID,
		"agent_type":    instance.AgentType,
		"container_id":  instance.ContainerID,
		"status":        string(instance.Status),
		"started_at":    instance.StartedAt,
		"progress":      instance.Progress,
		"error_message": instance.ErrorMessage,
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

// performCleanup checks for and cleans up stale containers
func (m *Manager) performCleanup(ctx context.Context) {
	m.logger.Debug("running cleanup check")

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
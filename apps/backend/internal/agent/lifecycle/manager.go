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
}

// LaunchRequest contains parameters for launching an agent
type LaunchRequest struct {
	TaskID          string
	AgentType       string
	WorkspacePath   string                 // Host path to workspace
	TaskDescription string                 // Task description to send via ACP prompt
	Env             map[string]string      // Additional env vars
	Metadata        map[string]interface{}
}

// CredentialsManager interface for credential retrieval
type CredentialsManager interface {
	GetCredentialValue(ctx context.Context, key string) (value string, err error)
}

// Manager manages agent instance lifecycles
type Manager struct {
	docker   *docker.Client
	registry *registry.Registry
	eventBus bus.EventBus
	credsMgr CredentialsManager
	logger   *logger.Logger

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

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting lifecycle manager")

	// Start cleanup loop
	m.wg.Add(1)
	go m.cleanupLoop(ctx)

	return nil
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
		zap.String("agent_type", req.AgentType))

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

	// 3. Generate a new instance ID
	instanceID := uuid.New().String()

	// 4. Build container config from registry config
	containerConfig := m.buildContainerConfig(instanceID, req, agentConfig)

	// 5. Create and start the container
	containerID, err := m.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		_ = m.docker.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// 6. Get container IP for agentctl communication
	containerIP, err := m.docker.GetContainerIP(ctx, containerID)
	if err != nil {
		m.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID),
			zap.Error(err))
		containerIP = "127.0.0.1"
	}

	// 7. Create agentctl client
	agentctlClient := agentctl.NewClient(containerIP, AgentCtlPort, m.logger)

	// 8. Track the instance
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

	// 9. Publish agent.started event
	m.publishEvent(ctx, events.AgentStarted, instance)

	// 10. Wait for agentctl to be ready and start the agent process
	go m.initializeAgent(instance, req.TaskDescription)

	m.logger.Info("agent launched successfully",
		zap.String("instance_id", instanceID),
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP),
		zap.String("task_id", req.TaskID))

	return instance, nil
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

	// 3. Set up updates stream to receive session notifications
	go m.handleUpdatesStream(instance)

	// 4. Send the task prompt if provided
	if taskDescription != "" {
		m.logger.Info("sending ACP prompt",
			zap.String("instance_id", instance.ID),
			zap.String("session_id", sessionID),
			zap.String("task_description", taskDescription))

		// Prompt is SYNCHRONOUS - it blocks until the agent completes the task
		// Use a long timeout context for this
		promptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		err := instance.agentctl.Prompt(promptCtx, taskDescription)
		if err != nil {
			m.logger.Error("ACP prompt failed",
				zap.String("instance_id", instance.ID),
				zap.Error(err))
			return fmt.Errorf("prompt failed: %w", err)
		}

		// Prompt completed - mark agent as READY for follow-up prompts
		m.logger.Info("ACP prompt completed, agent ready for follow-up prompts",
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
	}

	if update.AgentThoughtChunk != nil {
		m.logger.Debug("agent thought chunk",
			zap.String("instance_id", instance.ID))
	}

	if update.ToolCall != nil {
		m.logger.Info("tool call started",
			zap.String("instance_id", instance.ID),
			zap.String("tool_call_id", string(update.ToolCall.ToolCallId)))
		m.updateInstanceProgress(instance.ID, 60)
	}

	if update.ToolCallUpdate != nil {
		m.logger.Debug("tool call update",
			zap.String("instance_id", instance.ID),
			zap.String("tool_call_id", string(update.ToolCallUpdate.ToolCallId)))

		// Check if tool call completed
		if update.ToolCallUpdate.Status != nil {
			switch *update.ToolCallUpdate.Status {
			case "complete":
				m.logger.Info("tool call completed",
					zap.String("instance_id", instance.ID))
				m.updateInstanceProgress(instance.ID, 80)
			case "error":
				m.logger.Error("tool call error",
					zap.String("instance_id", instance.ID))
			}
		}
	}

	if update.Plan != nil {
		m.logger.Info("agent plan update",
			zap.String("instance_id", instance.ID),
			zap.Int("num_entries", len(update.Plan.Entries)))
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
func (m *Manager) buildContainerConfig(instanceID string, req *LaunchRequest, agentConfig *registry.AgentTypeConfig) docker.ContainerConfig {
	// Build image name with tag
	imageName := agentConfig.Image
	if agentConfig.Tag != "" {
		imageName = fmt.Sprintf("%s:%s", agentConfig.Image, agentConfig.Tag)
	}

	// Expand mount templates
	mounts := make([]docker.MountConfig, 0, len(agentConfig.Mounts))
	for _, mt := range agentConfig.Mounts {
		source := m.expandMountTemplate(mt.Source, req.WorkspacePath, req.TaskID)
		mounts = append(mounts, docker.MountConfig{
			Source:   source,
			Target:   mt.Target,
			ReadOnly: mt.ReadOnly,
		})
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
func (m *Manager) PromptAgent(ctx context.Context, instanceID string, prompt string) error {
	m.mu.Lock()
	instance, exists := m.instances[instanceID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("instance %q not found", instanceID)
	}

	if instance.agentctl == nil {
		m.mu.Unlock()
		return fmt.Errorf("instance %q has no agentctl client", instanceID)
	}

	// Accept both RUNNING (initial) and READY (after first prompt) states
	if instance.Status != v1.AgentStatusRunning && instance.Status != v1.AgentStatusReady {
		m.mu.Unlock()
		return fmt.Errorf("instance %q is not ready for prompts (status: %s)", instanceID, instance.Status)
	}

	// Set status to RUNNING while processing
	instance.Status = v1.AgentStatusRunning
	m.mu.Unlock()

	m.logger.Info("sending prompt to agent",
		zap.String("instance_id", instanceID),
		zap.Int("prompt_length", len(prompt)))

	// Prompt is synchronous - blocks until agent completes
	if err := instance.agentctl.Prompt(ctx, prompt); err != nil {
		return err
	}

	// Prompt completed - mark as READY for next prompt
	if err := m.MarkReady(instanceID); err != nil {
		m.logger.Error("failed to mark instance as ready after prompt",
			zap.String("instance_id", instanceID),
			zap.Error(err))
	}

	return nil
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

	// Update instance status
	m.mu.Lock()
	instance.Status = v1.AgentStatusStopped
	now := time.Now()
	instance.FinishedAt = &now
	m.mu.Unlock()

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
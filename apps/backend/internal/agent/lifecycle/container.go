// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

// ContainerConfig holds configuration for launching a Docker container
type ContainerConfig struct {
	AgentConfig     agents.Agent
	WorkspacePath   string
	TaskID          string
	TaskDescription string
	Model           string
	SessionID       string
	Credentials     map[string]string
	ProfileInfo     *AgentProfileInfo
	InstanceID      string
	MainRepoGitDir  string // Path to main repo's .git directory (for worktrees)
	McpServers      []McpServerConfig
}

// ContainerManager handles Docker container lifecycle operations
type ContainerManager struct {
	dockerClient   *docker.Client
	commandBuilder *CommandBuilder
	logger         *logger.Logger
	networkName    string
}

// NewContainerManager creates a new ContainerManager
func NewContainerManager(dockerClient *docker.Client, networkName string, log *logger.Logger) *ContainerManager {
	return &ContainerManager{
		dockerClient:   dockerClient,
		commandBuilder: NewCommandBuilder(),
		logger:         log.WithFields(zap.String("component", "container-manager")),
		networkName:    networkName,
	}
}

// LaunchContainer creates and starts a Docker container for an agent.
// Returns the container ID and agentctl client pointing to the instance.
func (cm *ContainerManager) LaunchContainer(ctx context.Context, config ContainerConfig) (string, *agentctl.Client, error) {
	// Build container config
	containerCfg, err := cm.buildContainerConfig(config)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build container config: %w", err)
	}

	// Create the container
	containerID, err := cm.dockerClient.CreateContainer(ctx, containerCfg)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container
	if err := cm.dockerClient.StartContainer(ctx, containerID); err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return "", nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get container IP for agentctl communication
	containerIP, err := cm.dockerClient.GetContainerIP(ctx, containerID)
	if err != nil {
		cm.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID),
			zap.Error(err))
		containerIP = "127.0.0.1"
	}

	// Create ControlClient to communicate with the container's control server
	ctl := agentctl.NewControlClient(containerIP, AgentCtlPort, cm.logger)

	// Wait for agentctl to be healthy
	if err := cm.waitForHealth(ctx, ctl); err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return "", nil, fmt.Errorf("agentctl health check failed: %w", err)
	}

	// Convert MCP server configs
	var mcpServers []agentctl.McpServerConfig
	for _, mcp := range config.McpServers {
		mcpServers = append(mcpServers, agentctl.McpServerConfig{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Type:    mcp.Type,
			Command: mcp.Command,
			Args:    mcp.Args,
		})
	}

	// Create an instance via the control API (same flow as standalone mode)
	agentType := ""
	if config.AgentConfig != nil {
		agentType = config.AgentConfig.ID()
	}
	createReq := &agentctl.CreateInstanceRequest{
		ID:            config.InstanceID,
		WorkspacePath: "/workspace",
		AgentCommand:  "", // Agent command set via Configure endpoint later
		AgentType:     agentType,
		Env:           config.Credentials,
		AutoStart:     false,
		McpServers:    mcpServers,
		SessionID:     config.SessionID,
	}

	resp, err := ctl.CreateInstance(ctx, createReq)
	if err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return "", nil, fmt.Errorf("failed to create instance in container: %w", err)
	}

	// Create agentctl client pointing to the instance port
	agentctlClient := agentctl.NewClient(containerIP, resp.Port, cm.logger,
		agentctl.WithExecutionID(config.InstanceID),
		agentctl.WithSessionID(config.SessionID))

	cm.logger.Info("docker container launched",
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP),
		zap.String("instance_id", config.InstanceID),
		zap.Int("instance_port", resp.Port))

	return containerID, agentctlClient, nil
}

// waitForHealth waits for agentctl to be healthy with retries
func (cm *ContainerManager) waitForHealth(ctx context.Context, ctl *agentctl.ControlClient) error {
	const maxRetries = 30
	const retryDelay = 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		if err := ctl.Health(ctx); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("agentctl not healthy after %d retries", maxRetries)
}

// StopContainer stops and removes a Docker container
func (cm *ContainerManager) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	if containerID == "" {
		return nil
	}

	if err := cm.dockerClient.StopContainer(ctx, containerID, timeout); err != nil {
		cm.logger.Warn("failed to stop container gracefully, forcing removal",
			zap.String("container_id", containerID),
			zap.Error(err))
	}

	if err := cm.dockerClient.RemoveContainer(ctx, containerID, true); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	cm.logger.Info("container stopped and removed",
		zap.String("container_id", containerID))

	return nil
}

// buildContainerConfig builds the Docker container configuration
func (cm *ContainerManager) buildContainerConfig(config ContainerConfig) (docker.ContainerConfig, error) {
	ag := config.AgentConfig
	rt := ag.Runtime()

	// Build image name with tag
	imageName := rt.Image
	if rt.Tag != "" {
		imageName = fmt.Sprintf("%s:%s", rt.Image, rt.Tag)
	}

	// Build command using Agent's BuildCommand
	cmdOpts := agents.CommandOptions{
		Model:            config.Model,
		SessionID:        config.SessionID,
		PermissionValues: make(map[string]bool),
	}
	// Get profile settings if available
	if config.ProfileInfo != nil {
		cmdOpts.AutoApprove = config.ProfileInfo.AutoApprove
		cmdOpts.PermissionValues["auto_approve"] = config.ProfileInfo.AutoApprove
		cmdOpts.PermissionValues["allow_indexing"] = config.ProfileInfo.AllowIndexing
		cmdOpts.PermissionValues["dangerously_skip_permissions"] = config.ProfileInfo.DangerouslySkipPermissions
	}
	cmd := ag.BuildCommand(cmdOpts)

	// Expand mounts
	mounts := cm.expandMounts(rt.Mounts, config.WorkspacePath, ag)

	// Add main repo .git directory mount for worktrees
	if config.MainRepoGitDir != "" {
		mounts = append(mounts, docker.MountConfig{
			Source:   config.MainRepoGitDir,
			Target:   config.MainRepoGitDir, // Same path inside container
			ReadOnly: false,
		})
		cm.logger.Debug("added main repo .git directory mount for worktree",
			zap.String("path", config.MainRepoGitDir))
	}

	// Build environment variables
	env := cm.buildEnvVars(config)

	// Calculate resource limits
	memoryBytes := rt.ResourceLimits.MemoryMB * 1024 * 1024
	cpuQuota := int64(rt.ResourceLimits.CPUCores * 100000) // Docker CPU quota

	containerName := fmt.Sprintf("kandev-agent-%s", config.InstanceID[:8])

	return docker.ContainerConfig{
		Name:        containerName,
		Image:       imageName,
		Cmd:         cmd.Args(),
		Env:         env,
		WorkingDir:  rt.WorkingDir,
		Mounts:      mounts,
		NetworkMode: cm.networkName,
		Memory:      memoryBytes,
		CPUQuota:    cpuQuota,
		Labels: map[string]string{
			"kandev.managed":     "true",
			"kandev.instance_id": config.InstanceID,
			"kandev.task_id":     config.TaskID,
			"kandev.session_id":  config.SessionID,
		},
		AutoRemove: false, // We manage cleanup ourselves
	}, nil
}

// expandMounts expands mount templates with actual paths
func (cm *ContainerManager) expandMounts(templates []agents.MountTemplate, workspacePath string, ag agents.Agent) []docker.MountConfig {
	mounts := make([]docker.MountConfig, 0, len(templates)+1) // +1 for potential session dir

	for _, mt := range templates {
		// Skip workspace mounts if no workspace path is provided
		if strings.Contains(mt.Source, "{workspace}") && workspacePath == "" {
			cm.logger.Debug("skipping workspace mount - no workspace path provided",
				zap.String("target", mt.Target))
			continue
		}

		source := cm.expandMountSource(mt.Source, workspacePath)
		mounts = append(mounts, docker.MountConfig{
			Source:   source,
			Target:   mt.Target,
			ReadOnly: mt.ReadOnly,
		})
	}

	// Add session directory mount from SessionConfig
	sessionDirSource := cm.commandBuilder.ExpandSessionDir(ag)
	sessionDirTarget := cm.commandBuilder.GetSessionDirTarget(ag)
	if sessionDirSource != "" && sessionDirTarget != "" {
		mounts = append(mounts, docker.MountConfig{
			Source:   sessionDirSource,
			Target:   sessionDirTarget,
			ReadOnly: false,
		})
		cm.logger.Debug("added session directory mount",
			zap.String("source", sessionDirSource),
			zap.String("target", sessionDirTarget))
	}

	return mounts
}

// expandMountSource expands template variables in mount source paths
func (cm *ContainerManager) expandMountSource(source, workspacePath string) string {
	result := source
	result = strings.ReplaceAll(result, "{workspace}", workspacePath)

	// Expand {home} to user's home directory
	if strings.Contains(result, "{home}") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/tmp"
		}
		result = strings.ReplaceAll(result, "{home}", homeDir)
	}

	return result
}

// buildEnvVars builds environment variables for the container
func (cm *ContainerManager) buildEnvVars(config ContainerConfig) []string {
	ag := config.AgentConfig
	rt := ag.Runtime()
	env := make([]string, 0)

	// Add default env from agent config
	for k, v := range rt.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add standard kandev env vars
	env = append(env,
		fmt.Sprintf("KANDEV_TASK_ID=%s", config.TaskID),
		fmt.Sprintf("KANDEV_INSTANCE_ID=%s", config.InstanceID),
	)

	// Pass protocol to agentctl inside the container
	if rt.Protocol != "" {
		env = append(env, fmt.Sprintf("AGENTCTL_PROTOCOL=%s", rt.Protocol))
	}

	// Configure Git to trust the workspace directory
	env = append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
	)

	// Inject credentials from the provided credentials map
	for k, v := range config.Credentials {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add profile-specific label if available
	if config.ProfileInfo != nil && config.ProfileInfo.ProfileID != "" {
		env = append(env, fmt.Sprintf("KANDEV_AGENT_PROFILE_ID=%s", config.ProfileInfo.ProfileID))
	}

	return env
}

// ListManagedContainers returns all containers managed by kandev
func (cm *ContainerManager) ListManagedContainers(ctx context.Context) ([]docker.ContainerInfo, error) {
	return cm.dockerClient.ListContainers(ctx, map[string]string{
		"kandev.managed": "true",
	})
}

// GetContainerInfo returns information about a specific container
func (cm *ContainerManager) GetContainerInfo(ctx context.Context, containerID string) (*docker.ContainerInfo, error) {
	return cm.dockerClient.GetContainerInfo(ctx, containerID)
}

// RemoveContainer removes a container
func (cm *ContainerManager) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	return cm.dockerClient.RemoveContainer(ctx, containerID, force)
}

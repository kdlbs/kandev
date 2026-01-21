// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

// ContainerConfig holds configuration for launching a Docker container
type ContainerConfig struct {
	AgentConfig     *registry.AgentTypeConfig
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
	createReq := &agentctl.CreateInstanceRequest{
		ID:            config.InstanceID,
		WorkspacePath: "/workspace",
		AgentCommand:  "", // Agent command set via Configure endpoint later
		Env:           config.Credentials,
		AutoStart:     false,
		McpServers:    mcpServers,
	}

	resp, err := ctl.CreateInstance(ctx, createReq)
	if err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return "", nil, fmt.Errorf("failed to create instance in container: %w", err)
	}

	// Create agentctl client pointing to the instance port
	agentctlClient := agentctl.NewClient(containerIP, resp.Port, cm.logger)

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
	agentConfig := config.AgentConfig

	// Build image name with tag
	imageName := agentConfig.Image
	if agentConfig.Tag != "" {
		imageName = fmt.Sprintf("%s:%s", agentConfig.Image, agentConfig.Tag)
	}

	// Build command using CommandBuilder
	cmdOpts := CommandOptions{
		Model:     config.Model,
		SessionID: config.SessionID,
	}
	cmd := cm.commandBuilder.BuildCommand(agentConfig, cmdOpts)

	// Expand mounts
	mounts := cm.expandMounts(agentConfig.Mounts, config.WorkspacePath, agentConfig)

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
	memoryBytes := agentConfig.ResourceLimits.MemoryMB * 1024 * 1024
	cpuQuota := int64(agentConfig.ResourceLimits.CPUCores * 100000) // Docker CPU quota

	containerName := fmt.Sprintf("kandev-agent-%s", config.InstanceID[:8])

	return docker.ContainerConfig{
		Name:        containerName,
		Image:       imageName,
		Cmd:         cmd,
		Env:         env,
		WorkingDir:  agentConfig.WorkingDir,
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
func (cm *ContainerManager) expandMounts(templates []registry.MountTemplate, workspacePath string, agentConfig *registry.AgentTypeConfig) []docker.MountConfig {
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
	sessionDirSource := cm.commandBuilder.ExpandSessionDir(agentConfig)
	sessionDirTarget := cm.commandBuilder.GetSessionDirTarget(agentConfig)
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
	return result
}

// buildEnvVars builds environment variables for the container
func (cm *ContainerManager) buildEnvVars(config ContainerConfig) []string {
	agentConfig := config.AgentConfig
	env := make([]string, 0)

	// Add default env from agent config
	for k, v := range agentConfig.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add standard kandev env vars
	env = append(env,
		fmt.Sprintf("KANDEV_TASK_ID=%s", config.TaskID),
		fmt.Sprintf("KANDEV_INSTANCE_ID=%s", config.InstanceID),
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

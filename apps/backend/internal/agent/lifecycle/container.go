// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	WorkspacePath   string // If empty, workspace is not mounted (will clone inside container)
	TaskID          string
	TaskDescription string
	Model           string
	SessionID       string
	Credentials     map[string]string
	ProfileInfo     *AgentProfileInfo
	InstanceID      string
	MainRepoGitDir  string // Path to main repo's .git directory (for worktrees)
	McpServers      []McpServerConfig
	McpMode         string
	PrepareScript   string // Script to run inside container before agent starts (e.g., clone repo)
	BootstrapNonce  string // one-time nonce for agentctl handshake (set internally)
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

// fallbackHomeDir is the directory used when os.UserHomeDir() fails.
const fallbackHomeDir = "/tmp"

// homeDir returns the current user's home directory, falling back to /tmp.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return fallbackHomeDir
	}
	return h
}

// LaunchResult holds the result of a successful container launch.
type LaunchResult struct {
	ContainerID string
	Client      *agentctl.Client
	AuthToken   string // auth token retrieved via handshake (for encrypted storage)
}

// LaunchContainer creates and starts a Docker container for an agent.
// It uses a bootstrap nonce to perform a secure handshake with agentctl:
// the nonce is passed via env var, agentctl generates its own token,
// and the backend retrieves it via POST /auth/handshake.
func (cm *ContainerManager) LaunchContainer(ctx context.Context, config ContainerConfig) (*LaunchResult, error) {
	// Generate bootstrap nonce (NOT the auth token — agentctl generates that)
	nonce, err := generateBootstrapNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate bootstrap nonce: %w", err)
	}
	config.BootstrapNonce = nonce

	containerID, containerIP, err := cm.createAndStartContainer(ctx, config)
	if err != nil {
		return nil, err
	}

	// Create ControlClient (no auth token yet — handshake hasn't happened)
	ctl := agentctl.NewControlClient(containerIP, AgentCtlPort, cm.logger)

	// Wait for agentctl to be healthy
	if err := cm.waitForHealth(ctx, ctl); err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("agentctl health check failed: %w", err)
	}

	// Perform handshake: nonce → token
	authToken, err := ctl.Handshake(ctx, nonce)
	if err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("agentctl handshake failed: %w", err)
	}

	// Create instance and client
	client, err := cm.createInstanceAndClient(ctx, ctl, config, containerID, containerIP)
	if err != nil {
		return nil, err
	}

	cm.logger.Info("docker container launched with handshake auth",
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP),
		zap.String("instance_id", config.InstanceID))

	return &LaunchResult{
		ContainerID: containerID,
		Client:      client,
		AuthToken:   authToken,
	}, nil
}

// createAndStartContainer builds, creates, and starts a Docker container.
func (cm *ContainerManager) createAndStartContainer(
	ctx context.Context, config ContainerConfig,
) (string, string, error) {
	containerCfg, err := cm.buildContainerConfig(config)
	if err != nil {
		return "", "", fmt.Errorf("failed to build container config: %w", err)
	}

	containerID, err := cm.dockerClient.CreateContainer(ctx, containerCfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := cm.dockerClient.StartContainer(ctx, containerID); err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return "", "", fmt.Errorf("failed to start container: %w", err)
	}

	containerIP, err := cm.dockerClient.GetContainerIP(ctx, containerID)
	if err != nil {
		cm.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID), zap.Error(err))
		containerIP = "127.0.0.1"
	}

	return containerID, containerIP, nil
}

// createInstanceAndClient creates an agent instance in the container and returns the client.
func (cm *ContainerManager) createInstanceAndClient(
	ctx context.Context,
	ctl *agentctl.ControlClient,
	config ContainerConfig,
	containerID, containerIP string,
) (*agentctl.Client, error) {
	agentType := ""
	if config.AgentConfig != nil {
		agentType = config.AgentConfig.ID()
	}
	disableAskQuestion := agents.IsPassthroughOnly(config.AgentConfig)
	assumeMcpSse := false
	if config.AgentConfig != nil {
		if rt := config.AgentConfig.Runtime(); rt != nil {
			assumeMcpSse = rt.AssumeMcpSse
		}
	}

	createReq := &agentctl.CreateInstanceRequest{
		ID:                 config.InstanceID,
		WorkspacePath:      "/workspace",
		AgentCommand:       "",
		AgentType:          agentType,
		Env:                config.Credentials,
		AutoStart:          false,
		McpServers:         config.McpServers,
		SessionID:          config.SessionID,
		DisableAskQuestion: disableAskQuestion,
		AssumeMcpSse:       assumeMcpSse,
		McpMode:            config.McpMode,
	}

	resp, err := ctl.CreateInstance(ctx, createReq)
	if err != nil {
		_ = cm.dockerClient.RemoveContainer(ctx, containerID, true)
		return nil, fmt.Errorf("failed to create instance in container: %w", err)
	}

	// ControlClient already has the auth token set via Handshake —
	// read it back for the per-instance Client.
	client := agentctl.NewClient(containerIP, resp.Port, cm.logger,
		agentctl.WithExecutionID(config.InstanceID),
		agentctl.WithSessionID(config.SessionID),
		agentctl.WithAuthToken(ctl.AuthToken()))

	return client, nil
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

	// If a prepare script is provided, pass it as env var for the entrypoint to run
	if config.PrepareScript != "" {
		env = append(env, "KANDEV_PREPARE_SCRIPT="+config.PrepareScript)
	}

	containerCfg := docker.ContainerConfig{
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
			"kandev.home_dir":    homeDir(),
		},
		AutoRemove: false, // We manage cleanup ourselves
	}

	if config.ProfileInfo != nil && config.ProfileInfo.ProfileID != "" {
		containerCfg.Labels["kandev.profile_id"] = config.ProfileInfo.ProfileID
	}

	return containerCfg, nil
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
			homeDir = fallbackHomeDir
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

	// Configure Git settings via environment
	// - Trust all directories (for mounted workspaces)
	// - URL rewriting: SSH → HTTPS for GitHub (enables token auth)
	// - Credential helper for GitHub HTTPS auth (uses GH_TOKEN env var)
	gitConfigCount := 3
	env = append(env,
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
		"GIT_CONFIG_KEY_1=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_1=git@github.com:",
		"GIT_CONFIG_KEY_2=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_2=ssh://git@github.com/",
	)

	// If GitHub token is provided, add credential helper
	// Use ${GH_TOKEN:-${GITHUB_TOKEN}} to support either env var being set
	if config.Credentials["GH_TOKEN"] != "" || config.Credentials["GITHUB_TOKEN"] != "" {
		env = append(env,
			"GIT_CONFIG_KEY_3=credential.https://github.com.helper",
			`GIT_CONFIG_VALUE_3=!f() { echo "username=x-access-token"; echo "password=${GH_TOKEN:-${GITHUB_TOKEN}}"; }; f`,
		)
		gitConfigCount = 4
	}
	env = append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", gitConfigCount))

	// Inject credentials from the provided credentials map
	for k, v := range config.Credentials {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add profile-specific label if available
	if config.ProfileInfo != nil && config.ProfileInfo.ProfileID != "" {
		env = append(env, fmt.Sprintf("KANDEV_AGENT_PROFILE_ID=%s", config.ProfileInfo.ProfileID))
	}

	// Inject bootstrap nonce for agentctl handshake (NOT the auth token)
	if config.BootstrapNonce != "" {
		env = append(env, "AGENTCTL_BOOTSTRAP_NONCE="+config.BootstrapNonce)
	}

	return env
}

// generateBootstrapNonce creates a cryptographically random 32-byte hex-encoded nonce.
func generateBootstrapNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate bootstrap nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
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

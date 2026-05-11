// Package lifecycle manages agent instance lifecycles including tracking,
// state transitions, and cleanup.
package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	dockerAgentctlInstancePortBase = 41001
	dockerAgentctlInstancePortMax  = 41100
	boolStringTrue                 = "true"
)

// ContainerConfig holds configuration for launching a Docker container
type ContainerConfig struct {
	AgentConfig       agents.Agent
	WorkspacePath     string // If empty, workspace is not mounted (will clone inside container)
	TaskID            string
	TaskTitle         string
	TaskEnvironmentID string
	TaskDescription   string
	Model             string
	SessionID         string
	ExecutorProfileID string
	Credentials       map[string]string
	ProfileInfo       *AgentProfileInfo
	InstanceID        string
	MainRepoGitDir    string // Path to main repo's .git directory (for worktrees)
	McpServers        []McpServerConfig
	McpMode           string
	PrepareScript     string // Script to run inside container before agent starts (e.g., clone repo)
	ImageTagOverride  string // If set, replaces the agent runtime's default image (e.g. profile.config.image_tag)
	LocalClonePath    string // Host path for file:// repository clone URLs; mounted read-only at the same path.
	BootstrapNonce    string // one-time nonce for agentctl handshake (set internally)
}

// ContainerManager handles Docker container lifecycle operations
type ContainerManager struct {
	dockerClient   *docker.Client
	commandBuilder *CommandBuilder
	logger         *logger.Logger
	networkName    string
	// kandevHomeDir is the resolved Kandev root dir, used to derive the
	// per-container agent session dirs that replace host home bind-mounts.
	// Empty means "fall back to legacy {home}/.<agent>" — production callers
	// always pass a non-empty value.
	kandevHomeDir string
	// resolveAgentctlBinary returns the host path to a linux/amd64 agentctl
	// binary. Indirected so tests can inject a stub.
	resolveAgentctlBinary func() (string, error)
	// resolveMockAgentBinary returns the host path to a linux/amd64 mock-agent
	// binary. When it returns "" without error, no mock-agent mount is added
	// (production case). Used by Docker E2E tests.
	resolveMockAgentBinary func() (string, error)
}

// NewContainerManager creates a new ContainerManager. kandevHomeDir is the
// resolved Kandev root dir used to host per-container agent session dirs;
// pass "" only in legacy callers/tests that don't exercise the session-dir
// mount path.
func NewContainerManager(dockerClient *docker.Client, networkName, kandevHomeDir string, log *logger.Logger) *ContainerManager {
	resolver := NewAgentctlResolver(log)
	mockResolver := NewMockAgentResolver(log)
	return &ContainerManager{
		dockerClient:           dockerClient,
		commandBuilder:         NewCommandBuilder(),
		logger:                 log.WithFields(zap.String("component", "container-manager")),
		networkName:            networkName,
		kandevHomeDir:          kandevHomeDir,
		resolveAgentctlBinary:  resolver.ResolveLinuxBinary,
		resolveMockAgentBinary: mockResolver.ResolveLinuxBinary,
	}
}

// LaunchResult holds the result of a successful container launch.
type LaunchResult struct {
	ContainerID    string
	Client         *agentctl.Client
	AuthToken      string // auth token retrieved via handshake (for encrypted storage)
	BootstrapNonce string // nonce injected into container env for future restart handshakes
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

	containerID, containerIP, controlHost, controlPort, err := cm.createAndStartContainer(ctx, config)
	if err != nil {
		return nil, err
	}

	// Create ControlClient (no auth token yet — handshake hasn't happened)
	ctl := agentctl.NewControlClient(controlHost, controlPort, cm.logger)

	// Wait for agentctl to be healthy
	if err := cm.waitForHealth(ctx, ctl); err != nil {
		cm.removeContainerBestEffort(containerID)
		return nil, fmt.Errorf("agentctl health check failed: %w", err)
	}

	// Perform handshake: nonce → token
	authToken, err := ctl.Handshake(ctx, nonce)
	if err != nil {
		cm.removeContainerBestEffort(containerID)
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
		ContainerID:    containerID,
		Client:         client,
		AuthToken:      authToken,
		BootstrapNonce: nonce,
	}, nil
}

// createAndStartContainer builds, creates, and starts a Docker container.
func (cm *ContainerManager) createAndStartContainer(
	ctx context.Context, config ContainerConfig,
) (string, string, string, int, error) {
	containerCfg, err := cm.buildContainerConfig(config)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("failed to build container config: %w", err)
	}

	containerID, err := cm.dockerClient.CreateContainer(ctx, containerCfg)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("failed to create container: %w", err)
	}

	if err := cm.dockerClient.StartContainer(ctx, containerID); err != nil {
		cm.removeContainerBestEffort(containerID)
		return "", "", "", 0, fmt.Errorf("failed to start container: %w", err)
	}

	containerIP, err := cm.dockerClient.GetContainerIP(ctx, containerID)
	if err != nil {
		cm.logger.Warn("failed to get container IP, trying localhost",
			zap.String("container_id", containerID), zap.Error(err))
		containerIP = "127.0.0.1"
	}

	controlHost, controlPort := cm.resolveContainerEndpoint(ctx, containerID, AgentCtlPort, containerIP)
	return containerID, containerIP, controlHost, controlPort, nil
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
		cm.removeContainerBestEffort(containerID)
		return nil, fmt.Errorf("failed to create instance in container: %w", err)
	}

	instanceHost, instancePort := cm.resolveContainerEndpoint(ctx, containerID, resp.Port, containerIP)

	// ControlClient already has the auth token set via Handshake —
	// read it back for the per-instance Client.
	client := agentctl.NewClient(instanceHost, instancePort, cm.logger,
		agentctl.WithExecutionID(config.InstanceID),
		agentctl.WithSessionID(config.SessionID),
		agentctl.WithAuthToken(ctl.AuthToken()))

	return client, nil
}

func (cm *ContainerManager) resolveContainerEndpoint(ctx context.Context, containerID string, containerPort int, fallbackHost string) (string, int) {
	host, port, err := cm.dockerClient.GetContainerHostPort(ctx, containerID, containerPort)
	if err == nil {
		return host, port
	}
	cm.logger.Warn("failed to resolve published Docker port, falling back to container IP",
		zap.String("container_id", containerID),
		zap.Int("container_port", containerPort),
		zap.String("fallback_host", fallbackHost),
		zap.Error(err))
	return fallbackHost, containerPort
}

func (cm *ContainerManager) removeContainerBestEffort(containerID string) {
	if containerID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := cm.dockerClient.RemoveContainer(ctx, containerID, true); err != nil {
		cm.logger.Warn("failed to remove container after launch failure",
			zap.String("container_id", containerID),
			zap.Error(err))
	}
}

// waitForHealth waits for agentctl to be healthy with retries.
// The budget covers the time it takes for the container's bootstrap to run the
// prepare script (git clone, optional network installs) and then exec agentctl.
// 120s is generous but matches what real workspaces need on first launch.
func (cm *ContainerManager) waitForHealth(ctx context.Context, ctl *agentctl.ControlClient) error {
	const maxRetries = 240
	const retryDelay = 500 * time.Millisecond

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := ctl.Health(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Cancelable wait that also skips the final retry's sleep — the loop
		// only re-enters if i+1 < maxRetries, so the extra delay was just
		// added latency on aborted launches.
		if i+1 < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	if lastErr != nil {
		return fmt.Errorf("agentctl not healthy after %s: %w",
			time.Duration(maxRetries)*retryDelay, lastErr)
	}
	return fmt.Errorf("agentctl not healthy after %s",
		time.Duration(maxRetries)*retryDelay)
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

	// Build image name with tag. A profile-level image_tag override (e.g. a
	// custom Dockerfile built via the executor profile UI) takes precedence
	// over the agent's hardcoded runtime default.
	imageName := rt.Image
	if rt.Tag != "" {
		imageName = fmt.Sprintf("%s:%s", rt.Image, rt.Tag)
	}
	if config.ImageTagOverride != "" {
		imageName = config.ImageTagOverride
	}

	// We don't pre-build the agent CLI command here: agentctl receives it later
	// via its HTTP API (CreateInstance). The container only needs to launch
	// agentctl as its main process — see the Entrypoint setup below.

	// Two paths through here use different "workspace" strings:
	//   - Mount sources expand against the HOST workspace path (config.WorkspacePath
	//     when set, otherwise no host mount in clone-inside-container mode).
	//   - WorkingDir is an in-container path; it must always be the container-side
	//     mount target. Both modes converge on /workspace (the bind-mount target
	//     in host mode, the clone destination in clone-inside mode), so we hard-
	//     code it here. Without this distinction, host-bind setups would set
	//     WorkingDir to the host path, and Docker would happily start the
	//     container in an unrelated directory.
	const containerWorkspacePath = "/workspace"

	// Expand mounts using the host path so {workspace} substitutions in mount
	// sources resolve to a real on-disk location.
	mounts := cm.expandMounts(rt.Mounts, config.WorkspacePath, ag, config.InstanceID)

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

	if config.LocalClonePath != "" {
		mounts = append(mounts, docker.MountConfig{
			Source:   config.LocalClonePath,
			Target:   config.LocalClonePath,
			ReadOnly: true,
		})
		cm.logger.Debug("added local clone source mount",
			zap.String("path", config.LocalClonePath))
	}

	// Mount the host agentctl linux binary into the container so user-built
	// images don't have to bake it in. Resolved via AgentctlResolver — same path
	// the Sprites executor uses.
	if cm.resolveAgentctlBinary != nil {
		agentctlPath, err := cm.resolveAgentctlBinary()
		if err != nil {
			return docker.ContainerConfig{}, fmt.Errorf("agentctl linux binary not found: %w", err)
		}
		mounts = append(mounts, docker.MountConfig{
			Source:   agentctlPath,
			Target:   "/usr/local/bin/agentctl",
			ReadOnly: true,
		})
	}

	// Optionally mount a host mock-agent binary for Docker E2E tests. Production
	// builds run real agents installed in the image; this mount only fires when
	// KANDEV_MOCK_AGENT_LINUX_BINARY is set or the binary is sitting in build/.
	if cm.resolveMockAgentBinary != nil {
		mockPath, err := cm.resolveMockAgentBinary()
		if err != nil {
			return docker.ContainerConfig{}, fmt.Errorf("mock-agent binary lookup: %w", err)
		}
		if mockPath != "" {
			mounts = append(mounts, docker.MountConfig{
				Source:   mockPath,
				Target:   "/usr/local/bin/mock-agent",
				ReadOnly: true,
			})
		}
	}

	// Build environment variables
	env := cm.buildEnvVars(config)

	// Calculate resource limits
	memoryBytes := rt.ResourceLimits.MemoryMB * 1024 * 1024
	cpuQuota := int64(rt.ResourceLimits.CPUCores * 100000) // Docker CPU quota

	containerName := fmt.Sprintf("kandev-agent-%s", config.InstanceID[:8])

	// If a prepare script is provided, pass it as env var for the bootstrap to run
	if config.PrepareScript != "" {
		env = append(env, "KANDEV_PREPARE_SCRIPT="+config.PrepareScript)
	}

	// We always launch agentctl as the container's main process and fan out the
	// agent subprocess from there via the agentctl HTTP API. This frees user-built
	// images from needing to bake an ENTRYPOINT or know which agent to run — they
	// only need a runtime that supports the agent CLI (typically node + git).
	//
	// The agent's BuildCommand result intentionally stops being passed here;
	// agentctl receives the agent command later via the CreateInstance API.
	//
	// Prepare runs in a subshell so its `set -e` (most prepare scripts opt in)
	// can't kill the bootstrap before exec'ing agentctl. If prepare fails, we
	// still bring agentctl up so the host can connect, surface the failure, and
	// the user can debug from the Executor Settings popover.
	//
	//nolint:dupword // two `fi` tokens close two distinct shell blocks.
	bootstrap := []string{
		"sh", "-c",
		`if [ -n "$KANDEV_PREPARE_SCRIPT" ]; then
  (eval "$KANDEV_PREPARE_SCRIPT")
  prep_rc=$?
  if [ "$prep_rc" -ne 0 ]; then
    echo "[kandev-bootstrap] prepare script failed (exit $prep_rc); starting agentctl anyway so the host can connect and the user can debug via Executor Settings" >&2
  fi
fi
exec /usr/local/bin/agentctl`,
	}

	containerCfg := docker.ContainerConfig{
		Name:         containerName,
		Image:        imageName,
		Entrypoint:   bootstrap,
		Cmd:          nil,
		Env:          env,
		WorkingDir:   cm.expandMountSource(rt.WorkingDir, containerWorkspacePath),
		Mounts:       mounts,
		PortBindings: dockerAgentctlPortBindings(),
		NetworkMode:  cm.networkName,
		Memory:       memoryBytes,
		CPUQuota:     cpuQuota,
		Labels: map[string]string{
			"kandev.managed":             boolStringTrue,
			"kandev.instance_id":         config.InstanceID,
			"kandev.task_id":             config.TaskID,
			"kandev.session_id":          config.SessionID,
			"kandev.task_environment_id": config.TaskEnvironmentID,
			"com.kandev.image":           imageName,
		},
		AutoRemove: false, // We manage cleanup ourselves
	}

	if config.ExecutorProfileID != "" {
		containerCfg.Labels["kandev.executor_profile_id"] = config.ExecutorProfileID
		containerCfg.Labels["kandev.profile_id"] = config.ExecutorProfileID
	}
	if config.TaskTitle != "" {
		containerCfg.Labels["kandev.task_title"] = config.TaskTitle
	}
	if config.ProfileInfo != nil && config.ProfileInfo.ProfileID != "" {
		containerCfg.Labels["kandev.profile_id"] = config.ProfileInfo.ProfileID
	}

	return containerCfg, nil
}

func dockerAgentctlPortBindings() []docker.PortBindingConfig {
	bindings := make([]docker.PortBindingConfig, 0, 1+dockerAgentctlInstancePortMax-dockerAgentctlInstancePortBase+1)
	bindings = append(bindings, newDockerPortBinding(AgentCtlPort))
	for port := dockerAgentctlInstancePortBase; port <= dockerAgentctlInstancePortMax; port++ {
		bindings = append(bindings, newDockerPortBinding(port))
	}
	return bindings
}

func newDockerPortBinding(containerPort int) docker.PortBindingConfig {
	return docker.PortBindingConfig{
		ContainerPort: containerPort,
		HostIP:        "127.0.0.1",
		HostPort:      "0",
	}
}

// expandMounts expands mount templates with actual paths.
//
// Per-agent session dirs (SessionConfig.SessionDirTemplate) are mapped to a
// kandev-managed path under <kandev-home>/agent-sessions/<instance_id>/, NOT
// the user's host home. The seeder (SeedAgentSessionDir) selectively copies
// auth files from the host beforehand, so every agent gets a fresh, isolated
// session dir per launch — host state DBs and session caches that contain
// absolute host paths (e.g. codex's state.db) stay out of the container.
func (cm *ContainerManager) expandMounts(templates []agents.MountTemplate, workspacePath string, ag agents.Agent, instanceID string) []docker.MountConfig {
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
	sessionDirSource := cm.commandBuilder.ExpandSessionDir(ag, cm.kandevHomeDir, instanceID)
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

// expandMountSource expands template variables in mount source paths.
// Only `{workspace}` is honoured here — agent session dirs that used to use
// `{home}/.<agent>` now route through the kandev-managed per-container
// session dir (see SessionDirHostPath), which keeps host state DBs out of
// the container. Production agents don't ship `{home}` in Mounts; codex_acp
// has a regression test asserting this.
func (cm *ContainerManager) expandMountSource(source, workspacePath string) string {
	return strings.ReplaceAll(source, "{workspace}", workspacePath)
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

	env = append(env,
		fmt.Sprintf("AGENTCTL_PORT=%d", AgentCtlPort),
		fmt.Sprintf("AGENTCTL_INSTANCE_PORT_BASE=%d", dockerAgentctlInstancePortBase),
		fmt.Sprintf("AGENTCTL_INSTANCE_PORT_MAX=%d", dockerAgentctlInstancePortMax),
	)

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
		"kandev.managed": boolStringTrue,
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

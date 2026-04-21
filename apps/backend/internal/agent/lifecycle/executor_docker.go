package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/scriptengine"
)

const dockerWorkspacePath = "/workspace"

// getMetadataString retrieves a string value from metadata map.
func getMetadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata[key].(string); ok {
		return v
	}
	return ""
}

func getMetadataBool(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	switch raw := metadata[key].(type) {
	case bool:
		return raw
	case string:
		return strings.EqualFold(strings.TrimSpace(raw), "true")
	default:
		return false
	}
}

// DockerExecutor implements Runtime for Docker-based agent execution.
// The Docker client is created lazily on first use (not at startup).
type DockerExecutor struct {
	cfg    config.DockerConfig
	logger *logger.Logger

	// newClientFunc creates the Docker client. Defaults to docker.NewClient.
	// Override in tests to simulate failures.
	newClientFunc func(config.DockerConfig, *logger.Logger) (*docker.Client, error)

	// Lazy-initialized on first use via ensureClient().
	// Uses mu + initialized instead of sync.Once so that transient Docker
	// daemon failures can be retried on subsequent calls.
	mu           sync.Mutex
	initialized  bool
	docker       *docker.Client
	containerMgr *ContainerManager
}

// NewDockerExecutor creates a new Docker runtime.
// The Docker client is NOT created here — it is initialized lazily
// when CreateInstance is called.
func NewDockerExecutor(cfg config.DockerConfig, log *logger.Logger) *DockerExecutor {
	return &DockerExecutor{
		cfg:           cfg,
		logger:        log.WithFields(zap.String("runtime", "docker")),
		newClientFunc: docker.NewClient,
	}
}

// ensureClient lazily creates the Docker client and ContainerManager.
// Unlike sync.Once, this retries on failure so transient Docker daemon
// unavailability doesn't permanently disable the executor.
func (r *DockerExecutor) ensureClient() (*docker.Client, *ContainerManager, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return r.docker, r.containerMgr, nil
	}

	cli, err := r.newClientFunc(r.cfg, r.logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	r.docker = cli
	r.containerMgr = NewContainerManager(cli, "", r.logger)
	r.initialized = true

	return r.docker, r.containerMgr, nil
}

// Client returns the lazily-initialized Docker client, or nil if Docker is unavailable.
func (r *DockerExecutor) Client() *docker.Client {
	cli, _, _ := r.ensureClient()
	return cli
}

// ContainerMgr returns the lazily-initialized ContainerManager, or nil if Docker is unavailable.
func (r *DockerExecutor) ContainerMgr() *ContainerManager {
	_, cm, _ := r.ensureClient()
	return cm
}

func (r *DockerExecutor) Name() executor.Name {
	return executor.NameDocker
}

func (r *DockerExecutor) HealthCheck(_ context.Context) error {
	// No-op: Docker availability is checked lazily when CreateInstance is called.
	return nil
}

func (r *DockerExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	dockerClient, containerMgr, err := r.ensureClient()
	if err != nil {
		return nil, fmt.Errorf("docker unavailable: %w", err)
	}

	// On resume, try to reconnect to the existing container
	if req.PreviousExecutionID != "" {
		instance, reconnectErr := r.reconnectToContainer(ctx, dockerClient, req)
		if reconnectErr == nil {
			return instance, nil
		}
		r.logger.Info("could not reconnect to previous container, creating new one",
			zap.String("previous_execution_id", req.PreviousExecutionID),
			zap.Error(reconnectErr))
	}

	// Extract runtime-specific values from metadata
	worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID)
	worktreeBranch := getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)

	// Resolve prepare script for cloning repo inside container
	prepareScript := r.resolvePrepareScript(req)

	// Convert ExecutorCreateRequest to ContainerConfig
	// Note: WorkspacePath is empty to skip mounting - we clone inside container
	containerCfg := ContainerConfig{
		AgentConfig:   req.AgentConfig,
		WorkspacePath: "", // Empty = no workspace mount, we'll clone instead
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		InstanceID:    req.InstanceID,
		Credentials:   req.Env, // Env contains credentials from the caller
		McpServers:    req.McpServers,
		PrepareScript: prepareScript,
	}

	// Use ContainerManager to launch container (includes nonce handshake)
	result, err := containerMgr.LaunchContainer(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	// Get container IP for logging
	containerIP, _ := dockerClient.GetContainerIP(ctx, result.ContainerID)

	// Build metadata
	metadata := make(map[string]interface{})
	metadata[MetadataKeyIsRemote] = true // Mark as remote for shell handling
	if worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = dockerWorkspacePath
		metadata["worktree_branch"] = worktreeBranch
	}

	r.logger.Info("docker instance created",
		zap.String("instance_id", req.InstanceID),
		zap.String("container_id", result.ContainerID),
		zap.String("container_ip", containerIP))

	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   string(r.Name()),
		Client:        result.Client,
		ContainerID:   result.ContainerID,
		ContainerIP:   containerIP,
		WorkspacePath: dockerWorkspacePath,
		Metadata:      metadata,
		AuthToken:     result.AuthToken,
	}, nil
}

// reconnectToContainer attempts to reconnect to an existing Docker container
// from a previous execution. Returns the reconnected instance if successful.
func (r *DockerExecutor) reconnectToContainer(ctx context.Context, dockerClient *docker.Client, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	// Derive container name from previous execution ID (same pattern as LaunchContainer)
	prevID := req.PreviousExecutionID
	if len(prevID) < 8 {
		return nil, fmt.Errorf("previous execution ID too short: %s", prevID)
	}
	containerName := fmt.Sprintf("kandev-agent-%s", prevID[:8])

	// Check if the container is still running
	running, err := dockerClient.IsContainerRunning(ctx, containerName)
	if err != nil || !running {
		return nil, fmt.Errorf("container %s not running", containerName)
	}

	// Get the container IP
	containerIP, err := dockerClient.GetContainerIP(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP for container %s: %w", containerName, err)
	}

	// Check if agentctl is healthy
	ctl := agentctl.NewControlClient(containerIP, AgentCtlPort, r.logger)
	if err := ctl.Health(ctx); err != nil {
		return nil, fmt.Errorf("agentctl not healthy in container %s: %w", containerName, err)
	}

	// Check if the previous instance is still alive
	instancePort, reusingProcess, err := r.findExistingInstance(ctx, ctl, containerIP, prevID)
	if err != nil {
		return nil, fmt.Errorf("failed to find instance in container %s: %w", containerName, err)
	}

	// Create client pointing to the instance port
	client := agentctl.NewClient(containerIP, instancePort, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID))

	// Get container info for the full container ID
	info, err := dockerClient.GetContainerInfo(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerName, err)
	}

	metadata := map[string]interface{}{
		MetadataKeyIsRemote:      true,
		"reuse_existing_process": reusingProcess,
	}

	r.logger.Info("reconnected to existing docker container",
		zap.String("container_name", containerName),
		zap.String("container_id", info.ID),
		zap.String("container_ip", containerIP),
		zap.Int("instance_port", instancePort),
		zap.Bool("reusing_process", reusingProcess))

	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   string(r.Name()),
		Client:        client,
		ContainerID:   info.ID,
		ContainerIP:   containerIP,
		WorkspacePath: dockerWorkspacePath,
		Metadata:      metadata,
	}, nil
}

// findExistingInstance checks if a previous instance is still running in the container.
// Returns the instance port and whether the agent subprocess is also running.
func (r *DockerExecutor) findExistingInstance(ctx context.Context, ctl *agentctl.ControlClient, containerIP, prevExecutionID string) (int, bool, error) {
	// Try to get the existing instance by its ID
	instance, err := ctl.GetInstance(ctx, prevExecutionID)
	if err == nil && instance != nil && instance.Port > 0 {
		// Instance exists, check if agent subprocess is running
		client := agentctl.NewClient(containerIP, instance.Port, r.logger)
		status, statusErr := client.GetStatus(ctx)
		processRunning := statusErr == nil && status != nil && status.IsAgentRunning()
		return instance.Port, processRunning, nil
	}

	// Instance not found — create a new instance in the existing container
	createReq := &agentctl.CreateInstanceRequest{
		ID:            prevExecutionID,
		WorkspacePath: dockerWorkspacePath,
		AutoStart:     false,
	}
	resp, createErr := ctl.CreateInstance(ctx, createReq)
	if createErr != nil {
		return 0, false, fmt.Errorf("failed to create new instance: %w", createErr)
	}
	return resp.Port, false, nil
}

func (r *DockerExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance.ContainerID == "" {
		return nil // No container to stop
	}

	dockerClient, _, err := r.ensureClient()
	if err != nil {
		return fmt.Errorf("docker unavailable: %w", err)
	}

	if force {
		err = dockerClient.KillContainer(ctx, instance.ContainerID, "SIGKILL")
	} else {
		err = dockerClient.StopContainer(ctx, instance.ContainerID, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

func (r *DockerExecutor) RecoverInstances(_ context.Context) ([]*ExecutorInstance, error) {
	// No-op: Docker client is initialized lazily on first use.
	// If no session has used Docker yet, there's nothing to recover.
	// Running containers from a previous backend process will be detected
	// when the user navigates to that session (via EnsureWorkspaceExecutionForSession).
	return nil, nil
}

// Close closes the Docker client if it was initialized.
// Safe to call even if the client was never created.
func (r *DockerExecutor) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.docker != nil {
		err := r.docker.Close()
		r.docker = nil
		r.containerMgr = nil
		r.initialized = false
		return err
	}
	return nil
}

// GetInteractiveRunner returns nil for Docker runtime.
// Passthrough mode is not supported in Docker-based execution.
func (r *DockerExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func (r *DockerExecutor) RequiresCloneURL() bool          { return true }
func (r *DockerExecutor) ShouldApplyPreferredShell() bool { return false }
func (r *DockerExecutor) IsAlwaysResumable() bool         { return true }

// resolvePrepareScript builds the resolved prepare script using scriptengine.
// This script clones the repository inside the container.
func (r *DockerExecutor) resolvePrepareScript(req *ExecutorCreateRequest) string {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		script = DefaultPrepareScript("local_docker")
	}
	if script == "" {
		return ""
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(dockerWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			getGitRemoteURL,
			r.injectTokenIntoURL,
		)).
		// Docker image has agents and agentctl pre-installed;
		// resolve these to empty so stored scripts with these placeholders don't break.
		// The entrypoint handles agentctl startup, so install/start must be no-ops.
		WithProvider(scriptengine.AgentInstallProvider(nil)).
		WithStatic(map[string]string{
			"kandev.agentctl.port":    "9999",
			"kandev.agentctl.install": "",
			"kandev.agentctl.start":   "",
		})

	return resolver.Resolve(script)
}

// injectTokenIntoURL adds a GitHub token to clone URLs for authentication.
// Handles both HTTPS and SSH URLs (converting SSH to authenticated HTTPS).
func (r *DockerExecutor) injectTokenIntoURL(cloneURL string, env map[string]string) string {
	token := env["GITHUB_TOKEN"]
	if token == "" {
		token = env["GH_TOKEN"]
	}
	if token == "" {
		return cloneURL
	}

	// Convert SSH URLs to HTTPS first
	if strings.HasPrefix(cloneURL, "git@github.com:") {
		path := strings.TrimPrefix(cloneURL, "git@github.com:")
		cloneURL = "https://github.com/" + path
	}

	// Convert https://github.com/... to https://x-access-token:TOKEN@github.com/...
	if strings.HasPrefix(cloneURL, "https://github.com/") {
		return strings.Replace(cloneURL, "https://github.com/", "https://x-access-token:"+token+"@github.com/", 1)
	}
	return cloneURL
}

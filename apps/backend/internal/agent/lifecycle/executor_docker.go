package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
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
		return strings.EqualFold(strings.TrimSpace(raw), boolStringTrue)
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
		AgentConfig:       req.AgentConfig,
		WorkspacePath:     "", // Empty = no workspace mount, we'll clone instead
		TaskID:            req.TaskID,
		TaskEnvironmentID: req.TaskEnvironmentID,
		SessionID:         req.SessionID,
		ExecutorProfileID: getMetadataString(req.Metadata, "executor_profile_id"),
		InstanceID:        req.InstanceID,
		Credentials:       req.Env, // Env contains credentials from the caller
		McpServers:        req.McpServers,
		PrepareScript:     prepareScript,
		ImageTagOverride:  getMetadataString(req.Metadata, MetadataKeyImageTagOverride),
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
		InstanceID:     req.InstanceID,
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		RuntimeName:    string(r.Name()),
		Client:         result.Client,
		ContainerID:    result.ContainerID,
		ContainerIP:    containerIP,
		WorkspacePath:  dockerWorkspacePath,
		Metadata:       metadata,
		AuthToken:      result.AuthToken,
		BootstrapNonce: result.BootstrapNonce,
	}, nil
}

// reconnectToContainer attempts to reconnect to an existing Docker container
// from a previous execution. Returns the reconnected instance if successful.
func (r *DockerExecutor) reconnectToContainer(ctx context.Context, dockerClient *docker.Client, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	containerRef, err := resolveReconnectContainerRef(req)
	if err != nil {
		return nil, err
	}
	prevID := req.PreviousExecutionID

	info, err := dockerClient.GetContainerInfo(ctx, containerRef)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerRef, err)
	}
	if shouldStartExistingDockerContainer(info.State) {
		r.logger.Info("starting stopped docker container for reconnect",
			zap.String("container_id", info.ID),
			zap.String("state", info.State))
		if err := dockerClient.StartContainer(ctx, info.ID); err != nil {
			return nil, fmt.Errorf("failed to start container %s: %w", info.ID, err)
		}
		info, err = dockerClient.GetContainerInfo(ctx, info.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect started container %s: %w", containerRef, err)
		}
	}
	if info.State != containerStateRunning {
		return nil, fmt.Errorf("container %s is %s, not %s", containerRef, info.State, containerStateRunning)
	}

	// Get the container IP
	containerIP, err := dockerClient.GetContainerIP(ctx, info.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get IP for container %s: %w", info.ID, err)
	}

	controlHost, controlPort := resolveDockerEndpoint(ctx, dockerClient, info.ID, AgentCtlPort, containerIP, r.logger)

	// Check if agentctl is healthy
	ctl := agentctl.NewControlClient(controlHost, controlPort, r.logger,
		agentctl.WithControlAuthToken(req.AuthToken))
	if err := r.waitForAgentctlHealth(ctx, ctl); err != nil {
		return nil, fmt.Errorf("agentctl not healthy in container %s: %w", info.ID, err)
	}

	// Check if the previous instance is still alive
	authToken := req.AuthToken
	instancePort, reusingProcess, err := r.findExistingInstance(ctx, dockerClient, ctl, req, info.ID, containerIP, prevID, authToken)
	if err != nil && req.BootstrapNonce != "" && isAgentctlAuthError(err) {
		var handshakeErr error
		authToken, handshakeErr = ctl.Handshake(ctx, req.BootstrapNonce)
		if handshakeErr != nil {
			return nil, fmt.Errorf("agentctl auth failed and re-handshake failed in container %s: %w", info.ID, handshakeErr)
		}
		instancePort, reusingProcess, err = r.findExistingInstance(ctx, dockerClient, ctl, req, info.ID, containerIP, prevID, authToken)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find instance in container %s: %w", info.ID, err)
	}
	instanceHost, resolvedInstancePort := resolveDockerEndpoint(ctx, dockerClient, info.ID, instancePort, containerIP, r.logger)

	// Create client pointing to the instance port
	client := agentctl.NewClient(instanceHost, resolvedInstancePort, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID),
		agentctl.WithAuthToken(authToken))

	metadata := map[string]interface{}{
		MetadataKeyIsRemote:      true,
		MetadataKeyContainerID:   info.ID,
		"reuse_existing_process": reusingProcess,
	}

	refreshedAuthToken := ""
	if authToken != "" && authToken != req.AuthToken {
		refreshedAuthToken = authToken
	}

	r.logger.Info("reconnected to existing docker container",
		zap.String("container_ref", containerRef),
		zap.String("container_id", info.ID),
		zap.String("container_ip", containerIP),
		zap.String("control_host", controlHost),
		zap.Int("control_port", controlPort),
		zap.String("instance_host", instanceHost),
		zap.Int("instance_port", resolvedInstancePort),
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
		AuthToken:     refreshedAuthToken,
	}, nil
}

func resolveReconnectContainerRef(req *ExecutorCreateRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("executor create request is nil")
	}
	if containerID := strings.TrimSpace(getMetadataString(req.Metadata, MetadataKeyContainerID)); containerID != "" {
		return containerID, nil
	}
	prevID := req.PreviousExecutionID
	if len(prevID) < 8 {
		return "", fmt.Errorf("previous execution ID too short: %s", prevID)
	}
	return fmt.Sprintf("kandev-agent-%s", prevID[:8]), nil
}

func shouldStartExistingDockerContainer(state string) bool {
	switch state {
	case containerStateCreated, containerStateExited:
		return true
	default:
		return false
	}
}

func isAgentctlAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "auth token")
}

// findExistingInstance checks if a previous instance is still running in the container.
// Returns the instance port and whether the agent subprocess is also running.
func (r *DockerExecutor) findExistingInstance(
	ctx context.Context,
	dockerClient *docker.Client,
	ctl *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	containerID string,
	containerIP string,
	prevExecutionID string,
	authToken string,
) (int, bool, error) {
	// Try to get the existing instance by its ID
	instance, err := ctl.GetInstance(ctx, prevExecutionID)
	if err == nil && instance != nil && instance.Port > 0 {
		// Instance exists, check if agent subprocess is running
		instanceHost, instancePort := resolveDockerEndpoint(ctx, dockerClient, containerID, instance.Port, containerIP, r.logger)
		client := agentctl.NewClient(instanceHost, instancePort, r.logger,
			agentctl.WithAuthToken(authToken))
		status, statusErr := client.GetStatus(ctx)
		processRunning := statusErr == nil && status != nil && status.IsAgentRunning()
		return instance.Port, processRunning, nil
	}
	if err != nil && isAgentctlAuthError(err) {
		return 0, false, err
	}

	// Instance not found — create a new instance in the existing container
	createReq := buildReconnectCreateInstanceRequest(req, prevExecutionID)
	resp, createErr := ctl.CreateInstance(ctx, createReq)
	if createErr != nil {
		return 0, false, fmt.Errorf("failed to create new instance: %w", createErr)
	}
	return resp.Port, false, nil
}

func buildReconnectCreateInstanceRequest(req *ExecutorCreateRequest, instanceID string) *agentctl.CreateInstanceRequest {
	agentType := ""
	disableAskQuestion := false
	assumeMcpSse := false
	if req.AgentConfig != nil {
		agentType = req.AgentConfig.ID()
		disableAskQuestion = agents.IsPassthroughOnly(req.AgentConfig)
		if rt := req.AgentConfig.Runtime(); rt != nil {
			assumeMcpSse = rt.AssumeMcpSse
		}
	}
	return &agentctl.CreateInstanceRequest{
		ID:                 instanceID,
		WorkspacePath:      dockerWorkspacePath,
		AgentType:          agentType,
		Env:                req.Env,
		AutoStart:          false,
		McpServers:         req.McpServers,
		SessionID:          req.SessionID,
		TaskID:             req.TaskID,
		DisableAskQuestion: disableAskQuestion,
		AssumeMcpSse:       assumeMcpSse,
		McpMode:            req.McpMode,
	}
}

func (r *DockerExecutor) waitForAgentctlHealth(ctx context.Context, ctl *agentctl.ControlClient) error {
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
		time.Sleep(retryDelay)
	}

	if lastErr != nil {
		return fmt.Errorf("agentctl not healthy after %s: %w",
			time.Duration(maxRetries)*retryDelay, lastErr)
	}
	return fmt.Errorf("agentctl not healthy after %s",
		time.Duration(maxRetries)*retryDelay)
}

func resolveDockerEndpoint(
	ctx context.Context,
	dockerClient *docker.Client,
	containerID string,
	containerPort int,
	fallbackHost string,
	log *logger.Logger,
) (string, int) {
	host, port, err := dockerClient.GetContainerHostPort(ctx, containerID, containerPort)
	if err == nil {
		return host, port
	}
	log.Warn("failed to resolve published Docker port, falling back to container IP",
		zap.String("container_id", containerID),
		zap.Int("container_port", containerPort),
		zap.String("fallback_host", fallbackHost),
		zap.Error(err))
	return fallbackHost, containerPort
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

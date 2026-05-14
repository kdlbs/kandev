package lifecycle

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
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

// DockerExecutor implements Runtime for Docker-based agent execution.
// The Docker client is created lazily on first use (not at startup).
type DockerExecutor struct {
	cfg           config.DockerConfig
	kandevHomeDir string
	logger        *logger.Logger

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
// when CreateInstance is called. kandevHomeDir is the resolved kandev root
// directory used to host per-container agent session dirs (the replacement
// for host home bind mounts that were leaking host state into containers).
func NewDockerExecutor(cfg config.DockerConfig, kandevHomeDir string, log *logger.Logger) *DockerExecutor {
	return &DockerExecutor{
		cfg:           cfg,
		kandevHomeDir: kandevHomeDir,
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
	r.containerMgr = NewContainerManager(cli, "", r.kandevHomeDir, r.logger)
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

func (r *DockerExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (instance *ExecutorInstance, err error) {
	dockerClient, containerMgr, err := r.ensureClient()
	if err != nil {
		return nil, fmt.Errorf("docker unavailable: %w", err)
	}

	if req.OnProgress != nil {
		defer reportCreateInstanceProgress(req, &err)()
	}

	if reconnected, ok := r.tryReconnect(ctx, dockerClient, req); ok {
		return reconnected, nil
	}

	r.seedSessionDir(ctx, req)

	containerCfg := r.buildContainerLaunchConfig(req)
	result, err := containerMgr.LaunchContainer(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	containerIP, _ := dockerClient.GetContainerIP(ctx, result.ContainerID)
	r.logger.Info("docker instance created",
		zap.String("instance_id", req.InstanceID),
		zap.String("container_id", result.ContainerID),
		zap.String("container_ip", containerIP))

	return r.buildCreatedInstance(req, result, containerIP), nil
}

// reportCreateInstanceProgress wires the "Waiting for Docker container" step
// into the caller's OnProgress callback. The returned closure must run after
// CreateInstance finishes (deferred) so it sees the final err state.
func reportCreateInstanceProgress(req *ExecutorCreateRequest, errPtr *error) func() {
	step := beginStep("Waiting for Docker container")
	reportProgress(req.OnProgress, step, 0, 1)
	return func() {
		if *errPtr != nil {
			completeStepError(&step, (*errPtr).Error())
		} else {
			completeStepSuccess(&step)
		}
		reportProgress(req.OnProgress, step, 0, 1)
	}
}

// tryReconnect returns (instance, true) if the request points at an existing
// container that's healthy enough to resume; otherwise (nil, false) and the
// caller falls back to provisioning a fresh container.
func (r *DockerExecutor) tryReconnect(ctx context.Context, dockerClient *docker.Client, req *ExecutorCreateRequest) (*ExecutorInstance, bool) {
	if req.PreviousExecutionID == "" {
		return nil, false
	}
	reconnected, reconnectErr := r.reconnectToContainer(ctx, dockerClient, req)
	if reconnectErr == nil {
		return reconnected, true
	}
	r.logger.Info("could not reconnect to previous container, creating new one",
		zap.String("previous_execution_id", req.PreviousExecutionID),
		zap.Error(reconnectErr))
	return nil, false
}

// seedSessionDir copies the agent's auth files (auth.json / config.toml /
// etc.) into the per-container session dir. Replaces the older pattern of
// bind-mounting the host's whole ~/.<agent>, which leaked absolute host
// paths into agent state DBs and broke resume on codex.
func (r *DockerExecutor) seedSessionDir(ctx context.Context, req *ExecutorCreateRequest) {
	if req.AgentConfig == nil || r.kandevHomeDir == "" {
		return
	}
	instanceRoot := InstanceSessionRoot(r.kandevHomeDir, req.InstanceID)
	if err := SeedAgentSessionDir(ctx, req.AgentConfig, instanceRoot, r.logger); err != nil {
		r.logger.Warn("failed to seed agent session dir (continuing)",
			zap.String("instance_id", req.InstanceID),
			zap.String("agent_id", req.AgentConfig.ID()),
			zap.Error(err))
	}
}

func (r *DockerExecutor) buildContainerLaunchConfig(req *ExecutorCreateRequest) ContainerConfig {
	return ContainerConfig{
		AgentConfig:       req.AgentConfig,
		WorkspacePath:     "", // Empty = no workspace mount; we clone inside container.
		TaskID:            req.TaskID,
		TaskTitle:         req.TaskTitle,
		TaskEnvironmentID: req.TaskEnvironmentID,
		SessionID:         req.SessionID,
		ExecutorProfileID: getMetadataString(req.Metadata, "executor_profile_id"),
		InstanceID:        req.InstanceID,
		Credentials:       req.Env,
		McpServers:        req.McpServers,
		PrepareScript:     r.resolvePrepareScript(req),
		ImageTagOverride:  getMetadataString(req.Metadata, MetadataKeyImageTagOverride),
		LocalClonePath:    localCloneMountPath(req.Metadata),
	}
}

func (r *DockerExecutor) buildCreatedInstance(req *ExecutorCreateRequest, result *LaunchResult, containerIP string) *ExecutorInstance {
	metadata := map[string]interface{}{
		MetadataKeyIsRemote: true,
	}
	if worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID); worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = dockerWorkspacePath
		metadata["worktree_branch"] = getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)
	}
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
	}
}

// reconnectToContainer attempts to reconnect to an existing Docker container
// from a previous execution. Returns the reconnected instance if successful.
func (r *DockerExecutor) reconnectToContainer(ctx context.Context, dockerClient *docker.Client, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	containerRef, err := resolveReconnectContainerRef(req)
	if err != nil {
		return nil, err
	}
	info, containerIP, err := r.ensureContainerRunning(ctx, dockerClient, containerRef)
	if err != nil {
		return nil, err
	}

	conn, err := r.bringupAgentctl(ctx, dockerClient, info.ID, containerIP, req)
	if err != nil {
		return nil, err
	}

	client := agentctl.NewClient(conn.instanceHost, conn.instancePort, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID),
		agentctl.WithAuthToken(conn.authToken))

	r.logger.Info("reconnected to existing docker container",
		zap.String("container_ref", containerRef),
		zap.String("container_id", info.ID),
		zap.String("container_ip", containerIP),
		zap.String("instance_host", conn.instanceHost),
		zap.Int("instance_port", conn.instancePort),
		zap.Bool("reusing_process", conn.reusingProcess))

	refreshedAuthToken := ""
	if conn.authToken != "" && conn.authToken != req.AuthToken {
		refreshedAuthToken = conn.authToken
	}
	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   string(r.Name()),
		Client:        client,
		ContainerID:   info.ID,
		ContainerIP:   containerIP,
		WorkspacePath: dockerWorkspacePath,
		Metadata: map[string]interface{}{
			MetadataKeyIsRemote:      true,
			MetadataKeyContainerID:   info.ID,
			"reuse_existing_process": conn.reusingProcess,
		},
		AuthToken: refreshedAuthToken,
	}, nil
}

// ensureContainerRunning inspects containerRef, starts it if it's stopped,
// re-inspects, and returns the live container info and IP. Errors out if the
// container can't be brought to a running state — the caller can't reconnect
// to a container that isn't alive.
func (r *DockerExecutor) ensureContainerRunning(ctx context.Context, dockerClient *docker.Client, containerRef string) (*docker.ContainerInfo, string, error) {
	info, err := dockerClient.GetContainerInfo(ctx, containerRef)
	if err != nil {
		return nil, "", fmt.Errorf("failed to inspect container %s: %w", containerRef, err)
	}
	if shouldStartExistingDockerContainer(info.State) {
		r.logger.Info("starting stopped docker container for reconnect",
			zap.String("container_id", info.ID),
			zap.String("state", info.State))
		if err := dockerClient.StartContainer(ctx, info.ID); err != nil {
			return nil, "", fmt.Errorf("failed to start container %s: %w", info.ID, err)
		}
		info, err = dockerClient.GetContainerInfo(ctx, info.ID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to inspect started container %s: %w", containerRef, err)
		}
	}
	if info.State != containerStateRunning {
		return nil, "", fmt.Errorf("container %s is %s, not %s", containerRef, info.State, containerStateRunning)
	}
	containerIP, err := dockerClient.GetContainerIP(ctx, info.ID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get IP for container %s: %w", info.ID, err)
	}
	return info, containerIP, nil
}

// reconnectAgentctlConn captures the resolved endpoint state needed by the
// caller after agentctl has been brought up: the instance-level host:port for
// the user-facing client, the auth token in effect (which may have been
// refreshed via re-handshake), and whether the agent subprocess is reusable.
type reconnectAgentctlConn struct {
	instanceHost   string
	instancePort   int
	authToken      string
	reusingProcess bool
}

// bringupAgentctl health-checks agentctl on the container, finds (or creates)
// the agent instance, transparently re-handshakes on a 401, and returns the
// resolved instance endpoint for the user-facing client.
func (r *DockerExecutor) bringupAgentctl(ctx context.Context, dockerClient *docker.Client, containerID, containerIP string, req *ExecutorCreateRequest) (reconnectAgentctlConn, error) {
	controlHost, controlPort := resolveDockerEndpoint(ctx, dockerClient, containerID, AgentCtlPort, containerIP, r.logger)
	ctl := agentctl.NewControlClient(controlHost, controlPort, r.logger,
		agentctl.WithControlAuthToken(req.AuthToken))
	if err := r.waitForAgentctlHealth(ctx, ctl); err != nil {
		return reconnectAgentctlConn{}, fmt.Errorf("agentctl not healthy in container %s: %w", containerID, err)
	}

	authToken := req.AuthToken
	instanceID := reconnectInstanceID(req, req.PreviousExecutionID)
	instancePort, reusingProcess, err := r.findExistingInstance(ctx, dockerClient, ctl, req, containerID, containerIP, instanceID, authToken)
	if err != nil && req.BootstrapNonce != "" && isAgentctlAuthError(err) {
		var handshakeErr error
		authToken, handshakeErr = ctl.Handshake(ctx, req.BootstrapNonce)
		if handshakeErr != nil {
			return reconnectAgentctlConn{}, fmt.Errorf("agentctl auth failed and re-handshake failed in container %s: %w", containerID, handshakeErr)
		}
		instancePort, reusingProcess, err = r.findExistingInstance(ctx, dockerClient, ctl, req, containerID, containerIP, instanceID, authToken)
	}
	if err != nil {
		return reconnectAgentctlConn{}, fmt.Errorf("failed to find instance in container %s: %w", containerID, err)
	}
	instanceHost, resolvedInstancePort := resolveDockerEndpoint(ctx, dockerClient, containerID, instancePort, containerIP, r.logger)
	return reconnectAgentctlConn{
		instanceHost:   instanceHost,
		instancePort:   resolvedInstancePort,
		authToken:      authToken,
		reusingProcess: reusingProcess,
	}, nil
}

func reconnectInstanceID(req *ExecutorCreateRequest, previousExecutionID string) string {
	if req != nil && strings.TrimSpace(req.InstanceID) != "" {
		return req.InstanceID
	}
	return previousExecutionID
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

// healthChecker is the narrow interface waitForAgentctlHealth needs from the
// agentctl control client. *agentctl.ControlClient satisfies it; tests can
// stub it without spinning up an HTTP server.
type healthChecker interface {
	Health(ctx context.Context) error
}

const (
	agentctlHealthMaxRetries = 240
	agentctlHealthRetryDelay = 500 * time.Millisecond
)

func (r *DockerExecutor) waitForAgentctlHealth(ctx context.Context, ctl healthChecker) error {
	return waitForAgentctlHealthWith(ctx, ctl, agentctlHealthMaxRetries, agentctlHealthRetryDelay)
}

// waitForAgentctlHealthWith is the parameterised body — exposed for tests so
// they can drive the retry loop without a 2-minute wait. The production path
// always calls it with the package constants.
func waitForAgentctlHealthWith(ctx context.Context, ctl healthChecker, maxRetries int, retryDelay time.Duration) error {
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

// hostPortLookup is the narrow interface resolveDockerEndpoint needs from the
// docker client. *docker.Client satisfies it; tests can stub it without
// having to spin up a docker daemon.
type hostPortLookup interface {
	GetContainerHostPort(ctx context.Context, containerID string, containerPort int) (string, int, error)
}

func resolveDockerEndpoint(
	ctx context.Context,
	dockerClient hostPortLookup,
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
	if instance == nil {
		return nil
	}

	// On destructive stop reasons (task/session deleted/archived), clean up
	// the kandev-managed per-container session dir so we don't leak GBs of
	// agent state on disk. Plain stops preserve the dir so resume re-attaches
	// to the same agent state, mirroring the Sprites preserve-on-stop rule.
	if shouldRunExecutorCleanup(instance.StopReason) && r.kandevHomeDir != "" && instance.InstanceID != "" {
		CleanupAgentSessionDir(InstanceSessionRoot(r.kandevHomeDir, instance.InstanceID), r.logger)
	}

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
//
// The kandev-managed feature-branch checkout is appended as an invariant
// postlude — older profiles that snapshot a then-current default lacked it,
// so simply updating DefaultPrepareScript wouldn't reach those users. The
// postlude runs after the user's prepare script and is idempotent.
func (r *DockerExecutor) resolvePrepareScript(req *ExecutorCreateRequest) string {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		script = DefaultPrepareScript("local_docker")
	}
	if script == "" {
		return ""
	}
	script += KandevBranchCheckoutPostlude()

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(dockerWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.WorktreeProvider(
			"",
			dockerWorkspacePath,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			getGitRemoteURL,
			injectGitHubTokenIntoCloneURL,
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

func localCloneMountPath(metadata map[string]interface{}) string {
	return localPathFromCloneURL(getMetadataString(metadata, "repository_clone_url"))
}

func localPathFromCloneURL(raw string) string {
	if raw == "" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "file" {
		return ""
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return ""
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil || !filepath.IsAbs(path) {
		return ""
	}
	return path
}

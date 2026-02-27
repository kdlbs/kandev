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
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

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

	// Extract runtime-specific values from metadata
	mainRepoGitDir := getMetadataString(req.Metadata, MetadataKeyMainRepoGitDir)
	worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID)
	worktreeBranch := getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)

	// Convert ExecutorCreateRequest to ContainerConfig
	containerCfg := ContainerConfig{
		AgentConfig:    req.AgentConfig,
		WorkspacePath:  req.WorkspacePath,
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		InstanceID:     req.InstanceID,
		MainRepoGitDir: mainRepoGitDir,
		Credentials:    req.Env, // Env contains credentials from the caller
		McpServers:     req.McpServers,
	}

	// Use ContainerManager to launch container
	containerID, client, err := containerMgr.LaunchContainer(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	// Get container IP for logging
	containerIP, _ := dockerClient.GetContainerIP(ctx, containerID)

	// Build metadata
	metadata := make(map[string]interface{})
	if worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = req.WorkspacePath
		metadata["worktree_branch"] = worktreeBranch
	}

	r.logger.Info("docker instance created",
		zap.String("instance_id", req.InstanceID),
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP))

	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   string(r.Name()),
		Client:        client,
		ContainerID:   containerID,
		ContainerIP:   containerIP,
		WorkspacePath: "/workspace", // Docker mounts workspace to /workspace
		Metadata:      metadata,
	}, nil
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

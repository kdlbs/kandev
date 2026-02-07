package lifecycle

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// RemoteDockerRuntime implements Runtime for remote Docker-based agent execution.
// Unlike the local Docker runtime which bind-mounts the host workspace, the remote
// Docker runtime clones the repository inside the container since the remote host
// does not have access to the local filesystem.
type RemoteDockerRuntime struct {
	logger *logger.Logger
}

// NewRemoteDockerRuntime creates a new remote Docker runtime.
// Remote docker runtimes are created lazily per docker_host when executor configs are resolved.
func NewRemoteDockerRuntime(log *logger.Logger) *RemoteDockerRuntime {
	return &RemoteDockerRuntime{
		logger: log.WithFields(zap.String("runtime", "remote_docker")),
	}
}

func (r *RemoteDockerRuntime) Name() runtime.Name {
	return runtime.NameRemoteDocker
}

func (r *RemoteDockerRuntime) HealthCheck(ctx context.Context) error {
	// Remote docker health is checked per-host when creating instances.
	// The runtime itself is always "available" as a capability.
	r.logger.Warn("remote_docker runtime is registered but not yet implemented; health check is a no-op")
	return nil
}

func (r *RemoteDockerRuntime) CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error) {
	// TODO: Implement remote docker instance creation.
	// Flow:
	// 1. Extract docker_host, git_token from req.Env or metadata
	// 2. Create Docker client pointing to remote host
	// 3. Create container with named volume for /workspace (no bind mount)
	// 4. Start container, wait for agentctl health
	// 5. Clone repo inside container via Docker exec API
	// 6. Create agentctl instance pointing to /workspace
	// 7. Return RuntimeInstance with container IP + agentctl client
	return nil, fmt.Errorf("remote_docker runtime is not yet implemented")
}

func (r *RemoteDockerRuntime) StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error {
	// TODO: Implement remote docker instance stopping.
	// Connect to the remote docker host and stop/kill the container.
	return fmt.Errorf("remote_docker runtime is not yet implemented")
}

func (r *RemoteDockerRuntime) RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error) {
	// Remote docker instances are not recovered on restart.
	// The containers on remote hosts are ephemeral.
	return nil, nil
}

func (r *RemoteDockerRuntime) GetInteractiveRunner() *process.InteractiveRunner {
	// Remote docker does not support passthrough mode.
	return nil
}

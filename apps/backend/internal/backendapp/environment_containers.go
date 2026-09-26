package backendapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// localContainerOps is the install-wide Docker daemon. It speaks the task
// service's status shape so this file stays clear of the runtime tier.
type localContainerOps interface {
	GetContainerLiveStatus(ctx context.Context, containerID string) (*taskservice.ContainerLiveStatus, error)
	DestroyContainer(ctx context.Context, containerID string) error
}

// remoteContainerOps reaches the daemon named by a remote executor's own
// stored connection.
type remoteContainerOps interface {
	Inspect(ctx context.Context, executorConfig map[string]string, containerID string) (*docker.ContainerInfo, error)
	Remove(ctx context.Context, executorConfig map[string]string, containerID string) error
}

// executorConfigLookup reads the executor row that owns an environment.
type executorConfigLookup interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

// containerOpsDispatch routes a task environment's container operations to the
// daemon that actually owns the container.
//
// A container ID identifies a container only relative to a daemon. Asking the
// install-wide daemon about a container on a remote host does not return "no",
// it returns "no such container", which is indistinguishable from deletion
// unless the caller knows which daemon it asked.
type containerOpsDispatch struct {
	local     localContainerOps
	remote    remoteContainerOps
	executors executorConfigLookup
}

// GetContainerLiveStatus reports the environment's container from its own
// daemon.
func (d *containerOpsDispatch) GetContainerLiveStatus(
	ctx context.Context,
	env *models.TaskEnvironment,
) (*taskservice.ContainerLiveStatus, error) {
	if env == nil || env.ContainerID == "" {
		return nil, nil
	}
	if !isRemoteDockerEnv(env) {
		return d.local.GetContainerLiveStatus(ctx, env.ContainerID)
	}

	cfg, err := d.remoteExecutorConfig(ctx, env)
	if err != nil {
		return nil, err
	}
	info, err := d.remote.Inspect(ctx, cfg, env.ContainerID)
	if err != nil {
		// Only the daemon's own answer means the container is gone. A
		// transport failure is reported as one, so an unreachable host is
		// never mistaken for a deleted container.
		if errors.Is(err, runtime.ErrRemoteContainerMissing) {
			return &taskservice.ContainerLiveStatus{
				ContainerID: env.ContainerID,
				State:       "missing",
				Missing:     true,
			}, nil
		}
		return nil, err
	}
	return fromContainerInfo(env.ContainerID, info), nil
}

// DestroyContainer removes the environment's container from its own daemon.
func (d *containerOpsDispatch) DestroyContainer(ctx context.Context, env *models.TaskEnvironment) error {
	if env == nil || env.ContainerID == "" {
		return nil
	}
	if !isRemoteDockerEnv(env) {
		return d.local.DestroyContainer(ctx, env.ContainerID)
	}
	cfg, err := d.remoteExecutorConfig(ctx, env)
	if err != nil {
		return err
	}
	return d.remote.Remove(ctx, cfg, env.ContainerID)
}

// remoteExecutorConfig reads the connection saved on the owning executor. The
// failure is surfaced rather than falling back to the local daemon: without
// the connection there is no daemon that can answer for this container.
func (d *containerOpsDispatch) remoteExecutorConfig(
	ctx context.Context,
	env *models.TaskEnvironment,
) (map[string]string, error) {
	if d.executors == nil {
		return nil, fmt.Errorf("remote docker: no executor lookup configured")
	}
	exec, err := d.executors.GetExecutor(ctx, env.ExecutorID)
	if err != nil {
		return nil, fmt.Errorf("remote docker: read executor %s: %w", env.ExecutorID, err)
	}
	if exec == nil {
		return nil, fmt.Errorf("remote docker: executor %s not found", env.ExecutorID)
	}
	return exec.Config, nil
}

func isRemoteDockerEnv(env *models.TaskEnvironment) bool {
	return models.ExecutorType(env.ExecutorType) == models.ExecutorTypeRemoteDocker
}

func fromContainerInfo(containerID string, info *docker.ContainerInfo) *taskservice.ContainerLiveStatus {
	if info == nil {
		return nil
	}
	out := &taskservice.ContainerLiveStatus{
		ContainerID: containerID,
		State:       info.State,
		Status:      info.Status,
		ExitCode:    info.ExitCode,
		Health:      info.Health,
	}
	if !info.StartedAt.IsZero() {
		out.StartedAt = info.StartedAt.Format(time.RFC3339)
	}
	if !info.FinishedAt.IsZero() {
		out.FinishedAt = info.FinishedAt.Format(time.RFC3339)
	}
	return out
}

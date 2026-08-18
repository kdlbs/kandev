package backendapp

import (
	"context"
	"errors"
	"os"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
)

const (
	e2eDockerScopeEnv   = "KANDEV_E2E_DOCKER_SCOPE"
	e2eDockerScopeLabel = "kandev.e2e.run"
)

type lazyStorageDocker struct {
	provider func() *agentdocker.Client
	activity *activity.Coordinator
}

func (d *lazyStorageDocker) client() (*agentdocker.Client, error) {
	if d.provider == nil {
		return nil, errors.New("docker client is not configured")
	}
	client := d.provider()
	if client == nil {
		return nil, errors.New("docker client is unavailable")
	}
	client.SetActivityCoordinator(d.activity)
	return client, nil
}

func (d *lazyStorageDocker) Ping(ctx context.Context) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	return client.Ping(ctx)
}

func (d *lazyStorageDocker) ListContainers(
	ctx context.Context,
	labels map[string]string,
) ([]agentdocker.ContainerInfo, error) {
	client, err := d.client()
	if err != nil {
		return nil, err
	}
	return client.ListContainers(ctx, scopedDockerLabels(labels))
}

func (d *lazyStorageDocker) RemoveContainer(ctx context.Context, id string, force bool) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	return client.RemoveContainer(ctx, id, force)
}

func (d *lazyStorageDocker) DiskUsage(ctx context.Context) (agentdocker.DiskUsage, error) {
	client, err := d.client()
	if err != nil {
		return agentdocker.DiskUsage{}, err
	}
	usage, err := client.DiskUsage(ctx)
	if err != nil {
		return agentdocker.DiskUsage{}, err
	}
	return scopeDockerUsage(usage), nil
}

func scopedDockerLabels(labels map[string]string) map[string]string {
	scope := os.Getenv(e2eDockerScopeEnv)
	if scope == "" {
		return labels
	}
	scoped := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		scoped[key] = value
	}
	scoped[e2eDockerScopeLabel] = scope
	return scoped
}

func scopeDockerUsage(usage agentdocker.DiskUsage) agentdocker.DiskUsage {
	scope := os.Getenv(e2eDockerScopeEnv)
	if scope == "" {
		return usage
	}
	containers := make([]agentdocker.ContainerUsage, 0, len(usage.Containers))
	for _, container := range usage.Containers {
		if container.Labels[e2eDockerScopeLabel] == scope {
			containers = append(containers, container)
		}
	}
	usage.Containers = containers
	return usage
}

func (d *lazyStorageDocker) PruneBuildCache(
	ctx context.Context,
	options agentdocker.BuildCachePruneOptions,
) (agentdocker.PruneResult, error) {
	client, err := d.client()
	if err != nil {
		return agentdocker.PruneResult{}, err
	}
	return client.PruneBuildCache(ctx, options)
}

func (d *lazyStorageDocker) PruneUnusedImages(
	ctx context.Context,
	cutoff time.Time,
) (agentdocker.PruneResult, error) {
	client, err := d.client()
	if err != nil {
		return agentdocker.PruneResult{}, err
	}
	return client.PruneUnusedImages(ctx, cutoff)
}

var _ dockerstore.DockerClient = (*lazyStorageDocker)(nil)

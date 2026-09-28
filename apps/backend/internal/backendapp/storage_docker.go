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

// storageNetworksClient is the docknet provider's typed client surface.
type storageNetworksClient interface {
	Ping(context.Context) error
	ListNetworks(context.Context, agentdocker.NetworkListOptions) ([]agentdocker.NetworkInfo, error)
	InspectNetwork(context.Context, string) (agentdocker.NetworkInfo, error)
	RemoveNetwork(context.Context, string) error
	CreateNetwork(context.Context, agentdocker.CreateNetworkOptions) (agentdocker.NetworkInfo, error)
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

func (d *lazyStorageDocker) ListNetworks(
	ctx context.Context,
	options agentdocker.NetworkListOptions,
) ([]agentdocker.NetworkInfo, error) {
	client, err := d.client()
	if err != nil {
		return nil, err
	}
	return client.ListNetworks(ctx, scopeDockerNetworkOptions(options))
}

func (d *lazyStorageDocker) InspectNetwork(ctx context.Context, id string) (agentdocker.NetworkInfo, error) {
	client, err := d.client()
	if err != nil {
		return agentdocker.NetworkInfo{}, err
	}
	network, err := client.InspectNetwork(ctx, id)
	if err != nil {
		return agentdocker.NetworkInfo{}, err
	}
	return scopeDockerNetworkDetail(network), nil
}

func (d *lazyStorageDocker) RemoveNetwork(ctx context.Context, id string) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	return client.RemoveNetwork(ctx, id)
}

func (d *lazyStorageDocker) CreateNetwork(
	ctx context.Context,
	options agentdocker.CreateNetworkOptions,
) (agentdocker.NetworkInfo, error) {
	client, err := d.client()
	if err != nil {
		return agentdocker.NetworkInfo{}, err
	}
	return client.CreateNetwork(ctx, options)
}

// scopeDockerNetworkOptions keeps the e2e containers project isolated: when
// the scope env is set, every census list filters to e2e-scoped networks so a
// test daemon shared with the host never sees foreign networks.
func scopeDockerNetworkOptions(options agentdocker.NetworkListOptions) agentdocker.NetworkListOptions {
	scope := os.Getenv(e2eDockerScopeEnv)
	if scope == "" {
		return options
	}
	options.Labels = append(options.Labels, e2eDockerScopeLabel+"="+scope)
	return options
}

// scopeDockerNetworkDetail hides non-scoped networks from the e2e provider by
// making them textually unresolvable; the classifier then excludes them from
// any destructive action.
func scopeDockerNetworkDetail(network agentdocker.NetworkInfo) agentdocker.NetworkInfo {
	scope := os.Getenv(e2eDockerScopeEnv)
	if scope == "" || network.Labels[e2eDockerScopeLabel] == scope {
		return network
	}
	network.Containers = map[string]string{"e2e-out-of-scope": "foreign"}
	return network
}

var _ dockerstore.DockerClient = (*lazyStorageDocker)(nil)
var _ storageNetworksClient = (*lazyStorageDocker)(nil)

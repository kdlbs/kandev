package dockerstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/system/storage"
)

var (
	ErrDockerUnavailable     = errors.New("docker client is unavailable")
	ErrGlobalCleanupDisabled = errors.New("global Docker cleanup is disabled")
)

type DockerClient interface {
	Ping(context.Context) error
	ListContainers(context.Context, map[string]string) ([]agentdocker.ContainerInfo, error)
	RemoveContainer(context.Context, string, bool) error
	DiskUsage(context.Context) (agentdocker.DiskUsage, error)
	PruneBuildCache(context.Context, agentdocker.BuildCachePruneOptions) (agentdocker.PruneResult, error)
	PruneUnusedImages(context.Context, time.Time) (agentdocker.PruneResult, error)
}

type TaskInventory interface {
	ContainerTaskRemovable(context.Context, string) (bool, error)
}

type SettingsReader interface {
	GetSettings(context.Context) (storage.StorageMaintenanceSettings, error)
}

type Result struct {
	Available bool     `json:"available"`
	Removed   int      `json:"removed"`
	Kept      int      `json:"kept"`
	Warnings  []string `json:"warnings"`
}

type Analysis struct {
	Available             bool     `json:"available"`
	ManagedContainerCount int      `json:"managed_container_count"`
	ManagedContainerBytes int64    `json:"managed_container_bytes"`
	ImageLayerBytes       int64    `json:"image_layer_bytes"`
	BuildCacheBytes       int64    `json:"build_cache_bytes"`
	UnusedImageBytes      int64    `json:"unused_image_bytes"`
	Warnings              []string `json:"warnings"`
}

type Provider struct {
	docker    DockerClient
	inventory TaskInventory
	settings  SettingsReader
	now       func() time.Time
}

func NewProvider(docker DockerClient, inventory TaskInventory, settings SettingsReader) *Provider {
	return &Provider{docker: docker, inventory: inventory, settings: settings, now: time.Now}
}

func (p *Provider) CleanupContainers(ctx context.Context) Result {
	if warning := p.availabilityWarning(ctx); warning != "" {
		return Result{Warnings: []string{warning}}
	}
	containers, err := p.docker.ListContainers(ctx, map[string]string{"kandev.managed": "true"})
	if err != nil {
		return Result{Warnings: []string{fmt.Sprintf("list Docker containers: %v", err)}}
	}
	result := Result{Available: true}
	for _, candidate := range containers {
		p.cleanupContainer(ctx, candidate, &result)
	}
	return result
}

func (p *Provider) Analyze(ctx context.Context) Analysis {
	if warning := p.availabilityWarning(ctx); warning != "" {
		return Analysis{Warnings: []string{warning}}
	}
	usage, err := p.docker.DiskUsage(ctx)
	if err != nil {
		return Analysis{Warnings: []string{fmt.Sprintf("read Docker disk usage: %v", err)}}
	}
	settings, err := p.settings.GetSettings(ctx)
	if err != nil {
		return Analysis{Warnings: []string{fmt.Sprintf("read storage settings: %v", err)}}
	}
	settings, err = storage.NormalizeSettings(settings)
	if err != nil {
		return Analysis{Warnings: []string{fmt.Sprintf("validate storage settings: %v", err)}}
	}
	cutoff := p.now().Add(-time.Duration(settings.Docker.UnusedImagesHours) * time.Hour)
	managedCount, managedBytes := managedContainerUsage(usage.Containers)
	return Analysis{
		Available:             true,
		ManagedContainerCount: managedCount,
		ManagedContainerBytes: managedBytes,
		ImageLayerBytes:       usage.ImageLayerBytes,
		BuildCacheBytes:       usage.BuildCacheBytes,
		UnusedImageBytes:      unusedImageBytes(usage.Images, cutoff),
	}
}

func (p *Provider) PruneBuildCache(ctx context.Context) (agentdocker.PruneResult, error) {
	if p.docker == nil {
		return agentdocker.PruneResult{}, ErrDockerUnavailable
	}
	settings, err := p.settings.GetSettings(ctx)
	if err != nil {
		return agentdocker.PruneResult{}, fmt.Errorf("read storage settings: %w", err)
	}
	settings, err = storage.NormalizeSettings(settings)
	if err != nil {
		return agentdocker.PruneResult{}, fmt.Errorf("validate storage settings: %w", err)
	}
	if !settings.Docker.DedicatedDaemonAcknowledged || !settings.Docker.BuildCacheEnabled {
		return agentdocker.PruneResult{}, ErrGlobalCleanupDisabled
	}
	return p.docker.PruneBuildCache(ctx, agentdocker.BuildCachePruneOptions{
		KeepBytes: settings.Docker.BuildCacheKeepBytes,
		UnusedBefore: p.now().Add(
			-time.Duration(settings.Docker.BuildCacheUnusedHours) * time.Hour,
		),
	})
}

func (p *Provider) PruneUnusedImages(ctx context.Context) (agentdocker.PruneResult, error) {
	if p.docker == nil {
		return agentdocker.PruneResult{}, ErrDockerUnavailable
	}
	settings, err := p.settings.GetSettings(ctx)
	if err != nil {
		return agentdocker.PruneResult{}, fmt.Errorf("read storage settings: %w", err)
	}
	settings, err = storage.NormalizeSettings(settings)
	if err != nil {
		return agentdocker.PruneResult{}, fmt.Errorf("validate storage settings: %w", err)
	}
	if !settings.Docker.DedicatedDaemonAcknowledged || !settings.Docker.UnusedImagesEnabled {
		return agentdocker.PruneResult{}, ErrGlobalCleanupDisabled
	}
	cutoff := p.now().Add(-time.Duration(settings.Docker.UnusedImagesHours) * time.Hour)
	return p.docker.PruneUnusedImages(ctx, cutoff)
}

func (p *Provider) availabilityWarning(ctx context.Context) string {
	if p.docker == nil {
		return "Docker unavailable: client not configured"
	}
	if err := p.docker.Ping(ctx); err != nil {
		return fmt.Sprintf("Docker unavailable: %v", err)
	}
	return ""
}

func (p *Provider) cleanupContainer(ctx context.Context, candidate agentdocker.ContainerInfo, result *Result) {
	if candidate.Labels["kandev.managed"] != "true" || !stoppedContainer(candidate.State) {
		result.Kept++
		return
	}
	taskID := candidate.Labels["kandev.task_id"]
	if taskID == "" {
		result.Kept++
		return
	}
	removable, err := p.inventory.ContainerTaskRemovable(ctx, taskID)
	if err != nil {
		result.Kept++
		result.Warnings = append(result.Warnings, fmt.Sprintf("classify container %s: %v", candidate.ID, err))
		return
	}
	if !removable {
		result.Kept++
		return
	}
	if err := p.docker.RemoveContainer(ctx, candidate.ID, false); err != nil {
		result.Kept++
		result.Warnings = append(result.Warnings, fmt.Sprintf("remove container %s: %v", candidate.ID, err))
		return
	}
	result.Removed++
}

func stoppedContainer(state string) bool {
	switch state {
	case "created", "exited", "dead":
		return true
	default:
		return false
	}
}

func unusedImageBytes(images []agentdocker.ImageUsage, cutoff time.Time) int64 {
	var total int64
	for _, image := range images {
		if image.Containers == 0 && image.CreatedAt.Before(cutoff) && image.SizeBytes > 0 {
			total += image.SizeBytes
		}
	}
	return total
}

func managedContainerUsage(containers []agentdocker.ContainerUsage) (int, int64) {
	var count int
	var bytes int64
	for _, item := range containers {
		if item.Labels["kandev.managed"] != "true" {
			continue
		}
		count++
		if item.WritableBytes > 0 {
			bytes += item.WritableBytes
		}
	}
	return count, bytes
}

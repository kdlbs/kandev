package docker

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/moby/moby/client"
)

type storageAPI interface {
	DiskUsage(context.Context, client.DiskUsageOptions) (client.DiskUsageResult, error)
	BuildCachePrune(context.Context, client.BuildCachePruneOptions) (client.BuildCachePruneResult, error)
	ImagePrune(context.Context, client.ImagePruneOptions) (client.ImagePruneResult, error)
}

type ImageUsage struct {
	ID         string    `json:"id"`
	SizeBytes  int64     `json:"size_bytes"`
	Containers int64     `json:"containers"`
	CreatedAt  time.Time `json:"created_at"`
}

type ContainerUsage struct {
	ID            string            `json:"id"`
	WritableBytes int64             `json:"writable_bytes"`
	Labels        map[string]string `json:"labels"`
}

type DiskUsage struct {
	ImageLayerBytes int64            `json:"image_layer_bytes"`
	BuildCacheBytes int64            `json:"build_cache_bytes"`
	Images          []ImageUsage     `json:"images"`
	Containers      []ContainerUsage `json:"containers"`
}

type BuildCachePruneOptions struct {
	KeepBytes    int64
	UnusedBefore time.Time
}

type PruneResult struct {
	Deleted        int   `json:"deleted"`
	BytesReclaimed int64 `json:"bytes_reclaimed"`
}

func (c *Client) DiskUsage(ctx context.Context) (DiskUsage, error) {
	// Verbose is required for the daemon to return per-image and per-container
	// records alongside the aggregate totals.
	usage, err := c.storageClient().DiskUsage(ctx, client.DiskUsageOptions{
		Containers: true,
		Images:     true,
		BuildCache: true,
		Verbose:    true,
	})
	if err != nil {
		return DiskUsage{}, fmt.Errorf("read Docker disk usage: %w", err)
	}
	result := DiskUsage{
		ImageLayerBytes: usage.Images.TotalSize,
		Images:          make([]ImageUsage, 0, len(usage.Images.Items)),
		Containers:      make([]ContainerUsage, 0, len(usage.Containers.Items)),
	}
	for _, cache := range usage.BuildCache.Items {
		if cache.Size > 0 {
			result.BuildCacheBytes += cache.Size
		}
	}
	for _, item := range usage.Images.Items {
		result.Images = append(result.Images, ImageUsage{
			ID: item.ID, SizeBytes: item.Size, Containers: item.Containers,
			CreatedAt: time.Unix(item.Created, 0).UTC(),
		})
	}
	for _, item := range usage.Containers.Items {
		result.Containers = append(result.Containers, ContainerUsage{
			ID: item.ID, WritableBytes: item.SizeRw, Labels: item.Labels,
		})
	}
	return result, nil
}

func (c *Client) PruneBuildCache(ctx context.Context, options BuildCachePruneOptions) (PruneResult, error) {
	// The client translates ReservedSpace into the legacy "keep-storage"
	// parameter when talking to daemons on API <= 1.47.
	result, err := c.storageClient().BuildCachePrune(ctx, client.BuildCachePruneOptions{
		All:           true,
		ReservedSpace: options.KeepBytes,
		Filters:       make(client.Filters).Add("until", unixFilter(options.UnusedBefore)),
	})
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune Docker build cache: %w", err)
	}
	return PruneResult{
		Deleted:        len(result.Report.CachesDeleted),
		BytesReclaimed: reclaimedBytes(result.Report.SpaceReclaimed),
	}, nil
}

func (c *Client) PruneUnusedImages(ctx context.Context, unusedBefore time.Time) (PruneResult, error) {
	result, err := c.storageClient().ImagePrune(ctx, client.ImagePruneOptions{
		Filters: make(client.Filters).
			Add("dangling", "false").
			Add("until", unixFilter(unusedBefore)),
	})
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune unused Docker images: %w", err)
	}
	return PruneResult{
		Deleted:        len(result.Report.ImagesDeleted),
		BytesReclaimed: reclaimedBytes(result.Report.SpaceReclaimed),
	}, nil
}

func (c *Client) storageClient() storageAPI {
	if c.storage != nil {
		return c.storage
	}
	return c.cli
}

func unixFilter(value time.Time) string {
	return strconv.FormatInt(value.UTC().Unix(), 10)
}

func reclaimedBytes(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

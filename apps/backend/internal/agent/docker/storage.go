package docker

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

type storageAPI interface {
	DiskUsage(context.Context, dockertypes.DiskUsageOptions) (dockertypes.DiskUsage, error)
	BuildCachePrune(context.Context, build.CachePruneOptions) (*build.CachePruneReport, error)
	ImagesPrune(context.Context, filters.Args) (image.PruneReport, error)
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
	usage, err := c.storageClient().DiskUsage(ctx, dockertypes.DiskUsageOptions{
		Types: []dockertypes.DiskUsageObject{
			dockertypes.ContainerObject,
			dockertypes.ImageObject,
			dockertypes.BuildCacheObject,
		},
	})
	if err != nil {
		return DiskUsage{}, fmt.Errorf("read Docker disk usage: %w", err)
	}
	result := DiskUsage{
		ImageLayerBytes: usage.LayersSize,
		Images:          make([]ImageUsage, 0, len(usage.Images)),
		Containers:      make([]ContainerUsage, 0, len(usage.Containers)),
	}
	for _, cache := range usage.BuildCache {
		if cache != nil && cache.Size > 0 {
			result.BuildCacheBytes += cache.Size
		}
	}
	for _, item := range usage.Images {
		if item == nil {
			continue
		}
		result.Images = append(result.Images, ImageUsage{
			ID: item.ID, SizeBytes: item.Size, Containers: item.Containers,
			CreatedAt: time.Unix(item.Created, 0).UTC(),
		})
	}
	for _, item := range usage.Containers {
		if item == nil {
			continue
		}
		result.Containers = append(result.Containers, ContainerUsage{
			ID: item.ID, WritableBytes: item.SizeRw, Labels: item.Labels,
		})
	}
	return result, nil
}

func (c *Client) PruneBuildCache(ctx context.Context, options BuildCachePruneOptions) (PruneResult, error) {
	pruneFilters := filters.NewArgs(filters.Arg("until", unixFilter(options.UnusedBefore)))
	report, err := c.storageClient().BuildCachePrune(ctx, build.CachePruneOptions{
		All: true, ReservedSpace: options.KeepBytes, KeepStorage: options.KeepBytes, Filters: pruneFilters,
	})
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune Docker build cache: %w", err)
	}
	if report == nil {
		return PruneResult{}, nil
	}
	return PruneResult{
		Deleted: len(report.CachesDeleted), BytesReclaimed: reclaimedBytes(report.SpaceReclaimed),
	}, nil
}

func (c *Client) PruneUnusedImages(ctx context.Context, unusedBefore time.Time) (PruneResult, error) {
	pruneFilters := filters.NewArgs(
		filters.Arg("dangling", "false"),
		filters.Arg("until", unixFilter(unusedBefore)),
	)
	report, err := c.storageClient().ImagesPrune(ctx, pruneFilters)
	if err != nil {
		return PruneResult{}, fmt.Errorf("prune unused Docker images: %w", err)
	}
	return PruneResult{
		Deleted: len(report.ImagesDeleted), BytesReclaimed: reclaimedBytes(report.SpaceReclaimed),
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

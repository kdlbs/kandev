package docker

import (
	"context"
	"encoding/json"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

func TestStorageResultJSONUsesSnakeCase(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "image usage", value: ImageUsage{
				ID: "image", SizeBytes: 10, Containers: 2, CreatedAt: time.Unix(0, 0).UTC(),
			},
			want: `{"id":"image","size_bytes":10,"containers":2,"created_at":"1970-01-01T00:00:00Z"}`,
		},
		{
			name: "container usage",
			value: ContainerUsage{
				ID: "container", WritableBytes: 20, Labels: map[string]string{"managed": "true"},
			},
			want: `{"id":"container","writable_bytes":20,"labels":{"managed":"true"}}`,
		},
		{
			name: "disk usage",
			value: DiskUsage{
				BuildCacheBytes: 30, Images: []ImageUsage{}, Containers: []ContainerUsage{},
			},
			want: `{"image_layer_bytes":0,"build_cache_bytes":30,"images":[],"containers":[]}`,
		},
		{
			name: "prune result", value: PruneResult{Deleted: 3, BytesReclaimed: 40},
			want: `{"deleted":3,"bytes_reclaimed":40}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Fatalf("JSON = %s, want %s", encoded, tt.want)
			}
		})
	}
}

func TestDiskUsageMapsTypedSDKResponse(t *testing.T) {
	created := time.Unix(100, 0)
	lastUsed := time.Unix(200, 0)
	sdk := &fakeStorageAPI{usage: client.DiskUsageResult{
		Images: client.ImagesDiskUsage{
			TotalSize: 500,
			Items:     []image.Summary{{ID: "image-1", Size: 300, Containers: 0, Created: created.Unix()}},
		},
		Containers: client.ContainersDiskUsage{
			Items: []container.Summary{
				{ID: "managed", SizeRw: 75, Labels: map[string]string{"kandev.managed": "true"}},
				{ID: "unrelated", SizeRw: 125, Labels: map[string]string{"owner": "user"}},
			},
		},
		BuildCache: client.BuildCacheDiskUsage{
			Items: []build.CacheRecord{
				{ID: "cache-1", Size: 100},
				{ID: "cache-2", Size: 200, LastUsedAt: &lastUsed},
			},
		},
	}}
	dockerClient := &Client{storage: sdk}

	got, err := dockerClient.DiskUsage(context.Background())
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	if got.ImageLayerBytes != 500 || got.BuildCacheBytes != 300 ||
		len(got.Images) != 1 || !got.Images[0].CreatedAt.Equal(created) ||
		len(got.Containers) != 2 || got.Containers[0].WritableBytes != 75 ||
		got.Containers[0].Labels["kandev.managed"] != "true" {
		t.Fatalf("disk usage = %#v", got)
	}
	// Verbose is what makes the daemon return the per-object Items above.
	wantOptions := client.DiskUsageOptions{
		Containers: true, Images: true, BuildCache: true, Verbose: true,
	}
	if !reflect.DeepEqual(sdk.usageOptions, wantOptions) {
		t.Fatalf("disk usage options = %#v, want %#v", sdk.usageOptions, wantOptions)
	}
}

func TestPruneBuildCacheUsesAgeAndReservedSpaceFilters(t *testing.T) {
	sdk := &fakeStorageAPI{buildResult: client.BuildCachePruneResult{
		Report: build.CachePruneReport{CachesDeleted: []string{"one"}, SpaceReclaimed: 42},
	}}
	dockerClient := &Client{storage: sdk}
	cutoff := time.Unix(1234, 0).UTC()

	got, err := dockerClient.PruneBuildCache(context.Background(), BuildCachePruneOptions{
		KeepBytes: 1024, UnusedBefore: cutoff,
	})
	if err != nil {
		t.Fatalf("PruneBuildCache: %v", err)
	}
	if got.Deleted != 1 || got.BytesReclaimed != 42 {
		t.Fatalf("prune result = %#v", got)
	}
	if !sdk.buildOptions.All || sdk.buildOptions.ReservedSpace != 1024 ||
		!sdk.buildOptions.Filters["until"]["1234"] {
		t.Fatalf("build prune options = %#v", sdk.buildOptions)
	}
}

func TestPruneUnusedImagesUsesAllUnusedAndAgeFilters(t *testing.T) {
	sdk := &fakeStorageAPI{imageResult: client.ImagePruneResult{
		Report: image.PruneReport{
			ImagesDeleted: []image.DeleteResponse{{Deleted: "image-1"}}, SpaceReclaimed: 84,
		},
	}}
	dockerClient := &Client{storage: sdk}

	got, err := dockerClient.PruneUnusedImages(context.Background(), time.Unix(5678, 0).UTC())
	if err != nil {
		t.Fatalf("PruneUnusedImages: %v", err)
	}
	if got.Deleted != 1 || got.BytesReclaimed != 84 {
		t.Fatalf("prune result = %#v", got)
	}
	if !sdk.imageOptions.Filters["dangling"]["false"] ||
		!sdk.imageOptions.Filters["until"]["5678"] {
		t.Fatalf("image prune filters = %#v", sdk.imageOptions.Filters)
	}
}

func TestRemoveContainerRemovesAttachedVolumesWithoutGlobalPrune(t *testing.T) {
	remover := &fakeContainerRemover{}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	dockerClient := &Client{remover: remover, logger: log}

	if err := dockerClient.RemoveContainer(context.Background(), "container-1", false); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}
	if remover.id != "container-1" || remover.options.Force || !remover.options.RemoveVolumes {
		t.Fatalf("remove call id=%q options=%#v", remover.id, remover.options)
	}
}

func TestListNetworksAppliesOwnershipFilters(t *testing.T) {
	dangling := true
	sdk := &fakeNetworkAPI{listResult: client.NetworkListResult{Items: []network.Summary{{
		Network: network.Network{
			ID:     "network-1",
			Name:   "kd_task_default",
			Driver: "bridge",
			Scope:  "local",
			Labels: map[string]string{"com.docker.compose.project": "kd_task"},
		},
	}}}}
	dockerClient := &Client{networks: sdk}

	got, err := dockerClient.ListNetworks(context.Background(), NetworkListOptions{
		Driver:   "bridge",
		Labels:   []string{"com.docker.compose.project=kd_task"},
		Dangling: &dangling,
	})
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(got) != 1 || got[0].ID != "network-1" || got[0].Labels["com.docker.compose.project"] != "kd_task" {
		t.Fatalf("networks = %#v, want mapped network summary", got)
	}
	if !sdk.listOptions.Filters["driver"]["bridge"] ||
		!sdk.listOptions.Filters["label"]["com.docker.compose.project=kd_task"] ||
		!sdk.listOptions.Filters["dangling"]["true"] {
		t.Fatalf("network filters = %#v", sdk.listOptions.Filters)
	}
}

func TestRemoveNetworkUsesNetworkID(t *testing.T) {
	sdk := &fakeNetworkAPI{}
	dockerClient := &Client{networks: sdk}

	if err := dockerClient.RemoveNetwork(context.Background(), "network-1"); err != nil {
		t.Fatalf("RemoveNetwork: %v", err)
	}
	if sdk.removedID != "network-1" {
		t.Fatalf("removed network ID = %q, want network-1", sdk.removedID)
	}
}

func TestInspectNetworkMapsAttachmentsAndSubnets(t *testing.T) {
	subnet := netip.MustParsePrefix("172.19.0.0/16")
	sdk := &fakeNetworkAPI{
		inspectResult: client.NetworkInspectResult{Network: network.Inspect{
			Network: network.Network{
				ID: "network-1", Name: "kd_task_default", Driver: "bridge", Scope: "local",
				Labels: map[string]string{"com.docker.compose.project": "kd_task"},
				IPAM:   network.IPAM{Config: []network.IPAMConfig{{Subnet: subnet}}},
			},
			Containers: map[string]network.EndpointResource{
				"container-1": {Name: "container_1"},
			},
		}},
	}
	dockerClient := &Client{networks: sdk}

	got, err := dockerClient.InspectNetwork(context.Background(), "network-1")
	if err != nil {
		t.Fatalf("InspectNetwork: %v", err)
	}
	if len(got.IPv4Subnets) != 1 || got.IPv4Subnets[0] != subnet {
		t.Fatalf("subnet = %v, want [%s]", got.IPv4Subnets, subnet)
	}
	if len(got.Containers) != 1 || got.Containers["container-1"] != "container_1" {
		t.Fatalf("containers = %#v, want mapped endpoint", got.Containers)
	}
	if sdk.inspectedID != "network-1" {
		t.Fatalf("inspected network ID = %q, want network-1", sdk.inspectedID)
	}
}

func TestCreateNetworkCreatesBridgeAndInspectsResult(t *testing.T) {
	sdk := &fakeNetworkAPI{
		createResult: client.NetworkCreateResult{ID: "network-1"},
		inspectResult: client.NetworkInspectResult{Network: network.Inspect{
			Network: network.Network{
				ID: "network-1", Name: "kd_probe", Driver: "bridge",
				IPAM: network.IPAM{Config: []network.IPAMConfig{
					{Subnet: netip.MustParsePrefix("172.20.0.0/16")},
				}},
			},
		}},
	}
	dockerClient := &Client{networks: sdk}

	got, err := dockerClient.CreateNetwork(context.Background(), CreateNetworkOptions{
		Name: "kd_probe", Labels: map[string]string{"kandev.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if got.ID != "network-1" || len(got.IPv4Subnets) != 1 {
		t.Fatalf("created network = %#v, want inspected subnet detail", got)
	}
	if sdk.createName != "kd_probe" || sdk.createOptions.Driver != "bridge" ||
		sdk.createOptions.EnableIPv4 == nil || !*sdk.createOptions.EnableIPv4 ||
		sdk.createOptions.Labels["kandev.managed"] != "true" {
		t.Fatalf("create call name=%q options=%#v", sdk.createName, sdk.createOptions)
	}
}

type fakeStorageAPI struct {
	usage        client.DiskUsageResult
	usageErr     error
	usageOptions client.DiskUsageOptions
	buildOptions client.BuildCachePruneOptions
	buildResult  client.BuildCachePruneResult
	buildErr     error
	imageOptions client.ImagePruneOptions
	imageResult  client.ImagePruneResult
	imageErr     error
}

func (f *fakeStorageAPI) DiskUsage(_ context.Context, options client.DiskUsageOptions) (client.DiskUsageResult, error) {
	f.usageOptions = options
	return f.usage, f.usageErr
}

func (f *fakeStorageAPI) BuildCachePrune(_ context.Context, options client.BuildCachePruneOptions) (client.BuildCachePruneResult, error) {
	f.buildOptions = options
	return f.buildResult, f.buildErr
}

func (f *fakeStorageAPI) ImagePrune(_ context.Context, options client.ImagePruneOptions) (client.ImagePruneResult, error) {
	f.imageOptions = options
	return f.imageResult, f.imageErr
}

type fakeContainerRemover struct {
	id      string
	options client.ContainerRemoveOptions
}

type fakeNetworkAPI struct {
	listOptions   client.NetworkListOptions
	listResult    client.NetworkListResult
	listErr       error
	inspectedID   string
	inspectResult client.NetworkInspectResult
	inspectErr    error
	removedID     string
	removeErr     error
	createName    string
	createOptions client.NetworkCreateOptions
	createResult  client.NetworkCreateResult
	createErr     error
}

func (f *fakeNetworkAPI) NetworkList(_ context.Context, options client.NetworkListOptions) (client.NetworkListResult, error) {
	f.listOptions = options
	return f.listResult, f.listErr
}

func (f *fakeNetworkAPI) NetworkInspect(_ context.Context, id string, _ client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	f.inspectedID = id
	return f.inspectResult, f.inspectErr
}

func (f *fakeNetworkAPI) NetworkRemove(_ context.Context, id string, _ client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	f.removedID = id
	return client.NetworkRemoveResult{}, f.removeErr
}

func (f *fakeNetworkAPI) NetworkCreate(_ context.Context, name string, options client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
	f.createName = name
	f.createOptions = options
	return f.createResult, f.createErr
}

func (f *fakeContainerRemover) ContainerRemove(
	_ context.Context, id string, options client.ContainerRemoveOptions,
) (client.ContainerRemoveResult, error) {
	f.id = id
	f.options = options
	return client.ContainerRemoveResult{}, nil
}

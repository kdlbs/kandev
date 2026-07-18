package docker

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/kandev/kandev/internal/common/logger"
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
			want: `{"build_cache_bytes":30,"images":[],"containers":[]}`,
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
	sdk := &fakeStorageAPI{usage: dockertypes.DiskUsage{
		Images: []*image.Summary{{ID: "image-1", Size: 300, Containers: 0, Created: created.Unix()}},
		Containers: []*container.Summary{
			{ID: "managed", SizeRw: 75, Labels: map[string]string{"kandev.managed": "true"}},
			{ID: "unrelated", SizeRw: 125, Labels: map[string]string{"owner": "user"}},
		},
		BuildCache: []*build.CacheRecord{
			{ID: "cache-1", Size: 100},
			{ID: "cache-2", Size: 200, LastUsedAt: &lastUsed},
		},
	}}
	client := &Client{storage: sdk}

	got, err := client.DiskUsage(context.Background())
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	if got.BuildCacheBytes != 300 || len(got.Images) != 1 || !got.Images[0].CreatedAt.Equal(created) ||
		len(got.Containers) != 2 || got.Containers[0].WritableBytes != 75 ||
		got.Containers[0].Labels["kandev.managed"] != "true" {
		t.Fatalf("disk usage = %#v", got)
	}
	wantTypes := []dockertypes.DiskUsageObject{
		dockertypes.ContainerObject, dockertypes.ImageObject, dockertypes.BuildCacheObject,
	}
	if !reflect.DeepEqual(sdk.usageOptions.Types, wantTypes) {
		t.Fatalf("disk usage types = %#v, want %#v", sdk.usageOptions.Types, wantTypes)
	}
}

func TestPruneBuildCacheUsesAgeAndReservedSpaceFilters(t *testing.T) {
	sdk := &fakeStorageAPI{buildReport: &build.CachePruneReport{CachesDeleted: []string{"one"}, SpaceReclaimed: 42}}
	client := &Client{storage: sdk}
	cutoff := time.Unix(1234, 0).UTC()

	got, err := client.PruneBuildCache(context.Background(), BuildCachePruneOptions{
		KeepBytes: 1024, UnusedBefore: cutoff,
	})
	if err != nil {
		t.Fatalf("PruneBuildCache: %v", err)
	}
	if got.Deleted != 1 || got.BytesReclaimed != 42 {
		t.Fatalf("prune result = %#v", got)
	}
	if !sdk.buildOptions.All || sdk.buildOptions.ReservedSpace != 1024 ||
		sdk.buildOptions.KeepStorage != 1024 ||
		!sdk.buildOptions.Filters.ExactMatch("until", "1234") {
		t.Fatalf("build prune options = %#v", sdk.buildOptions)
	}
}

func TestPruneUnusedImagesUsesAllUnusedAndAgeFilters(t *testing.T) {
	sdk := &fakeStorageAPI{imageReport: image.PruneReport{
		ImagesDeleted: []image.DeleteResponse{{Deleted: "image-1"}}, SpaceReclaimed: 84,
	}}
	client := &Client{storage: sdk}

	got, err := client.PruneUnusedImages(context.Background(), time.Unix(5678, 0).UTC())
	if err != nil {
		t.Fatalf("PruneUnusedImages: %v", err)
	}
	if got.Deleted != 1 || got.BytesReclaimed != 84 {
		t.Fatalf("prune result = %#v", got)
	}
	if !sdk.imageFilters.ExactMatch("dangling", "false") || !sdk.imageFilters.ExactMatch("until", "5678") {
		t.Fatalf("image prune filters = %#v", sdk.imageFilters)
	}
}

func TestRemoveContainerRemovesAttachedVolumesWithoutGlobalPrune(t *testing.T) {
	remover := &fakeContainerRemover{}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	client := &Client{remover: remover, logger: log}

	if err := client.RemoveContainer(context.Background(), "container-1", false); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}
	if remover.id != "container-1" || remover.options.Force || !remover.options.RemoveVolumes {
		t.Fatalf("remove call id=%q options=%#v", remover.id, remover.options)
	}
}

type fakeStorageAPI struct {
	usage        dockertypes.DiskUsage
	usageErr     error
	usageOptions dockertypes.DiskUsageOptions
	buildOptions build.CachePruneOptions
	buildReport  *build.CachePruneReport
	buildErr     error
	imageFilters filters.Args
	imageReport  image.PruneReport
	imageErr     error
}

func (f *fakeStorageAPI) DiskUsage(_ context.Context, options dockertypes.DiskUsageOptions) (dockertypes.DiskUsage, error) {
	f.usageOptions = options
	return f.usage, f.usageErr
}

func (f *fakeStorageAPI) BuildCachePrune(_ context.Context, options build.CachePruneOptions) (*build.CachePruneReport, error) {
	f.buildOptions = options
	return f.buildReport, f.buildErr
}

func (f *fakeStorageAPI) ImagesPrune(_ context.Context, pruneFilters filters.Args) (image.PruneReport, error) {
	f.imageFilters = pruneFilters
	return f.imageReport, f.imageErr
}

type fakeContainerRemover struct {
	id      string
	options container.RemoveOptions
}

func (f *fakeContainerRemover) ContainerRemove(_ context.Context, id string, options container.RemoveOptions) error {
	f.id = id
	f.options = options
	return nil
}

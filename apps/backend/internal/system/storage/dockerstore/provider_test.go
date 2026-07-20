package dockerstore

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/system/storage"
)

func TestResultJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(Result{Available: true, Removed: 1, Kept: 2, Warnings: []string{"warn"}})
	if err != nil {
		t.Fatalf("Marshal Result: %v", err)
	}
	want := `{"available":true,"removed":1,"kept":2,"warnings":["warn"]}`
	if string(encoded) != want {
		t.Fatalf("Result JSON = %s, want %s", encoded, want)
	}
}

func TestAnalysisJSONUsesStorageAPISnakeCase(t *testing.T) {
	encoded, err := json.Marshal(Analysis{
		Available: true, ManagedContainerCount: 1, ManagedContainerBytes: 2,
		BuildCacheBytes: 3, UnusedImageBytes: 4, Warnings: []string{"warn"},
	})
	if err != nil {
		t.Fatalf("Marshal Analysis: %v", err)
	}
	want := `{"available":true,"managed_container_count":1,"managed_container_bytes":2,"image_layer_bytes":0,"build_cache_bytes":3,"unused_image_bytes":4,"warnings":["warn"]}`
	if string(encoded) != want {
		t.Fatalf("Analysis JSON = %s, want %s", encoded, want)
	}
}

func TestCleanupContainersRemovesOnlyPositivelyClassifiedStoppedManagedContainers(t *testing.T) {
	docker := &fakeDockerClient{containers: []agentdocker.ContainerInfo{
		{ID: "orphan", State: "exited", Labels: map[string]string{"kandev.managed": "true", "kandev.task_id": "gone"}},
		{ID: "running", State: "running", Labels: map[string]string{"kandev.managed": "true", "kandev.task_id": "gone"}},
		{ID: "unrelated", State: "exited", Labels: map[string]string{"kandev.task_id": "gone"}},
		{ID: "active", State: "exited", Labels: map[string]string{"kandev.managed": "true", "kandev.task_id": "active"}},
	}}
	inventory := &fakeInventory{removable: map[string]bool{"gone": true, "active": false}}
	provider := NewProvider(docker, inventory, staticSettings(storage.DefaultSettings()))

	got := provider.CleanupContainers(context.Background())
	if !got.Available || got.Removed != 1 || got.Kept != 3 {
		t.Fatalf("cleanup result = %#v, want available removed=1 kept=3", got)
	}
	if !reflect.DeepEqual(docker.listLabels, map[string]string{"kandev.managed": "true"}) {
		t.Fatalf("list labels = %#v", docker.listLabels)
	}
	if !reflect.DeepEqual(docker.removed, []removeCall{{id: "orphan", force: false}}) {
		t.Fatalf("removed = %#v", docker.removed)
	}
}

func TestCleanupContainersFailsClosedOnInventoryError(t *testing.T) {
	docker := &fakeDockerClient{containers: []agentdocker.ContainerInfo{{
		ID: "uncertain", State: "exited",
		Labels: map[string]string{"kandev.managed": "true", "kandev.task_id": "task-1"},
	}}}
	provider := NewProvider(docker, &fakeInventory{err: errors.New("database unavailable")}, staticSettings(storage.DefaultSettings()))

	got := provider.CleanupContainers(context.Background())
	if got.Removed != 0 || got.Kept != 1 || len(got.Warnings) != 1 || len(docker.removed) != 0 {
		t.Fatalf("cleanup result = %#v, removed calls = %#v", got, docker.removed)
	}
}

func TestAnalyzeIsReadOnlyAndReportsUnavailableWithoutError(t *testing.T) {
	t.Run("read only", func(t *testing.T) {
		docker := &fakeDockerClient{usage: agentdocker.DiskUsage{
			ImageLayerBytes: 150,
			BuildCacheBytes: 200,
			Containers: []agentdocker.ContainerUsage{
				{ID: "managed", WritableBytes: 25, Labels: map[string]string{"kandev.managed": "true"}},
				{ID: "managed-zero", WritableBytes: -1, Labels: map[string]string{"kandev.managed": "true"}},
				{ID: "unrelated", WritableBytes: 75, Labels: map[string]string{"owner": "user"}},
			},
			Images: []agentdocker.ImageUsage{
				{ID: "old-unused", SizeBytes: 300, Containers: 0, CreatedAt: time.Unix(1, 0)},
				{ID: "used", SizeBytes: 400, Containers: 1, CreatedAt: time.Unix(1, 0)},
			},
		}}
		provider := NewProvider(docker, &fakeInventory{}, staticSettings(storage.DefaultSettings()))
		provider.now = func() time.Time { return time.Unix(2000000, 0) }

		got := provider.Analyze(context.Background())
		if !got.Available || got.ImageLayerBytes != 150 || got.BuildCacheBytes != 200 || got.UnusedImageBytes != 300 ||
			got.ManagedContainerCount != 2 || got.ManagedContainerBytes != 25 {
			t.Fatalf("analysis = %#v", got)
		}
		if docker.buildPrunes != 0 || docker.imagePrunes != 0 || len(docker.removed) != 0 {
			t.Fatalf("analysis mutated Docker: %#v", docker)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		docker := &fakeDockerClient{pingErr: errors.New("no daemon")}
		provider := NewProvider(docker, &fakeInventory{}, staticSettings(storage.DefaultSettings()))
		got := provider.Analyze(context.Background())
		if got.Available || len(got.Warnings) != 1 {
			t.Fatalf("analysis = %#v, want unavailable warning", got)
		}
	})
}

func TestGlobalPruneRechecksPersistedAcknowledgementImmediatelyBeforeSDKCall(t *testing.T) {
	settings := storage.DefaultSettings()
	settings.Docker.DedicatedDaemonAcknowledged = true
	settings.Docker.BuildCacheEnabled = true
	settings.Docker.UnusedImagesEnabled = true
	docker := &fakeDockerClient{}
	reader := &sequenceSettings{values: []storage.StorageMaintenanceSettings{settings, storage.DefaultSettings()}}
	provider := NewProvider(docker, &fakeInventory{}, reader)

	if _, err := provider.PruneBuildCache(context.Background()); err != nil {
		t.Fatalf("first prune: %v", err)
	}
	if _, err := provider.PruneUnusedImages(context.Background()); !errors.Is(err, ErrGlobalCleanupDisabled) {
		t.Fatalf("second prune error = %v, want ErrGlobalCleanupDisabled", err)
	}
	if docker.buildPrunes != 1 || docker.imagePrunes != 0 {
		t.Fatalf("prune calls build=%d image=%d", docker.buildPrunes, docker.imagePrunes)
	}
}

func TestGlobalPruneRejectsMissingDockerClient(t *testing.T) {
	settings := storage.DefaultSettings()
	settings.Docker.DedicatedDaemonAcknowledged = true
	settings.Docker.BuildCacheEnabled = true
	settings.Docker.UnusedImagesEnabled = true

	tests := []struct {
		name  string
		prune func(*Provider) error
	}{
		{
			name: "build cache",
			prune: func(provider *Provider) error {
				_, err := provider.PruneBuildCache(context.Background())
				return err
			},
		},
		{
			name: "unused images",
			prune: func(provider *Provider) error {
				_, err := provider.PruneUnusedImages(context.Background())
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(nil, &fakeInventory{}, staticSettings(settings))
			if err := tt.prune(provider); !errors.Is(err, ErrDockerUnavailable) {
				t.Fatalf("prune error = %v, want ErrDockerUnavailable", err)
			}
		})
	}
}

func TestGlobalPruneRejectsOverflowingAgeWithoutSDKCall(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*storage.StorageMaintenanceSettings)
		prune  func(*Provider) error
	}{
		{
			name: "build cache",
			mutate: func(settings *storage.StorageMaintenanceSettings) {
				settings.Docker.BuildCacheEnabled = true
				settings.Docker.BuildCacheUnusedHours = math.MaxInt
			},
			prune: func(provider *Provider) error {
				_, err := provider.PruneBuildCache(context.Background())
				return err
			},
		},
		{
			name: "unused images",
			mutate: func(settings *storage.StorageMaintenanceSettings) {
				settings.Docker.UnusedImagesEnabled = true
				settings.Docker.UnusedImagesHours = math.MaxInt
			},
			prune: func(provider *Provider) error {
				_, err := provider.PruneUnusedImages(context.Background())
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := storage.DefaultSettings()
			settings.Docker.DedicatedDaemonAcknowledged = true
			tt.mutate(&settings)
			docker := &fakeDockerClient{}
			provider := NewProvider(docker, &fakeInventory{}, staticSettings(settings))

			if err := tt.prune(provider); !errors.Is(err, storage.ErrValidation) {
				t.Fatalf("prune error = %v, want storage.ErrValidation", err)
			}
			if docker.buildPrunes != 0 || docker.imagePrunes != 0 {
				t.Fatalf("unsafe SDK calls build=%d image=%d", docker.buildPrunes, docker.imagePrunes)
			}
		})
	}
}

type fakeDockerClient struct {
	containers  []agentdocker.ContainerInfo
	listLabels  map[string]string
	removed     []removeCall
	pingErr     error
	usage       agentdocker.DiskUsage
	usageErr    error
	buildPrunes int
	imagePrunes int
}

type removeCall struct {
	id    string
	force bool
}

func (f *fakeDockerClient) Ping(context.Context) error { return f.pingErr }

func (f *fakeDockerClient) ListContainers(_ context.Context, labels map[string]string) ([]agentdocker.ContainerInfo, error) {
	f.listLabels = labels
	return f.containers, nil
}

func (f *fakeDockerClient) RemoveContainer(_ context.Context, id string, force bool) error {
	f.removed = append(f.removed, removeCall{id: id, force: force})
	return nil
}

func (f *fakeDockerClient) DiskUsage(context.Context) (agentdocker.DiskUsage, error) {
	return f.usage, f.usageErr
}

func (f *fakeDockerClient) PruneBuildCache(context.Context, agentdocker.BuildCachePruneOptions) (agentdocker.PruneResult, error) {
	f.buildPrunes++
	return agentdocker.PruneResult{BytesReclaimed: 10}, nil
}

func (f *fakeDockerClient) PruneUnusedImages(context.Context, time.Time) (agentdocker.PruneResult, error) {
	f.imagePrunes++
	return agentdocker.PruneResult{BytesReclaimed: 20}, nil
}

type fakeInventory struct {
	removable map[string]bool
	err       error
}

func (f *fakeInventory) ContainerTaskRemovable(_ context.Context, taskID string) (bool, error) {
	return f.removable[taskID], f.err
}

type sequenceSettings struct {
	values []storage.StorageMaintenanceSettings
	index  int
}

func (s *sequenceSettings) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	value := s.values[s.index]
	s.index++
	return value, nil
}

func staticSettings(settings storage.StorageMaintenanceSettings) *sequenceSettings {
	return &sequenceSettings{values: []storage.StorageMaintenanceSettings{settings}}
}

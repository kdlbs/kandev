package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestStorageOverviewIncludesQuarantineAndManagedContainers(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{filepath.Join(home, "tasks"), filepath.Join(home, "trash")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := storagepkg.QuarantineEntry{
		ID: "overview-entry", ResourceType: storagepkg.ResourceTypeGoCache,
		OriginalPath:   filepath.Join(home, "cache", "go-build"),
		QuarantinePath: filepath.Join(home, "trash", "go-cache", "overview-entry"),
		SizeBytes:      42, State: storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC(), DeleteAfter: time.Now().UTC().Add(time.Hour),
	}
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatalf("CreateQuarantineEntry: %v", err)
	}
	docker := dockerstore.NewProvider(
		&overviewDockerClient{usage: agentdocker.DiskUsage{ImageLayerBytes: 128, Containers: []agentdocker.ContainerUsage{
			{ID: "managed", WritableBytes: 64, Labels: map[string]string{"kandev.managed": "true"}},
		}}},
		overviewContainerInventory{}, settings,
	)
	workspaceFactory := func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
		return workspaces.New(workspaces.Config{
			TasksRoot: filepath.Join(home, "tasks"), TrashRoot: filepath.Join(home, "trash"),
			Inventory: overviewWorkspaceInventory{}, Store: store,
			GracePeriod: time.Duration(current.OrphanGraceHours) * time.Hour,
			Retention:   time.Duration(current.QuarantineRetentionHours) * time.Hour,
		})
	}
	overview := &storageOverview{
		settings: settings, quarantine: store, workspaceFactory: workspaceFactory,
		goCache: gocache.New(gocache.Config{
			HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: settings, Store: store,
		}),
		docker: docker, homeDir: home,
	}

	summary, err := overview.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	quarantine, ok := summary.Quarantine.(storagepkg.QuarantineSummary)
	if !ok || quarantine.Count != 1 || quarantine.SizeBytes != 42 {
		t.Fatalf("quarantine summary = %#v", summary.Quarantine)
	}
	dockerSummary, ok := summary.Docker.(map[string]any)
	if !ok || dockerSummary["managed_container_count"] != 1 ||
		dockerSummary["managed_container_bytes"] != int64(64) ||
		dockerSummary["image_layer_bytes"] != int64(128) {
		t.Fatalf("docker summary = %#v", summary.Docker)
	}

	overview.quarantine = failingQuarantineSummarizer{err: errors.New("quarantine unavailable")}
	degraded, err := overview.Summary(context.Background())
	if err != nil {
		t.Fatalf("degraded Summary: %v", err)
	}
	quarantineWarning, ok := degraded.Quarantine.(map[string]any)
	if !ok || quarantineWarning["available"] != false || quarantineWarning["warning"] != "quarantine unavailable" {
		t.Fatalf("degraded quarantine = %#v", degraded.Quarantine)
	}
	if _, ok := degraded.Workspaces.(workspaces.Analysis); !ok {
		t.Fatalf("workspace summary should remain available, got %#v", degraded.Workspaces)
	}
}

func TestStorageCleanupProvidersIncludeWorkspaceDependencyCleanup(t *testing.T) {
	settings, store := newStorageMaintenanceStores(t)
	home := t.TempDir()
	workspaceFactory := func(storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
		return workspaces.New(workspaces.Config{TasksRoot: filepath.Join(home, "tasks"), Store: store})
	}
	providers := storageCleanupProviders(settings, workspaceFactory, nil, nil, nil)
	for _, provider := range providers {
		if provider.Name() == "workspace_dependencies" {
			return
		}
	}
	t.Fatal("storage cleanup providers did not include workspace_dependencies")
}

func TestWorkspaceDependencyCleanupProviderIsDefaultOff(t *testing.T) {
	settings, store := newStorageMaintenanceStores(t)
	home := t.TempDir()
	factoryCalls := 0
	workspaceFactory := func(storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
		factoryCalls++
		return workspaces.New(workspaces.Config{TasksRoot: filepath.Join(home, "tasks"), Store: store})
	}
	var dependencyProvider storagepkg.CleanupProvider
	for _, provider := range storageCleanupProviders(settings, workspaceFactory, nil, nil, nil) {
		if provider.Name() == "workspace_dependencies" {
			dependencyProvider = provider
			break
		}
	}
	if dependencyProvider == nil {
		t.Fatal("workspace dependency provider is missing")
	}
	if result, err := dependencyProvider.Cleanup(context.Background()); err != nil || result != nil {
		t.Fatalf("default-off cleanup = (%#v, %v), want no-op", result, err)
	}
	if factoryCalls != 0 {
		t.Fatalf("workspace factory calls = %d, want 0 while disabled", factoryCalls)
	}

	enabled := storagepkg.DefaultSettings()
	enabled.Workspaces.DependencyCleanupEnabled = true
	if _, err := settings.SaveSettings(context.Background(), enabled); err != nil {
		t.Fatalf("enable dependency cleanup: %v", err)
	}
	if _, err := dependencyProvider.Cleanup(context.Background()); err == nil {
		t.Fatal("enabled dependency cleanup unexpectedly succeeded without complete inventory")
	}
	if factoryCalls != 1 {
		t.Fatalf("workspace factory calls = %d, want 1 after enabling", factoryCalls)
	}
}

func TestStorageOverviewStartsIndependentMeasurementsTogether(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{filepath.Join(home, "tasks"), filepath.Join(home, "trash")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	settings, store := newStorageMaintenanceStores(t)
	workspaceStarted, workspaceRelease := make(chan struct{}), make(chan struct{})
	goCacheStarted, goCacheRelease := make(chan struct{}), make(chan struct{})
	quarantine := &blockingOverviewQuarantine{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	dockerClient := &blockingOverviewDockerClient{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseMeasurements := func() {
		releaseOnce.Do(func() {
			close(workspaceRelease)
			close(goCacheRelease)
			close(quarantine.release)
			close(dockerClient.release)
		})
	}
	t.Cleanup(releaseMeasurements)
	docker := dockerstore.NewProvider(dockerClient, overviewContainerInventory{}, settings)
	overview := &storageOverview{
		settings: settings, quarantine: quarantine,
		workspaceAnalyze: func(context.Context, storagepkg.StorageMaintenanceSettings) (workspaces.Analysis, error) {
			close(workspaceStarted)
			<-workspaceRelease
			return workspaces.Analysis{}, nil
		},
		goCacheAnalyze: func(context.Context) (gocache.Analysis, error) {
			close(goCacheStarted)
			<-goCacheRelease
			return gocache.Analysis{}, nil
		},
		workspaceFactory: func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
			return workspaces.New(workspaces.Config{
				TasksRoot: filepath.Join(home, "tasks"), TrashRoot: filepath.Join(home, "trash"),
				Inventory: overviewWorkspaceInventory{}, Store: store,
				GracePeriod: time.Duration(current.OrphanGraceHours) * time.Hour,
				Retention:   time.Duration(current.QuarantineRetentionHours) * time.Hour,
			})
		},
		goCache: gocache.New(gocache.Config{
			HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: settings, Store: store,
		}),
		docker: docker, homeDir: home,
	}

	resultCh := make(chan struct {
		summary storagepkg.Summary
		err     error
	}, 1)
	go func() {
		summary, err := overview.Summary(context.Background())
		resultCh <- struct {
			summary storagepkg.Summary
			err     error
		}{summary: summary, err: err}
	}()

	for _, measurement := range []struct {
		name    string
		started <-chan struct{}
	}{
		{name: "workspace", started: workspaceStarted},
		{name: "Go cache", started: goCacheStarted},
		{name: "quarantine", started: quarantine.started},
		{name: "Docker", started: dockerClient.started},
	} {
		select {
		case <-measurement.started:
		case <-time.After(time.Second):
			t.Fatalf("%s measurement did not start before the barrier was released", measurement.name)
		}
	}
	releaseMeasurements()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("Summary: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Summary did not finish")
	}
}

type blockingOverviewQuarantine struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingOverviewQuarantine) SummarizeQuarantine(context.Context) (storagepkg.QuarantineSummary, error) {
	close(s.started)
	<-s.release
	return storagepkg.QuarantineSummary{}, nil
}

type blockingOverviewDockerClient struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingOverviewDockerClient) Ping(context.Context) error { return nil }
func (c *blockingOverviewDockerClient) ListContainers(context.Context, map[string]string) ([]agentdocker.ContainerInfo, error) {
	return nil, nil
}
func (c *blockingOverviewDockerClient) RemoveContainer(context.Context, string, bool) error {
	return nil
}
func (c *blockingOverviewDockerClient) DiskUsage(context.Context) (agentdocker.DiskUsage, error) {
	close(c.started)
	<-c.release
	return agentdocker.DiskUsage{}, nil
}
func (c *blockingOverviewDockerClient) PruneBuildCache(context.Context, agentdocker.BuildCachePruneOptions) (agentdocker.PruneResult, error) {
	return agentdocker.PruneResult{}, nil
}
func (c *blockingOverviewDockerClient) PruneUnusedImages(context.Context, time.Time) (agentdocker.PruneResult, error) {
	return agentdocker.PruneResult{}, nil
}

type failingQuarantineSummarizer struct{ err error }

func (s failingQuarantineSummarizer) SummarizeQuarantine(context.Context) (storagepkg.QuarantineSummary, error) {
	return storagepkg.QuarantineSummary{}, s.err
}

type overviewWorkspaceInventory struct{}

func (overviewWorkspaceInventory) LoadWorkspaceInventory(context.Context) (workspaces.Inventory, error) {
	return workspaces.Inventory{Complete: true}, nil
}

type overviewContainerInventory struct{}

func (overviewContainerInventory) ContainerTaskRemovable(context.Context, string) (bool, error) {
	return false, nil
}

type overviewDockerClient struct{ usage agentdocker.DiskUsage }

func (c *overviewDockerClient) Ping(context.Context) error { return nil }
func (c *overviewDockerClient) ListContainers(context.Context, map[string]string) ([]agentdocker.ContainerInfo, error) {
	return nil, nil
}
func (c *overviewDockerClient) RemoveContainer(context.Context, string, bool) error { return nil }
func (c *overviewDockerClient) DiskUsage(context.Context) (agentdocker.DiskUsage, error) {
	return c.usage, nil
}
func (c *overviewDockerClient) PruneBuildCache(context.Context, agentdocker.BuildCachePruneOptions) (agentdocker.PruneResult, error) {
	return agentdocker.PruneResult{}, nil
}
func (c *overviewDockerClient) PruneUnusedImages(context.Context, time.Time) (agentdocker.PruneResult, error) {
	return agentdocker.PruneResult{}, nil
}

func TestWorkspaceQuarantineControllerRestoresTaskUsingCurrentSettings(t *testing.T) {
	settings, store := newStorageMaintenanceStores(t)
	want := storagepkg.DefaultSettings()
	want.OrphanGraceHours = 48
	if _, err := settings.SaveSettings(context.Background(), want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	var captured storagepkg.StorageMaintenanceSettings
	controller := &workspaceQuarantineController{
		settings: settings,
		factory: func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
			captured = current
			return workspaces.New(workspaces.Config{Store: store})
		},
	}

	recovery := controller.RestoreTask(context.Background(), "task-1")
	if captured.OrphanGraceHours != want.OrphanGraceHours {
		t.Fatalf("factory settings orphan_grace_hours = %d, want %d", captured.OrphanGraceHours, want.OrphanGraceHours)
	}
	if recovery.TaskID != "task-1" || recovery.Status != "not_found" {
		t.Fatalf("recovery = %#v, want task-1 not_found", recovery)
	}
}

func TestWorkspaceQuarantineControllerMapsRestoreConflict(t *testing.T) {
	home := t.TempDir()
	tasksRoot := filepath.Join(home, "tasks")
	original := filepath.Join(tasksRoot, "task-1")
	quarantined := filepath.Join(home, "trash", "tasks", "entry-1")
	for _, path := range []string{original, quarantined} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := storagepkg.QuarantineEntry{
		ID: "entry-1", ResourceType: storagepkg.ResourceTypeTaskWorkspace,
		OriginalPath: original, QuarantinePath: quarantined,
		State:         storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC(), DeleteAfter: time.Now().UTC().Add(time.Hour),
	}
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	controller := &workspaceQuarantineController{
		settings: settings, store: store,
		factory: func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
			return workspaces.New(workspaces.Config{
				TasksRoot: tasksRoot, TrashRoot: filepath.Join(home, "trash"), Store: store,
				GracePeriod: time.Duration(current.OrphanGraceHours) * time.Hour,
				Retention:   time.Duration(current.QuarantineRetentionHours) * time.Hour,
			})
		},
	}

	_, err := controller.Restore(context.Background(), entry.ID)
	if !errors.Is(err, storagepkg.ErrConflict) {
		t.Fatalf("Restore error = %v, want storage ErrConflict", err)
	}
}

func TestQuarantineControllerRestoresGoCache(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(time.Hour))
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	restored, err := controller.Restore(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != storagepkg.QuarantineStateRestored {
		t.Fatalf("state = %q, want restored", restored.State)
	}
	if _, err := os.Stat(filepath.Join(original, "artifact")); err != nil {
		t.Fatalf("restored cache artifact: %v", err)
	}
}

func TestQuarantineControllerRestoresGoCacheOverEmptyReplacement(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	current, err := settings.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.GoCache.Enabled = true
	if _, err := settings.SaveSettings(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	provider := gocache.New(gocache.Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: settings,
	})
	if _, err := provider.ExecutionEnvironment(context.Background()); err != nil {
		t.Fatalf("create managed replacement: %v", err)
	}
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(time.Hour))
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	restored, err := controller.Restore(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != storagepkg.QuarantineStateRestored {
		t.Fatalf("state = %q, want restored", restored.State)
	}
	if data, err := os.ReadFile(filepath.Join(original, "artifact")); err != nil || string(data) != "cache" {
		t.Fatalf("restored cache artifact: data=%q err=%v", data, err)
	}
}

func TestQuarantineControllerRetainsGoCacheWhileTaskActivityIsRunning(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(quarantined, "artifact")
	if err := os.WriteFile(artifact, []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	current, err := settings.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.GoCache.Enabled = true
	if _, err := settings.SaveSettings(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	provider := gocache.New(gocache.Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: settings,
	})
	if _, err := provider.ExecutionEnvironment(context.Background()); err != nil {
		t.Fatalf("create managed replacement: %v", err)
	}
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(time.Hour))
	coordinator := activity.NewCoordinator(activity.Options{})
	taskLease, err := coordinator.AcquireTask(context.Background(), activity.KindExecutionRunning)
	if err != nil {
		t.Fatal(err)
	}
	defer taskLease.Release()
	controller := &workspaceQuarantineController{
		settings: settings, store: store, homeDir: home, activity: coordinator,
	}

	_, err = controller.Restore(context.Background(), entry.ID)
	var busy *storagepkg.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("Restore error = %v, want BusyError", err)
	}
	if busy.ForceAvailable || len(busy.Resources) != 1 || busy.Resources[0].Label == "" {
		t.Fatalf("busy response = %#v, want labeled non-overridable resource", busy)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("quarantined cache changed while task active: %v", err)
	}
}

func TestQuarantineControllerRestoresAdoptedExternalGoCache(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	original := filepath.Join(root, "external", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	for _, path := range []string{home, original, quarantined} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(original); err != nil {
		t.Fatalf("remove recreated cache destination: %v", err)
	}
	settings, store := newStorageMaintenanceStores(t)
	if _, err := settings.AdoptGoCachePath(context.Background(), original); err != nil {
		t.Fatalf("AdoptGoCachePath: %v", err)
	}
	entry := createGoCacheQuarantineEntry(
		t, store, original, quarantined, time.Now().UTC().Add(time.Hour),
	)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	restored, err := controller.Restore(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != storagepkg.QuarantineStateRestored {
		t.Fatalf("state = %q, want restored", restored.State)
	}
	if _, err := os.Stat(filepath.Join(original, "artifact")); err != nil {
		t.Fatalf("restored external cache artifact: %v", err)
	}
}

func TestQuarantineControllerRetriesFailedGoCacheRestore(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(time.Hour))
	markGoCacheQuarantineFailed(t, store, entry.ID)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	restored, err := controller.Restore(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != storagepkg.QuarantineStateRestored {
		t.Fatalf("state = %q, want restored", restored.State)
	}
	if _, err := os.Stat(filepath.Join(original, "artifact")); err != nil {
		t.Fatalf("restored cache artifact: %v", err)
	}
}

func TestQuarantineControllerRetriesFailedGoCacheDelete(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(-time.Hour))
	markGoCacheQuarantineFailed(t, store, entry.ID)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	deleted, err := controller.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	if deleted.State != storagepkg.QuarantineStateDeleted {
		t.Fatalf("state = %q, want deleted", deleted.State)
	}
	if _, err := os.Stat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine path still exists: %v", err)
	}
}

func TestQuarantineControllerForceDeletesProtectedGoCache(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(time.Hour))
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	if _, err := controller.PermanentDeleteForce(context.Background(), entry.ID, "DELETE"); !errors.Is(err, storagepkg.ErrForceDeleteConfirmation) {
		t.Fatalf("wrong force confirmation error = %v, want %v", err, storagepkg.ErrForceDeleteConfirmation)
	}
	deleted, err := controller.PermanentDeleteForce(context.Background(), entry.ID, "DELETE ALL NOW")
	if err != nil {
		t.Fatalf("PermanentDeleteForce: %v", err)
	}
	if deleted.State != storagepkg.QuarantineStateDeleted {
		t.Fatalf("state = %q, want deleted", deleted.State)
	}
	if _, err := os.Stat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine path still exists: %v", err)
	}
}

func TestQuarantineControllerPurgeEligibleReportsProtectedAndDeleted(t *testing.T) {
	home := t.TempDir()
	settings, store := newStorageMaintenanceStores(t)
	protected := storagepkg.QuarantineEntry{
		ID: "protected-workspace", ResourceType: storagepkg.ResourceTypeTaskWorkspace,
		OriginalPath:   filepath.Join(home, "tasks", "protected"),
		QuarantinePath: filepath.Join(home, "trash", "tasks", "protected-workspace"),
		SizeBytes:      17, State: storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC(), DeleteAfter: time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(protected.QuarantinePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateQuarantineEntry(context.Background(), &protected); err != nil {
		t.Fatal(err)
	}
	eligible := createGoCacheQuarantineEntryWithID(
		t, store, home, "eligible-cache", time.Now().UTC().Add(-time.Hour),
	)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	result, err := controller.Purge(context.Background(), storagepkg.QuarantinePurgeScopeEligible, "DELETE ELIGIBLE")
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if result.Considered != 2 || result.Deleted != 1 || result.Protected != 1 || result.Failed != 0 {
		t.Fatalf("purge result = %#v, want considered=2 deleted=1 protected=1 failed=0", result)
	}
	if result.DeletedBytes != eligible.SizeBytes || result.ProtectedBytes != protected.SizeBytes {
		t.Fatalf("purge bytes = deleted:%d protected:%d, want deleted:%d protected:%d", result.DeletedBytes, result.ProtectedBytes, eligible.SizeBytes, protected.SizeBytes)
	}
	if _, err := os.Stat(eligible.QuarantinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("eligible quarantine path still exists: %v", err)
	}
	if _, err := os.Stat(protected.QuarantinePath); err != nil {
		t.Fatalf("protected quarantine path changed: %v", err)
	}
}

func TestQuarantineCleanupProviderPurgesEligibleEntries(t *testing.T) {
	purger := &recordingQuarantinePurger{}
	provider := quarantineCleanupProvider{purger: purger}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if purger.scope != storagepkg.QuarantinePurgeScopeEligible || purger.confirmation != "DELETE ELIGIBLE" {
		t.Fatalf("purge request = scope:%q confirmation:%q", purger.scope, purger.confirmation)
	}
	if result["deleted"] != float64(2) || result["protected"] != float64(1) {
		t.Fatalf("provider result = %#v, want deleted=2 protected=1", result)
	}
}

func TestStorageCleanupProvidersIncludesQuarantineProvider(t *testing.T) {
	providers := storageCleanupProviders(nil, nil, nil, nil, &recordingQuarantinePurger{})
	if len(providers) != 7 {
		t.Fatalf("provider count = %d, want 7", len(providers))
	}
	if providers[0].Name() != "quarantine" {
		t.Fatalf("first provider = %q, want quarantine", providers[0].Name())
	}
	if providers[1].Name() != "workspaces" || providers[2].Name() != "workspace_dependencies" || providers[3].Name() != "go_cache" {
		t.Fatalf("provider order = %q, %q, %q, want workspaces, workspace_dependencies, go_cache", providers[1].Name(), providers[2].Name(), providers[3].Name())
	}
}

type recordingQuarantinePurger struct {
	scope        storagepkg.QuarantinePurgeScope
	confirmation string
}

func (p *recordingQuarantinePurger) Purge(
	_ context.Context,
	scope storagepkg.QuarantinePurgeScope,
	confirmation string,
) (storagepkg.QuarantinePurgeResult, error) {
	p.scope = scope
	p.confirmation = confirmation
	return storagepkg.QuarantinePurgeResult{Deleted: 2, Protected: 1}, nil
}

func TestQuarantineControllerDoesNotTreatPopulatedReplacementAsRestoredPayload(t *testing.T) {
	states := []storagepkg.QuarantineState{
		storagepkg.QuarantineStateQuarantined,
		storagepkg.QuarantineStateFailed,
	}
	for _, state := range states {
		for _, operation := range []string{"restore", "delete"} {
			t.Run(string(state)+"_"+operation, func(t *testing.T) {
				home := t.TempDir()
				original := filepath.Join(home, "cache", "go-build")
				quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
				if err := os.MkdirAll(original, 0o700); err != nil {
					t.Fatal(err)
				}
				artifact := filepath.Join(original, "replacement-artifact")
				if err := os.WriteFile(artifact, []byte("active cache"), 0o600); err != nil {
					t.Fatal(err)
				}
				settings, store := newStorageMaintenanceStores(t)
				entry := createGoCacheQuarantineEntry(
					t, store, original, quarantined, time.Now().UTC().Add(-time.Hour),
				)
				if state == storagepkg.QuarantineStateFailed {
					markGoCacheQuarantineFailed(t, store, entry.ID)
				}
				controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

				var err error
				if operation == "delete" {
					_, err = controller.PermanentDelete(context.Background(), entry.ID, "DELETE")
				} else {
					_, err = controller.Restore(context.Background(), entry.ID)
				}
				if !errors.Is(err, storagepkg.ErrConflict) {
					t.Fatalf("%s error = %v, want storage ErrConflict", operation, err)
				}
				stored, err := store.GetQuarantineEntry(context.Background(), entry.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.State != state {
					t.Fatalf("state = %q, want unchanged %q", stored.State, state)
				}
				if data, err := os.ReadFile(artifact); err != nil || string(data) != "active cache" {
					t.Fatalf("replacement cache changed: data=%q err=%v", data, err)
				}
			})
		}
	}
}

func TestQuarantineControllerDoesNotMarkMissingPayloadPlaceholderRestored(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	settings, store := newStorageMaintenanceStores(t)
	current, err := settings.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.GoCache.Enabled = true
	if _, err := settings.SaveSettings(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	provider := gocache.New(gocache.Config{
		HomeDir: home, TrashDir: filepath.Join(home, "trash"), Settings: settings,
	})
	if _, err := provider.ExecutionEnvironment(context.Background()); err != nil {
		t.Fatalf("create managed replacement: %v", err)
	}
	entry := createGoCacheQuarantineEntry(
		t, store, original, quarantined, time.Now().UTC().Add(time.Hour),
	)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	_, err = controller.Restore(context.Background(), entry.ID)
	if !errors.Is(err, storagepkg.ErrConflict) {
		t.Fatalf("Restore error = %v, want storage ErrConflict", err)
	}
	stored, err := store.GetQuarantineEntry(context.Background(), entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != storagepkg.QuarantineStateQuarantined {
		t.Fatalf("state = %q, want quarantined", stored.State)
	}
}

func TestQuarantineControllerReportsPersistenceAndFailsClosedAfterRollbackFailure(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := storagepkg.QuarantineEntry{
		ID: "entry-cache", ResourceType: storagepkg.ResourceTypeGoCache,
		OriginalPath: original, QuarantinePath: quarantined,
		State: storagepkg.QuarantineStateQuarantined, Metadata: []byte(`{"ownership":"managed"}`),
	}
	store := &failingTransitionQuarantineStore{entry: entry, err: errors.New("database unavailable")}
	renameCalls := 0
	controller := &workspaceQuarantineController{
		store: store, homeDir: home,
		rename: func(oldPath, newPath string) error {
			renameCalls++
			if renameCalls == 2 {
				return errors.New("rollback blocked")
			}
			return os.Rename(oldPath, newPath)
		},
	}

	_, err := controller.Restore(context.Background(), entry.ID)
	if err == nil || !strings.Contains(err.Error(), "database unavailable") ||
		!strings.Contains(err.Error(), "rollback blocked") {
		t.Fatalf("Restore error = %v, want persistence and rollback failures", err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("restored data missing after failed rollback: %v", err)
	}
	if _, err := os.Stat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine path exists after failed rollback: %v", err)
	}

	store.err = nil
	_, err = controller.Restore(context.Background(), entry.ID)
	if !errors.Is(err, storagepkg.ErrConflict) {
		t.Fatalf("ambiguous retry error = %v, want storage ErrConflict", err)
	}
	if store.entry.State != storagepkg.QuarantineStateQuarantined {
		t.Fatalf("retry state = %q, want quarantined", store.entry.State)
	}
}

type failingTransitionQuarantineStore struct {
	entry storagepkg.QuarantineEntry
	err   error
}

func (s *failingTransitionQuarantineStore) GetQuarantineEntry(
	context.Context, string,
) (storagepkg.QuarantineEntry, error) {
	return s.entry, nil
}

func (s *failingTransitionQuarantineStore) ListQuarantineEntries(
	context.Context, bool,
) ([]storagepkg.QuarantineEntry, error) {
	return []storagepkg.QuarantineEntry{s.entry}, nil
}

func (s *failingTransitionQuarantineStore) TransitionQuarantineEntry(
	_ context.Context,
	_ string,
	next storagepkg.QuarantineState,
	lastError string,
) (storagepkg.QuarantineEntry, error) {
	if s.err != nil {
		return storagepkg.QuarantineEntry{}, s.err
	}
	s.entry.State = next
	s.entry.LastError = lastError
	return s.entry, nil
}

func TestQuarantineControllerFailedGoCacheRetryRejectsSymlinkedRestorePath(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(filepath.Join(external, "go-build"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(root, "linked-external")
	if err := os.Symlink(external, linkedParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	original := filepath.Join(linkedParent, "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	settings, store := newStorageMaintenanceStores(t)
	if _, err := settings.AdoptGoCachePath(context.Background(), original); err != nil {
		t.Fatalf("AdoptGoCachePath: %v", err)
	}
	entry := createGoCacheQuarantineEntry(
		t, store, original, quarantined, time.Now().UTC().Add(time.Hour),
	)
	markGoCacheQuarantineFailed(t, store, entry.ID)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	_, err := controller.Restore(context.Background(), entry.ID)
	if !errors.Is(err, storagepkg.ErrValidation) {
		t.Fatalf("Restore error = %v, want storage ErrValidation", err)
	}
	stored, err := store.GetQuarantineEntry(context.Background(), entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != storagepkg.QuarantineStateFailed {
		t.Fatalf("state = %q, want failed", stored.State)
	}
}

func TestQuarantineControllerPermanentlyDeletesGoCache(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(t, store, original, quarantined, time.Now().UTC().Add(-time.Hour))
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	deleted, err := controller.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	if deleted.State != storagepkg.QuarantineStateDeleted {
		t.Fatalf("state = %q, want deleted", deleted.State)
	}
	if _, err := os.Stat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine path still exists: %v", err)
	}
}

func TestQuarantineControllerDeletesHistoricalAdoptedGoCacheAfterReadoption(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	original := filepath.Join(root, "first-cache")
	current := filepath.Join(root, "second-cache")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	if _, err := settings.AdoptGoCachePath(context.Background(), current); err != nil {
		t.Fatalf("AdoptGoCachePath: %v", err)
	}
	entry := storagepkg.QuarantineEntry{
		ID: "entry-cache", ResourceType: storagepkg.ResourceTypeGoCache,
		OriginalPath: original, QuarantinePath: quarantined,
		State:         storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC().Add(-2 * time.Hour),
		DeleteAfter:   time.Now().UTC().Add(-time.Hour),
		Metadata:      json.RawMessage(`{"ownership":"adopted"}`),
	}
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	deleted, err := controller.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	if deleted.State != storagepkg.QuarantineStateDeleted {
		t.Fatalf("state = %q, want deleted", deleted.State)
	}
	if _, err := os.Stat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("historical quarantine path still exists: %v", err)
	}
}

func TestQuarantineControllerRejectsEarlyGoCacheDeleteAndKeepsQuarantine(t *testing.T) {
	home := t.TempDir()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", "entry-cache")
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntry(
		t, store, original, quarantined, time.Now().UTC().Add(time.Hour),
	)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	_, err := controller.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if !errors.Is(err, storagepkg.ErrConflict) {
		t.Fatalf("PermanentDelete error = %v, want storage ErrConflict", err)
	}
	if _, err := os.Stat(quarantined); err != nil {
		t.Fatalf("quarantine path changed before deadline: %v", err)
	}
	got, err := store.GetQuarantineEntry(context.Background(), entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != storagepkg.QuarantineStateQuarantined {
		t.Fatalf("quarantine state = %q, want quarantined", got.State)
	}
}

func createGoCacheQuarantineEntry(
	t *testing.T,
	store *storagepkg.Store,
	original string,
	quarantined string,
	deleteAfter time.Time,
) storagepkg.QuarantineEntry {
	t.Helper()
	homeDir := filepath.Dir(filepath.Dir(filepath.Dir(quarantined)))
	ownership := "adopted"
	if filepath.Clean(original) == filepath.Join(homeDir, "cache", "go-build") {
		ownership = "managed"
	}
	metadata, err := json.Marshal(map[string]string{"ownership": ownership})
	if err != nil {
		t.Fatal(err)
	}
	entry := storagepkg.QuarantineEntry{
		ID: "entry-cache", ResourceType: storagepkg.ResourceTypeGoCache,
		OriginalPath: original, QuarantinePath: quarantined,
		State:         storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC().Add(-2 * time.Hour), DeleteAfter: deleteAfter,
		Metadata: metadata,
	}
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func createGoCacheQuarantineEntryWithID(
	t *testing.T,
	store *storagepkg.Store,
	home string,
	id string,
	deleteAfter time.Time,
) storagepkg.QuarantineEntry {
	t.Helper()
	original := filepath.Join(home, "cache", "go-build")
	quarantined := filepath.Join(home, "trash", "go-cache", id)
	if err := os.MkdirAll(quarantined, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantined, "artifact"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	ownership := "managed"
	metadata, err := json.Marshal(map[string]string{"ownership": ownership})
	if err != nil {
		t.Fatal(err)
	}
	entry := storagepkg.QuarantineEntry{
		ID: id, ResourceType: storagepkg.ResourceTypeGoCache,
		OriginalPath: original, QuarantinePath: quarantined,
		State:         storagepkg.QuarantineStateQuarantined,
		QuarantinedAt: time.Now().UTC().Add(-2 * time.Hour), DeleteAfter: deleteAfter,
		Metadata: metadata,
	}
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func markGoCacheQuarantineFailed(t *testing.T, store *storagepkg.Store, id string) {
	t.Helper()
	if _, err := store.TransitionQuarantineEntry(
		context.Background(), id, storagepkg.QuarantineStateFailed, "retryable failure",
	); err != nil {
		t.Fatalf("mark quarantine failed: %v", err)
	}
}

func newStorageMaintenanceStores(t *testing.T) (*storagepkg.SettingsStore, *storagepkg.Store) {
	t.Helper()
	connection, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "storage.db"))
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	pool := db.NewPool(connection, connection)
	t.Cleanup(func() { _ = pool.Close() })
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storagepkg.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	return storagepkg.NewSettingsStore(rawSettings), store
}

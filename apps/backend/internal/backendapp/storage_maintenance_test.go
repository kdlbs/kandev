package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		&overviewDockerClient{usage: agentdocker.DiskUsage{Containers: []agentdocker.ContainerUsage{
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
	if !ok || dockerSummary["managed_container_count"] != 1 || dockerSummary["managed_container_bytes"] != int64(64) {
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

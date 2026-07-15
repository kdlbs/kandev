package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type storageComposition struct {
	handler           *storagepkg.Handler
	runtime           *storagepkg.Runtime
	workspaceRestorer *workspaceQuarantineController
}

func provideStorageComposition(
	cfg *config.Config,
	pool *db.Pool,
	tracker *jobs.Tracker,
	lifecycleMgr *lifecycle.Manager,
	worktreeMgr *worktree.Manager,
	taskSvc *taskservice.Service,
) (*storageComposition, error) {
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		return nil, fmt.Errorf("initialize storage settings: %w", err)
	}
	settings := storagepkg.NewSettingsStore(rawSettings)
	store, err := storagepkg.NewStore(pool)
	if err != nil {
		return nil, fmt.Errorf("initialize storage store: %w", err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	taskSvc.SetTaskResourceCleanupActivityGate(&taskCleanupActivityGate{coordinator: coordinator})
	goCache := gocache.New(gocache.Config{
		HomeDir: cfg.ResolvedHomeDir(), TrashDir: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
		Settings: settings, Store: store,
	})
	lifecycleMgr.SetActivityCoordinator(coordinator)
	lifecycleMgr.SetManagedGoCacheEnvironmentProvider(goCache)
	if worktreeMgr != nil {
		worktreeMgr.SetScriptEnvironmentProvider(goCache)
	}

	inventory := &storageInventory{reader: pool.Reader(), worktrees: worktreeMgr, lifecycle: lifecycleMgr}
	workspaceFactory := newWorkspaceFactory(cfg, store, inventory, worktreeMgr)
	dockerClient := &lazyStorageDocker{provider: lifecycleMgr.DockerClientProvider(), activity: coordinator}
	dockerProvider := dockerstore.NewProvider(
		dockerClient, &containerInventory{reader: pool.Reader()}, settings,
	)
	overview := &storageOverview{
		settings: settings, quarantine: store, workspaceFactory: workspaceFactory, goCache: goCache,
		docker: dockerProvider, dockerClient: dockerClient, dockerHost: cfg.Docker.Host,
		homeDir: cfg.ResolvedHomeDir(),
	}
	providers := storageCleanupProviders(settings, workspaceFactory, goCache, dockerProvider)
	runner := storagepkg.NewRunner(storagepkg.RunnerConfig{
		Activity: coordinator, Store: store, Providers: providers,
	})
	scheduler := storagepkg.NewScheduler(settings, runner, storagepkg.SchedulerOptions{})
	runtime := storagepkg.NewRuntime(storagepkg.RuntimeConfig{
		Scheduler: scheduler, Settings: settings, Worker: taskSvc,
		Reconciler: &workspaceReconciler{settings: settings, factory: workspaceFactory},
	})
	quarantine := &workspaceQuarantineController{
		settings: settings, store: store, factory: workspaceFactory, homeDir: cfg.ResolvedHomeDir(),
		activity: coordinator,
	}
	operations := storagepkg.NewOperations(storagepkg.OperationsConfig{
		Settings: settings, Store: store, Jobs: tracker, Activity: coordinator,
		Providers: providers, Overview: overview, GoCache: goCache, Quarantine: quarantine,
	})
	handler := storagepkg.NewHandler(storagepkg.HandlerConfig{
		Settings: settings, Runs: store, Quarantine: store, Overview: overview,
		Mutations: operations, OnSettingsChanged: runtime.ApplySettings,
	})
	return &storageComposition{
		handler: handler, runtime: runtime, workspaceRestorer: quarantine,
	}, nil
}

type taskCleanupActivityGate struct {
	coordinator *activity.Coordinator
}

func (g *taskCleanupActivityGate) AcquireTaskResourceCleanup(
	ctx context.Context,
) (taskservice.TaskResourceCleanupActivityLease, error) {
	return g.coordinator.AcquireTask(ctx, activity.KindCleanupScript)
}

type workspaceFactory func(storagepkg.StorageMaintenanceSettings) *workspaces.Provider

func newWorkspaceFactory(
	cfg *config.Config,
	store *storagepkg.Store,
	inventory workspaces.InventorySource,
	pruner workspaces.WorktreePruner,
) workspaceFactory {
	return func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
		return workspaces.New(workspaces.Config{
			TasksRoot: filepath.Join(cfg.ResolvedHomeDir(), "tasks"),
			TrashRoot: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
			Inventory: inventory, Store: store, Pruner: pruner,
			GracePeriod: time.Duration(current.OrphanGraceHours) * time.Hour,
			Retention:   time.Duration(current.QuarantineRetentionHours) * time.Hour,
		})
	}
}

type quarantineSummarizer interface {
	SummarizeQuarantine(context.Context) (storagepkg.QuarantineSummary, error)
}

type storageOverview struct {
	settings         *storagepkg.SettingsStore
	quarantine       quarantineSummarizer
	workspaceFactory workspaceFactory
	goCache          *gocache.Provider
	docker           *dockerstore.Provider
	dockerClient     *lazyStorageDocker
	dockerHost       string
	homeDir          string
}

func (o *storageOverview) Summary(ctx context.Context) (storagepkg.Summary, error) {
	settings, err := o.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.Summary{}, err
	}
	workspaceSummary, workspaceErr := o.workspaceFactory(settings).Analyze(ctx)
	goCacheSummary, goCacheErr := o.goCache.Analyze(ctx)
	quarantineSummary, quarantineErr := o.quarantine.SummarizeQuarantine(ctx)
	dockerSummary := o.docker.Analyze(ctx)
	return storagepkg.Summary{
		Workspaces: summaryValue(workspaceSummary, workspaceErr),
		GoCache:    summaryValue(goCacheSummary, goCacheErr),
		Quarantine: summaryValue(quarantineSummary, quarantineErr),
		Docker: map[string]any{
			"available": dockerSummary.Available, "build_cache_bytes": dockerSummary.BuildCacheBytes,
			"unused_image_bytes": dockerSummary.UnusedImageBytes, "warnings": dockerSummary.Warnings,
			"managed_container_count": dockerSummary.ManagedContainerCount,
			"managed_container_bytes": dockerSummary.ManagedContainerBytes,
		},
	}, nil
}

func (o *storageOverview) Capabilities(
	ctx context.Context,
	settings storagepkg.StorageMaintenanceSettings,
) storagepkg.Capabilities {
	dockerAvailable := o.dockerClient.Ping(ctx) == nil
	goPath := settings.GoCache.AdoptedPath
	if goPath == "" {
		goPath = filepath.Join(o.homeDir, "cache", "go-build")
	}
	return storagepkg.Capabilities{
		ManagedGoCachePath: goPath, GoCacheAdoptionAvailable: true,
		DockerAvailable: dockerAvailable, DockerHost: o.dockerHost,
		HostGlobalDockerCleanup: dockerAvailable && settings.Docker.DedicatedDaemonAcknowledged,
	}
}

func summaryValue(value any, err error) any {
	if err == nil {
		return value
	}
	return map[string]any{"available": false, "warning": err.Error()}
}

type namedCleanupProvider struct {
	name    string
	cleanup func(context.Context) (map[string]any, error)
}

type goCacheCleanupProvider struct {
	provider *gocache.Provider
}

func (p goCacheCleanupProvider) Name() string { return "go_cache" }
func (p goCacheCleanupProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	result, err := p.provider.Cleanup(ctx)
	return toMap(result), err
}
func (p goCacheCleanupProvider) CleanupExplicit(ctx context.Context) (map[string]any, error) {
	result, err := p.provider.CleanupExplicit(ctx)
	return toMap(result), err
}

func (p namedCleanupProvider) Name() string { return p.name }
func (p namedCleanupProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	return p.cleanup(ctx)
}

func storageCleanupProviders(
	settings *storagepkg.SettingsStore,
	workspaceFactory workspaceFactory,
	goCache *gocache.Provider,
	docker *dockerstore.Provider,
) []storagepkg.CleanupProvider {
	return []storagepkg.CleanupProvider{
		workspaceCleanupAdapter(settings, workspaceFactory),
		goCacheCleanupProvider{provider: goCache},
		dockerContainerCleanupAdapter(settings, docker),
		dockerBuildCacheCleanupAdapter(settings, docker),
		dockerImageCleanupAdapter(settings, docker),
	}
}

func workspaceCleanupAdapter(
	settings *storagepkg.SettingsStore,
	factory workspaceFactory,
) storagepkg.CleanupProvider {
	return namedCleanupProvider{name: "workspaces", cleanup: func(ctx context.Context) (map[string]any, error) {
		current, err := settings.GetSettings(ctx)
		if err != nil || !current.Workspaces.Enabled {
			return nil, err
		}
		result, err := factory(current).Cleanup(ctx)
		return toMap(result), err
	}}
}

func dockerContainerCleanupAdapter(
	settings *storagepkg.SettingsStore,
	provider *dockerstore.Provider,
) storagepkg.CleanupProvider {
	return namedCleanupProvider{name: "kandev_containers", cleanup: func(ctx context.Context) (map[string]any, error) {
		current, err := settings.GetSettings(ctx)
		if err != nil || !current.KandevContainers.Enabled {
			return nil, err
		}
		return toMap(provider.CleanupContainers(ctx)), nil
	}}
}

func dockerBuildCacheCleanupAdapter(
	settings *storagepkg.SettingsStore,
	provider *dockerstore.Provider,
) storagepkg.CleanupProvider {
	return namedCleanupProvider{name: "docker_build_cache", cleanup: func(ctx context.Context) (map[string]any, error) {
		current, err := settings.GetSettings(ctx)
		if err != nil || !current.Docker.BuildCacheEnabled {
			return nil, err
		}
		result, err := provider.PruneBuildCache(ctx)
		return toMap(result), err
	}}
}

func dockerImageCleanupAdapter(
	settings *storagepkg.SettingsStore,
	provider *dockerstore.Provider,
) storagepkg.CleanupProvider {
	return namedCleanupProvider{name: "docker_unused_images", cleanup: func(ctx context.Context) (map[string]any, error) {
		current, err := settings.GetSettings(ctx)
		if err != nil || !current.Docker.UnusedImagesEnabled {
			return nil, err
		}
		result, err := provider.PruneUnusedImages(ctx)
		return toMap(result), err
	}}
}

func toMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(encoded, &result)
	return result
}

type workspaceReconciler struct {
	settings *storagepkg.SettingsStore
	factory  workspaceFactory
}

func (r *workspaceReconciler) Reconcile(ctx context.Context) error {
	settings, err := r.settings.GetSettings(ctx)
	if err != nil {
		return err
	}
	_, err = r.factory(settings).Reconcile(ctx)
	return err
}

type workspaceQuarantineController struct {
	settings *storagepkg.SettingsStore
	store    quarantineEntryStore
	factory  workspaceFactory
	homeDir  string
	activity *activity.Coordinator
	rename   func(string, string) error
}

type quarantineEntryStore interface {
	GetQuarantineEntry(context.Context, string) (storagepkg.QuarantineEntry, error)
	TransitionQuarantineEntry(
		context.Context, string, storagepkg.QuarantineState, string,
	) (storagepkg.QuarantineEntry, error)
}

const (
	goCacheOwnershipManaged = "managed"
	goCacheOwnershipAdopted = "adopted"
)

func (c *workspaceQuarantineController) RestoreTask(
	ctx context.Context,
	taskID string,
) workspaces.WorkspaceRecovery {
	current, err := c.settings.GetSettings(ctx)
	if err != nil {
		return workspaces.WorkspaceRecovery{TaskID: taskID, Status: "failed", Message: err.Error()}
	}
	return c.factory(current).RestoreTask(ctx, taskID)
}

func (c *workspaceQuarantineController) Restore(
	ctx context.Context,
	id string,
) (storagepkg.QuarantineEntry, error) {
	entry, err := c.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if entry.ResourceType == storagepkg.ResourceTypeGoCache {
		return c.restoreGoCache(ctx, entry)
	}
	if entry.ResourceType != storagepkg.ResourceTypeTaskWorkspace {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: unsupported quarantine resource %q", storagepkg.ErrValidation, entry.ResourceType)
	}
	settings, err := c.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	restored, err := c.factory(settings).Restore(ctx, id)
	if errors.Is(err, workspaces.ErrRestoreConflict) {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: %v", storagepkg.ErrConflict, err)
	}
	return restored, err
}

func (c *workspaceQuarantineController) PermanentDelete(
	ctx context.Context,
	id string,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	entry, err := c.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if entry.ResourceType == storagepkg.ResourceTypeGoCache {
		return c.deleteGoCache(ctx, entry, confirmation)
	}
	if entry.ResourceType != storagepkg.ResourceTypeTaskWorkspace {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: unsupported quarantine resource %q", storagepkg.ErrValidation, entry.ResourceType)
	}
	settings, err := c.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	return c.factory(settings).PermanentDelete(ctx, id, confirmation)
}

func (c *workspaceQuarantineController) restoreGoCache(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
) (storagepkg.QuarantineEntry, error) {
	if err := c.validateGoCacheEntry(ctx, entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	lease, err := c.acquireGoCacheMaintenance(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if lease != nil {
		defer lease.Release()
	}
	if err := c.rejectAmbiguousMissingGoCachePayload(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := c.prepareGoCacheRestoreDestination(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := c.renamePath(entry.QuarantinePath, entry.OriginalPath); err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("restore Go cache: %w", err)
	}
	return c.persistGoCacheRestore(ctx, entry)
}

func (c *workspaceQuarantineController) prepareGoCacheRestoreDestination(
	entry storagepkg.QuarantineEntry,
) error {
	if err := c.validateGoCacheRestorePath(entry.OriginalPath); err != nil {
		return err
	}
	if _, err := os.Lstat(entry.OriginalPath); err == nil {
		ownership, ownershipErr := goCacheEntryOwnership(entry)
		if ownershipErr != nil {
			return ownershipErr
		}
		removed, removeErr := gocache.RemoveRestorePlaceholder(
			entry.OriginalPath, ownership == goCacheOwnershipAdopted,
		)
		if removeErr != nil {
			return removeErr
		}
		if !removed {
			return fmt.Errorf("%w: Go-cache restore destination already exists", storagepkg.ErrConflict)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Go-cache restore destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o700); err != nil {
		return fmt.Errorf("create Go-cache restore parent: %w", err)
	}
	return nil
}

func (c *workspaceQuarantineController) persistGoCacheRestore(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
) (storagepkg.QuarantineEntry, error) {
	restored, err := c.store.TransitionQuarantineEntry(
		ctx, entry.ID, storagepkg.QuarantineStateRestored, "",
	)
	if err != nil {
		persistErr := fmt.Errorf("persist Go-cache restore: %w", err)
		if rollbackErr := c.renamePath(entry.OriginalPath, entry.QuarantinePath); rollbackErr != nil {
			return storagepkg.QuarantineEntry{}, errors.Join(
				persistErr, fmt.Errorf("rollback Go-cache restore: %w", rollbackErr),
			)
		}
		return storagepkg.QuarantineEntry{}, persistErr
	}
	return restored, nil
}

func (c *workspaceQuarantineController) renamePath(oldPath, newPath string) error {
	if c.rename != nil {
		return c.rename(oldPath, newPath)
	}
	return os.Rename(oldPath, newPath)
}

func (c *workspaceQuarantineController) acquireGoCacheMaintenance(
	ctx context.Context,
) (*activity.MaintenanceLease, error) {
	if c.activity == nil {
		return nil, nil
	}
	lease, busy, err := c.activity.TryAcquireMaintenance(ctx, 0)
	if errors.Is(err, activity.ErrBusy) {
		return nil, &storagepkg.BusyError{Resources: busy}
	}
	return lease, err
}

func (c *workspaceQuarantineController) deleteGoCache(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	if confirmation != "DELETE" {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: quarantine deletion requires DELETE confirmation", storagepkg.ErrValidation)
	}
	if err := c.validateGoCacheEntry(ctx, entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if time.Now().UTC().Before(entry.DeleteAfter) {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: quarantine retention deadline has not elapsed", storagepkg.ErrConflict)
	}
	if err := c.rejectAmbiguousMissingGoCachePayload(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := os.RemoveAll(entry.QuarantinePath); err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("delete quarantined Go cache: %w", err)
	}
	deleted, err := c.store.TransitionQuarantineEntry(
		ctx, entry.ID, storagepkg.QuarantineStateDeleted, "",
	)
	if err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("persist Go-cache deletion: %w", err)
	}
	return deleted, nil
}

func (c *workspaceQuarantineController) validateGoCacheEntry(
	_ context.Context,
	entry storagepkg.QuarantineEntry,
) error {
	if entry.State != storagepkg.QuarantineStateQuarantined &&
		entry.State != storagepkg.QuarantineStateFailed {
		return fmt.Errorf("%w: Go-cache quarantine entry is %q", storagepkg.ErrConflict, entry.State)
	}
	expectedQuarantine := filepath.Join(c.homeDir, "trash", "go-cache", entry.ID)
	if filepath.Clean(entry.QuarantinePath) != filepath.Clean(expectedQuarantine) {
		return fmt.Errorf("%w: Go-cache quarantine paths do not match managed storage", storagepkg.ErrValidation)
	}
	ownership, err := goCacheEntryOwnership(entry)
	if err != nil {
		return err
	}
	switch ownership {
	case goCacheOwnershipManaged:
		expectedOriginal := filepath.Join(c.homeDir, "cache", "go-build")
		if filepath.Clean(entry.OriginalPath) != filepath.Clean(expectedOriginal) {
			return fmt.Errorf("%w: managed Go-cache original path does not match owned storage", storagepkg.ErrValidation)
		}
	case goCacheOwnershipAdopted:
		original := filepath.Clean(entry.OriginalPath)
		if !filepath.IsAbs(original) || original == filepath.VolumeName(original)+string(filepath.Separator) {
			return fmt.Errorf("%w: adopted Go-cache original path is unsafe", storagepkg.ErrValidation)
		}
	default:
		return fmt.Errorf("%w: unknown Go-cache ownership policy %q", storagepkg.ErrValidation, ownership)
	}
	if err := storagepkg.ValidateNoSymlinkPath(c.homeDir, entry.QuarantinePath); err != nil {
		return fmt.Errorf("%w: validate Go-cache quarantine path: %v", storagepkg.ErrValidation, err)
	}
	return nil
}

func goCacheEntryOwnership(entry storagepkg.QuarantineEntry) (string, error) {
	var metadata struct {
		Ownership string `json:"ownership"`
	}
	if err := json.Unmarshal(entry.Metadata, &metadata); err != nil || metadata.Ownership == "" {
		return "", fmt.Errorf("%w: invalid Go-cache quarantine ownership metadata", storagepkg.ErrValidation)
	}
	return metadata.Ownership, nil
}

func (c *workspaceQuarantineController) rejectAmbiguousMissingGoCachePayload(
	entry storagepkg.QuarantineEntry,
) error {
	if entry.State != storagepkg.QuarantineStateFailed &&
		entry.State != storagepkg.QuarantineStateQuarantined {
		return nil
	}
	if _, err := os.Lstat(entry.OriginalPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect failed Go-cache original path: %w", err)
	}
	if _, err := os.Lstat(entry.QuarantinePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect failed Go-cache quarantine path: %w", err)
	}
	if err := c.validateGoCacheRestorePath(entry.OriginalPath); err != nil {
		return err
	}
	ownership, err := goCacheEntryOwnership(entry)
	if err != nil {
		return err
	}
	placeholder, err := gocache.IsRestorePlaceholder(
		entry.OriginalPath, ownership == goCacheOwnershipAdopted,
	)
	if err != nil {
		return fmt.Errorf("inspect Go-cache restore placeholder: %w", err)
	}
	if placeholder {
		return fmt.Errorf(
			"%w: quarantined Go-cache payload is missing and the original path is only a rotation placeholder",
			storagepkg.ErrConflict,
		)
	}
	return fmt.Errorf(
		"%w: quarantined Go-cache payload is missing and the populated original cannot be proven restored",
		storagepkg.ErrConflict,
	)
}

func (c *workspaceQuarantineController) validateGoCacheRestorePath(path string) error {
	anchor, err := storagepkg.CommonPath(c.homeDir, path)
	if err != nil {
		return fmt.Errorf("%w: resolve Go-cache restore safety anchor: %v", storagepkg.ErrValidation, err)
	}
	if err := storagepkg.ValidateNoSymlinkPath(anchor, path); err != nil {
		return fmt.Errorf("%w: validate Go-cache restore path: %v", storagepkg.ErrValidation, err)
	}
	return nil
}

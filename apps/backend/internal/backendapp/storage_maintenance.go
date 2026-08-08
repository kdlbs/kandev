package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type storageComposition struct {
	handler           *storagepkg.Handler
	runtime           *storagepkg.Runtime
	workspaceRestorer *workspaceQuarantineController
	tempArtifacts     *tempartifacts.Registry
}

func provideStorageComposition(
	cfg *config.Config,
	pool *db.Pool,
	tracker *jobs.Tracker,
	lifecycleMgr *lifecycle.Manager,
	worktreeMgr *worktree.Manager,
	taskSvc *taskservice.Service,
	log *logger.Logger,
	logError func(string, error),
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
	tempArtifacts := tempartifacts.NewRegistry(tempartifacts.Config{
		Store: store, TempRoot: os.TempDir(),
	})
	if err := tempArtifacts.Reconcile(context.Background()); err != nil {
		logError("reconcile temporary artifact registry", err)
	}
	tempProvider := tempartifacts.NewProvider(tempartifacts.ProviderConfig{
		Registry: tempArtifacts, Store: store, HomeDir: cfg.ResolvedHomeDir(),
		TrashDir: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
	})
	if err := tempProvider.Reconcile(context.Background()); err != nil {
		logError("reconcile temporary artifact quarantine", err)
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
		homeDir: cfg.ResolvedHomeDir(), tempArtifacts: tempProvider,
	}
	cachedOverview := storagepkg.NewOverviewCache(overview)
	quarantine := &workspaceQuarantineController{
		settings: settings, store: store, factory: workspaceFactory, homeDir: cfg.ResolvedHomeDir(),
		activity: coordinator, temporary: tempProvider,
	}
	providers := storageCleanupProviders(settings, workspaceFactory, goCache, dockerProvider, quarantine, tempProvider)
	if taskSvc.AttachmentService() == nil && taskSvc.AttachmentRepository() != nil {
		attachmentSvc, attachmentErr := taskservice.NewAttachmentService(
			taskSvc.AttachmentRepository(), cfg.ResolvedHomeDir(), taskSvc.AuthorizeWorkspaceAccess, log,
		)
		if attachmentErr != nil {
			return nil, fmt.Errorf("initialize prompt attachment storage: %w", attachmentErr)
		}
		taskSvc.SetAttachmentService(attachmentSvc)
	}
	if attachmentSvc := taskSvc.AttachmentService(); attachmentSvc != nil {
		if lifecycleMgr != nil {
			lifecycleMgr.SetAttachmentReader(attachmentSvc)
		}
		providers = append(providers, attachmentCleanupProvider{service: attachmentSvc})
	}
	runner := storagepkg.NewRunner(storagepkg.RunnerConfig{
		Activity: coordinator, Store: store, Providers: providers, Overview: cachedOverview,
	})
	scheduler := storagepkg.NewScheduler(settings, runner, storagepkg.SchedulerOptions{})
	runtime := storagepkg.NewRuntime(storagepkg.RuntimeConfig{
		Scheduler: scheduler, Settings: settings, Worker: taskSvc,
		Reconciler: &workspaceReconciler{settings: settings, factory: workspaceFactory},
	})
	operations := storagepkg.NewOperations(storagepkg.OperationsConfig{
		Settings: settings, Store: store, Jobs: tracker, Activity: coordinator,
		Providers: providers, Overview: cachedOverview, GoCache: goCache, Quarantine: quarantine,
	})
	handler := storagepkg.NewHandler(storagepkg.HandlerConfig{
		Settings: settings, Runs: store, Quarantine: store, Overview: cachedOverview,
		DiskCapacity: func(ctx context.Context, path string) (storagepkg.DiskCapacity, error) {
			capacity, err := systemmetrics.DiskUsage(ctx, path)
			if err != nil {
				return storagepkg.DiskCapacity{}, err
			}
			return storagepkg.DiskCapacity{
				TotalBytes: capacity.TotalBytes, UsedBytes: capacity.UsedBytes,
				AvailableBytes: capacity.AvailableBytes, UsedPercent: capacity.UsedPercent,
			}, nil
		},
		DiskPath:  cfg.ResolvedHomeDir(),
		Mutations: operations, OnSettingsChanged: runtime.ApplySettings, LogError: logError,
	})
	return &storageComposition{
		handler: handler, runtime: runtime, workspaceRestorer: quarantine, tempArtifacts: tempArtifacts,
	}, nil
}

type taskCleanupActivityGate struct {
	coordinator *activity.Coordinator
}

type attachmentCleanupProvider struct {
	service *taskservice.AttachmentService
}

func (p attachmentCleanupProvider) Name() string { return "prompt_attachments" }

func (p attachmentCleanupProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	deleted, err := p.service.CleanupExpired(ctx)
	return map[string]any{"deleted": deleted}, err
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
	workspaceAnalyze func(context.Context, storagepkg.StorageMaintenanceSettings) (workspaces.Analysis, error)
	goCache          *gocache.Provider
	goCacheAnalyze   func(context.Context) (gocache.Analysis, error)
	docker           *dockerstore.Provider
	tempArtifacts    *tempartifacts.Provider
	dockerClient     *lazyStorageDocker
	dockerHost       string
	homeDir          string
}

func (o *storageOverview) Summary(ctx context.Context) (storagepkg.Summary, error) {
	settings, err := o.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.Summary{}, err
	}
	var (
		workspaceSummary  workspaces.Analysis
		workspaceErr      error
		goCacheSummary    gocache.Analysis
		goCacheErr        error
		quarantineSummary storagepkg.QuarantineSummary
		quarantineErr     error
		dockerSummary     dockerstore.Analysis
		tempSummary       tempartifacts.Analysis
		tempErr           error
	)
	var measurements sync.WaitGroup
	measurements.Add(4)
	workspaceAnalyze := o.workspaceAnalyze
	if workspaceAnalyze == nil {
		workspaceAnalyze = func(ctx context.Context, settings storagepkg.StorageMaintenanceSettings) (workspaces.Analysis, error) {
			return o.workspaceFactory(settings).Analyze(ctx)
		}
	}
	goCacheAnalyze := o.goCacheAnalyze
	if goCacheAnalyze == nil {
		goCacheAnalyze = o.goCache.Analyze
	}
	go func() {
		defer measurements.Done()
		workspaceSummary, workspaceErr = workspaceAnalyze(ctx, settings)
	}()
	go func() {
		defer measurements.Done()
		goCacheSummary, goCacheErr = goCacheAnalyze(ctx)
	}()
	go func() {
		defer measurements.Done()
		quarantineSummary, quarantineErr = o.quarantine.SummarizeQuarantine(ctx)
	}()
	go func() {
		defer measurements.Done()
		dockerSummary = o.docker.Analyze(ctx)
	}()
	if o.tempArtifacts != nil {
		measurements.Add(1)
		go func() {
			defer measurements.Done()
			tempSummary, tempErr = o.tempArtifacts.Analyze(ctx)
		}()
	}
	measurements.Wait()
	return storagepkg.Summary{
		Workspaces:         summaryValue(workspaceSummary, workspaceErr),
		GoCache:            summaryValue(goCacheSummary, goCacheErr),
		Quarantine:         summaryValue(quarantineSummary, quarantineErr),
		TemporaryArtifacts: summaryValue(tempSummary, tempErr),
		Docker: map[string]any{
			"available": dockerSummary.Available, "build_cache_bytes": dockerSummary.BuildCacheBytes,
			"image_layer_bytes":  dockerSummary.ImageLayerBytes,
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
	return o.SettingsCapabilities(ctx, settings)
}

func (o *storageOverview) SettingsCapabilities(
	ctx context.Context,
	settings storagepkg.StorageMaintenanceSettings,
) storagepkg.Capabilities {
	goPath := settings.GoCache.AdoptedPath
	if goPath == "" {
		goPath = filepath.Join(o.homeDir, "cache", "go-build")
	}
	dockerAvailable := o.dockerClient != nil && o.dockerClient.Ping(ctx) == nil
	return storagepkg.Capabilities{
		ManagedGoCachePath: goPath, GoCacheAdoptionAvailable: true,
		TemporaryArtifactsAvailable: o.tempArtifacts != nil,
		DockerAvailable:             dockerAvailable, DockerHost: o.dockerHost,
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

type quarantinePurger interface {
	Purge(context.Context, storagepkg.QuarantinePurgeScope, string) (storagepkg.QuarantinePurgeResult, error)
}

type quarantineCleanupProvider struct {
	purger quarantinePurger
}

func (p quarantineCleanupProvider) Name() string { return "quarantine" }

func (p quarantineCleanupProvider) Cleanup(ctx context.Context) (map[string]any, error) {
	result, err := p.purger.Purge(ctx, storagepkg.QuarantinePurgeScopeEligible, storagepkg.QuarantineConfirmationEligible)
	return toMap(result), err
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
	quarantine quarantinePurger,
	temporary ...storagepkg.CleanupProvider,
) []storagepkg.CleanupProvider {
	providers := []storagepkg.CleanupProvider{
		quarantineCleanupProvider{purger: quarantine},
		workspaceCleanupAdapter(settings, workspaceFactory),
		workspaceDependencyCleanupAdapter(settings, workspaceFactory),
		goCacheCleanupProvider{provider: goCache},
		dockerContainerCleanupAdapter(settings, docker),
		dockerBuildCacheCleanupAdapter(settings, docker),
		dockerImageCleanupAdapter(settings, docker),
	}
	return append(providers, temporary...)
}

func workspaceDependencyCleanupAdapter(
	settings *storagepkg.SettingsStore,
	factory workspaceFactory,
) storagepkg.CleanupProvider {
	return namedCleanupProvider{name: "workspace_dependencies", cleanup: func(ctx context.Context) (map[string]any, error) {
		current, err := settings.GetSettings(ctx)
		if err != nil || !current.Workspaces.DependencyCleanupEnabled {
			return nil, err
		}
		result, err := factory(current).CleanupDependencies(ctx)
		return toMap(result), err
	}}
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
	settings  *storagepkg.SettingsStore
	store     quarantineEntryStore
	factory   workspaceFactory
	homeDir   string
	activity  *activity.Coordinator
	temporary *tempartifacts.Provider
	rename    func(string, string) error
}

type quarantineEntryStore interface {
	GetQuarantineEntry(context.Context, string) (storagepkg.QuarantineEntry, error)
	ListQuarantineEntries(context.Context, bool) ([]storagepkg.QuarantineEntry, error)
	TransitionQuarantineEntry(
		context.Context, string, storagepkg.QuarantineState, string,
	) (storagepkg.QuarantineEntry, error)
}

func (c *workspaceQuarantineController) Purge(
	ctx context.Context,
	scope storagepkg.QuarantinePurgeScope,
	confirmation string,
) (storagepkg.QuarantinePurgeResult, error) {
	result := storagepkg.QuarantinePurgeResult{Scope: scope}
	force := false
	switch scope {
	case storagepkg.QuarantinePurgeScopeEligible:
		if confirmation != storagepkg.QuarantineConfirmationEligible {
			return result, fmt.Errorf("%w: eligible quarantine purge requires %s confirmation", storagepkg.ErrValidation, storagepkg.QuarantineConfirmationEligible)
		}
	case storagepkg.QuarantinePurgeScopeAll:
		if confirmation != storagepkg.QuarantineConfirmationForce {
			return result, storagepkg.ErrForceDeleteConfirmation
		}
		force = true
	default:
		return result, fmt.Errorf("%w: unknown quarantine purge scope %q", storagepkg.ErrValidation, scope)
	}
	entries, err := c.store.ListQuarantineEntries(ctx, false)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	var purgeErrs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Considered++
		if !force && now.Before(entry.DeleteAfter) {
			result.Protected++
			result.ProtectedBytes += entry.SizeBytes
			continue
		}
		var deleted storagepkg.QuarantineEntry
		if force {
			deleted, err = c.PermanentDeleteForce(ctx, entry.ID, confirmation)
		} else {
			deleted, err = c.PermanentDelete(ctx, entry.ID, storagepkg.QuarantineConfirmationDelete)
		}
		if err != nil {
			result.Failed++
			result.FailedBytes += entry.SizeBytes
			result.Failures = append(result.Failures, storagepkg.QuarantinePurgeFailure{ID: entry.ID, Error: err.Error()})
			purgeErrs = append(purgeErrs, fmt.Errorf("%s: %w", entry.ID, err))
			continue
		}
		result.Deleted++
		result.DeletedBytes += deleted.SizeBytes
	}
	return result, errors.Join(purgeErrs...)
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
	if entry.ResourceType == storagepkg.ResourceTypeTemporaryArtifact {
		if c.temporary == nil {
			return storagepkg.QuarantineEntry{}, errors.New("temporary artifact provider is unavailable")
		}
		return c.temporary.Restore(ctx, id)
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
	return c.permanentDelete(ctx, id, confirmation, false)
}

func (c *workspaceQuarantineController) PermanentDeleteForce(
	ctx context.Context,
	id string,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	return c.permanentDelete(ctx, id, confirmation, true)
}

func (c *workspaceQuarantineController) permanentDelete(
	ctx context.Context,
	id string,
	confirmation string,
	force bool,
) (storagepkg.QuarantineEntry, error) {
	entry, err := c.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if entry.ResourceType == storagepkg.ResourceTypeGoCache {
		if force {
			return c.deleteGoCacheForce(ctx, entry, confirmation)
		}
		return c.deleteGoCache(ctx, entry, confirmation)
	}
	if entry.ResourceType == storagepkg.ResourceTypeTemporaryArtifact {
		if c.temporary == nil {
			return storagepkg.QuarantineEntry{}, errors.New("temporary artifact provider is unavailable")
		}
		if force {
			return c.temporary.PermanentDeleteForce(ctx, id, confirmation)
		}
		return c.temporary.PermanentDelete(ctx, id, confirmation)
	}
	if entry.ResourceType != storagepkg.ResourceTypeTaskWorkspace {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: unsupported quarantine resource %q", storagepkg.ErrValidation, entry.ResourceType)
	}
	settings, err := c.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if force {
		return c.factory(settings).PermanentDeleteForce(ctx, id, confirmation)
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
		return nil, &storagepkg.BusyError{
			Resources:      activity.BusyResourcesForKinds(busy),
			ForceAvailable: false,
		}
	}
	return lease, err
}

func (c *workspaceQuarantineController) deleteGoCache(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	return c.deleteGoCacheWithRetention(ctx, entry, confirmation, false)
}

func (c *workspaceQuarantineController) deleteGoCacheForce(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	return c.deleteGoCacheWithRetention(ctx, entry, confirmation, true)
}

func (c *workspaceQuarantineController) deleteGoCacheWithRetention(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
	confirmation string,
	force bool,
) (storagepkg.QuarantineEntry, error) {
	if force {
		if confirmation != storagepkg.QuarantineConfirmationForce {
			return storagepkg.QuarantineEntry{}, storagepkg.ErrForceDeleteConfirmation
		}
	} else if confirmation != storagepkg.QuarantineConfirmationDelete {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: quarantine deletion requires DELETE confirmation", storagepkg.ErrValidation)
	}
	if err := c.validateGoCacheEntry(ctx, entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if !force && time.Now().UTC().Before(entry.DeleteAfter) {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: quarantine retention deadline has not elapsed", storagepkg.ErrConflict)
	}
	if err := c.rejectAmbiguousMissingGoCachePayload(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := os.RemoveAll(entry.QuarantinePath); err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("delete quarantined Go cache: %w", err)
	}
	deleted, err := c.store.TransitionQuarantineEntry(
		context.WithoutCancel(ctx), entry.ID, storagepkg.QuarantineStateDeleted, "",
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

package backendapp

import (
	"context"
	"encoding/json"
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
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/jobs"
	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
	"github.com/kandev/kandev/internal/system/storage/filescan"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

const workspaceDependenciesProviderName = "workspace_dependencies"

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
	eventBus bus.EventBus,
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
	scanner := filescan.NewLimiter(4)
	tempProvider := tempartifacts.NewProvider(tempartifacts.ProviderConfig{
		Registry: tempArtifacts, Store: store, HomeDir: cfg.ResolvedHomeDir(),
		TrashDir: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
		Scanner:  scanner,
	})
	if err := tempProvider.Reconcile(context.Background()); err != nil {
		logError("reconcile temporary artifact quarantine", err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	taskSvc.SetTaskResourceCleanupActivityGate(&taskCleanupActivityGate{coordinator: coordinator})
	goCache := gocache.New(gocache.Config{
		HomeDir: cfg.ResolvedHomeDir(), TrashDir: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
		Settings: settings, Store: store, Scanner: scanner,
	})
	lifecycleMgr.SetActivityCoordinator(coordinator)
	lifecycleMgr.SetManagedGoCacheEnvironmentProvider(goCache)
	if worktreeMgr != nil {
		worktreeMgr.SetScriptEnvironmentProvider(goCache)
	}

	inventory := &storageInventory{reader: pool.Reader(), worktrees: worktreeMgr, lifecycle: lifecycleMgr}
	workspaceFactory := newWorkspaceFactory(cfg, store, inventory, worktreeMgr, scanner)
	dockerClient := &lazyStorageDocker{provider: lifecycleMgr.DockerClientProvider(), activity: coordinator}
	dockerProvider := dockerstore.NewProvider(
		dockerClient, &containerInventory{reader: pool.Reader()}, settings,
	)
	overview := &storageOverview{
		settings: settings, quarantine: store, workspaceFactory: workspaceFactory, goCache: goCache,
		docker: dockerProvider, dockerClient: dockerClient, dockerHost: cfg.Docker.Host,
		homeDir: cfg.ResolvedHomeDir(), tempArtifacts: tempProvider,
	}
	cachedOverview := storagepkg.NewOverviewCacheWithOptions(overview, storagepkg.OverviewCacheOptions{
		Publisher: func(update storagepkg.StorageAnalysisUpdated) {
			if eventBus == nil {
				return
			}
			if err := eventBus.Publish(
				context.Background(), events.SystemStorageAnalysisUpdated,
				bus.NewEvent(events.SystemStorageAnalysisUpdated, "storage-analysis", update),
			); err != nil {
				logError("publish storage analysis update", err)
			}
		},
		Observer: func(completion storagepkg.StorageAnalysisCompletion) {
			logStorageAnalysisCompletion(log, completion)
		},
	})
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
	scanner *filescan.Limiter,
) workspaceFactory {
	return func(current storagepkg.StorageMaintenanceSettings) *workspaces.Provider {
		return workspaces.New(workspaces.Config{
			TasksRoot: filepath.Join(cfg.ResolvedHomeDir(), "tasks"),
			TrashRoot: filepath.Join(cfg.ResolvedHomeDir(), "trash"),
			Inventory: inventory, Store: store, Pruner: pruner,
			GracePeriod: time.Duration(current.OrphanGraceHours) * time.Hour,
			Retention:   time.Duration(current.QuarantineRetentionHours) * time.Hour,
			Scanner:     scanner,
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
	return o.summary(ctx, nil)
}

func (o *storageOverview) SummaryWithProgress(
	ctx context.Context,
	notify storagepkg.OverviewProgressCallback,
) (storagepkg.Summary, error) {
	return o.summary(ctx, notify)
}

func (o *storageOverview) summary(
	ctx context.Context,
	notify storagepkg.OverviewProgressCallback,
) (storagepkg.Summary, error) {
	settings, err := o.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.Summary{}, err
	}
	reporter := newStorageProgressReporter(notify)
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
		reporter.start(storagepkg.StorageSourceWorkspaces)
		workspaceSummary, workspaceErr = o.analyzeWorkspaces(ctx, settings, workspaceAnalyze, reporter)
		reporter.complete(storagepkg.StorageSourceWorkspaces, summaryValue(workspaceSummary, workspaceErr), workspaceErr)
	}()
	go func() {
		defer measurements.Done()
		reporter.start(storagepkg.StorageSourceGoCache)
		goCacheSummary, goCacheErr = o.analyzeGoCache(ctx, goCacheAnalyze, reporter)
		reporter.complete(storagepkg.StorageSourceGoCache, summaryValue(goCacheSummary, goCacheErr), goCacheErr)
	}()
	go func() {
		defer measurements.Done()
		reporter.start(storagepkg.StorageSourceQuarantine)
		quarantineSummary, quarantineErr = o.quarantine.SummarizeQuarantine(ctx)
		reporter.complete(storagepkg.StorageSourceQuarantine, summaryValue(quarantineSummary, quarantineErr), quarantineErr)
	}()
	go func() {
		defer measurements.Done()
		reporter.start(storagepkg.StorageSourceDocker)
		dockerSummary = o.docker.Analyze(ctx)
		reporter.complete(storagepkg.StorageSourceDocker, dockerSummaryMap(dockerSummary), nil)
	}()
	if o.tempArtifacts != nil {
		measurements.Add(1)
		go func() {
			defer measurements.Done()
			reporter.start(storagepkg.StorageSourceTemporaryArtifacts)
			tempSummary, tempErr = o.analyzeTemporaryArtifacts(ctx, reporter)
			reporter.complete(
				storagepkg.StorageSourceTemporaryArtifacts,
				summaryValue(tempSummary, tempErr), tempErr,
			)
		}()
	} else {
		reporter.start(storagepkg.StorageSourceTemporaryArtifacts)
		reporter.complete(storagepkg.StorageSourceTemporaryArtifacts, tempSummary, nil)
	}
	measurements.Wait()
	if err := ctx.Err(); err != nil {
		return storagepkg.Summary{}, err
	}
	return summaryFromMeasurements(
		workspaceSummary, workspaceErr, goCacheSummary, goCacheErr,
		quarantineSummary, quarantineErr, tempSummary, tempErr, dockerSummary,
	), nil
}

func summaryFromMeasurements(
	workspaceSummary workspaces.Analysis,
	workspaceErr error,
	goCacheSummary gocache.Analysis,
	goCacheErr error,
	quarantineSummary storagepkg.QuarantineSummary,
	quarantineErr error,
	tempSummary tempartifacts.Analysis,
	tempErr error,
	dockerSummary dockerstore.Analysis,
) storagepkg.Summary {
	return storagepkg.Summary{
		Workspaces:         summaryValue(workspaceSummary, workspaceErr),
		GoCache:            summaryValue(goCacheSummary, goCacheErr),
		Quarantine:         summaryValue(quarantineSummary, quarantineErr),
		TemporaryArtifacts: summaryValue(tempSummary, tempErr),
		Docker:             dockerSummaryMap(dockerSummary),
	}
}

type workspaceProgressAnalyzer interface {
	AnalyzeWithProgress(context.Context, func(filescan.Progress)) (workspaces.Analysis, error)
}

type goCacheProgressAnalyzer interface {
	AnalyzeWithProgress(context.Context, func(filescan.Progress)) (gocache.Analysis, error)
}

type temporaryArtifactsProgressAnalyzer interface {
	AnalyzeWithProgress(context.Context, func(filescan.Progress)) (tempartifacts.Analysis, error)
}

var (
	_ workspaceProgressAnalyzer          = (*workspaces.Provider)(nil)
	_ goCacheProgressAnalyzer            = (*gocache.Provider)(nil)
	_ temporaryArtifactsProgressAnalyzer = (*tempartifacts.Provider)(nil)
)

func (o *storageOverview) analyzeWorkspaces(
	ctx context.Context,
	settings storagepkg.StorageMaintenanceSettings,
	fallback func(context.Context, storagepkg.StorageMaintenanceSettings) (workspaces.Analysis, error),
	reporter *storageProgressReporter,
) (workspaces.Analysis, error) {
	if o.workspaceAnalyze != nil {
		return fallback(ctx, settings)
	}
	provider := o.workspaceFactory(settings)
	return provider.AnalyzeWithProgress(ctx, reporter.filesystem(storagepkg.StorageSourceWorkspaces))
}

func (o *storageOverview) analyzeGoCache(
	ctx context.Context,
	fallback func(context.Context) (gocache.Analysis, error),
	reporter *storageProgressReporter,
) (gocache.Analysis, error) {
	if o.goCacheAnalyze != nil {
		return fallback(ctx)
	}
	return o.goCache.AnalyzeWithProgress(ctx, reporter.filesystem(storagepkg.StorageSourceGoCache))
}

func (o *storageOverview) analyzeTemporaryArtifacts(
	ctx context.Context,
	reporter *storageProgressReporter,
) (tempartifacts.Analysis, error) {
	return o.tempArtifacts.AnalyzeWithProgress(
		ctx, reporter.filesystem(storagepkg.StorageSourceTemporaryArtifacts),
	)
}

func dockerSummaryMap(summary dockerstore.Analysis) map[string]any {
	return map[string]any{
		"available": summary.Available, "build_cache_bytes": summary.BuildCacheBytes,
		"image_layer_bytes":  summary.ImageLayerBytes,
		"unused_image_bytes": summary.UnusedImageBytes, "warnings": summary.Warnings,
		"managed_container_count": summary.ManagedContainerCount,
		"managed_container_bytes": summary.ManagedContainerBytes,
	}
}

type storageProgressReporter struct {
	notify           storagepkg.OverviewProgressCallback
	mu               sync.Mutex
	sourceStarted    map[string]time.Time
	sourceCounters   map[string]storageSourceCounters
	sourcePartitions map[string]int
	activeWalkers    int
	maxActiveWalkers int
}

type storageSourceCounters struct {
	completedItems int
	totalItems     int
	hasTotalItems  bool
	bytesScanned   int64
}

func newStorageProgressReporter(notify storagepkg.OverviewProgressCallback) *storageProgressReporter {
	return &storageProgressReporter{
		notify: notify, sourceStarted: make(map[string]time.Time),
		sourceCounters:   make(map[string]storageSourceCounters),
		sourcePartitions: make(map[string]int),
	}
}

func (r *storageProgressReporter) sendLocked(progress storagepkg.OverviewProgress) {
	if r.notify != nil {
		r.notify(progress)
	}
}

func (r *storageProgressReporter) start(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sourceStarted[source] = time.Now()
	r.sendLocked(storagepkg.OverviewProgress{Source: source, State: storagepkg.SourceStateScanning})
}

func (r *storageProgressReporter) complete(source string, value any, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	duration := time.Duration(0)
	if started, ok := r.sourceStarted[source]; ok {
		duration = time.Since(started)
	}
	counters := r.sourceCounters[source]
	var totalItems *int
	if counters.hasTotalItems {
		totalItems = &counters.totalItems
	}
	state := storagepkg.SourceStateReady
	if err != nil {
		state = storagepkg.SourceStateFailed
	}
	r.sendLocked(storagepkg.OverviewProgress{
		Source: source, State: state, Value: value, Err: err, Duration: duration,
		CompletedItems: counters.completedItems, TotalItems: totalItems,
		BytesScanned: counters.bytesScanned,
	})
}

func (r *storageProgressReporter) filesystem(source string) func(filescan.Progress) {
	return func(progress filescan.Progress) {
		completed, total := progress.CompletedPartitions, progress.TotalPartitions
		if progress.Phase == filescan.RootCompleted {
			completed, total = progress.CompletedRoots, progress.TotalRoots
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if progress.Phase == filescan.PartitionStarted {
			r.activeWalkers++
			if r.activeWalkers > r.maxActiveWalkers {
				r.maxActiveWalkers = r.activeWalkers
			}
		}
		if progress.Phase == filescan.PartitionCompleted && r.activeWalkers > 0 {
			r.activeWalkers--
		}
		if progress.TotalPartitions > r.sourcePartitions[source] {
			r.sourcePartitions[source] = progress.TotalPartitions
		}
		partitionCount := 0
		for _, count := range r.sourcePartitions {
			partitionCount += count
		}
		maxActiveWalkers := r.maxActiveWalkers
		counters := r.sourceCounters[source]
		if completed > counters.completedItems {
			counters.completedItems = completed
		}
		if total > counters.totalItems {
			counters.totalItems = total
			counters.hasTotalItems = true
		}
		if progress.BytesScanned > counters.bytesScanned {
			counters.bytesScanned = progress.BytesScanned
		}
		r.sourceCounters[source] = counters
		var totalItems *int
		if counters.hasTotalItems {
			totalItems = &counters.totalItems
		}
		r.sendLocked(storagepkg.OverviewProgress{
			Source: source, State: storagepkg.SourceStateScanning,
			CompletedItems: counters.completedItems, TotalItems: totalItems,
			BytesScanned:   counters.bytesScanned,
			PartitionCount: partitionCount, MaxActiveWalkers: maxActiveWalkers,
		})
	}
}

func logStorageAnalysisCompletion(
	log *logger.Logger,
	completion storagepkg.StorageAnalysisCompletion,
) {
	if log == nil {
		return
	}
	outcomes := make(map[string]string, len(completion.SourceStates))
	durations := make(map[string]int64, len(completion.SourceDurations))
	for source, state := range completion.SourceStates {
		outcomes[source] = string(state)
	}
	for source, duration := range completion.SourceDurations {
		durations[source] = duration.Milliseconds()
	}
	log.Info("system.storage.analysis.completed",
		zap.Uint64("generation", completion.Generation),
		zap.Duration("duration", completion.Duration),
		zap.Any("source_durations_ms", durations),
		zap.Any("source_outcomes", outcomes),
		zap.Int("partition_count", completion.PartitionCount),
		zap.Int("max_active_walkers", completion.MaxActiveWalkers),
		zap.Bool("succeeded", completion.Succeeded),
	)
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
	return namedCleanupProvider{name: workspaceDependenciesProviderName, cleanup: func(ctx context.Context) (map[string]any, error) {
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

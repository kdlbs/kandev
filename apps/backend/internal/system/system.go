// Package system composes the System-pages backend: a top-level
// Provide that constructs the info, disk, database, backups, logs,
// updates, and jobs sub-services and registers the corresponding
// HTTP route group under /api/v1/system.
//
// The existing internal/health package continues to own
// GET /api/v1/system/health independently — this composer does not
// replace it. The wiring layer (cmd/kandev) registers health alongside
// this package.
package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/backups"
	"github.com/kandev/kandev/internal/system/database"
	"github.com/kandev/kandev/internal/system/disk"
	"github.com/kandev/kandev/internal/system/frontenderrors"
	"github.com/kandev/kandev/internal/system/info"
	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/kandev/kandev/internal/system/logbundle"
	"github.com/kandev/kandev/internal/system/metrics"
	"github.com/kandev/kandev/internal/system/queuesettings"
	"github.com/kandev/kandev/internal/system/restart"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/system/sleepinhibition"
	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/updates"
	"go.uber.org/zap"
)

const e2eNPMRegistryURLEnv = "KANDEV_E2E_NPM_REGISTRY_URL"

// BuildInfo holds the ldflag-injected build metadata that cmd/kandev
// passes to Provide.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// Wiring supplies the runtime hooks and repositories owned by the wider
// application. OrchestratorShutdown stops in-flight agent executions before
// destructive resets. DatabaseQuiesce stops database-backed workers before a
// factory reset mutates the shared schema. RestoreQuiesce stops the complete
// database-backed runtime before a SQLite restore closes the shared pool.
// TaskSessions is the authoritative session reader used by the install-wide
// sleep-inhibition service.
type Wiring struct {
	OrchestratorShutdown func()
	DatabaseQuiesce      func() error
	RestoreQuiesce       func() error
	MessageQueue         queuesettings.Target
	MessageQueueConfig   queuesettings.Configuration
	TaskSessions         sleepinhibition.SessionReader
}

// Service exposes the composed system sub-services. Each field is
// addressable so the cmd/kandev wiring can attach callbacks (Restart)
// after construction.
type Service struct {
	logger          *logger.Logger
	Info            *info.Service
	Jobs            *jobs.Tracker
	Disk            *disk.Service
	Database        *database.Service
	Backups         *backups.Service
	LogBundles      *logbundle.Service
	FrontendErrors  *frontenderrors.Service
	Metrics         *metrics.Service
	MessageQueue    *queuesettings.Service
	SleepInhibition *sleepinhibition.Service
	Updates         *updates.Service
	Restart         restart.Manager
	Storage         *storage.Handler
	// StorageRuntime owns the scheduler, reconciliation, and durable cleanup worker.
	StorageRuntime *storage.Runtime
}

// Provide constructs the composed Service. The HTTP routes are
// registered separately via RegisterRoutes and the updates poller is
// started via StartBackground, so callers can opt out (in tests, in
// CLI subcommands).
func Provide(cfg *config.Config, log *logger.Logger, pool *db.Pool, eventBus bus.EventBus, build BuildInfo, wiring Wiring) *Service {
	tracker := jobs.NewTracker(eventBus, log)
	dataDir := cfg.ResolvedDataDir()
	homeDir := cfg.ResolvedHomeDir()
	databasePath := cfg.Database.Path
	if databasePath == "" {
		databasePath = filepath.Join(dataDir, "kandev.db")
	}

	resetDirs := database.ResetDirs{
		Worktrees: filepath.Join(homeDir, "worktrees"),
		Repos:     filepath.Join(homeDir, "repos"),
		Sessions:  filepath.Join(homeDir, "sessions"),
		Tasks:     filepath.Join(homeDir, "tasks"),
		QuickChat: filepath.Join(homeDir, "quick-chat"),
	}
	dbSvc := database.NewService(pool, databasePath, resetDirs, tracker, log)
	dbSvc.OrchestratorShutdown = wiring.OrchestratorShutdown
	dbSvc.DatabaseQuiesce = wiring.DatabaseQuiesce

	backupsSvc := backups.NewService(databasePath, pool, tracker, log)
	backupsSvc.OrchestratorShutdown = wiring.OrchestratorShutdown
	backupsSvc.RestoreQuiesce = wiring.RestoreQuiesce

	settingsStore, err := systemsettings.NewStore(pool)
	if err != nil {
		log.Error("Failed to initialize system settings store", zap.Error(err))
	}
	var metricsSvc *metrics.Service
	var queueSettingsSvc *queuesettings.Service
	var sleepInhibitionSvc *sleepinhibition.Service
	updatesOpts := []updates.Option{updates.WithHomeDir(homeDir), updates.WithJobs(tracker)}
	if settingsStore != nil {
		metricsStore := metrics.NewStore(settingsStore)
		metricsSvc = metrics.NewService(metricsStore, metrics.NewCollector())
		if wiring.MessageQueue != nil {
			queueSettingsSvc = queuesettings.NewService(
				queuesettings.NewStore(settingsStore), wiring.MessageQueue, nil, log,
				wiring.MessageQueueConfig,
			)
		}
		if wiring.TaskSessions != nil {
			sleepInhibitionSvc = sleepinhibition.NewService(
				sleepinhibition.NewStore(settingsStore),
				wiring.TaskSessions,
				sleepinhibition.NewPlatformInhibitor(),
				eventBus,
				log,
			)
		}
		updatesOpts = append(updatesOpts, updates.WithSettingsStore(settingsStore))
	}

	updatesSvc := updates.NewService(pool, build.Version, nil, log, updatesOpts...)
	if registryURL := e2eNPMRegistryURL(); registryURL != "" {
		updatesSvc.SetNightlyURL(registryURL)
	}

	return &Service{
		logger:   log,
		Info:     info.NewService(build.Version, build.Commit, build.BuildTime),
		Jobs:     tracker,
		Disk:     disk.NewService(homeDir, tracker, log),
		Database: dbSvc,
		Backups:  backupsSvc,
		LogBundles: logbundle.New(logbundle.Config{
			HomeDir: homeDir, Version: build.Version, Commit: build.Commit,
			BuildTime: build.BuildTime, Log: log,
		}),
		FrontendErrors:  frontenderrors.New(log, nil),
		Metrics:         metricsSvc,
		MessageQueue:    queueSettingsSvc,
		SleepInhibition: sleepInhibitionSvc,
		Updates:         updatesSvc,
		Restart:         restart.NewManagerFromEnv(),
	}
}

func e2eNPMRegistryURL() string {
	if os.Getenv("KANDEV_E2E_MOCK") != "true" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(e2eNPMRegistryURLEnv))
}

// RegisterRoutes mounts every system endpoint under /api/v1/system.
// The health endpoint is mounted by internal/health.RegisterRoutes,
// not here, to keep the existing package's surface unchanged.
func (s *Service) RegisterRoutes(router *gin.Engine, log *logger.Logger) {
	g := router.Group("/api/v1/system")
	// Destructive install-wide mutations require the admin role. With
	// authentication disabled the synthetic single-user identity is an admin,
	// so behavior is unchanged; with it enabled, members are read-only here.
	admin := g.Group("", authn.RequireAdmin())

	g.GET("/info", info.Handler(s.Info))
	if s.Storage != nil {
		storage.RegisterRoutes(g, admin, s.Storage)
	}

	g.GET("/disk-usage", disk.HandleGet(s.Disk))
	g.POST("/disk-usage/refresh", disk.HandleRefresh(s.Disk))
	admin.POST("/disk-usage/open", disk.HandleOpenFolder(s.Disk))

	g.GET("/database", database.HandleStats(s.Database))
	admin.POST("/database/vacuum", database.HandleVacuum(s.Database))
	admin.POST("/database/optimize", database.HandleOptimize(s.Database))
	admin.POST("/database/reset", database.HandleReset(s.Database))

	backups.RegisterRoutes(g, admin, s.Backups)

	if s.FrontendErrors != nil {
		g.POST("/logs/frontend-errors", frontenderrors.Handle(s.FrontendErrors))
	}
	if s.LogBundles != nil {
		logbundle.RegisterRoutes(g, s.LogBundles)
	}

	if s.Metrics != nil {
		metrics.RegisterRoutes(g, s.Metrics)
	}
	if s.MessageQueue != nil {
		queuesettings.RegisterRoutes(g, admin, s.MessageQueue)
	}
	if s.SleepInhibition != nil {
		sleepinhibition.RegisterRoutes(g, admin, s.SleepInhibition)
	}

	g.GET("/updates", updates.HandleGet(s.Updates))
	admin.POST("/updates/check", updates.HandleCheck(s.Updates))
	admin.PATCH("/updates/channel", updates.HandleSetChannel(s.Updates))
	admin.POST("/updates/apply", updates.HandleApply(s.Updates))
	g.GET("/restart-capability", restart.HandleCapability(s.Restart))
	admin.POST("/restart", restart.HandleRequest(s.Restart))

	g.GET("/jobs/:id", jobs.HandleGet(s.Jobs))

	log.Debug("Registered System routes (HTTP)")
}

// StartBackground starts the System-owned pollers and reconciliation loops.
// They stop when the application cleanup path calls StopBackground.
func (s *Service) StartBackground(ctx context.Context) {
	if s.LogBundles != nil {
		s.LogBundles.Start(ctx)
	}
	if s.Updates != nil {
		s.Updates.StartPoller(ctx)
	}
	if s.StorageRuntime != nil {
		_ = s.StorageRuntime.Start(ctx)
	}
	if s.SleepInhibition != nil {
		if err := s.SleepInhibition.Start(ctx); err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to start sleep inhibition service", zap.Error(err))
			}
		}
	}
}

// StopBackground joins owned storage background workers.
func (s *Service) StopBackground() {
	if s.SleepInhibition != nil {
		s.SleepInhibition.Stop()
	}
	if s.LogBundles != nil {
		s.LogBundles.Stop()
	}
	if s.StorageRuntime != nil {
		s.StorageRuntime.Stop()
	}
}

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
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/backups"
	"github.com/kandev/kandev/internal/system/database"
	"github.com/kandev/kandev/internal/system/disk"
	"github.com/kandev/kandev/internal/system/info"
	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/kandev/kandev/internal/system/logs"
	"github.com/kandev/kandev/internal/system/metrics"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/system/updates"
	"go.uber.org/zap"
)

// BuildInfo holds the ldflag-injected build metadata that cmd/kandev
// passes to Provide.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// Wiring holds the runtime hooks the destructive endpoints need —
// currently only OrchestratorShutdown, which stops in-flight agent
// executions before factory reset wipes their backing data. The frontend
// dialog prompts the user to relaunch after a successful reset/restore;
// no in-process re-exec is performed.
type Wiring struct {
	OrchestratorShutdown func()
}

// Service exposes the composed system sub-services. Each field is
// addressable so the cmd/kandev wiring can attach callbacks (Restart)
// after construction.
type Service struct {
	Info     *info.Service
	Jobs     *jobs.Tracker
	Disk     *disk.Service
	Database *database.Service
	Backups  *backups.Service
	Logs     *logs.Service
	Metrics  *metrics.Service
	Updates  *updates.Service
}

// Provide constructs the composed Service. The HTTP routes are
// registered separately via RegisterRoutes and the updates poller is
// started via StartBackground, so callers can opt out (in tests, in
// CLI subcommands).
func Provide(cfg *config.Config, log *logger.Logger, pool *db.Pool, eventBus bus.EventBus, build BuildInfo, wiring Wiring) *Service {
	tracker := jobs.NewTracker(eventBus, log)
	dataDir := cfg.ResolvedDataDir()
	homeDir := cfg.ResolvedHomeDir()

	resetDirs := database.ResetDirs{
		Worktrees: filepath.Join(homeDir, "worktrees"),
		Repos:     filepath.Join(homeDir, "repos"),
		Sessions:  filepath.Join(homeDir, "sessions"),
		Tasks:     filepath.Join(homeDir, "tasks"),
		QuickChat: filepath.Join(homeDir, "quick-chat"),
	}
	dbSvc := database.NewService(pool, dataDir, resetDirs, tracker, log)
	dbSvc.OrchestratorShutdown = wiring.OrchestratorShutdown

	backupsSvc := backups.NewService(dataDir, pool, tracker, log)

	logDir := log.LogDirectory()
	logFile := log.LogFilename()
	settingsStore, err := systemsettings.NewStore(pool)
	if err != nil {
		log.Error("Failed to initialize system settings store", zap.Error(err))
	}
	var metricsSvc *metrics.Service
	if settingsStore != nil {
		metricsStore := metrics.NewStore(settingsStore)
		metricsSvc = metrics.NewService(metricsStore, metrics.NewCollector())
	}

	return &Service{
		Info:     info.NewService(build.Version, build.Commit, build.BuildTime),
		Jobs:     tracker,
		Disk:     disk.NewService(homeDir, tracker, log),
		Database: dbSvc,
		Backups:  backupsSvc,
		Logs:     logs.NewService(logDir, logFile, log),
		Metrics:  metricsSvc,
		Updates:  updates.NewService(pool, build.Version, nil, log, updates.WithHomeDir(homeDir), updates.WithJobs(tracker)),
	}
}

// RegisterRoutes mounts every system endpoint under /api/v1/system.
// The health endpoint is mounted by internal/health.RegisterRoutes,
// not here, to keep the existing package's surface unchanged.
func (s *Service) RegisterRoutes(router *gin.Engine, log *logger.Logger) {
	g := router.Group("/api/v1/system")

	g.GET("/info", info.Handler(s.Info))

	g.GET("/disk-usage", disk.HandleGet(s.Disk))
	g.POST("/disk-usage/refresh", disk.HandleRefresh(s.Disk))
	g.POST("/disk-usage/open", disk.HandleOpenFolder(s.Disk))

	g.GET("/database", database.HandleStats(s.Database))
	g.POST("/database/vacuum", database.HandleVacuum(s.Database))
	g.POST("/database/optimize", database.HandleOptimize(s.Database))
	g.POST("/database/reset", database.HandleReset(s.Database))

	backups.RegisterRoutes(g, s.Backups)

	g.GET("/logs", logs.HandleList(s.Logs))
	g.GET("/logs/tail", logs.HandleTail(s.Logs))
	g.GET("/logs/:name/download", logs.HandleDownload(s.Logs))

	if s.Metrics != nil {
		metrics.RegisterRoutes(g, s.Metrics)
	}

	g.GET("/updates", updates.HandleGet(s.Updates))
	g.POST("/updates/check", updates.HandleCheck(s.Updates))
	g.POST("/updates/apply", updates.HandleApply(s.Updates))

	g.GET("/jobs/:id", jobs.HandleGet(s.Jobs))

	log.Debug("Registered System routes (HTTP)")
}

// StartBackground kicks off the updates poller goroutine. The poller
// stops when ctx is cancelled.
func (s *Service) StartBackground(ctx context.Context) {
	if s.Updates != nil {
		s.Updates.StartPoller(ctx)
	}
}

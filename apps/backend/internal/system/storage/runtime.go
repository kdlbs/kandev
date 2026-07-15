package storage

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/health"
)

type CleanupWorker interface {
	StartTaskResourceCleanupWorker(context.Context) error
	StopTaskResourceCleanupWorker()
}

type StartupReconciler interface {
	Reconcile(context.Context) error
}

type RuntimeSettings interface {
	GetSettings(context.Context) (StorageMaintenanceSettings, error)
}

type RuntimeConfig struct {
	Scheduler  *Scheduler
	Settings   RuntimeSettings
	Worker     CleanupWorker
	Reconciler StartupReconciler
}

type Runtime struct {
	config RuntimeConfig
	mu     sync.Mutex
	ctx    context.Context
	issues []health.Issue
}

func NewRuntime(config RuntimeConfig) *Runtime {
	return &Runtime{config: config}
}

func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.ctx != nil {
		r.mu.Unlock()
		return nil
	}
	r.ctx = ctx
	r.mu.Unlock()
	if r.config.Worker != nil {
		if err := r.config.Worker.StartTaskResourceCleanupWorker(ctx); err != nil {
			r.setIssue("storage_cleanup_worker", "Task cleanup worker failed to start", err.Error())
		}
	}
	if r.config.Reconciler != nil {
		if err := r.config.Reconciler.Reconcile(ctx); err != nil {
			r.setIssue("storage_reconciliation", "Storage reconciliation failed", err.Error())
		}
	}
	settings, err := r.config.Settings.GetSettings(ctx)
	if err != nil {
		r.setIssue("storage_settings_invalid", "Storage maintenance is disabled", err.Error())
		return nil
	}
	if settings.Enabled && r.config.Scheduler != nil {
		return r.config.Scheduler.Start(ctx)
	}
	return nil
}

func (r *Runtime) Stop() {
	if r.config.Scheduler != nil {
		r.config.Scheduler.Stop()
	}
	if r.config.Worker != nil {
		r.config.Worker.StopTaskResourceCleanupWorker()
	}
	r.mu.Lock()
	r.ctx = nil
	r.mu.Unlock()
}

func (r *Runtime) ApplySettings(settings StorageMaintenanceSettings) {
	r.mu.Lock()
	ctx := r.ctx
	r.mu.Unlock()
	if r.config.Scheduler == nil || ctx == nil {
		return
	}
	if !settings.Enabled {
		r.config.Scheduler.Stop()
		return
	}
	if err := r.config.Scheduler.Start(ctx); err != nil {
		r.setIssue("storage_scheduler", "Storage scheduler failed to start", err.Error())
		return
	}
	r.config.Scheduler.ApplySettings(settings)
}

func (r *Runtime) Name() string     { return "Storage maintenance" }
func (r *Runtime) Category() string { return "storage" }

func (r *Runtime) Check(context.Context) []health.Issue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]health.Issue(nil), r.issues...)
}

func (r *Runtime) setIssue(id, title, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issues = append(r.issues, health.Issue{
		ID: id, Category: "storage", Title: title, Message: message,
		Severity: health.SeverityWarning, FixURL: "/settings/system/storage", FixLabel: "Review storage",
	})
}

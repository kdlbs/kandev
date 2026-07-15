package storage

import (
	"context"
	"sort"
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
	config      RuntimeConfig
	lifecycleMu sync.Mutex
	mu          sync.Mutex
	ctx         context.Context
	issues      map[string]health.Issue
}

func NewRuntime(config RuntimeConfig) *Runtime {
	return &Runtime{config: config}
}

func (r *Runtime) Start(ctx context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
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
		} else {
			r.clearIssue("storage_cleanup_worker")
		}
	}
	if r.config.Reconciler != nil {
		if err := r.config.Reconciler.Reconcile(ctx); err != nil {
			r.setIssue("storage_reconciliation", "Storage reconciliation failed", err.Error())
		} else {
			r.clearIssue("storage_reconciliation")
		}
	}
	settings, err := r.config.Settings.GetSettings(ctx)
	if err != nil {
		r.setIssue("storage_settings_invalid", "Storage maintenance is disabled", err.Error())
		return nil
	}
	r.clearIssue("storage_settings_invalid")
	if settings.Enabled && r.config.Scheduler != nil {
		if err := r.config.Scheduler.Start(ctx); err != nil {
			r.setIssue("storage_scheduler", "Storage scheduler failed to start", err.Error())
			return err
		}
	}
	r.clearIssue("storage_scheduler")
	return nil
}

func (r *Runtime) Stop() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
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
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	ctx := r.ctx
	r.mu.Unlock()
	if r.config.Scheduler == nil || ctx == nil {
		return
	}
	if !settings.Enabled {
		r.config.Scheduler.Stop()
		r.clearIssue("storage_scheduler")
		return
	}
	if err := r.config.Scheduler.Start(ctx); err != nil {
		r.setIssue("storage_scheduler", "Storage scheduler failed to start", err.Error())
		return
	}
	r.clearIssue("storage_scheduler")
	r.config.Scheduler.ApplySettings(settings)
}

func (r *Runtime) Name() string     { return "Storage maintenance" }
func (r *Runtime) Category() string { return "storage" }

func (r *Runtime) Check(context.Context) []health.Issue {
	r.mu.Lock()
	defer r.mu.Unlock()
	issues := make([]health.Issue, 0, len(r.issues))
	for _, issue := range r.issues {
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues
}

func (r *Runtime) setIssue(id, title, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.issues == nil {
		r.issues = make(map[string]health.Issue)
	}
	r.issues[id] = health.Issue{
		ID: id, Category: "storage", Title: title, Message: message,
		Severity: health.SeverityWarning, FixURL: "/settings/system/storage", FixLabel: "Review storage",
	}
}

func (r *Runtime) clearIssue(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.issues, id)
}

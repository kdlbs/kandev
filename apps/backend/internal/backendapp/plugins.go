package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/startup"
	"go.uber.org/zap"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/delivery"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
)

// pluginActivePluginsAdapter adapts *plugins.Service to delivery.PluginLister
// so the Deliverer can track which plugins should receive live deliveries
// (StatusActive) or buffer them (StatusError) without internal/plugins/delivery
// importing internal/plugins — see that package's doc comment ("backendapp
// adapts *plugins.Service to satisfy them").
type pluginActivePluginsAdapter struct {
	svc *plugins.Service
}

// ActivePlugins implements delivery.PluginLister.
func (a pluginActivePluginsAdapter) ActivePlugins() []delivery.PluginRecord {
	records := a.svc.Registry().List()
	out := make([]delivery.PluginRecord, 0, len(records))
	for _, rec := range records {
		if rec.Status != pluginstore.StatusActive && rec.Status != pluginstore.StatusError {
			continue
		}
		out = append(out, delivery.PluginRecord{
			ID:            rec.ID,
			EventSubjects: rec.Capabilities.Events,
			Status:        rec.Status,
		})
	}
	return out
}

type pluginStartupLifecycle interface {
	pluginToolCatalogLifecycle
	ReconcileStartupPluginInventory(context.Context) error
}

// startPluginsSubsystems attaches event delivery to svc and spawns every
// already-active, runtime-managed plugin (resuming plugins that were active
// before a backend restart), registering shutdown via addCleanup. Mirrors
// how the Jira/Linear/Sentry pollers are started in
// startAgentInfrastructure: construction happens in initPluginsService
// (services.go), lifecycle start happens here once ctx/addCleanup exist.
func startPluginsSubsystems(ctx context.Context, svc *plugins.Service, lifecycle pluginStartupLifecycle, eventBus bus.EventBus, log *logger.Logger, addCleanup func(func() error)) {
	transport := plugins.NewRuntimeTransport(svc.Runtime())
	deliverer := delivery.New(eventBus, transport, pluginActivePluginsAdapter{svc: svc}, log)
	svc.SetDeliverer(deliverer)
	addCleanup(func() error { deliverer.Stop(); return nil })
	toolRefresher := newPluginToolCatalogRefresher(ctx, svc, lifecycle, log)
	svc.SetAgentToolCatalogListener(toolRefresher)
	addCleanup(func() error { toolRefresher.Stop(); return nil })

	// Start the opt-in auto-update poller after the active plugins are spawned,
	// so its immediate first sweep sees them as StatusActive (only active,
	// opted-in plugins auto-update).
	autoUpdatePoller := plugins.NewAutoUpdatePoller(svc, log)
	workerCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	addCleanup(func() error {
		cancel()
		workers.Wait()
		autoUpdatePoller.Stop()
		svc.Shutdown()
		return nil
	})
	startup.AfterReady(workerCtx, func() {
		workers.Add(1)
		go func() {
			defer workers.Done()
			deliverer.Refresh()
			svc.StartActivePlugins(workerCtx)
			if workerCtx.Err() == nil {
				if err := lifecycle.ReconcileStartupPluginInventory(workerCtx); err != nil && workerCtx.Err() == nil {
					log.Warn("plugin startup operation reconciliation failed", zap.Error(err))
				}
			}
			if workerCtx.Err() == nil {
				autoUpdatePoller.Start(workerCtx)
			}
		}()
	})
	log.Info("Plugins subsystems started (event delivery + active plugin spawn + auto-update poller)")
}
